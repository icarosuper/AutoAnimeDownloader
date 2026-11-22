# Config Reference

All fields in `config.json` (`~/.autoAnimeDownloader/config.json` on Linux, `%APPDATA%\.autoAnimeDownloader\config.json` on Windows).

Struct defined in `src/internal/files/filemanager.go`. Defaults set in `getDefaultConfig()`.

## Fields

| Field | JSON key | Type | Default | Description |
|-------|----------|------|---------|-------------|
| `CompletedAnimePath` | `completed_anime_path` | `string` | `~/Animes` (`""` if the home dir is unknown) | The Jellyfin library location. Completed episodes are **hardlinked** here (not moved/copied). **Required**. The download/seeding working directory (rain's DataDir) is **derived** from it — see "Download Path" below — so there is no separate path to configure |
| `AnilistUsernames` | `anilist_usernames` | `[]string` | `[]` | Anilist usernames to sync watch lists from (multi-account supported). **Optional** — an installation can run entirely on standalone animes (`standalone_animes`, see [decisions/049](decisions/049-standalone-anime-tracked-by-the-list-is-the.md)) |
| `MALUsernames` | `mal_usernames` | `[]string` | `[]` | MyAnimeList usernames, used as **list sources only**: each MAL account answers which anime, in which status, with what progress — identity, episodes and metadata still come from AniList, reached through `idMal` ([decisions/095](decisions/095-myanimelist-is-a-list-source-not-a-metadata-source.md)). The list must be **public** (read from the unofficial `load.json`, no client ID). `ExcludedLists` doesn't apply (MAL has no custom lists). Status mapping: watching→`CURRENT`, completed→`COMPLETED`, on hold→`PAUSED`, dropped→`DROPPED`, plan to watch→`PLANNING`, rewatching flag→`REPEATING`; `DownloadStatuses`/`DeleteStatuses` apply to it with the same OR/AND rules. **Optional**, not validated |
| `AnilistUsername` | `anilist_username` | `string` | `""` | **Legacy.** Single-username field, `omitempty`. Migrated into `AnilistUsernames` and cleared by `FileManager.LoadConfigs()` (`filemanager.go`) — **only there**, and the migration persists. `handleUpdateConfig` deliberately does **not** repeat it: a PUT still carrying the singular key is saved and cured on the next load (decisions/090). Kept only for backward compatibility |
| `CheckInterval` | `check_interval` | `int` | `10` | Minutes between verification loops. Must be > 0 |
| `MaxEpisodesPerAnime` | `max_episodes_per_anime` | `int` | `12` | Max saved episodes per anime before oldest are deleted, and the width of the pack-selection window (`daemon.windowEnd`). `0` = off — no ceiling, no window end (`daemon.effectiveMax`/`windowEnd`). **Applies only to the episode-by-episode path** — never to a batch download (a batch is one torrent, so limiting records would limit neither bytes nor library files; see [decisions/048](decisions/048-max-episodes-per-anime-never-limited-batches-the.md)). Must be >= 0 |
| `MaxBatchTorrentSizeGB` | `max_batch_torrent_size_gb` | `float64` | `100` | The **only** guard on batch eligibility — packs are no longer gated by anime metadata (finished/episode count), just by what the search actually returns. Ceiling in **GiB** per batch torrent: results above it are dropped from the Nyaa search result (`daemon.filterBySize`), not downloaded and deleted. Also pushed into the nyaa package by `LoadConfigs` (`applyNyaaSettings` → `nyaa.SetMaxBatchTorrentSizeGB`), where an oversized pack row is dropped **before** it counts toward the pagination floor — otherwise three giant packs on page 1 end the page descent ahead of the partial packs that fit (see [Decisions #80](decisions/080-the-pack-size-ceiling-is-applied-twice-in-the-nyaa.md)). `100` fits a full 1080p season pack but not a full One Piece pack (for a long-running series what passes is a partial pack, covering the selection window). `0` = off. A torrent whose size failed to parse (`Size == 0`) passes the filter. Must be >= 0. **Release note:** an installation that already has `max_batch_torrent_size_gb: 0` saved keeps the filter off — `LoadConfigs` unmarshals over the new default, so an explicit `0` on disk is not overwritten and must be raised by hand. With `max_episodes_per_anime = 0` the window is fully open and a series like One Piece can resolve ~14 packs in a single pass, throttled only by `max_batch_torrent_size_gb` (per torrent) and `checkDiskSpace` |
| `MaxEpisodeTorrentSizeGB` | `max_episode_torrent_size_gb` | `float64` | `5` | Same, for the single-episode / multi-episode / movie searches. `5` passes a WEB-DL (~1.4 GB) and a competent BD encode (~2 GB) and blocks a BDRemux (~6-8 GB) — an episode had no ceiling at all until this default existed (the field was absent from `getDefaultConfig()`, and `0` = unlimited is a valid zero value, so the gap was invisible). `0` = off. Must be >= 0 |
| `MinSeeders` | `min_seeders` | `int` | `3` | Floor on the seeders column: results below it are dropped from every Nyaa search (`daemon.filterBySeeders`, applied by `filterSearchResults` alongside the size ceiling). `0` = off — with no floor a 0-peer result is a valid candidate and gets picked whenever it is the only one (measured: `[DB] Naruto Shippuuden Movie 3` was the sole candidate for episode 3, at 0 peers). Unlike the size ceiling, an unparseable seeders column counts as `0` and is **discarded** — see [decisions/054](decisions/054-seeders-floor-unreadable-seeders-are-discarded.md). The default is `3` and not `1` because `1` blocks only the literally dead torrent, and a single seeder stalls often. Must be >= 0. Ranking among survivors uses `healthTier` bands with floors 10 and 50 seeders (not a config field; see [decisions/107](decisions/107-source-ranks-finished-series-empty-source-is-web.md)) |
| `MaxSearchPages` | `max_search_pages` | `int` | `5` | Ceiling on Nyaa result pages **per search**. Not a fixed cost: a search reads page 1 and only goes deeper while it has fewer than 3 accepted candidates, and stops early on an empty page (`nyaa.fetchSearchPages`). Page 2 used to be fetched unconditionally, so a search that resolves on page 1 now costs **one** fetch instead of two — that saving is what pays for the higher ceiling. `<= 1` = page 1 only. Applies to all three searches (episode, anime — which covers both packs and episodes, movie); movie reads a single page before. Pushed into the nyaa package by `LoadConfigs` (`applyNyaaSettings`), like `priorities`. Must be >= 0 |
| `MinFreeDiskGB` | `min_free_disk_gb` | `int` | `10` | Slack kept on the library volume, in **GiB** (2^30). A new torrent is refused when `free - reserved` would fall below this (`daemon.checkDiskSpace`, applied in `attemptDownloadWithRetries` and `addAndPrioritize`). The check subtracts the announced torrent size and what the queue still has to write first ([decisions/101](decisions/101-the-disk-guard-counts-what-the-queue-still-has-to.md)). The verification pass still runs in full — pruning and organizing are what free space. `0` = off **for the floor only**: a torrent bigger than `free - reserved` is still refused ([decisions/093](decisions/093-the-disk-guard-has-two-triggers-and-the-size-one.md)). Must be >= 0. Replaces `min_free_disk_percent` (unknown JSON key, dropped on load — no migration). Also drives `disk_low` in `GET /status`, which is this add-gate for a torrent of size 0 |
| `EpisodeRetryLimit` | `episode_retry_limit` | `int` | `5` | Max magnet links to try per episode before giving up. Must be >= 0 |
| `MaxConcurrentDownloads` | `max_concurrent_downloads` | `int` | `3` | How many **incomplete** torrents may run at once; the rest wait in the download queue (`torrents/queue.go`, status slug `queued`). `0` = no limit. Seeding is never limited. Must be >= 0. Applied by `SetMaxActiveDownloads` from three places: boot (`cmd/daemon/main.go`), `PUT /config`, and the top of every `AnimeVerification`. No migration needed — `LoadConfigs` unmarshals **over** `getDefaultConfig()`, so a `config.json` written before this field loads with the default already in place |
| `DeleteWatchedEpisodes` | `delete_watched_episodes` | `bool` | `true` | Whether to auto-delete episodes marked as watched on Anilist |
| `WatchedEpisodesToKeep` | `watched_episodes_to_keep` | `int` | `2` | Number of watched episodes to keep before deleting. `0` = delete all watched. The default is `2` because `DeleteWatchedEpisodes` ships **on**: with no slack the episode disappears the moment it is marked, which forgives neither "marked it by mistake" nor "wanted to rewatch". Must be >= 0 |
| `ExcludedLists` | `excluded_lists` | `[]string` | `[]` | Names of Anilist custom lists to exclude from downloads. AniList accounts only — MyAnimeList has no custom lists |
| `ExcludedList` | `excluded_list` | `string` | `""` | **Legacy.** Same migration pattern as `AnilistUsername` — merged (comma-split) into `ExcludedLists` by `FileManager.LoadConfigs()` on load |
| `DisableSeeding` | `disable_seeding` | `bool` | `false` | No upload at all: the torrent session never unchokes a peer and serves no allowed-fast piece, even during the download, and every torrent is removed with its download copy as soon as it is organized into the library (the hardlink stays). A pack's parts that were not linked (another season, extras) are deleted with it. Downloads may be slower, since peers favor whoever uploads back. Toggling rebuilds the session: right away from `PUT /config` (only when the value changed), and the pass re-applies it every cycle. Turning it on also releases the torrents already seeding, on the next pass. See [decisions/119](decisions/119-disable-seeding-means-no-upload-at-all-and-a.md) |
| `RenameFilesForJellyfin` | `rename_files_for_jellyfin` | `bool` | `false` | Give the **library hardlink** a Jellyfin-compatible name (`"Anime Name - E05.mkv"`). Single episodes use the episode number from the record (only when the torrent holds one video file); **batch packs** rename each file using the number parsed out of its own filename, so pack episodes mix into the anime folder alongside individually downloaded ones. Files with no readable number (NCOP/NCED, extras, movies) keep the raw name. The seeded copy in the download directory is never renamed (that would break seeding) |
| `DownloadStatuses` | `download_statuses` | `[]string` | `["CURRENT", "REPEATING"]` | Anilist **list** statuses (user's relationship to the anime) to download. Also governs which not-yet-downloaded animes appear in `/api/v1/animes` (filtered server-side via GraphQL `status_in`). Valid values: `CURRENT`, `REPEATING`, `COMPLETED`, `PAUSED`, `DROPPED`, `PLANNING` |
| `DownloadMediaStatuses` | `download_media_statuses` | `[]string` | `["RELEASING", "FINISHED"]` | Anilist **media** statuses (the anime's own airing state) eligible for download. Also governs which not-yet-downloaded animes appear in `/api/v1/animes` (filtered client-side, since AniList doesn't support this in the same `status_in` filter as list status). Filtered via `anilist.MediaStatusAllowed` in both `searchAnilist` (`daemon/verification.go`, download pipeline) and `fetchAniListEntries` (`api/endpoint_animes.go`, frontend listing). Animes with at least one downloaded episode are never hidden by either filter regardless of current status — see [Architecture](architecture.md#media-status-filter). Whitelist semantics: empty = nothing downloads/shows. Valid values: `RELEASING`, `FINISHED`, `CANCELLED`, `HIATUS` (`NOT_YET_RELEASED` excluded — can never have episodes) |
| `CheckForUpdates` | `check_for_updates` | `bool` | `true` | Asks GitHub once a day (first check ~2 min after boot) for the latest release: the update badge in the web UI, `Update available` in CLI `status`, and the `update_available` event. `false` = no request ever leaves; read before every check, so it needs no restart. A `dev` build never checks. Whoever installs from the AUR or a container image already hears about updates there |
| `DeleteStatuses` | `delete_statuses` | `[]string` | `[]` | Anilist list statuses to auto-delete episodes from. Same valid values as `DownloadStatuses`. With several accounts (AniList and MyAnimeList alike) the rule is **AND**: every account that has the anime must have it in one of these statuses (not necessarily the same one). Download is the opposite, **OR** — see [Architecture](architecture.md#media-status-filter) |
| `AnimeIDsAreMediaIDs` | `anime_ids_are_media_ids` | `bool` | `false` | Internal marker: `daemon.MigrateAnimeIDsToMedia` already converted the saved `anime_id`s from entry id (`MediaList.Id`, per account) to media id. Default `false` on purpose — a `config.json` older than the field needs to migrate; on a new install the migration runs with nothing to do and turns the field on. Don't edit by hand: turning it on without migrating leaves the records on disk keyed in a way nothing else understands. See [Architecture](architecture.md#media-status-filter) and decisions/043 |
| `Notifications.Webhooks` | `notifications.webhooks` | `[]WebhookPreset` | `[]` | List of webhook presets to fire on events |
| `Notifications.Webhooks[].Name` | `name` | `string` | — | Human-readable label (used to identify webhook in API/UI) |
| `Notifications.Webhooks[].URL` | `url` | `string` | — | Target URL — supports `{{vars}}` |
| `Notifications.Webhooks[].Method` | `method` | `string` | — | HTTP method (`POST`/`GET`/`PUT`) |
| `Notifications.Webhooks[].Headers` | `headers` | `map[string]string` | — | Request headers — values support `{{vars}}` |
| `Notifications.Webhooks[].Body` | `body` | `string` | — | Request body — supports `{{vars}}` |
| `Notifications.Webhooks[].AnimeSources` | `anime_sources` | `[]string` | `[]` | Which animes the webhook notifies about: `anilist:<user>`, `mal:<user>`, `standalone`. Empty = every anime. OR between them; an anime's sources are the accounts whose lists the last check fetched hold it (download and delete statuses) plus `standalone`, resolved when the webhook fires, with no AniList call. An anime none of them holds is unknown and still reaches filtered webhooks (fail-open). Ignored by the Test button. Not `sources`: `priorities.sources` is torrent sources. See decisions/126 |
| `Notifications.BatchWindowSeconds` | `notifications.batch_window_seconds` | `int` | `60` | Groups the events of one window into a single webhook (one queue **per event**, never mixing success with failure). `0` disables it: one webhook per item, the original behavior — a pack is one item. A `config.json` older than the field loads with **60**, not `0`: `LoadConfigs` deserializes on top of `getDefaultConfig()`. The 60s are measured, not taste — in a real backfill of 67 organizes, a 10s window would deliver 53 webhooks (and the service answered 429) and the 60s one delivers 24. With one item in the window the message is identical to the ungrouped one; with N > 1, `{{title}}` gets the count, `{{message}}` becomes one line per run of consecutive episodes, `{{count}}` carries the episode total, and `{{anime_name}}`/`{{episode}}`/`{{reason}}`/`{{anime_id}}`/`{{torrent_hash}}` are empty (there's no single value). See decisions/047 |
| `Priorities` | `priorities` | `nyaa.Priorities` | see below | Ordered lists driving torrent ranking/filtering. Defined in `src/internal/nyaa/priorities.go` |
| `Priorities.CriteriaOrder` | `priorities.criteria_order` | `[]string` | `["uncensored","resolution","health","source","codec","fansub","audio","size"]` | Order in which sort criteria are applied. `SortTorrentResults` uses the episode subset (`uncensored`, `resolution`, `health`, `codec`, `fansub`, `size` — no `source`, no `audio`). `SortFinishedSeriesResults` is that subset plus `source`, used when `Media.Status == FINISHED`. `SortMovieResults` uses all. `health` sits **before** `fansub` on purpose — see [decisions/055](decisions/055-health-compares-a-seeders-band-and-it-comes-before.md). **An existing `config.json` keeps the order it already has** (`LoadConfigs` unmarshals over the defaults); restore-defaults (or a hand edit) is how an old install picks up the new relative order of source vs health vs resolution. Health **bands** do not wait on that click |
| `Priorities.Fansubs` | `priorities.fansubs` | `[]string` | `["subsplease","erai-raws","judas","toonshub","asw","ember","hd-zone","kamig","remix","aniverse","dub","raw"]` | Fansub preference order, lowercase, matched as substring of torrent name |
| `Priorities.Resolutions` | `priorities.resolutions` | `[]string` | `["1080p","720p","1440p","2160p","480p"]` | Resolution preference order. Tokens must match what `ExtractResolution` returns, which **canonicalizes**: `4k`/`uhd` → `2160p`, `1920x1080`/`fhd` → `1080p`, `hd` → `720p` (see [decisions/070](decisions/070-resolution-is-canonicalized-in-the-extractor-and.md)). `8k` is deliberately absent, so an 8K release ranks last. 4K below 720p is deliberate too: anime in 4K is almost always an upscale of a 1080p master |
| `Priorities.Sources` | `priorities.sources` | `[]string` | `["bdremux","bd","bdrip","web-dl","webrip","tv","dvd","hdtv"]` | Source preference order. Ranks movies always and series when `Media.Status == FINISHED`. Empty extract canonicalizes to `web-dl`. BDRemux is the untouched disc bitstream and BDRip is a reencode — the order used to be inverted |
| `Priorities.Codecs` | `priorities.codecs` | `[]string` | `["h.264","hevc","av1","xvid"]` | Codec preference order. Applies to **both** episode and movie sort. Tokens must match what `ExtractCodec` returns (`hevc`, `av1`, `h.264`, `xvid`) — `x265`/`x264` would be inert; a release whose name carries no codec tag ties on this criterion instead of losing it — see [decisions/069](decisions/069-codec-now-applies-to-episodes-and-a-name-with-no.md). H.264 leads by default because it plays direct on any player: HEVC/AV1 force the media server to transcode for a browser, which burns the ASS subtitle into the video and desyncs it. The Priorities page has a one-click preset for the opposite trade-off (smaller files) |
| `Priorities.Audio` | `priorities.audio` | `[]string` | `["flac","dts-hd","truehd","ddp","aac","ac3","mp3"]` | Audio codec preference order (movie sort only) |
| `Priorities.IgnoreList` | `priorities.ignore_list` | `[]string` | `["[dub]","[raw]","[hardcoded]","[hc]","re-encode"]` | Substrings (case-insensitive) that discard a release entirely |

Items absent from a list rank worst (sent to the end). Edited via the `#/priorities` screen, persisted through the regular `GET/PUT /api/v1/config` endpoints.

## Download Path

There is no `save_path` field. The download/seeding working directory is **derived** from `completed_anime_path` at every use, via `Config.DownloadPath()` (`internal/files/filemanager.go`):

```
<completed_anime_path>/.torrents
```

Torrents download and keep seeding there (rain's `DataDir`, layout `<DownloadPath()>/<torrent-id>/...`); on completion, video files are hardlinked out into `completed_anime_path` itself (the Jellyfin library). Because the download directory is a subfolder of the library, the two are guaranteed to share a filesystem — the old "must be on the same volume" requirement is now an architectural invariant rather than something the user can misconfigure. `Librarian.ProbePath(completed_anime_path)` still runs (on config save and on every verification pass) to catch filesystems that don't support hardlinks at all (exFAT/FAT32, some SMB shares), unrelated to the cross-volume case.

**No automatic migration.** `daemon.MigrateSavePath` used to move a legacy `save_path`'s torrent
folders into the derived download path at boot; it was **removed** along with the `SavePath` field.
A `config.json` that still carries `save_path` loads fine (JSON ignores unknown fields) and the key
disappears on the next save, but its data is **not** moved — rain opens the derived path, finds it
empty and re-downloads. Moving the folder is now a manual, pre-upgrade step. See decisions/031.

## Removed Field

`qbittorrent_url` (and its `QBITTORRENT_URL` env override) was **removed** when the external qBittorrent dependency was replaced by an embedded BitTorrent client. Old configs that still contain the key load fine (JSON ignores unknown fields) and the key disappears on the next save.

`save_path` was **removed** entirely — first as a user-configurable field (the download directory is derived, see "Download Path" above), then as a struct field once the migration that read it was deleted. Old configs carrying the key load fine and the key disappears on the next save.

## Required Fields

Daemon checks these in `isConfigComplete()` (`daemon/helpers.go`) before starting the verification loop:
- `completed_anime_path` — **the only one.** An AniList account is *not* required: with standalone animes the app works with no list at all, and demanding an account would strand that installation on the config screen

If missing, daemon opens browser to `http://localhost:<port>/#/config?missingConfig=true`.

Default: `completed_anime_path` starts as `~/Animes` (`getDefaultConfig()`), so a fresh install is already "complete". On the **first boot only** (no `config.json` yet — `FileManager.ConfigExists()`, checked in `main()` before the first `LoadConfigs()`) the daemon opens `http://localhost:<port>/#/status`.

## Validation (API)

`handleUpdateConfig()` in `endpoint_config.go` validates:
- `completed_anime_path` — non-empty. `anilist_usernames` is **not** validated (see Required Fields); the legacy `anilist_username` is migrated into it by `LoadConfigs`, not by this handler (decisions/090)
- `completed_anime_path` must support hardlinks — verified with a single-path probe (`Librarian.ProbePath`); a filesystem without hardlink support is rejected with HTTP 400
- `check_interval` — > 0
- `episode_retry_limit`, `watched_episodes_to_keep`, `max_concurrent_downloads`, `max_episodes_per_anime`, `max_batch_torrent_size_gb`, `max_episode_torrent_size_gb`, `min_seeders`, `max_search_pages` — >= 0 (`max_episodes_per_anime` and `max_batch_torrent_size_gb` treat `0` as "off"; see their field descriptions above)
- `min_free_disk_gb` — >= 0

## Per-Anime Settings (`AnimeSettings`)

Not part of `Config` — keyed by anime ID, stored/loaded via `FileManager.{Load,Save}AnimeSettings` (`files/filemanager.go`) and exposed at `GET/PUT /api/v1/animes/{id}/settings`.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| `Progress` | `progress` | `int` | Manual watch progress, used only by **standalone** (avulso) animes — a list anime's progress always comes from AniList. Absent/missing reads as `0`. Injected into the synthetic `MediaList` built for a standalone anime, so `shouldSkipEpisode`, `firstEpisodeToConsider`, `buildWatchedKeepSet`, pruning and the `EpisodesWatched` count all treat it exactly like AniList progress, no `isStandalone` branch needed |

`PUT /animes/{id}/settings` (`api/endpoint_anime_settings.go`) is **partial**: `progress` is a pointer (`*int`) so a body that omits it does not zero the saved progress. `progress < 0` is rejected with HTTP 400.

## Webhook Template Variables

Available in `url`, `headers` values, and `body`:

| Variable | Value |
|----------|-------|
| `{{title}}` | Short event label (e.g. "New episode detected") |
| `{{message}}` | Full sentence with anime name and episode number — a batch pack names only the anime (`Frieren downloaded successfully`). In a window with N > 1, one line per run of consecutive episodes of the same anime and reason (`Frieren EPs 1-24 downloaded successfully`) |
| `{{anime_name}}` | Anime title |
| `{{episode}}` | Episode number as string; always a single number — empty for a batch pack (its message names only the anime) |
| `{{reason}}` | Failure reason (for `download_failed`); empty for other events |
| `{{anime_id}}` | AniList media id; empty when unknown |
| `{{torrent_hash}}` | Torrent infohash; empty when there is none yet (`new_episode`) |
| `{{count}}` | Number of episodes in the window (a pack counts each of its episodes) |
| `{{quality}}` | Always empty — not tracked at hook point |
| `{{file_path}}` | Always empty — not tracked |
| `{{timestamp}}` | Current time formatted as `2006-01-02 15:04` |
| `{{version}}` | `update_available` only: the new version (`2.11.0`) |
| `{{url}}` | `update_available` only: the release page |

## Webhook Events

The values of `notifications.webhooks[].events`. The web UI groups them as Episodes, Library and
System; a new message preset starts with the episode events plus the system ones.

| Event | Group | Fires when |
|-------|-------|------------|
| `new_episode` | Episodes | a torrent for a new episode was found and is being added |
| `download_failed` | Episodes | a real failure: every candidate refused by the torrent client (`{{reason}}` = `torrent rejected`) or the torrent dropped for an error or stalling (`{{reason}}` = its error). Once per (episode, torrent); a different torrent failing is sent again. **Not** a missing torrent (that is `episode_stuck`) nor a full disk (`disk_low`) |
| `download_completed` | Episodes | an episode was organized into the library |
| `episode_deleted` | Episodes | episodes were removed from the library |
| `episode_limit_reached` | Library | an anime hit `max_episodes_per_anime` (N downloaded, M waiting for you to watch); once until it drops below the limit |
| `episode_stuck` | Library | an episode has gone 48 h without downloading since its first problem (empty search, everything above the size limit, no seeders, add refused, disk full, or its torrent dropped for stalling/failing), while that problem is still recent. One line per run of episodes: `Frieren EPs 5-7: no torrent found for 2 days`. Again once per **new** problem code; cleared when the episode is organized. Awaiting an earlier episode never counts |
| `season_complete` | Library | a finished anime (AniList status `FINISHED`) has every episode you haven't watched in the library; once per anime, ever |
| `unwatched_backlog` | Library | the next episode to watch (progress + 1) has been in the library for 30 days; says how many unwatched episodes and how much space they take. Once per anime and progress — watching one and stopping again restarts the 30 days from that episode's download date |
| `leftover_files` | Library | weekly digest, while files you are done with are still in the library after the check's own cleanup: episodes at or below progress − `watched_episodes_to_keep`, and animes that left every download status. One line per anime with its size; the title carries the total of the lines that webhook gets (a webhook filtered by account counts only its animes). Skipped on a check where some list account failed to answer |
| `library_missing` | System | the library folder vanished or was replaced (unmounted drive); everything in it will be downloaded again |
| `check_failing` | System | checks have aborted for ≥ 30 min and ≥ 2 passes in a row (setup incomplete doesn't count); once per run of failures, and again with `{{reason}}` = `recovered` on the first pass that finishes. `{{reason}}` is otherwise the pass error code |
| `update_available` | System | a newer release is on GitHub (`check_for_updates`); once per version. `{{version}}` and `{{url}}` (the release page) are filled only here |
| `disk_low` | System | two levels, each once until space is freed (free > its floor × 1.25): `{{reason}}` = `warning` when free space drops under 2 × `min_free_disk_gb` (5 % of the volume with the policy at 0), `blocked` when a pass refused downloads for lack of space. A disk that goes straight from fine to full sends `blocked` only |
| `daemon_crashed` | System | at boot, when the previous run crashed (the crash report is waiting on the Report screen) |

The Library and System events are **alerts**: `{{message}}` is one line per alert, `{{title}}`
names the alert, the alerts of one evaluation are **one** webhook even with
`batch_window_seconds: 0`, and `{{anime_name}}`/`{{episode}}`/`{{anime_id}}` are empty for the System ones,
which also reach a webhook filtered by `anime_sources` — they are not about any account's anime.

## Loading Behavior

`LoadConfigs()` starts from `getDefaultConfig()`, then unmarshals `config.json` on top. Fields missing from JSON keep their defaults. If file is empty, corrupt, or missing, defaults are written to disk.
