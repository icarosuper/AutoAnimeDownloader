# Architecture

## Components

```
src/cmd/daemon/      → Daemon binary entry point (embeds frontend build, starts API)
src/cmd/cli/         → CLI binary for managing the daemon via REST API
src/internal/
  api/               → HTTP server, REST handlers, WebSocket, middleware, Swagger docs
  daemon/            → Verification loop: Anilist → Nyaa → embedded torrent client → track episodes
  files/             → Config, episode tracking (JSON files), and library hardlinking (Librarian)
  anilist/           → GraphQL client for Anilist API
  nyaa/              → HTML scraper for Nyaa torrent site
  torrents/          → Embedded BitTorrent client (github.com/cenkalti/rain/v2) behind a TorrentBackend interface
  frontend/          → Svelte 5 + Vite + Tailwind 3 + daisyUI 4 web UI (compiled to Go embed)
                       (o par de versões é obrigatório — ver decisão 33)
  notifications/     → Webhook template interpolation and HTTP firing. Called by daemon on NewEpisode/DownloadFailed; by job queue on DownloadCompleted.
  logger/            → zerolog-based structured logger (console + rotating file)
  tray/              → System tray icon (fyne/systray)
  version/           → Build-time version injection via ldflags
src/tests/
  unit/              → Unit tests with mocks
  integration/       → Docker-based end-to-end tests
  mocks/             → Mock servers for Anilist and Nyaa
```

The daemon ships as a single self-contained binary — the BitTorrent client is embedded (`github.com/cenkalti/rain/v2`), so there is no external qBittorrent to install, configure, or connect to.

## Key Data Flow

1. **Verification loop** (periodic, configurable interval):
   - Fetch watch list from Anilist GraphQL API
   - Per anime: scrape Nyaa for matching torrents (filter by resolution/fansub)
   - Add new episodes to the embedded torrent client (`TorrentBackend.Add`)
   - Record downloaded episodes in `episodes.json` — skip re-downloads
   - Torrents download to the derived download path (`Config.DownloadPath()`, `<completed_anime_path>/.torrents`) and keep **seeding** there; on completion an `organize` job hardlinks the video files into `completed_anime_path` (the Jellyfin library)

2. **Frontend embedding**: `bun run build` → `src/internal/frontend/dist/`, Go embeds via `//go:embed dist` in API server. Daemon serves SPA at `/`, proxies `/api/` to REST handlers.

3. **Real-time updates**: WebSocket at `/api/v1/ws` broadcasts daemon state changes. Svelte frontend subscribes for live status.

## Configuration & State Files

| File | Location (Linux) | Purpose |
|------|-----------------|---------|
| `config.json` | `~/.autoAnimeDownloader/` | User settings (Anilist usernames, paths, intervals) |
| `downloaded_episodes` | `~/.autoAnimeDownloader/` | Tracks downloaded episodes (JSONL, no extension) |
| `blocked_episodes` | `~/.autoAnimeDownloader/` | Episodes to skip (JSON array of `{anime_id, episode}`, no extension). O formato antigo (array de ids de nó da AniList) é descartado com aviso ao ser lido — ver decisions.md #52 |
| `anime_settings` | `~/.autoAnimeDownloader/` | Per-anime settings keyed by AniList **media** ID (JSON map, no extension) |
| `standalone_animes` | `~/.autoAnimeDownloader/` | Media IDs tracked **without** being in any AniList list (JSON array of IDs, no extension) |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |
| `pending_jobs.json` | `~/.autoAnimeDownloader/` | Persisted job queue (`organize` jobs) |
| `session.db` | `~/.autoAnimeDownloader/` | rain resume database (bbolt) — piece bitfields, kept **outside** the download path so it survives a library path change |
| `queue.json` | `~/.autoAnimeDownloader/` | Download queue state: `{"order": [...], "paused": [...]}`. Lives next to the resume database because it is torrent-client state, not user config. Missing/corrupted = rebuilt from `AddedAt`, never fatal — see decisions.md #41 |
| `download_root.id` | `~/.autoAnimeDownloader/` | Id of the download folder the session is bound to. Its twin, `.aad_root`, lives **inside** the download folder; the pair is how a moved/trashed/replaced folder is detected — see decisions.md #34 |

Windows uses `%APPDATA%\.autoAnimeDownloader\` for **all** the config/state files above (note the leading dot — same folder name as on Linux). See `configsFolder` in `files/filemanager.go` and `getJobsFilePath` / `getSessionDBPath` / `getPIDFilePath` in `cmd/daemon/main.go`. There is no dotless `%APPDATA%\AutoAnimeDownloader\` variant.

## On-Disk Layout

- **Download / seeding:** torrents live at `<Config.DownloadPath()>/<torrent-id>/...`, i.e. `<completed_anime_path>/.torrents/<torrent-id>/...` (rain's `DataDir` with `DataDirIncludesTorrentID`). Files are **never renamed here** — renaming would break seeding. Torrents keep seeding after completion. The download directory is **derived**, not user-configured — see decisions.md #31.
- **Library (Jellyfin):** when a torrent completes, its video files are **hardlinked** into `<completed_anime_path>/<AnimeName>/` — one folder **per AniList entry**, season/cour marker kept (`sanitizeName`, decisions.md #45), plus a `tvshow.nfo` carrying the AniList id. With `RenameFilesForJellyfin`, every file gets the Jellyfin name `"Anime Name - E05.mkv"` — a single episode from its record, a batch file from the episode number parsed out of its own filename (`nyaa.ExtractEpisodeNumber`); files with no readable number (NCOP/NCED, extras, movies) and two files of one pack landing on the same number keep the raw filename. The hardlink shares bytes with the seeded copy, so no space is duplicated.
- **Same volume, by construction:** the download directory lives inside `completed_anime_path`, so the old cross-filesystem failure mode is now structurally impossible. `Librarian.ProbePath(completedPath)` still validates that the filesystem supports hardlinks at all (exFAT/FAT32/some SMB shares don't) — it runs on config save and on every verification pass (decisions.md #26).
- **Deletion** frees space by removing **both** links: the library hardlink (`Librarian.RemoveFromLibrary`) and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). A batch torrent shared by multiple episodes is only removed once **all** its episodes are deleted **and** the pack covers no episode without a record (`canRemoveTorrent`, decisions.md #74).
- **No `save_path` migration.** `daemon.MigrateSavePath` used to move a legacy `save_path`'s torrent folders into the derived download path at boot and at the top of every pass; it was removed along with the `Config.SavePath` field. An old `config.json` still carrying the key loads (JSON ignores unknown fields) but its data is **not** moved — moving it is a manual pre-upgrade step. See decisions.md #31.

## API

Swagger UI at `http://localhost:8091/swagger/` when daemon running.

Key endpoints:

