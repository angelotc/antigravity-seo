# PostToolUse hook: JSON-LD schema quality gate on file writes (Windows).
# PowerShell counterpart of hooks/schema_linter.sh — same contract:
#   - BLOCKS (exit 2) on placeholder values and deprecated schema types
#   - WARNs (exit 0 + findings on stderr) on invalid JSON or missing @context/@type
#   - prints {} on stdout when clean (PostToolUse contract)
#
# Windows hook config (hooks.json):
#   "command": "powershell -NoProfile -ExecutionPolicy Bypass -File ./hooks/schema_linter.ps1"

param()

$ErrorActionPreference = "SilentlyContinue"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Engine = Join-Path $ScriptDir "..\bin\seo-engine.exe"

$payload = [Console]::In.ReadToEnd()

if (-not (Test-Path $Engine)) {
    # Engine unavailable - never block edits
    Write-Output "{}"
    exit 0
}

$payload | & $Engine lint-schema-file
exit $LASTEXITCODE
