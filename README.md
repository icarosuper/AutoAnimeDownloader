# AutoAnimeDownloader

**Automatically downloads your anime from your Anilist watching list.**

Syncs with [Anilist](https://anilist.co) (one or more accounts — or none at all), scrapes [Nyaa](https://nyaa.si) for matching torrents, and downloads them with a built-in BitTorrent client — all unattended, from a single self-contained binary (no external torrent client to install or configure). Includes an embedded Svelte web UI for monitoring, torrent management and configuration.

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
- **Multi-account Anilist** — sync several usernames at once; an episode is only deleted once every account has watched it
- **Standalone animes** — track anime that is in no Anilist list (search + add from the UI), with manual watch progress. An Anilist account is optional; the app runs fine with none
- **Batch / season packs** — when the Nyaa search returns a pack that covers what's missing, it downloads the pack instead of episode by episode (capped by a configurable torrent size ceiling)
- **Embedded BitTorrent client** — downloads and seeds internally (via [rain](https://github.com/cenkalti/rain)); no qBittorrent or other external client required
- **Download queue** — concurrent-download limit with a queue, manual prioritization, pause/resume/announce/delete per torrent or in bulk
- **Disk-space guard** — stops adding torrents below a configurable free-space percentage; free/total space shown on the dashboard
- **Smart torrent picking** — configurable ranking (fansub, resolution, source, codec, audio, health), ignore list, minimum seeders, size ceilings and adaptive Nyaa pagination
- **Jellyfin-ready library** — completed episodes are hardlinked into your library folder (optionally with Jellyfin naming) while the original keeps seeding
- **Webhook notifications** — fire templated webhooks on new episode / download completed / download failed, optionally batched into one message per time window
- **Manual magnet paste** — bypass the search and hand a magnet link straight to the client, per episode or per anime
- **Web UI** — monitoring, live torrent management, configuration and logs in the browser
- **CLI** — command-line interface for scripting and advanced users
- **Real-time Updates** — WebSocket support for live status in the UI
- **Self-contained** — frontend and torrent client embedded in the binary, no separate server needed
- **Cross-platform** — Linux (amd64/arm64) and Windows

## Screenshots

| Dashboard | Anime Status | Configuration |
|-----------|--------------|--------------|
| ![Dashboard](https://i.imgur.com/i8gVepu.png) | ![Anime Status](https://i.imgur.com/vjmUxqL.png) | ![Configuration](https://i.imgur.com/vAFpGTy.png) |

## Requirements

- A **completed anime path** — the library folder. It's the only required setting; the download/seeding directory (`<completed_anime_path>/.torrents`) is derived from it, so the two always share a filesystem
- **Anilist account(s)** (usernames only, no password needed) — *optional*: without one you can still add animes manually as standalone

That's it — the BitTorrent client is built in, so there is no qBittorrent (or any other torrent client) to install or configure.

> **Upgrading from a qBittorrent-based version?** See the [Migration Guide](docs/guides/migration-embedded-torrent.md) — `completed_anime_path` is now required. There is no separate `save_path` field anymore: the download directory lives nested inside `completed_anime_path` (same volume by construction), and any pre-existing torrent data at a legacy `save_path` is migrated into the new location automatically on first boot after upgrading.

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

- **Status** (`#/`) — daemon status, start/stop, force check, download speed, disk space, and the anime list; click an anime for its episodes and per-episode actions
- **Add anime** (`#/add`) — search Anilist and track an anime that is in no list
- **Downloads** (`#/downloads`) — live torrents grouped by anime: pause, resume, re-announce, prioritize, delete (single or bulk)
- **Config** (`#/config`) — library path, Anilist usernames, download rules, torrent search tuning
- **Priorities** (`#/priorities`) — fansub/resolution/source/codec/audio ranking and ignore list
- **Notifications** (`#/notifications`) — webhook presets and test firing
- **Logs** (`#/logs`) — tail the daemon log with level filter and search

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
| Completed Anime Path | Jellyfin library — **the only required setting**. Episodes are hardlinked into it; torrents download and seed in `<path>/.torrents` |
| Anilist Usernames | One or more Anilist usernames to sync (optional) |
| Check Interval | How often to check for new episodes (minutes) |
| Download / Delete Statuses | Which Anilist list statuses (`CURRENT`, `COMPLETED`, …) and media statuses (`RELEASING`, `FINISHED`, …) are eligible for download or auto-deletion |
| Max Episodes per Anime | Ceiling of kept episodes per anime, and the width of the pack-selection window |
| Max Concurrent Downloads | How many torrents download at once; the rest wait in the queue |
| Min Free Disk % | Below this, no new torrent is added |
| Max Batch / Episode Torrent Size | Size ceilings (GiB) dropping oversized results from the search |
| Min Seeders / Max Search Pages | Nyaa result floor and how deep the paginated search may go |
| Rename Files for Jellyfin | Name the library hardlink `Anime Name - E05.mkv` |
| Notifications | Webhook presets and the batching window |

Full field-by-field reference: [Config Reference](docs/agents/config.md).

## Troubleshooting

**Daemon won't start**
- Check if port 8091 is in use: `ss -tlnp | grep 8091`
- Make sure the completed anime path is set
- Check service logs: `systemctl --user status autoanimedownloader`

**Downloaded but nothing shows up in the library**
- The library filesystem must support hardlinks (exFAT/FAT32 and some SMB shares don't) — the daemon rejects such a path on save
- Check the logs for `Organize:` messages: `autoanimedownloader logs --search Organize`

**Anime not found on Nyaa**
- The anime title from Anilist may not match Nyaa's naming — set a custom search title in the anime's detail page
- Relax or adjust the subtitle group / resolution filters

**No torrents matching filters**
- Check your priority lists (resolution, fansub) and the ignore list on `#/priorities`
- Lower `min_seeders` or raise the size ceilings — results below/above them are dropped before ranking

**Downloads stuck as "queued"**
- That's the concurrency limit (`max_concurrent_downloads`); raise it, or prioritize a torrent from the Downloads screen

**Nothing is being added anymore**
- Free space may be below `min_free_disk_percent` — the dashboard shows a low-disk warning while that's the case

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
