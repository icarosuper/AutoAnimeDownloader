# Standalone torrent picker — confirm the auto-pick, read the Nyaa page

**Date:** 2026-10-23
**Status:** designed, not implemented — `v2.7.0`
**Origin:** adding a standalone always takes the daemon's top torrent; the listing (name, size,
seeders) is not enough to decide; the user needs the Nyaa `/view/<id>` page (description,
screenshots, MediaInfo, file list) and a way to confirm or swap before anything is saved.
**Scope:** `internal/nyaa` (detail page scrape, `DetailURL` on movie rows), `internal/daemon`
(preview of ranked whole-title torrents, add-with-magnet, rollback), `internal/api` (candidates,
detail, optional magnet on standalone POST), `cmd/cli` (parity),
`frontend` (picker route, Add flow, replace flow, i18n).

---

## The problem

`POST /standalone-animes` adds the id and immediately runs `DownloadStandaloneAnime`, which is the
same movie → pack → singles pipeline as the verification pass. The only way to take a different
torrent is to paste a magnet on the anime page, after the wrong one is already queued.

The ranking is still the right default. What is missing is a **confirm** step: show the torrent the
ranking would pick, let the user read the Nyaa page, swap to another of the ranked options, then
save and download.

## What this is not

- Not a Nyaa browser. The app still filters (ignore list, `min_seeders`, size ceilings, packs-only
  or movie) and ranks (Priorities). The user sees at most **8** rows. They do not page through the
  raw 75-row listing.
- Not a per-episode picker. The unit of choice is **one torrent for the whole title** (a pack, or
  a movie). Leftover episodes the pack does not cover stay on the daemon's automatic singles path
  on the next pass.
- Not a replacement of magnet paste. Paste stays on the picker as a fallback.
- Not a change to the verification pass. List-tracked anime keep auto-picking every cycle.

## Flow

### Add (`#/add`)

1. The user clicks Add on a search card.
2. The frontend `GET`s candidates for that media id, with no `q` (daemon title variants).
3. **GET error (Nyaa down):** still navigate to `#/add/:id/torrents` with that error. The picker
   shows it in the list; it does not auto-retry until Enter. Nothing is saved.
4. **Empty list (200, `items: []`):** do not open the picker. `POST /standalone-animes` with no
   magnet — today's behaviour (singles, or track-with-zero for unreleased).
5. **Non-empty:** navigate to `#/add/:id/torrents`. The picker shows the list; index 0 is selected
   and labelled recommended; its Nyaa page loads in the description pane.
6. Confirm: `POST /standalone-animes` with `{media_id, magnet}` of the selected row. Success:
   today's toast (with the downloads link) and back to `#/add`. Cancel / back: nothing is saved.

The first candidates response is kept in memory so the picker does not search Nyaa a second time
on navigation. A reload of the picker URL fetches again. Editing the query always fetches.

### Replace (`#/status/:id`)

"Replace torrent" on the anime page opens `#/status/:id/torrents` — the same screen. Confirm calls
the existing `POST /animes/{id}/replace`. Magnet paste lives at the bottom of this screen, not in
a separate dialog. This entry point is for **any** tracked anime (list or standalone). Success
returns to `#/status/:id`.

### Query field

Prefill with the `query` the server actually used (the title variant that produced the list). The
user may replace it with any Nyaa `q` they prefer (`frieren s01`, `[judas] frieren`, …) and press
Enter.

Enter with a non-empty field: new Nyaa search with that `q`, same filters and ranking, new list,
new recommended = new index 0. One in-flight request at a time; a second Enter aborts the first
(same pattern as AniList search on Add).

Enter on an empty field: no request.

A custom query that returns no packs: the picker **stays open**, list empty, copy "no packs, try
another query". It does not fall through to the automatic-singles Add. That fall-through exists
only for the first, daemon-query search triggered from the Add card.

Custom `q` skips the AniList title-variant loop and the season/part row filter. Those exist to
disambiguate an automatic search; a user who typed the query has taken that over. What still
runs: `ShouldIgnore`, `min_seeders`, size ceiling, packs-only (or movie, when AniList format is
`MOVIE`), Priorities sort, cap 8, `titleMatchesQuery` against the query tokens.

The custom query is not persisted. The next time the picker opens, it is the daemon query again.

### Layout

Full page, not a modal. The Add screen is already a route because a cover grid and reload beat a
dialog on mobile; this screen holds more (screenshots, MediaInfo, file list).

- **Desktop:** list on the left, Nyaa page on the right. Query field under the title block.
  Confirm / back in a footer.
- **Mobile:** one column. List first; tapping a row shows that row's Nyaa page below (same
  scroll). Footer: Back / Download this.
- Breadcrumb back to Add or to the anime page. The system back button pops the hash route.

