package vaultsync

import (
	"os"
	"testing"
)

// TestVaultRoundTripIntegration exercises the real HTTP write/read path
// (PutSecretAt -> GetSecretAt) against a live Vault KV v2 engine. It is skipped
// unless VAULTSYNC_INTEGRATION=1 and VAULT_ADDR/VAULT_TOKEN are set, so it never
// runs during the normal unit-test suite.
//
// To run it against a local dev server:
//
//	vault server -dev -dev-root-token-id=root
//	export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root VAULTSYNC_INTEGRATION=1
//	vault secrets enable -path=kv kv-v2      # if the kv engine is not enabled yet
//	make test-integration
func TestVaultRoundTripIntegration(t *testing.T) {
	if os.Getenv("VAULTSYNC_INTEGRATION") != "1" {
		t.Skip("set VAULTSYNC_INTEGRATION=1 (with VAULT_ADDR/VAULT_TOKEN) to run")
	}

	// VAULT_NAMESPACE is optional; leave empty for an OSS dev server.
	client, err := NewVaultClientFromEnv(os.Getenv("VAULT_NAMESPACE"))
	if err != nil {
		t.Fatalf("NewVaultClientFromEnv: %v", err)
	}

	kvEngine := os.Getenv("VAULTSYNC_KV_ENGINE")
	if kvEngine == "" {
		kvEngine = "kv"
	}

	ref := NewSecretRef(kvEngine, "vaultsync-itest/roundtrip")
	want := map[string]interface{}{"username": "alice", "password": "s3cr3t"}

	if err := client.PutSecretAt(ref, want); err != nil {
		t.Fatalf("PutSecretAt: %v", err)
	}

	got, err := client.GetSecretAt(ref)
	if err != nil {
		t.Fatalf("GetSecretAt: %v", err)
	}

	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("missing key %q in read-back secret", k)
			continue
		}
		if gv != v {
			t.Errorf("key %q: got %v, want %v", k, gv, v)
		}
	}
}
