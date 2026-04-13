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
| `episodes.json` | `~/.autoAnimeDownloader/` | Tracks downloaded episodes (JSONL) |
| `blocked_episodes.json` | `~/.autoAnimeDownloader/` | Episodes to skip (JSON array of IDs) |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |

Windows uses `%APPDATA%\AutoAnimeDownloader\` instead.

## API

Swagger UI at `http://localhost:8091/swagger/` when daemon running.

Key endpoints:

| Method | Endpoint | Handler func | File |
|--------|----------|-------------|------|
| `GET` | `/api/v1/status` | `handleStatus` | `endpoint_status.go` |
| `GET/PUT` | `/api/v1/config` | `handleConfig` | `endpoint_config.go` |
| `GET` | `/api/v1/animes` | `handleAnimes` | `endpoint_animes.go` |
| `GET` | `/api/v1/animes/{id}/episodes` | `handleAnimeEpisodes` | `endpoint_anime_episodes.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeId}/download` | `handleDownloadEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeId}/redownload` | `handleRedownloadEpisode` | `endpoint_episode_actions.go` |
| `DELETE` | `/api/v1/animes/{id}/episodes/{episodeId}` | `handleDeleteEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeId}/release` | `handleReleaseEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeId}/replace` | `handleReplaceEpisodeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/replace` | `handleReplaceAnimeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/check` | `handleCheck` | `endpoint_check.go` |
| `POST` | `/api/v1/daemon/start` | `handleDaemonStart` | `endpoint_daemon_start.go` |
| `POST` | `/api/v1/daemon/stop` | `handleDaemonStop` | `endpoint_daemon_stop.go` |
| `GET` | `/api/v1/logs` | `handleLogs` | `endpoint_logs.go` |
| `WS` | `/api/v1/ws` | `handleWebSocket` | `websocket.go` |

## Version Injection

Version injected at build time via ldflags:
```bash
go build -ldflags "-X AutoAnimeDownloader/src/internal/version.Version=v1.2.0" ./src/cmd/daemon
```
Defaults to `"dev"` if not injected.

---

## Detailed File Map

### `src/internal/daemon/daemon.go`

Main verification orchestrator. Key functions:

| Function | Purpose |
|----------|---------|
| `StartLoop(p)` | Creates goroutine loop, returns `LoopControl` (Cancel/UpdateInterval) |
| `AnimeVerification(ctx, fm, state)` | Main check: fetches Anilist → Nyaa → qBittorrent |
| `processAnimeEpisodes(...)` | Per-anime: decide download/delete per episode, execute download strategy |
| `checkEpisode(...)` | Returns `(shouldDownload, shouldDelete)` per episode |
| `shouldSkipEpisode(...)` | Skip if: excluded list, already watched, not yet aired |
| `handleAlreadySavedEpisode(...)` | Re-download if missing from torrents, delete if over limit |
| `handleSavedEpisodes(...)` | Post-loop: save new, delete watched, delete torrent files |
| `attemptDownloadWithRetries(...)` | Tries up to `EpisodeRetryLimit` magnets, returns first hash |
| `searchNyaaForSingleEpisode(...)` | Single ep search — used as fallback (priority 4) |
| `searchNyaaForBatch(...)` | Batch search for finished animes (priority 2) |
| `searchNyaaForMovie(...)` | Movie search (priority 1) |
| `searchNyaaForMultipleEpisodes(...)` | Multi-episode search for airing animes (priority 3) |
| `ensureAnimeIsInCompletedFolder(...)` | Moves finished anime torrents to CompletedAnimePath |
| `ManualDownloadEpisode(animeId, episodeId, cfg)` | Used by API for manual download — calls Anilist then Nyaa |
| `ManualDownloadEpisodeWithMagnet(...)` | Used by API for replace-with-magnet per episode |
| `ManualDownloadAnimeWithMagnet(...)` | Used by API for replace-with-magnet for full anime batch |

