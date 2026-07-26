# AutoAnimeDownloader

**Automatically downloads your anime from your Anilist watching list.**

Syncs with [Anilist](https://anilist.co), scrapes [Nyaa](https://nyaa.si) for matching torrents, and downloads them with a built-in BitTorrent client — all unattended, from a single self-contained binary (no external torrent client to install or configure). Includes an embedded Svelte web UI for monitoring and configuration.

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
- **Embedded BitTorrent client** — downloads and seeds internally (via [rain](https://github.com/cenkalti/rain)); no qBittorrent or other external client required
- **Jellyfin-ready library** — completed episodes are hardlinked into your library folder (optionally with Jellyfin naming) while the original keeps seeding
- **Web UI** — modern browser interface for monitoring, configuration, and control
- **CLI** — command-line interface for scripting and advanced users
- **Real-time Updates** — WebSocket support for live status in the UI
- **Self-contained** — frontend and torrent client embedded in the binary, no separate server needed
- **Cross-platform** — Linux (amd64/arm64) and Windows

## Screenshots

| Dashboard | Anime Status | Configuration |
|-----------|--------------|--------------|
| ![Dashboard](https://i.imgur.com/i8gVepu.png) | ![Anime Status](https://i.imgur.com/vjmUxqL.png) | ![Configuration](https://i.imgur.com/vAFpGTy.png) |

## Requirements

- **Anilist account** (username only, no password needed)
- A **save path** and a **completed anime path** on the **same filesystem/volume** (completed episodes are hardlinked, which cannot cross volumes)

That's it — the BitTorrent client is built in, so there is no qBittorrent (or any other torrent client) to install or configure.

> **Upgrading from a qBittorrent-based version?** See the [Migration Guide](docs/guides/migration-embedded-torrent.md) — `completed_anime_path` is now required and must share a volume with `save_path`, and in-progress torrents re-download once on first run.

> **Ports:** the embedded client listens on a default port range (20000–30000) and finds peers via DHT/PEX. There is no automatic port forwarding (UPnP/NAT-PMP); forwarding a port is optional and only improves inbound connectivity.

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

Grab the zip for your architecture from the [latest release](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest) — `AutoAnimeDownloader_Linux_x86_v*.zip` (x86_64) or `AutoAnimeDownloader_Linux_Arm64_v*.zip` (ARM64).

```bash
unzip AutoAnimeDownloader_Linux_x86_v*.zip
cd AutoAnimeDownloader_Linux_x86_v*
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

Download `AutoAnimeDownloader_Windows_v*.exe` from the [latest release](https://github.com/icarosuper/AutoAnimeDownloader/releases/latest) and run it — no installation needed.

Access the web UI at **http://localhost:8091**.

For running as a Windows Service, see the [Development Guide](docs/guides/development.md#windows-service-install).

See [all releases](https://github.com/icarosuper/AutoAnimeDownloader/releases) for older versions.

## Usage

### Web UI

Open **http://localhost:8091** in your browser.

- **Status** — daemon status, start/stop control, force episode check
- **Episodes** — browse downloaded episodes
- **Config** — Anilist username, paths, intervals

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

**Prerequisites:** Go 1.25+, Node.js 20+, npm, make (Linux)

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
| Save Path | Download/seeding working directory |
| Completed Anime Path | Jellyfin library — required, and must be on the same volume as the Save Path (episodes are hardlinked into it) |
| Check Interval | How often to check for new episodes (minutes) |

## Troubleshooting

**Daemon won't start**
- Check if port 8091 is in use: `ss -tlnp | grep 8091`
- Make sure both the save path and completed anime path are set and on the same volume
- Check service logs: `systemctl --user status autoanimedownloader`

**Downloaded but nothing shows up in the library**
- The save path and completed anime path must be on the **same filesystem/volume** — completed episodes are hardlinked, which cannot cross volumes
- Check the logs for `Organize:` messages: `autoanimedownloader logs --search Organize`

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

## Contributing

Contributions are welcome! See the [Development Guide](docs/guides/development.md#contributing) for details.

## License

Licensed under the [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.html) — see [LICENSE](LICENSE) for details.

## Credits

- Backend: [Go](https://golang.org/)
- Frontend: [Svelte](https://svelte.dev/)
- BitTorrent client: [rain](https://github.com/cenkalti/rain)
- Watch list: [Anilist](https://anilist.co/)
- Torrents: [Nyaa](https://nyaa.si/)
