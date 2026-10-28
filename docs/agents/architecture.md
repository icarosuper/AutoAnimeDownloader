# Architecture

> **How to use this file.** It answers *"which file do I open and what bites me there"* — flow
> between files, invariants that hurt to break, negative facts, ambiguous names and the pointer to
> the decision. It does **not** list symbols with a one-line purpose: for "which functions exist in
> this package", `grep '^func ' <file>.go` answers for free and always up to date. Read the
> component map and the flow, then jump straight to the section of the package you'll touch.

## Components

```
src/cmd/daemon/      → Daemon binary entry point (embeds frontend build, starts API)
src/cmd/cli/         → CLI binary (same package `main`, split by resource) talking to the daemon REST API
src/internal/
  api/               → HTTP server, REST handlers, WebSocket, middleware
  daemon/            → Verification loop: Anilist → Nyaa → embedded torrent client → track episodes
  files/             → Config, episode tracking (JSON files), and library hardlinking (Librarian)
  anilist/           → GraphQL client for Anilist API
  nyaa/              → HTML scraper for Nyaa torrent site
  torrents/          → Embedded BitTorrent client (github.com/cenkalti/rain/v2) behind a TorrentBackend interface
  frontend/          → Svelte 5 + Vite + Tailwind 3 web UI (compiled to Go embed). No
                       component library: the primitives live in components/ui/ and the
                       colors in lib/design/tokens.css (decisions.md #91)
  notifications/     → Webhook template interpolation and HTTP firing. Called by daemon on NewEpisode/DownloadFailed; by job queue on DownloadCompleted.
  logger/            → zerolog-based structured logger (console + rotating file)
  datadir/           → The data dir path (~/.autoAnimeDownloader, %APPDATA%\.autoAnimeDownloader) — leaf package
  diagnostics/       → Bug report bundle (redacted environment, config, log) and the crash file lifecycle
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
| `blocked_episodes` | `~/.autoAnimeDownloader/` | Episodes to skip (JSON array of `{anime_id, episode}`, no extension). The old format (an array of AniList node ids) is discarded with a warning when read — see decisions.md #52 |
| `anime_settings` | `~/.autoAnimeDownloader/` | Per-anime settings keyed by AniList **media** ID (JSON map, no extension) |
| `standalone_animes` | `~/.autoAnimeDownloader/` | Media IDs tracked **without** being in any AniList list (JSON array of IDs, no extension) |
| `anilist_cache` | `~/.autoAnimeDownloader/` | Snapshot of the `anilist` caches (MAL lists and the MAL→AniList id mapping included), so the app survives AniList being down (JSON, no extension). **Opaque** bytes to `files` on purpose: the schema belongs to `anilist` (`persist.go`), and that's what keeps `files` from importing `anilist` just to persist its cache. Disposable — deleting it only costs an outage with no safety net until the next fetch |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |
| `pending_jobs.json` | `~/.autoAnimeDownloader/` | Persisted job queue (`organize` jobs) |
| `session.db` | `~/.autoAnimeDownloader/` | rain resume database (bbolt) — piece bitfields, kept **outside** the download path so it survives a library path change |
| `queue.json` | `~/.autoAnimeDownloader/` | Download queue state: `{"order": [...], "paused": [...]}`. Lives next to the resume database because it is torrent-client state, not user config. Missing/corrupted = rebuilt from `AddedAt`, never fatal — see decisions.md #41 |
| `crash.log` | `~/.autoAnimeDownloader/` | Where the Go runtime writes a fatal panic (`debug.SetCrashOutput`, set in `main.go` before `logger.Init`). Empty on a clean run |
| `crash-pending.log` | `~/.autoAnimeDownloader/` | The previous run's crash, promoted from `crash.log` at boot. Its existence is `pending_crash_at` in `/status` |
| `crashes/` | `~/.autoAnimeDownloader/` | Handled crashes (`crash-<yyyymmdd-hhmmss>.log`, named from the crash's mtime), newest 5 kept |
| `download_root.id` | `~/.autoAnimeDownloader/` | Id of the download folder the session is bound to. Its twin, `.aad_root`, lives **inside** the download folder; the pair is how a moved/trashed/replaced folder is detected — see decisions.md #34 |

Windows uses `%APPDATA%\.autoAnimeDownloader\` for **all** the config/state files above (note the leading dot — same folder name as on Linux). The path is computed in **one** place, `datadir.Path()` — every file above (and the logger, the CLI's PID file and the crash files) goes through it. It's a leaf package on purpose: `files` imports `logger`, so the function can't live in `files`. There is no dotless `%APPDATA%\AutoAnimeDownloader\` variant.

## On-Disk Layout

- **Download / seeding:** torrents live at `<Config.DownloadPath()>/<torrent-id>/...`, i.e. `<completed_anime_path>/.torrents/<torrent-id>/...` (rain's `DataDir` with `DataDirIncludesTorrentID`). Files are **never renamed here** — renaming would break seeding. Torrents keep seeding after completion. The download directory is **derived**, not user-configured — see decisions.md #31.
- **Library (Jellyfin):** when a torrent completes, its video files are **hardlinked** into `<completed_anime_path>/<AnimeName>/` — one folder **per AniList entry**, season/cour marker kept (`sanitizeName`, decisions.md #45), plus a `tvshow.nfo` carrying the AniList id. With `RenameFilesForJellyfin`, every file gets the Jellyfin name `"Anime Name - E05.mkv"` — a single episode from its record, a batch file from the episode number parsed out of its own filename (`nyaa.ExtractEpisodeNumber`); files with no readable number (NCOP/NCED, extras, movies) and two files of one pack landing on the same number keep the raw filename. The hardlink shares bytes with the seeded copy, so no space is duplicated.
- **Same volume, by construction:** the download directory lives inside `completed_anime_path`, so the old cross-filesystem failure mode is now structurally impossible. `Librarian.ProbePath(completedPath)` still validates that the filesystem supports hardlinks at all (exFAT/FAT32/some SMB shares don't) — it runs on config save and on every verification pass (decisions.md #26).
- **Deletion** frees space by removing **both** links: the library hardlink (`Librarian.RemoveFromLibrary`, which also drops the sibling `.nfo`) and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). After those hardlinks, a library folder with no remaining video is wiped (`RemoveFolderIfNoVideo`). The end-of-pass sweep (`DeleteEmptyFolders`) then removes empty folders and folders left with only Jellyfin leftover metadata (`.nfo` files and `.chapters`) and still skips `.torrents` ([decisions.md #106](decisions.md#106-the-sweep-stays-nfo-only-the-contextual-wipe-is-no-video), [decisions.md #110](decisions.md#110-chapters-is-leftover-metadata-same-class-as-nfo)). A batch torrent shared by multiple episodes is only removed once **all** its episodes are deleted **and** every episode the pack covers is claimed by some entry in the group (`canRemoveTorrent` / `hasUnclaimedContent`, decisions.md #74). While it is held back, its records are kept too (`forgettableKeys`) — they carry the declared range. The manual delete path passes `force` and bypasses that guard.
- **No `config.json` versioning.** There is no `config_version` field, no migration chain and no
  startup block: `LoadConfigs` unmarshals **over** `getDefaultConfig()`, so an added or removed field
  needs no migration at all, and the two legacy renames are detected by **shape**, in `LoadConfigs`
  only — `PUT /config` does not repeat them. See decisions.md #90.
- **No `save_path` migration.** `daemon.MigrateSavePath` used to move a legacy `save_path`'s torrent folders into the derived download path at boot and at the top of every pass; it was removed along with the `Config.SavePath` field. An old `config.json` still carrying the key loads (JSON ignores unknown fields) but its data is **not** moved — moving it is a manual pre-upgrade step. See decisions.md #31.

## API

The table below is the **only** endpoint doc: there is no OpenAPI spec or `/swagger/` anymore
(removed — they were 33% of the binary and nothing consumed the spec).

Key endpoints:

| Method | Endpoint | Handler func | File |
|--------|----------|-------------|------|
| `GET` | `/api/v1/status` | `handleStatus` | `endpoint_status.go` — the `disk_low` threshold lives **only** on the server. It is the add-gate for a torrent of size 0: `free - reserved` against `min_free_disk_gb` (`daemon.DiskLow`), so the Status banner cannot disagree with what the daemon will refuse ([decisions.md #93](decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_gb--0)). The AniList state is written by **any** call (a pass or a frontend poll), because they all go through `sendAnilistRequest` — decisions.md #65/#66. `anilist.cache_saved_at` is filled in on **read**, not when the state is written: it's not a fact about AniList, it's what the banner uses to say how old the cache on screen is. `pending_crash_at` (mtime of `crash-pending.log`, `null` when there is none) is what makes the web UI offer the report screen after a crash |
| `GET` | `/api/v1/last-check` | `handleLastCheck` | `endpoint_last_check.go` — memory only. `problems` (should have downloaded and did not) and `limits` (the config doing what it was configured to do). When `pass_error` is set both lists come empty, because `SetLastCheckError` clears the report (decisions.md #82). A manual download stays out: that path returns the error on its own HTTP response |
| `GET/PUT` | `/api/v1/config` | `handleConfig` | `endpoint_config.go` |
| `GET` | `/api/v1/config/priorities/defaults` | `handlePriorityDefaults` | `endpoint_priorities.go` |
| `GET` | `/api/v1/animes` | `handleAnimes` | `endpoint_animes.go` — `next_airing_at` is a **timestamp**, not a countdown: the response is cached 60s and a precomputed counter would freeze. `media_status` is AniList media status, copied at merge; the dashboard type filter is client-side (`lib/utils/status.ts`). Search by `alt_names` is client-side. `episodes_downloaded` is **files on disk right now**, not a lifetime total: it DROPS when the daemon prunes watched episodes, so `released - downloaded` is not "still to download" — `episodes_pending` is ([decisions.md #88](decisions.md#88-the-progress-bar-derives-downloaded-from-episodes_pending-not-from-episodes_downloaded)) |
| `GET` | `/api/v1/anilist/search?q=&include_unreleased=` | `handleAniListSearch` | `endpoint_anilist_search.go` — a term shorter than 3 chars returns an empty list, **not** 400. `block_reason` per result (`""` = addable) |
| `POST` | `/api/v1/standalone-animes` | `handleStandaloneAnimeAdd` | `endpoint_standalone_animes.go` — body `{"media_id":21,"magnet":"…"}` with `magnet` optional; optional `name`/`nyaa_id`/`size_bytes` for coverage and the disk guard. No magnet: auto-pick. Magnet: 409s first, then `DownloadStandaloneAnimeWithMagnet` adds the torrent then writes the id (not the handler); records only covered aired episodes. 400 `INVALID_MAGNET` if the value lacks the `magnet:` prefix. Answers `{"added": N}`; 409 `LIBRARY_NOT_CONFIGURED` / `ALREADY_BLACKLISTED` / `ALREADY_STANDALONE` / `ALREADY_TRACKED` / `ALREADY_DOWNLOADED` |
| `DELETE` | `/api/v1/standalone-animes/{id}?delete_episodes=<bool>` | `handleStandaloneAnimeRemove` | `endpoint_standalone_animes.go` — without `delete_episodes` the episodes stay and are marked `ManuallyManaged` |
| `GET/PUT` | `/api/v1/animes/{id}/settings` | `handleAnimeSettings` | `endpoint_anime_settings.go` |
| `GET` | `/api/v1/animes/{id}/episodes` | `handleAnimeEpisodes` | `endpoint_anime_episodes.go` |
| `GET` | `/api/v1/animes/{id}/torrent-candidates?q=` | `handleTorrentCandidates` | `endpoint_torrent_candidates.go` — ranked whole-title list, cap 8. Empty `items` is 200 (`[]`, not null). `{id}` need not be tracked. `q` skips title variants and season/part. Does not fetch `/view/<id>`. 502 `NYAA_UNAVAILABLE`. 404 `ANIME_NOT_FOUND` |
| `GET` | `/api/v1/animes/{id}/torrent-candidates/{nyaaId}` | `handleTorrentCandidateDetail` | sanitised Nyaa page + file list. `{nyaaId}` digits only. `{id}` unused for the fetch (still validated). `files` is `[]` not null. 404 `TORRENT_NOT_FOUND` |
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
| `GET` | `/api/v1/diagnostics?titles=<bool>&error=&context=&anime=` | `handleDiagnostics` | `endpoint_diagnostics.go` — `{bundle, environment}`. `titles` defaults to on; `error`/`context`/`anime` are the error toast's, echoed into the bundle's Context section after redaction |
| `DELETE` | `/api/v1/diagnostics/crash` | `handleDiagnosticsCrash` | `endpoint_diagnostics.go` — moves `crash-pending.log` to `crashes/`. Idempotent; answers the usual 200 envelope, not 204, because `apiRequest` always parses the body |
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

`RunAnimeDebug` mirrors `processAnimeEpisodes`: episode selection (`anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`), the pack search/selection (`partitionSearchResults` + `pickBatches`) that lifts the per-anime limit when it resolves, and the per-episode `searchSingleEpisode` fallback when no batch/movie/single magnet was found — so the debug pass reports the pipeline the daemon actually runs (see [decisions.md #58](decisions.md#58-runanimedebug-mirrors-processanimeepisodes-enumeration-pack-resolution-and-single-fallback)). The one deliberate difference: it searches every uncovered episode, without the miss backoff of [decisions.md #100](decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes).

`scripts/robustness-animes.txt` (curated media IDs, one per line) + `scripts/debug-batch.sh` (`make debug-batch`) run that pass over the whole list and write a triaged `.debug_batch/report.md`.

---

## File map — what lives where and what bites

### `src/internal/daemon/` — the pass

**There is no `daemon.go`.** The orchestrator is split: `loop.go` (the goroutine), `verification.go`
(the pass), `episodes.go` (per-episode selection and removal), `search.go` (Nyaa search + filters),
`coverage.go` (ownership by coverage), `helpers.go` (config/disk guards), `stalled.go` (torrent
with no peers), `manual_download.go` (downloads coming from the API), `standalone.go` and
`preview.go`.

**One pass at a time**: `AnimeVerification` takes `verificationMu.TryLock()` and returns immediately
if another is already running (decisions.md #67).

**Download priority order** (in `processAnimeEpisodes`) — the project's central flow:

0. **Ownership by coverage** (`adoptCoveredEpisodes`, before any search): an episode already contained
   in a pack on disk, under **any** `anime_id` of the same series, becomes a record pointing at that
   hash — no download and no Nyaa (decisions.md #78)
1. **Movie** → `resolveMovie` → `skipSubfolder=true`
2. **Packs**, resolved before the episode loop, covering the window from the first pending episode
   → `partitionSearchResults` + `pickBatches`/`assignBatches`. Eligibility comes from the search
   **result** (size, seeders, covered range), never from the anime's metadata
3. **Single episode**, per still-uncovered episode. An episode can skip this search without
   any Nyaa line in the log: after an empty search it waits 1h→24h before the next one, and the
   episodes after the first miss of the pass skip it too (`searchMisses`, in memory,
   [decisions.md #100](decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes)).
   The manual download (`DownloadStandaloneAnime`) and `RunAnimeDebug` don't apply it.

All three go through the `min_seeders` floor (`filterSearchResults`).

**Invariants that hurt to break:**

- `DownloadStandaloneAnime` **must never** call `handleSavedEpisodes`. With only one anime's
  episodes in hand and `delete_watched_episodes` on, `identifyEpisodesNotInWatching` would delete
  the rest of the library ([decisions.md #49](decisions.md#49-standalone-anime-tracked-by-the-list-is-the-snapshot-the-daemon-processes-and-downloadstandaloneanime-never-calls-handlesavedepisodes))
- `DownloadStandaloneAnimeWithMagnet` adds the torrent, then the standalone id, then coverage
  records — inverting that leaves a tracked id with no torrent
  ([decisions.md #108](decisions.md#108-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)).
  An unknown pack range writes **no** episode records, not `1..total` (decisions.md #84). It also
  must never call `handleSavedEpisodes`.
- `max_episodes_per_anime` is lifted **only after** a pack was actually chosen; without a pack, the
  limited selection stands and the oldest episodes are the ones that stay. `selectEpisodes` runs
  twice in the pass, and `dropAdopted` must run at **both** points — the function is pure, the
  second pass would bring the adopted ones back
- `firstEpisodeToConsider` walks back to the smallest already-saved episode. Without that walk-back,
  a save below progress is not "checked" and the prune would delete it, ignoring
  `watched_episodes_to_keep`
- A pack range that neither the name nor the file list resolved is recorded as **zero**, not as
  `1..total`: both sources failed and "I don't know" is the only honest record (decisions.md #84)
- The "unknown range" sentinel is the **end** (`BatchEnd`). A start of 0 or negative is a legitimate
  range ("00 ~ 12", or a season pack recorded under a later cour)
- `checkDiskSpace` bars a download, **never** the verification pass. A `statfs` error does not
  block. It receives the torrent size and is called **per candidate**: the second in the list may
  fit where the first did not, and the size trigger still applies with the GiB floor off
  ([#93](decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_gb--0)).
  In the pass it goes through a per-pass `diskBudget` (`diskbudget.go`): the size is checked on top
  of what the session's incomplete torrents and this pass's adds still have to write
  ([#101](decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space)).
  A pack that passes `packsThatFit` and then loses at `reserve` is dropped and the choice runs
  again ([#111](decisions.md#111-a-pack-that-loses-at-reserve-does-not-keep-the-window))
- The pack size ceiling is applied **twice**: in the Nyaa search and again in the daemon filter
  (decisions.md #80). Changing only one of the two does not change the behavior

**Network cost.** `packSet` resolves a pack's range **on demand** — from the name when the name
carries it, otherwise from the detail page — and memos **including failure**; without that,
`assignBatches`, which walks the packs once per episode, would become one request per episode. Cap
of `maxPackDetailFetches` (3) detail pages per anime per pass. See [decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything).

**`preview.go`** — `PreviewWholeTitleTorrents` is the picker list: the same search + filter +
partition as the pass, capped at 8, **without** adding a torrent, writing a standalone id, or
fetching `/view/<id>`. Range comes from the name (`ExtractBatchInfo`); unknown stays 0. A custom `q`
skips title variants and season/part. Empty list is not an error; Nyaa transport failure wraps
`ErrNyaaUnavailable`. The pass helper `searchNyaaWithVariants` still swallows total failure — the
preview has its own loop so "Nyaa down" is not an empty list. The picker **does not persist**: this
function is request-scoped; candidates live in a frontend memory cache for one navigation; a reload
of `#/add/:id/torrents` fetches again; custom `q` is not saved
([decisions.md #108](decisions.md#108-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)).

**Numbering axes.** `packAxis` are the hypotheses for how much to add to the entry's **local**
number: relative (0, always first), continuous across the season (`ComputeEpisodeOffset` — the gate
requires `part >= 2` and PREQUEL `TV`/`TV_SHORT`, decisions.md #9) and absolute across the series
(`anilist.Series.Offset`). The zero-value = relative only, the old behavior (decisions.md #79).
Among the hypotheses that cover the episode, the winner is the one that makes the pack **end** on
the entry's last episode — tie-break by count, see `sources.md`.

**`coverage.go`** — the unit of ownership stops being `files.EpisodeKey{AnimeID, Episode}` and
becomes the range the torrent covers on the **series' absolute axis** (decisions.md #77). That is
what stops cour 2 from re-downloading from Nyaa the season pack that is already on disk under
cour 1. The daemon seeds the index (`resolveSeriesIndex`): the ids of the pass **plus** the
`anime_id`s from `episodes.json` — the previous cour, already `COMPLETED`, left the pass universe,
and it is its offset that converts the range (decisions.md #78). A resolve error is logged, it
does not abort.

**Reconciliation** (`reconcileLibrary`, `verification.go`) — on boot and periodically, enqueues an
`organize` job for every completed torrent whose episode is not yet in the library. The criterion is
empty `LibraryPaths`, not the hardlink missing from disk (decisions.md #29); `clearLibraryPathsAfterRootSwap`
is the only exception, when `Ensure` reports `RootSwapped` (decisions.md #34).

**`stalled.go`** — called at the top of the pass, right after `backend.List()`. A torrent at zero
peers for more than an hour **leaves the session** through the same `HandleTorrentFailure` as a
torrent rain stopped with an error: no new path, and the next pass searches Nyaa again
([#94](decisions.md#94-a-stalled-torrent-is-dropped-through-the-failure-path-and-only-one-that-is-trying-counts-as-stalled)).
The clock is package-level (`stalledTorrents`), in memory, and is **not** the frontend's
`stallTracker.ts` — that one paints the "no seeds" chip at 10 minutes and does not talk to the
daemon. The snapshot passed into the function is not rebuilt after the drop: whoever left is
searched again only on the next pass.

**`webui.go`** — what the process calls a "port" is actually the `http.Server` **Addr** (`":8091"`).
`WebUIURL` strips the leading colon: interpolating the raw value produces `http://localhost::8091`,
an invalid host. The conversion used to be written in three places and the newest copy forgot the
strip. `WaitForListener` exists because `apiServer.Start()` runs in a goroutine and logs "API
server started" **before** the socket is bound — whoever opens the browser right after loses the
race and gets `ERR_CONNECTION_REFUSED`.

**`jobs.go`** — a persisted job queue, to decouple hardlink-into-library from the verification
loop. A single idempotent job, `organize` (replaced the old poll-based `rename_file` /
`move_to_completed` / `notify_on_complete`). 5s tick, backoff `30s * 2^(n-1)` up to 10min, max 20
attempts, persisted in `pending_jobs.json`. Idempotence: an episode with `LibraryPaths` already
filled counts as done — no re-link and no re-firing the webhook — so the completion event and the
reconciliation pass can both enqueue without fear. Created in `main.go` before `NewServer`, wired
to the backend/librarian by `SetOrchestration`.

**`migration.go`** — runs on boot **and** at the top of every pass. `verification.go` reloads the
config right after calling it, because the migration persists a changed config that the rest of the
pass needs to see. The `save_path` migration that lived here was removed along with the field
(decisions.md #31).

**`report.go`** — detail fields are flattened with `omitempty`, **never** a `map[string]any`: that
does not generate a usable TS type. The precedence cascade of the three search problems is in
decisions.md #81.

**`state.go`** — `SetLastCheckError` **clears** `lastCheckReport`. Consequence: `SetLastCheckReport`
must be called after the pass-end `SetLastCheckError(nil)` (decisions.md #82).
`GetLastCheckReport` returns a **value**, so the handler can fill `pass_error` without writing to
the shared object.

**`passerror.go`** — the backend sends a **code**, the frontend builds the phrase (the same
boundary as `lib/domain/checkIssue.ts`); before that the banner dumped raw `error.Error()`,
sometimes AniList JSON. The code travels **inside** the error, wrapped at the abort site — not as a
new `SetLastCheckError` parameter. A new site that forgets to wrap falls into `PassErrUnknown`,
which has its own phrase: it degrades, it does not break.

**`debug.go`** — mirrors `processAnimeEpisodes` (enumeration, pack resolution, single fallback) so
the debug pass reports the pipeline production actually runs
([decisions.md #58](decisions.md#58-runanimedebug-mirrors-processanimeepisodes-enumeration-pack-resolution-and-single-fallback)).
No torrent backend and no `episodes.json`; output goes to `.debug_<animeId>_<N>/` in the caller's
cwd, **not** in `~/.autoAnimeDownloader`.

### `src/internal/files/` — persistence

**There is no `save_path`.** The download directory is derived by `Config.DownloadPath()`
(`<completed_anime_path>/.torrents`), computed on every call (decisions.md #31).

**`EpisodeKey{AnimeID, Episode}` is an episode's identity** across the app — episode file,
blocked list, API routes.

**Trap:** `SaveEpisodesToFile` only **appends** the new ones and **silently discards** an update to
an existing id (decision 27). To rewrite a record — for example to write `LibraryPaths` after
organizing — use `UpsertEpisodes`.

**`FileManagerInterface` is declared twice**, in `api/server.go` and `daemon/helpers.go`. A new
method on `files.FileManager` → update **both** (see `conventions.md`).

**`mkv.go`** — `VideoCodec` reads the codec from the file **header**, not from the name:
`nyaa.ExtractCodec` reads from the release name and a large share of uploads never write it
(Erai-raws does not; a lot of BD rips neither). Matroska is EBML (RFC 8794), and the codec sits on
a short, fixed path **before** the Clusters (`Segment → Tracks → TrackEntry → {TrackType, CodecID}`),
so the read costs a few KB and a few seeks, never the whole file. `TrackType` is checked because an
anime `.mkv` carries video, audio and several subtitles — blindly taking the first `TrackEntry`
would report the audio codec. `CodecID`s are mapped to the **same vocabulary as `nyaa.ExtractCodec`**
(`HEVC`, `H.264`, `AV1`), otherwise the panel and the priorities screen would name codecs
differently. Returns `""` — **never** an error — for every unanswered case. Matroska only: MP4 would
have an analogous structure but would need a second parser, and anime in practice is `.mkv`.

**`librarian.go`** — hardlink of completed videos into `<completed_anime_path>/<AnimeName>/`, one
folder **per AniList entry**, with the season/cour marker preserved (decisions.md #45) and a
`tvshow.nfo` carrying the AniList id (so the Jellyfin plugin matches by id, not by folder name).
On the Jellyfin rename, a pack file's number comes from the file's own name minus
`packEpisodeOffset` — a season ≥ 2 pack numbered continuously becomes the entry's numbering
(decisions.md #68). With no readable number (NCOP/NCED, extras, movies) or on a name collision
inside the pack, it keeps the raw name; two files with the same basename (per-season subfolder,
flattened) use the relative path so one does not overwrite the other. `Organize` is idempotent.
`ProbePath` validates that the filesystem supports hardlinks (exFAT/FAT32/some SMB do not) and runs
on config save and on every pass (decisions.md #26, #31). `BackfillShowNFOs` is **not** on the
`Librarian` interface — `main.go` holds the concrete `*organizer` — and only runs when
`MigrateAnimeIDsToMedia` succeeded.

**`parser.go`** — JSONL, one object per line. The legacy text format (`id:hash:name`) was removed
along with `EpisodeID`: with no anime id and no episode number, that record has no key
(decisions.md #52).

**`filesystem.go`** — the `FileSystem` interface exists for tests (`MockFileSystem`); it includes
`Link`, which is what the `Librarian` uses.

### `src/cmd/cli/` — daemon CLI

Split by resource, same package `main` (`main_test.go` keeps testing unexported helpers): `main.go`
holds the app, global flags, `start` / `stop` / `open` / `loop`; `args.go` reorders flags that
follow positionals before urfave parses ([decisions.md #109](decisions.md#109-flags-after-positionals-are-reordered-before-urfave-parses))
— tests must call `run`, not `newApp().Run`; `config.go`, `status.go`,
`animes.go`, `torrents.go`, `logs.go`, `report.go`, `notifications.go` hold the matching trees.
There is no root `episodes` command and no `GetEpisodes` — `GET /api/v1/episodes` is gone; listing
is `animes episodes <id>`. There is no command that prints a Nyaa description — `animes candidates`
plus `nyaa_id` is the terminal equivalent.

`report` and `report crash-ack` call `diagnostics.Build` / `ArchivePendingCrash` against
`datadir.Path()` and never the HTTP client, so they work with the daemon dead. `--yes` is a CLI
gate: the API has no confirmation parameter ([decisions.md #104](decisions.md#104---yes-is-a-cli-gate-the-api-keeps-its-query-defaults)).

### `src/internal/api/` — HTTP

Handler pattern, response envelope and the new-endpoint checklist live in
[conventions.md](conventions.md), not here.

**`client.go` is the HTTP client _for_ the daemon API, used by the CLI binary** (`cmd/cli`) — the
daemon does not use it. Confusing the two leads to looking for business logic in it. There is no
`GetEpisodes`.

**`endpoint_animes.go`** — the two filters (`DownloadStatuses` server-side, `DownloadMediaStatuses`
client-side) only bar **new** entries: an anime that already has a downloaded episode never leaves
the list, whatever its current status. For those orphans, `refreshOrphanAnimes` reloads
cover/progress/blacklist in at most two batch queries; failure only blanks the AniList-sourced
fields, instead of dropping the anime or failing the request. `countPendingEpisodes` counts by
**episode number**, not `released - downloaded`, because a watched episode can still be saved
(`watched_episodes_to_keep`).

**`endpoint_anime_episodes.go`** — `AnimeEpisodeInfo` **does not carry an episode name**: AniList
does not expose a per-episode title (`AiringNode` only has number and time) — decisions.md #52.
`animeId` is the **media** id, the same one as the `anilist.co/anime/{id}` link; there is no
separate `anilistId` field.

**Vocabulary — never call a standalone anime "manual".** Standalone = tracked without being on any
AniList list. "Manual" already means something else: the user touched that episode by hand
(`ManuallyManaged`, `ManualDownloadEpisode`). The files are `standalone.go`, `standalone_guard.go`,
`endpoint_standalone_animes.go` and `endpoint_anilist_search.go`. `standaloneGuard` is **one
blocking rule with two consumers** (the `POST` and the search handler), so "the front does not let
you click" and "the back returns an error" agree by construction.

**`endpoint_torrent_candidates.go`** — the picker **does not persist**. Candidates live in a
frontend memory cache for one navigation; a reload of `#/add/:id/torrents` fetches again. Custom
`q` is not saved. The daemon has no picker session
([decisions.md #108](decisions.md#108-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)).

**`endpoint_torrents.go`** — a file's `path` is the **raw** path from the metadata and is never
rewritten to "Ep NN": the point of the panel is reading fansub/resolution/codec off it. `Progress`
falls back to the piece ratio (`PiecesHave/PiecesTotal`) when `BytesCompleted` reads 0 with a
non-zero total. `DELETE` is registered on the **same** mux pattern as pause/resume/announce — a
Go 1.22+ pattern with no method prefix matches every verb — so the method check inside the handler
is what separates the two.

### `src/internal/anilist/`

**`EpisodeList` is the only source of "which episodes exist".** It synthesizes the list, using the
real `airingSchedule` node when one exists, because AniList only keeps a window of the schedule.
`AiringNode.ID` is the schedule-node id and **does not identify** an episode.

**Identity: `Media.Id`, never `MediaList.Id`.** The same anime on several accounts appears once per
account, each with its own `MediaList.Id`/`Progress`/`Status`, but the same `Media`. `Media.Id` is
what `episodes.json`, `anime_settings`, the `/animes/{id}/*` routes and the AniList link use
([decisions.md #43](decisions.md#43-an-animes-identity-is-mediaid-status-is-a-per-account-question-download--or-deletion--and)).
Older installs are converted by `MigrateAnimeIDsToMedia`, with the pass aborting until it succeeds.

**`DedupeByMedia` collapses entries by `Media.Id` and keeps the **smallest** `Progress`** — an
episode is only "watched"/deletable when every account has seen it. The surviving entry's `Status`
**must not be read**: it belongs to an arbitrary account. Status is per account:

- **Download — OR**: one account having the anime in a `DownloadStatuses` status is enough
- **Deletion — AND** (`allAccountsAgreeOnDelete`): every account that *has* the anime needs it in
  some `DeleteStatuses` status (they need not be the same). An account that does not track it does
  not vote; an account in a neutral status (`PLANNING`) **vetoes**, and so does an account whose
  list failed to fetch

<a id="media-status-filter"></a>**Two status filters, similar names, different things**:
`Config.DownloadStatuses` filters by **list** status (`MediaListStatus` — the user's relationship with
the anime, e.g. `CURRENT`) and is applied server-side by AniList (`status_in`). `Config.DownloadMediaStatuses`
filters by **media** status (`MediaStatus` — the anime's airing state, e.g. `RELEASING`); AniList
doesn't accept that one in the same filter, so both consumers apply it client-side via
`anilist.MediaStatusAllowed`: `searchAnilist` (`daemon/verification.go`, the download pipeline, right
after the per-account fetch and before the dedup) and `fetchAniListEntries` (`api/endpoint_animes.go`,
the frontend listing). **Neither filter hides an anime that already has a downloaded episode**,
whatever its current status — see `endpoint_animes.go` above.

**Every query declares a `Priority`, and there is no default on purpose**: a new query does not
compile until someone decides the criticality. The budget gate (`health.go`) refuses
`PriorityDisposable` with `ErrBudgetLow` when AniList quota is low — nothing goes out, and the
package health does not change (decisions.md #72). The limit is 30 req/min per IP.

**Health is exposed as a code, never a ready-made phrase** — the daemon does not know the browser
locale (same reason as `daemon/passerror.go`). It is written by **every** response that goes through
`sendAnilistRequest`, and AniList returns 200 with an error in the body, so the envelope is read
too (decisions.md #65/#66).

**Caches** (`ttlCache[T]`, all in memory; the ones below also go to an on-disk snapshot, see
`persist.go`): `GetFrontendAnimeList` 60s per `username+statuses` — the `/api/v1/animes` poll, 30s
per open tab, is what used to blow AniList's quota (decisions.md #46); `customLists` 5min
(decisions.md #11); `mediaByIDCache` 60s per media id (standalones); `seriesCache` 24h; `searchCache`
10min per `lowercase-term+includeUnreleased` — **the only one with a free-text key**, which is why
it is the only one with a cap (`searchCacheMaxEntries`, total drop, not LRU) and the only one
`ttlCache.size()` serves. `passListCache` and `mediaEntryCache` are **fallback only**: written with
TTL zero, never consulted by `get`.

**Three list fetches with similar names, and the fields are NOT the same** — this is the
disambiguation that bites most here:

| function | who calls | has `relations`? | has entry `status`? | has `coverImage`? |
|---|---|---|---|---|
| `GetFrontendAnimeList` | `/animes` poll | ✗ | ✗ | ✓ |
| `GetAllCurrentAnime` | daemon pass | ✓ | ✓ | ✗ |
| `getMediaListEntry` (`mediaByIDFields`) | `/animes/{id}/*`, manual download | ✓ | ✓ | ✓ |

That is why the detail screen **must not** be served from the frontend cache: it would lack
`relations`, and that is where the manual-download Nyaa search comes from. It is also why there are
three `MediaList` caches instead of one (decisions.md #92).

**`persist.go` — the on-disk snapshot (`anilist_cache`, in the config folder).** It exists for one
thing: screens and the pass keep standing with AniList down, restart included. **The invariant that
hurts to break:** `MediaList` collections come back from disk with **TTL zero** (`restore(..., 0)`),
so only `getStale` sees them and only the failure path serves them — restoring with a live TTL
would make every screen show yesterday's list without even trying the network. `seriesCache` is the
exception (immutable by construction, comes back with a full TTL). Negative fact: there is **no**
cache mode, offline flag or repository layer — `ttlCache` remains the only read front.

**`ErrFromCache` accompanies the value, it does not replace it.** Whoever does not check
`errors.Is` sees `err != nil` and behaves as before the cache existed, which is the safe default;
whoever opts in to using the value writes that at the call site. `/animes` uses it to **refuse** the
orphan refresh (that would be the #65 burst against a dead API); `verification.go` uses it to
**accept** the list, status-based deletion included — with the risk recorded in decisions.md #92.
`timeUntilAiring` is relative to the fetch, so write and read go through `stampAiringAt`/`rebaseAiring`;
without that, a pass served from cache would forever conclude that nothing new has aired.

**`series.go` — the series' absolute axis.** The continuous numbering that crosses cour and part,
which a large share of groups use in file names. AniList **has no franchise id**: the `PREQUEL`
chain is the only source (decisions.md #71), and because one query only expands 2 levels of
`relations`, resolving the chain is a BFS, not a read (decisions.md #77). Each fetched media teaches
**two** links (levels 0 and 1, the only ones in an authoritative position); the grandparent, whose
`relations` came cut, goes back to the frontier instead of becoming a root. The seed comes ready
from the caller because **`anilist` cannot import `files`** — and only the daemon knows the right
set.

**`mal.go` — MyAnimeList, as a list source only.** It lives in `anilist` on purpose (it needs
`ttlCache`, the snapshot, `sendAnilistRequest` and `GetMediaByIDs`); there is **no** `internal/mal`
package and no `Account` type. A MAL entry becomes the same synthetic `MediaList` a standalone anime
gets, so `MediaList.Id == 0` means "standalone **or** MAL" and nothing can tell them apart by it.
What bites: `ErrMALEmpty` is a **failure** — every caller must treat it like a failed fetch, or the
account loses its deletion veto; and the status filter must run **before** `GetMediaByIDs`, or the
frontend poll hydrates the whole list every minute. The MAL account loops live next to the AniList
ones in `daemon/verification.go` and `api/endpoint_animes.go`/`api/standalone_guard.go`, keyed
`"mal:" + user` in `inDeleteStatus`
([decisions.md #95](decisions.md#95-myanimelist-is-a-list-source-not-a-metadata-source)).

### `src/internal/nyaa/`

**`ScrapNyaaForAnime` returns packs and episodes in the _same_ list** — what splits them is
`daemon.partitionSearchResults`. It is the single search behind pack + episode resolution.

**Adaptive pagination** (`fetchSearchPages`, shared by the three searches): page 1, and it only
walks further while accepted candidates are below `enoughCandidates` (3) and the page has rows, up
to `ActiveMaxSearchPages()`. An error is only an error on page 1.

**Episode search drops batches and movies, and the right guard is `hasMovieMarker`, not `isMovie`**
(decisions.md #53) — two similar functions and only one is safe here. A long series (>100 episodes)
**also** gets the 3-digit zero-padded variant, never in place of the plain one, and what unlocks it
is `LastAiredEpisode` (decisions.md #56).

**`ExtractPart` returns `nil` when the name declares two parts** ("(Part 1 + Part 2)", "(Season 4 Part
03+04)"): it covers both halves, and the first match would lie.

**The range read from the name is the first _plausible_ match, not the first match**
(decisions.md #83): it rejects a resolution tag (`[720-1080p]`) and an inverted range
(`"Euphonium 2 - 01 ~ 13"` → `2..1`).

**`PackFileRange`** reads the range the pack actually carries from the detail-page file list, on the
same ruler as the name (decisions.md #84). **`IsVideoFile` lives here, not in `files`**, because both
read the same pack file list (the Librarian after download, `PackFileRange` before) and `files`
already imports `nyaa` — the other way would be a cycle.

**`detail.go` — view-page scrape for the picker.** `FetchTorrentDetail` builds the URL through
`ViewURL` (the `NYAA_URL` override lives only in `getNyaaBaseURL`) and **never takes a URL from the
caller**
([decisions.md #105](decisions.md#105-fetchtorrentdetail-builds-the-view-url-itself-it-never-takes-one-from-the-caller));
HTTP 404 is `ErrTorrentNotFound`, any other transport/non-200 is `ErrNyaaUnavailable`. Movie rows
fill `DetailURL` the same way pack rows do — the description pane needs it. `PackFileRange` still
takes an absolute URL; daemon callers that only have an id use `ViewURL`.

**Sorting** (`priorities.go`): the `health` criterion compares the **band** (`healthTier`, floors
`10/50` seeders), not the raw score (decisions.md #55, #107). `SortTorrentResults` omits
`source`; `SortFinishedSeriesResults` includes it and the daemon applies that sort only when
`Media.Status == FINISHED` (`rankSeriesResults` in `daemon/search.go`). `SetPriorities` and
`SetMaxSearchPages`/`SetMaxBatchTorrentSizeGB` use the same atomic+restore pattern, pushed by
`files.LoadConfigs` on every successful load.

Resolution is canonicalized **in the extractor**, and codec presets are a reordering done in the
frontend (decisions.md #70).

**Title matching** (`nyaa_match.go`): technical tokens are stripped, the name is cut at the first
season/episode marker, and the Jaccard threshold depends on query length — `0.8` for ≤3 tokens
(stops "SAO" matching "SAO Alternative Gun Gale Online"), `0.4` for ≥4 (a fansub that embeds both
title translations). Only if the truncated name fails is each alternative title written **after**
the marker tried on its own (decisions.md #75).

**`nyaa_regex.go`** is only package-level precompiled regex, so it is not recompiled inside a
per-torrent loop. No logic.

### `src/internal/torrents/` — embedded BitTorrent client

`github.com/cenkalti/rain/v2` behind the `TorrentBackend` interface. One seam, two sides: production
uses `SessionManager`/`Session`; tests use the in-memory `FakeBackend`.

**Lock order: `queue.mu` → `SessionManager.mu`, never the other way.** `queue.enforce` holds
`queue.mu` while calling `list`/`pause`/`resume`, so every `SessionManager` method drops its own
lock **before** touching the queue. Corollary: `PrioritizeAll` **must not** call `Get`/`List` —
both go through `markQueued`, which takes `queue.mu`.

**`queueOps` are the raw delegations, all unexported.** Going through `List`/`Pause`/`Resume` would
re-enter the queue: infinite recursion on pause/resume.

**`enforce` gives up when `list()` returns `nil`** — no session. `nil` is **not** an empty session;
treating the two the same would empty the persisted queue (decision 41).

**`completedFromStats` is deliberately independent of `Status`** (`Pieces.Have >= Pieces.Total`):
pausing a finished torrent takes it out of `Seeding`, and reading completion from the status would
mark a complete torrent as incomplete.

**`queued` is the only slug rain never produces** — `queue.markQueued` is who writes it
(decision 41). `statusSlug` maps rain's enum to the API's stable slugs, and **must be kept in sync
by hand with `src/lib/utils/torrentStatus.ts` in the frontend**.

**Download-folder identity** (`rootmarker.go`): `.aad_root` lives **inside** the download folder and
travels with it; `download_root.id` lives in the config folder, which the user does not move. The
pair is how a moved/deleted/swapped folder is detected (decisions.md #34). With no id recorded
(first run or upgrade) it is **never** a swap. The swap is latched by `Ensure` and read by
`ConsumeRootSwap`, because the manual-download endpoints also call `Ensure`.

`session.db` (bbolt, resume) and `queue.json` live **outside** the download folder, so they survive
a library-path swap.

### `src/internal/notifications/`

`Notify` fires in goroutines; `Flush` fires the pending one **synchronously** and only returns when
the requests have finished — it is called on shutdown, where a goroutine would be the same as not
sending.

`NewEpisode` only fires when there is **at least one magnet to try**; an episode with no search
result goes straight to `DownloadFailed`/`ReasonNotFound`.

Notifications are grouped by window, and JSON escaping happens **only in the body and only when the
preset declares `Content-Type: application/json`** (decisions.md #47).
Header values have `\n`/`\r` stripped: `net/http` rejects the whole request with them, and ntfy
uses a header for the title.

Template variables (URL, headers and body): `{{title}}`, `{{message}}`, `{{anime_name}}`,
`{{episode}}`, `{{reason}}`, `{{count}}`. In a window with N > 1, `{{message}}` becomes one line per
item.

### `src/internal/diagnostics/` — bug report

Builds the report bundle from **files in the data dir only** — config, `anime_settings`,
`daemon.log` (or the newest `daemon-*.log.gz` after a rotation), `crash-pending.log` — and never
asks the running daemon. That's what lets the CLI `report` command build the same bundle
with the daemon dead. The config is read as a raw map, **not** through `LoadConfigs`: a config that
fails to load is exactly what a report must still carry. Nothing is ever sent; the user attaches
the file by hand ([decisions.md #102](decisions.md#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file)).

- **Redaction is by log key** (`redact.go`, `keyClasses`): keep / drop / pseudonymize (usernames →
  `userN`) / title (`<title-N>` when titles are off). A key missing from the table is treated as a
  title at runtime, and `keys_test.go` fails the build for any `.Str("key", …)` in the source that
  isn't classified ([decisions.md #103](decisions.md#103-log-redaction-is-by-key-and-a-test-forces-every-new-key-to-be-classified)).
  On top of the keys, free text is scrubbed: webhook URLs/headers anywhere → `<webhook-N>`, the
  home dir → `~`, URLs to hosts outside the allowlist → `<url>`. `"HTTP request"` lines are dropped
  whole (they carry the LAN IPs)
- **Crash lifecycle** (`crash.go`): the runtime writes a fatal panic to `crash.log`; the next boot
  (`StartCrashCapture`) promotes a non-empty one to `crash-pending.log`; handling it in the UI
  (`DELETE /diagnostics/crash`) moves it to `crashes/`, newest 5 kept. An older pending crash
  nobody handled is archived before a new one takes its place, so none is lost
- `environment.go` reads the OS through a `host` struct (goos/getenv/readFile) so the docker,
  systemd and desktop detection is testable without the real machine

### Small packages

- **`datadir/`** — `Path()`: the data dir, created if missing. Not cached: it's a stat and a
  `MkdirAll`, and callers run once at boot
- **`logger/`** — zerolog, console + rotating `daemon.log`. `InitDebug` sends the trace to another
  file, so that `--debug-anime` never touches `~/.autoAnimeDownloader`
- **`version/`** — `Version`, `"dev"` by default, overridden by ldflags
- **`cli/`** — daemon lifecycle control for the CLI binary (`cmd/cli` is the only importer). Two
  paths, in this order: a systemd **user** unit and, if it isn't installed, a PID file. **There is no
  Windows Service Manager integration** — it existed, calling `sc`/`net`, but nothing in the repo
  installs that service (`development.md` says a Windows service isn't supported) and
  `package-windows` doesn't even ship the CLI binary, so all three paths were unreachable in every
  published artifact. On Windows the PID file applies
- **`tray/`** — build-tagged: the real implementation needs CGO, so `tray_stub.go` keeps the
  CGO-less build compiling (CI enforces it in `build.yml`). On Linux the daemon only starts the
  tray with a non-empty `DISPLAY`. "Report a problem" opens `#/report` in the browser
- **`frontend/embed.go`** — `//go:embed dist/*`. `dist/` has to exist **before** `go build`, which
  is why the frontend always builds first

### `src/internal/frontend/src/` — Svelte 5

There is no component library: daisyUI was removed (decisions.md #91). Button, modal, checkbox,
chip and the like live in `components/ui/`; color comes from `lib/design/tokens.css`, which is the
single source of truth and switches themes through its own `[data-theme]`.

**Routes** (hash router): `Status.svelte` (`#/`), `AddAnime.svelte` (`#/add`), `TorrentPicker.svelte`
(`#/add/:id/torrents` and `#/status/:id/torrents` — longer paths are registered first in
`App.svelte` or svelte-spa-router would let `/status/:id` swallow the picker), `Downloads.svelte`
(`#/downloads`), `AnimeDetail.svelte` (`#/status/:id`), `Config.svelte` (`#/config`),
`Priorities.svelte` (`#/priorities`), `Logs.svelte` (`#/logs`), `Notifications.svelte`
(`#/notifications`), `Report.svelte` (`#/report`). The picker cache (`stores/pickerCache.ts`) is
**in-memory**: Add fills it before navigating so the first paint is free; a reload of the picker
URL fetches; confirm clears it. Add's first search (`getTorrentCandidates` with no `q`) decides
the route: empty `items` POSTs today's add and stays (same toasts, no picker); a non-empty list
or `NYAA_UNAVAILABLE` pushes `#/add/:id/torrents` without POSTing; any other GET error toasts and
stays
([decisions.md #108](decisions.md#108-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)).
AnimeDetail's header "Replace torrent" pushes `#/status/:id/torrents` — there is **no**
whole-title magnet modal; the per-episode magnet modal stays. The description pane is on-demand
(`GET …/torrent-candidates/{nyaaId}`), never bundled into the list.

**Negative facts that save a search:**

- **Status and the anime list are one screen.** There is no separate "Library" route
- **There is no `Layout.svelte`** — it was deleted; the shell is `AppShell.svelte`. It wrote the six
  navigation links twice, a desktop block and a mobile one
- **There is no health-check endpoint.** `stores/backendHealth.ts` derives backend health inside
  `apiRequest`, from the responses the app already makes
- **There is one toast store**, `stores/toast.ts`. A second one (`toasts.ts`) used to receive the
  client's automatic error toasts and nothing rendered it — every one of them was silently lost
- `AnimeDetail` has no `#` column: it repeated, in another font, the number the episode title
  already carries

**Structure:** `components/shell/` (AppShell, NavRail, NavTabBar, MoreMenu, SystemBanner),
`components/ui/` (primitives), `lib/domain/` (pure logic, no Svelte import, covered by vitest),
`lib/stores/`, `lib/utils/`, `lib/api/client.ts` (typed wrappers per endpoint),
`lib/websocket/client.ts` and `lib/report.ts` (links to the report screen and to GitHub; `REPO_URL`
lives only there).

**UI layer rules:**

- **No `ui/` component contains a hex literal** — they all consume the semantic Tailwind names
  backed by `lib/design/tokens.css`
- `lib/navItems.ts` is the **single source** of the navigation list, read by `NavRail`/`NavTabBar`/`MoreMenu`
- `AppShell` picks `NavRail` vs `NavTabBar` by `matchMedia` in JS, **not** by `hidden md:flex` — the
  CSS version would keep both mounted, duplicating ids
- `torrentStatus.ts` has to be kept in sync by hand with `statusSlug()` in
  `src/internal/torrents/status.go`
- **A banner is degraded state; a toast is a failed action.** `lib/domain/systemBanner.ts` picks ONE
  banner by precedence (backend unreachable → backend 5xx → AniList down → rate limit), never stacks
  them (decisions.md #66). Client errors become a `toast.error` unless the caller passes `silent`.
  A reportable banner's "Report" link opens `#/report` too, pre-filled with the banner's kind and
  detail — not a bare GitHub link.
  An **unexpected** failure (5xx, network, unreadable response — not a 4xx, not an abort) gets a
  10 s toast with a Report link to `#/report` pre-filled with the error. `toast.ts` skips a toast
  identical to one already on screen, because most pages also toast the same `err.message` in
  their catch — the client's one, with the link, wins
- **Crash → report screen.** `SystemBanner` polls `/status` every 30 s; a new `pending_crash_at`
  opens `#/report?crash=1` on a fresh page load and only toasts in a tab that was already open (a
  route change could throw away an unsaved form). The last crash reacted to is remembered in
  `localStorage` (`aad.lastPendingCrash`)
- Progress comes from **one** `Stats()` per torrent, pulled only while a screen is open
  (decisions.md #30) — `speedHistory` and `stallTracker` don't poll on their own: the owning screen pushes them
