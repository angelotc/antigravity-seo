# antigravity-seo installer (Windows PowerShell)
#
# Mirrors install.sh on Windows:
#   1. builds the Go engine (bin\seo-engine.exe)
#   2. copies the plugin into Antigravity's plugin directory
#   3. runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File install.ps1
#
# Overrides:
#   $env:INSTALL_DIR  plugin target directory (default: ~\.gemini\config\plugins)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

$ErrorActionPreference = "Stop"

$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:USERPROFILE ".gemini\config\plugins" }
$PluginName = "antigravity-seo"
$SrcDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Engine = Join-Path $SrcDir "bin\seo-engine.exe"

function Write-Info($msg) { Write-Host "==> $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "WARN: $msg" -ForegroundColor Yellow }
function Die($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

Write-Info "antigravity-seo installer"

# ---------------------------------------------------------------- build engine
# Reject Windows Store python-style stubs for go if present but unusable
$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) {
    Write-Info "Building seo-engine with $(go version)"
    Push-Location $SrcDir
    try {
        go build -buildvcs=false -o bin/seo-engine.exe ./cmd/seo-engine
        if ($LASTEXITCODE -ne 0) { Die "engine build failed" }
    } finally {
        Pop-Location
    }
} elseif (Test-Path $Engine) {
    Write-Warn "Go toolchain not found - keeping existing prebuilt engine at bin\seo-engine.exe"
    Write-Warn "Install Go 1.22+ (https://go.dev/dl/) to rebuild from source"
} else {
    Die "Go toolchain not found and no prebuilt engine at bin\seo-engine.exe. Install Go 1.22+ and re-run."
}

# ------------------------------------------------------------ install plugin
Write-Info "Installing plugin into $InstallDir"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir $PluginName
if (Test-Path $target) {
    Write-Warn "replacing existing install at $target"
    Remove-Item -Recurse -Force $target
}
# Copy instead of symlink: directory symlinks need admin/dev-mode on Windows
Copy-Item -Recurse $SrcDir $target

# -------------------------------------------------------------- setup + verify
Write-Info "Initializing runtime (data dir + state manifest)"
& $Engine setup
if ($LASTEXITCODE -ne 0) { Die "setup failed" }

Write-Info "Verifying readiness with doctor"
& $Engine doctor
$doctorStatus = $LASTEXITCODE
switch ($doctorStatus) {
    0 { Write-Info "All checks passed - READY" }
    10 { Write-Warn "Partially ready (optional integrations missing) - core features available" }
    default { Die "doctor failed with status $doctorStatus - see output above" }
}

Write-Host ""
Write-Host "Installed."
Write-Host "Next steps:"
Write-Host "  * Restart Antigravity so the plugin is discovered, then ask:"
Write-Host "      `"Audit the SEO for https://example.com`""
Write-Host "  * CLI:        $Engine page https://example.com"
Write-Host "  * MCP config: see $SrcDir\adapters\ for Codex and Cursor snippets"
Write-Host "  * Hook (Windows): use hooks\schema_linter.ps1 in hooks.json"
Write-Host "  * Uninstall:  powershell -File $SrcDir\uninstall.ps1"
