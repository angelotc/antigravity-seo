# PostToolUse hook: JSON-LD schema quality gate on file writes (Windows).
# PowerShell counterpart of hooks/schema_linter.sh — same contract:
#   - BLOCKS (exit 2) on placeholder values and deprecated schema types
#   - WARNs (exit 0 + findings on stderr) on invalid JSON or missing @context/@type
#   - prints {} on stdout when clean (PostToolUse contract)
#   - never blocks the edit on an engine problem: missing binary, wrong-arch
#     binary, a crash, or any exit code other than 0 or 2 all fall through to
#     `{}` / exit 0
#
# Not wired into hooks.json by default — hooks.json has no per-OS command
# field, so it always shells out to schema_linter.sh. See README.md's "Hooks"
# section for the exact hooks.json edit that registers this script instead.

param()

$ErrorActionPreference = "SilentlyContinue"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Engine = Join-Path $ScriptDir "..\bin\seo-engine.exe"

$payload = [Console]::In.ReadToEnd()

if (-not (Test-Path $Engine)) {
    # No bundled binary for this platform — fall back to a PATH install.
    $pathEngine = Get-Command seo-engine.exe -ErrorAction SilentlyContinue
    if (-not $pathEngine) {
        $pathEngine = Get-Command seo-engine -ErrorAction SilentlyContinue
    }
    if ($pathEngine) {
        $Engine = $pathEngine.Source
    } else {
        Write-Output "{}"
        exit 0
    }
}

try {
    $output = $payload | & $Engine lint-schema-file
    $status = $LASTEXITCODE
} catch {
    # Engine could not be started (wrong arch, missing file) — never block.
    Write-Output "{}"
    exit 0
}

if ($status -eq 0 -or $status -eq 2) {
    if ([string]::IsNullOrEmpty($output)) {
        Write-Output "{}"
    } else {
        Write-Output $output
    }
    exit $status
} else {
    # Engine exited abnormally (wrong arch, crashed, missing libs, ...) —
    # never block the edit on an engine problem.
    Write-Output "{}"
    exit 0
}
