#!/usr/bin/env bash
# antigravity-seo installer
#
# Supports two installation modes:
#   1. Remote one-line install (no clone required):
#      curl -fsSL https://raw.githubusercontent.com/angelotc/antigravity-seo/main/install.sh | bash
#   2. Local clone install (development):
#      bash install.sh
#
# Operational contract:
#   1. Ensures plugin exists in ~/.gemini/config/plugins/antigravity-seo
#   2. Builds or downloads the Go engine (single static binary)
#   3. Runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Environment overrides:
#   INSTALL_DIR  plugin target directory (default: ~/.gemini/config/plugins)
#   BIN_DIR      directory for global CLI symlink (optional, e.g. ~/.local/bin)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

set -euo pipefail

REPO_URL="https://github.com/angelotc/antigravity-seo.git"
TARBALL_URL="https://github.com/angelotc/antigravity-seo/archive/refs/heads/main.tar.gz"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.gemini/config/plugins}"
PLUGIN_NAME="antigravity-seo"
TARGET="$INSTALL_DIR/$PLUGIN_NAME"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

log "antigravity-seo installer"

# ---------------------------------------------------- detect local vs remote mode
SCRIPT_DIR=""
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/plugin.json" ] && [ -d "$SCRIPT_DIR/cmd/seo-engine" ]; then
    log "Running from local repository checkout: $SCRIPT_DIR"
    mkdir -p "$INSTALL_DIR"
    if [ "$TARGET" != "$SCRIPT_DIR" ]; then
        if [ -L "$TARGET" ]; then
            rm "$TARGET"
        elif [ -d "$TARGET" ]; then
            warn "replacing existing directory $TARGET with a symlink to $SCRIPT_DIR"
            rm -rf "$TARGET"
        fi
        ln -s "$SCRIPT_DIR" "$TARGET"
    fi
    SRC_DIR="$SCRIPT_DIR"
else
    log "Remote installation mode: deploying into $TARGET"
    mkdir -p "$INSTALL_DIR"
    if [ -d "$TARGET/.git" ]; then
        log "Existing git repository found at $TARGET — pulling latest"
        (cd "$TARGET" && git pull --ff-only || true)
    elif [ -d "$TARGET" ]; then
        log "Directory $TARGET already exists"
    else
        if command -v git >/dev/null 2>&1; then
            log "Cloning repository from $REPO_URL"
            git clone --depth=1 "$REPO_URL" "$TARGET"
        else
            log "Git not found — downloading release archive"
            mkdir -p "$TARGET"
            curl -fsSL "$TARBALL_URL" | tar -xz --strip-components=1 -C "$TARGET"
        fi
    fi
    SRC_DIR="$TARGET"
fi

ENGINE="$SRC_DIR/bin/seo-engine"
mkdir -p "$SRC_DIR/bin"

# ---------------------------------------------------------------- build engine
if command -v go >/dev/null 2>&1; then
    log "Building seo-engine with $(go version)"
    (cd "$SRC_DIR" && go build -buildvcs=false -o bin/seo-engine ./cmd/seo-engine) \
        || die "engine build failed"
elif [ -x "$ENGINE" ]; then
    warn "Go toolchain not found — keeping existing prebuilt engine at $ENGINE"
else
    # Attempt to download prebuilt release binary
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
    esac
    RELEASE_URL="https://github.com/angelotc/antigravity-seo/releases/latest/download/seo-engine-${OS}-${ARCH}"
    log "Go toolchain not found; attempting to download prebuilt binary (${OS}/${ARCH})..."
    if curl -fsSL -o "$ENGINE" "$RELEASE_URL" 2>/dev/null && [ -s "$ENGINE" ]; then
        chmod +x "$ENGINE"
        log "Successfully downloaded prebuilt binary to $ENGINE"
    else
        die "Go toolchain not found and no prebuilt binary available.
     Please install Go 1.22+ from https://go.dev/dl/ and re-run this script."
    fi
fi

# -------------------------------------------------- optional CLI symlink to PATH
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
if [ -d "$BIN_DIR" ] && [ -w "$BIN_DIR" ]; then
    ln -sf "$ENGINE" "$BIN_DIR/seo-engine"
    log "Symlinked CLI binary to $BIN_DIR/seo-engine"
fi

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

=======================================================
  Antigravity SEO Suite Installed Successfully!
=======================================================

Usage with Antigravity:
  • Restart Antigravity to discover the plugin.
  • In any conversation, ask:
      "Audit the SEO for https://example.com"
      "Generate an executive SEO report for https://example.com"

Direct CLI Usage:
  • $ENGINE doctor
  • $ENGINE report https://example.com --pdf
  • $ENGINE backlinks example.com --limit 25

Uninstall:
  • bash $SRC_DIR/uninstall.sh
EOF
