# Config Reference

All fields in `config.json` (`~/.autoAnimeDownloader/config.json` on Linux, `%APPDATA%\AutoAnimeDownloader\config.json` on Windows).

Struct defined in `src/internal/files/filemanager.go`. Defaults set in `getDefaultConfig()`.

## Fields

| Field | JSON key | Type | Default | Description |
|-------|----------|------|---------|-------------|
| `CompletedAnimePath` | `completed_anime_path` | `string` | `""` | The Jellyfin library location. Completed episodes are **hardlinked** here (not moved/copied). **Required**. The download/seeding working directory (rain's DataDir) is **derived** from it — see "Download Path" below — so there is no separate path to configure |
| `AnilistUsernames` | `anilist_usernames` | `[]string` | `[]` | Anilist usernames to sync watch lists from (multi-account supported). **Required** — at least one |
| `AnilistUsername` | `anilist_username` | `string` | `""` | **Legacy.** Single-username field, `omitempty`. Migrated into `AnilistUsernames` and cleared — by `FileManager.LoadConfigs()` (`filemanager.go`) on every load, and again by `handleUpdateConfig` (`endpoint_config.go`) so a PUT from an old client is migrated before validation. Kept only for backward compatibility |
| `CheckInterval` | `check_interval` | `int` | `10` | Minutes between verification loops. Must be > 0 |
| `MaxEpisodesPerAnime` | `max_episodes_per_anime` | `int` | `12` | Max saved episodes per anime before oldest are deleted. Must be > 0 |
| `EpisodeRetryLimit` | `episode_retry_limit` | `int` | `5` | Max magnet links to try per episode before giving up. Must be >= 0 |
| `MaxConcurrentDownloads` | `max_concurrent_downloads` | `int` | `3` | How many **incomplete** torrents may run at once; the rest wait in the download queue (`torrents/queue.go`, status slug `queued`). `0` = no limit. Seeding is never limited. Must be >= 0. Applied by `SetMaxActiveDownloads` from three places: boot (`cmd/daemon/main.go`), `PUT /config`, and the top of every `AnimeVerification`. No migration needed — `LoadConfigs` unmarshals **over** `getDefaultConfig()`, so a `config.json` written before this field loads with the default already in place |
| `DeleteWatchedEpisodes` | `delete_watched_episodes` | `bool` | `true` | Whether to auto-delete episodes marked as watched on Anilist |
| `WatchedEpisodesToKeep` | `watched_episodes_to_keep` | `int` | `0` | Number of watched episodes to keep before deleting. 0 = delete all watched. Must be >= 0 |
| `ExcludedLists` | `excluded_lists` | `[]string` | `[]` | Names of Anilist custom lists to exclude from downloads |
| `ExcludedList` | `excluded_list` | `string` | `""` | **Legacy.** Same migration pattern as `AnilistUsername` — merged (comma-split) into `ExcludedLists` by `FileManager.LoadConfigs()` on load |
| `RenameFilesForJellyfin` | `rename_files_for_jellyfin` | `bool` | `false` | Give the **library hardlink** a Jellyfin-compatible name (`"Anime Name - E05.mkv"`). Applies only to single episodes (not batches/movies) with a known episode number and a single video file. The seeded copy in the download directory is never renamed (that would break seeding) |
| `DownloadStatuses` | `download_statuses` | `[]string` | `["CURRENT", "REPEATING"]` | Anilist **list** statuses (user's relationship to the anime) to download. Also governs which not-yet-downloaded animes appear in `/api/v1/animes` (filtered server-side via GraphQL `status_in`). Valid values: `CURRENT`, `REPEATING`, `COMPLETED`, `PAUSED`, `DROPPED`, `PLANNING` |
| `DownloadMediaStatuses` | `download_media_statuses` | `[]string` | `["RELEASING", "FINISHED"]` | Anilist **media** statuses (the anime's own airing state) eligible for download. Also governs which not-yet-downloaded animes appear in `/api/v1/animes` (filtered client-side, since AniList doesn't support this in the same `status_in` filter as list status). Filtered via `anilist.MediaStatusAllowed` in both `searchAnilist` (`daemon/verification.go`, download pipeline) and `mergeCurrentAniListAnimes` (`api/endpoint_animes.go`, frontend listing). Animes with at least one downloaded episode are never hidden by either filter regardless of current status — see [Architecture](architecture.md#media-status-filter). Whitelist semantics: empty = nothing downloads/shows. Valid values: `RELEASING`, `FINISHED`, `CANCELLED`, `HIATUS` (`NOT_YET_RELEASED` excluded — can never have episodes) |
| `DeleteStatuses` | `delete_statuses` | `[]string` | `[]` | Anilist list statuses to auto-delete episodes from. Same valid values as `DownloadStatuses` |
| `Notifications.Webhooks` | `notifications.webhooks` | `[]WebhookPreset` | `[]` | List of webhook presets to fire on events |
| `Notifications.Webhooks[].Name` | `name` | `string` | — | Human-readable label (used to identify webhook in API/UI) |
| `Notifications.Webhooks[].URL` | `url` | `string` | — | Target URL — supports `{{vars}}` |
| `Notifications.Webhooks[].Method` | `method` | `string` | — | HTTP method (`POST`/`GET`/`PUT`) |
| `Notifications.Webhooks[].Headers` | `headers` | `map[string]string` | — | Request headers — values support `{{vars}}` |
| `Notifications.Webhooks[].Body` | `body` | `string` | — | Request body — supports `{{vars}}` |
| `Priorities` | `priorities` | `nyaa.Priorities` | see below | Ordered lists driving torrent ranking/filtering. Defined in `src/internal/nyaa/priorities.go` |
| `Priorities.CriteriaOrder` | `priorities.criteria_order` | `[]string` | `["uncensored","source","resolution","codec","fansub","audio","health","size"]` | Order in which sort criteria are applied. `SortTorrentResults` only uses the episode-relevant subset (`uncensored`, `resolution`, `fansub`, `health`, `size`); `SortMovieResults` uses all |
| `Priorities.Fansubs` | `priorities.fansubs` | `[]string` | `["subsplease","erai-raws","judas","toonshub","asw","ember","hd-zone","kamig","remix","aniverse","dub","raw"]` | Fansub preference order, lowercase, matched as substring of torrent name |
| `Priorities.Resolutions` | `priorities.resolutions` | `[]string` | `["1080p","720p","480p","4k","8k","fhd","uhd","hd","2160p","1440p"]` | Resolution preference order |
| `Priorities.Sources` | `priorities.sources` | `[]string` | `["bd","bdrip","bdremux","web-dl","webrip","tv","dvd","hdtv"]` | Source preference order (movie sort only) |
| `Priorities.Codecs` | `priorities.codecs` | `[]string` | `["hevc","av1","x265","h.264","x264","xvid"]` | Codec preference order (movie sort only) |
| `Priorities.Audio` | `priorities.audio` | `[]string` | `["flac","dts-hd","truehd","ddp","aac","ac3","mp3"]` | Audio codec preference order (movie sort only) |
| `Priorities.IgnoreList` | `priorities.ignore_list` | `[]string` | `["[dub]","[raw]","[hardcoded]","[hc]","re-encode"]` | Substrings (case-insensitive) that discard a release entirely |

Items absent from a list rank worst (sent to the end). Edited via the `#/priorities` screen, persisted through the regular `GET/PUT /api/v1/config` endpoints.

## Download Path

There is no `save_path` field. The download/seeding working directory is **derived** from `completed_anime_path` at every use, via `Config.DownloadPath()` (`internal/files/filemanager.go`):

```
<completed_anime_path>/.torrents
```

Torrents download and keep seeding there (rain's `DataDir`, layout `<DownloadPath()>/<torrent-id>/...`); on completion, video files are hardlinked out into `completed_anime_path` itself (the Jellyfin library). Because the download directory is a subfolder of the library, the two are guaranteed to share a filesystem — the old "must be on the same volume" requirement is now an architectural invariant rather than something the user can misconfigure. `Librarian.ProbePath(completed_anime_path)` still runs (on config save and on every verification pass) to catch filesystems that don't support hardlinks at all (exFAT/FAT32, some SMB shares), unrelated to the cross-volume case.

**Automatic migration:** installations upgrading from a version that still had a configured `save_path` are migrated automatically and idempotently by `daemon.MigrateSavePath` (`internal/daemon/migration.go`), which runs at boot and at the top of every verification pass. It moves (renames, same filesystem) each torrent's data directory from the old `save_path` into the derived download path, then clears `SavePath` and persists the config. See decisions.md #31.

## Removed Field

`qbittorrent_url` (and its `QBITTORRENT_URL` env override) was **removed** when the external qBittorrent dependency was replaced by an embedded BitTorrent client. Old configs that still contain the key load fine (JSON ignores unknown fields) and the key disappears on the next save.

`save_path` was **removed** as a user-configurable field (see "Download Path" above). It survives in the `Config` struct only as a legacy, `omitempty` field read by the migration; `PUT /config` always zeroes it, and it no longer appears in `config.json` once migrated.

## Required Fields

Daemon checks these in `isConfigComplete()` (`daemon/helpers.go`) before starting the verification loop:
- `anilist_usernames` — at least one
- `completed_anime_path`

If missing, daemon opens browser to `http://localhost:<port>/#/config?missingConfig=true`.

## Validation (API)

`handleUpdateConfig()` in `endpoint_config.go` validates:
- `anilist_usernames` — at least one entry (after legacy-field migration), `completed_anime_path` — non-empty
- `completed_anime_path` must support hardlinks — verified with a single-path probe (`Librarian.ProbePath`); a filesystem without hardlink support is rejected with HTTP 400
- `check_interval`, `max_episodes_per_anime` — > 0
- `episode_retry_limit`, `watched_episodes_to_keep`, `max_concurrent_downloads` — >= 0
- `save_path` is always zeroed on the incoming config before it is persisted, regardless of what the client sends

## Webhook Template Variables

Available in `url`, `headers` values, and `body`:

| Variable | Value |
|----------|-------|
| `{{title}}` | Short event label (e.g. "Novo episódio detectado") |
| `{{message}}` | Full sentence with anime name and episode number |
| `{{anime_name}}` | Anime title |
| `{{episode}}` | Episode number as string |
| `{{reason}}` | Failure reason (for `download_failed`); empty for other events |
| `{{quality}}` | Always empty — not tracked at hook point |
| `{{file_path}}` | Always empty — not tracked |
| `{{timestamp}}` | Current time formatted as `2006-01-02 15:04` |

## Loading Behavior

`LoadConfigs()` starts from `getDefaultConfig()`, then unmarshals `config.json` on top. Fields missing from JSON keep their defaults. If file is empty, corrupt, or missing, defaults are written to disk.
