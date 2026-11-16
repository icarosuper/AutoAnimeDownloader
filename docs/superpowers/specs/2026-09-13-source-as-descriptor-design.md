# The torrent source becomes an external descriptor — taking Nyaa out of the repository

**Date:** 2026-09-13
**Status:** designed, not executed
**Origin:** "take the hard dependency on nyaa out of the code so it's safer to publish in
places. I think in this case it would be a plugin system like the manga reader app mihon"
**Expected scope:** `src/internal/nyaa` (split into two packages), `src/internal/daemon/search.go`
(`nyaaSearcher` starts carrying the source), `src/internal/files` (load the descriptor from the
config folder), `src/tests/mocks` and `src/tests/unit/nyaa_test.go` (fixtures stop citing `nyaa.si`).

---

## The goal is publishability, not multiple sources

Switching indexers still doesn't bring a swarm back to life. This spec doesn't measure that and doesn't build
a second source.

What this spec wants is one thing only: **the repository and the binary no longer containing Nyaa's address,
query and layout**. That this unlocks a second source is a consequence, not the reason — and that is
why the design below refuses to build aggregation, result merging or a catalog.

**What it does not buy:** legal shielding. The app is still "downloads anime by torrent
automatically". What goes away is the most direct artifact — the code that scrapes a named site. Useful
precedent: the Tachiyomi repo survived, it was `tachiyomi-extensions` that got the takedown and had to leave
the org (it became keiyoushi), and even so the app got a C&D later. Worth designing knowing that.

## A compiled plugin is out

The stdlib `plugin` package is Linux-only, requires the app and the plugin to be compiled with the **same**
compiler and the **same** versions of every shared dependency, and does not work with
`CGO_ENABLED=0`. The project has a tray on Windows (`internal/tray`). A Go plugin ABI would be a
build matrix per platform and per release, to host ~150 lines of CSS selectors.

An embedded script runtime (goja, starlark, yaegi, wasm) would solve it, at the cost of a new
dependency and of a sandbox for third-party code. Disproportionate: the three sources already measured in
[`sources.md`](../../agents/sources.md) are all *"GET a URL → rows"*. That is data, not code.

**Decision: a declarative descriptor in JSON, with no code execution.**

## The boundary is already measured

The `internal/nyaa` package has 2217 lines. What is Nyaa-specific:

| what | where |
|---|---|
| base URL and query parameters (`f`, `c`, `s`, `o`, `p`) | `nyaa.go:56`, and repeated in the three `Scrap*` |
| cell positions — `cells.Eq(1/2/3/5/6)` | inside the three `parseRow` |
| row selector `.torrent-list tbody tr` | `parsePagesWith` (`nyaa.go:423`) |
| date format `2006-01-02 15:04` | `parseNyaaDate` (`nyaa.go:1048`) |
| detail page file list — `div.torrent-file-list`, `span.file-size` | `PackFileRange` (`nyaa.go:224`), `fileListEntryName` |
| building `/view/<id>` | `detailURL` (`nyaa.go:277`) |

That is ~150 lines. **Everything else — ~2050 — is not Nyaa:** all of `nyaa_match.go` (title jaccard),
all of `nyaa_regex.go`, all of `priorities.go` (`ShouldIgnore`, sort by criterion), and the
`Extract*` / `parseSize` / `ParseSeeders` / `SortTorrentResults` in `nyaa.go`. That is parsing of the
**release name**, which follows the same scene convention on any source.

Empirical proof that the boundary falls exactly on `TorrentResult`: `sources.md` records that
AnimeTosho's JSON "maps almost 1:1 onto `nyaa.TorrentResult`".

> **That is why the plugin does NOT include the processing.** Mihon is a bad analogy here: manga sites have no
> naming convention, so the extension has to parse. Anime releases do —
> `[Group] Name - 01 (1080p) [HEVC]` is the same thing on every source. Sending the processing to the
> descriptor would duplicate 2050 lines per source and throw away the tests that already exist over them.

## The descriptor

```jsonc
{
  "name": "example",
  "search": {
    "url": "https://example.tld/?q={query}&s=seeders&o=desc&p={page}",
    "kind": "html",
    "row": ".torrent-list tbody tr",
    "fields": {
      "name":     "td:eq(1) a:not(.comments)",
      "magnet":   "td:eq(2) a:eq(1)@href",
      "detail":   "td:eq(1) a:not(.comments)@href",
      "size":     "td:eq(3)",
      "seeders":  "td:eq(5)",
      "leechers": "td:eq(6)",
      "date":     "td:eq(4)"
    },
    "dateFormat": "2006-01-02 15:04"
  },
  "detail": { "kind": "html", "fileList": "div.torrent-file-list li", "fileSize": "span.file-size" }
}
```

`{query}` and `{page}` are the only two placeholders. **Building the query** (`extractSeasonFromName`,
`episodeQueries`, the title variants from `GenerateSearchTitleVariants`) stays in the app: it is search
text, not source-specific. The page cap is still `max_search_pages` from
`config.json`, not a descriptor field.

`kind` accepts `html` (goquery, already installed), `json` (`encoding/json`) and `rss` (`encoding/xml`).
No new dependency. For `json`/`rss`, `fields` carries a field path instead of a CSS selector.

