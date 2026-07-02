package vaultsync

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// capturedRequest records the parts of an outbound request we want to assert on.
type capturedRequest struct {
	method    string
	path      string
	namespace string
	body      map[string]interface{}
}

func captureSingleRequest(t *testing.T, target *capturedRequest) roundTripFunc {
	t.Helper()

	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var parsed map[string]interface{}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("failed to parse request body %q: %v", string(body), err)
			}
		}

		*target = capturedRequest{
			method:    r.Method,
			path:      r.URL.Path,
			namespace: r.Header.Get("X-Vault-Namespace"),
			body:      parsed,
		}

		return textResponse(http.StatusOK, ""), nil
	})
}

func TestPutSecretAtSendsWrappedDataWrite(t *testing.T) {
	t.Parallel()

	var captured capturedRequest

	client := NewVaultClient("https://vault.example", "token", "team-a")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: captureSingleRequest(t, &captured)}

	ref := NewSecretRef("kv", "app/db")
	if err := client.PutSecretAt(ref, map[string]interface{}{"username": "alice"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.method != http.MethodPost {
		t.Fatalf("expected POST for KVv2 write, got %q", captured.method)
	}

	if captured.path != "/v1/kv/data/app/db" {
		t.Fatalf("expected KVv2 data path, got %q", captured.path)
	}

	if captured.namespace != "team-a" {
		t.Fatalf("expected namespace header, got %q", captured.namespace)
	}

	data, ok := captured.body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected body wrapped in data field, got %#v", captured.body)
	}

	if data["username"] != "alice" {
		t.Fatalf("expected wrapped secret payload, got %#v", data)
	}
}

func TestPutSecretAtReturnsHTTPErrorOnFailure(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "team-a")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return textResponse(http.StatusForbidden, "permission denied"), nil
	})}

	err := client.PutSecretAt(NewSecretRef("kv", "app/db"), map[string]interface{}{"k": "v"})
	if err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}

	if httpErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 status, got %d", httpErr.StatusCode)
	}
}

func TestPushSecretsFromFilesDirectAtWritesRealRequest(t *testing.T) {
	t.Parallel()

	inputDir := t.TempDir()
	secretFile := filepath.Join(inputDir, "db")
	if err := os.WriteFile(secretFile, []byte("username: alice\npassword: s3cret\n"), 0644); err != nil {
		t.Fatalf("failed to write fixture secret: %v", err)
	}

	var captured capturedRequest

	client := NewVaultClient("https://vault.example", "token", "team-a")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: captureSingleRequest(t, &captured)}

	ref := NewSecretRef("kv", "app")
	if err := client.PushSecretsFromFilesDirectAt(inputDir, ref, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.method != http.MethodPost {
		t.Fatalf("expected POST for KVv2 write, got %q", captured.method)
	}

	if captured.path != "/v1/kv/data/app/db" {
		t.Fatalf("expected direct KVv2 data path, got %q", captured.path)
	}

	data, ok := captured.body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected body wrapped in data field, got %#v", captured.body)
	}

	if data["username"] != "alice" || data["password"] != "s3cret" {
		t.Fatalf("expected full secret payload, got %#v", data)
	}
}

func TestPushSecretsFromFilesDirectAtDryRunSendsNoWrite(t *testing.T) {
	t.Parallel()

	inputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "db"), []byte("username: alice\n"), 0644); err != nil {
		t.Fatalf("failed to write fixture secret: %v", err)
	}

	disableExternalDiffTools(t)

	var writeSeen bool

	client := NewVaultClient("https://vault.example", "token", "team-a")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			writeSeen = true
			return textResponse(http.StatusOK, ""), nil
		}
		// Dry-run only reads existing state to build a diff.
		return textResponse(http.StatusNotFound, "not found"), nil
	})}

	if err := client.PushSecretsFromFilesDirectAt(inputDir, NewSecretRef("kv", "app"), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writeSeen {
		t.Fatal("expected dry-run to send no write requests")
	}
}
