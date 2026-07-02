package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: vaultsync") {
		t.Fatalf("expected usage output, got %q", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"frobnicate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Unknown command: frobnicate") {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
}

func TestRunListWithoutNamespace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"list"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "list <namespace>") {
		t.Fatalf("expected list usage, got %q", stderr.String())
	}
}

func TestRunMissingEnvReturnsError(t *testing.T) {
	// VAULT_ADDR/VAULT_TOKEN are unset in the test environment, so a valid
	// command should surface the configuration error and exit non-zero
	// rather than panic.
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"list", "my-namespace"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "VAULT_ADDR") {
		t.Fatalf("expected VAULT_ADDR error, got %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "vaultsync ") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestLooksLikePath(t *testing.T) {
	cases := map[string]bool{
		"./secrets":  true,
		"../secrets": true,
		"/abs/path":  true,
		"~/home":     true,
		"app":        false,
		"app/db":     false,
		"secrets":    false,
	}
	for in, want := range cases {
		if got := looksLikePath(in); got != want {
			t.Errorf("looksLikePath(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParsePullArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    pullArgs
		wantErr bool
	}{
		{
			name: "namespace only defaults output dir",
			args: []string{"ns"},
			want: pullArgs{namespace: "ns", subPath: "", outputDir: "./secrets"},
		},
		{
			name: "namespace and subpath",
			args: []string{"ns", "app"},
			want: pullArgs{namespace: "ns", subPath: "app", outputDir: "./secrets"},
		},
		{
			name: "namespace subpath and output dir",
			args: []string{"ns", "app", "./out"},
			want: pullArgs{namespace: "ns", subPath: "app", outputDir: "./out"},
		},
		{
			name: "namespace and output dir (path-like second arg)",
			args: []string{"ns", "./out"},
			want: pullArgs{namespace: "ns", subPath: "", outputDir: "./out"},
		},
		{
			name:    "no args is an error",
			args:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePullArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parsePullArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParsePushArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    pushArgs
		wantErr bool
	}{
		{
			name: "namespace only",
			args: []string{"ns"},
			want: pushArgs{namespace: "ns", inputDir: "./secrets"},
		},
		{
			name: "dry-run flag after namespace",
			args: []string{"ns", "--dry-run"},
			want: pushArgs{namespace: "ns", inputDir: "./secrets", dryRun: true},
		},
		{
			name: "subpath input dir and dry-run interspersed",
			args: []string{"ns", "app", "./in", "--dry-run"},
			want: pushArgs{namespace: "ns", subPath: "app", inputDir: "./in", dryRun: true},
		},
		{
			name: "dry-run before positionals",
			args: []string{"--dry-run", "ns", "app"},
			want: pushArgs{namespace: "ns", subPath: "app", inputDir: "./secrets", dryRun: true},
		},
		{
			name:    "no positional args is an error",
			args:    []string{"--dry-run"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePushArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parsePushArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestGlobalKVEngineFlagParsed(t *testing.T) {
	// --kv-engine before the command should be consumed, leaving the command
	// usage to fire (namespace missing) rather than an "unknown command".
	var stdout, stderr bytes.Buffer
	code := run([]string{"--kv-engine=secrets", "list"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "list <namespace>") {
		t.Fatalf("expected list usage after global flag, got %q", stderr.String())
	}
}
