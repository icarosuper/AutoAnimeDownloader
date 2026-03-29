# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AutoAnimeDownloader is a Go daemon that automatically downloads anime episodes by integrating Anilist (watching list), Nyaa (torrent scraper), and qBittorrent. It serves an embedded Svelte web UI and exposes a REST API.

## Commands

### Backend (Go)

```bash
# Run unit tests
go test ./...
go test -v ./src/tests/unit/

# Run a single test
go test -v -run TestName ./src/tests/unit/

# Run integration tests (requires Docker)
docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit

# Generate Swagger docs
swag init -g src/cmd/daemon/main.go -o src/internal/api/docs

# Build manually (frontend must be built first)
cd src/internal/frontend && bun install --frozen-lockfile && bun run build && cd ../../..
go build -o build/autoanimedownloader-daemon ./src/cmd/daemon
go build -o build/autoanimedownloader ./src/cmd/cli
```

### Frontend (Svelte + Vite + Bun)

```bash
cd src/internal/frontend
bun install
bun run dev    # dev server (proxies API to localhost:8091)
bun run build  # production build (output embedded in Go binary)
bun run check  # TypeScript type checking
bun run i18n:compile  # recompile i18n messages (paraglide-js)
```

### Full Platform Builds (Docker required)

```bash
make build                          # build all platforms
make build PLATFORM=linuxamd64      # linux x86_64
make build PLATFORM=linuxarm64      # linux ARM64
make build PLATFORM=windows         # windows amd64

# Linux installation
make install    # install daemon, CLI, and systemd service
make uninstall
```

### Running the Daemon

```bash
ENVIRONMENT=dev ./autoanimedownloader-daemon   # formatted logs
PORT=:8092 ./autoanimedownloader-daemon        # custom port (default 8091)
```

## Architecture

### Components

```
src/cmd/daemon/      → Daemon binary entry point (embeds frontend build, starts API)
src/cmd/cli/         → CLI binary for managing the daemon via REST API
src/internal/
  api/               → HTTP server, REST handlers, WebSocket, middleware, Swagger docs
  daemon/            → Verification loop: Anilist → Nyaa → qBittorrent → track episodes
  files/             → Config and episode tracking (JSON files on local filesystem)
  anilist/           → GraphQL client for Anilist API
  nyaa/              → HTML scraper for Nyaa torrent site
  torrents/          → qBittorrent WebUI HTTP client
  frontend/          → Svelte 5 + Vite + Tailwind web UI (compiled to Go embed)
  logger/            → zerolog-based structured logger (console + rotating file)
  tray/              → System tray icon (fyne/systray)
  version/           → Build-time version injection via ldflags
src/tests/
  unit/              → Unit tests with mocks
  integration/       → Docker-based end-to-end tests
  mocks/             → Mock servers for Anilist, Nyaa, qBittorrent
```

### Key Data Flow

1. **Verification loop** (periodic, configurable interval):
   - Fetch user's watching list from Anilist GraphQL API
   - For each anime, scrape Nyaa for matching torrents (filtered by resolution/fansub)
   - Add new episodes to qBittorrent via its WebUI API
   - Record downloaded episodes in `episodes.json` to avoid re-downloading

2. **Frontend embedding**: `bun run build` outputs to `src/internal/frontend/dist/`, which Go embeds via `//go:embed dist` in the API server. The daemon serves the SPA at `/` and proxies `/api/` to the REST handlers.

3. **Real-time updates**: WebSocket at `/api/v1/ws` broadcasts daemon state changes. The Svelte frontend subscribes for live status.

### Configuration & State Files

| File | Location (Linux) | Purpose |
|------|-----------------|---------|
| `config.json` | `~/.autoAnimeDownloader/` | User settings (qBittorrent URL, Anilist username, intervals) |
| `episodes.json` | `~/.autoAnimeDownloader/` | Tracks downloaded episodes |
| `blocked_episodes.json` | `~/.autoAnimeDownloader/` | Episodes to skip |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |

Windows uses `%APPDATA%\AutoAnimeDownloader\` instead.

### API

Swagger UI is available at `http://localhost:8091/swagger/` when the daemon is running.

Key endpoints: `GET /api/v1/status`, `POST /api/v1/loop/start`, `POST /api/v1/loop/stop`, `PUT /api/v1/config`, `POST /api/v1/check` (force check), `WS /api/v1/ws`.

### Version Injection

The version string is injected at build time:
```bash
go build -ldflags "-X AutoAnimeDownloader/src/internal/version.Version=v1.2.0" ./src/cmd/daemon
```
Defaults to `"dev"` when not injected.