**Download priority** (in `processAnimeEpisodes`):
1. Movie → `searchNyaaForMovie` → `skipSubfolder=true`, epName = animeName
2. Batch (finished + >1 ep) → `searchNyaaForBatch` → `skipSubfolder=true`
3. Multiple ep search → `searchNyaaForMultipleEpisodes`
4. Single ep fallback → `searchNyaaForSingleEpisode`

### `src/internal/daemon/state.go`

Thread-safe daemon state. Key types:

| Symbol | Purpose |
|--------|---------|
| `Status` (string enum) | `stopped` / `running` / `checking` |
| `State` struct | Holds `status`, `lastCheck`, `lastCheckError`, notifier |
| `StateNotifier` interface | `NotifyStateChange(status, lastCheck, hasError)` — WebSocket subscribes |
| `State.GetAll()` | Returns `(status, lastCheck, hasError)` atomically |
| `State.SetStatus(s)` | Sets status and fires notifier if changed |

### `src/internal/files/filemanager.go`

All persistence. Key types:

| Symbol | Purpose |
|--------|---------|
| `Config` struct | All user settings — maps to `config.json` |
| `EpisodeStruct` struct | `EpisodeID`, `AnimeID`, `EpisodeHash`, `EpisodeName`, `DownloadDate`, `ManuallyManaged` |
| `FileManagerInterface` | Interface used by daemon + API — mock in tests |
| `FileManager.LoadConfigs()` | Reads `config.json`; creates with defaults if missing |
| `FileManager.LoadSavedEpisodes()` | Reads `episodes.json` (JSONL), migrates old format |
| `FileManager.SaveEpisodesToFile(eps)` | Appends only new episodes (deduped by ID) |
| `FileManager.DeleteEpisodesFromFile(ids)` | Removes episodes by ID from JSONL |
| `FileManager.BlockEpisode(id)` | Appends ID to `blocked_episodes.json` |
| `FileManager.UnblockEpisode(id)` | Removes ID from `blocked_episodes.json` |
| `FileManager.UnmanageEpisode(id)` | Sets `ManuallyManaged=false` for episode |
| `FileManager.DeleteEmptyFolders(...)` | Removes empty dirs in `savePath` and `completedPath` |

Config defaults: `CheckInterval=10`, `MaxEpisodesPerAnime=12`, `EpisodeRetryLimit=5`, `QBittorrentUrl="http://127.0.0.1:8080"`.

### `src/internal/files/filesystem.go`

`FileSystem` interface + `OSFileSystem` implementation. Used for testability — tests inject `MockFileSystem`.

### `src/internal/files/parser.go`

`ParseEpisodes(string)` and `SerializeEpisodes([]EpisodeStruct)` — JSONL format (one JSON object per line).

### `src/internal/api/server.go`

| Symbol | Purpose |
|--------|---------|
| `Server` struct | Wraps `http.Server` + `State`, `FileManager`, `WSManager`, `currentLoopControl` |
| `NewServer(port, state, fm, startLoopFunc)` | Constructor — wires WebSocket state getter |
| `Server.SetupRoutes()` | Registers all routes on `http.ServeMux` |
| `Server.StartDaemonLoop()` | Loads config → calls `StartLoopFunc` → stores `LoopControl` |
| `Server.StopDaemonLoop()` | Cancels loop, waits for goroutine, sets `StatusStopped` |
| `Server.handleStaticFiles()` | Serves embedded SPA; falls back to `index.html` for SPA routing |
| `FileManagerInterface` | Same methods as `daemon.FileManagerInterface` — separate declaration, same shape |

Middleware stack (API routes): CORS → JSON Content-Type → Logging. Static files: CORS → Logging only.

### `src/internal/api/middleware.go`

- `corsMiddleware` — allows localhost origins only
- `jsonMiddleware` — sets `Content-Type: application/json`
- `loggingMiddleware` — logs method, path, status, duration
- `ApplyMiddlewares(h)` — applies all three in order

