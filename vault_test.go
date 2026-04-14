package vaultsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSecretRefNormalizesPath(t *testing.T) {
	t.Parallel()

	ref := NewSecretRef("/kv/", "/app/config/")
	if ref.Engine != "kv" {
		t.Fatalf("expected normalized engine, got %q", ref.Engine)
	}

	if ref.Path != "app/config" {
		t.Fatalf("expected normalized path, got %q", ref.Path)
	}

	if got := ref.MetadataPath(); got != "kv/metadata/app/config" {
		t.Fatalf("expected metadata path, got %q", got)
	}
}

func TestDeprecatedStringAPIStillRoutesThroughSecretRefMethods(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"db"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/db":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"username": "alice"},
				},
			})
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	outputDir := t.TempDir()
	metadataPath := BuildMetadataPath("kv", "app")
	if metadataPath != "kv/metadata/app" {
		t.Fatalf("expected compatibility metadata path, got %q", metadataPath)
	}

	if err := client.PullSecretsToFiles(metadataPath, outputDir); err != nil {
		t.Fatalf("expected compatibility pull to succeed, got %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(outputDir, "app", "db.yaml"))
	if err != nil {
		t.Fatalf("expected compatibility write output, got %v", err)
	}

	if !strings.Contains(string(contents), "username: alice") {
		t.Fatalf("expected compatibility YAML contents, got %q", string(contents))
	}
}

func TestPullSecretsRecursivelyReturnsErrorOnSecretFetchFailure(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"good", "bad"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/good":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": "ok"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/bad":
			return textResponse(http.StatusInternalServerError, "boom"), nil
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	secrets, err := client.PullSecretsRecursivelyAt(NewSecretRef("kv", "app"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to get secret kv/metadata/app/bad") {
		t.Fatalf("expected secret-specific error, got %v", err)
	}

	if _, ok := secrets["kv/metadata/app/good"]; !ok {
		t.Fatalf("expected previously fetched secret to be preserved in partial result, got %#v", secrets)
	}
}

func TestPullSecretsToFilesReturnsErrorOnWriteFailure(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"db"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/db":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"username": "alice"},
				},
			})
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "blocked")
	if err := os.WriteFile(outputPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	err := client.PullSecretsToFilesAt(NewSecretRef("kv", "app"), outputPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to write secret kv/metadata/app/db") {
		t.Fatalf("expected write error to surface, got %v", err)
	}

	if !strings.Contains(err.Error(), "failed to create directory") && !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected filesystem failure details, got %v", err)
	}
}

func TestPullSecretsToFilesWritesFetchedSecretsBeforeReturningPullError(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"good", "bad"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/good":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"username": "alice"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/bad":
			return textResponse(http.StatusInternalServerError, "boom"), nil
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	outputDir := t.TempDir()
	err := client.PullSecretsToFilesAt(NewSecretRef("kv", "app"), outputDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to pull secrets") {
		t.Fatalf("expected pull error, got %v", err)
	}

	contents, readErr := os.ReadFile(filepath.Join(outputDir, "app", "good.yaml"))
	if readErr != nil {
		t.Fatalf("expected fetched secret to be written, got %v", readErr)
	}

	if !strings.Contains(string(contents), "username: alice") {
		t.Fatalf("expected written YAML contents, got %q", string(contents))
	}
}

func TestPullSecretsToFilesContinuesTraversalAfterSecretFetchFailure(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"bad", "good", "nested/"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app/nested" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"leaf"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/bad":
			return textResponse(http.StatusInternalServerError, "boom"), nil
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/good":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"username": "alice"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/nested/leaf":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": "leaf"},
				},
			})
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	outputDir := t.TempDir()
	err := client.PullSecretsToFilesAt(NewSecretRef("kv", "app"), outputDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to get secret kv/metadata/app/bad") {
		t.Fatalf("expected bad secret error, got %v", err)
	}

	goodContents, readErr := os.ReadFile(filepath.Join(outputDir, "app", "good.yaml"))
	if readErr != nil {
		t.Fatalf("expected later sibling secret to be written, got %v", readErr)
	}

	if !strings.Contains(string(goodContents), "username: alice") {
		t.Fatalf("expected good YAML contents, got %q", string(goodContents))
	}

	leafContents, readErr := os.ReadFile(filepath.Join(outputDir, "app", "nested", "leaf.yaml"))
	if readErr != nil {
		t.Fatalf("expected nested subtree secret to be written, got %v", readErr)
	}

	if !strings.Contains(string(leafContents), "value: leaf") {
		t.Fatalf("expected nested YAML contents, got %q", string(leafContents))
	}
}

