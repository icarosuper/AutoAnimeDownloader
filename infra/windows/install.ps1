# Installation script for Windows
# Installs AutoAnimeDownloader as a Windows service using NSSM

$ErrorActionPreference = "Stop"

Write-Host "Installing AutoAnimeDownloader as Windows Service..." -ForegroundColor Green

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Error: This script must be run as Administrator." -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    exit 1
}

# Get the script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $ScriptDir)

# Check for NSSM
$nssmPath = Get-Command nssm -ErrorAction SilentlyContinue
if (-not $nssmPath) {
    Write-Host "NSSM (Non-Sucking Service Manager) is required but not found." -ForegroundColor Yellow
    Write-Host "Please download NSSM from: https://nssm.cc/download" -ForegroundColor Yellow
    Write-Host "Extract it and add to PATH, or place nssm.exe in the same directory as this script." -ForegroundColor Yellow
    
    $nssmLocal = Join-Path $ScriptDir "nssm.exe"
    if (Test-Path $nssmLocal) {
        $nssmPath = $nssmLocal
        Write-Host "Found NSSM locally: $nssmPath" -ForegroundColor Green
    } else {
        Write-Host "Error: NSSM not found. Please install NSSM first." -ForegroundColor Red
        exit 1
    }
} else {
    $nssmPath = $nssmPath.Source
}

# Find the daemon executable
$daemonExe = Join-Path $ProjectRoot "AutoAnimeDownloader-daemon.exe"
if (-not (Test-Path $daemonExe)) {
    Write-Host "Error: AutoAnimeDownloader-daemon.exe not found at: $daemonExe" -ForegroundColor Red
    Write-Host "Please build the project first using scripts/build.ps1" -ForegroundColor Yellow
    exit 1
}

$serviceName = "AutoAnimeDownloader"
$serviceDisplayName = "Auto Anime Downloader"
$serviceDescription = "Automatically downloads anime from Anilist via qBittorrent"

# Check if service already exists
$existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existingService) {
    Write-Host "Service '$serviceName' already exists. Stopping and removing..." -ForegroundColor Yellow
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    & $nssmPath remove $serviceName confirm
    Start-Sleep -Seconds 2
}

# Install the service
Write-Host "Installing service..." -ForegroundColor Yellow
& $nssmPath install $serviceName $daemonExe

if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Failed to install service." -ForegroundColor Red
    exit 1
}

# Configure service
Write-Host "Configuring service..." -ForegroundColor Yellow
& $nssmPath set $serviceName DisplayName $serviceDisplayName
& $nssmPath set $serviceName Description $serviceDescription
& $nssmPath set $serviceName Start SERVICE_AUTO_START
& $nssmPath set $serviceName AppEnvironmentExtra "ENVIRONMENT=prod" "PORT=:8091"
& $nssmPath set $serviceName AppStdout (Join-Path $ProjectRoot "logs\service.log")
& $nssmPath set $serviceName AppStderr (Join-Path $ProjectRoot "logs\service-error.log")

# Create logs directory
$logsDir = Join-Path $ProjectRoot "logs"
New-Item -ItemType Directory -Force -Path $logsDir | Out-Null

# Start the service
Write-Host "Starting service..." -ForegroundColor Yellow
Start-Service -Name $serviceName

Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host "Service '$serviceName' is now running." -ForegroundColor Green
Write-Host ""
Write-Host "You can:" -ForegroundColor Yellow
Write-Host "  - Access the web UI at http://localhost:8091" -ForegroundColor White
Write-Host "  - Manage the service: Get-Service $serviceName" -ForegroundColor White
Write-Host "  - View logs: Get-Content $logsDir\service.log" -ForegroundColor White

