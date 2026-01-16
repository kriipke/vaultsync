# VaultSync

A CLI tool to sync secrets between HashiCorp Vault and local YAML files, with support for HCP (HashiCorp Cloud Platform) Vault.

## Features

- **List secrets** from Vault KVv2 engines
- **Pull secrets** to local YAML files with directory structure mirroring
- **Push secrets** back to Vault from YAML files
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
go build -o vaultsync .
```

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
vaultsync list <namespace> <kvv2-path>

# Examples
vaultsync list my-namespace kv/metadata
vaultsync list my-namespace kv/metadata/app
```

#### Pull Secrets to Files
```bash
vaultsync pull <namespace> <kvv2-path> [output-dir]

# Examples
vaultsync pull my-namespace kv/metadata                    # saves to ./vault-secrets/
vaultsync pull my-namespace kv/metadata ./my-secrets      # saves to ./my-secrets/
vaultsync pull my-namespace kv/metadata/app ./app-secrets # saves to ./app-secrets/app/
```

#### Push Secrets from Files
```bash
vaultsync push <namespace> <kvv2-path> [input-dir] [--dry-run]

# Examples
vaultsync push my-namespace kv/metadata --dry-run          # dry-run from ./vault-secrets/
vaultsync push my-namespace kv/metadata                    # push from ./vault-secrets/
vaultsync push my-namespace kv/metadata ./my-secrets      # push from ./my-secrets/
```

### Workflow Example

```bash
# 1. Pull secrets from Vault
vaultsync pull my-namespace kv/metadata

# 2. Edit files in ./vault-secrets/
# Files are organized like: ./vault-secrets/app/database.yaml

# 3. Preview changes with enhanced diff
vaultsync push my-namespace kv/metadata --dry-run | delta

# 4. Push changes back to Vault
vaultsync push my-namespace kv/metadata
```

## Directory Structure

The tool maintains a consistent directory structure between pull and push operations:

**Vault Path → File Path**
- `kv/metadata/app/database` → `./vault-secrets/app/database.yaml`
- `kv/metadata/shared/config` → `./vault-secrets/shared/config.yaml`

**For specific subpaths:**
- Pull from `kv/metadata/app` → Files in `./vault-secrets/app/`
- Push to `kv/metadata/app` → Reads files from `./vault-secrets/app/`

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

Secrets are stored as YAML files with the secret keys as top-level properties:

```yaml
# ./vault-secrets/app/database.yaml
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