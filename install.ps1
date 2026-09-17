# antigravity-seo installer (Windows PowerShell)
#
# Supports two installation modes:
#   1. Remote one-line install (no clone required):
#      irm https://raw.githubusercontent.com/angelotc/antigravity-seo/main/install.ps1 | iex
#   2. Local clone install (development):
#      powershell -ExecutionPolicy Bypass -File install.ps1
#
# Operational contract:
#   1. Copies or clones plugin into ~/.gemini/config/plugins/antigravity-seo
#   2. Builds or downloads the Go engine (bin\seo-engine.exe)
#   3. Runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Overrides:
#   $env:INSTALL_DIR  plugin target directory (default: ~\.gemini\config\plugins)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

$ErrorActionPreference = "Stop"

$RepoUrl = "https://github.com/angelotc/antigravity-seo.git"
$ZipUrl = "https://github.com/angelotc/antigravity-seo/archive/refs/heads/main.zip"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:USERPROFILE ".gemini\config\plugins" }
$PluginName = "antigravity-seo"
$Target = Join-Path $InstallDir $PluginName

function Write-Info($msg) { Write-Host "==> $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "WARN: $msg" -ForegroundColor Yellow }
function Die($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

Write-Info "antigravity-seo installer"

# ---------------------------------------------------- detect local vs remote mode
$SrcDir = $null
if ($MyInvocation.MyCommand.Path) {
    $PotentialDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    if ((Test-Path (Join-Path $PotentialDir "plugin.json")) -and (Test-Path (Join-Path $PotentialDir "cmd\seo-engine"))) {
        $SrcDir = $PotentialDir
    }
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

if ($SrcDir) {
    Write-Info "Running from local repository checkout: $SrcDir"
    if ($Target -ne $SrcDir) {
        if (Test-Path $Target) {
            Write-Warn "replacing existing install at $Target"
            Remove-Item -Recurse -Force $Target
        }
        Copy-Item -Recurse $SrcDir $Target
    }
    $WorkingDir = $SrcDir
} else {
    Write-Info "Remote installation mode: deploying into $Target"
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
        if (Test-Path (Join-Path $Target ".git")) {
            Write-Info "Existing git repository found at $Target - pulling latest"
            Push-Location $Target
            try { git pull --ff-only } finally { Pop-Location }
        } elseif (Test-Path $Target) {
            Write-Info "Directory $Target already exists"
        } else {
            Write-Info "Cloning repository from $RepoUrl"
            git clone --depth=1 $RepoUrl $Target
        }
    } else {
        if (-not (Test-Path $Target)) {
            Write-Info "Git not found - downloading release zip archive"
            $tempZip = Join-Path $env:TEMP "antigravity-seo-main.zip"
            Invoke-WebRequest -Uri $ZipUrl -OutFile $tempZip
            Expand-Archive -Path $tempZip -DestinationPath $env:TEMP -Force
            Move-Item (Join-Path $env:TEMP "antigravity-seo-main") $Target
            Remove-Item $tempZip -Force
        }
    }
    $WorkingDir = $Target
}

$Engine = Join-Path $WorkingDir "bin\seo-engine.exe"
New-Item -ItemType Directory -Force -Path (Join-Path $WorkingDir "bin") | Out-Null

# ---------------------------------------------------------------- build engine
$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) {
    Write-Info "Building seo-engine with $(go version)"
    Push-Location $WorkingDir
    try {
        go build -buildvcs=false -o bin/seo-engine.exe ./cmd/seo-engine
        if ($LASTEXITCODE -ne 0) { Die "engine build failed" }
    } finally {
        Pop-Location
    }
} elseif (Test-Path $Engine) {
    Write-Warn "Go toolchain not found - keeping existing prebuilt engine at $Engine"
} else {
    $ReleaseUrl = "https://github.com/angelotc/antigravity-seo/releases/latest/download/seo-engine-windows-amd64.exe"
    Write-Info "Go not found; attempting to download prebuilt binary..."
    try {
        Invoke-WebRequest -Uri $ReleaseUrl -OutFile $Engine
        Write-Info "Successfully downloaded prebuilt engine to $Engine"
    } catch {
        Die "Go toolchain not found and no prebuilt binary available. Install Go 1.22+ from https://go.dev/dl/ and re-run."
    }
}

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
Write-Host "======================================================="
Write-Host "  Antigravity SEO Suite Installed Successfully!"
Write-Host "======================================================="
Write-Host ""
Write-Host "Usage with Antigravity:"
Write-Host "  * Restart Antigravity so the plugin is discovered, then ask:"
Write-Host "      `"Audit the SEO for https://example.com`""
Write-Host "      `"Generate an executive SEO report for https://example.com`""
Write-Host ""
Write-Host "Direct CLI Usage:"
Write-Host "  * $Engine doctor"
Write-Host "  * $Engine report https://example.com --pdf"
Write-Host "  * $Engine backlinks example.com --limit 25"
Write-Host ""
Write-Host "Uninstall:"
Write-Host "  * powershell -File $(Join-Path $WorkingDir 'uninstall.ps1')"
