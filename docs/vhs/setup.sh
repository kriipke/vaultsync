# Sourced (hidden) at the start of each tape to build a clean demo world.
# Usage: source setup.sh [pulled|edited]
VHS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK=/tmp/vaultsync-demo
export VAULTSYNC_DEMO_REMOTE=/tmp/vaultsync-demo-remote

rm -rf "$WORK" "$VAULTSYNC_DEMO_REMOTE"
mkdir -p "$WORK" "$VAULTSYNC_DEMO_REMOTE/app"

cat > "$VAULTSYNC_DEMO_REMOTE/app/database.yaml" <<'EOF'
host: db.internal.example.com
password: s3cr3t-old-password
port: "5432"
username: app_service
EOF

cat > "$VAULTSYNC_DEMO_REMOTE/app/redis.yaml" <<'EOF'
host: redis.internal.example.com
password: r3d1s-p@ss
port: "6379"
EOF

cat > "$VAULTSYNC_DEMO_REMOTE/app/smtp.yaml" <<'EOF'
host: smtp.example.com
password: sm7p-s3nd3r
port: "587"
username: noreply@example.com
EOF

cat > "$VAULTSYNC_DEMO_REMOTE/app/api-keys.yaml" <<'EOF'
github: ghp_demo1234567890abcdef
stripe: sk_test_demo1234567890
EOF

cd "$WORK"

# Stage prior sessions' state when a later tape starts mid-workflow.
if [ "${1:-}" = "pulled" ] || [ "${1:-}" = "edited" ]; then
  mkdir -p secrets/app
  cp "$VAULTSYNC_DEMO_REMOTE"/app/*.yaml secrets/app/
fi
if [ "${1:-}" = "edited" ]; then
  sed -i '' 's/s3cr3t-old-password/n3w-r0tated-p@ssw0rd/' secrets/app/database.yaml
fi

export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
export PATH="$VHS_DIR/bin:$PATH"
export VAULT_ADDR="https://vault.example.com:8200"
export VAULT_TOKEN="hvs.CAESIJ5x9k2mP7qR8tW3vY6zA1bC4dE"
# Starship prompt (falls back to a plain prompt if starship isn't installed).
if command -v starship >/dev/null 2>&1; then
  export STARSHIP_CONFIG="$VHS_DIR/starship.toml"
  eval "$(starship init bash)"
else
  export PS1='$ '
fi