func TestPullSecretsToFilesWritesSecretsInDeterministicOrderBeforeWriteFailure(t *testing.T) {
	t.Parallel()

	client := NewVaultClient("https://vault.example", "token", "namespace")
	client.Output = nil
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/metadata/app" && r.URL.RawQuery == "list=true":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"keys": []string{"zeta", "alpha/nested"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/zeta":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": "z"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/alpha/nested":
			return jsonResponse(t, http.StatusOK, map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": "a"},
				},
			})
		default:
			return textResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	outputDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outputDir, "app", "zeta.yaml"), 0o755); err != nil {
		t.Fatalf("failed to create blocking directory: %v", err)
	}

	err := client.PullSecretsToFilesAt(NewSecretRef("kv", "app"), outputDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to write secret kv/metadata/app/zeta") {
		t.Fatalf("expected deterministic first write failure, got %v", err)
	}

	alphaContents, readErr := os.ReadFile(filepath.Join(outputDir, "app", "alpha", "nested.yaml"))
	if readErr != nil {
		t.Fatalf("expected earlier lexicographic secret to be written before failure, got %v", readErr)
	}

	if !strings.Contains(string(alphaContents), "value: a") {
		t.Fatalf("expected alpha YAML contents, got %q", string(alphaContents))
	}

	if _, statErr := os.Stat(filepath.Join(outputDir, "app", "zeta.yaml")); statErr != nil {
		t.Fatalf("expected blocking zeta directory to remain present, got %v", statErr)
	}
}

func TestShowDryRunDiffReturnsErrorOnVaultFailure(t *testing.T) {
	disableExternalDiffTools(t)

	client := NewVaultClient("https://vault.example", "token", "namespace")
	var stdout bytes.Buffer
	client.Output = &stdout
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/db" {
			return textResponse(http.StatusInternalServerError, "vault unavailable"), nil
		}
		return textResponse(http.StatusNotFound, "not found"), nil
	})}

	err := client.showDryRunDiff("kv/metadata/app/db", map[string]any{"username": "alice"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to get existing secret kv/metadata/app/db") {
		t.Fatalf("expected wrapped dry-run error, got %v", err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("expected no diff output on vault failure, got %q", stdout.String())
	}
}

func TestShowDryRunDiffTreatsNotFoundAsNewSecret(t *testing.T) {
	disableExternalDiffTools(t)

	client := NewVaultClient("https://vault.example", "token", "namespace")
	var stdout bytes.Buffer
	client.Output = &stdout
	client.ErrOutput = nil
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/kv/data/app/db" {
			return textResponse(http.StatusNotFound, "missing"), nil
		}
		return textResponse(http.StatusNotFound, "not found"), nil
	})}

	err := client.showDryRunDiff("kv/metadata/app/db", map[string]any{"username": "alice"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "new file mode 100644") {
		t.Fatalf("expected new-file diff output, got %q", output)
	}

	if !strings.Contains(output, "+++ b/kv/metadata/app/db") {
		t.Fatalf("expected diff path in output, got %q", output)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(t *testing.T, status int, payload map[string]any) (*http.Response, error) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		t.Fatalf("failed to encode response: %v", err)
	}

	resp := textResponse(status, string(body))
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func disableExternalDiffTools(t *testing.T) {
	t.Helper()

	originalTool := diffTool
	originalDetected := diffToolDetected
	diffTool = ""
	diffToolDetected = true

	t.Cleanup(func() {
		diffTool = originalTool
		diffToolDetected = originalDetected
	})
}
