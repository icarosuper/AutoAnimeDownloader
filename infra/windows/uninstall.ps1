# Uninstallation script for Windows
# Removes AutoAnimeDownloader Windows service

$ErrorActionPreference = "Stop"

Write-Host "Uninstalling AutoAnimeDownloader Windows Service..." -ForegroundColor Green

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Error: This script must be run as Administrator." -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    exit 1
}

$serviceName = "AutoAnimeDownloader"

# Check if service exists
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if (-not $service) {
    Write-Host "Service '$serviceName' not found. Nothing to uninstall." -ForegroundColor Yellow
    exit 0
}

# Stop the service
Write-Host "Stopping service..." -ForegroundColor Yellow
Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

# Find NSSM
$nssmPath = Get-Command nssm -ErrorAction SilentlyContinue
if (-not $nssmPath) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $nssmLocal = Join-Path $ScriptDir "nssm.exe"
    if (Test-Path $nssmLocal) {
        $nssmPath = $nssmLocal
    } else {
        Write-Host "Error: NSSM not found. Cannot remove service." -ForegroundColor Red
        Write-Host "You may need to manually remove the service using sc.exe delete $serviceName" -ForegroundColor Yellow
        exit 1
    }
} else {
    $nssmPath = $nssmPath.Source
}

# Remove the service
Write-Host "Removing service..." -ForegroundColor Yellow
& $nssmPath remove $serviceName confirm

if ($LASTEXITCODE -eq 0) {
    Write-Host "Service '$serviceName' has been removed successfully." -ForegroundColor Green
} else {
    Write-Host "Warning: Service removal may have failed. You may need to remove it manually." -ForegroundColor Yellow
    Write-Host "Try running: sc.exe delete $serviceName" -ForegroundColor Yellow
}

