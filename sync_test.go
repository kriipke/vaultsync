package vaultsync

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newMockClient returns a VaultClient wired to a mock transport that serves a
// single secret ("db") under whatever base metadata path is listed, and records
// every write request it receives.
func newMockClient(t *testing.T, namespace string, writes *[]*http.Request) *VaultClient {
	t.Helper()

	client := NewVaultClient("https://vault.example", "token", namespace)
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{"keys": []string{"db"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/data/"):
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"username": namespace + "-user"},
				},
			})
		case (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.Contains(r.URL.Path, "/data/"):
			body, _ := io.ReadAll(r.Body)
			recorded := r.Clone(r.Context())
			recorded.Body = io.NopCloser(strings.NewReader(string(body)))
			*writes = append(*writes, recorded)
			return jsonResponse(t, http.StatusOK, map[string]any{"data": map[string]any{}})
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	return client
}

func TestRunPullAllWritesEachSyncTargetToItsLocalPath(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	cfg := &VaultSyncConfig{
		Syncs: []SyncTarget{
			{Namespace: "team-a", VaultPath: "app/database", LocalPath: dirA},
			{Namespace: "team-b", VaultPath: "shared/config", LocalPath: dirB},
		},
	}

	var seenNamespaces []string
	factory := func(namespace string) (*VaultClient, error) {
		seenNamespaces = append(seenNamespaces, namespace)
		return newMockClient(t, namespace, nil), nil
	}

	if err := RunPullAll(cfg, "kv", factory); err != nil {
		t.Fatalf("RunPullAll returned error: %v", err)
	}

	// A client must be constructed per configured namespace.
	if len(seenNamespaces) != 2 || seenNamespaces[0] != "team-a" || seenNamespaces[1] != "team-b" {
		t.Fatalf("expected per-namespace clients, got %v", seenNamespaces)
	}

	// Config-driven pulls write extensionless files directly under local_path.
	for _, dir := range []string{dirA, dirB} {
		contents, err := os.ReadFile(filepath.Join(dir, "db"))
		if err != nil {
			t.Fatalf("expected extensionless secret file in %s, got %v", dir, err)
		}
		if !strings.Contains(string(contents), "username:") {
			t.Fatalf("expected YAML secret contents in %s, got %q", dir, contents)
		}
	}
}

func TestRunPushAllSendsWriteRequestPerSecret(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "db"), []byte("username: alice\n"), 0644); err != nil {
		t.Fatalf("failed to seed secret file: %v", err)
	}

	cfg := &VaultSyncConfig{
		Syncs: []SyncTarget{
			{Namespace: "team-a", VaultPath: "app/database", LocalPath: dir},
		},
	}

	var writes []*http.Request
	factory := func(namespace string) (*VaultClient, error) {
		return newMockClient(t, namespace, &writes), nil
	}

	if err := RunPushAll(cfg, "kv", false, factory); err != nil {
		t.Fatalf("RunPushAll returned error: %v", err)
	}

	if len(writes) != 1 {
		t.Fatalf("expected exactly one write request, got %d", len(writes))
	}

	req := writes[0]
	if req.URL.Path != "/v1/kv/data/app/database/db" {
		t.Fatalf("unexpected write path: %s", req.URL.Path)
	}

	body, _ := io.ReadAll(req.Body)
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode write body: %v", err)
	}
	if payload.Data["username"] != "alice" {
		t.Fatalf("expected wrapped data payload, got %s", body)
	}
}

func TestRunPushAllDryRunSendsNoWriteRequests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "db"), []byte("username: alice\n"), 0644); err != nil {
		t.Fatalf("failed to seed secret file: %v", err)
	}

	cfg := &VaultSyncConfig{
		Syncs: []SyncTarget{
			{Namespace: "team-a", VaultPath: "app/database", LocalPath: dir},
		},
	}

	var writes []*http.Request
	factory := func(namespace string) (*VaultClient, error) {
		return newMockClient(t, namespace, &writes), nil
	}

	if err := RunPushAll(cfg, "kv", true, factory); err != nil {
		t.Fatalf("RunPushAll dry-run returned error: %v", err)
	}

	if len(writes) != 0 {
		t.Fatalf("expected no write requests during dry-run, got %d", len(writes))
	}
}

func TestRunPullAllContinuesAfterPerTargetFailure(t *testing.T) {
	dir := t.TempDir()

	cfg := &VaultSyncConfig{
		Syncs: []SyncTarget{
			{Namespace: "broken", VaultPath: "app/database", LocalPath: filepath.Join(dir, "a")},
			{Namespace: "ok", VaultPath: "shared/config", LocalPath: filepath.Join(dir, "b")},
		},
	}

	factory := func(namespace string) (*VaultClient, error) {
		if namespace == "broken" {
			return nil, io.ErrUnexpectedEOF
		}
		return newMockClient(t, namespace, nil), nil
	}

	err := RunPullAll(cfg, "kv", factory)
	if err == nil {
		t.Fatalf("expected aggregated error for failing target")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected error to mention failing namespace, got %v", err)
	}

	// The healthy target must still have been synced.
	if _, statErr := os.Stat(filepath.Join(dir, "b", "db")); statErr != nil {
		t.Fatalf("expected healthy target to sync despite sibling failure, got %v", statErr)
	}
}

func TestRunPullAllDefaultsKVEngine(t *testing.T) {
	dir := t.TempDir()

	cfg := &VaultSyncConfig{
		Syncs: []SyncTarget{
			{Namespace: "team-a", VaultPath: "app/database", LocalPath: dir},
		},
	}

	var listedPaths []string
	factory := func(namespace string) (*VaultClient, error) {
		client := NewVaultClient("https://vault.example", "token", namespace)
		client.Output = nil
		client.ErrOutput = nil
		client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.RawQuery == "list=true" {
				listedPaths = append(listedPaths, r.URL.Path)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{"keys": []string{}},
			})
		})}
		return client, nil
	}

	if err := RunPullAll(cfg, "", factory); err != nil {
		t.Fatalf("RunPullAll returned error: %v", err)
	}

	if len(listedPaths) == 0 || !strings.HasPrefix(listedPaths[0], "/v1/kv/metadata/") {
		t.Fatalf("expected default 'kv' engine in listed path, got %v", listedPaths)
	}
}
