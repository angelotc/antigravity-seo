#!/usr/bin/env bash
set -euo pipefail

# Read Antigravity context from stdin
PAYLOAD=$(cat)

# PostToolUse contract requires empty JSON object on stdout
echo '{}'
