#!/usr/bin/env bash
# PostToolUse hook: JSON-LD schema quality gate on file writes.
#
# Delegates to the Go engine (`lint-schema-file`), which:
#   - extracts the written file path from the Antigravity hook payload on stdin
#   - skips non-HTML-like files and files over 10MB
#   - BLOCKS (exit 2) on placeholder values ([Business Name], REPLACE_*, TODO)
#     and deprecated schema types (HowTo, SpecialAnnouncement, ...)
#   - WARNs (exit 0 + findings on stderr) on invalid JSON or missing @context/@type
#   - prints {} on stdout when clean (PostToolUse contract)

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SCRIPT_DIR/../bin/seo-engine"

payload=$(cat)

if [ ! -x "$ENGINE" ]; then
    # Engine unavailable — never block edits
    echo '{}'
    exit 0
fi

status=0
printf '%s' "$payload" | "$ENGINE" lint-schema-file || status=$?
exit $status
