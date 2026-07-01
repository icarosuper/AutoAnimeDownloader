# Config Reference

All fields in `config.json` (`~/.autoAnimeDownloader/config.json` on Linux, `%APPDATA%\AutoAnimeDownloader\config.json` on Windows).

Struct defined in `src/internal/files/filemanager.go`. Defaults set in `getDefaultConfig()`.

## Fields

| Field | JSON key | Type | Default | Description |
|-------|----------|------|---------|-------------|
| `SavePath` | `save_path` | `string` | `""` | Directory where torrents are saved. **Required** for daemon to work |
| `CompletedAnimePath` | `completed_anime_path` | `string` | `""` | Directory to move finished anime to. Empty = don't move |
| `AnilistUsername` | `anilist_username` | `string` | `""` | Anilist username to sync watch list from. **Required** |
| `CheckInterval` | `check_interval` | `int` | `10` | Minutes between verification loops. Must be > 0 |
| `QBittorrentUrl` | `qbittorrent_url` | `string` | `"http://127.0.0.1:8080"` | qBittorrent WebUI URL. **Required**. Can be overridden by `QBITTORRENT_URL` env var |
| `MaxEpisodesPerAnime` | `max_episodes_per_anime` | `int` | `12` | Max saved episodes per anime before oldest are deleted. Must be > 0 |
| `EpisodeRetryLimit` | `episode_retry_limit` | `int` | `5` | Max magnet links to try per episode before giving up. Must be >= 0 |
| `DeleteWatchedEpisodes` | `delete_watched_episodes` | `bool` | `true` | Whether to auto-delete episodes marked as watched on Anilist |
| `WatchedEpisodesToKeep` | `watched_episodes_to_keep` | `int` | `0` | Number of watched episodes to keep before deleting. 0 = delete all watched. Must be >= 0 |
| `ExcludedList` | `excluded_list` | `string` | `""` | Name of Anilist custom list to exclude from downloads |
| `RenameFilesForJellyfin` | `rename_files_for_jellyfin` | `bool` | `false` | Rename downloaded files to Jellyfin-compatible format |
| `DownloadStatuses` | `download_statuses` | `[]string` | `["CURRENT", "REPEATING"]` | Anilist statuses to download. Valid values: `CURRENT`, `REPEATING`, `COMPLETED`, `PAUSED`, `DROPPED`, `PLANNING` |
| `DeleteStatuses` | `delete_statuses` | `[]string` | `[]` | Anilist statuses to auto-delete episodes from. Same valid values as above |
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

## Required Fields

Daemon checks these in `isConfigComplete()` before starting the verification loop:
- `anilist_username`
- `save_path`
- `qbittorrent_url`

If missing, daemon opens browser to `http://localhost:<port>/#/config?missingConfig=true`.

## Validation (API)

`handleUpdateConfig()` in `endpoint_config.go` validates:
- `anilist_username`, `save_path`, `qbittorrent_url` — non-empty
- `check_interval`, `max_episodes_per_anime` — > 0
- `episode_retry_limit`, `watched_episodes_to_keep` — >= 0

## Webhook Template Variables

Available in `url`, `headers` values, and `body`:

| Variable | Value |
|----------|-------|
| `{{title}}` | Short event label (e.g. "Novo episódio detectado") |
| `{{message}}` | Full sentence with anime name and episode number |
| `{{anime_name}}` | Anime title |
| `{{episode}}` | Episode number as string |
| `{{quality}}` | Always empty — not tracked at hook point |
| `{{file_path}}` | Always empty — not tracked |
| `{{timestamp}}` | Current time formatted as `2006-01-02 15:04` |

## Loading Behavior

`LoadConfigs()` starts from `getDefaultConfig()`, then unmarshals `config.json` on top. Fields missing from JSON keep their defaults. If file is empty, corrupt, or missing, defaults are written to disk.
