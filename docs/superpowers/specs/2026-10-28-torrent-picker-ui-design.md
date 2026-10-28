# Torrent picker UI: identity, hierarchy, two confirms

**Date:** 2026-10-28
**Status:** designed, not implemented — `v2.7.2`
**Origin:** the whole-title picker shipped in `v2.7.0` without a real UI pass. The Nyaa query
looks like the AniList search, the description overflows, one confirm button eats a pasted
magnet, and a single surviving pack (Onimai) looks like Nyaa is empty.
**Scope:** `TorrentPicker.svelte`, picker i18n, `GET /animes/{id}/torrent-candidates` media
object, Nyaa description sanitizer (table tags), tests, `webui.md` / `architecture.md` /
decision **#111**. Same two routes. No new endpoint. No new CLI command (`animes candidates`
already returns this payload).

---

## The problem

The picker is a confirm step after AniList, and a Nyaa search, and a magnet escape hatch, on
one screen. Today those three jobs share chrome:

- The query input is pre-filled with the title, so the placeholder "Search on Nyaa…" never
  shows. There is no search button. The page title is "Choose torrent". The anime's name lives
  in the search box.
- One solid **Download this** takes `pastedMagnet.trim() || selected.magnet`. A paste silently
  overrides the highlighted row.
- Nyaa description HTML unwraps `<table>` (not in the allow-list), so markdown/HTML tables
  become a wall of text; long URLs overflow the card. Files have no hover.
- Back is a caption breadcrumb plus a ghost in a sticky footer next to the confirm. Easy to
  miss.
- The screen never says it lists **packs only**. One live batch looks like a Nyaa miss.

## What this is not

