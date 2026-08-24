# Migration: Embedded BitTorrent Client (qBittorrent removed)

AutoAnimeDownloader no longer talks to an external **qBittorrent**. It now embeds a
BitTorrent client (`github.com/cenkalti/rain/v2`) directly in the binary. The app is a
single self-contained process — you no longer install, run, or configure qBittorrent.

## What you must do when upgrading

1. **Set `completed_anime_path` (now required).** The daemon will not run until it is set.
   If it is empty, the app opens the config page and waits. Set it in the WebUI, CLI
   (`config set completed_anime_path <path>`), or `config.json`.
2. **Nothing to do about `save_path`.** It is no longer a configurable field: the
   download/seeding directory is **derived** as `<completed_anime_path>/.torrents`, so the
   two always share a filesystem by construction. A pre-existing `save_path` in an old
   `config.json` is migrated automatically on first boot (`daemon.MigrateSavePath`) and then
   cleared. What is still checked is that `completed_anime_path` sits on a filesystem that
   supports hardlinks at all — exFAT/FAT32 and some SMB shares are rejected on save.
3. **Point your media server at `completed_anime_path`.** This is the biggest behavior
   change — see below.

## Behavior changes

- **The library is now `completed_anime_path`, for *all* episodes.** Previously only
  *finished* animes were moved there and *ongoing* episodes were renamed in place in the old
  `save_path`. Now **every** completed episode (ongoing or finished) is hardlinked into
  `completed_anime_path`. If your Jellyfin/Plex library pointed at the old `save_path`,
  repoint it at `completed_anime_path`, or it will look empty after upgrading.
- **Files keep seeding from the download directory** (`<completed_anime_path>/.torrents`,
  hidden from the Jellyfin scanner) and are hardlinked (not copied) into the library, so no
  extra disk space is used. Deleting a watched episode removes **both** links to actually
  free space.
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
- `save_path` is **removed** as a user-configurable field too — see `docs/agents/config.md`
  ("Download Path"). It survives in the struct only as a legacy `omitempty` field read by
  the migration, and disappears from `config.json` once migrated.
- Resume data (piece bitfields) lives in `~/.autoAnimeDownloader/session.db`
  (`%APPDATA%\.autoAnimeDownloader\session.db` on Windows), kept in the config folder
  rather than alongside the downloads so it survives a library move.

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
