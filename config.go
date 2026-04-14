package vaultsync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type SyncTarget struct {
	Namespace string `yaml:"namespace"`
	VaultPath string `yaml:"vault_path"`
	LocalPath string `yaml:"local_path"`
}

type VaultSyncConfig struct {
	RootDir string       `yaml:"root_dir"`
	Syncs   []SyncTarget `yaml:"syncs"`
}

const secretsDirName = "secrets"

func LoadVaultSyncConfig() (*VaultSyncConfig, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	config, err := parseVaultSyncConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	if err := normalizeAndValidateConfig(config); err != nil {
		return nil, err
	}

	for i := range config.Syncs {
		if err := normalizeAndValidateSyncTarget(&config.Syncs[i], config.RootDir); err != nil {
			return nil, fmt.Errorf("invalid sync entry %d: %w", i+1, err)
		}
	}

	if len(config.Syncs) == 0 {
		return nil, fmt.Errorf("%s does not contain any sync entries", configPath)
	}

	return config, nil
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(home, ".config", "vaultsync", "config.yaml"), nil
}

func parseVaultSyncConfig(data []byte) (*VaultSyncConfig, error) {
	var config VaultSyncConfig
	if err := yaml.Unmarshal(data, &config); err == nil && len(config.Syncs) > 0 {
		return &config, nil
	}

	var syncs []SyncTarget
	if err := yaml.Unmarshal(data, &syncs); err != nil {
		return nil, err
	}

	return &VaultSyncConfig{Syncs: syncs}, nil
}

func normalizeAndValidateConfig(config *VaultSyncConfig) error {
	config.RootDir = strings.TrimSpace(config.RootDir)
	if config.RootDir == "" {
		return nil
	}

	expandedRootDir, err := expandPath(config.RootDir)
	if err != nil {
		return fmt.Errorf("failed to expand root_dir %q: %w", config.RootDir, err)
	}

	if !filepath.IsAbs(expandedRootDir) {
		return fmt.Errorf("root_dir must be absolute, got %q", config.RootDir)
	}

	config.RootDir = expandedRootDir
	return nil
}

func normalizeAndValidateSyncTarget(sync *SyncTarget, rootDir string) error {
	sync.Namespace = strings.TrimSpace(sync.Namespace)
	sync.VaultPath = strings.Trim(strings.TrimSpace(sync.VaultPath), "/")
	sync.LocalPath = strings.TrimSpace(sync.LocalPath)

	if sync.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	if sync.LocalPath == "" {
		return fmt.Errorf("local_path is required")
	}

	expandedLocalPath, err := expandPath(sync.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to expand local_path %q: %w", sync.LocalPath, err)
	}

	if !filepath.IsAbs(expandedLocalPath) {
		if rootDir == "" {
			return fmt.Errorf("local_path must be absolute when root_dir is not set, got %q", sync.LocalPath)
		}

		expandedLocalPath = filepath.Join(rootDir, secretsDirName, expandedLocalPath)
	}

	sync.LocalPath = expandedLocalPath
	return nil
}

func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine home directory: %w", err)
		}

		if path == "~" {
			return home, nil
		}

		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}

	return path, nil
}
