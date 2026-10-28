# Status: type filter, counts, URL, clear-all

**Date:** 2026-10-28
**Status:** designed, not implemented
**Origin:** the Status table mixes currently-airing titles with finished catch-up. The strip
already has search, Unwatched, and Standalone; Standalone is origin, Unwatched is progress, and
there is no airing-state slice. A three-state cycle on one chip was rejected.
**Scope:** `internal/api/endpoint_animes.go` (`media_status` on `AnimeInfo`), `frontend` (Status
strip, `filterAnimes`, view-state querystring, ActionMenu trigger slot, i18n). No new endpoint.
No CLI command — `animes` already prints `GET /animes`. No config field.

Independent of the recents shelf. May ship in the same minor.

---

## The problem

A library of fifty titles is one table. The user wants "what is airing this week" or "finished
shows I have not finished watching" without scrolling. Unwatched already answers progress.
Nothing answers media airing state. Reloading the dashboard drops search and chips. Clearing
three independent controls takes three clicks.

## What this is not

- Not a Behind / `episodes_pending` filter. That chip is **removed** from the recents spec; it
  is not in this one either.
- Not a server-side filter. `GET /animes` still returns the full list. The front recuts it,
  same as search and Unwatched today.
- Not a ResizeObserver. The hybrid control switches at the **768px** viewport breakpoint the
  shell already uses (`md:`, ActionMenu `max-width: 767px`).
- Not a new AniList query. `ml.Media.Status` is already on the merge payload.
- Not sort-in-URL. Table sort stays local, as it is today.

---

## Payload

`GET /animes` gains an optional string on `AnimeInfo`:

```json
"media_status": "RELEASING"
```

Values are AniList `MediaStatus`: `RELEASING`, `HIATUS`, `FINISHED`, `CANCELLED`,
`NOT_YET_RELEASED`. `omitempty` when empty.

Set in `mergeAniListAnimes` and `refreshOrphanAnimes` from `ml.Media.Status` (the same places
that already copy `NextAiringAt`). Standalone entries that went through `GetMediaByIDs` carry
it too.

Do **not** derive this from `next_airing_at`. That timestamp is also absent during a hiatus
between seasons.

TypeScript `AnimeInfo` gets `media_status?: string`.

---

## Filter model

Two axes. `filterAnimes` in `lib/utils/status.ts` takes both (the boolean `filterStandalone`
argument goes away).

```ts
export type AnimeTypeFilter = 'all' | 'airing' | 'finished' | 'standalone' | 'blacklisted'

filterAnimes(animes, search, { unwatched?: boolean, type?: AnimeTypeFilter })
```

Defaults: `unwatched: false`, `type: 'all'`.

| Axis | Values | Combines with the other axis |
|---|---|---|
| Type (radio) | `all` · `airing` · `finished` · `standalone` · `blacklisted` | yes |
| Progress | `unwatched` on/off | yes |

Type buckets (a row may match more than one; the radio picks the slice):

| Control | Keep when |
|---|---|
| Tudo / `all` | always (search and Unwatched still apply) |
| Saindo / `airing` | `media_status` is `RELEASING` or `HIATUS` |
| Finalizados / `finished` | `media_status` is `FINISHED` or `CANCELLED` |
| Avulsos / `standalone` | `is_standalone` |
| Blacklist / `blacklisted` | `is_blacklisted` |

`NOT_YET_RELEASED` and missing `media_status` stay out of `airing` and `finished`. They still
appear in `all`, and in `standalone` / `blacklisted` when those flags match.

Unwatched is the existing rule: drop a row when
`episodes_watched >= episodes_released && episodes_released > 0`.

Search is unchanged (any title, NFKD).

Shared helpers next to `filterAnimes` so the strip and the table cannot disagree:

```ts
isAiringStatus(status?: string): boolean   // RELEASING | HIATUS
isFinishedStatus(status?: string): boolean // FINISHED | CANCELLED
```

---

## Counts

Every type segment (and every ActionMenu item) shows how many rows **would remain if that type
were selected now**, given the current search and Unwatched. `all` is that count with
`type: 'all'`.

The Unwatched chip shows how many rows would remain if Unwatched were on, given the current
search and type.

A bucket at 0 still renders. Do not hide empty segments.

`N of M` (`status_x_of_y`) is visible when search is non-empty, Unwatched is on, or
`type !== 'all'`.

---

## URL

Same contract as Logs / Downloads. Defaults omitted; a clean view is `#/status` with no `?`.
`replace` on the hash so filter clicks do not stack history. Unknown values fall back to
defaults.

| Param | Default | Notes |
|---|---|---|
| `q` | (empty) | search text |
| `unwatched` | off | present as `1` when on |
| `type` | `all` | `airing` · `finished` · `standalone` · `blacklisted` |

`encodeStatusView` / `decodeStatusView` live next to `filterAnimes`.

Opening `#/status/{id}` and going back restores the previous hash, querystring included,
because the filtered URL is the history entry.

Table sort is **not** in the querystring.

---

## UI

Layout stays the current Status toolbar: title on the left; `N of M` + clear-all on the header
row when anything is active; search · Unwatched · type control on the right (stacked under the
title on mobile, as today).

