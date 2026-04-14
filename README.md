# VaultSync

An importable Go library plus CLI for syncing secrets between HashiCorp Vault and local YAML files, with support for HCP (HashiCorp Cloud Platform) Vault.

## Features

- **List secrets** from Vault KVv2 engines
- **Pull secrets** to local YAML files with directory structure mirroring
- **Push secrets** back to Vault from YAML files
- **Bulk pull/push** via a config file at `~/.config/vaultsync/config.yaml`
- **Dry-run mode** with diff output (enhanced with delta/difftastic if available)
- **HCP Vault support** with enterprise namespaces
- **Automatic diff tool detection** (delta, difftastic, diff-so-fancy)

## Installation

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/username/vaultsync/releases).

#### Linux/macOS
```bash
# Download and extract (replace with your platform)
curl -L https://github.com/username/vaultsync/releases/latest/download/vaultsync-linux-amd64.tar.gz | tar xz

# Make executable and move to PATH
chmod +x vaultsync
sudo mv vaultsync /usr/local/bin/
```

#### Windows
Download the `.zip` file and extract `vaultsync.exe` to a directory in your PATH.

### Build from Source

```bash
git clone https://github.com/username/vaultsync.git
cd vaultsync
go build -o vaultsync ./cmd/vaultsync
```

## Library Usage

The repository root is now `package vaultsync`, so other Go CLI applications can import and reuse the Vault sync logic directly.

```go
package main

import (
	"log"

	"vaultsync"
)

func main() {
	client, err := vaultsync.NewVaultClientFromEnv("my-namespace")
	if err != nil {
		log.Fatal(err)
	}

	ref := vaultsync.NewSecretRef("kv", "app")

	if err := client.PullSecretsToFilesAt(ref, "./secrets"); err != nil {
		log.Fatal(err)
	}
}
```

Useful exported entry points:

- `vaultsync.NewVaultClient(address, token, namespace)`
- `vaultsync.NewVaultClientFromEnv(namespace)`
- `vaultsync.NewSecretRef(kvEngine, path)`
- `(*vaultsync.VaultClient).ListSecretsAt(...)`
- `(*vaultsync.VaultClient).GetSecretAt(...)`
- `(*vaultsync.VaultClient).PutSecretAt(...)`
- `(*vaultsync.VaultClient).PullSecretsToFilesAt(...)`
- `(*vaultsync.VaultClient).PushSecretsFromFilesAt(...)`
- `vaultsync.LoadVaultSyncConfig()`

## Usage

### Environment Variables

```bash
export VAULT_ADDR="https://your-vault.example.com:8200"
export VAULT_TOKEN="your-vault-token"
```

For HCP Vault:
```bash
export VAULT_ADDR="https://myvault.hashicorp.cloud:8200"
export VAULT_TOKEN="your-hcp-token"
```

### Commands

#### List Secrets
```bash
vaultsync [--kv-engine=name] list <namespace> [path]

# Examples
vaultsync list my-namespace                    # list all secrets in default 'kv' engine
vaultsync list my-namespace app                # list secrets under 'app' path
vaultsync --kv-engine=secrets list my-namespace app  # use 'secrets' engine instead of 'kv'
```

#### Pull Secrets to Files
```bash
vaultsync [--kv-engine=name] pull <namespace> [path] [output-dir]

# Examples
vaultsync pull my-namespace                     # pull all from 'kv' to ./secrets/
vaultsync pull my-namespace app                 # pull 'app' path to ./secrets/app/
vaultsync pull my-namespace app ./secrets      # pull 'app' path to ./secrets/app/
vaultsync --kv-engine=secrets pull my-namespace app  # use 'secrets' engine
```

#### Push Secrets from Files
```bash
vaultsync [--kv-engine=name] push <namespace> [path] [input-dir] [--dry-run]

# Examples
vaultsync push my-namespace --dry-run           # dry-run all from ./secrets/
vaultsync push my-namespace                     # push all from ./secrets/
vaultsync push my-namespace app --dry-run       # dry-run 'app' path from ./secrets/app/
vaultsync push my-namespace app ./secrets      # push 'app' from ./secrets/app/
```

