# Development Guide

For contributors working on AutoAnimeDownloader itself (as opposed to just running it).

## Prerequisites

- **Go** 1.25+
- **Bun** (frontend package manager/build tool — not npm)
- **Docker** (for integration tests and cross-platform builds)
- An **Anilist account** (for manual testing). No external torrent client is needed — the BitTorrent client is embedded.

## Project Structure

```
AutoAnimeDownloader/
├── src/
│   ├── cmd/
│   │   ├── daemon/          # Daemon entry point
│   │   └── cli/             # CLI entry point
│   ├── internal/
│   │   ├── api/              # REST API and WebSocket
│   │   ├── daemon/           # Core daemon logic / verification loop
│   │   ├── files/            # Config and file management
│   │   ├── logger/           # Logging system
│   │   ├── anilist/          # Anilist API client (MAL lists live here too)
│   │   ├── nyaa/             # Nyaa scraper and torrent priority/matching
│   │   ├── torrents/         # Embedded BitTorrent client (rain/v2) behind TorrentBackend
│   │   ├── notifications/    # Webhook notifications
│   │   ├── diagnostics/      # Bug-report bundle and crash files
│   │   ├── datadir/          # Data-dir path (leaf package)
│   │   ├── version/          # Build-time version via ldflags
│   │   ├── tray/             # System tray icon
│   │   └── frontend/         # Svelte frontend (embedded into the daemon binary)
│   └── tests/
│       ├── integration/     # Integration tests (Docker + mock servers)
│       ├── unit/            # Unit tests (use torrents.FakeBackend, not a mock qBittorrent)
│       └── mocks/           # Mock servers (Anilist, Nyaa)
├── docs/                     # Documentation
├── scripts/                  # Build, dev, and test scripts
├── infra/linux/              # systemd unit, desktop entry and the installer Makefile shipped in the release zip
└── build/                    # Build outputs (gitignored)
```

See [Architecture](../agents/architecture.md) for flow, invariants and which file to open.

## Setup

```bash
git clone https://github.com/icarosuper/AutoAnimeDownloader.git
cd AutoAnimeDownloader

go mod download
cd src/internal/frontend && bun install && cd ../../..
```

## Dev Workflow

```bash
make dev
```

Runs the frontend dev server (Vite, hot reload) and the Go daemon together, proxying API calls. See `scripts/dev.sh` for the exact wiring.

To run the daemon alone with verbose console logs:

```bash
ENVIRONMENT=dev go run ./src/cmd/daemon
```

Logs are written to `~/.autoAnimeDownloader/daemon.log` (Linux/macOS) or `%APPDATA%\.autoAnimeDownloader\daemon.log` (Windows); in dev mode they also print to console.

## Testing

```bash
make test                       # everything, with a pass/fail summary
make test-backend-unit          # go test ./src/tests/unit/... ./src/internal/...
make test-backend-integration   # Docker-based integration tests
make test-frontend-unit         # Vitest unit tests
make test-frontend-component    # Vitest + Testing Library component tests
make test-frontend-smoke        # Playwright end-to-end smoke tests
```

For mock patterns and how to write new tests, see [Testing](../agents/testing.md).

## Debugging a Specific Anime

```bash
make debug-anime ID=123   # ID is the AniList media ID (anilist.co/anime/{id})
```

Runs a one-shot search/match pass against Nyaa without downloading anything or touching the daemon state. See [Commands](../agents/commands.md#debugging-a-specific-anime) and the [debug-anime skill](../../.claude/skills/debug-anime/SKILL.md) for details.

## Windows Service Install

Not supported yet. The Windows release is a bare `.exe` you run in the foreground — there is
no installer and no service wrapper. Running detached as a background service is tracked in
`docs/TODO.md` under `v3.1.0` — proper Windows release.

## Contributing

How to report bugs, suggest features and open a pull request: see [CONTRIBUTING.md](../../CONTRIBUTING.md).

## See Also

- [Build Guide](build.md) — building release binaries
- [Release](../../.claude/skills/release/SKILL.md) — tagging a version, GitHub Release, AUR bump (a Claude Code skill; readable as plain steps)
- [CLI Guide](cli.md) — CLI reference
- [WebUI Guide](webui.md) — web interface reference