### `src/internal/api/responses.go`

- `SuccessResponse` struct — `{success, data, error}` wraps all responses
- `JSONSuccess(w, code, data)` / `JSONError(w, code, errCode, msg)` / `JSONInternalError(w, err)`

### `src/internal/api/endpoint_animes.go`

- `AnimeInfo` struct — aggregated anime info from `episodes.json`
- `handleAnimes` — groups saved episodes by anime name, merges current AniList watching list (so animes with 0 eps still show)
- `mergeCurrentAniListAnimes` — adds AniList CURRENT/REPEATING animes not yet in episodes.json
- `extractAnimeName(episodeName)` — strips episode number suffix from torrent name to get anime name

### `src/internal/api/endpoint_anime_episodes.go`

- `AnimeEpisodeInfo` struct — per-episode detail (aired, watched, downloaded, blocked, manually managed)
- `AnimeDetailResponse` struct — `{animeId, totalEpisodes, progress, status, episodes[]}`
- `handleAnimeEpisodes` — fetches `GetAnimeInfo(id)` from AniList + saved episodes + blocked list → merges

### `src/internal/api/endpoint_episode_actions.go`

All episode mutation endpoints. Each shares same pattern:
1. Parse path params
2. Load config + saved episodes
3. Create `TorrentService`
4. Call `daemon.ManualDownload*` or direct `torrentsService` operation
5. Update `FileManager` (save/delete/block/unblock)

Actions: `download`, `redownload`, `delete` (+ block), `release` (unblock + unmanage), `replace` (per episode magnet), `replaceAnime` (full anime magnet).

### `src/internal/api/websocket.go`

- `WebSocketManager` — manages connected clients, broadcasts state changes
- `WebSocketManager.HandleWebSocket(w, r)` — upgrades connection, adds client, sends current state immediately
- `WebSocketManager.NotifyStateChange(status, lastCheck, hasError)` — implements `StateNotifier`, broadcasts to all clients
- `WebSocketManager.SetStateGetter(fn)` — wires initial state on new connection

### `src/internal/anilist/anilist.go`

| Symbol | Purpose |
|--------|---------|
| `AniListResponse` | Response for `GetAllCurrentAnime` — `Data.Page.MediaList[]` |
| `MediaListDetailResponse` | Response for `GetAnimeInfo` — single `MediaList` with full `AiringSchedule` |
| `MediaList` struct | `Id`, `Status`, `Progress`, `CustomLists`, `Media` |
| `Media` struct | `Format`, `Status`, `Title`, `Episodes`, `AiringSchedule` |
| `AiringNode` struct | `ID`, `Episode`, `TimeUntilAiring`, `AiringAt` |
| `MediaFormat` consts | `TV`, `MOVIE`, `OVA`, `ONA`, etc. |
| `MediaListStatus` consts | `CURRENT`, `COMPLETED`, `DROPPED`, `PAUSED`, `PLANNING`, `REPEATING` |
| `GetAllCurrentAnime(username)` | Fetches CURRENT+REPEATING anime list (used by verification loop) |
| `GetAnimeInfo(mediaListId)` | Fetches single anime detail with full airing schedule (used by `/animes/{id}/episodes`) |
| `sendAnilistRequest[T]` | Generic GraphQL POST helper |
| `httpDo` var | Swappable HTTP func — overridden in tests via `MockAniListDo` |

### `src/internal/nyaa/nyaa.go`

