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
- **Disk card** — free/total space on the save path's filesystem, with a warning color when space is low
- **Monitored animes** — searchable/filterable table of tracked anime (progress, last download, blacklist status). Click a row to open its detail page

Status changes push over WebSocket, so the page updates live without a refresh.

### Anime Detail (`/status/:id`)

Per-anime episode list, opened by clicking an anime on the Status page:

- Episode table (episode number, Anilist watch status, downloaded state)
- Per-episode actions: download, delete, re-download, replace with a manual magnet link
- Bulk actions on selected episodes: download, delete, release (unpin from retry queue)
- Custom search query override for that anime's Nyaa searches

### Config (`/config`)

Daemon configuration, grouped into sections: Anilist, Downloads, Automation, qBittorrent, Filters. Covers paths, check interval, qBittorrent URL, episode limits/retries, and download/delete rules. See [Config Reference](../agents/config.md) for the full field list and defaults.

If required config is missing, the daemon opens this page automatically with a `?missingConfig=true` banner.

### Priorities (`/priorities`)

Reorderable lists that drive torrent ranking during search (fansub groups, resolutions, sources, codecs, audio, an ignore list, and the order criteria are applied in). See [Config Reference](../agents/config.md#fields) for what each list does.

### Notifications (`/notifications`)

Webhook configuration: add/edit/remove webhook presets (name, URL, method, headers, body template), pick which events trigger each one (new episode, download failed, download completed), and send a test request. Template variables like `{{title}}`/`{{message}}` are documented in [Config Reference](../agents/config.md#webhook-template-variables).

### Logs (`/logs`)

Real-time daemon logs over WebSocket:

- Line count and level filter (all/debug/info/warn/error), text search
- Autoscroll and live-tail toggles, copy-to-clipboard
- Manual reload button

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
