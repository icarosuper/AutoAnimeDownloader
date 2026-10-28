# Dashboard: Recents shelf, clickable last-check rows

**Date:** 2026-10-23
**Status:** designed, not implemented — `v2.8.0`
**Origin:** the Status anime list is a scan of everything tracked. Newly landed, still-unwatched
episodes hide in that table. The last-check report names animes as dead text.
**Scope:** `internal/api/endpoint_animes.go` (`recent` on `AnimeInfo`), `internal/files`
(`recent_shelf_days`), `frontend` (Status shelf + last-check links, Config field, i18n). No new
endpoint. No CLI command — `config set` already covers the field. The Behind table filter was
dropped; type filters live in [status-type-filters](2026-10-28-status-type-filters-design.md).

---

## The problem

A weekly episode that finished downloading two hours ago is one row among fifty, sorted by last
download only if the user left that sort on. There is no surface that answers "what just landed
that I have not watched yet". Last-check failures name the anime and stop there.

## What this is not

- Not a Netflix home. The existing table stays. Recents is one horizontal shelf above it.
- Not "newly added to tracking". An anime tracked for years appears when a new episode downloads
  inside the window and is still unwatched. There is no `tracked_since`.
- Not a download inbox dismissed by opening the detail. Opening the card is inspection. Dismiss
  is **watch progress** (AniList list progress, or standalone `AnimeSettings.Progress` / "watched
  here").
- Not a new API resource. `GET /animes` already walks every saved episode. Recents is derived
  there.
- Not a second "searching" shelf for titles with zero files.

---

## Recents: payload and rules

`GET /animes` gains an optional object on `AnimeInfo`:

```json
"recent": {
  "ranges": [{"from": 1, "to": 3}, {"from": 5, "to": 5}],
  "is_batch": false,
  "downloaded_at": "2026-10-23T14:02:00Z"
}
```

Omitted when the anime has nothing in the window. The list endpoint stays an array of `AnimeInfo`.

Computed **after** the AniList/MAL/standalone merge, because `episodes_watched` is only known
then. A second pass over the already-loaded `episodes` slice; no extra I/O.

### Qualifying episode

All of:

1. `EpisodeNumber > 0`
2. `EpisodeNumber > EpisodesWatched`
3. `DownloadDate >= now - recent_shelf_days * 24h`

`recent_shelf_days == 0` attaches `recent` to nobody (shelf off).

### One card per anime

- `ranges`: qualifying episode numbers, sorted, collapsed into inclusive runs
  (`1,2,3,5` → `[{1,3},{5,5}]`).
- `downloaded_at`: max `DownloadDate` among those episodes (queue order).
- `is_batch`: the qualifying set shares one `EpisodeHash` **and** has two or more episodes
  (same detection the detail screen uses: several episodes on one hash are a pack). A single
  leftover from a pack is a normal "Episode N".

Watching episode 1 of a 1–3 bundle leaves `ranges: [{2,3}]`. Watching all of them drops `recent`.

A pack 1–12 with progress 5 and the whole pack still inside the window: `ranges: [{6,12}]`,
`is_batch: true`. Caption uses the unwatched remainder, not the declared `BatchStart`/`BatchEnd`.

`EpisodeNumber == 0` is skipped (cannot sit in a range).

### Queue

The handler does **not** cap. Every qualifying anime carries `recent`. The Status shelf sorts by
`downloaded_at` descending and keeps the first **12**. Rank 13+ still have the field (the table
and a future CLI consumer can see it). 12 is a frontend constant, not a config field.

### Clock and cache

`now` is the request time. AniList progress on this endpoint can be up to 60s stale
(`GetFrontendAnimeList` cache) plus the 30s poll — the same lag the Unwatched filter already
has. Standalone "watched here" writes `AnimeSettings` and is visible on the next `GET /animes`.

Existing downloads older than the window never appear. After upgrade the shelf is empty until
something new lands; that is intended, not a backfill.

---

## Status UI

Placement: after the speed / library / disk grid, before the Animes table. It is part of the
library, not of the daemon header.

Heading: **Recently downloaded** / **Baixados recentemente**. No subtitle about tracking — the
criterion is unwatched files in the window, including titles followed for years.

**Empty:** the section is absent. Same rule as the last-check report: a permanent "0 recent"
card trains the eye to skip it.

Each card is a link to `#/status/{id}` (same target as the table row):

- cover
- red `NEW` badge (presence on the shelf is the news)
- title
- caption from `ranges` + `is_batch` (i18n, never a server string)
- relative time of `downloaded_at` via the existing `formatTimeAgo`

Caption:

| `ranges` | `is_batch` | English | pt-BR |
|---|---|---|---|
| one point `[5,5]` | false | Episode 5 | Episódio 5 |
| one run `[1,3]` | false | Episodes 1–3 | Episódios 1–3 |
| one run `[1,12]` | true | 1–12 · pack | 1–12 · lote |
| several | either | Episodes 1–3, 5 | Episódios 1–3, 5 |

Horizontal overflow scrolls. The shelf does not filter the table.

Recents is the downloaded-and-still-unwatched gap inside the window. The row still shows its
Behind chip (`episodes_pending`); there is no Behind control on the filter strip — see
[status-type-filters](2026-10-28-status-type-filters-design.md).

### Last-check rows

Each problem **and** each limit becomes one `<a href="#/status/{anime_id}">`: cover (looked up
from the already-loaded animes list; placeholder if missing), name, episodes, reason, chevron.
Keyboard and hover match a table row. `anime_id == 0` stays plain text.

---

## Config

`recent_shelf_days` `int`, default **3**, valid **0..30**. `0` turns the shelf off. `> 30` is
`400 VALIDATION_ERROR`.

Lives in the **Downloads** group on `#/config`, next to `check_interval`. Helper text: days a
finished download stays on Recents while still unwatched; `0` disables.

`config set recent_shelf_days N` is enough for the CLI. No `animes recent` command.

---

## Errors

| Case | Behaviour |
|---|---|
| `recent_shelf_days` 0 | `recent` omitted everywhere; shelf hidden |
| `recent_shelf_days` > 30 or < 0 | `PUT /config` 400, field unchanged |
| Qualifying set empty after a watch | `recent` omitted; card gone on next poll |
| Last-check `anime_id` 0 | row is not a link |
| Cover missing | card / issue row still works; empty cover slot |
| More than 12 qualifying | shelf shows 12; the rest keep `recent` on the payload |

A failed AniList merge already skips orphan refresh. Recents still runs on saved episodes and
whatever progress the merge did fill in.

---

## Tests

**API** (`endpoint_animes_test.go`, plus a focused helper test next to the builder)

- Inside window, unwatched → `recent` present with collapsed ranges and max `downloaded_at`.
- `EpisodeNumber <= EpisodesWatched` → omitted even inside the window.
- Older than the window → omitted even if unwatched.
- `recent_shelf_days == 0` → omitted.
- Three consecutive singles → one range `1–3`.
- Hole (`1,2,5`) → two ranges.
- Shared hash, two+ qualifying → `is_batch true`; progress into the pack shrinks the range.
- One qualifying episode from a pack hash → `is_batch` false, single-episode caption data.
- Builder takes an explicit `now` so tests do not sleep.

**Frontend**

- Caption helper: the four rows of the table above, both locales.
- Queue: 15 animes with `recent` → 12 cards, newest `downloaded_at` first.
- Status: no `recent` anywhere → the shelf node is absent.
- Last-check problem with `anime_id` is a link to `#/status/{id}`; `anime_id` 0 is not.

**Config**

- Default 3 on a config that never had the key (`LoadConfigs` unmarshals over
  `getDefaultConfig()`).
- PUT 3 succeeds; PUT 31 and PUT -1 are 400.

No new integration test. No CLI test beyond the existing `config set` passthrough.

---

## Files this will touch (planning)

- `files/filemanager.go` — `RecentShelfDays` on `Config`, default 3.
- `api/endpoint_config.go` — 0..30 validation.
- `api/endpoint_animes.go` — `Recent` on `AnimeInfo`, builder after the merge.
- `frontend/src/lib/api/client.ts` — `recent` on `AnimeInfo`.
- `frontend/src/lib/utils/status.ts` — shelf sort/cap, caption helper.
- `frontend/src/routes/Status.svelte` — shelf, last-check links.
- `frontend/src/routes/Config.svelte` — Downloads field.
- `frontend/messages/en.json`, `pt-BR.json`.
- Tests as above.
- `docs/agents/architecture.md` (GET `/animes` row), `docs/agents/config.md`,
  `docs/guides/webui.md`. A `decisions.md` entry for "recent is derived after the merge, from
  unwatched episodes in the window" if the grouping surprises.

---

## Out of scope

- Replacing the table with shelves.
- Configurable cap.
- `tracked_since` / "new to the list".
- Dismiss-on-open or a dismiss button.
- `GET /animes/recent`.
- Changing the AniList 60s cache to make watch-dismiss faster.
- CLI listing of Recents.
