# WebUI Guide

This guide covers the AutoAnimeDownloader web interface.

## Overview

The WebUI is a Svelte single-page app embedded directly in the daemon binary — no separate installation or server needed. It uses hash-based routing, so URLs look like `http://localhost:8091/#/status`.

## Accessing the WebUI

After starting the daemon, open **http://localhost:8091** in your browser.

## Pages

### Status (`/` or `/status`)

The main dashboard:

- **Daemon card** — current status (stopped/running/checking), last check time, next check countdown, and Start/Stop/Force-check controls
- **Library card** — total anime and episode counts
- **Disk card** — free/total space on the library's filesystem (`completed_anime_path`, where torrents also download), with a warning color when space is low
- **Monitored animes** — searchable table of tracked anime (progress, last download, blacklist status). Type is a radio (Airing / Finished / Standalone / Blacklist, or All); Unwatched is an independent toggle so Finished+Unwatched is the binge queue. Counts follow search + Unwatched. Active filters persist in the querystring (`q`, `unwatched=1`, `type`); the X next to `N of M` clears them all. Click a row to open its detail page

Status changes push over WebSocket, so the page updates live without a refresh.

### Anime Detail (`/status/:id`)

Per-anime episode list, opened by clicking an anime on the Status page:

- Episode table (episode number, Anilist watch status, downloaded state)
- Header **Replace torrent** opens the [torrent picker](#torrent-picker) (`/status/:id/torrents`)
- Per-episode actions: download, delete, re-download, replace with a manual magnet link (the
  per-episode modal stays here)
- Bulk actions on selected episodes: download, delete, release (unpin from retry queue)
- File list of the torrent attached to an episode, when metadata is in

### Add anime (`/add`)

Search AniList and add a title as a **standalone** anime (tracked without being on any AniList
list). An AniList account is optional; this is the entry point for an install with none.

Clicking Add on a card first searches ranked whole-title torrents. An empty first search adds
without opening the picker (singles, or track-with-zero for an unreleased title). A non-empty
list, or Nyaa being down, opens the [torrent picker](#torrent-picker).

### Torrent picker (`/add/:id/torrents`, `/status/:id/torrents`)

Confirm or swap the ranked whole-title torrent before anything is saved. At most 8 rows; the
first is the auto-pick. Selecting a row loads its Nyaa description on demand (the other rows
are not fetched). The query field is a Nyaa `q` and is not saved. Magnet paste lives at the
bottom of this page, not in a dialog.

Confirm on Add posts the selected magnet (or the pasted one) and returns to Add. Confirm on
Replace uses the existing replace action and returns to the anime page. Cancel saves nothing.
The per-episode replace modal stays on Anime Detail.

### Downloads (`/downloads`)

Live torrent table for the embedded client: progress, speeds, queue order, pause / resume /
announce / prioritize / delete (single or bulk). Expanding a row shows the files inside the
torrent.

### Config (`/config`)

Daemon configuration, grouped into sections: Library, Lists (Anilist and MyAnimeList usernames), Downloads and Torrent search (advanced). Covers paths, check interval, episode limits/retries, and download/delete rules. The **Completed Anime Path** is the only required setting: torrents download and seed in `<path>/.torrents` and completed episodes are hardlinked into the library — saving a path whose filesystem doesn't support hardlinks (exFAT/FAT32, some SMB shares) is rejected. There is no qBittorrent setting: the torrent client is embedded. See [Config Reference](../agents/config.md) for the full field list and defaults.

If required config is missing, the daemon opens this page automatically with a `?missingConfig=true` banner.

### Priorities (`/priorities`)

Reorderable lists that drive torrent ranking during search (fansub groups, resolutions, sources, codecs, audio, an ignore list, and the order criteria are applied in). Sources rank movies and series whose AniList media status is FINISHED; audio stays movies-only. Restore-defaults is how an existing install picks up the new `criteria_order`. See [Config Reference](../agents/config.md#fields) for what each list does.

### Notifications (`/notifications`)

Webhook configuration: add/edit/remove webhook presets (name, URL, method, headers, body template), pick which events trigger each one (new episode, download failed, download completed, episode deleted), and send a test request. Presets include the usual notification services plus **Jellyfin** and **Plex** — those two fire a library scan on `download_completed` and `episode_deleted` instead of a message (fill in the URL, the token and, for Plex, the library section id). Template variables like `{{title}}`/`{{message}}` are documented in [Config Reference](../agents/config.md#webhook-template-variables).

### Logs (`/logs`)

Real-time daemon logs over WebSocket:

- Line count and level filter (all/debug/info/warn/error), text search
- Autoscroll and live-tail toggles, copy-to-clipboard
- Manual reload button

### Report (`/report`)

Builds a redacted diagnostics bundle (environment, config, log, optional crash file) to attach to
a GitHub issue. Consent is the act of attaching the file; there is no backend. The tray menu and
the crash banner on Status also land here. See [decisions.md #102](../agents/decisions.md#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file).

## Troubleshooting

**Blank page / WebUI not loading**
- Confirm the daemon is running and reachable at `http://localhost:8091/api/v1/status`
- Check the browser console for errors
- Hard refresh (`Ctrl/Cmd + Shift + R`) if styling looks broken — usually a stale cached bundle

**Status/logs not updating live**
- Check the WebSocket connection isn't blocked by a firewall or proxy
- Refresh the page to reconnect

## See Also

- [CLI Guide](cli.md) — command-line equivalent for most of these actions
- [Config Reference](../agents/config.md) — full config field reference
