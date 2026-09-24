#!/usr/bin/env bash
# PostToolUse hook: JSON-LD schema quality gate on file writes.
#
# Delegates to the Go engine (`lint-schema-file`), which:
#   - extracts the written file path from the hook payload on stdin
#   - skips non-HTML-like files and files over 10MB
#   - BLOCKS (exit 2) on placeholder values ([Business Name], REPLACE_*, TODO)
#     and deprecated schema types (HowTo, SpecialAnnouncement, ...)
#   - WARNs (exit 0 + findings on stderr) on invalid JSON or missing @context/@type
#   - prints {} on stdout when clean (PostToolUse contract)
#
# Never blocks edits on an engine problem: a missing binary, a wrong-arch
# binary (exit 126/127), a crash, or any exit code other than 0 or 2 all fall
# through to `{}` / exit 0.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SCRIPT_DIR/../bin/seo-engine"

payload=$(cat)

if [ ! -x "$ENGINE" ]; then
    # No bundled binary for this platform — fall back to a PATH install.
    if command -v seo-engine >/dev/null 2>&1; then
        ENGINE="$(command -v seo-engine)"
    else
        echo '{}'
        exit 0
    fi
fi

output=$(printf '%s' "$payload" | "$ENGINE" lint-schema-file)
status=$?

case "$status" in
    0|2)
        if [ -z "$output" ]; then
            echo '{}'
        else
            printf '%s\n' "$output"
        fi
        exit "$status"
        ;;
    *)
        # Engine exited abnormally (wrong arch, crashed, missing libs, ...) —
        # never block the edit on an engine problem.
        echo '{}'
        exit 0
        ;;
esac
