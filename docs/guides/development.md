# Development Guide

For contributors working on AutoAnimeDownloader itself (as opposed to just running it).

## Prerequisites

- **Go** 1.24+
- **Bun** (frontend package manager/build tool — not npm)
- **Docker** (for integration tests and cross-platform builds)
- **qBittorrent** with WebUI enabled, and an **Anilist account** (for manual testing)

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
│   │   ├── anilist/          # Anilist API client
│   │   ├── nyaa/             # Nyaa scraper and torrent priority/matching
│   │   ├── torrents/         # qBittorrent client
│   │   ├── notifications/    # Webhook notifications
│   │   ├── tray/             # System tray icon
│   │   └── frontend/         # Svelte frontend (embedded into the daemon binary)
│   └── tests/
│       ├── integration/     # Integration tests (Docker + mock servers)
│       ├── unit/            # Unit tests
│       └── mocks/           # Mock servers (Anilist, Nyaa, qBittorrent)
├── docs/                     # Documentation
├── scripts/                  # Build, dev, and test scripts
├── infra/                    # systemd/NSSM service files and installers
└── build/                    # Build outputs (gitignored)
```

See [Architecture](../agents/architecture.md) for a package-by-package map of symbols and data flow.

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

Logs are written to `~/.autoAnimeDownloader/daemon.log` (Linux/macOS) or `%APPDATA%\AutoAnimeDownloader\daemon.log` (Windows); in dev mode they also print to console.

Swagger UI for the API is at `http://localhost:8091/swagger/` while the daemon is running.

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
make debug-anime ID=123   # ID is the AniList MediaList ID
```

Runs a one-shot search/match pass against Nyaa without touching qBittorrent or the daemon state. See [Commands](../agents/commands.md#debugging-a-specific-anime) and [Troubleshooting Downloads](../agents/troubleshooting-downloads.md) for details.

## Regenerating Swagger Docs

After changing API endpoints or their comments in `src/cmd/daemon/main.go`:

```bash
swag init -g src/cmd/daemon/main.go -o docs/swagger
```

## Windows Service Install

To run the daemon as a Windows service (via [NSSM](https://nssm.cc/download)):

```powershell
# Run PowerShell as Administrator
cd infra\windows
.\install.ps1
```

`.\uninstall.ps1` removes it. See `infra/windows/install.ps1` for what it configures if you need to do it manually.

## Contributing

1. Fork the repo and create a branch off `master`: `git checkout -b feat/your-feature`
2. Make your changes — run `go test ./...` and, for frontend changes, `make test-frontend` before opening a PR
3. Push and open a pull request against `master`

### Commit Messages

```
feat: add new endpoint for episode filtering
fix: resolve memory leak in state manager
docs: update build guide
test: add tests for config validation
refactor: simplify daemon loop logic
```

### Code Style

- **Go**: `gofmt`, handle all errors, follow the patterns in [Conventions](../agents/conventions.md)
- **Frontend**: Svelte + Tailwind, formatted with Prettier

### Reporting Bugs

Include: steps to reproduce, expected vs. actual behavior, relevant log lines (`autoanimedownloader logs` or `~/.autoAnimeDownloader/daemon.log`), and your OS/version.

## See Also

- [Build Guide](build.md) — building release binaries
- [CLI Guide](cli.md) — CLI reference
- [WebUI Guide](webui.md) — web interface reference
