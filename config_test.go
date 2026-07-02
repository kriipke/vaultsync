package vaultsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVaultSyncConfigMappingShape(t *testing.T) {
	t.Parallel()

	data := []byte(`
root_dir: /srv/vault
syncs:
  - namespace: team-a
    vault_path: kv/app
    local_path: app
  - namespace: team-b
    vault_path: kv/db
    local_path: /abs/db
`)

	config, err := parseVaultSyncConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.RootDir != "/srv/vault" {
		t.Fatalf("expected root_dir to be parsed, got %q", config.RootDir)
	}

	if len(config.Syncs) != 2 {
		t.Fatalf("expected 2 syncs, got %d", len(config.Syncs))
	}

	if config.Syncs[0].Namespace != "team-a" || config.Syncs[0].LocalPath != "app" {
		t.Fatalf("unexpected first sync: %+v", config.Syncs[0])
	}
}

func TestParseVaultSyncConfigBareListShape(t *testing.T) {
	t.Parallel()

	data := []byte(`
- namespace: team-a
  vault_path: kv/app
  local_path: /abs/app
`)

	config, err := parseVaultSyncConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.RootDir != "" {
		t.Fatalf("expected empty root_dir for bare list, got %q", config.RootDir)
	}

	if len(config.Syncs) != 1 || config.Syncs[0].Namespace != "team-a" {
		t.Fatalf("unexpected syncs for bare list: %+v", config.Syncs)
	}
}

func TestParseVaultSyncConfigInvalidYAML(t *testing.T) {
	t.Parallel()

	if _, err := parseVaultSyncConfig([]byte("not: [valid")); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestNormalizeAndValidateConfigRootDir(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to determine home dir: %v", err)
	}

	tests := []struct {
		name    string
		rootDir string
		want    string
		wantErr bool
	}{
		{name: "empty stays empty", rootDir: "", want: ""},
		{name: "whitespace trimmed to empty", rootDir: "   ", want: ""},
		{name: "absolute kept", rootDir: "/srv/vault", want: "/srv/vault"},
		{name: "tilde expanded to home", rootDir: "~", want: home},
		{name: "tilde subpath expanded", rootDir: "~/vault", want: filepath.Join(home, "vault")},
		{name: "relative rejected", rootDir: "relative/path", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &VaultSyncConfig{RootDir: tt.rootDir}
			err := normalizeAndValidateConfig(config)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for root_dir %q, got nil", tt.rootDir)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if config.RootDir != tt.want {
				t.Fatalf("expected root_dir %q, got %q", tt.want, config.RootDir)
			}
		})
	}
}

func TestNormalizeAndValidateSyncTarget(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to determine home dir: %v", err)
	}

	tests := []struct {
		name          string
		sync          SyncTarget
		rootDir       string
		wantLocalPath string
		wantVaultPath string
		wantErr       bool
	}{
		{
			name:          "absolute local path kept",
			sync:          SyncTarget{Namespace: "team-a", VaultPath: "/kv/app/", LocalPath: "/abs/app"},
			rootDir:       "/srv/vault",
			wantLocalPath: "/abs/app",
			wantVaultPath: "kv/app",
		},
		{
			name:          "relative local path joined under root secrets dir",
			sync:          SyncTarget{Namespace: "team-a", VaultPath: "kv/app", LocalPath: "app"},
			rootDir:       "/srv/vault",
			wantLocalPath: filepath.Join("/srv/vault", secretsDirName, "app"),
			wantVaultPath: "kv/app",
		},
		{
			name:          "tilde local path expanded and used directly",
			sync:          SyncTarget{Namespace: "team-a", VaultPath: "kv/app", LocalPath: "~/secrets/app"},
			rootDir:       "/srv/vault",
			wantLocalPath: filepath.Join(home, "secrets", "app"),
			wantVaultPath: "kv/app",
		},
		{
			name:    "relative local path without root dir rejected",
			sync:    SyncTarget{Namespace: "team-a", VaultPath: "kv/app", LocalPath: "app"},
			rootDir: "",
			wantErr: true,
		},
		{
			name:    "missing namespace rejected",
			sync:    SyncTarget{Namespace: "  ", VaultPath: "kv/app", LocalPath: "/abs/app"},
			rootDir: "/srv/vault",
			wantErr: true,
		},
		{
			name:    "missing local path rejected",
			sync:    SyncTarget{Namespace: "team-a", VaultPath: "kv/app", LocalPath: ""},
			rootDir: "/srv/vault",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sync := tt.sync
			err := normalizeAndValidateSyncTarget(&sync, tt.rootDir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %+v)", sync)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sync.LocalPath != tt.wantLocalPath {
				t.Fatalf("expected local_path %q, got %q", tt.wantLocalPath, sync.LocalPath)
			}

			if sync.VaultPath != tt.wantVaultPath {
				t.Fatalf("expected vault_path %q, got %q", tt.wantVaultPath, sync.VaultPath)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to determine home dir: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde only", in: "~", want: home},
		{name: "tilde subpath", in: "~/foo/bar", want: filepath.Join(home, "foo", "bar")},
		{name: "absolute unchanged", in: "/etc/config", want: "/etc/config"},
		{name: "relative unchanged", in: "some/rel", want: "some/rel"},
		{name: "tilde without slash unchanged", in: "~user/foo", want: "~user/foo"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := expandPath(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("expandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to determine home dir: %v", err)
	}

	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(home, ".config", "vaultsync", "config.yaml")
	if got != want {
		t.Fatalf("expected config path %q, got %q", want, got)
	}

	if !strings.HasSuffix(got, filepath.Join("vaultsync", "config.yaml")) {
		t.Fatalf("unexpected config path suffix: %q", got)
	}
}
