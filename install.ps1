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
#   2. Builds or downloads the Go engine (bin\seo-engine.exe), verifying the
#      SHA256 checksum of any downloaded binary against the release's
#      SHA256SUMS.txt
#   3. Runs `seo-engine setup` and verifies readiness with `seo-engine doctor`
#
# Overrides:
#   $env:INSTALL_DIR          plugin target directory (default: ~\.gemini\config\plugins)
#   $env:SEO_ENGINE_VERSION   release tag to install, without the leading "v"
#                             (default: the version pinned in this script / plugin.json)
#
# Exit codes: 0 = ready, 10 = partial (usable, with warnings), 1 = failure

$ErrorActionPreference = "Stop"

# Keep in lockstep with plugin.json / marketplace.json / cmd/seo-engine/main.go
# -- the CI version-consistency check fails the build if these drift apart.
$SeoEngineDefaultVersion = "2.1.0"

$RepoUrl = "https://github.com/angelotc/antigravity-seo.git"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:USERPROFILE ".gemini\config\plugins" }
$PluginName = "antigravity-seo"
$Target = Join-Path $InstallDir $PluginName

function Write-Info($msg) { Write-Host "==> $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "WARN: $msg" -ForegroundColor Yellow }
function Die($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

# Test-Checksum: looks up $Name in a `sha256sum`-style checksums file (lines
# of "<hex-digest>  <filename>") and compares it against the actual SHA256 of
# $File. Returns $true on match, $false otherwise.
function Test-Checksum($File, $SumsFile, $Name) {
    foreach ($line in Get-Content $SumsFile) {
        $parts = $line -split '\s+'
        if ($parts.Length -ge 2 -and $parts[1] -eq $Name) {
            $expected = $parts[0].ToLowerInvariant()
            $actual = (Get-FileHash -Path $File -Algorithm SHA256).Hash.ToLowerInvariant()
            return ($expected -eq $actual)
        }
    }
    return $false
}

Write-Info "antigravity-seo installer"

# ---------------------------------------------------- detect local vs remote mode
$SrcDir = $null
if ($MyInvocation.MyCommand.Path) {
    $PotentialDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    if ((Test-Path (Join-Path $PotentialDir "plugin.json")) -and (Test-Path (Join-Path $PotentialDir "cmd\seo-engine"))) {
        $SrcDir = $PotentialDir
    }
}

# ------------------------------------------------------------- resolve version
# Precedence: explicit $env:SEO_ENGINE_VERSION override > the version declared
# in a local checkout's plugin.json > the default pinned in this script.
$Version = $env:SEO_ENGINE_VERSION
if (-not $Version -and $SrcDir) {
    $pluginJsonPath = Join-Path $SrcDir "plugin.json"
    if (Test-Path $pluginJsonPath) {
        try {
            $Version = (Get-Content $pluginJsonPath -Raw | ConvertFrom-Json).version
        } catch {
            $Version = $null
        }
    }
}
if (-not $Version) { $Version = $SeoEngineDefaultVersion }
$ZipUrl = "https://github.com/angelotc/antigravity-seo/archive/refs/tags/v$Version.zip"

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
            Write-Info "Git not found - downloading release archive for v$Version"
            $tempZip = Join-Path $env:TEMP "antigravity-seo-$Version.zip"
            $tempExtract = Join-Path $env:TEMP "antigravity-seo-extract-$Version"
            if (Test-Path $tempExtract) { Remove-Item -Recurse -Force $tempExtract }
            Invoke-WebRequest -Uri $ZipUrl -OutFile $tempZip
            Expand-Archive -Path $tempZip -DestinationPath $tempExtract -Force
            $extractedDir = Get-ChildItem -Path $tempExtract -Directory | Select-Object -First 1
            if (-not $extractedDir) { Die "release archive for v$Version did not contain a source directory" }
            Move-Item $extractedDir.FullName $Target
            Remove-Item $tempZip -Force -ErrorAction SilentlyContinue
            Remove-Item -Recurse -Force $tempExtract -ErrorAction SilentlyContinue
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
        go build -buildvcs=false -ldflags="-X main.Version=$Version" -o bin/seo-engine.exe ./cmd/seo-engine
        if ($LASTEXITCODE -ne 0) { Die "engine build failed" }
    } finally {
        Pop-Location
    }
} elseif (Test-Path $Engine) {
    Write-Warn "Go toolchain not found - keeping existing prebuilt engine at $Engine"
} else {
    $BinaryName = "seo-engine-windows-amd64.exe"
    $ReleaseUrl = "https://github.com/angelotc/antigravity-seo/releases/download/v$Version/$BinaryName"
    $SumsUrl = "https://github.com/angelotc/antigravity-seo/releases/download/v$Version/SHA256SUMS.txt"
    Write-Info "Go not found; attempting to download prebuilt binary v$Version..."
    $tempSums = Join-Path $env:TEMP "antigravity-seo-SHA256SUMS-$Version.txt"
    try {
        Invoke-WebRequest -Uri $ReleaseUrl -OutFile $Engine
        Invoke-WebRequest -Uri $SumsUrl -OutFile $tempSums
        if (-not (Test-Checksum -File $Engine -SumsFile $tempSums -Name $BinaryName)) {
            Remove-Item $Engine -Force -ErrorAction SilentlyContinue
            Die "checksum verification failed for $BinaryName - refusing to install an unverified binary"
        }
        Write-Info "Checksum verified - downloaded prebuilt engine to $Engine"
    } catch {
        Remove-Item $Engine -Force -ErrorAction SilentlyContinue
        Die "Go toolchain not found and no verified prebuilt binary available for v$Version. Install Go 1.26+ from https://go.dev/dl/ and re-run, or set `$env:SEO_ENGINE_VERSION to a release that publishes this platform."
    } finally {
        Remove-Item $tempSums -Force -ErrorAction SilentlyContinue
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
