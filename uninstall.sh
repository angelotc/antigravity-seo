#!/usr/bin/env bash
# antigravity-seo uninstaller
#
# Removes the plugin symlink/directory from the Antigravity plugin directory.
# Does NOT touch MCP configs or credentials (same contract as upstream
# uninstallers). Pass --purge to also delete the engine data directory
# (drift snapshots and runtime state).

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.gemini/config/plugins}"
DATA_DIR="${ANTIGRAVITY_SEO_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/antigravity-seo}"
PLUGIN_NAME="antigravity-seo"
target="$INSTALL_DIR/$PLUGIN_NAME"

if [ -L "$target" ]; then
    rm "$target"
    echo "Removed plugin symlink: $target"
elif [ -d "$target" ]; then
    rm -rf "$target"
    echo "Removed plugin directory: $target"
else
    echo "Plugin not installed at $target — nothing to do"
fi

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
if [ -L "$BIN_DIR/seo-engine" ]; then
    rm "$BIN_DIR/seo-engine"
    echo "Removed CLI binary symlink: $BIN_DIR/seo-engine"
fi

if [ "${1:-}" = "--purge" ]; then
    if [ -d "$DATA_DIR" ]; then
        rm -rf "$DATA_DIR"
        echo "Purged data directory: $DATA_DIR"
    fi
else
    echo "Data directory kept at $DATA_DIR (use --purge to remove it)"
fi
