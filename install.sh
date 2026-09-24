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
#   2. Builds or downloads the Go engine (single static binary), verifying the
#      SHA256 checksum of any downloaded binary against the release's
#      SHA256SUMS.txt
#   3. Runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Environment overrides:
#   INSTALL_DIR         plugin target directory (default: ~/.gemini/config/plugins)
#   BIN_DIR              directory for global CLI symlink (default: ~/.local/bin)
#   SEO_ENGINE_VERSION  release tag to install, without the leading "v"
#                        (default: the version pinned in this script / plugin.json)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

set -euo pipefail

# Keep in lockstep with plugin.json / marketplace.json / cmd/seo-engine/main.go
# — the CI version-consistency check fails the build if these drift apart.
SEO_ENGINE_DEFAULT_VERSION="2.1.0"

REPO_URL="https://github.com/angelotc/antigravity-seo.git"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.gemini/config/plugins}"
PLUGIN_NAME="antigravity-seo"
TARGET="$INSTALL_DIR/$PLUGIN_NAME"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# verify_checksum <file> <sums-file> <name-as-listed-in-sums-file>
# Looks up <name> in a `sha256sum`-style checksums file (lines of
# "<hex-digest>  <filename>") and compares it against the actual digest of
# <file>. Returns 0 on match, 1 otherwise (with a warning on stderr).
verify_checksum() {
    local file="$1" sums_file="$2" name="$3" expected actual
    expected="$(awk -v f="$name" '$2==f{print $1; exit}' "$sums_file")"
    if [ -z "$expected" ]; then
        warn "no checksum entry for $name in $sums_file"
        return 1
    fi
    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        warn "no sha256sum/shasum available to verify $file"
        return 1
    fi
    if [ "$expected" != "$actual" ]; then
        warn "checksum mismatch for $name: expected $expected, got $actual"
        return 1
    fi
    return 0
}

log "antigravity-seo installer"

# ---------------------------------------------------- detect local vs remote mode
SCRIPT_DIR=""
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

# ------------------------------------------------------------- resolve version
# Precedence: explicit SEO_ENGINE_VERSION env override > the version declared
# in a local checkout's plugin.json > the default pinned in this script.
VERSION="${SEO_ENGINE_VERSION:-}"
if [ -z "$VERSION" ] && [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/plugin.json" ]; then
    VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$SCRIPT_DIR/plugin.json" | head -n1)"
fi
VERSION="${VERSION:-$SEO_ENGINE_DEFAULT_VERSION}"
TARBALL_URL="https://github.com/angelotc/antigravity-seo/archive/refs/tags/v${VERSION}.tar.gz"

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
            log "Git not found — downloading release archive for v${VERSION}"
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
    (cd "$SRC_DIR" && go build -buildvcs=false -ldflags="-X main.Version=${VERSION}" -o bin/seo-engine ./cmd/seo-engine) \
        || die "engine build failed"
elif [ -x "$ENGINE" ]; then
    warn "Go toolchain not found — keeping existing prebuilt engine at $ENGINE"
else
    # Attempt to download and verify a prebuilt release binary
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
    esac
    BINARY_NAME="seo-engine-${OS}-${ARCH}"
    RELEASE_URL="https://github.com/angelotc/antigravity-seo/releases/download/v${VERSION}/${BINARY_NAME}"
    SUMS_URL="https://github.com/angelotc/antigravity-seo/releases/download/v${VERSION}/SHA256SUMS.txt"
    log "Go toolchain not found; attempting to download prebuilt binary v${VERSION} (${OS}/${ARCH})..."
    if curl -fsSL -o "$ENGINE" "$RELEASE_URL" 2>/dev/null && [ -s "$ENGINE" ]; then
        SUMS_FILE="$(mktemp)"
        trap 'rm -f "$SUMS_FILE"' EXIT
        if curl -fsSL -o "$SUMS_FILE" "$SUMS_URL" 2>/dev/null && [ -s "$SUMS_FILE" ]; then
            if verify_checksum "$ENGINE" "$SUMS_FILE" "$BINARY_NAME"; then
                chmod +x "$ENGINE"
                log "Checksum verified — downloaded prebuilt binary to $ENGINE"
            else
                rm -f "$ENGINE"
                die "checksum verification failed for ${BINARY_NAME} — refusing to install an unverified binary"
            fi
        else
            rm -f "$ENGINE"
            die "failed to download SHA256SUMS.txt for v${VERSION} — refusing to install an unverified binary"
        fi
    else
        die "Go toolchain not found and no prebuilt binary available for v${VERSION} (${OS}/${ARCH}).
     Please install Go 1.26+ from https://go.dev/dl/ and re-run this script,
     or set SEO_ENGINE_VERSION to a release that publishes this platform."
    fi
fi

# -------------------------------------------------- optional CLI symlink to PATH
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR" 2>/dev/null || warn "could not create $BIN_DIR — skipping CLI symlink"
if [ -d "$BIN_DIR" ] && [ -w "$BIN_DIR" ]; then
    ln -sf "$ENGINE" "$BIN_DIR/seo-engine"
    log "Symlinked CLI binary to $BIN_DIR/seo-engine"
    case ":$PATH:" in
        *":$BIN_DIR:"*) ;;
        *) warn "$BIN_DIR is not on your PATH — add it, e.g.: export PATH=\"$BIN_DIR:\$PATH\"" ;;
    esac
elif [ -d "$BIN_DIR" ]; then
    warn "$BIN_DIR exists but is not writable — skipping CLI symlink"
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
