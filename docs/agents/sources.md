# Torrent sources — survey

Read this before adding a new source to the daemon. It records what was **measured** in
August 2026, not the folklore about each site. Today the daemon has a single source: Nyaa, via
`internal/nyaa` (HTML scraping with goquery).

**Caveat on the original verdicts (1st round, Aug 2026):** they were formed by measuring
**modern** anime, a population where every source is equivalent by construction — same uploaders,
same releases, all alive. That is why they missed the difference that matters for old and niche
anime. The 16 Aug measurements (marked below) correct what fell out.

The seam for adding a source already exists: the `nyaaSearcher` interface, injected into
`daemon.processAnimeEpisodes` (see [decisions.md #14](decisions.md#14-nyaasearcher--dependency-injection-for-nyaa-search-in-processanimeepisodes)). A new source is a new
implementation, not a refactor.

---

## Nyaa (current source)

`https://nyaa.si/?f=0&c=1_2&q=<title>&s=seeders&o=desc`, paginated with `&p=N`.

**Limits that matter for the algorithm:**

- **There is no size filter on the query.** No `minsize`/`maxsize`. Size is filtered by the
  daemon, after the page is downloaded. (The `minage`/`maxage`/`minsize` that shows up in scraper
  issues belongs to NyaaPantsu, a different fork, not `nyaa.si`.)
- **`s=seeders&o=desc` is terrible for a long-running series pack.** The sort buries a 40–100
  seeder pack under hundreds of weekly episodes with 200–1000 seeders. Measured on One Piece: the
  most useful partial pack (61 GiB) only appears on **page 3**. That was the cause of the "One Piece
  never downloads" bug — see [decisions.md #80](decisions.md#80-the-pack-size-ceiling-is-applied-twice-in-the-nyaa-search-and-in-the-daemon-filter).
- **It has RSS** (`?page=rss`), with its own seeders/leechers/infohash/size tags — cheaper to parse
  than HTML and immune to layout changes. **But it does not replace HTML** (measured 16 Aug):

  | | `s=seeders&o=desc` | `&p=N` | items/page |
  |---|---|---|---|
  | HTML | honored — `332 147 130 107 72 …` | distinct pages | 75 |
  | RSS | **ignored** — comes out `5 3 9 0 6 6 46 …` (timestamp desc) | **ignored** — md5 of `p=2` guids identical to `p=1` | 75 |

  In other words: RSS delivers the 75 most recent uploads and stops. You cannot walk 5 pages or
  sort by seeders. Switching HTML→RSS drops goquery and drops the search with it.
- `f=1` filters "trusted" only, `f=2` filters "no remakes". Today the daemon uses `f=0`.

**Why HTML scraping stays:** `s=seeders&o=desc` combined with `&p=N` exists only on the HTML
interface. None of the measured alternatives (Nyaa RSS, AnimeTosho JSON) sort by seeders. goquery
is not accidental debt — it is the price of the only interface that sorts.

### Detail page (`/view/<id>`) — what it adds

Measured on 28 Aug 2026 at `nyaa.si/view/1323474` (`[Judas] One Piece 001-574`, uploaded Jan 2021,
171.8 GiB, 574 files) — worst case on purpose: a giant, old pack.

| Field only on the detail page | Useful? |
|---|---|
| **File list** (`div.torrent-file-list`, `<li>` with name + `span.file-size`) | **Yes.** All 574 files came back complete, with name and size, in a folder tree. |
| `Completed` (snatches; 8723 in this case) | Weak health signal — it decides nothing that `Seeders` does not already decide. |
| Submitter, info hash, category | Already derived from the name/magnet by the daemon. |
| Uploader description (source, audio, subs, notes) | Free text. Noisy heuristic on fansub prose — not worth it against `ShouldIgnore`, which decides by name. |

The trusted/remake flag **does not** need the detail page: it already comes on the listing `<tr>`
class (`success`/`danger`).

**Measured cost:** 175 KB on that page — but it is the worst case (574 files). Re-measured on
29 Aug 2026 over 7 real packs (Vinland Saga, Mushoku Tensei, AoT, One Piece): **30 KB average**,
against 125 KB for a full listing. A detail page costs ~1/4 of a search, not more than one. What
stays expensive is fetching detail for **every** row: 75 × 30 KB ≈ 18× one search.

**The difference that matters against AnimeTosho:** Nyaa's file list is generated from the
`.torrent` itself and **works on old content**. AnimeTosho's `files[]` does not (it comes back `[]`
for an old entry — see the caveat in its section). For the long-running series problem, the right
source of the file list is Nyaa itself, not AnimeTosho.

### Which search case is worth a detail fetch

- **Individual episode — no.** One file. No new input for the decision.
- **Movie — almost nothing.** It would only distinguish a "movie" result that is actually a BD
  folder with extras or a collection. Marginal.
- **Pack — yes, and it is the only one that pays.** Two things, of very different weights:

**1. Real pack coverage — this is a bug, not an improvement.** `ExtractBatchInfo`
(`internal/nyaa/nyaa.go`) reads the range from the **name** only. An unknown range becomes
`EndEpisode == 0`, and then the pack counted as complete: `pickBatches` stopped the cursor there
and `assignBatches` wrote `batchStart/batchEnd = 1..total`. Every episode in the window became an
`EpisodeStruct` pointing at the pack hash. If a `"(Season 1+OVA) [Batch]"` covers only 1-12 and the
window was 1-50, the remaining 38 were **recorded as downloaded**: the Librarian links only the
files that exist and nothing reconciles the records with disk afterwards, so those episodes never
return to the search.

> **Implemented** (29 Aug 2026, [decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)): `nyaa.PackFileRange` reads the range from the file
> list before adding the torrent, and `daemon.packSet` resolves it on demand — only for a pack
> with no range in the name, only after filter and sort, at most 3 per anime per pass. A range
> that neither the name nor the files resolved now records **zero** (unknown), not `1..total`.
>
> How common it was: in the first 8 rows of `?q=<anime>+batch` for three animes, **19 of 24 packs
> do not declare a range in the name**. The case that forced it is `view/1891610` — `(Season 2 | Part 2) (Batch)`,
> no range in the name, files **13..24**: under the Part 1 entry the daemon recorded 1..12,
> intersection zero with the contents.
>
> Read rate measured on 7 packs: 574/574, 24/24, 23/23, 12/12, 103/111 files yielded a number
> (the 8 that missed are OP/ED, correctly outside the range). Numbered OP/ED and `S00E`/OVA
> later left min-max as well ([decisions.md #112](decisions.md#112-packfilerange-min-max-skips-extrasova-an-isolated-total1-drops-next-cour-stays)). Two cases needed the fixes that
> shipped with it: a name separated by `_` (`[DB]Vinland Saga_-_01_(...)`) matched no pattern, and
> `(2021-2022)` was read as the episode range 2021..2022 ([decisions.md #83](decisions.md#83-the-pack-names-range-is-the-first-plausible-match-not-the-first-match)).

**2. A per-episode ceiling instead of a total-size ceiling** — a policy change, not a bugfix. The
pack above fails any sane `max_batch_torrent_size_gb`, but it is ~300 MiB/episode, which is great.
**It does not save download:** `rain` does not select files (see AnimeTosho's "What it does NOT
solve"), so the whole pack still comes down — only the accept criterion changes. It is only worth
it if the current ceiling is rejecting a pack the user wants.

> **Measured on 30 Aug 2026, and the precondition did NOT hold — do not do this.** 58 real packs
> with size and file count (`nyaa/live_pack_measure_test.go`): median **461 MiB/episode**,
> min 73 MiB, max 4.2 GiB. At the default 100 GB ceiling:
>
> - it rejects **4 of 58**, and those 4 are whole-series One Piece (587 GiB / 1498 files, 172 GiB /
>   574 files). Rejecting those is the documented behavior in `config.md`, not an accident: the
>   partial One Piece packs (86, 61, 36 GiB) pass and cover the window.
> - the 3 packs that are actually fat per episode **pass** — ZeroBuild AoT S3 (91.4 GiB, 22 files,
>   **4.2 GiB/ep**), Mushoku Tensei II (87.2 GiB, 2.25 GiB/ep), Mushoku Tensei S1 (82.7 GiB,
>   1.6 GiB/ep). All under 100 GB because they are short.
>
> The size ceiling is anti-correlated with per-episode economy — but that was never its job. It
> guards **disk**, and because `rain` downloads the whole pack, disk is the right axis for
> "does this fit here". Switching to a per-episode ceiling would mean accepting 172–587 GiB of
> download; only reopen if the user says they want a whole-series pack.

**How to keep it cheap, if it is implemented:** pack rows only, and only when the data changes the
decision — name with no range (`EndEpisode == 0`) or pack above the ceiling. That is ~1-3 requests
per anime. And **outside `parseRow`**, after sort/filter: inside the page loop it becomes one
request per row.

**What remains unanswered:** dead swarm, file selection inside the pack (`rain` limitation), and
sorting.

---

## Pack granularity and numbering — what the source delivers vs. what AniList asks for

Measured on 28 Aug 2026, from `Raw Nyaa row` lines of real passes. This is not a parsing detail:
it is why the daemon and the source disagree about what "one episode" is.

**1. A pack is almost always a whole season, not a cour.** AniList splits cour/part into
**separate media ids**, each with its own numbering from 1 (see [decisions.md #45](decisions.md#45-one-library-folder-per-anilist-entry--the-season-marker-stays-in-the-name) and
[#71](decisions.md#71-anilist-has-no-franchise-id-the-prequel-chain-is-the-only-source-and-searching-by-name-is-a-trap)). Groups do not follow that cut: the released pack covers the whole season. Practical
consequence, measured on Mushoku Tensei II Part 2 (`166873`): the only available packs were
whole-season, and the search built from the "Part 2" entry discarded them for not carrying an
explicit part marker. A pack "with no part" is not a wrong pack — it is the normal release
format.

**2. There is no numbering convention; there are four.** All observed in the same period:

| Release | Numbering in the name/files | Convention |
|---|---|---|
| `[Erai-raws] … Honki Dasu - 00 ~ 12 [BATCH]` (MT S2 Part 1) | `00 ~ 12` | restarts per **season** |
| `[EMBER] … (Season 2 \| Part 2) … (Batch)` (MT S2 Part 2) | `1..12` | restarts per **part** |
| `[Diddy] Mushoku Tensei - S02 (BD 1080p HEVC Opus)` | `1..25` | continuous across the **whole season** |
| SubsPlease / Erai-raws, AoT Final Season Part 2 | `76..87` | **absolute across the series** |

The fourth row is the one that usually surprises: `76..87` matches exactly the accumulated sum of
episodes in the AniList `PREQUEL` chain (see [decisions.md #71](decisions.md#71-anilist-has-no-franchise-id-the-prequel-chain-is-the-only-source-and-searching-by-name-is-a-trap)). So some groups use the
franchise's absolute axis as file numbering — and none of them announce which convention they
adopted.

**3. That is why mapping pack numbering is always a guess, and the tie-break is the file
count.** You cannot read the convention from the name. You can reduce it to a few hypotheses
(offset 0, immediate-prequel offset, whole-series offset) and use the pack's **file count** to
choose: a pack with 25 files numbered from 1 can only be a continuous season, not a 12-episode
cour. That is the same evidence `packEpisodeOffset` already uses in organize
([decisions.md #68](decisions.md#68-pack-numbering-the-offset-is-guessed-from-the-files-themselves-and-a-basename-collision-uses-the-relative-path)), and it is what makes Nyaa's detail-page file list (section "Detail
page", above) valuable **before** downloading, not only after.

**4. A "Part 1 + Part 2" in the name is not "part 1".** `[Fuchs] … - S02 … (Season 2) (Part 1 + Part 2)`
covers both halves, but an extractor that returns the first number it matches reads `1`. Reading a
part marker is always fragile; asking "does this pack cover the pending window?" is not.

> **Implemented** (29 Aug 2026, [decisions.md #79](decisions.md#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses)): on the pack path the part marker only
> rejects a declared conflict, `ExtractPart` returns `nil` for a name with two parts, and the three
> numbering hypotheses in this section became `daemon.packAxis` — with the name's span in place of
> the file count as the tie-break. The detail scrape came later ([decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)) and
> resolves the **range** of a pack with no range in the name; the per-folder file count, which
> would tie-break a pack whose numbering restarts per season, **stays out on purpose** — measured
> on 30 Aug 2026 ([decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)): 6% of packs, delta 0 picks correctly on all of them, and a
> per-folder count would read extras as episodes.

---

## AnimeTosho — good complement, bad replacement

**Watch the domain.** `animetosho.org` froze in May 2026 and no longer indexes anything. The
continuation is `animetosho.xyz`, and the API/feeds moved to `feed.animetosho.xyz` (the old
domain's endpoints went away on 1 Aug 2026). Both were verified responding in August 2026.

It **indexes Nyaa itself** (fields `source: "nyaa"`, `nyaa_id`, `article_url`). It is not a
competing archive — it is a better front-end for the archive the daemon already uses.

**How much "better front-end" means, measured on 16 Aug** (`?q=cowboy+bebop`, 75 results):
non-null `nyaa_id` in **75/75**, non-null `anidex_id` in **0/75**, `article_title == "Nyaa"` in
100%. Same infohash ⇒ same swarm. For the "old anime finds a dead candidate" problem, the gain is
**zero** — switching indexer does not resurrect a swarm.

### API

`GET https://feed.animetosho.xyz/json?q=<query>` → JSON array. Fields per item:

```
title, torrent_name, magnet_uri, torrent_url, info_hash,
seeders, leechers, total_size, num_files, timestamp,
nyaa_id, article_url, anidb_aid, anidb_eid, nzb_url
```

Maps almost 1:1 onto `nyaa.TorrentResult` — goquery goes away and so does the fragility of
counting `<td>`.

**Two limits measured on 16 Aug that stop it from replacing Nyaa:**

- **It does not sort by seeders.** `order=seeders-d` is accepted and **silently ignored** (falls
  back to the default, timestamp desc). Only `order=size-d` works. With 75 items/page, walking 5
  pages would become "the 375 most recent uploads that mention the name" — the worst possible case
  for old anime, and it would reintroduce the [decisions.md #80](decisions.md#80-the-pack-size-ceiling-is-applied-twice-in-the-nyaa-search-and-in-the-daemon-filter) bug in a worse form.
  (`page=N` **works**, unlike Nyaa RSS.)
- **The `seeders` field is a stale cache.** On `?q=cowboy+bebop`, `tracker_updated` age had a
  median of **22 days** and a max of **95 days**. Top-of-list comparison at the same instant:

  ```
  nyaa  (live):   333 147 130 105 72
  tosho (cache):  191  72  44  42 37
  ```

  `min_seeders`/`filterBySeeders` would start deciding on a stale number, and the error is
  systematically **downward** — a false negative exactly in the direction of the symptom that
  motivated the multi-source investigation.

### Pack file list BEFORE downloading

`GET https://feed.animetosho.xyz/json?show=torrent&id=<id>` adds `is_batch`, `files[]` and
`primary_file_id`. Each `files[]` item has `filename`, `size` and `crc32`.

That answers "which episodes this pack covers and how much each one weighs" **without** adding the
torrent and waiting for metadata. It is the data that is missing today to measure a ceiling per
covered episode instead of per total pack size.

**Measured caveat:** old entries come back with `files: []`. The pack `[Judas] One Piece 575-782`
(id 369911) returns an empty list; a recent SubsPlease batch returns the 26 `.mkv` files with name
and size. The file list is only reliable for recent content — treat `files: []` as "unknown", not
as "empty pack".

### What it does NOT solve

It does not let you download only some files from a pack. That is a **client** limitation, not a
source one: `rain` v2.3.1 has no file selection (`Torrent.Files()`/`FileStats()` are read-only, and
there is no per-file priority/skip in the module). Per-episode selection inside a pack would need
a `rain` fork or a client swap.

It also does not solve a dead swarm (same infohash) or replace seeder sorting — see above.

### When it is worth it

As an **additional** source, when a requirement shows up that Nyaa HTML does not meet: matching by
`anidb_aid` instead of a title string. **`files[]` left that list** (measured 28 Aug): Nyaa's own
detail page delivers the file list and, unlike here, works on old content — see "Detail page" in
the Nyaa section. As a **replacement** for Nyaa today, it is not worth it: parsing gain, loss of
sorting and of seeder freshness.

---

## Other sources evaluated

| Source | Verdict |
|---|---|
| **AniDex** | **Unviable** (measured 16 Aug): `GET anidex.info/rss/?q=…` → **HTTP 403**, DDoS-Guard js-challenge (`/.well-known/ddos-guard/js-challenge/`). Would require a headless browser on every daemon request. Out of scope. |
| **TokyoTosho** | **The earlier verdict ("same content as Nyaa") fell.** Measured 16 Aug: `rss.php?terms=…&type=1` → 147 items, and it **aggregates other indexers** — trackers `t.acg.rip`, `tr.bangumi.moe`, `open.acgtracker.com`, `tracker.kamigami.org`, hosts `acg.rip` and `files.catbox.moe`. Different infohash ⇒ potentially different swarm: it is the only candidate that could resurrect dead content. **The catch:** the RSS does not expose seeders — the `<item>` tags are only `title`, `category`, `link`, `description`, `guid`, `pubDate` (the magnet is in the `description` HTML, and the infohash is in **base32**, not hex). With no seeders, `min_seeders` has nothing to filter; measuring "is it alive?" requires going to the swarm. |
| **Direct fansub RSS** (SubsPlease, Erai-raws) | Very clean signal and no noise for the current season; **useless for backlog** (they do not host an old catalog). Serves as a "preferred source" for airing anime, not as a general source. |
| **Usenet / NZB** | AnimeTosho carries `nzb_url`. HTTP speed, no seeder problem, long retention. Requires a paid provider + indexer + a whole download path in parallel with `rain`. Large change. |
| **Debrid** (Real-Debrid, AllDebrid) | You send the magnet, the service downloads on its side, and you pick **which files** to pull over HTTP. It is the only way to get per-episode selection inside a pack without touching `rain`. Costs a monthly fee and becomes an external dependency. |

---

## Recommended order

1. **Fix pack search on Nyaa** — done, see [decisions.md #80](decisions.md#80-the-pack-size-ceiling-is-applied-twice-in-the-nyaa-search-and-in-the-daemon-filter). It was a bug, not a missing
   source.
2. **Do not replace Nyaa with anything right now.** Not RSS (loses sorting and pagination) nor
   AnimeTosho (loses sorting, seeders cached ~22 days). The gain would be parsing; the cost is the
   search.
3. **Pack file list** — done, see [decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything). It came from **Nyaa's** detail page
   (`/view/<id>`), not AnimeTosho: only that one works on an old pack. It solved real coverage; the
   second use — tie-breaking the numbering convention by per-folder file count (section "Pack
   granularity and numbering") — stays out. AnimeTosho as an additional source is left only for
   matching by `anidb_aid`. Neither as a replacement.
4. **Debrid/Usenet** — only if file selection becomes a real requirement. That is a *protocol*
   swap, not an indexer swap.
