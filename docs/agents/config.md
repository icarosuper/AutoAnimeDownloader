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

## Loading Behavior

`LoadConfigs()` starts from `getDefaultConfig()`, then unmarshals `config.json` on top. Fields missing from JSON keep their defaults. If file is empty, corrupt, or missing, defaults are written to disk.
