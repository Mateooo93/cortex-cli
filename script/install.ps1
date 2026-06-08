# Install cortex-cli on Windows. Run in PowerShell:
#   irm https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo = "Mateooo93/cortex-cli"
$Arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$Asset = "cortex-windows-$Arch.exe"
$DestDir = Join-Path $env:LOCALAPPDATA "Programs\cortex"
$DestExe = Join-Path $DestDir "cortex.exe"

Write-Host "==> Installing cortex ($Arch) to $DestDir"

$release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
$version = $release.tag_name
$asset = $release.assets | Where-Object { $_.name -eq $Asset }
if (-not $asset) { throw "Release $version has no asset $Asset" }

New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $DestExe

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$DestDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$DestDir", "User")
  Write-Host "==> Added $DestDir to user PATH (open a new terminal to use 'cortex' globally)"
}

Write-Host "==> Installed cortex $version"
& $DestExe --version
Write-Host "==> Run: cortex"