- Not the episode picker (`v2.9.0`). Singles stay off this screen.
- Not a Nyaa browser. Cap 8, packs-only (or movie), `min_seeders`, size ceiling, Priorities,
  custom `q` dropping variants — all stay ([decisions.md #108](../../agents/decisions.md#108-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)).
- Not a change to add-with-magnet coverage, replace coverage, or the empty-first-search skip.
- Not a second AniList round-trip. `PreviewWholeTitleTorrents` already called `GetMediaByID`.
- Not a year in the header. `GetMediaByID` does not ask for `seasonYear`; do not widen that
  shared query for this screen. Meta is format + episode count.

---

## Layout

One screen, two modes (add `#/add/:id/torrents`, replace `#/status/:id/torrents`). Desktop is
master/detail; below `md` the list stacks above the detail.

**Header**

- Text button, not a breadcrumb: add → `← Add anime` (`#/add`); replace → `← {title}`
  (`#/status/:id`).
- Cover (`Cover.svelte`, medium image, ~48×68) + kicker "Choose torrent" + the AniList title
  (English, else romaji) + meta `FORMAT · N episodes`.
- One hint line: "Pick the pack. The first is what the daemon would download."

**Nyaa search**

- Visible label "Nyaa search".
- Input with a persistent `Nyaa` badge. Value is the query the daemon used (or the custom `q`).
- **Search** button next to the field (same submit as Enter). Empty submit stays a no-op.
  The button is control/ghost chrome, not the solid accent.

**Count**

`{n} packs · single episodes stay off this screen`. Shown whenever the list loaded, including
`n = 1` and `n = 0` (empty copy already says "No packs, try another query").

**Master/detail**

- List column slightly wider (~1.15 / 0.85). First row is still Recommended / `aria-selected`.
- **Download this** (solid, the one accent on the screen) sits in a footer **inside the list
  card**, right-aligned. Disabled when there is no selected magnet (empty list, or Nyaa error
  with no rows). Independent of the magnet field.
- Detail card: description (scroll + overflow), files (hover), "Open on Nyaa". Detail failure
  still leaves the list and **Download this** usable.

**Magnet strip** (below the grid, not a sticky page footer)

- Label "Or paste a magnet".
- Field + ghost **Download magnet**. Button disabled while the field is blank (trim). Enabled
  paste does not clear or restyle the list selection and does not change **Download this**.
- No page-level sticky footer. No second **Back** next to the magnet.

Replace mode uses the same two labels. The kicker already says this is a torrent choice; the
replace POST stays `replaceAnimeWithMagnet`.

---

## Two confirms (decision #111)

| Control | Target | POST |
|---|---|---|
| **Download this** | selected row | add: magnet + `name` / `nyaa_id` / `size_bytes` from that row; replace: that magnet |
| **Download magnet** | trimmed field | add: `{ magnet }` only (coverage unknown, same as #108.5); replace: that magnet |

A non-empty paste **does not** win on **Download this**. That is the behavior change: the test
that POSTs the paste when clicking the list confirm is deleted and replaced by "list confirm
still POSTs the selected row when the magnet field is filled".

Both may be enabled at once. The click chooses. Either success still `pop` (add) or
`push(/status/:id)` (replace) and clears `pickerCache`.

---

## Media on the candidates payload

`GET /animes/{id}/torrent-candidates` already has the AniList media inside
`PreviewWholeTitleTorrents`. Return it so a reload of the picker URL still has identity (add
has not written a tracked anime yet, so `GET /animes/{id}` cannot be the source).

```json
{
  "query": "Kaguya-sama: Love is War",
  "media": {
    "title": "Kaguya-sama: Love is War",
    "cover_image": "https://…",
    "format": "TV",
    "episodes": 12
  },
  "items": [ ]
}
```

- `title`: English if set, else romaji (same as Add cards). Always present.
- `cover_image`: `coverImage.medium`. `omitempty` when empty; `Cover.svelte` draws the hatch.
- `episodes`: pointer / `omitempty` when AniList has none (movies).
- Built from the `GetMediaByID` already done. No extra AniList call. No `seasonYear`.
- Until `media` arrives (loading, or AniList miss on a reload), the header shows the kicker
  without a title and the replace back label is `← Choose torrent`.

`pickerCache` may hold `media` for the first paint from Add; a cache miss uses this object.
`animes candidates` already prints the response; the extra object shows up in `--json` and in
the human dump. No new CLI verb.

---

## Description HTML

`sanitizeDescriptionHTML` allow-list gains `table`, `thead`, `tbody`, `tfoot`, `tr`, `th`,
`td`, `hr`. Still no `style` / `class` / event handlers. `a.href` and `img.src` stay the only
attributes.

The detail pane wraps the HTML in a scroll box: `max-height`, `overflow-x-auto`,
`overflow-wrap: anywhere`, `img` `max-w-full`, `pre` `overflow-x-auto` + `whitespace-pre-wrap`,
tables with a border so unwrapped-no-longer tables read as tables. ASCII pipe-art that Nyaa
left as `<p>` stays text; this spec does not parse markdown tables.

File rows: `hover:bg-control` and name `text-heading` on hover. Truncate + `title` stay.

---

## i18n

New keys in `en.json` and `pt-BR.json` (Portuguese is the translation of the English string):

| Key | en |
|---|---|
| `picker_query_label` | Nyaa search |
| `picker_search` | Search |
| `picker_hint` | Pick the pack. The first is what the daemon would download. |
| `picker_pack_count` | `{count} packs · single episodes stay off this screen` |
| `picker_back_add` | ← Add anime |
| `picker_back_replace` | ← {title} |
| `picker_magnet_label` | Or paste a magnet |
| `picker_magnet_confirm` | Download magnet |

Existing `picker_confirm` ("Download this" / "Baixar este") stays on the list button.
`picker_query_placeholder` can stay as the input's accessible name / empty placeholder; the
visible label is `picker_query_label`. Drop the breadcrumb (`picker` no longer uses
`add_title` / `nav_status` as crumbs).

---

## Tests

**Frontend (`TorrentPicker.test.ts`)**

- Header shows `media.title`; back link is `← Add anime` on add and `← {title}` on replace.
- Search label + Search button exist; Enter still refetches; empty submit still does not.
- Pack count is in the document for a 1-row list and a 2-row list.
- **Download this** POSTs the selected row even when the magnet field is filled.
- **Download magnet** is disabled when the field is empty; when filled, POSTs `{ magnet }`
  only (no `name` / `nyaa_id` / `size_bytes`).
- Replace: list button still calls `replaceAnimeWithMagnet` with the selected magnet; magnet
  button with the pasted URI.
- File row has a hover class (presence of the class is enough; no visual regression suite).

**Go**

- `sanitizeDescriptionHTML` keeps a `<table>` (and a `<hr>`) and still strips `<script>`.
- Candidates handler JSON includes `media` from the AniList fixture already used by preview
  tests. `PreviewWholeTitleTorrents` return type grows the media identity; existing tests
  update.

---

## Docs (same change)

- `docs/guides/webui.md` — picker subsection: identity header, two confirms in two places,
  packs-only line, magnet strip.
- `docs/agents/architecture.md` — picker paragraph: no sticky footer; two buttons; `media` on
  the candidates payload.
- `docs/agents/decisions.md` — **#111**: two confirms, paste does not override the list
  button. Point at it from `TorrentPicker.svelte`. #108.5 stays (API still accepts a magnet
  without `name` / `nyaa_id` / `size_bytes`).
- `docs/TODO.md` — `v2.7.2` bullet; this spec's status → implemented when it ships.
- Specs README — drop this row when implemented (same as every other executed spec).

---

## Out of scope

- Episode picker (`v2.9.0`).
- Showing dropped singles, or a count of filtered-out Nyaa rows.
- Changing `min_seeders`, size ceilings, cap 8, or merging title variants.
- Markdown-table reconstruction of pipe-art in `<p>`.
- Adding `seasonYear` to `GetMediaByID`.
- Changing replace coverage or the per-episode magnet modal on Anime Detail.
- Persisting `pickerCache` or the custom `q`.
