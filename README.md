# AutoAnimeDownloader

**Automatically downloads your anime from your Anilist watching list.**

Syncs with [Anilist](https://anilist.co), scrapes [Nyaa](https://nyaa.si) for matching torrents, and sends them to [qBittorrent](https://www.qbittorrent.org) — all unattended. Includes an embedded Svelte web UI for monitoring and configuration.

[![Build Status](https://github.com/icarosuper/AutoAnimeDownloader/workflows/Build/badge.svg)](https://github.com/icarosuper/AutoAnimeDownloader/actions)

---

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Requirements](#requirements)
- [Installation](#installation)
- [Usage](#usage)
- [Building from Source](#building-from-source)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Automatic Downloads** — monitors your Anilist watching list and downloads new episodes as they air
- **Web UI** — modern browser interface for monitoring, configuration, and control
- **CLI** — command-line interface for scripting and advanced users
- **Real-time Updates** — WebSocket support for live status in the UI
- **Self-contained** — frontend embedded in the binary, no separate server needed
- **Cross-platform** — Linux (amd64/arm64) and Windows

## Screenshots

| Dashboard | Anime Status | Configuration |
|-----------|--------------|--------------|
| ![Dashboard](https://i.imgur.com/i8gVepu.png) | ![Anime Status](https://i.imgur.com/vjmUxqL.png) | ![Configuration](https://i.imgur.com/vAFpGTy.png) |

## Requirements

- **qBittorrent** 4.3.0+ with WebUI enabled
- **Anilist account** (username only, no password needed)

### Setting up qBittorrent WebUI

1. Open qBittorrent → **Tools → Options → Web UI**
2. Enable **Web User Interface (Remote control)**
3. Set a username and password (or leave blank for local use)
4. Note the port (default: 8080)

![qBittorrent WebUI tutorial](https://i.imgur.com/vYgUdyy.png)

## Installation

### Arch Linux (AUR)

```bash
# using yay
yay -S autoanimedownloader-bin

# using paru
paru -S autoanimedownloader-bin
```

After install, the systemd user service starts automatically. Access the web UI at **http://localhost:8091**.

### Linux (other distros)

Download the ZIP for your architecture:

| Architecture | Download |
|---|---|
| x86_64 | [AutoAnimeDownloader_Linux_x86_v1.3.2.zip](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_x86_v1.3.2.zip) |
| ARM64 | [AutoAnimeDownloader_Linux_Arm64_v1.3.2.zip](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Linux_Arm64_v1.3.2.zip) |

```bash
unzip AutoAnimeDownloader_Linux_x86_v1.3.2.zip
cd AutoAnimeDownloader_Linux_x86_v1.3.2
make install
```

This installs:
- `autoanimedownloader-daemon` — main daemon with embedded web UI
- `autoanimedownloader` — command-line interface
- systemd user service (starts automatically)

Access the web UI at **http://localhost:8091**.

**Uninstall:**
```bash
make uninstall
```

### Windows

Download [AutoAnimeDownloader_Windows_v1.3.2.exe](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest/download/AutoAnimeDownloader_Windows_v1.3.2.exe) and run it — no installation needed.

Access the web UI at **http://localhost:8091**.

For running as a Windows Service, see the [Installation Guide](docs/guides/installation.md#windows-installation).

See [all releases](https://github.com/icarosuper/AutoAnimeDownloader/releases) for older versions.

## Usage

### Web UI

Open **http://localhost:8091** in your browser.

- **Status** — daemon status, start/stop control, force episode check
- **Episodes** — browse downloaded episodes
- **Config** — Anilist username, qBittorrent URL, paths, intervals

### CLI

```bash
autoanimedownloader status          # check daemon status
autoanimedownloader loop start      # start the download loop
autoanimedownloader loop stop       # stop the download loop
autoanimedownloader check           # force a check for new episodes
autoanimedownloader config get      # view current configuration
autoanimedownloader animes          # list monitored anime
autoanimedownloader logs --lines 50 # view recent logs
```

See the [CLI Guide](docs/guides/cli.md) for the full reference.

## Building from Source

**Prerequisites:** Go 1.24+, Node.js 20+, npm, make (Linux)

The frontend must be built before the Go binaries, as it's embedded into the daemon.

```bash
git clone https://github.com/icarosuper/AutoAnimeDownloader.git
cd AutoAnimeDownloader

# Linux/macOS/WSL
make build

# Windows
.\scripts\build.ps1
```

See the [Build Guide](docs/guides/build.md) for full details including cross-compilation and Docker builds.

## Configuration

Configure through the web UI or CLI. Settings are stored in:
- Linux: `~/.autoAnimeDownloader/config.json`
- Windows: `%APPDATA%\AutoAnimeDownloader\config.json`

Key settings:

| Setting | Description |
|---|---|
| Anilist Username | Your Anilist username |
| qBittorrent URL | Usually `http://localhost:8080` |
| Save Path | Where to save in-progress downloads |
| Completed Anime Path | Where to move finished series |
| Check Interval | How often to check for new episodes (minutes) |

## Troubleshooting

**Daemon won't start**
- Check if port 8091 is in use: `ss -tlnp | grep 8091`
- Verify qBittorrent WebUI is enabled
- Check service logs: `systemctl --user status autoanimedownloader`

**Can't connect to qBittorrent**
- Verify WebUI is enabled in qBittorrent settings
- Confirm the URL and port in AAD configuration
- Ensure qBittorrent is running before starting AAD

**Anime not found on Nyaa**
- The anime title from Anilist may not match Nyaa's naming — set a custom search title in the anime config
- Relax or adjust the subtitle group / resolution filters

**No torrents matching filters**
- Check your preferred resolution and subtitle group settings
- Try leaving filters blank to see if any results come back

**Frontend not loading**
- The frontend is embedded — if the daemon is running, the UI should work
- Verify API is reachable: http://localhost:8091/api/v1/status
- Check browser console for errors

See the [Installation Guide](docs/guides/installation.md#troubleshooting) for more.

## Contributing

Contributions are welcome! See the [Contributing Guide](docs/guides/contributing.md) for details.

## License

Licensed under the [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.html) — see [LICENSE](LICENSE) for details.

## Credits

- Backend: [Go](https://golang.org/)
- Frontend: [Svelte](https://svelte.dev/)
- Downloads: [qBittorrent](https://www.qbittorrent.org/)
- Watch list: [Anilist](https://anilist.co/)
- Torrents: [Nyaa](https://nyaa.si/)
