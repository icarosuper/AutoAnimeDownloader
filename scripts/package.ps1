# Package script for Windows
# Creates a single executable with embedded frontend

$ErrorActionPreference = "Stop"

Write-Host "Packaging AutoAnimeDownloader for Windows..." -ForegroundColor Green

# Get the project root directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
Set-Location $ProjectRoot

# Check if build directory exists
if (-not (Test-Path "build\windows-amd64")) {
    Write-Host "Error: build directory not found. Run scripts/build.ps1 first." -ForegroundColor Red
    exit 1
}

Write-Host "Creating Windows package..." -ForegroundColor Yellow

# The daemon executable already has the frontend embedded
# For Windows, we'll create a single executable package
# In this case, the daemon.exe is already self-contained

$PackageName = "AutoAnimeDownloader_Windows.exe"
$DaemonPath = "build\windows-amd64\autoanimedownloader-daemon.exe"

if (-not (Test-Path $DaemonPath)) {
    Write-Host "Error: daemon executable not found at $DaemonPath" -ForegroundColor Red
    exit 1
}

# Copy daemon as the main executable
Copy-Item $DaemonPath $PackageName

# Generate checksum
$Hash = Get-FileHash -Algorithm SHA256 $PackageName
"$($Hash.Hash)  $PackageName" | Out-File -Encoding utf8 "$PackageName.sha256"

Write-Host "Package created: $PackageName" -ForegroundColor Green
Write-Host ""
Write-Host "Usage:" -ForegroundColor Yellow
Write-Host "  - Run directly: .\$PackageName"
Write-Host "  - Or install as Windows service using NSSM"
Write-Host ""
Write-Host "The executable contains the frontend embedded and can be run standalone."

