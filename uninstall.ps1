# antigravity-seo uninstaller (Windows PowerShell)
#
# Removes the plugin copy from the Antigravity plugin directory.
# Does NOT touch user credentials. Pass -Purge to also delete
# the engine data directory (drift snapshots and runtime state).

param([switch]$Purge)

$ErrorActionPreference = "Stop"

$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:USERPROFILE ".gemini\config\plugins" }
$DataDir = if ($env:ANTIGRAVITY_SEO_DATA_DIR) { $env:ANTIGRAVITY_SEO_DATA_DIR } else { Join-Path $env:LOCALAPPDATA "antigravity-seo" }
$target = Join-Path $InstallDir "antigravity-seo"

if (Test-Path $target) {
    Remove-Item -Recurse -Force $target
    Write-Host "Removed plugin directory: $target"
} else {
    Write-Host "Plugin not installed at $target - nothing to do"
}

if ($Purge) {
    if (Test-Path $DataDir) {
        Remove-Item -Recurse -Force $DataDir
        Write-Host "Purged data directory: $DataDir"
    }
} else {
    Write-Host "Data directory kept at $DataDir (use -Purge to remove it)"
}