The description pane is the Nyaa `/view/<id>` payload: sanitised HTML (formatting, links, images;
no script, no iframe), file list, link "open on Nyaa". It loads **on selection**, never for the
other seven rows. A failed description leaves the list usable; the pane shows an error and the
Nyaa link; Confirm still works (the magnet came from the listing).

---

## API

### `GET /api/v1/animes/{id}/torrent-candidates`

`{id}` is the AniList media id. The anime does not have to be tracked.

| Query | |
|---|---|
| `q` | Optional. Absent: daemon title-variant search. Present and non-empty: that string as Nyaa `q`. |

`200` body:

```json
{
  "query": "Frieren: Beyond Journey's End",
  "items": [
    {
      "name": "[SubsPlease] Frieren (01-28) [1080p]",
      "magnet": "magnet:?xt=urn:btih:…",
      "size_bytes": 9019431321,
      "seeders": 312,
      "leechers": 12,
      "fansub": "SubsPlease",
      "resolution": "1080p",
      "is_batch": true,
      "range_start": 1,
      "range_end": 28,
      "nyaa_id": 1664122
    }
  ]
}
```

- `items` is already filtered and ranked. Index 0 is the auto-pick. Cap **8**. Empty `items` is
  **200**, not 404 — the frontend branches on length.
- `query` is what actually ran (winning title variant, or the `q` the client sent). The field
  displays this.
- `range_start` / `range_end` come from the **name** only (`ExtractBatchInfo`). `0` means unknown.
  This endpoint does not fetch `/view/<id>` for every row.
- `nyaa_id` is the integer in `/view/<id>`. The client never sends a free-form URL.
- Movie vs series: AniList `format == MOVIE` uses the movie search and `SortMovieResults`;
  otherwise packs only (`partitionSearchResults` / pack side) and `SortTorrentResults`.
- On the detail entry point, per-anime settings (if any) feed the ranking, same as the pass. On
  Add there are none yet: global Priorities.

Nyaa down / timeout: `502` `NYAA_UNAVAILABLE`. The picker still opens (from Add, after a failed
GET the frontend navigates with an empty store and the picker shows the error in the list). The
anime is not saved.

AniList does not know the id: `404` `ANIME_NOT_FOUND`.

### `GET /api/v1/animes/{id}/torrent-candidates/{nyaaId}`

`nyaaId` is digits only. The server fetches `getNyaaBaseURL() + "/view/" + nyaaId`. It does not
accept a URL from the client (SSRF). `{id}` is unused for the fetch; it stays in the path so the
candidates collection has one prefix.

`200` body:

```json
{
  "description_html": "<p>…sanitised…</p>",
  "files": [{"name": "Frieren - 01.mkv", "size_bytes": 327155712}],
  "nyaa_url": "https://nyaa.si/view/1664122"
}
```

- `description_html` is the inner HTML of `#torrent-description`, sanitised (allow `p`, headings,
  lists, `pre`/`code`, `a[href]`, `img[src]` http(s), emphasis; strip `script`, `iframe`, event
  handlers). Relative image URLs resolved against the Nyaa origin.
- `files` is the flattened video-and-other file list from `.torrent-file-list` (name + size). No
  cap: a 574-file pack is a legitimate thing to inspect. Folders are not separate rows; nested
  names keep a path prefix if the markup has one.
- Nyaa down or unknown id: `502` `NYAA_UNAVAILABLE` or `404` `TORRENT_NOT_FOUND`. The list on the
  client does not clear.

`ScrapNyaaForMovie` today does not set `DetailURL`. This work fills it the same way
`ScrapNyaaForAnime` already does, otherwise movies have no description.

### `POST /api/v1/standalone-animes`

Body becomes `{"media_id": 21, "magnet": "magnet:?xt=…"}` with `magnet` optional.

- No magnet: today's `DownloadStandaloneAnime`.
- Magnet present: the 409s (`LIBRARY_NOT_CONFIGURED`, `ALREADY_*`, …) still run first. Then add
  that magnet as the whole-title torrent and record the standalone id.