| Symbol | Purpose |
|--------|---------|
| `TorrentResult` struct | `Name`, `MagnetLink`, `Seeders`, `Episode*`, `Resolution*`, `Season*`, `Size`, `Fansub`, `IsBatch` |
| `ScrapNyaa(title, episode)` | Scrapes Nyaa for a single episode |
| `ScrapNyaaForBatch(title, season*)` | Scrapes for batch (completed anime) |
| `ScrapNyaaForMovie(title, isMovie)` | Scrapes for movie |
| `ScrapNyaaForMultipleEpisodes(title, eps[])` | Scrapes for multiple specific episodes |
| `GenerateSearchTitleVariants(romaji, english)` | Generates search query variants: clean romaji → original romaji → clean english → original english |
| `httpGet` var | Swappable — overridden in tests via `MockNyaaHttpGet` |
| `getNyaaBaseURL()` | Reads `NYAA_URL` env or defaults to `https://nyaa.si` |

### `src/internal/torrents/torrents.go`

| Symbol | Purpose |
|--------|---------|
| `TorrentService` struct | `httpClient`, `baseURL`, `savePath`, `completedPath` |
| `Torrent` struct | `Hash`, `Magnet`, `Name`, `SavePath`, `ContentPath` |
| `NewTorrentService(client, url, save, completed)` | Constructor |
| `TorrentService.GetDownloadedTorrents()` | Lists torrents in `autoAnimeDownloader` category |
| `TorrentService.DownloadTorrent(magnet, animeName, epName, isCompleted)` | Adds torrent, waits for hash, returns hash |
| `TorrentService.DownloadTorrentWithOptions(..., skipSubfolder)` | Like above but `skipSubfolder=true` saves directly in savePath |
| `TorrentService.DeleteTorrents(hashes[])` | Deletes torrents + files |
| `TorrentService.RenameEpisodeFile(hash, animeName, epNum)` | Renames file to Jellyfin-compatible name (async, called in goroutine) |
| `TorrentService.MoveToCompletedFolder(hash)` | Sets torrent location to `completedPath` |
| `HTTPClient` interface | `Get(url)`, `PostForm(url, data)` — `DefaultHTTPClient` wraps std lib |
| `CATEGORY` const | `"autoAnimeDownloader"` — used to tag/filter torrents |

### `src/internal/stringutil/stringutil.go`

- `RemoveSpecialCharacters(s)` — strips chars that break Nyaa search queries

### `src/internal/logger/logger.go`

- `Logger` global — zerolog instance, writes to console + rotating `daemon.log`

### `src/internal/frontend/src/`

**Routes** (rendered via hash router):

| File | Route | Purpose |
|------|-------|---------|
| `routes/Status.svelte` | `#/` | Daemon status, start/stop, anime list |
| `routes/AnimeDetail.svelte` | `#/anime/:id` | Per-anime episode list + actions |
| `routes/Config.svelte` | `#/config` | Edit all config fields |
| `routes/Logs.svelte` | `#/logs` | Tail daemon logs |

**Shared components** (`src/components/`):

| File | Purpose |
|------|---------|
| `Layout.svelte` | Shell with nav |
| `StatusBadge.svelte` | Colored badge for daemon/episode status |
| `ConfirmDialog.svelte` | Modal confirmation dialog |
| `Toasts.svelte` | Toast notification container |
| `ErrorMessage.svelte` | Inline error display |
| `Input.svelte` | Styled input field |
| `Loading.svelte` | Loading spinner |

**Stores** (`src/lib/stores/`):

| File | Export | Purpose |
|------|--------|---------|
| `wsState.ts` | `wsState` | Svelte store for WebSocket daemon state (`status`, `lastCheck`, `hasError`) |
| `toasts.ts` | `toasts` | Toast queue store |
| `theme.ts` | `theme` | Dark/light theme |
| `locale.ts` | `locale` | i18n locale |

**API client** (`src/lib/api/client.ts`):

Exports typed fetch wrappers for every endpoint. Uses `window.location.origin` as base URL (works with reverse proxies). All errors surface via `toasts.add(message)`.

**WebSocket client** (`src/lib/websocket/client.ts`):

Connects to `/api/v1/ws`, updates `wsState` store on messages.
