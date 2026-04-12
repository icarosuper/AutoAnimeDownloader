# Architecture

## Components

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

## Key Data Flow

1. **Verification loop** (periodic, configurable interval):
   - Fetch watch list from Anilist GraphQL API
   - Per anime: scrape Nyaa for matching torrents (filter by resolution/fansub)
   - Add new episodes to qBittorrent via WebUI API
   - Record downloaded episodes in `episodes.json` — skip re-downloads

2. **Frontend embedding**: `bun run build` → `src/internal/frontend/dist/`, Go embeds via `//go:embed dist` in API server. Daemon serves SPA at `/`, proxies `/api/` to REST handlers.

3. **Real-time updates**: WebSocket at `/api/v1/ws` broadcasts daemon state changes. Svelte frontend subscribes for live status.

## Configuration & State Files

| File | Location (Linux) | Purpose |
|------|-----------------|---------|
| `config.json` | `~/.autoAnimeDownloader/` | User settings (qBittorrent URL, Anilist username, intervals) |
| `episodes.json` | `~/.autoAnimeDownloader/` | Tracks downloaded episodes |
| `blocked_episodes.json` | `~/.autoAnimeDownloader/` | Episodes to skip |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |

Windows uses `%APPDATA%\AutoAnimeDownloader\` instead.

## API

Swagger UI at `http://localhost:8091/swagger/` when daemon running.

Key endpoints:

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `GET` | `/api/v1/status` | Daemon status |
| `POST` | `/api/v1/loop/start` | Start verification loop |
| `POST` | `/api/v1/loop/stop` | Stop verification loop |
| `PUT` | `/api/v1/config` | Update config |
| `POST` | `/api/v1/check` | Force check now |
| `WS` | `/api/v1/ws` | Real-time state updates |

## Version Injection

Version injected at build time via ldflags:
```bash
go build -ldflags "-X AutoAnimeDownloader/src/internal/version.Version=v1.2.0" ./src/cmd/daemon
```
Defaults to `"dev"` if not injected.
