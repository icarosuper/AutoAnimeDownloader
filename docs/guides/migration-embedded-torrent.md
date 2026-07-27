# Migration: Embedded BitTorrent Client (qBittorrent removed)

AutoAnimeDownloader no longer talks to an external **qBittorrent**. It now embeds a
BitTorrent client (`github.com/cenkalti/rain/v2`) directly in the binary. The app is a
single self-contained process — you no longer install, run, or configure qBittorrent.

## What you must do when upgrading

1. **Set `completed_anime_path` (now required).** The daemon will not run until it is set.
   If it is empty, the app opens the config page and waits. Set it in the WebUI, CLI
   (`config set completed_anime_path <path>`), or `config.json`.
2. **Put `completed_anime_path` on the SAME volume/filesystem as `save_path`.** Completed
   episodes are **hardlinked** into the completed folder (the Jellyfin library), and
   hardlinks cannot cross filesystems. The config-save step verifies this and rejects
   cross-device paths with a clear error. (Docker: make sure both paths are under the same
   bind mount / volume.)
3. **Point your media server at `completed_anime_path`.** This is the biggest behavior
   change — see below.

## Behavior changes

- **The library is now `completed_anime_path`, for *all* episodes.** Previously only
  *finished* animes were moved there and *ongoing* episodes were renamed in place in
  `save_path`. Now **every** completed episode (ongoing or finished) is hardlinked into
  `completed_anime_path`. `save_path` is only the download/seeding working directory. If
  your Jellyfin/Plex library pointed at `save_path`, repoint it at `completed_anime_path`,
  or it will look empty after upgrading.
- **Files keep seeding from `save_path`** and are hardlinked (not copied) into the library,
  so no extra disk space is used. Deleting a watched episode removes **both** links to
  actually free space.
- **Batch torrents:** a watched episode that belongs to a multi-episode batch keeps its
  library link until the *entire* batch torrent is removed (all its episodes deleted). This
  is intentional — see `docs/agents/decisions.md`.

## One-time migration cost

- **In-progress downloads are not imported from qBittorrent.** On first run the embedded
  client starts empty. The daemon re-adds the same magnets, and because the on-disk layout
  differs from qBittorrent's, rain **re-downloads** the torrents from scratch. Downloaded
  episode records (`downloaded_episodes`) stay valid once the torrents are present again
  (the join key is the infohash). Expect a one-time bandwidth cost.
- **Orphaned qBittorrent torrents.** Torrents in qBittorrent's old `autoAnimeDownloader`
  category are left behind. Remove them from qBittorrent manually if you want.

## Config and env changes

- `qbittorrent_url` config field is **removed**. Old `config.json` files still load (unknown
  JSON fields are ignored); the key disappears on the next save.
- The `QBITTORRENT_URL` environment variable / override is **removed**.
- Resume data (piece bitfields) lives in `~/.autoAnimeDownloader/session.db`
  (`%APPDATA%\.autoAnimeDownloader\session.db` on Windows), kept outside `save_path` so it
  survives a `save_path` change.

## Networking

- rain listens on a default port range (20000–30000). There is **no UPnP/NAT-PMP**; DHT and
  PEX work without any setup, but for best inbound connectivity you may optionally forward a
  port to the machine. A fixed configurable port is a possible future enhancement.

## Not changed

- Webhook notifications are unchanged. The completion event still uses the key
  `download_completed` — existing webhook configurations keep working.
- One nuance: the **event keys** are unchanged, but the `{{reason}}` value of `download_failed`
  when the client rejects a torrent changed from `"qBittorrent rejeitou o torrent"` to
  `"torrent rejeitado"` (`notifications/notifications.go`). Only relevant if you string-match
  on `{{reason}}` downstream; the event key and the payload shape are the same.