`detail` is optional. A source without it turns off `packRange` — a pack without a range in its name records a zero range,
which is exactly the fallback that [decisions.md #84](../../agents/decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)
already defined.

### The contract the source has to meet — and who fails it

Declaring this in the format is what keeps the descriptor from turning into a programming language:

1. **Sort by seeders descending.** The page descent of
   [decisions.md #57](../../agents/decisions.md#57-adaptive-pagination-sequential-with-a-candidate-floor-no-per-anime-budget)
   stops at the candidate floor because it trusts that the top is the best.
2. **Actually paginate.** `{page}` has to return a different page.
3. **Expose seeders per row.** `filterBySeeders` and `min_seeders` have nothing to filter without it.

Measured in `sources.md`, this already fails: Nyaa's RSS (ignores `s=` and `p=`), AnimeTosho's JSON
(`order=seeders-d` silently ignored, `seeders` cached with a ~22-day median) and
TokyoTosho (RSS without a seeders tag). **No escape hatch for them until a real, conforming source shows up
asking for one** — including TokyoTosho's base32 magnet, which is not normalized here.

## The real refactor: `parseRow` splits in three

The work is not the descriptor, it is that today DOM extraction and classification live in the same
`parseRow` (`nyaa.go:714` is the full case). In one function it: reads cells by position,
calls `ShouldIgnore`, matches the title, extracts season/part/episode, applies the season/part gates,
applies `batchTooBig`, and accumulates. Only the first of those things is Nyaa's.

Proposed split:

```
extract(row)   → RawRow{Name, Magnet, DetailURL, Seeders, Leechers, Size, Date}   ← descriptor
classify(RawRow) → TorrentResult                                                  ← generic
accept(TorrentResult, criteria) → bool                                            ← generic
```

The page loop (`fetchSearchPages`) stays on the generic side, because the `accepted()` it counts
depends on `accept` — only fetching a page and the `extract` of a row cross over to the source.

**No interface.** There will be only one implementation (the descriptor-driven one); a `Source` struct
is enough. The tests don't need a fake: pointing the descriptor at an `httptest.Server` is cheaper
than today's `MockNyaaHttpGet`, and `getNyaaBaseURL`/`NYAA_URL` stop existing.

### Packages after the split

| package | contents |
|---|---|
| `internal/release` | `nyaa_match.go`, `nyaa_regex.go`, `priorities.go`, all the `Extract*`, `parseSize`, `ParseSeeders`, `SortTorrentResults`, `TorrentResult` |
| `internal/source` | descriptor, fetch, `extract`, pagination, detail page file list |

Measured mechanical cost: 21 files import `internal/nyaa`, with 277 occurrences of `nyaa.`.
It is the bulk of the diff, and it is search-and-replace. Good side effect: `ScrapNyaaForAnime`,
`searchNyaaForMovie` and company stop lying in their names.

The `nyaaSearcher` from
[decisions.md #14](../../agents/decisions.md#14-nyaasearcher--dependency-injection-for-nyaa-search-in-processanimeepisodes)
survives almost intact — the four fields are already the right four, including `packRange`. Its
name changes and `defaultNyaaSearcher()` starts receiving the loaded source instead of closing over package
functions.

## Where the descriptor lives

- **In the app repo: none.** Not even as an example with a real host. `docs/` describes the format; an
  `example.json` pointing at `localhost` serves as a reference.
- **On disk:** next to `config.json` (`files.configFileName`, `filemanager.go:17`), in
  `sources/*.json`. With no source configured, the daemon **starts normally** and search becomes a no-op with
  a clear message in the UI — the anime screen, the history and in-progress torrents keep
  working. It is not a fatal error.
- **Installation:** the user drops the file there. No official index, no embedded catalog, no
  "install from the community repository" button. The catalog is exactly the piece that makes you an owner — that is what
  happened to `tachiyomi-extensions`.
- **Community:** the documented format is enough for someone to maintain their own index. You don't host
  and don't link.

## Tests

`src/tests/mocks/` already has a Nyaa mock and `docker-compose.test.yml` already injects `NYAA_URL`
(`testing.md:229`). It becomes: a test descriptor pointing at the mock. The four `nyaa.si` literals
in `src/tests/unit/nyaa_test.go:1179-1238` become a fixture host.

The live test `nyaa/live_pack_measure_test.go` (gate `AAD_LIVE_NYAA=1`) is the only place that
**needs** a real host. Either it leaves the repo, or it starts reading the host from env with no default.

## Execution order

1. **Split `internal/nyaa` into `release` + `source`, with Nyaa still hardcoded.** Big diff,
   identical behavior, green `go test ./...` is the criterion. No descriptor yet.
2. **Separate `extract`/`classify`/`accept`** inside `source`, still hardcoded.
3. **JSON descriptor driving `extract`**, with `nyaa.json` generated from what was
   hardcoded — and used only as a local test fixture.
4. **Remove the hardcoding**, load from `sources/*.json`, treat absence as a no-op with a warning.
5. **Clean up repo and docs**: fixtures, `NYAA_URL`, `environment.md:17`, `sources.md`, `testing.md`.

Each step is committable on its own. If it stops at 2, the project has already improved; if it stops at 4, the goal
is met.

## Out of scope

- **Multiple sources at once**, merging and deduplication across sources. One active source at a time.
- **`kind: json` and `kind: rss`** can wait until after step 4 — Nyaa is `html`, and no
  source conforming to the contract above is identified today. Designed here so the format doesn't
  need to change when they arrive.
- **Base32 magnet normalization, missing seeders, client-side sorting.** All of that is an escape
  hatch for a source that doesn't meet the contract. Don't build it preemptively.
- **Source management UI.** A file on disk is enough; a screen only after more than one exists.

## Docs to update when executing

`environment.md` (`NYAA_URL` goes away), `config.md` (`sources/` folder), `architecture.md` (the two
new packages, and the negative fact "there is no `internal/nyaa`"), `testing.md` (descriptor-based mock),
`sources.md` (starts describing the contract, not the source), and a new entry in `decisions.md` for
"why the processing doesn't go in the descriptor" and "why there is no `Source` interface".
