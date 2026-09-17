#!/usr/bin/env bash
# antigravity-seo installer
#
# Mirrors the operational contract of an installable SEO plugin:
#   1. builds the Go engine (single static binary)
#   2. installs the plugin into Antigravity's plugin directory (symlink)
#   3. runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Environment overrides:
#   INSTALL_DIR  plugin target directory (default: ~/.gemini/config/plugins)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.gemini/config/plugins}"
PLUGIN_NAME="antigravity-seo"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SRC_DIR/bin/seo-engine"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

log "antigravity-seo installer"

# ---------------------------------------------------------------- build engine
if command -v go >/dev/null 2>&1; then
    log "Building seo-engine with $(go version)"
    (cd "$SRC_DIR" && go build -buildvcs=false -o bin/seo-engine ./cmd/seo-engine) \
        || die "engine build failed"
elif [ -x "$ENGINE" ]; then
    warn "Go toolchain not found — keeping existing prebuilt engine at bin/seo-engine"
    warn "Install Go 1.22+ (https://go.dev/dl/) to rebuild from source"
else
    die "Go toolchain not found and no prebuilt engine at bin/seo-engine.
     Install Go 1.22+ from https://go.dev/dl/ and re-run this script."
fi

# ------------------------------------------------------------ install plugin
log "Installing plugin into $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
target="$INSTALL_DIR/$PLUGIN_NAME"
if [ -L "$target" ]; then
    rm "$target"
elif [ -d "$target" ]; then
    warn "replacing existing directory $target with a symlink"
    rm -rf "$target"
fi
ln -s "$SRC_DIR" "$target"

# -------------------------------------------------------------- setup + verify
log "Initializing runtime (data dir + state manifest)"
"$ENGINE" setup || die "setup failed"

log "Verifying readiness with doctor"
doctor_status=0
"$ENGINE" doctor || doctor_status=$?
case "$doctor_status" in
    0)  log "All checks passed — READY" ;;
    10) warn "Partially ready (optional integrations missing) — core features available" ;;
    *)  die "doctor failed with status $doctor_status — see output above" ;;
esac

cat <<EOF

Installed.

Next steps:
  • Restart Antigravity so the plugin is discovered, then ask:
      "Audit the SEO for https://example.com"
  • CLI:        $ENGINE page https://example.com
  • MCP config: see $SRC_DIR/adapters/ for Codex and Cursor snippets
  • Uninstall:  bash $SRC_DIR/uninstall.sh
EOF
