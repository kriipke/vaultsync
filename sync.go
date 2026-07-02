package vaultsync

import (
	"errors"
	"fmt"
)

// DefaultKVEngine is the KV v2 secrets engine used for config-driven syncs when
// no engine is specified.
const DefaultKVEngine = "kv"

// ClientFactory constructs a VaultClient scoped to the given namespace. The
// default factory, NewVaultClientFromEnv, reads VAULT_ADDR/VAULT_TOKEN from the
// environment; tests inject their own to drive a mock transport.
type ClientFactory func(namespace string) (*VaultClient, error)

// RunPullAll pulls every sync target defined in cfg into its configured
// local_path. Each target's local_path is treated as the direct output root, so
// secret files are written there without an extension (matching the documented
// pull-all behavior). Failures are aggregated so a single unreachable target
// does not abort syncing of the others.
func RunPullAll(cfg *VaultSyncConfig, kvEngine string, newClient ClientFactory) error {
	return runSyncs(cfg, kvEngine, newClient, func(client *VaultClient, ref SecretRef, sync SyncTarget) error {
		return client.PullSecretsToFilesDirectAt(ref, sync.LocalPath)
	})
}

// RunPushAll pushes secret files from each sync target's local_path back to its
// configured Vault path. When dryRun is true, changes are previewed instead of
// written. As with RunPullAll, per-target failures are aggregated rather than
// fatal.
func RunPushAll(cfg *VaultSyncConfig, kvEngine string, dryRun bool, newClient ClientFactory) error {
	return runSyncs(cfg, kvEngine, newClient, func(client *VaultClient, ref SecretRef, sync SyncTarget) error {
		return client.PushSecretsFromFilesDirectAt(sync.LocalPath, ref, dryRun)
	})
}

func runSyncs(cfg *VaultSyncConfig, kvEngine string, newClient ClientFactory, action func(*VaultClient, SecretRef, SyncTarget) error) error {
	if cfg == nil {
		return errors.New("nil config")
	}

	if kvEngine == "" {
		kvEngine = DefaultKVEngine
	}

	if newClient == nil {
		newClient = NewVaultClientFromEnv
	}

	var resultErr error
	for i, sync := range cfg.Syncs {
		client, err := newClient(sync.Namespace)
		if err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sync entry %d (namespace %q): %w", i+1, sync.Namespace, err))
			continue
		}

		ref := NewSecretRef(kvEngine, sync.VaultPath)
		if err := action(client, ref, sync); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sync entry %d (%s -> %s): %w", i+1, sync.VaultPath, sync.LocalPath, err))
		}
	}

	return resultErr
}