**Unwatched** stays the Eye chip, `aria-pressed`, accent when on. Combines with type.

**Standalone only** leaves the strip. Origin becomes the Avulsos segment.

### ≥ 768px (`md:`)

A connected segmented control, `role="radiogroup"`, `aria-label` from i18n. Five
`role="radio"` options:

`Tudo N | Saindo N | Finalizados N | Avulsos N | Blacklist N`

Selected segment uses the Downloads-pill accent tokens (`border-accent-tint/28
bg-accent-tint/12 text-accent`). No icons on segments.

Clicking the already-selected type does nothing. **Tudo** is how type returns to `all`.

The ActionMenu stays `md:hidden` (`display: none`). Both controls may exist in the DOM; only
one is exposed. Status does not need its own `matchMedia`.

### < 768px

Search stays on its own row (already). Unwatched chip + an ActionMenu whose trigger is the
current type label and count (`Saindo 12 ▾`). Accent on the trigger when `type !== 'all'`.
Items are the same five options, each with its count. Picking one sets `typeFilter` and
closes.

ActionMenu today only has a kebab trigger. Add a `trigger` slot that replaces the inner
content of the existing `<button>` (not a nested button), and a `triggerClass` that replaces
the kebab padding classes when the slot is used. Without the slot, the kebab is unchanged.
Status keeps this ActionMenu `md:hidden`. Below 768px ActionMenu's own `max-width: 767px`
query is true, so the sheet path is the one that shows. The kebab-only callers are unchanged.

### Clear-all

Next to `N of M`, same visibility condition. A small X, `aria-label` “Clear filters”. One
click: `q = ''`, `unwatched = false`, `type = 'all'`. Querystring becomes empty. Search keeps
its own X (clears text only).

Empty filtered list: the existing empty copy.

The recents shelf, when it exists, **ignores** these filters.

---

## i18n

`en` + `pt-BR`. Reuse `status_filter_unwatched` and `status_filter_standalone`. Add:

| Key | en | pt-BR |
|---|---|---|
| `status_filter_all` | All | Tudo |
| `status_filter_airing` | Airing | Saindo |
| `status_filter_finished` | Finished | Finalizados |
| `status_filter_blacklisted` | Blacklist | Blacklist |
| `status_filter_type` | Filter by type | Filtrar por tipo |
| `status_clear_filters` | Clear filters | Limpar filtros |

---

## Errors / edges

| Case | Behaviour |
|---|---|
| `media_status` omitted | out of Airing/Finished; in All; in Standalone/Blacklist if the flag matches |
| Standalone + airing + blacklisted | counted in all three buckets; radio shows one slice; row chips unchanged |
| Type bucket count is 0 | segment / menu item still visible |
| `type=foo` in the URL | treated as `all` |
| AniList merge failed | existing list behaviour; `media_status` stays whatever the last successful merge wrote, or omitted |

---

## Docs (same change as the code)

With the code: `docs/agents/architecture.md` (one line on the `GET /animes` row: `media_status`
is AniList media status, copied at merge; the dashboard type filter is client-side) and
`docs/guides/webui.md` (type radio + Unwatched, querystring, clear-all).

Already done with this spec: specs README, TODO v2.8.0 bullet, Behind removed from the recents
spec.

No `decisions.md` entry. Radio + independent toggle and default-omitted querystring already
exist on Downloads and Logs.

No CLI command, no `config.json` field, no integration test.

---

## Tests

**API** (`endpoint_animes_test.go`)

- Merge copies `Media.Status` onto `media_status`.
- Orphan refresh does the same.
- Empty status is omitted from the JSON.

**Vitest** (`status.utils.test.ts`)

- Each type keeps the matching rows; `all` keeps everyone (search / Unwatched still apply).
- HIATUS → airing; CANCELLED → finished; `NOT_YET_RELEASED` and missing status → only `all`
  (plus flags).
- Unwatched combines with each type.
- Search × type × Unwatched.
- The old `filterStandalone` case becomes `type: 'standalone'`.
- Counts: with Unwatched on and a search term, the Finished count equals
  `filterAnimes(..., { unwatched: true, type: 'finished' }).length`.

**Vitest** (encode/decode next to the filter)

- Default view encodes to `''`.
- `type=airing&unwatched=1&q=beta` round-trips.
- Unknown `type` and leftover params decode as defaults / ignored.

**Vitest** (`ActionMenu.test.ts`)

- With a `trigger` slot, the slotted content is inside the trigger button; click still opens
  the menu.
- Without the slot, the kebab trigger is unchanged.

**Component / smoke Status**

- At 800px the radiogroup is visible and the type ActionMenu is not; at 700px the reverse.
- Clicking Airing filters the table; All restores it.
- Unwatched + Airing combine; visible counts match the filtered length.
- `N of M` and the clear X appear only when something is active; the X resets search, type,
  and Unwatched.
- `#/status?type=airing&unwatched=1&q=beta` restores the strip.
- Filtered empty state.
- The "Standalone only" chip is gone; Standalone is a type option.
