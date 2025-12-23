# Build script for Windows
# Requires: Go, Node.js, npm

$ErrorActionPreference = "Stop"

Write-Host "Building AutoAnimeDownloader for Windows..." -ForegroundColor Green

# Get the project root directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
Set-Location $ProjectRoot

# Create build directory
New-Item -ItemType Directory -Force -Path "build\windows-amd64" | Out-Null

Write-Host "Step 1: Building frontend..." -ForegroundColor Yellow
Set-Location "src\internal\frontend"
npm ci
npm run build
Set-Location $ProjectRoot

Write-Host "Step 2: Building daemon for Windows amd64..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -a -installsuffix cgo -ldflags="-w -s" -o "build\windows-amd64\autoanimedownloader-daemon.exe" .\src\cmd\daemon

Write-Host "Step 3: Building CLI for Windows amd64..." -ForegroundColor Yellow
go build -a -installsuffix cgo -ldflags="-w -s" -o "build\windows-amd64\autoanimedownloader.exe" .\src\cmd\cli

Write-Host "Step 4: Generating checksums..." -ForegroundColor Yellow
Set-Location "build\windows-amd64"
Get-FileHash -Algorithm SHA256 autoanimedownloader-daemon.exe | ForEach-Object { "$($_.Hash)  $($_.Path)" } | Out-File -Encoding utf8 autoanimedownloader-daemon.exe.sha256
Get-FileHash -Algorithm SHA256 autoanimedownloader.exe | ForEach-Object { "$($_.Hash)  $($_.Path)" } | Out-File -Encoding utf8 autoanimedownloader.exe.sha256
Set-Location $ProjectRoot

Write-Host "Build complete!" -ForegroundColor Green
Write-Host "Binaries are in: build\windows-amd64\" -ForegroundColor Green

