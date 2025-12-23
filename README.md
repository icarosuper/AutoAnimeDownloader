# AutoAnimeDownloader

**An app that automatically downloads your anime from Anilist!**

It searches [Anilist](https://anilist.co) for the anime you're watching, scrapes [Nyaa](https://nyaa.si) for torrents and downloads them through [qBittorrent](https://www.qbittorrent.org).

[![Build Status](https://github.com/icarosuper/AutoAnimeDownloader/workflows/Build/badge.svg)](https://github.com/icarosuper/AutoAnimeDownloader/actions)

## Features

- 🎯 **Automatic Downloads** - Automatically downloads new episodes from your Anilist watching list
- 🌐 **Web UI** - Modern web interface for monitoring and configuration (embedded in daemon)
- 💻 **CLI** - Command-line interface for advanced users
- 🔄 **Real-time Updates** - WebSocket support for live status updates
- 📦 **Self-contained** - Frontend embedded in binary, no external files needed
- 🐧 **Cross-platform** - Supports Linux (amd64/arm64) and Windows

## Requirements

- **qBittorrent** 4.3.0 or later with WebUI enabled
- **Anilist account** (username only, no password needed)
- **Internet connection**

### Setting up qBittorrent WebUI

1. Open qBittorrent
2. Go to **Tools → Options → Web UI**
3. Enable **Web User Interface (Remote control)**
4. Set a username and password (or leave blank for local use)
5. Note the port (default: 8080)

![qBittorrent WebUI tutorial](https://i.imgur.com/vYgUdyy.png)

## Quick Start

### Linux

1. Download the ZIP for your architecture:
   - [Linux x86_64](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_x86.zip)
   - [Linux ARM64](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_Arm64.zip)

2. Extract and install:
   ```bash
   unzip AutoAnimeDownloader_Linux_x86.zip
   cd AutoAnimeDownloader_Linux_x86
   make install
   ```

3. Access the web UI at http://localhost:8091

### Windows

1. Download the executable:
   [AutoAnimeDownloader_Windows.exe](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Windows.exe)

2. Run the executable - it's self-contained with the frontend embedded!

3. Access the web UI at http://localhost:8091

## Downloads

### Latest Release

- **Windows**: [AutoAnimeDownloader_Windows.exe](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Windows.exe)
- **Linux x86_64**: [AutoAnimeDownloader_Linux_x86.zip](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_x86.zip)
- **Linux ARM64**: [AutoAnimeDownloader_Linux_Arm64.zip](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_Arm64.zip)

See [all releases](https://github.com/icarosuper/AutoAnimeDownloader/releases) for older versions.

## Installation

### Linux

After downloading and extracting the ZIP:

```bash
make install
```

This installs:
- `autoanimedownloader-daemon` - Main daemon with embedded web UI
- `autoanimedownloader` - Command-line interface
- systemd user service (starts automatically)

**Uninstall:**
```bash
make uninstall
```

### Windows

**Standalone:**
Just run `AutoAnimeDownloader_Windows.exe` - it's completely self-contained!

**As Windows Service:**
See [Installation Guide](docs/installation.md#windows-installation) for service installation instructions.

## Usage

### Web UI

After installation, access the web interface at **http://localhost:8091**

- **Status** - View daemon status, control start/stop, force checks
- **Episodes** - Browse downloaded episodes
- **Config** - Configure Anilist username, qBittorrent URL, paths, etc.

### CLI

```bash
# Check daemon status
autoanimedownloader status

# Start/stop daemon loop
autoanimedownloader loop start
autoanimedownloader loop stop

# Force a check for new episodes
autoanimedownloader check

# View configuration
autoanimedownloader config get

# List monitored animes
autoanimedownloader animes

# View logs
autoanimedownloader logs --lines 50
```

See [CLI Guide](docs/cli-guide.md) for more details.

## Building from Source

See [Build Guide](docs/build.md) for detailed instructions.

**Quick build:**
```bash
# Linux
./scripts/build.sh

# Windows
.\scripts\build.ps1
```

**Important:** The frontend must be built before the Go build, as it's embedded into the binary.

## Documentation

- [Installation Guide](docs/installation.md) - Detailed installation instructions
- [Build Guide](docs/build.md) - Building from source
- [CLI Guide](docs/cli-guide.md) - Command-line interface reference
- [WebUI Guide](docs/webui-guide.md) - Web interface guide
- [Development Guide](docs/development.md) - Contributing and development

## Architecture

- **Daemon** - Main service that monitors Anilist and downloads episodes
- **Web UI** - Embedded Svelte frontend (no separate installation needed)
- **CLI** - Command-line tool for managing the daemon
- **API** - REST API with WebSocket support for real-time updates

The frontend is **embedded directly in the daemon binary** using Go's `embed` package, making distribution simple and deployment easy.

## Configuration

Configuration is done through the web UI or CLI. Key settings:

- **Anilist Username** - Your Anilist username
- **qBittorrent URL** - Usually `http://localhost:8080`
- **Save Path** - Where to save downloads
- **Completed Anime Path** - Where to move completed series
- **Check Interval** - How often to check for new episodes (minutes)

Configuration is stored in:
- Linux: `~/.autoAnimeDownloader/config.json`
- Windows: `%APPDATA%\AutoAnimeDownloader\config.json`

## Troubleshooting

**Daemon won't start:**
- Check if port 8091 is available
- Verify qBittorrent WebUI is enabled
- Check logs: `systemctl --user status autoanimedownloader` (Linux)

**Can't connect to qBittorrent:**
- Verify WebUI is enabled in qBittorrent settings
- Check the qBittorrent URL in configuration
- Ensure qBittorrent is running

**Frontend not loading:**
- The frontend is embedded in the binary - if the daemon runs, the frontend should work
- Check browser console for errors
- Verify API is accessible: http://localhost:8091/api/v1/status

See [Installation Guide](docs/installation.md#troubleshooting) for more troubleshooting tips.

## Contributing

Contributions are welcome! Please see [Contributing Guide](docs/contributing.md) for details.

## License

This project is licensed under the GNU General Public License v3.0 (GPL v3) - see the [LICENSE](LICENSE) file for details.

**GPL v3** allows you to:
- ✅ Use, modify, and distribute
- ✅ Use commercially
- ✅ Patent use (with protections)

**You must:**
- ❗ Include copyright notice
- ❗ Include license notice
- ❗ State changes (if any)
- ❗ Make source code available when distributing

For more information, see [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.html).

## Credits

- Built with [Go](https://golang.org/)
- Frontend with [Svelte](https://svelte.dev/)
- Uses [qBittorrent](https://www.qbittorrent.org/) for downloads
- Data from [Anilist](https://anilist.co/)
- Torrents from [Nyaa](https://nyaa.si/)