#### Bulk Pull and Push from Config
```bash
vaultsync [--kv-engine=name] pull-all
vaultsync [--kv-engine=name] push-all [--dry-run]
```

Config-driven syncs write files directly into each configured `local_path` and do not add a `.yaml` suffix.

Config file location:

```bash
~/.config/vaultsync/config.yaml
```

Supported YAML formats:

```yaml
syncs:
  - namespace: team-a
    vault_path: app/database
    local_path: /absolute/path/to/team-a-secrets
  - namespace: team-b
    vault_path: shared/config
    local_path: /absolute/path/to/team-b-secrets
```

```yaml
- namespace: team-a
  vault_path: app/database
  local_path: /absolute/path/to/team-a-secrets
- namespace: team-b
  vault_path: shared/config
  local_path: /absolute/path/to/team-b-secrets
```

`vault_path` is relative to the selected KV engine. `local_path` must be absolute unless `root_dir` is set, in which case relative values are resolved under `<root_dir>/secrets`.

You can also define a top-level `root_dir` and use relative `local_path` values. In that mode, VaultSync resolves each sync target under `<root_dir>/secrets`:

```yaml
root_dir: ~/vaultsync-demo
syncs:
  - namespace: team-a
    vault_path: app/database
    local_path: dev
  - namespace: team-b
    vault_path: shared/config
    local_path: qa
```

The example above writes to `~/vaultsync-demo/secrets/dev` and `~/vaultsync-demo/secrets/qa`.

For config-driven syncs, the configured `local_path` is the direct root for that Vault path. For example, if `vault_path` is `kubernetes/dev/example-app` and `local_path` resolves to `~/vaultsync-demo/secrets/dev`, then `ls ~/vaultsync-demo/secrets/dev` will show the secret files immediately instead of another nested `kubernetes/dev/example-app` directory tree.

### Workflow Example

```bash
# 1. Pull secrets from Vault
vaultsync pull my-namespace

# 2. Edit files in ./secrets/
# Files are organized like: ./secrets/app/database.yaml

# 3. Preview changes with enhanced diff
vaultsync push my-namespace --dry-run | delta

# 4. Push changes back to Vault
vaultsync push my-namespace
```

## Directory Structure

Direct `pull` and `push` commands mirror the requested Vault path and use `.yaml` files:

**Vault Path → File Path (default 'kv' engine)**
- `kv/app/database` → `./secrets/app/database.yaml`
- `kv/shared/config` → `./secrets/shared/config.yaml`

**For specific subpaths:**
- Pull from `app` → Files in `./secrets/app/`
- Push to `app` → Reads files from `./secrets/app/`

**Custom KV engines:**
- `--kv-engine=secrets` with path `app/db` → `./secrets/app/db.yaml`

Config-driven `pull-all` and `push-all` use each configured local directory as the direct root and write extensionless files:

- `vault_path: kubernetes/dev/example-app` with `local_path: ~/vaultsync-demo/secrets/dev` → `~/vaultsync-demo/secrets/dev/database`
- Nested Vault secrets below that path still create subdirectories only for the secret path segments below the configured base.

## Enhanced Diff Output

The tool automatically detects and uses enhanced diff tools if available:

1. **delta** - Side-by-side diffs with syntax highlighting
2. **difftastic** - Syntax-aware structural diffs  
3. **diff-so-fancy** - Enhanced unified diffs

Install any of these tools to get improved `--dry-run` output:

```bash
# Install delta
cargo install git-delta

# Or via package managers
brew install git-delta          # macOS
sudo apt install git-delta      # Ubuntu
```

## File Format

Secrets are stored as YAML content with the secret keys as top-level properties. Direct CLI syncs use `.yaml` files; config-driven syncs use extensionless filenames.

```yaml
# ./secrets/app/database.yaml or ~/vaultsync-demo/secrets/dev/database
host: db.example.com
port: 5432
username: myapp
password: secret123
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.