**Coverage, not "every aired episode".** `ManualDownloadAnimeWithMagnet` (the existing replace
path) writes a record for every aired episode pointing at the hash. That is wrong here: a S1
pack on a 50-episode title would hide 38 episodes from the next pass. The picker add resolves
the chosen torrent's range the way the daemon does (name via `ExtractBatchInfo`, then one
`PackFileRange` if the name has none) and writes records **only for covered aired episodes**.
Unknown range after both: write **no** episode records (decision #84 — never invent `1..total`).
The torrent still enters the session; the next pass adopts by coverage and searches singles for
the rest. A movie writes its movie record.

**Order and rollback.** Accept the torrent into the session **before** writing the standalone
id. If `backend.Add` (or disk guard) refuses, the id is never written. If the id is written and
a later step fails, `RemoveStandaloneAnime` runs before the error returns. The response stays
`{"added": N}` — `N` is how many episode records were written (0 is valid).

Existing 409 codes and messages do not change.

`POST /api/v1/animes/{id}/replace` (the detail-entry confirm) keeps its current contract,
including marking every aired episode. Changing replace coverage is out of this spec; the
picker only supplies the magnet.

---

## CLI

Same change as the endpoints ([conventions.md](../../agents/conventions.md) checklist).

- `animes candidates <id> [--query STRING]` — prints the ranked list (`query` + items). `--json`
  is the existing agent contract.
- `animes add <id> [--magnet URI]` — without `--magnet`, today's add. With it, the magnet POST.
- `animes replace <id> --magnet URI --yes` already exists; no new flag.

There is no CLI command that prints the sanitised HTML of a view page. The description pane is a
web-app need; `animes candidates` plus a Nyaa URL (`nyaa_id` in the JSON) is enough in a terminal.

---

## Errors (product)

| Case | Behaviour |
|---|---|
| Nyaa down on candidates | Picker opens, list shows the error, query field works for retry. Nothing saved. |
| Nyaa down on description | List stays; pane shows error + Nyaa link; Confirm still enabled. |
| First Add search empty | No picker; today's POST (singles / track with 0). |
| Custom query empty on Enter | No request. |
| Custom query, no packs | Picker stays, empty-state copy, no automatic singles. |
| Confirm 409 | Toast, stay on the picker. |
| Confirm disk / add failure | Error toast, id not left in `standalone_animes` (rollback above). |
| Broken image in description | Stays broken; Nyaa link is the escape. |

---

## Tests

Nyaa is mocked (`MockNyaaHttpGet`). No `AAD_LIVE_NYAA` in this work.

**API / daemon**

- Candidates: ranked packs, index 0 is the same pick `pickBatches` / movie sort would make for
  that list; empty is 200 `{items:[]}`; `?q=` hits Nyaa with that `q` and still applies
  ignore / seeders / size / pack-or-movie / ranking.
- Movie path returns movie rows with `nyaa_id` set.
- Description: sanitised HTML (a `<script>` in the fixture does not survive); file list parsed;
  non-numeric `nyaaId` is 400; Nyaa 404 → `TORRENT_NOT_FOUND`.
- POST with magnet: torrent added, id written, `added` counts only episodes inside the pack
  range (a 1-12 pack on a 24-ep title writes 12, not 24). Unknown range: `added` is 0, torrent
  is in the session. Disk refusal: id absent afterwards.
- POST without magnet: existing standalone tests still pass.

**CLI**

- `animes candidates` / `animes add --magnet` hit the new client methods (same style as
  `client_test.go`).

**Frontend (Vitest, `AddAnime.test.ts` style)**

- Empty first search: no navigation to the picker; `addStandaloneAnime` called without magnet.
- Non-empty: navigates to `#/add/:id/torrents`; first row selected; description GET fires for
  that `nyaa_id`.
- Enter on the query field calls candidates with `q=`; recommended is the new index 0.
- Confirm POSTs the selected magnet.
- Description failure: list intact, error in the pane.

---

## Files this will touch (planning)

- `nyaa/nyaa.go` — `DetailURL` on movie rows; scrape `#torrent-description` + file list (next to
  `PackFileRange`, same `fetchNyaaPage`).
- `daemon` — a preview function that runs the search + partition + sort and returns the cap-8
  list **without** adding torrents. `DownloadStandaloneAnime` with a magnet, rollback.
- `api/endpoint_standalone_animes.go`, new candidates handler, `server.go` routes, `client.go`.
- `cmd/cli/animes.go`.
- Frontend: `TorrentPicker.svelte` (or equivalent), routes in `App.svelte`, Add navigation,
  AnimeDetail replace, `client.ts`, `en.json` / `pt-BR.json`.
- Docs in the same change: `architecture.md` (endpoints + the "picker does not persist" fact),
  `guides/cli.md`, `guides/webui.md`. A `decisions.md` entry for: empty first search skips the
  picker; custom `q` drops season/part and title variants; description is on-demand; rollback
  order on add-with-magnet.

Do not fetch `/view/<id>` inside the candidates handler. The measured cost of a detail page is
~1/4 of a search; eight of them up front is the thing [sources.md](../../agents/sources.md)
called out as expensive. The daemon's `maxPackDetailFetches = 3` stays a pass concern, not this
preview.

---

## Out of scope — next feature

Reuse this screen for **per-episode download / redownload**. Today those buttons auto-pick a
single via `ManualDownloadEpisode`. The follow-up is the same picker with the episode search
(`searchSingleEpisode`) instead of packs: ranked singles, on-demand Nyaa page, confirm the
magnet. Query field, sanitised description, and the candidates/detail endpoints are the parts
that carry over; the Add-card empty-list fall-through and the whole-title POST do not.

Do not build any of that in this change. It is its own spec that starts from this one, and it
ships in the same `v2.7.0` after this screen exists. Tracked in [TODO.md](../../TODO.md).