| Method | Endpoint | Handler func | File |
|--------|----------|-------------|------|
| `GET` | `/api/v1/status` | `handleStatus` | `endpoint_status.go` — `StatusResponse` carries `disk_total`, `disk_free` and `disk_low` (free below `min_free_disk_percent`, i.e. the daemon stopped adding torrents; the threshold lives server-side only), plus `anilist` (`anilist.Health`: `state` = `ok`/`rate_limited`/`outage`/`app_bug`, the raw `message`, and `retry_at` from a 429's `Retry-After`). The AniList state is written by **any** call — daemon pass or frontend poll — since all of them funnel through `sendAnilistRequest`; see `decisions.md` #65/#66 |
| `GET` | `/api/v1/last-check` | `handleLastCheck` | `endpoint_last_check.go` — o relatório do último passe automático: `problems` (o que devia ter baixado e não baixou) e `limits` (a config funcionando como configurada), um `Issue` por par (anime, código), ordenado por `anime_name`. `pass_error` é `State.GetLastCheckError()`, acompanhado de `pass_error_code` (a causa classificada — `anilist`/`setup`/`library`/`torrent_backend`/`config`/`storage`/`unknown`, ver `daemon/passerror.go`), e quando ele existe as duas listas estão vazias (`SetLastCheckError` limpa o relatório). O frontend monta a frase a partir do **código**; o texto cru fica recolhido num `<details>`. Só memória: um passe limpo devolve listas vazias e um `finished_at` zero significa que o daemon ainda não completou um passe. Download manual fica fora — aquele caminho já devolve o erro na própria resposta HTTP |
| `GET/PUT` | `/api/v1/config` | `handleConfig` | `endpoint_config.go` |
| `GET` | `/api/v1/config/priorities/defaults` | `handlePriorityDefaults` | `endpoint_priorities.go` |
| `GET` | `/api/v1/animes` | `handleAnimes` | `endpoint_animes.go` — `AnimeInfo.is_standalone` marks animes tracked via `standalone_animes`; `next_airing_at` (unix seconds, omitted when there's no scheduled episode) feeds the "next episode in X" chip; `alt_names` carries the media's other titles (romaji/english/native/synonyms, minus `name`) so the dashboard search matches whatever title the user knows — filtering is client-side, in `lib/utils/status.ts` |
| `GET` | `/api/v1/anilist/search?q=<term>&include_unreleased=<bool>` | `handleAniListSearch` | `endpoint_anilist_search.go` — 20 results with a `block_reason` per result (`""` = addable); a term under 3 chars returns an empty list, not a 400. `include_unreleased` defaults to false and hides `NOT_YET_RELEASED` **server-side**; a non-boolean value is a 400 `INVALID_QUERY_PARAM` (via `parseBoolQueryParam`, shared with `/torrents`) |
| `POST` | `/api/v1/standalone-animes` | `handleStandaloneAnimeAdd` | `endpoint_standalone_animes.go` — body `{"media_id": 21}`, answers `{"added": N}`; 409 `LIBRARY_NOT_CONFIGURED` / `ALREADY_BLACKLISTED` / `ALREADY_STANDALONE` / `ALREADY_TRACKED` / `ALREADY_DOWNLOADED` |
| `DELETE` | `/api/v1/standalone-animes/{id}?delete_episodes=<bool>` | `handleStandaloneAnimeRemove` | `endpoint_standalone_animes.go` — without `delete_episodes` the episodes stay and are marked `ManuallyManaged` |
| `GET/PUT` | `/api/v1/animes/{id}/settings` | `handleAnimeSettings` | `endpoint_anime_settings.go` |
| `GET` | `/api/v1/animes/{id}/episodes` | `handleAnimeEpisodes` | `endpoint_anime_episodes.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/download` | `handleDownloadEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/redownload` | `handleRedownloadEpisode` | `endpoint_episode_actions.go` |
| `DELETE` | `/api/v1/animes/{id}/episodes/{episodeNumber}` | `handleDeleteEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/release` | `handleReleaseEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/replace` | `handleReplaceEpisodeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/replace` | `handleReplaceAnimeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/check` | `handleCheck` | `endpoint_check.go` |
| `POST` | `/api/v1/daemon/start` | `handleDaemonStart` | `endpoint_daemon_start.go` |
| `POST` | `/api/v1/daemon/stop` | `handleDaemonStop` | `endpoint_daemon_stop.go` |
| `GET` | `/api/v1/logs` | `handleLogs` | `endpoint_logs.go` |
| `POST` | `/api/v1/notifications/webhooks/{name}/test` | `handleNotificationWebhookTest` | `endpoint_notifications.go` |
| `GET` | `/api/v1/torrents` | `handleTorrents` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/pause` | `handleTorrentPause` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/resume` | `handleTorrentResume` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/announce` | `handleTorrentAnnounce` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/prioritize` | `handleTorrentPrioritize` | `endpoint_torrents.go` |
| `GET` | `/api/v1/torrents/{hash}/files` | `handleTorrentFiles` | `endpoint_torrents.go` — files inside the torrent, in metadata order |
| `POST` | `/api/v1/torrents/prioritize` | `handleTorrentsPrioritize` | `endpoint_torrents.go` — batch, body `{"hashes":[...]}`, applied in the order received; unknown/completed hashes ignored |
| `DELETE` | `/api/v1/torrents/{hash}?keep_data=<bool>&block=<bool>` | `handleTorrentDelete` | `endpoint_torrents.go` |
| `WS` | `/api/v1/ws` | `handleWebSocket` | `websocket.go` |

## Version Injection

Version injected at build time via ldflags:
```bash
go build -ldflags "-X AutoAnimeDownloader/src/internal/version.Version=v1.2.0" ./src/cmd/daemon
```
Defaults to `"dev"` if not injected.

## Debug Mode

`cmd/daemon/main.go` accepts `--debug-anime <anilistId>`: runs `daemon.RunAnimeDebug` (see `daemon/debug.go`) and exits, skipping the PID file / API server / tray / loop entirely. See [Commands](commands.md) and [Troubleshooting Downloads](troubleshooting-downloads.md).

`RunAnimeDebug` mirrors `processAnimeEpisodes`: episode selection (`anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`), the pack search/selection (`partitionSearchResults` + `pickBatches`) that lifts the per-anime limit when it resolves, and the per-episode `searchSingleEpisode` fallback when no batch/movie/single magnet was found — so the debug pass reports the pipeline the daemon actually runs (see [decisions.md #58](decisions.md)).

`scripts/robustness-animes.txt` (curated media IDs, one per line) + `scripts/debug-batch.sh` (`make debug-batch`) run that pass over the whole list and write a triaged `.debug_batch/report.md`.

---

## Detailed File Map

### `src/internal/daemon/` — núcleo do passe

There is no `daemon.go`. The orchestrator is spread over `loop.go` (the goroutine loop),
`verification.go` (the pass itself), `episodes.go` (per-episode selection and removal),
`search.go` (Nyaa search + filters), `helpers.go` (config/disk guards), `manual_download.go`
(the API-driven downloads) and `standalone.go`. The table below is the package's key
functions; the parenthesised file name says where each one lives.

| Function | Purpose |
|----------|---------|

| `StartLoop(p)` | (`loop.go`) Creates goroutine loop, returns `LoopControl` (Cancel/UpdateInterval) |
| `AnimeVerification(ctx, fm, state, jobQueue, backend, librarian)` | Main check: fetches Anilist → Nyaa → embedded torrent client (`verification.go`). **Um passe por vez**: pega `verificationMu.TryLock()` e volta na hora se outro já está rodando (ver decisions.md #67) |
| `processAnimeEpisodes(...)` | Per-anime: monta a lista com `anilist.EpisodeList(anime, firstEpisodeToConsider(...))`, decide download/delete por episódio e executa a estratégia de busca |
| `firstEpisodeToConsider(anime, savedEpisodes)` | Onde a lista começa: `Progress + 1` (avulso tem progresso 0, logo começa no 1), recuando para o menor episódio já salvo — sem o recuo, um salvo abaixo do progresso não é "checado" e a poda o apagaria ignorando `watched_episodes_to_keep` |
| `checkEpisode(configs, maxEpisodes, ...)` | Returns `(shouldDownload, shouldDelete, skipCode)` per episode — `skipCode` é `IssueMaxEpisodesPerAnime` quando o limite por anime barrou o episódio e `""` em todo skip normal. `maxEpisodes` is the **effective** per-anime limit computed by the caller — unlimited (`len(episodes)+1`) once a pack was picked for this pass |
| `selectEpisodes(configs, maxEpisodes, anime, episodes, ...)` | Pure selection loop extracted from `processAnimeEpisodes`: per episode, `(shouldDownload, shouldDelete, skipCode)`. Runs twice in a pass when a pack covers the window — once with the real limit (produces the deletions), once with it lifted (so pack-covered records aren't pruned). O `episodeSelection` devolvido carrega também `downloaded`/`limitSkipped`, o par de que o `Issue` de `max_episodes_per_anime` é montado — vem do resultado FINAL, então na segunda passada (limite levantado) `limitSkipped` é zero, que é o certo |
| `effectiveMax(configs, episodes)` | `max_episodes_per_anime` if > 0, else `len(episodes)+1` (the "no ceiling" sentinel) |
| `windowEnd(configs, firstPending)` | Last episode number packs need to cover this pass: `firstPending + max_episodes_per_anime - 1`, or `math.MaxInt` when the ceiling is off |
| `partitionSearchResults(configs, results)` | Splits `ScrapNyaaForAnime`'s single mixed list into `(packs, singles, packStats)` by `IsBatch`/`Episode != nil`, each filtered by its own size ceiling (`max_batch_torrent_size_gb` / `max_episode_torrent_size_gb`) and `min_seeders`. `packStats` (`dropStats`) descreve o que o filtro fez com as linhas de PACK e só com elas — é o que distingue "a busca não devolveu pack nenhum" de "o teto cortou todos" |
| `pickBatches(results, firstPending, windowEnd)` | Minimum set of packs (already sorted/filtered) that covers `[firstPending, windowEnd]`. Batch eligibility comes from this — the search **result**, not anime metadata |
| `coveringBatch(results, episode)` | First pack in `results` whose range (`nyaa.ExtractBatchInfo`) contains `episode`; a pack with no parseable range (`EndEpisode == 0`) counts as covering everything |
| `assignBatches(animeTitle, episodes, batches)` | Gives each episode its own pack's magnet; truncates at the first uncovered episode (prefix cut — the schedule is ascending and picked packs are contiguous from `firstPending`) so nothing falls through to the single-episode fallback |
| `resolveMovie(configs, anime, animeTitle, episodes, customQuery, searcher)` | Movie path: on a hit, every pending episode (or a synthetic episode 1 if there were none) gets the movie's magnet |
| `magnetsByEpisode(singles, episodes)` | Single-episode fallback: maps each pending episode to the magnets of Nyaa rows whose parsed episode number matches |
| `filterBySize(results, maxGB)` | Devolve `([]nyaa.TorrentResult, int)` — o `int` é quantos descartou. Drops Nyaa results above the GiB ceiling, after priority sorting and preserving order (`search.go`). `maxGB <= 0` = off; `Size == 0` (parse failure) passes |
| `filterBySeeders(results, minSeeders)` | Devolve `([]nyaa.TorrentResult, int)`, mesmo contrato. Drops Nyaa results below the seeders floor, same contract (`search.go`). `minSeeders <= 0` = off; an unparseable seeders column counts as `0` and **is** dropped |
| `filterSearchResults(results, maxGB, minSeeders)` | The pair above, applied at all four call sites: movie, packs, single episodes from the anime search, and the single-episode fallback. Devolve `([]nyaa.TorrentResult, dropStats)`: é o que distingue "o Nyaa não devolveu nada" de "o filtro cortou tudo" |
| `checkDiskSpace(configs)` | `ErrInsufficientDiskSpace` when the library volume is below `min_free_disk_percent` (`helpers.go`). A `statfs` error does **not** block. Guards `attemptDownloadWithRetries` and `addAndPrioritize` — never the verification pass |
| `shouldSkipEpisode(...)` | Skip if: excluded list, already watched, not yet aired |
| `handleAlreadySavedEpisode(...)` | Re-download if missing from torrents, delete if over limit |
| `handleSavedEpisodes(...)` | Post-loop: save new, delete watched, delete torrent files |
| `attemptDownloadWithRetries(...)` | Tries up to `EpisodeRetryLimit` magnets, returns first hash. Returns `""` with **no** `Add` call and no retry when `checkDiskSpace` blocks |
| `searchNyaaForSingleEpisode(ep, titles, synonyms, relations, customQuery, totalEpisodes)` | Single ep search — extracts season/part from titles+synonyms, falls back to `ep+offset` (no part filter) if 0 results and PREQUEL has episode count. `totalEpisodes` (from `anilist.LastAiredEpisode`) only drives the zero-padded query variant |
| `searchNyaaForMovie(...)` | Movie search (priority 1) |
| `searchNyaaForAnime(titles, synonyms, episodes, customQuery)` | The one search behind pack + episode resolution (priority 2): wraps `nyaa.ScrapNyaaForAnime`, which returns packs and episodes in the **same** list — `partitionSearchResults` is what splits them |
| `ExtractAnimeSeasonPart(title, synonyms)` | Exported: reads english→romaji→synonyms, returns `(season, part *int)` — first non-nil wins independently |
| `ComputeEpisodeOffset(relations, part)` | Exported: returns PREQUEL episode count when `part >= 2` and the PREQUEL is `TV`/`TV_SHORT`; 0 otherwise (gates prevent spurious offsets on non-split seasons and on recap movies/OVAs) |
| `RemoveEpisodesWithLinks(fm, backend, librarian, keys []files.EpisodeKey) error` | Deletes episodes: removes library hardlinks + seeding torrents, applying the batch guard (`episodes.go`). Returns an error when the record could not be removed from the JSONL (load/delete failure); freeing disk space is best-effort and only logged |
| `RemoveTorrentWithEpisodes(fm, backend, librarian, hash, opts) error` | Deletes a torrent and every saved episode sharing its hash as one unit (a batch always leaves together) — used by `DELETE /torrents/{hash}`. `RemoveTorrentOptions{KeepData, Block}`: `Block` marks every episode in the group blocked before removing its record; an orphan hash (no saved episode matches) is removed directly via `backend.Remove` (`episodes.go`) |
| `reconcileLibrary(downloaded, saved, jobQueue)` | Startup/periodic reconciliation: enqueues an `organize` job for any completed torrent whose episode isn't yet in the library (`verification.go`) |
| `clearLibraryPathsAfterRootSwap(fileManager, completedPath)` | Runs when `Ensure` reports `RootSwapped`: wipes every `LibraryPaths` so the library is rebuilt at the configured path after the redownloads (`verification.go`) — the one exception to decisions.md #29, see #34 |
| `ManualDownloadEpisode(backend, animeId, episodeNumber, cfg, customQuery)` | Used by API for manual download — calls Anilist then Nyaa (`manual_download.go`). Resolves the anime via `resolveAnimeDetails`, which falls back to `anilist.GetMediaByID` when no account tracks it — that fallback is what makes the per-episode buttons work on a standalone anime (and at all when no AniList account is configured) |
| `ManualDownloadEpisodeWithMagnet(...)` | Used by API for replace-with-magnet per episode |
| `ManualDownloadAnimeWithMagnet(...)` | Used by API for replace-with-magnet for full anime batch |
| `searchAnilist(fm, configs, standaloneIDs)` | Builds the pass's anime universe: the union of the accounts' lists plus the standalone animes, appended **after** `DedupeByMedia` (`verification.go`) |
| `appendStandaloneAnimes(fm, merged, standaloneIDs)` | Drops the standalone record of any id the lists already cover (with a log), then appends the rest via `anilist.GetMediaByID`. No media-status filter — a standalone anime is tracked while `NOT_YET_RELEASED` too (`standalone.go`) |
| `DownloadStandaloneAnime(fm, backend, configs, mediaID) (int, error)` | `Ensure` + `processAnimeEpisodes` + `saveEpisodesToFile` for one anime, nothing else. **Must never call `handleSavedEpisodes`** — with a single anime's episodes in hand and `delete_watched_episodes` on, `identifyEpisodesNotInWatching` would wipe the rest of the library (`standalone.go`, decisions.md) |

**Download priority** (in `processAnimeEpisodes`):
1. Movie → `resolveMovie`/`searchNyaaForMovie` → `skipSubfolder=true`, epName = animeName
2. Packs resolved before the episode loop, covering the window from the first pending episode → `searchNyaaForAnime` + `partitionSearchResults` + `pickBatches`/`assignBatches` → `skipSubfolder=true`, filtered by `max_batch_torrent_size_gb`. Eligibility is decided by the filtered search **result** (size, seeders, covered range), not by anime metadata — see decisions.md
3. Single ep fallback, per still-uncovered episode → `searchNyaaForSingleEpisode`, filtered by `max_episode_torrent_size_gb`

All three also pass through the `min_seeders` floor (`filterSearchResults`).

`max_episodes_per_anime` is lifted (`selectEpisodes` re-run with `len(episodes)+1`) only once a pack was actually picked for the pass; if no pack covers the window, the original (limited) selection stands and the oldest episodes are what gets kept.

### `src/internal/daemon/webui.go`

| Symbol | Purpose |
|--------|---------|
| `WebUIURL(port, route)` | Builds the Web UI URL from what the process calls "the port" — which is really the `http.Server` **Addr** (`":8091"`). Strips the leading colon: interpolating the raw value produces `http://localhost::8091`, an invalid host, and the browser lands on an error page instead of the app. Single source for the verification pass, the tray and the first-boot open — the conversion used to be written out three times and the newest copy forgot the strip |
| `WaitForListener(addr, timeout)` | Dials until the port accepts. `apiServer.Start()` runs in a goroutine and the "API server started successfully" log fires **before** the socket is bound, so anything opening a browser right after races `ListenAndServe`; losing that race is `ERR_CONNECTION_REFUSED` |

### `src/internal/daemon/standalone.go`

Standalone animes (animes tracked without being in any AniList list). `appendStandaloneAnimes` and `DownloadStandaloneAnime` are listed in the package symbol table above; both are documented in [decisions.md #49](decisions.md).

### `src/internal/daemon/debug.go`

One-shot diagnostic for a single anime, driven by the `--debug-anime` flag on the daemon binary (see `cmd/daemon/main.go`). No torrent backend or episodes.json involved. Output goes to `.debug_<animeId>_<N>/` in the invoker's cwd, not `~/.autoAnimeDownloader`.

| Symbol | Purpose |
|--------|---------|
| `RunAnimeDebug(animeId, configs, fileManager)` | Fetches the anime, logs the raw AniList response, runs `checkEpisode` + the same pack search/selection path (`partitionSearchResults`/`pickBatches`) as production against live Nyaa, logs raw vs. matched results per episode. Returns a `*DebugSummary` |
| `DebugSummary` / `EpisodeDebugResult` structs | JSON-tagged summary written to `summary.json` — per episode, whether it would be searched and how many magnets were found |
| `NextDebugDir(baseDir, animeId)` | Returns the next unused `.debug_<animeId>_<N>` directory name inside `baseDir` (scans for existing ones, doesn't create it) |
| `WriteDebugSummary(dir, summary)` | Marshals `DebugSummary` to `<dir>/summary.json` |

### `src/internal/daemon/jobs.go`

Deferred job queue for async library organization. Decouples the hardlink-into-library + webhook step from the main verification loop by persisting jobs to disk and retrying them on a background ticker. The former poll-based `rename_file` / `move_to_completed` / `notify_on_complete` jobs were replaced by a single idempotent `organize` job.

**Lifecycle**: Created in `main.go` before `NewServer`, wired with the torrent backend + librarian via `SetOrchestration`, passed to `Server.JobQueue`, threaded into every `AnimeVerification` call via `StartLoopPayload.JobQueue`. Starts on daemon boot, stops on shutdown.

**Tick interval**: 5 seconds.

**Backoff**: `30s * 2^(attempts-1)`, capped at 10 minutes.

| Symbol | Purpose |
|--------|---------|
| `JobType` / `JobOrganize` | The single job type (`"organize"`) |
| `JobQueue` struct | Background processor; loads/saves `pending_jobs.json`; holds `backend` + `librarian` |
| `NewJobQueue(fm, jobsPath)` | Constructor — takes FileManager (for config) and file path |
| `JobQueue.SetOrchestration(backend, librarian)` | Injects the torrent backend + `files.Librarian` used by `JobOrganize` |
| `JobQueue.Start()` | Loads persisted jobs, starts background goroutine |
| `JobQueue.Stop()` | Signals goroutine to stop and waits |
| `JobQueue.EnqueueOrganize(hash)` | Schedule organizing a completed torrent into the library; no-op if one is already pending for the same hash; max 20 retries |
| `organizeTorrent(hash, backend, librarian, fm, configs)` | Package func executing the job: hardlinks completed video files into the library, writes back `LibraryPaths` (the "organized" marker), then fires the `DownloadCompleted` webhook exactly once. Idempotent across restarts |

**Job type**:

| Type | Payload | Trigger |
|------|---------|---------|
| `organize` | `hash` | Torrent completion event, or `reconcileLibrary` finding a completed-but-unorganized torrent |

**Persistence**: `~/.autoAnimeDownloader/pending_jobs.json` (Windows: `%APPDATA%\.autoAnimeDownloader\pending_jobs.json`). Written after every enqueue and after every tick that changes queue state. Jobs survive daemon restarts.

**Idempotency**: `organizeTorrent` treats an episode whose `LibraryPaths` is already set as done — no re-link, no re-fired webhook — so completion events and reconciliation passes can both enqueue safely.

### `src/internal/daemon/migration.go`

Migrations that run at boot and at the top of the verification pass. The `save_path` migration that used to live here was removed with the field — see decisions.md #31.

| Symbol | Purpose |
|--------|---------|
| `isAncestorOrEqual(dir, child)` | Guards against renaming a directory into itself — relevant for Docker's default layout, where the library nested inside the old save path |

Called from `cmd/daemon/main.go` (boot, before the verification loop starts) and from the top of `AnimeVerification` (`verification.go`) on every pass, before the hardlink probe. `verification.go` reloads the config immediately after calling it, since migration persists a changed config that the rest of the pass must see.

### `src/internal/daemon/report.go`

- `Issue` / `CheckReport` — os tipos do relatório da última verificação, serializados direto pelo endpoint `/last-check`. Campos de detalhe achatados com `omitempty` (nunca um `map[string]any`: não gera Swagger nem tipo TS utilizável).
- Códigos: `IssueAllAboveSizeLimit`, `IssueNoSeeders`, `IssueNoTorrentFound`, `IssueDiskFull`, `IssueTorrentRejected` (problemas) e `IssueMaxEpisodesPerAnime` (limite). `BatchSkippedNoResult` / `BatchSkippedAboveSizeLimit` / `BatchSkippedNoCoverage` são detalhe do limite, não códigos.
- `searchIssue(...)` — a cascata de precedência dos três problemas de busca (ver decisions.md #60).
- `aggregateIssues(raw)` — um `Issue` por par (anime, código), separado em problemas e limites, ordenado por `AnimeName`.

### `src/internal/daemon/state.go`

Thread-safe daemon state. Key types:

| Symbol | Purpose |
|--------|---------|
| `Status` (string enum) | `stopped` / `running` / `checking` |
| `State` struct | Holds `status`, `lastCheck`, `lastCheckError`, `lastCheckReport`, notifier |
| `SetLastCheckReport(CheckReport)` / `GetLastCheckReport() CheckReport` | O relatório do último passe, em memória. `GetLastCheckReport` devolve **valor**, para o handler poder preencher `pass_error` sem escrever no objeto compartilhado |
| `SetLastCheckError(err)` | **Limpa** `lastCheckReport`. Consequência: `SetLastCheckReport` tem de ser chamado depois do `SetLastCheckError(nil)` do fim do passe (ver decisions.md #61) |
| `StateNotifier` interface | `NotifyStateChange(status, lastCheck, hasError)` — WebSocket subscribes |
| `State.GetAll()` | Returns `(status, lastCheck, hasError)` atomically |
| `State.SetStatus(s)` | Sets status and fires notifier if changed |

### `src/internal/daemon/passerror.go`

Códigos de causa de um passe abortado. O backend manda **código**, o frontend monta a frase
(mesma fronteira que `lib/domain/checkIssue.ts`) — antes disso o banner da tela de Status
despejava o `error.Error()` cru, às vezes um JSON de resposta da AniList.

| Symbol | Purpose |
|--------|---------|
| `PassErrConfig` / `PassErrSetup` / `PassErrLibrary` / `PassErrTorrent` / `PassErrAnilist` / `PassErrStorage` / `PassErrUnknown` | Os códigos exportados, consumidos pelo `GET /last-check` e pelo frontend |
| `errCause*` (unexported) | Sentinelas embrulhadas no erro no sítio do aborto — o código viaja **dentro** do erro, não num parâmetro novo de `SetLastCheckError` |
| `passErrorCode(err)` | Traduz o erro embrulhado no código. Um sítio novo que esqueça de embrulhar cai em `PassErrUnknown`, que tem frase própria — degrada, não quebra |

### `src/internal/files/filemanager.go`

All persistence. Key types:

| Symbol | Purpose |
|--------|---------|
| `Config` struct | All user settings — maps to `config.json`. There is no `save_path`: the download directory is derived by `Config.DownloadPath()` |
| `Config.DownloadPath()` | Derives the download/seeding directory: `filepath.Join(CompletedAnimePath, ".torrents")` (`downloadDirName` const). Computed on every call, not stored |
| `EpisodeKey` struct | `AnimeID`, `Episode` — **a identidade de um episódio** em todo o app (arquivo de episódios, bloqueados, rotas da API). `EpisodeStruct.Key()` a produz |
| `EpisodeStruct` struct | `AnimeID`, `EpisodeHash`, `EpisodeName`, `DownloadDate`, `ManuallyManaged`, `EpisodeNumber int`, `IsBatch bool`, `BatchStart`/`BatchEnd int` (faixa real do pack: lida do nome do torrent, ou 1..total quando o pack não traz faixa e por isso vale como completo; 0 = desconhecida), `LibraryPaths []string` (hardlink paths in the library, set once organized) |
| `FileManagerInterface` | Interface used by daemon + API — mock in tests |
| `FileManager.LoadConfigs()` | Reads `config.json`; creates with defaults if missing |
| `FileManager.LoadSavedEpisodes()` | Reads `episodes.json` (JSONL), migrates old format |
| `FileManager.SaveEpisodesToFile(eps)` | Appends only new episodes (deduped by ID) — **silently discards updates to existing IDs**; see decision 27 |
| `FileManager.UpsertEpisodes(eps)` | Inserts or **fully replaces** episodes by ID — used to write back `LibraryPaths` after a torrent is organized |
| `FileManager.DeleteEpisodesFromFile(keys)` | Removes episodes by `EpisodeKey` from JSONL |
| `FileManager.BlockEpisode(key)` | Appends `files.EpisodeKey` to `blocked_episodes` |
| `FileManager.UnblockEpisode(key)` | Removes `files.EpisodeKey` from `blocked_episodes` |
| `FileManager.UnmanageEpisode(id)` | Sets `ManuallyManaged=false` for episode |
| `FileManager.LoadAnimeSettings(animeID)` | Returns `*AnimeSettings` for one anime (empty struct if not set) |
| `FileManager.SaveAnimeSettings(animeID, settings)` | Persists `AnimeSettings` for one anime to `anime_settings` |
| `FileManager.LoadStandaloneAnimes()` | Media IDs from `standalone_animes` (missing file = empty list) |
| `FileManager.AddStandaloneAnime(id)` | Appends ID to `standalone_animes` (idempotent) |
| `FileManager.RemoveStandaloneAnime(id)` | Removes ID from `standalone_animes` (absent ID is not an error) |
| `FileManager.LoadAllAnimeSettings()` | Returns full `map[int]AnimeSettings` — used by daemon loop |
| `FileManager.DeleteEmptyFolders(completedAnimeSaveFolder)` | Removes empty dirs under the single `completed_anime_path` tree (single argument now that download and library share a root); skips the `.torrents` download folder itself |

`AnimeSettings` struct fields: `CustomSearchQuery string` — overrides Nyaa search query for this anime.

Config defaults: `CheckInterval=10`, `MaxEpisodesPerAnime=12`, `EpisodeRetryLimit=5`. (There is no `qbittorrent_url` field — the torrent client is embedded.)

| `DiskSpace(path)` | Cross-platform total/free bytes for the filesystem containing `path` (`diskspace_unix.go` for Linux/Darwin via `syscall.Statfs`, `diskspace_windows.go` via `golang.org/x/sys/windows.GetDiskFreeSpaceEx`) |

### `src/internal/files/standalone.go`

`LoadStandaloneAnimes` / `AddStandaloneAnime` / `RemoveStandaloneAnime` on `*FileManager`, over `standalone_animes` (JSON array of media ids). Built on the `loadIntListLocked` / `saveIntListLocked` helpers in `filemanager.go`. `blocked_episodes` NÃO usa mais esse par: ele guarda `EpisodeKey` (objeto), não int.

### `src/internal/files/filesystem.go`

`FileSystem` interface + `OSFileSystem` implementation. Used for testability — tests inject `MockFileSystem`. The interface includes a `Link(oldname, newname)` method (`os.Link`) used by the `Librarian` for hardlinking into the library.

### `src/internal/files/mkv.go`

`VideoCodec(path)` — the video codec read from the file's **own header**, not from its name.

`nyaa.extractCodec` reads the codec off the release *name*, and a large share of uploads never write it (Erai-raws does not; many BD rips do not). Matroska (`.mkv`) is an EBML document (RFC 8794) — a tree of `ID + size + payload` elements with variable-width sizes, i.e. binary XML — and the codec sits at a short, fixed path that comes **before** the Clusters (the video data):

```
Segment → Tracks → TrackEntry → { TrackType (1 = video), CodecID }
```

So the read costs a few KB and a couple of seeks, never the whole file. It walks siblings by seeking past each declared size, gives up at the first `Cluster`, and bounds the `Tracks` scan with an `io.LimitReader` so it cannot run off the end into cluster data. `TrackType` is checked because an anime `.mkv` carries video, audio and several subtitle tracks — taking the first `TrackEntry` blind would report the audio codec.

`CodecID`s are mapped to the **same vocabulary `nyaa.extractCodec` uses** (`HEVC`, `H.264`, `AV1`) so the panel and the priorities screen do not name codecs differently; anything off the map falls back to the raw `CodecID` minus its `V_` prefix rather than guessing a prettier label. Returns `""` — never an error — for every unanswerable case: not `.mkv`, unreadable, not EBML, or a header not on disk yet. Only Matroska: MP4 has an analogous box structure but would need a second parser, and anime is `.mkv` in practice.

### `src/internal/files/librarian.go`

Hardlinks completed torrent files into the Jellyfin library. The seeded copy stays in place; the library holds a second name pointing at the same bytes.

| Symbol | Purpose |
|--------|---------|
| `Librarian` interface | `Organize`, `RemoveFromLibrary`, `ProbePath` |
| `NewLibrarian(fs)` | Constructor — `link` defaults to `fs.Link`, shared by `Organize` and `ProbePath` so they never disagree |
| `OrganizeRequest` struct | `TorrentDataDir`, `AnimeName`, `AnimeID` (AniList media id, for the `.nfo`), `CompletedPath`, `EpisodeNumber *int`, `IsBatch`, `RenameJellyfin`, `TotalEpisodes` (total da entrada AniList, so para `packEpisodeOffset`) |
| `Librarian.Organize(req)` | Hardlinks video files into `<CompletedPath>/<AnimeName>/`; Jellyfin name when `RenameJellyfin`: from `EpisodeNumber` for a single episode (exactly one video file), from each file's own name via `nyaa.ExtractEpisodeNumber` for a batch, menos `packEpisodeOffset` (pack de season >= 2 numerado continuamente → numeracao da entrada, decisions.md #68). Raw filename without the flag, without a readable number, or on a name collision inside the pack; dois arquivos do pack com o mesmo basename (subpasta por season, achatada) usam o caminho relativo em vez de sobrescrever um ao outro. Idempotent — returns paths of created/existing links. Also writes `tvshow.nfo` (see below) |
| `organizer.writeShowNFO` | Writes `<destDir>/tvshow.nfo` with `<uniqueid type="AniList">` after the links succeed, so the Jellyfin AniList plugin matches by id instead of by folder name. Skipped when `AnimeID == 0` or the file already exists; write failures only log (the hardlinks are what matter) |
| `organizer.BackfillShowNFOs(episodes)` | Writes the `.nfo` for library folders that predate the feature (`Organize` never re-runs for already-organized episodes). Folder comes from `LibraryPaths`, one per anime, missing folders skipped. Called from `main.go` at boot, **only when `MigrateAnimeIDsToMedia` succeeded** — not on the `Librarian` interface, `main.go` holds the concrete `*organizer` |
| `Librarian.RemoveFromLibrary(path)` | Deletes one library hardlink; missing file is not an error |
| `Librarian.ProbePath(completedPath)` | Single-path validation (replaced the two-path `ProbePaths`): writes a probe file under `<completedPath>/.torrents` and hardlinks it in place; returns an error if the filesystem doesn't support hardlinks at all (exFAT/FAT32/some SMB shares). Called on config save and on every verification pass (decisions.md #26, #31) |

### `src/internal/files/crossdevice_unix.go` / `crossdevice_windows.go`

`isCrossDevice(err) bool` — platform-specific detection of the "cross-device link" error (`EXDEV` on Unix), used by `Librarian` to turn a failed hardlink into a clear same-volume error message.

### `src/internal/files/parser.go`

`ParseEpisodes(string)` and `SerializeEpisodes([]EpisodeStruct)` — JSONL (um objeto JSON por linha). O formato de texto legado (`id:hash:name`) foi removido junto com `EpisodeID`: sem anime id nem número de episódio, um registro daqueles não tem chave (decisions.md #52).

### `src/internal/api/server.go`

| Symbol | Purpose |
|--------|---------|
| `Server` struct | Wraps `http.Server` + `State`, `FileManager`, `WSManager`, `currentLoopControl`, `JobQueue`, `Torrents` (`torrents.TorrentBackend`), `Librarian` (`files.Librarian`) |
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
- `nextAiringAt(next *anilist.AiringNode)` — nil-safe read of AniList's `nextAiringEpisode` into `AnimeInfo.NextAiringAt`. Called from all three write paths (merge of a new entry, merge onto an existing one, orphan refresh). Exposed as `next_airing_at` (unix **seconds**, `omitempty`), and it's the **timestamp** on purpose rather than AniList's `timeUntilAiring`: `GET /animes` is cached 60s and the frontend counts down on its own, so a pre-computed countdown would freeze with the cached response. `nextAiringEpisode` is nil both for finished anime and during a hiatus between seasons
- `handleAnimes` — groups saved episodes by anime name, merges current AniList watching list (so animes with 0 eps still show), then calls `refreshOrphanAnimes` for already-downloaded animes not covered by that merge
- `fetchAniListEntries` — one account's AniList entries, filtered by both `DownloadStatuses` (server-side) and `DownloadMediaStatuses` (client-side), with customLists overlaid; returns `nil` (not an empty slice) on fetch failure
- `mergeAniListAnimes` — adds the (already deduped) AniList entries not yet in episodes.json into animeMap; never removes an existing entry
- `refreshOrphanAnimes` — for animeMap entries with a known AnimeID that no account's fetch covered (current status fell outside the allowed sets), re-fetches cover/progress/blacklist via `resolveMediaList` per anime, bounded to `maxConcurrentOrphanRefresh` (5) in flight; a failed refresh just logs a warning and leaves the anime as-is — it's never removed
- `computeAnimeFields` — shared field-derivation (name, total/released episodes, cover, blacklist) used by both the batch merge loop and the single-anime orphan refresh
- `countPendingEpisodes` — preenche `AnimeInfo.EpisodesPending`: episódios lançados **acima do progresso da AniList** que não estão em disco. Conta por número de episódio (não `released - downloaded`) porque assistidos podem continuar salvos (`watched_episodes_to_keep`); o daemon nunca baixa episódio ≤ progresso, então ele não é atraso. É o número do chip "Atrasado" no frontend (`lib/domain/animeState.ts`)
- `extractAnimeName(episodeName)` — strips episode number suffix from torrent name to get anime name
- Standalone animes are appended **after** `DedupeByMedia` (`appendStandaloneEntries`), so an anime that is also in a list keeps the real entry's progress; `AnimeInfo.IsStandalone` is stamped from the same set at the end

### `src/internal/api/endpoint_anime_episodes.go`

- `AnimeEpisodeInfo` struct — per-episode detail (aired, watched, downloaded, blocked, manually managed). **Não carrega nome de episódio**: o AniList não expõe título por episódio (`AiringNode` só tem número/horário), e o `EpisodeName` do registro salvo é o rótulo de log do download (`"<anime> - Episode <n>"`, ou o nome do pack) — a tela mostra sempre o título localizado a partir de `episode_number`. `EpisodeHash` (`episode_hash`, `omitempty`) is the info hash of the torrent that downloaded it, if any — the frontend uses it to join the episode against `GET /torrents` (see `torrentsByEpisode.ts`) to show live progress inline before the episode finishes. `BatchStart`/`BatchEnd` (`batch_start`/`batch_end`, `omitempty`) carry the pack's real episode range: the screen cannot derive it from the list, because watched episodes never get a saved record and the min/max of the records sharing a hash shrinks the pack (a 01-11 pack downloaded with 5 episodes watched rendered as "6-11"). A pack with no range in its name ("(Season 1+OVA) [Batch]") was picked as a *complete* pack — that is why `coveringBatch` lets it cover any episode — so it is recorded as 1..total. 0/omitted means unknown (old record, or the AniList total is unknown too) and the row falls back to the min/max.
- `AnimeDetailResponse` struct — `{animeId, totalEpisodes, progress, status, episodes[]}` — `animeId` is the AniList **media** ID: the primary key everywhere else *and* the `anilist.co/anime/{id}` link component (there is no separate `anilistId` field — see [decisions.md #43](decisions.md))
- `handleAnimeEpisodes` — fetches the anime via `resolveMediaList(id, usernames, standaloneSet)` + saved episodes + blocked list → merges; 404s when no configured account tracks the media **and** it is not a standalone anime

### `src/internal/api/standalone.go` / `standalone_guard.go` / `endpoint_standalone_animes.go` / `endpoint_anilist_search.go`

Standalone animes — animes tracked without being in any AniList list (`standalone_animes`). "Avulso" in the UI; **never** call the concept `manual`, which already means "the user touched this episode by hand" (`ManuallyManaged`, `ManualDownloadEpisode`).

- `loadStandaloneSet(fm)` — the file as a `map[int]bool`; a read failure degrades to "no standalone animes" instead of failing the request
- `resolveMediaList(id, usernames, standalone)` — `anilist.GetAnimeInfo` with a fallback to `anilist.GetMediaByID` **only** when the id is standalone. Without it a standalone anime 404s on the detail screen; without the set check every AniList id would answer on `/animes/{id}/*`
- `appendStandaloneEntries(entries, standalone, covered)` — merges standalone animes into a list of AniList entries, skipping ids the lists already cover
- `standaloneGuard` / `blockReason(mediaID, totalEpisodes)` — one blocking rule, two consumers (the `POST` and the search handler), so "the front won't let you click" and "the back returns an error" agree by construction. Precedence: `blacklist` > `standalone` > `tracked` > `downloaded`. `downloaded` only blocks with a **known** total (`totalEpisodes > 0 && downloaded >= totalEpisodes`). `tracked` comes from `fetchAniListEntries` — the snapshot the daemon **processes**, not "an entry exists on AniList" (see decisions.md)
- `handleAniListSearch` — `AniListSearchResult` carries `block_reason` (one field, not four booleans)
- `handleStandaloneAnimeAdd` — library check → `GetMediaByID` (404) → `blockReason` (409) → `AddStandaloneAnime` → `daemon.DownloadStandaloneAnime`. Synchronous, like the per-episode download endpoint. `{"added": 0}` is a normal answer, not an error
- `handleStandaloneAnimeRemove` — `RemoveStandaloneAnime`, then either `daemon.RemoveEpisodesWithLinks` (`delete_episodes=true`) or `UpsertEpisodes` marking the anime's episodes `ManuallyManaged=true`

### `src/internal/api/endpoint_episode_actions.go`

All episode mutation endpoints. Each shares same pattern:
1. Parse path params
2. Load config + saved episodes
3. Use the shared `server.Torrents` (`TorrentBackend`) and `server.Librarian` — no per-request client construction
4. Call `daemon.ManualDownload*`, or `daemon.RemoveEpisodesWithLinks(fm, backend, librarian, keys)` for deletes (removes library hardlinks + seeding torrents). It returns an `error`: handlers must answer 500 via `JSONInternalError` and, on redownload/replace, **abort before adding the new torrent** — otherwise the new torrent is untracked while the stale record survives
5. Update `FileManager` (save/delete/block/unblock)

Actions: `download`, `redownload`, `delete` (+ block), `release` (unblock + unmanage), `replace` (per episode magnet), `replaceAnime` (full anime magnet).

### `src/internal/api/endpoint_torrents.go`

- `TorrentFileResponse` struct — one file inside a torrent: `path` (the RAW relative path from the metadata — the point of the panel is reading fansub/resolution/codec off it, so it is never rewritten into "Ep NN"), `size`, `bytes_completed` (**null** when unknown: a paused torrent frees rain's pieces, so per-file progress is unavailable — 0 would be the lie "nothing downloaded") `codec` (see below) and `episode`, from `nyaa.ExtractEpisodeNumber(path.Base(path))`, the same cascade the Librarian uses to rename a pack's files, so episode extraction has one truth and not a second one in TypeScript. **`path.Base`, never the whole path**: the pack's FOLDER usually carries the range in its name (`… - 00 ~ 12 [1080p]/…`), and running the cascade over that returns the wrong number or none at all — measured on the Erai-raws Mushoku Tensei II pack, where the full path yields `nil` for every file while the file name yields the right episode. `codec` is read from the **file's own header** via `files.VideoCodec` (`filepath.Join(info.DataDir, f.Path)`, same join the Librarian uses) and is **omitted** unless the file is `.mkv` *and* complete: rain does not download sequentially, so a file at 90% may be missing exactly the first pieces, where the Matroska header lives. Order is the metadata's, never re-sorted: it is the order the release was built in. `handleTorrentFiles` is 404 for a hash absent from the session (same `server.Torrents.Get` check as `torrentAction`) and 200 with `data: []` while the metadata has not arrived.
- `TorrentResponse` struct — one row per torrent: live progress (`bytes_completed/total/uploaded`, `progress` 0..1, `download_speed`, `upload_speed`, `peers_total`, `eta_seconds`, `seeded_for_seconds`), a piece-derived `completed` flag, joined with the anime/episode that shares its info hash. A **batch** torrent covers several episodes but is still one torrent, so it appears **once**, with `episode_number: null` and `is_batch: true`. `handleTorrents` returns an **empty list, not an error**, when no session exists yet (`completed_anime_path` not configured, so the derived download path can't be computed) — `TorrentBackend.List()` returns `nil` in that case and that is treated as the normal empty state. `completed` comes straight from `TorrentInfo.Completed` (piece-derived, see decisions.md #30) rather than `Status == "seeding"`, because pausing takes a finished torrent out of `Seeding` — the list sort keys on `completed` for the same reason.
- `handleTorrents` — lists `server.Torrents.List()`, joins each entry against `episodes.json` by `Hash == EpisodeHash` (best-effort: a `LoadSavedEpisodes` failure logs a warning and falls back to torrents with no anime metadata rather than failing the request), sorts unfinished torrents first (keyed on `Completed`, not the status slug) then alphabetically.
- `buildTorrentResponse(t, eps)` — the join + batch-collapse logic described above. `Progress` normally comes from `BytesCompleted/BytesTotal`, but falls back to the piece ratio (`PiecesHave/PiecesTotal`) whenever `BytesCompleted` reads 0 with a nonzero total — pausing frees rain's piece data and zeroes `Bytes.Completed` while the bitfield backing `PiecesHave/PiecesTotal` survives, so without the fallback a paused torrent's progress bar would collapse to 0%.
- `torrentAction(server, action)` — shared shape for `pause`/`resume`/`announce`: POST only, hash from the path, 404 when `Get(hash)` misses, backend call last.
- `handleTorrentPause` / `handleTorrentResume` / `handleTorrentAnnounce` — thin wrappers over `torrentAction` calling `Torrents.Pause/Resume/Announce`.
- `parseBoolQueryParam(r, name)` — reads a boolean query param, defaulting to `false` when absent; an unparseable value becomes a 400 (`INVALID_QUERY_PARAM`).
- `handleTorrentDelete` — `DELETE /torrents/{hash}?keep_data=<bool>&block=<bool>`. Registered on the same `/api/v1/torrents/{hash}` mux pattern as pause/resume/announce (a Go 1.22+ pattern with no method prefix matches every verb), so the method check turns non-DELETE requests into a 405. 404 is decided the same way as `torrentAction` — only by `server.Torrents.Get(hash)` — so an orphaned saved-episode record with no matching live torrent is left alone; cleaning that up is `DELETE /animes/{id}/episodes/{episodeNumber}`'s job, not this route's. Delegates to `daemon.RemoveTorrentWithEpisodes` with `daemon.RemoveTorrentOptions{KeepData: keep_data, Block: block}`. See decisions.md for the default (delete + block) and why `keep_data` can't split the library copy from the seeding copy.

### `src/internal/api/websocket.go`

- `WebSocketManager` — manages connected clients, broadcasts state changes
- `WebSocketManager.HandleWebSocket(w, r)` — upgrades connection, adds client, sends current state immediately
- `WebSocketManager.NotifyStateChange(status, lastCheck, hasError)` — implements `StateNotifier`, broadcasts to all clients
- `WebSocketManager.SetStateGetter(fn)` — wires initial state on new connection

### `src/internal/api/client.go`

HTTP client **for** the daemon's own REST API, used by the CLI binary (`cmd/cli`) — not by the
daemon itself. `NewClient(baseURL)` plus `doRequest`/`parseResponse` and one method per command:
`GetStatus`, `GetConfig`, `UpdateConfig`, `GetAnimes`, `GetEpisodes`, `TriggerCheck`,
`StartLoop`, `StopLoop`.

### `src/internal/anilist/anilist.go`

| Symbol | Purpose |
|--------|---------|
| `AniListResponse` | Response for `GetAllCurrentAnime` — `Data.Page.MediaList[]` |
| `MediaList` struct | `Id`, `Status`, `Progress`, `CustomLists`, `Media` |
| `Media` struct | `Format`, `Status`, `Title`, `Episodes`, `AiringSchedule`, `NextAiringEpisode *AiringNode`, `Synonyms`, `Relations` |
| `AiringNode` struct | `ID`, `Episode`, `TimeUntilAiring`, `AiringAt`. `ID` é só o id do nó de agenda da AniList e **não identifica** um episódio (ver `EpisodeList`) |
| `MediaRelations` struct | `Edges []MediaRelationEdge` — PREQUEL/SEQUEL links |
| `MediaRelationEdge` struct | `RelationType string`, `Node MediaRelationNode` |
| `MediaRelationNode` struct | `Title`, `Synonyms`, `Episodes *int` — the related anime |
| `MediaFormat` consts | `TV`, `MOVIE`, `OVA`, `ONA`, etc. |
| `EpisodeList(ml, fromEpisode)` (`episodes.go`) | **A única fonte de "quais episódios existem"**. Sintetiza a lista de `fromEpisode` até o último no ar, usando o nó real do `airingSchedule` quando existe. Necessária porque a AniList guarda só uma JANELA de ~500 entradas de agenda por mídia: One Piece começa no 1123 e anime antigo/finalizado vem com agenda VAZIA — ver decisions.md #52 |
| `lastAiredEpisode(ml)` / `LastAiredEpisode(ml)` (`episodes.go`) | Combina agenda + `nextAiringEpisode - 1` + `media.episodes` (este só quando FINISHED, pois num RELEASING é a contagem prevista). A versão exportada é a medida de "tamanho da série" usada pela busca de episódio — `media.Episodes` não serve, é nil justamente na série longa em andamento |
| `MediaListStatus` consts | `CURRENT`, `COMPLETED`, `DROPPED`, `PAUSED`, `PLANNING`, `REPEATING` |
| `GetAllCurrentAnime(username)` | Fetches CURRENT+REPEATING anime list with synonyms and relations (used by verification loop) |
| `GetAnimeInfo(mediaId, usernames)` | Fetches one anime by **media** id with full airing schedule, synonyms, and relations, querying each account and collapsing via `DedupeByMedia`. Returns `(nil, nil)` when no account tracks it — a normal state, not an error (used by `/animes/{id}/episodes`, `refreshOrphanAnimes` and `daemon.RunAnimeDebug`) |
| `SearchMedia(term, includeUnreleased)` | `Page(perPage:20){media(search:...)}` — feeds the add-anime search bar. Not cached: every keystroke is a different key, so the debounce is what limits the volume. With `includeUnreleased=false` (the screen's default) the query gains `status_not: NOT_YET_RELEASED`, appended as a **string**, not passed as a null GraphQL variable — see decisions.md |
| `GetMediaByID(mediaId)` | Reads one anime by media id **without** going through any account's list — the primitive a standalone anime needs (`GetAnimeInfo` returns nil for it). Returns a **synthetic** `MediaList`: only `Media` filled, `Progress: 0`, `Status: ""`, `Id: 0` — the zeros are part of the contract, since `DedupeByMedia` keeps the LOWEST progress. Same fields as `getMediaListEntry` (synonyms, relations, airing-schedule ids) because the search/selection path (`searchNyaaForAnime`, `searchNyaaForSingleEpisode`) depends on them. Cached 60s per id |
| `GetMediaListStatus(username, mediaId)` | One account's list status for one media; the bool reports whether that account tracks it at all. Only `allAccountsAgreeOnDelete` uses it — see the delete rule below |
| `GetMediaIDForEntry(mediaListId)` | Legacy entry id → media id. The only thing left that keys by entry id: `MigrateAnimeIDsToMedia`. Returns 0 when the entry no longer exists |
| `ErrNotFound` | Sentinel for AniList's 404 — lets a by-id lookup tell "was deleted" from "AniList is down" |
| `sendAnilistRequest[T]` | Generic GraphQL POST helper |
| `GetFrontendAnimeList(username, statuses)` | Lighter list query behind `GET /animes`. **Cached for 60s** per `username+statuses` and hands out a copy of the slice — the frontend polls this endpoint every 30s per open tab, which used to blow AniList's 30 req/min budget and 429 the daemon's own loop (decisions.md #46) |
| `GetCustomListsMap(username, statuses)` | Minimal `id + customLists` query, cached 5min (30s when the response is empty) — see decisions.md #11 |
| `ttlCache[T]` | Tiny TTL map behind all three caches (`get`/`set`/`clear`) |
| `httpDo` var | Swappable HTTP func — overridden in tests via `MockAniListDo`, which also clears every cache so one test can't serve another's responses |

### `src/internal/anilist/standalone.go`

`SearchMedia(term)`, `GetMediaByID(id)`, `MediaSearchResult` and `mediaByIDCache` — the two queries the standalone-anime feature needs, both listed in the `anilist.go` symbol table above.

### `src/internal/anilist/health.go`

Saúde da AniList, exposta ao frontend como **código** (nunca frase pronta — o daemon não sabe o
locale do navegador), pela mesma razão de `daemon/passerror.go`.

| Symbol | Purpose |
|--------|---------|
| `Health` struct | Estado corrente: `state`, `message`, `retryAt` |
| `CurrentHealth()` | Lê o estado atual (`atomic.Pointer`, sem lock) |
| `setHealth` / `clearHealth` (unexported) | Gravam/limpam o estado a partir das respostas HTTP |
| `firstError(body)` | Extrai `(status, message)` do primeiro erro GraphQL do corpo — a AniList devolve 200 com erro no body |
| `classify(status)` | Mapeia o status HTTP no código de estado |
| `retryAfter(headerValue)` | Lê o `Retry-After` do rate limit (30 req/min) |

### `src/internal/nyaa/nyaa.go`

| Symbol | Purpose |
|--------|---------|
| `TorrentResult` struct | `Name`, `MagnetLink`, `Seeders`, `Leechers`, `Episode*`, `Resolution*`, `Season*`, `Part*`, `Size`, `Fansub`, `IsBatch` |
| `BatchInfo` struct | `StartEpisode`, `EndEpisode`, `Season`, `IsComplete` — extracted from batch torrent name |
| `torrentSummaries(results)` | Formats each result as `name \| S:412/L:3 \| 1.4GiB \| t=5 h=4.21` (t = health tier, h = raw score) for debug logging, in the order given (sorted, when logged after `SortTorrentResults`) — the sort-deciding fields, not just the name |
| `formatSize(bytes)` | Human-readable size for the log only (`?` when the size failed to parse) |
| `ScrapNyaa(title, episode, season*, part*, totalEpisodes...)` | Scrapes Nyaa for a single episode (adaptive pagination **per query variant**); discards batch (`isBatch`) and movie/OVA/special (`hasMovieMarker`); hard-filters by season and part when non-nil. With `totalEpisodes > 100` it also queries the zero-padded episode (`one piece 001`) — see `episodeQueries` and [decisions.md #56](decisions.md) |
| `episodeQueries(query, episode, totalEpisodes)` | The episode search queries: plain, plus 3-digit zero-padded on a long series (additional, never a replacement) |
| `longSeriesEpisodes` const | `100` — threshold above which the padded variant is added |
| `ScrapNyaaForAnime(title, episodes[], season*, part*)` | The single search behind pack + episode resolution: adaptive pagination, filters by the given episode numbers, and returns packs and episodes in the **same** result list — `IsBatch` on a row marks a pack, `Episode != nil` marks a matched single episode. Replaces the old `ScrapNyaaForBatch`/`ScrapNyaaForMultipleEpisodes` split — `daemon.partitionSearchResults` does the splitting the two separate functions used to do |
| `fetchSearchPages(url, floor, accepted, parse)` | Adaptive pagination shared by all three searches: page 1, then deeper only while `accepted() < floor` and the page had rows, up to `ActiveMaxSearchPages()`. Errors only if page 1 fails |
| `parsePagesWith(parseRow)` | Adapts a `parseRow` into the `parse` callback of `fetchSearchPages` (returns the row count of the page) |
| `enoughCandidates` const | `3` — the accepted-candidate floor that stops the descent |
| `SetMaxSearchPages(n)` / `ActiveMaxSearchPages()` | Page ceiling from `max_search_pages`, pushed by `files.LoadConfigs`; same atomic+restore pattern as `SetPriorities`. Getter never returns < 1 |
| `SetMaxBatchTorrentSizeGB(gb)` / `batchTooBig(size)` | Pack size ceiling from `max_batch_torrent_size_gb`, pushed by `files.LoadConfigs` (same atomic+restore pattern). `ScrapNyaaForAnime` drops an oversized pack row **before** it counts toward `enoughCandidates`, so giant packs can't end the page descent ahead of the partial packs that fit — see [Decisions](decisions.md) #59. Default `0` (off); `Size == 0` passes, same rule as `daemon.filterBySize` |
| `ScrapNyaaForMovie(title, isMovie)` | Scrapes for movie — sorted by `SortMovieResults` |
| `hasMovieMarker(name)` | Explicit movie/OVA/special marker check — the part of `isMovie` safe to use as a guard on episode searches (see [Decisions](decisions.md)) |
| `ExtractSeason(name)` | Exported: extracts season number from torrent name |
| `ExtractPart(name)` | Exported: extracts part/cour number from torrent name |
| `ExtractBatchInfo(name)` | Exported version of `extractBatchInfo`: parses a pack's episode range (`StartEpisode`/`EndEpisode`/`Season`/`IsComplete`) out of the torrent name. `daemon.pickBatches`/`coveringBatch` use it to decide which episodes a pack covers. Guards against reading a resolution tag (`[720-1080p]`) as an episode range — see decisions.md. Contract: `EndEpisode == 0` means "unknown range", and every caller treats that as a complete pack |
| `GenerateSearchTitleVariants(romaji, english)` | Search query variants: clean romaji → original romaji → clean english → original english |
| `SortTorrentResults(results)` | Sorts by the episode-relevant subset of `ActivePriorities().CriteriaOrder` (default: uncensored → resolution → health tier → fansub → size) |
| `SortMovieResults(results)` | Sorts by all of `ActivePriorities().CriteriaOrder` (default: uncensored → source → resolution → health tier → codec → fansub → audio → size) |
| `IsBatch(name)` | Exported batch detection for tests |
| `IsMovie(torrentName, animeName, isFormatMovie?)` | Exported movie detection for tests |
| `MockNyaaHttpGet(fn)` | Replaces `httpGet` for tests; returns restore func |
| `httpGet` var | Swappable HTTP func — overridden in tests via `MockNyaaHttpGet` |
| `getNyaaBaseURL()` | Reads `NYAA_URL` env or defaults to `https://nyaa.si` |

All three `ScrapNyaa*` functions fetch through `fetchNyaaPage`/`fetchSearchPages` (so every page request is logged), log every parsed row at Debug (`"Raw Nyaa row"`, before any filter) and log the matched torrents alongside the count in their final `"Found ..."` log (`matched_torrents`, via `torrentSummaries`) — used by `daemon.RunAnimeDebug` and manual troubleshooting to see what got filtered out.

### `src/internal/nyaa/priorities.go`

Holds the user-configurable priority lists that drive torrent ranking/filtering. See [Config Reference](config.md) for the `Priorities` struct fields.

| Symbol | Purpose |
|--------|---------|
| `Priorities` struct | `CriteriaOrder`, `Fansubs`, `Resolutions`, `Sources`, `Codecs`, `Audio`, `IgnoreList` |
| `DefaultPriorities()` | Returns the built-in defaults (faithfully reproduces the original hardcoded maps/regex) |
| `ActivePriorities()` | Returns the currently active `Priorities` (never nil — package `init()` seeds it with defaults) |
| `SetPriorities(p)` | Atomically swaps the active priorities; returns a `restore func()` to revert (same pattern as `MockNyaaHttpGet`). Called by `files.LoadConfigs()` on every successful load |
| `ShouldIgnore(name)` | True if `name` matches any (case-insensitive substring) entry in the active `IgnoreList` |
| `priorityIndex(list, token)` | Index of `token` (lowercased) in `list`, or `len(list)` (worst) if absent |
| `criterionCompare` map | `criteria_order` value → comparator `func(a, b TorrentResult) int` |
| `healthTier(r)`, `healthTierFloors` | Health band of a result (0..5, floors `1/5/20/100/400` seeders). What the `health` criterion compares — **not** the raw score (decisions.md #55) |
| `sortByCriteria(results, criteria)` | Stable sort applying `criteria` in order, first non-zero comparator wins |
| `episodeCriteria` | Subset of criteria valid for `SortTorrentResults` (excludes `source`, `codec`, `audio`) |

### `src/internal/nyaa/nyaa_regex.go`

Pre-compiled package-level regex vars. All compiled at package init for performance inside per-torrent loops.

| Var | Purpose |
|-----|---------|
| `reSeasonStrip` | Strips season suffixes from query names before search |
| `reBatchPatterns` | 17 patterns detecting batch torrents (range, complete, season markers) |
| `reBatchRange`, `reBatchComplete` | Batch episode range and "complete/batch" extraction |
| `reMovieKeywords`, `reOvaPattern`, `reSpecialPattern`, `reHasEpisode` | Movie/OVA/special detection, episode presence check |
| `reFansub` | Extracts fansub from `[FANSUB]` or `(FANSUB)` prefix |
| `reEpisodePatterns` | 10 episode extraction patterns ordered by specificity (SxxExx → `- 05` → `[05]` → etc.) |
| `reSeasonPatterns` | 7 season extraction patterns (S01 → Season 1 → ordinal → Cour) |
| `reResolutionPatterns` | Resolution extraction (1080p, 1920x1080, 4K, etc.) |
| `reSourcePatterns` | Release source (BD, BDRip, WEB-DL, HDTV, etc.) |
| `reCodecPatterns` | Video codec (HEVC, AV1, H.264, XviD) |
| `reAudioPatterns` | Audio codec (FLAC, DTS-HD, TrueHD, DDP, AAC, AC3, MP3) |
| `reSeasonNamePatterns` | Season/part name stripping for query base extraction (Season N, S N, Cour N, Part N) |
| `rePartPatterns` | 5 part/cour extraction patterns ordered by specificity (`\| Part 02`, `(Part 2)`, `[Part 2]`, `Part 2`, `Cour 2`) |
| `rePartStrip` | Strips part/cour suffixes from query names before search |
| `reParseSizeRe` | Size string parsing (e.g. `"1.5 GiB"`) |

### `src/internal/nyaa/nyaa_match.go`

Title-matching logic for filtering Nyaa search results.

| Symbol | Purpose |
|--------|---------|
| `titleTechnicalTokens` | Map of tokens to strip before title comparison (codecs, sources, fansubs, season/ep markers) |
| `extractTitleTokens(name)` | Returns meaningful title tokens, stripping all technical metadata |
| `isTitleTechnicalToken(s)` | True if token is technical (codec, resolution, episode marker, hex hash, etc.) |
| `jaccardSimilarity(a, b)` | `|intersection| / |union|` over two token sets |
| `titleJaccardThreshold` | `0.8` — minimum Jaccard similarity for a match |
| `titleMatchesQuery(torrentName, query)` | Two-pass: (1) all query tokens present in torrent title; (2) Jaccard ≥ 0.8. Prevents partial-title and spinoff false positives |
| `TitleMatchesQuery(torrentName, query)` | Exported alias for tests |

### `src/internal/torrents/`

Embedded BitTorrent client (`github.com/cenkalti/rain/v2`) behind a `TorrentBackend` interface. The daemon and tests share one seam: production uses the rain-backed `SessionManager`/`Session`; tests use the in-memory `FakeBackend`.

**`backend.go`**

| Symbol | Purpose |
|--------|---------|
| `TorrentBackend` interface | `Ensure(savePath)`, `ConsumeRootSwap()`, `Add(magnet)`, `List()`, `Get(hash)`, `Files(hash)`, `Remove(hash, keepData)`, `Pause(hash)`, `Resume(hash)`, `Announce(hash)`, `Prioritize(hash)`, `PrioritizeAll(hashes)`, `SetMaxActiveDownloads(n)`, `SetCallbacks(onComplete, onFailed)`, `Close()` |
| `TorrentBackend.Pause/Resume/Announce(hash)` | Per-torrent controls, all queue-aware. `Pause` stops the torrent (non-blocking — `stopping` for up to ~5s before `stopped`) **and marks it paused-by-the-user** in the queue, so it keeps its place but never starts on its own; `Resume` puts it at the **back** of the queue and re-arms the completion listener that pausing consumed — it is not "start now" (see decision 41). Both **bypass the queue entirely for a completed torrent** (seeding never held a slot). `Announce` forces a tracker/DHT re-announce without overriding the trackers' minimum interval |
| `TorrentBackend.Files(hash)` | Files inside the torrent (`[]FileInfo{Path, Size, BytesCompleted *int64}`), raw metadata paths and metadata order. Prefers rain's `FileStats()` (per-file progress) and falls back to `Files()` when the torrent is stopped — `Stop()` frees `t.pieces`, which makes `FileStats()` fail, while `t.info` survives; in that state `BytesCompleted` is `nil` ("unknown"), never 0. **Empty slice, no error, while the metadata has not arrived** — a fresh magnet has no list, and that is a normal state. Read-only by necessity: rain v2 has no `SetWanted`/`FilePriority`, so selecting or dropping a file inside a pack is not possible |
| `TorrentBackend.Prioritize(hash)` | Moves the torrent to the **front** of the queue and starts it, demoting whichever active torrent is now last in queue order when that exceeds the limit (position, not progress). Errors on an unknown or already-completed hash. Backs the row's "Priorizar" button and the manual-download endpoints (`daemon.addAndPrioritize`) |
| `TorrentBackend.PrioritizeAll(hashes)` | Batch form, applied **in the order received** — one call, because N `Prioritize` calls would front-push past each other and reverse the batch. Unknown/completed hashes are ignored, not rejected. Backs the group and bulk "Priorizar" buttons |
| `TorrentBackend.SetMaxActiveDownloads(n)` | Caps concurrent **incomplete** torrents; `0` disables the cap. Fed by `Config.MaxConcurrentDownloads` |
| `TorrentBackend.ConsumeRootSwap()` | Reports **and clears** a swap latched by `Ensure`: the download folder was moved/trashed/replaced. Latched rather than returned by `Ensure` because the manual-download endpoints call `Ensure` too and must not swallow it — only the verification pass consumes it (decisions.md #34) |
| `TorrentInfo` struct | Backend-agnostic snapshot: `Hash` (join key with `EpisodeHash`), `Name`, `DataDir` (`<download-path>/<id>`), `Completed`, `Status` (API slug from `statusSlug`), plus progress fields (`BytesCompleted/Total/Uploaded`, `DownloadSpeed`, `UploadSpeed`, `PeersTotal`, `PiecesHave/Total`, `ETASeconds`, `SeededForSeconds`, `AddedAt`) — all filled from a single `Stats()` call per torrent in `toInfo`. `QueuePosition` is the exception: 1-based place in the queue's waiting line, written by `queue.markQueued`, `0` = not waiting |

**`status.go`**

| Symbol | Purpose |
|--------|---------|
| `statusSlug(torrent.Status)` | Maps rain's status enum to the stable API slug (`stopped`, `downloading_metadata`, `allocating`, `verifying`, `downloading`, `seeding`, `stopping`, `unknown`) — never `Status.String()`, which is display text (`"Downloading Metadata"`) and can be reworded by a library upgrade |
| `StatusStopped` / `StatusStopping` / `StatusQueued` | The three slugs other code compares by name. `queued` is the only slug rain never produces — `queue.markQueued` writes it (decision 41) |

**`session.go`** — rain-backed implementation.

| Symbol | Purpose |
|--------|---------|
| `Session` struct | Wraps a `torrent.Session`; `DataDir=Config.DownloadPath()`, `Database=session.db`, `DataDirIncludesTorrentID=true`, RPC disabled |
| `NewSession(savePath, databasePath)` | Creates the embedded client |
| `Session.Add/List/Get/Remove/Pause/Resume/Announce/SetCallbacks/Close` | Implement `TorrentBackend` |
| `toInfo(t)` | Builds a `TorrentInfo` from one `t.Stats()` call; `Completed` comes from `completedFromStats`, not from `Status` |
| `completedFromStats(st)` | `st.Pieces.Total > 0 && st.Pieces.Have >= st.Pieces.Total` — deliberately independent of `Status`, because pausing a finished torrent takes it out of `Seeding` (see decision 30) |
| `parseInfoHash(magnet)` | Extracts the lowercase-hex info hash from a magnet link |

**`queue.go`** — the ordered list of **every** incomplete torrent, the concurrency limit, and the manual priority override. Owned by `SessionManager` (not `Session`), so it survives a session being torn down and rebuilt.

| Symbol | Purpose |
|--------|---------|
| `queueOps` interface | `list()`, `pause()`, `resume()` — the **raw** delegations, all unexported. Going through `List`/`Pause`/`Resume` would re-enter the queue: infinite recursion for pause/resume, deadlock on `queue.mu` for list |
| `queue` struct | `limit`, `order []string` (every incomplete torrent, in download order), `paused []string` (paused by the user; incomplete hashes only), `queued map[string]int` (hash → 1-based waiting position, the output of `enforce`'s step 3), `path`/`lastSaved` (persistence), `seedPaused` (one-shot upgrade latch) |
| `queue.enforce(ops)` | The single decision point, a reconciliation in five steps: **0** bail out when `list()` is `nil` (no session — `nil` ≠ empty session, see decision 41); **1** prune hashes that are gone or completed; **2** append missing incompletes at the end, ordered by `AddedAt`; **3** compute the wanted set and the waiting positions; **4** apply the diff **iterating `order`, never the session** (that would pause every seeder), leaving `stopping` alone; **5** save when changed. Triggered by `Add`, the completion callback (`wrapComplete`), `Prioritize`/`PrioritizeAll`, `Resume`, `Pause`, `Remove`, `SetMaxActiveDownloads` and the `Ensure` that creates a session |
| `queue.markQueued(infos)` | Writes the `queued` slug **and** `QueuePosition` from `q.queued` — not from `order`, which now holds the active ones too. Called by `SessionManager.List`/`Get`, never by `enforce` |
| `queue.prioritize(hashes)` | Moves to the front the hashes already in `order` and **inserts** the ones that are not, in the order received; clears them from `paused` |
| `queue.pushBack/markPaused/drop/setLimit` | Queue-order primitives. `pushBack` = `Resume` (to the **end**, and out of `paused`); `markPaused` = `Pause` (into `paused`, position untouched); `drop` = `Remove` |
| `queue.load(path)` / `queue.save()` | `queue.json` next to the resume DB. `load` runs once in `NewSessionManager`; a **missing** file arms `seedPaused`, a corrupted one only warns. `save` is tmp + `Rename`, only when the marshaled state differs from `lastSaved`, and `lastSaved` only advances after a successful `Rename` |

**Lock order: `queue.mu` → `SessionManager.mu`, never the reverse.** `enforce` holds `queue.mu` while calling `list`/`pause`/`resume`, so every `SessionManager` method releases its own lock **before** touching the queue. A reentrant `RLock` deadlocks the moment a writer queues between the two acquisitions.

**`sessionmanager.go`** — lifecycle owner.

| Symbol | Purpose |
|--------|---------|
| `SessionManager` struct | Owns the current `Session`; recreates it when the download path changes **or when the download root was swapped**; keeps `session.db` stable across changes |
| `NewSessionManager(dbPath)` | Constructor; derives `download_root.id` and `queue.json` from `dbPath`'s folder, and loads the persisted queue |
| `SessionManager.Ensure(savePath)` | Creates/recreates the session; returns `true` when a new session was made (caller reconciles); latches `pendingSwap`; `ErrSessionNotReady` if `savePath==""` |
| `SessionManager.ConsumeRootSwap()` | Reads and clears `pendingSwap` |
| `SessionManager.checkRoot(savePath)` | Compares `download_root.id` with `<savePath>/.aad_root`; mismatch ⇒ swapped. No id on record (first run/upgrade) is never a swap |
| `SessionManager.Pause/Resume/Announce/Prioritize(hash)` | Delegate to the current `Session` under the read lock, then run the queue **outside** it; `ErrSessionNotReady` if no session exists. `Pause`/`Resume` of a **completed** torrent skip the queue bookkeeping entirely |
| `SessionManager.PrioritizeAll(hashes)` | Batch prioritize. It must **not** call `Get`/`List` — both go through `markQueued`, which takes `queue.mu`; `Prioritize(hash)` validates *before* delegating here, never during |
| `SessionManager.list()` / `pause()` / `resume()` | The unexported `queueOps` implementation — raw delegation, no queue side effects |
| `SessionManager.wrapComplete(cb)` | Wraps the caller's completion handler so `enforce` runs first: a torrent finishing is the moment a slot frees. The raw handler stays in `m.onComplete` so `Ensure` re-wraps per session instead of stacking wrappers |
| `ErrSessionNotReady` | Returned when no session exists yet (incomplete config — `completed_anime_path` empty, so `Config.DownloadPath()` can't be derived) |

**`rootmarker.go`** — download-root identity.

| Symbol | Purpose |
|--------|---------|
| `RootMarkerName` (`.aad_root`) | Marker written inside the download folder; travels with the folder when the user moves it |
| `rootIDFileName` (`download_root.id`) | The same id in the config folder, where the user cannot move it |
| `newRootID` / `readRootID` / `writeRootID` | Generate/read/write the id. A read error other than "not exists" is returned, never silently read as a swap |

**`fakebackend.go`** — in-memory test double.

| Symbol | Purpose |
|--------|---------|
| `FakeBackend` struct + `NewFakeBackend()` | Implements `TorrentBackend` with an in-memory map |
| `FakeBackend.Pause/Resume(hash)` | Set `Status` to `"stopped"`/`"downloading"`; error if the hash is absent |
| `FakeBackend.Announce(hash)` | Records the call in `announceCalls`; error if the hash is absent |
| `FakeBackend.AnnounceCalls()` | Returns the hashes passed to `Announce`, in order — for test assertions |
| `FakeBackend.RootSwapped` | Makes `Ensure` report a swapped root, so daemon-side recovery is testable without a real session |
| `FakeBackend.EnsureCalls()` | Returns the paths passed to `Ensure`, in order — used to assert which download path a caller opened the session at, and whether it opened one at all |
| `FakeBackend.AddCompleted(hash, dataDir)` / `CompleteTorrent(hash, dataDir)` / `FailTorrent(hash, err)` | Test helpers to drive completion/failure callbacks |

### `src/internal/notifications/notifications.go`

| Symbol | Purpose |
|--------|---------|
| `Event` type | `NewEpisode`, `DownloadFailed`, `DownloadCompleted` (the webhook event key string for the last one is still `download_completed` — only the Go constant was renamed from `QBittorrentDownloadCompleted`) |
| `NewEpisode` ordering | Fired by `processAnimeEpisodes` **only when there is at least one magnet to try** — an episode with no search result goes straight to `DownloadFailed`/`ReasonNotFound`. Firing it earlier sent a false "starting download" push on every loop pass (every `check_interval`) for an episode that never started |
| `Notify(cfg, event, animeName, episode int, reason string)` | Fires all configured webhooks for an event in background goroutines. No-op if cfg is nil or has no webhooks. With `notifications.batch_window_seconds > 0` the event joins a **per-event** queue and leaves with the rest of its window as one webhook (decisions.md #47) |
| `Flush()` | Fires every pending batch **synchronously** and only returns once the requests finished. Called from `cmd/daemon/main.go` at shutdown — firing in goroutines there would be the same as not firing |
| `flushEvent(event, wait)` / `fireBatch(event, items, webhooks, wait)` | Take the queue out under lock and fan out to the subscribed presets; `wait` is what makes `Flush` synchronous |
| `FireTestWebhook(cfg, name)` | Fires one named webhook with sample variables. Returns error if not found |
| `interpolate(template, vars)` | Replaces `{{var}}` placeholders — missing vars become empty string |
| `buildVars(animeName, episode, event, reason)` | Wrapper over `buildBatchVars` with a single item — keeps a one-item window byte-identical to the unbatched message |
| `buildBatchVars(event, items)` | Template vars for a window. N > 1: `{{title}}` gets the count, `{{message}}` becomes one line per item, `{{count}}` is N, and `{{anime_name}}`/`{{episode}}`/`{{reason}}` are empty — no single value exists |
| `jsonEscape` / `escapeVarsForJSON` / `presetIsJSON` | Escape var values **in the body only, and only when the preset declares `Content-Type: application/json`**. The batch `\n` (and any quote in an anime name) would otherwise produce an invalid JSON body → 400 with the notification vanishing; ntfy's raw-text body must keep the real newline |
| `fireWebhook(preset, vars)` | Sends HTTP request; logs error/warn on failure but never panics. Strips `\n`/`\r` from header values — `net/http` rejects the whole request otherwise, and ntfy uses `Title: {{title}}` |

**Template variables available in URL, headers, and body**: `{{title}}`, `{{message}}`, `{{anime_name}}`, `{{episode}}`, `{{reason}}` (failure reason, empty for non-failure events), `{{count}}` (items in the batch — `1` when not batching), `{{quality}}` (always empty), `{{file_path}}` (always empty), `{{timestamp}}` (formatted `2006-01-02 15:04`).

### `src/internal/stringutil/stringutil.go`

- `RemoveSpecialCharacters(s)` — strips chars that break Nyaa search queries

### `src/internal/logger/logger.go`

- `Logger` global — zerolog instance, writes to console + rotating `daemon.log`
- `InitDebug(filePath)` — like `Init(true)` but the structured JSONL trace goes to `filePath` instead of `daemon.log`. Used by `--debug-anime` so one-shot debug runs never touch `~/.autoAnimeDownloader`. Returns a `close func() error`

### `src/internal/cli/`

Daemon lifecycle control for the CLI binary (`cmd/cli` is the only importer). Everything below
lives in `process.go`, except the two build-tagged `getSysProcAttr` variants. Prefers the
platform service manager and falls back to a PID file (`daemon.pid`, in the config folder).

| Symbol | Purpose |
|--------|---------|
| `IsRunning()` / `Start(daemonBinary)` / `Stop()` / `GetPID()` | The exported surface — `loop start`/`loop stop`/`status` sit on top of these |
| `startWithSystemd` / `stopWithSystemd` / `isRunningWithSystemd` (unexported) | Linux path via `autoanimedownloader.service` (systemd **user** unit), used when `isSystemdAvailable()` |
| `startWithWindowsService` / `stopWithWindowsService` / `isRunningWithWindowsService` | Windows path via the `AutoAnimeDownloader` service, used when `isWindowsServiceAvailable()` |
| `isRunningWithPID` | Last-resort path when no service manager answers |
| `getSysProcAttr()` | Build-tagged detach flags — `process_unix.go` (`!windows`) vs `process_windows.go` |

### `src/internal/tray/`

System tray icon. Build-tagged: the real implementation needs CGO, so a stub keeps the
CGO-free build (which CI asserts in `build.yml`) compiling.

| File | Purpose |
|------|---------|
| `tray.go` (`cgo && !notray`) | `TrayManager` — `NewTrayManager(apiPort)`, `Start()`, `WaitForShutdown()`; menu opens the Web UI (`daemon.WebUIURL`) and triggers a check |
| `tray_stub.go` (`!cgo \|\| notray`) | Same `TrayManager` API, does nothing. This is what a `CGO_ENABLED=0` binary gets |
| `icon.go` | `GetIconData()` over the embedded `icon.png` (also copied into the Linux release zip by `make package-*`) |

On Linux the daemon only starts the tray when `DISPLAY` is non-empty (`cmd/daemon/main.go`).

### `src/internal/version/version.go`

`Version` string, `"dev"` by default, overwritten at build time via
`-ldflags "-X AutoAnimeDownloader/src/internal/version.Version=$VERSION"` (see
`docker/Dockerfile.build.*` and `scripts/build.sh`).

### `src/internal/frontend/embed.go`

`DistFS` — the `//go:embed dist/*` of the built frontend. `dist/` must exist before
`go build`, which is why the frontend always builds first (see [Build Guide](../guides/build.md)).
`api.Server.handleStaticFiles` serves it through `fs.Sub(DistFS, "dist")`.

### `src/internal/frontend/src/`

**Routes** (rendered via hash router):

| File | Route | Purpose |
|------|-------|---------|
| `routes/Status.svelte` | `#/` | Daemon status **and** anime list — one screen, not two (redesign decision D4; there is no separate "Biblioteca" route). Header holds the daemon pill (`PulseDot` + label + relative last-check) and start/stop/force-check; a hero card shows aggregate download speed (`formatSpeedParts`, split number/unit), a `Sparkline` fed by `speedHistory`, and one `ProgressRing` per active download; the right column has the library `TripleProgressBar` and disk/next-check cards; the anime list renders a derived `Chip` per row (`deriveAnimeChip`) with search, unwatched filter and sortable name/watched/last-download headers; the countdown to the next episode (`next_airing_at` → `nextAiringIn`) is its **own column** (`LIST_GRID`, between progress and last-download), holding just the time ("in 2 days") since the column header already says "Next ep." — it started as a third chip and crowded the 190px state column out; the mobile card, which has no column headers, appends the full phrase to the last-download line instead. The column has no sort key, and stays blank for anime with no scheduled episode; a standalone anime gets a second neutral "Avulso" chip **next to** the derived one, never inside `deriveAnimeChip` (that cascade returns a single download state, and origin isn't a state — a standalone anime that is downloading must keep its "Downloading" chip). Polls `GET /api/v1/torrents` every 5s — a failed poll sets a `stale` flag that switches the "polling 5s" note to a frozen-values warning and stops feeding `speedHistory` (never extrapolates). A full-width **first-steps card** (`data-testid="onboarding-card"`) sits after the alerts and before the last-check report and the hero: three **numbered** items — library folder → anime source → first check — in accent tint, not the neutral card surface, so it doesn't read as one more panel. Each number **is** a real checkbox the user ticks by hand (`onboardingDone`); nothing is derived into a checkmark, because on a fresh install steps ① and ③ came up green on their own (the path has a default, the pass runs by itself) and the tutorial looked half-finished before it was read. It disappears on any of three exits: all three ticked, dismissed, or `allDone(onboardingSteps(...))` — the daemon already configured and running, so an existing install is never taught the obvious (**no new request**; the screen keeps `completed_anime_path` raw and `anilist_usernames` for that last check). Item ② offers two alternatives joined by a literal "or" — `#/config?group=anilist` and `#/add`, the latter inheriting the same library-not-configured block as the header button — because side-by-side buttons without the "or" read as two required steps. Hints are one short line each: a paragraph nobody reads teaches nothing. The dismiss control is a text button ("Don't show again"), not a `×` — the behaviour is permanent and the label has to say so. Tint opacities use the bracket form (`bg-accent-tint/[.10]`): Tailwind only generates the default opacity scale, so a `/12` is a dead class and the surface silently loses its background. `libraryConfigured` (the header's "+ Add anime" gate) is derived from `onboarding.library` rather than a parallel `Boolean(completed_anime_path)`, so a whitespace-only path can't leave the card asking for the folder while the button is already enabled; it stays permissive while `loading` so the button never flashes disabled. |
| `routes/AddAnime.svelte` | `#/add` | Search AniList and start tracking an anime that is in no list ("avulso"). `<input>` with a 300ms debounce plus an `AbortController` cancelling the previous request — both requirements, not polish: without the debounce AniList's 30 req/min limit blows up while typing, and without the abort a stale result paints over a newer one. Searches from 3 characters. A `Toggle` under the search bar controls `include_unreleased` (off by default, hiding `NOT_YET_RELEASED`); flipping it re-runs the search **immediately**, bypassing the debounce, because a click doesn't fire in bursts. The toggle is blind — the server-side filter means nothing knows how many results were hidden, and it does not persist between visits. Each result card is cover + title + meta line + reason line + a footer driven by `block_reason`: `standalone`/`tracked`/`downloaded` (and anything added in this session) → a **link** to `#/status/{id}`, since `anime_id` is the AniList media id; `blacklist` → dimmed card + disabled Add button, the only reason with no detail page to open; `""` → Add / Adding…. The reason itself is a line in the card, not a tooltip — tooltips don't exist on mobile. The title is an `<a target="_blank">` to `https://anilist.co/anime/{id}`. The front is best-effort and the backend is the authority: the 409 toast has the final word, there is no retry or revalidation. Second item in the nav, with the same prominence as Status (`primaryNavItems` in `lib/navItems.ts`) — it is the door every anime comes through, and an installation with no AniList account has nothing else to do. Also reached from the primary button in the Status header (disabled with a tooltip when the library is not configured) and from the Status empty state. The `NavTabBar` columns are `flex-1`, so its count follows `navItems.ts`: five columns now, labels truncating on narrow phones (the documented degrade, same as "Configurações") |
| `routes/Downloads.svelte` | `#/downloads` | Live torrent list as an **accordion grouped by anime** (`groupTorrents`): group header with cover, aggregate bar and group-scoped bulk actions; indented torrent rows with status chip, per-row bar and icon actions (no hash column — the info hash is a join key for the code, not something the user reads). Group order is a fixed severity rule (problems → downloading → rest); the user's sort key orders rows *within* a group. Header shows a ↓/↑ bandwidth summary and a "polling 2s" note; a banner appears only while the WebSocket is disconnected, since progress comes from the HTTP poll and not the socket (this screen opens its own `WebSocketClient` so that state is meaningful here). Search/filter/sort **and the set of collapsed groups** round-trip through the URL querystring, not localStorage; select-all/bulk pause/resume/announce/delete live in `DownloadsToolbar.svelte`; per-row and bulk delete use `TorrentDeleteDialog.svelte` against `DELETE /torrents/{hash}`. Every torrent row — batch **and** single-episode — expands into a third accordion level listing the files inside it (`TorrentFiles.svelte`, `raw` mode): the file name is what tells fansub, resolution and codec apart, which the torrent name does not always repeat. The panel is fetched on expand and refreshed on this screen's poll tick; the open set is local state, not in the URL. Polls `GET /api/v1/torrents` every 2s while mounted (plus one non-polled `GET /animes` for cover art), stops polling on unmount |
| `routes/AnimeDetail.svelte` | `#/status/:id` | Per-anime episode list + actions. There is no `#` column: it repeated, in another typeface, the number the episode title already carries — the pack row's expand chevron lives in the title cell instead. **One** action definition — `episodeActions()` (`lib/domain/`) — drives both the desktop grid and the mobile stack, replacing the five icon-only buttons that used to be written out twice; each row shows a labelled principal action in a fixed column plus an `ActionMenu` (`⋯`) holding the rest, also labelled. `delete`/`redownload` still go through `ConfirmDialog` — deletion is never one click. Header carries a breadcrumb, cover, the derived `deriveAnimeChip` chip, the magnet-paste button and — only when `is_standalone` — a "Stop tracking" action (a `ConfirmDialog` with a "delete downloaded files" `Checkbox` in its slot, unchecked by default); the custom Nyaa search query (`custom_search_query`) lives in a collapsible block. Joins each episode against the live torrent list via `episode_hash` (`torrentsByEpisode.ts`) to show an inline 4px `ProgressBar` while a torrent is in flight. Um pack é uma linha só, e ela **engole** os episódios já lançados de dentro da sua faixa (`batch_start`/`batch_end`) que não têm torrent próprio — os assistidos nunca viram registro salvo, mas os arquivos vêm no mesmo torrent e o `Organize` hardlinka o diretório inteiro; oferecer "Baixar" neles abriria um segundo torrent do que já está vindo. Episódio não lançado e episódio com hash próprio mantêm a linha deles. A linha do pack — e só ela — expande num painel de arquivos (`TorrentFiles.svelte`, modo `episodes`): é a única linha com lacuna, já que episódio com torrent próprio já mostra progresso individual e os de dentro do pack são engolidos numa linha só de propósito. É também a primeira vez que a tela mostra o que o pack **realmente** contém — a faixa exibida vem do *nome* do torrent (`BatchStart`/`BatchEnd`). Arquivo cujo número a heurística não casar não aparece ali; a lista completa e crua está a um clique nas Downloads. Adaptive poll of `GET /api/v1/torrents`: 2s while this anime has an active torrent, 15s otherwise |
| `routes/Config.svelte` | `#/config` | Edit all config fields. 196px side index with **one group visible at a time** (Library / Anilist / Downloads / Torrent search, `type GroupId`), starting on Library — it holds the screen's only required field, which is where `#/config?missingConfig=true` points the user. A divider sits above "Torrent search" in the index, marking it advanced. Below `md` the index items **wrap** instead of scrolling horizontally (decision 39) — the `w-full` dividers force the breaks, so the three resulting rows are everyday groups / advanced group / exit links. Fields inside a group are separated by 1px dividers, each with label + control + help line; each field row is either **inline** (two columns — label + hint left, narrow control right; every numeric input and toggle) or **stacked** (the filesystem path, the chips inputs, the three status-pill fieldsets), collapsing to stacked below 768px. Save stays the only write path — no autosave, no debounce (redesign decision D5: `PUT /config` validates everything at once and does filesystem I/O, so a mid-typing save would 400 per keystroke). The eleven validations run client-side before the PUT and each one knows its group, so a failing rule **switches the visible group** to the offending field instead of firing an unreachable toast. They live in one `requiredChecks` list (was a chain of `if`s) because the screen now uses them twice: the Save toast, and the "still missing" dot in the side index — required fields carry a `*` plus a `* Required field` legend, and each group whose check fails gets the dot with `sr-only` text in the button's accessible name. Rewriting the conditions for the dot would let it lie the moment a rule changed. AniList status multi-selects are toggle pills with a "✓"; download and delete status sets stay mutually exclusive. `anilist_usernames`/`excluded_lists` use `ChipsInput`. The index ends with two real `<a>` links out to `#/priorities` and `#/notifications` — separate screens writing to the same `PUT /config`, also reachable from the "More" menu (`navItems.ts`). `checkQueryParams()` resolves the `URLSearchParams` **once** (`window.location.search` if present, otherwise the chunk after `?` inside the hash, since the app is a hash SPA) and reads both `missingConfig` and `group` from it — reading them in two branches would let the two diverge. `?group=<id>` opens the screen on that group, validated against the `groups` array the screen already builds; an unknown value is ignored and falls back to `library`. The Library group ends with a **First steps / Show again** row that clears both `onboardingDismissed` and `onboardingDone` (only resetting the dismissal would leave the button without visible effect for someone who hid the card by ticking all three) — a UI preference, so it is deliberately **not** in `requiredChecks` and not in the `PUT /config` body |
| `routes/Priorities.svelte` | `#/priorities` | Reorder/add/remove torrent priority lists (fansubs, resolutions, source, codec, audio, criteria order, ignore list); reset per-list or all, via `GET/PUT /api/v1/config` + `GET /api/v1/config/priorities/defaults`. The codec list also carries one-click presets (`lib/domain/priorityPresets.ts`), which only **reorder** the user's own list; `source` and `audio` are labelled "só filmes" because `SortTorrentResults` ignores them. Each row is a `Checkbox`, not a delete button: unchecking drops the token from the saved array and parks it, still visible, at the bottom of the list (absent = worst, so that *is* what removing did) — `✕` shows only on a token that is not in the served defaults, i.e. one the user typed or a leftover inert token from an older `config.json`. `criteria_order` has no add field at all: it is a closed set (`sortByCriteria` silently skips a criterion it doesn't know) and all eight always show, checked or not ([decisions.md #70](decisions.md)) |
| `routes/Logs.svelte` | `#/logs` | Tail daemon logs in a terminal-like body (`--bg-sunken`, darker than the surrounding cards) laid out as a 4-column grid — `82px 60px 90px 1fr`: time, level badge, **origin** (derived from the zerolog `caller` by `logSource.ts`), message. The grid only applies from `md` up; below that rows stack, because three fixed columns would leave ~130px for the message on a 390px screen. Rows are a real `<ul>`/`<li>`. Level filtering is pills **with counts** (was a count-less `<select>`); counts come from the search-filtered list, never the active level, so picking one pill doesn't zero the others. Search highlights the match (HTML-escaped before the `<mark>` is injected — log text is arbitrary daemon output). Lines-to-load, level and search round-trip through the querystring; follow-the-tail (scrolls to the **top**, since newest renders first), live reload with a chosen interval, the back-to-top button with its new-lines counter, and per-line copy are all preserved |
| `routes/Notifications.svelte` | `#/notifications` | Webhook configuration CRUD |

**Shell** (`src/components/shell/` — Fase 1 of the UI redesign, spec §5): `App.svelte` wraps the router in `AppShell`, not the old `Layout.svelte` (deleted; it wrote the six nav links twice — a desktop block and a mobile block — with the active-state classes repeated in each):

| File | Purpose |
|------|---------|
| `AppShell.svelte` | Grid rail + content; picks `NavRail` vs `NavTabBar` via a `(min-width: 768px)` `matchMedia` listener (JS, not `hidden md:flex`/`md:hidden` — that would keep both mounted, duplicating ids like `#theme-select`); hosts `<Toasts />` and `<SystemBanner />` |
| `SystemBanner.svelte` | The single system-wide banner, above `<main>` and outside it — it talks about the whole app (daemon down, AniList down), not the current route. Renders whatever `pickBanner` selects, polls `GET /api/v1/status` every 30s (`silent`) purely to feed it, and ticks a 1s timer only for the 429 countdown. The report link (own bugs only) is a prefilled GitHub issue URL, so no backend is involved |
| `NavRail.svelte` | 92px vertical rail (desktop), `sticky top-0 z-30` so it stays pinned while the page scrolls **and paints above page content** — `sticky` always creates a stacking context, so without a z-index here the whole rail (MoreMenu panel included, `z-50` notwithstanding) loses to any later positioned element, e.g. daisyUI's `position: relative` `.card` on Prioridades: Status/Downloads, a divider, Configurações, then "Mais" (opens `MoreMenu`); footer holds the theme select, locale toggle, WS dot, app version. The 92px is set by the longest label — "Configurações" measures 75px at the current type scale; it is not a free number to shrink |
| `NavTabBar.svelte` | 4-column bottom tab bar (mobile): Status, Downloads, Config, Mais; fixed, safe-area padding. Labels are `min-w-0` + `truncate` so the longest one ("Configurações", ~82px) ellipsizes rather than overlapping its neighbour on very narrow phones (~76px per column at 320px; it fits from 375px up) |
| `MoreMenu.svelte` | Local dropdown (no generic primitive yet — that's Fase 2's `ActionMenu`) listing Notificações/Prioridades/Logs. Two call sites, one component: from `NavRail` (`showFooterControls={false}`) and from `NavTabBar` (`showFooterControls={true}`, so it also hosts theme/locale/WS/version — the tab bar has nowhere else to put them). Closes on Escape, outside click, or navigation |

`lib/navItems.ts` is the single source of the nav item list (route, icon, i18n message function, which group) that `NavRail`/`NavTabBar`/`MoreMenu` all read instead of each declaring the six items themselves; also exports `isNavItemActive`/`isMoreMenuActive` for active-state highlighting.

**Shared components** (`src/components/`):

| File | Purpose |
|------|---------|
| `StatusBadge.svelte` | Colored badge for daemon/episode status |
| `ConfirmDialog.svelte` | Modal confirmation dialog. Binds the native `open` attribute on `<dialog>` (needed so a closed dialog is out of the a11y tree/role queries even with the daisyUI `.modal-open` class applied) and exposes an optional `<slot />` for callers that need extra content between the message and the action buttons |
| `TorrentFiles.svelte` | Read-only panel listing a torrent's files, from `GET /torrents/{hash}/files`. Owns its own fetch/loading/error so both screens share one of each; a failed fetch degrades silently (keeps the last list). Fetched **on expand only**, then refreshed on the host screen's existing poll tick (the `tick` prop) — never folded into `GET /torrents`, which is a 2s poll. Two render modes off one endpoint: `raw` (Downloads — every file, metadata order, plus a codec chip next to the name). Both modes render only the **file name** (`path.Base`), with the full path in `title`: the folder is identical on every row of a torrent, so repeating it pushes the distinguishing half of the name past the truncate — a long pack folder made all 13 rows look identical and `episodes` (AnimeDetail — only files whose episode number matched, via `filesByEpisode`). In `episodes` mode the desktop table passes `gridClass={EP_GRID}` and the pack's `chip`, so a file row repeats **exactly** the episode row's columns (episode, state, details, actions) instead of putting a second list style right under the first; the actions column stays empty because the panel is read-only. A file at 100% shows "Downloaded"; the rest carry the pack's own chip, since their state *is* the pack's. The row is labelled `Episode N`, not the file name — the screen is about episodes, and the raw name is one click away on Downloads (it stays in the `title`). The mobile stack passes no grid — that screen is not a table there. In the non-grid layout each file is a **card on mobile and a single row from `lg` up**, out of one definition: `lg:contents` dissolves the footer wrapper so its children rejoin the name's flex row. A bare wrapping flex line broke into loose fragments on a narrow screen (name, then the codec chip alone, then bar and size) with nothing saying which fragment belonged to which file |
| `TorrentDeleteDialog.svelte` | Wraps `ConfirmDialog` for the Downloads delete flow (single row or bulk selection). Two checkboxes — delete files, block re-download — both default checked; emits `confirm` with `{keepData, block}` |
| `DownloadsToolbar.svelte` | Search input + four filter pills with counts (All/Downloading/Seeding/Problems, replacing the old status multi-select dropdown) + bulk action bar (prioritize/pause/resume/announce/delete/deselect) for `Downloads.svelte`. The bulk group is always rendered — dimmed and disabled with an empty selection — rather than appearing and disappearing, which used to shift the list on every click. Controlled: holds no state of its own, just relays events; the view state lives in `Downloads.svelte` via `torrentFilters.ts`. Pill counts are computed from the search-filtered list only, never the active filter, so picking one pill doesn't zero the other three |
| `Toasts.svelte` | Toast notification container |
| `ErrorMessage.svelte` | Inline error display |
| `Input.svelte` | Styled input field: label + control + help line, on the redesign's tokens. `subtitle` is a `<p>` referenced by `aria-describedby`, not the second `<label>` it used to be (one control can't have two labels). Opt-in `inline` prop switches to a two-column row (label + hint left, control right, via explicit grid placement) and `suffix` shows a unit next to the control (`aria-hidden`, since the hint already states it); both default off, preserving the stacked layout. `Config.svelte` and `Notifications.svelte` consume it; `Notifications.svelte` does not pass `inline`/`suffix` |
| `Loading.svelte` | Loading spinner |

**UI primitives** (`src/components/ui/` — Fase 2 of the UI redesign, spec §6). All consume the semantic Tailwind names backed by `src/lib/design/tokens.css`; **no component contains a literal hex**:

| File | Purpose |
|------|---------|
| `Chip.svelte` | Tinted badge, five variants (accent/ok/warn/danger/neutral) mapping 1:1 onto `deriveAnimeChip`'s `variant`. Takes rendered text via slot, never an i18n key |
| `Button.svelte` | `solid` (the accent primary), `ghost`, `warn` (destructive/attention — tinted, not a solid red fill). There is intentionally **no `ok`/green variant**: spec §4.1 reserves the solid green for "Iniciar daemon", which `Status.svelte` writes inline so the style can't be reused elsewhere. With `href` set it renders an `<a>` instead (`target="_blank"` adds `rel="noopener noreferrer"`), so navigation that looks like a card's primary action keeps middle-click and open-in-new-tab; `disabled` is ignored in that mode — a link that shouldn't be followed shouldn't be rendered |
| `ProgressBar.svelte` | Single-segment bar, `transition: width .9s linear`; `stale` kills the transition and dims the fill (the screen owns staleness detection, not this component) |
| `TripleProgressBar.svelte` | Three **adjacent, summed** segments (watched/downloaded/released) over one track. `legend` is a required prop — the spec forbids showing this bar without a textual legend |
| `ProgressRing.svelte` | 46px `conic-gradient` ring + 34px inner disc with the percentage in mono |
| `Sparkline.svelte` | 20 bars × 7px, right-aligned so the newest sample is always rightmost; unfilled slots render as empty track |
| `PulseDot.svelte` | 2.4s pulse keyframe. **Only** for "alive" indicators, never decoration |
| `Modal.svelte` | Focus trap, Escape and outside-click close |
| `ActionMenu.svelte` | `⋯` dropdown on desktop / action sheet on mobile — one component, two layouts, one item definition |
| `Cover.svelte` | AniList cover with a hatched `repeating-linear-gradient` fallback |
| `Toggle.svelte`, `Checkbox.svelte` | Form controls in the redesign's styling. `Checkbox` wraps a real `<input type="checkbox">` (native keyboard behaviour, associated label); `label` is required even with `labelHidden`, so an unlabelled checkbox can't be shipped, and `indeterminate` works because it reaches that real input (it's a DOM property with no attribute form). `Toggle` has an opt-in `inline` prop that puts the label left and the switch right via `flex-row-reverse` — visual order only, so DOM order, label association and tab order are unchanged; default off, and `AddAnime.svelte` does not pass it |
| `ChipsInput.svelte` | Multi-value field (Fase 6, spec §9.4): chips live **inside** the box with the caret, and there is **no "+" button** — keyboard is the only path in. Five rules: Enter and comma commit, Backspace on an empty draft removes the last chip, the per-chip × removes, and pasting a comma-separated list creates several at once. Also commits on blur, so text typed but not confirmed isn't silently dropped when Save is clicked. Used by `Config.svelte` for `anilist_usernames` (with `required`) and `excluded_lists`. `required` adds the `*` in the label plus `aria-required` on the input — not the native `required` attribute, because what's mandatory is the *list*: with a chip already committed the input is empty, and native validation would reject a filled field |

**Pure domain logic** (`src/lib/domain/` — Fase 2, spec §7). No Svelte imports, no store reads, covered by vitest:

| File | Export | Purpose |
|------|--------|---------|
| `animeState.ts` | `deriveAnimeChip(anime, torrents, now, stalledSince)` | Ordered cascade → one chip state: blacklisted → downloading → no-seeds (≥10min at 0 peers) → paused → awaiting-premiere → up-to-date → behind. Returns a **key plus raw numbers**, never rendered text, so it stays locale-free; the screen translates the key |
| `episodeActions.ts` | `episodeActions(ep, torrent)` | The episode action model **as data** (`{ principal?, menu[] }`), so desktop and mobile render one definition instead of two copies. Covers only the five actions that exist |
| `passError.ts` | `passErrorMessage(code)` | Turns `CheckReport.pass_error_code` into the sentence shown when a verification pass aborted. Unknown codes fall back to a readable line — a backend newer than the frontend must not render a blank banner. The raw `pass_error` is shown collapsed next to it, never as the headline |
| `systemBanner.ts` | `pickBanner(backend, anilist)`, `secondsUntil` | Picks the ONE banner to show, by precedence: backend unreachable → backend 5xx → AniList outage → AniList rate limit. Never a stack — with the daemon down, the AniList state on screen is a dated reading. Also decides `reportable`, which is true only for our own bugs (backend 5xx, AniList 400). See decisions.md #66 |
| `format.ts` | `formatSpeed`, `formatSpeedParts`, `formatBytes`, `formatPercent`, `formatEta`, `formatDate` | Locale-aware formatting (pt-BR comma decimal / dot thousands). Takes `locale` as an argument rather than reading the store, which is the dependency this directory isn't allowed to have. `formatSpeedParts` returns number and unit separately for the Status hero, and `formatSpeed` re-joins it. Overlaps `utils/torrents.ts`/`utils/status.ts`; each call site migrates in the phase that redesigns its screen (Status already has) |
| `logSource.ts` | `logSourceFromCaller` | Maps zerolog's `caller` to a displayed origin (scheduler/anilist/rss/torrent/api). A deliberate approximation — a structured `component` field is backlog, not backend work for this redesign |
| `logLine.ts` | `parseLogLine`, `formatLogTime`, `countByLevel` | Parses a raw log line into the four columns of the Logs grid. Handles the three shapes the daemon emits, in order: zerolog JSON, zerolog console (`INF`/`WRN`/…), then a keyword fallback that never fails. Lifted out of `Logs.svelte` in Fase 7 for a concrete reason: `caller` used to be buried in the `extras` blob, and pulling it out as its own field is what makes the origin column possible |
| `priorityPresets.ts` | `PRESETS`, `applyPreset(list, first)` | One-click codec presets for the Priorities screen (compatibility = H.264 first, space = AV1/HEVC first). `applyPreset` **promotes** the tokens of `first` that the list already has instead of replacing it, so a typo is a no-op and a hand-added token moves down instead of disappearing — see [decisions.md #70](decisions.md) |
| `onboarding.ts` | `onboardingSteps(config, animes, status)`, `allDone(steps)` | The three first-run steps as booleans — library folder set, at least one anime source (AniList account **or** standalone anime), first check done. **Not** what the card's checkmarks show (those are ticked by hand, `stores/onboarding.ts`): `allDone` here is only the "this install is already up and running, never show the tutorial" gate, plus the `libraryConfigured` gate for the "+ Add anime" button. `firstCheck` rejects any year < 2010 because Go's `time.Time` zero value serializes to `0001-01-01T00:00:00Z`, a perfectly parseable date: testing only for the empty string would mark the step green on a daemon that never ran. `allDone` lives here too, so the render condition and the tests read the same rule |

**Stores** (`src/lib/stores/`):

| File | Export | Purpose |
|------|--------|---------|
| `wsState.ts` | `wsConnectionState` | Svelte store for the WebSocket connection state (`connected`/`reconnecting`/`disconnected`) |
| `backendHealth.ts` | `backendHealth`, `backendDegraded` | Health of **our own** backend, derived inside `apiRequest` from responses the app already makes — there is no health-check endpoint, and a daemon that answers `/status` is healthy by definition. Counts **consecutive** failures per endpoint (2 before alarming, so one dropped poll is not an alarm) and clears on the first success. 4xx never counts (404/405/409 are legitimate answers). Per-endpoint counters on purpose: a broken `/last-check` must not be erased by a healthy `/status` — that pair is exactly the blind spot `silent: true` created |
| `toasts.ts` | `toasts` | Toast queue store |
| `theme.ts` | `theme` | Dark/light theme |
| `locale.ts` | `locale` | i18n locale |
| `onboarding.ts` | `ONBOARDING_STEP_IDS`, `onboardingDismissed`, `onboardingDone` | The Status first-steps card: whether it was permanently dismissed (`localStorage` key `onboarding-dismissed`) and which steps the user ticked by hand (`onboarding-done`, a JSON array filtered against `ONBOARDING_STEP_IDS` so hand-edited storage can't hide the card). Same shape as `theme.ts`; per browser on purpose, **not** `config.json` fields — see decisions.md |
| `speedHistory.ts` | `speedHistory` | Last 20 aggregate download-speed samples for the Status hero `Sparkline`. Does **not** poll — the owning screen calls `push()` once per *successful* torrent poll. There is deliberately no `fail()`/`freeze()`: freezing is the *absence* of a push, so a failed poll can never be papered over with an extrapolated sample |
| `stallTracker.ts` | `stallTracker` | Per torrent hash, the timestamp since which `peers_total === 0`. Bookkeeping only (`sync()` per poll tick); the 10-minute "sem seeds" threshold is a chip rule and lives in `deriveAnimeChip`. In-memory, resets on reload |
| `activeTorrents.ts` | `activeTorrentCount` | Count of active torrents (`downloading`/`downloading_metadata`/`verifying`/`allocating`) for the NavRail/NavTabBar Downloads badge (Fase 1, spec §5). Custom `writable` start/stop function polls `GET /torrents` every 10s, paused while `document.visibilityState === 'hidden'`; a failed poll clears the count to 0 (ambient indicator, no toast). Separate from `Downloads.svelte`'s own 2s poll — consolidating the two is out of scope for Fase 1 |

**Utils** (`src/lib/utils/`, torrent-related additions):

| File | Export | Purpose |
|------|--------|---------|
| `torrentFilters.ts` | `ViewState`, `filterTorrents`, `sortTorrents`, `encodeViewState`, `decodeViewState`, `DEFAULT_VIEW_STATE`, `isProblemTorrent`, `groupKey`, `groupTorrents`, `applyFilterPreset`, `activeFilterPreset`, `DOWNLOADING_SLUGS`, `prioritizeOrder`, `selectionPrioritizeOrder` | Downloads screen search/status-filter/sort/grouping, and its round-trip to/from the URL querystring (`decodeViewState` is tolerant of garbage — unknown values fall back to defaults). `ViewState` also carries `problems` (the pill that can't be expressed as status slugs — see `isProblemTorrent`: actively transferring, not paused, not complete, zero peers) and `closed` (**collapsed** accordion groups, not open ones, so "everything expanded" serializes to an empty querystring). The four filter pills are presets over `statuses`/`problems`, so a pre-existing `?status=<slug>` deep link still filters exactly as before — it just lights up no pill. `prioritizeOrder` (group button: ascending `episode_number`, batches/numberless last) and `selectionPrioritizeOrder` (bulk button: the click order the selection `Set` preserves) decide the order sent to `POST /torrents/prioritize`, which applies it verbatim |
| `relativeTime.ts` | `formatRelative(targetMs, nowMs, locale)`, `nextAiringIn(nextAiringAt, nowMs, locale)` | `Intl.RelativeTimeFormat` for past **and** future off the same sign — `Status.svelte`'s "há X" and the "próximo episódio em X" of both screens. `Math.trunc` (not `round`) preserves the truncate-toward-zero the "há X" already had. `nextAiringIn` takes the API's unix **seconds** and returns `''` both when there's no scheduled episode and when the timestamp is already in the past (AniList is often late publishing the next one). Returns the time phrase only, never the full sentence — the screen interpolates it into `m.next_airing`, same i18n boundary as `domain/animeState.ts` |
| `torrentFiles.ts` | `filesByEpisode(files)` | Files that matched an episode number, one per number, sorted by number. Two files on the same number (`NCOP 01.mkv` matches the ` 05.mkv` pattern) resolve by **largest file wins**, so a 200 MB NCOP does not steal episode 1's bar. Used by `TorrentFiles.svelte` in `episodes` mode |
| `torrentsByEpisode.ts` | `indexTorrentsByEpisode(torrents, episodes)` | Joins torrents onto episodes by `episode.episode_hash === torrent.hash`; a batch torrent's hash appears under each episode it covers. Used by `AnimeDetail.svelte` |
| `torrentStatus.ts` | `STATUS_SLUGS`, `statusLabel`, `statusClass` | Shared status-badge label/class for a torrent's `status` slug (must be kept in sync by hand with `statusSlug()` in `src/internal/torrents/status.go`); used by `Downloads.svelte`, `DownloadsToolbar.svelte`, and `AnimeDetail.svelte`. `statusLabel` stays **pure and position-free** — the queue position is rendered by the Downloads row itself, because the same function feeds the toolbar's status filter, where "Queued #7" means nothing |

**API client** (`src/lib/api/client.ts`):

Exports typed fetch wrappers for every endpoint. Uses `window.location.origin` as base URL (works with reverse proxies). All errors surface via `toasts.add(message)` — except when the caller passes the 4th `ApiRequestOptions` argument `{ silent: true }`, which suppresses the toast (error is still logged and rethrown). `getTorrents()` always passes `silent: true`, since it's polled on a short interval (2-15s) by Downloads/Status/AnimeDetail and a transient failure must degrade silently rather than toast on every tick.

**WebSocket client** (`src/lib/websocket/client.ts`):

Connects to `/api/v1/ws`, updates `wsState` store on messages.

## Implemented Features (notable)

**Manual magnet paste**: `AnimeDetail.svelte` exposes a magnet input UI that calls `/api/v1/animes/{id}/episodes/{episodeNumber}/replace` (per-episode) or `/api/v1/animes/{id}/replace` (full anime/batch). Allows bypassing Nyaa search and adding any magnet link directly to the embedded torrent client.

**Multi-account Anilist**: `Config.AnilistUsernames []string` — the verification loop (`verification.go`) and `handleAnimes` (`endpoint_animes.go`) both iterate over every configured username. Episode tracking is not per-account; all accounts share the same `episodes.json`. See [Config Reference](config.md) for the legacy singular-field migration.

The same anime linked on multiple accounts appears once per account in the merged list, each with its own `MediaList.Id`/`Progress`/`Status` but the same `Media` (and the same airing-schedule episode IDs, which key every download/keep/delete decision). **`Media.Id` — not `MediaList.Id` — is therefore this app's anime identity**: it is what `episodes.json`, `anime_settings`, the `/animes/{id}/*` routes and the `anilist.co` link all use ([decisions.md #43](decisions.md)). Installations predating that are converted once by `daemon.MigrateAnimeIDsToMedia`, gated by the `anime_ids_are_media_ids` config flag; the verification pass aborts until it succeeds.

`anilist.DedupeByMedia(list)` collapses the per-account entries by `Media.Id`, keeping the **lowest** `Progress` (an episode is only "watched"/deletable once all accounts have seen it) — without it the account further ahead would delete episodes another account hasn't reached, and `GET /animes` would list the anime twice. The `media { id }` field is fetched by both `GetAllCurrentAnime` and `GetFrontendAnimeList` specifically for this key. The surviving entry's **`Status` must not be read** — it belongs to one arbitrary account. Status is resolved per account instead:

- **Download — OR**: one account having the anime in a `DownloadStatuses` status is enough. Falls out of the union of the per-account fetches in `searchAnilist` (each is filtered server-side by `status_in`).
- **Deletion — AND** (`deletableMediaIDs` / `allAccountsAgreeOnDelete`, `daemon/verification.go`): every account that *has* the anime must have it in some `DeleteStatuses` status; the statuses need not match (`DROPPED` in one and `COMPLETED` in another still deletes). An account that doesn't track the anime doesn't vote; one holding it in a neutral status (`PLANNING`) **vetoes**, as does an account whose list fetch failed. Telling "doesn't track it" from "tracks it in a neutral status" costs one `GetMediaListStatus` call per account that didn't report the anime — only for animes **with episodes on disk** that some account wants deleted, a set that empties itself as those episodes get removed.

**Media-status filter** {#media-status-filter}: `Config.DownloadStatuses` filters by *list* status (`MediaListStatus` — the user's relationship to the anime, e.g. `CURRENT`); `Config.DownloadMediaStatuses` filters by *media* status (`MediaStatus` — the anime's own airing state, e.g. `RELEASING`). The former is applied server-side by AniList (`status_in` in the GraphQL query); the latter can't share that filter, so both consumers apply it client-side per anime via `anilist.MediaStatusAllowed`:

- **Download pipeline**: `searchAnilist` (`daemon/verification.go`) applies it right after the per-account fetch, before dedup — an anime whose media status isn't allowed is simply never a download candidate.
- **Frontend listing**: `fetchAniListEntries` (`api/endpoint_animes.go`) applies it (alongside the server-side list-status filter) to decide which *not-yet-downloaded* animes get merged into `GET /animes`. Critically, this filtering only gates *new* entries — an anime that already has a downloaded episode is never removed from the listing by either filter, no matter what its current status is. If such an anime's status falls outside both allowed sets (so it wasn't covered by the filtered fetch), `refreshOrphanAnimes` fetches it individually via `anilist.GetAnimeInfo` to keep its cover image/progress/blacklist fields fresh; a failed individual fetch just leaves the anime with its `episodes.json`-derived fields (name, total episodes, last download date) and blank AniList-derived ones, rather than disappearing or failing the request.

See [Config Reference](config.md).

