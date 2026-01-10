# Installation Guide

This guide explains how to install AutoAnimeDownloader on Linux and Windows.

## Prerequisites

- **qBittorrent** installed and running
- **qBittorrent WebUI** enabled (Settings → Web UI)
- **Anilist account** (username only, no password needed)

## Linux Installation

### Quick Install

1. Download the appropriate ZIP file for your architecture:
   - [Linux x86_64](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_x86.zip)
   - [Linux ARM64](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_Arm64.zip)

2. Extract the archive:
   ```bash
   unzip AutoAnimeDownloader_Linux_x86.zip
   cd AutoAnimeDownloader_Linux_x86
   ```

3. Install with one command:
   ```bash
   make install
   ```

That's it! The installation will:
- Install the daemon binary to `~/.local/bin/autoanimedownloader-daemon`
- Install the CLI binary to `~/.local/bin/autoanimedownloader`
- Install and start the systemd user service

### Manual Installation

If you prefer to install manually:

```bash
# Copy binaries
mkdir -p ~/.local/bin
cp autoanimedownloader-daemon ~/.local/bin/
cp autoanimedownloader ~/.local/bin/

# Install systemd service
mkdir -p ~/.config/systemd/user
cp autoanimedownloader.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable autoanimedownloader
systemctl --user start autoanimedownloader
```

### Verification

Check if the service is running:

```bash
systemctl --user status autoanimedownloader
```

Access the web UI at http://localhost:8091

### Uninstallation

To uninstall:

```bash
cd AutoAnimeDownloader_Linux_x86  # or wherever you extracted it
make uninstall
```

Or manually:

```bash
systemctl --user stop autoanimedownloader
systemctl --user disable autoanimedownloader
rm ~/.config/systemd/user/autoanimedownloader.service
rm ~/.local/bin/autoanimedownloader-daemon
rm ~/.local/bin/autoanimedownloader
systemctl --user daemon-reload
```

## Windows Installation

### Quick Install

1. Download the Windows executable:
   [AutoAnimeDownloader_Windows.exe](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Windows.exe)

2. Run the executable:
   ```
   AutoAnimeDownloader_Windows.exe
   ```

The daemon will start and be accessible at http://localhost:8091

### Install as Windows Service

To run AutoAnimeDownloader as a Windows service:

1. Download and install [NSSM](https://nssm.cc/download) (Non-Sucking Service Manager)

2. Run the installation script as Administrator:
   ```powershell
   # Right-click PowerShell and select "Run as Administrator"
   cd infra\windows
   .\install.ps1
   ```

The script will:
- Install AutoAnimeDownloader as a Windows service
- Configure it to start automatically
- Start the service immediately

### Manual Service Installation

If you prefer to install the service manually:

```powershell
# Run PowerShell as Administrator
nssm install AutoAnimeDownloader "C:\path\to\autoanimedownloader-daemon.exe"
nssm set AutoAnimeDownloader DisplayName "Auto Anime Downloader"
nssm set AutoAnimeDownloader Description "Automatically downloads anime from Anilist via qBittorrent"
nssm set AutoAnimeDownloader Start SERVICE_AUTO_START
nssm set AutoAnimeDownloader AppEnvironmentExtra "ENVIRONMENT=prod" "PORT=:8091"
nssm start AutoAnimeDownloader
```

### Uninstallation

To remove the Windows service:

```powershell
# Run PowerShell as Administrator
cd infra\windows
.\uninstall.ps1
```

Or manually:

```powershell
nssm stop AutoAnimeDownloader
nssm remove AutoAnimeDownloader confirm
```

## Configuration

### Initial Setup

1. Start the daemon (it should start automatically after installation)

2. Access the web UI at http://localhost:8091

3. Go to the Config page and configure:
   - **Anilist Username** - Your Anilist username
   - **qBittorrent URL** - Usually `http://localhost:8080` (or your qBittorrent WebUI URL)
   - **Save Path** - Where to save downloaded episodes
   - **Completed Anime Path** - Where to move completed anime
   - **Check Interval** - How often to check for new episodes (in minutes)

4. Click "Save" to apply the configuration

### Environment Variables

You can configure the daemon using environment variables:

- `ENVIRONMENT` - Set to `prod` for production (default: `dev`)
- `PORT` - Port to listen on (default: `:8091`)
- `QBITTORRENT_URL` - qBittorrent WebUI URL (default: `http://localhost:8080`)
- `ANILIST_USERNAME` - Your Anilist username
- `SAVE_PATH` - Path to save downloads
- `COMPLETED_ANIME_PATH` - Path for completed anime
- `CHECK_INTERVAL` - Check interval in minutes (default: `10`)

For Linux (systemd), add environment variables to the service file:

```ini
[Service]
Environment="QBITTORRENT_URL=http://localhost:8080"
Environment="ANILIST_USERNAME=yourusername"
```

For Windows (NSSM), use:

```powershell
nssm set AutoAnimeDownloader AppEnvironmentExtra "QBITTORRENT_URL=http://localhost:8080" "ANILIST_USERNAME=yourusername"
```

## Usage

### Web UI

Access the web interface at http://localhost:8091

- **Status** - View daemon status and control it
- **Episodes** - View downloaded episodes
- **Config** - Configure the daemon

### CLI

Use the CLI to manage the daemon:

```bash
# Check status
autoanimedownloader status

# Start the daemon loop
autoanimedownloader loop start

# Stop the daemon loop
autoanimedownloader loop stop

# Force a check
autoanimedownloader check

# View configuration
autoanimedownloader config get

# List animes
autoanimedownloader animes

# View logs
autoanimedownloader logs
```

## Troubleshooting

### Daemon won't start

- Check if the port 8091 is already in use: `netstat -tuln | grep 8091` (Linux) or `netstat -an | findstr 8091` (Windows)
- Check logs: `systemctl --user status autoanimedownloader` (Linux) or check the service logs (Windows)

### Can't connect to qBittorrent

- Verify qBittorrent WebUI is enabled
- Check the qBittorrent URL in the configuration
- Ensure qBittorrent is running and accessible

### Frontend not loading

The frontend is embedded in the daemon binary, so if the daemon is running, the frontend should be available. If you see a blank page:

- Check browser console for errors
- Verify the daemon is running: `systemctl --user status autoanimedownloader` (Linux)
- Try accessing http://localhost:8091/api/v1/status to verify the API is working

### Service won't start on boot

**Linux:**
- Ensure the service is enabled: `systemctl --user enable autoanimedownloader`
- Check if user services are enabled: `loginctl enable-linger $USER`

**Windows:**
- Verify the service is set to start automatically: Check Services (services.msc)
- Ensure NSSM is properly configured

## Requirements

- **qBittorrent** 4.3.0 or later
- **Anilist account** (free, username only)
- **Internet connection** for downloading torrents

## Notes

- The frontend is **embedded** in the daemon binary - no separate installation needed
- The daemon runs as a **user service** (Linux) or Windows service
- All configuration is stored in `~/.autoAnimeDownloader/config.json` (Linux) or `%APPDATA%\AutoAnimeDownloader\config.json` (Windows)

