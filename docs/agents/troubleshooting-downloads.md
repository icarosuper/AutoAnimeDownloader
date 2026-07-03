# Troubleshooting Failed Downloads

Use this guide when investigating why animes are failing to download, to identify bugs or gaps in the search/filter logic.

---

## Step 0 — Fast path: `make debug-anime`

Before doing anything manual, try:

```bash
make debug-anime ID=<anilistId>
```

`<anilistId>` is the AniList MediaList ID (visible in the web UI URL for that anime, `#/anime/<id>`, or via `GET /api/v1/animes`). This runs a one-shot pass — no daemon or qBittorrent required — that:

1. Fetches the anime from AniList and logs the full response (title, progress, status, synonyms, relations, airing schedule).
2. Picks the episodes that would be searched (same `checkEpisode` logic the real loop uses).
3. Runs the real search/match pipeline (`resolveSearchStrategy` — same movie→batch→multi→single priority as production) against live Nyaa.
4. Logs **every raw torrent row** Nyaa returned for the query (`"Raw Nyaa row"`), then the **matched subset** (`"Found ... results"` with `matched_names`), then a per-episode summary (`magnets_found`).

Output goes to `.debug_<anilistId>_<N>/` in the current directory — nothing is written to `~/.autoAnimeDownloader`:
- `debug.jsonl` has the full trace from steps 1-4 above (one JSON object per line — grep/`jq` it, or just read it directly)
- `summary.json` has the quick per-episode view (`would_search`, `magnets_found`)

Comparing `debug.jsonl`'s raw rows against its matched set tells you immediately whether the problem is upstream (Nyaa has nothing for this title/episode) or in filtering (Nyaa has it, but it got rejected — see Step 6 for why).

**Known limitations** (logged as a warning at the start of every run): episodes are always treated as not-yet-downloaded (no `episodes.json` / qBittorrent check), and excluded-list membership is never evaluated (the single-anime AniList query doesn't fetch custom-list data). If the anime is being skipped for one of those two reasons, this tool won't show it — fall back to Steps 1-8 below.

---

## Step 1 — Read the logs

```bash
build/autoanimedownloader logs
# or
grep -E '"level":"warn"|"level":"error"' ~/.autoAnimeDownloader/daemon.log
```

For archived logs:

```bash
zcat ~/.autoAnimeDownloader/daemon-*.log.gz | grep -E '"level":"warn"|"level":"error"'
```

---

## Step 2 — Identify the failure type

Look for:

```json
{"level":"warn","episode":"Anime Name - Episode N","attempts":N,"message":"Failed to download episode after all attempts"}
```

**`attempts: 0`** → search returned zero magnets. The download loop never ran. Root cause is upstream in the Nyaa search or result filtering.

**`attempts > 0`** → magnets were found but qBittorrent rejected all of them (wrong URL format, qBittorrent unreachable, etc.). Check qBittorrent connectivity separately.

---

## Step 3 — Check if Nyaa returned HTTP errors

Enable debug-level logs and re-run, or check if there are already debug entries:

```bash
grep -i "Error searching Nyaa\|status 50" ~/.autoAnimeDownloader/daemon.log
```

A `502`/`503` means Nyaa was temporarily down — not a logic bug. The daemon retries on the next cycle automatically.

If the same anime fails across **multiple daemon runs on different days**, it is a logic problem, not a transient Nyaa outage.

---

## Step 4 — Find the Nyaa search URLs that were used

Enable debug logging, then look for entries from `nyaa.go`:

```bash
grep '"Searching Nyaa' ~/.autoAnimeDownloader/daemon.log | grep '"anime_name":"Failing Anime"'
```

This gives you the exact URL sent to Nyaa. Fetch it and extract the torrent names:

```bash
curl -sL "<url>" | grep -E 'title="[^"]{20,}' | grep 'href="/view/'
```

If the URL returns zero results, the search query is wrong. Proceed to Step 5.

If the URL returns results but the anime still fails, the results are being filtered out. Proceed to Step 6.

---

## Step 5 — Diagnose the search query

The query is built from the anime title variants. Entry point: `search.go:buildTitleVariants` → `nyaa.GenerateSearchTitleVariants`.

Check what variants are generated for the failing anime:

1. Look at the Anilist romaji and english title (visible in the debug log field `title`).
2. Trace `GenerateSearchTitleVariants` (`nyaa/nyaa.go`) — it tries: clean romaji → original romaji → clean english → original english.
3. Check `ExtractAnimeSeasonPart` (`daemon/helpers.go`) — season/part affect how the query is constructed and filtered.

Search [nyaa.si](https://nyaa.si) manually with each variant to find which one (if any) returns results. If none work:

- The anime may not be available on Nyaa under any of those names.
- The fansub community may use a different title — compare against what's actually posted on Nyaa.
- Consider improving `GenerateSearchTitleVariants` to generate better variants (e.g., stripping subtitles after `:`, trying shorter prefixes).

---

## Step 6 — Diagnose result filtering

If the Nyaa URL returns results but none reach `attemptDownloadWithRetries`, results are being dropped somewhere in the scraping/filtering pipeline. Trace through `nyaa/nyaa.go`:

**6a. Ignore patterns** (`reIgnorePatterns` in `nyaa_regex.go`):
Dub, raw, hardcoded, HC, re-encode are dropped. Check if the available torrents on Nyaa for this anime are all in one of these categories.

**6b. Episode number extraction** (`reEpisodePatterns`):
For single-episode searches, a torrent is only kept if its extracted episode number matches the target. Check if the torrent names on Nyaa use an unusual pattern not covered by `reEpisodePatterns`.

**6c. Season/Part filtering**:
If `ExtractAnimeSeasonPart` returns a season or part, results are filtered to match. Check if the season/part extracted from the title is correct. A wrong extraction causes all results to be dropped.

**6d. Batch detection** (`reBatchPatterns`):
Single-episode searches drop batch torrents. If Nyaa only has a batch available for a finished anime, the single-episode search will return nothing (correct behavior — the batch strategy in `resolveSearchStrategy` should have caught it first).

**6e. Title match / Jaccard filtering** (`titleMatchesQuery` in `nyaa_match.go`):
This check runs before episode/season/batch filters and rejects torrents in two ways:
1. All query title tokens must appear in the torrent name.
2. Jaccard similarity between query tokens and torrent tokens must meet a length-dependent threshold: ≥ 0.8 for queries ≤ 3 tokens (strict — prevents "SAO" matching "SAO Alternative Gun Gale Online"), or ≥ 0.4 for queries ≥ 4 tokens (looser — allows long queries to match fansubs that embed both title translations in the torrent name).

Metadata tokens not in `titleTechnicalTokens` inflate the torrent's token count and lower Jaccard — this is the most common cause of `results: 0` when Nyaa clearly has the anime.

To diagnose: trace `extractTitleTokens` on both the query and a torrent name, then compute Jaccard manually:

```
query tokens   = extractTitleTokens("<search variant>")
torrent tokens = extractTitleTokens("<torrent name from Nyaa>")
jaccard        = |intersection| / |union|
```

If Jaccard < 0.8, add the offending torrent-side tokens to `titleTechnicalTokens` in `nyaa_match.go` (streaming service tags like `nf`/`amzn`/`cr`, codec fragments like `eac3`/`ddp2`, subtitle markers like `multisub`/`multi`/`subs`, etc.).

---

## Step 7 — Write a failing test

Once the root cause is identified, write a test before fixing:

- For search query issues: add a case to `src/tests/unit/nyaa_test.go` with the failing title and assert the correct variant is tried.
- For regex/filter issues: add a torrent name to the relevant test table in `nyaa_test.go` and assert the correct episode/season/part is extracted (or that the torrent is not incorrectly filtered).
- For `ExtractAnimeSeasonPart` issues: add a case to `src/tests/unit/season_part_test.go`.

Run tests:

```bash
go test ./...
```

---

## Step 8 — Fix and verify

Fix the logic, run tests, then trigger a manual download cycle to confirm:

```bash
build/autoanimedownloader trigger
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

**Rebuild before verifying** — changes only apply after a new binary is built:

```bash
go build -o build/autoanimedownloader ./src/cmd/daemon
```

Then trigger and check:

```bash
build/autoanimedownloader trigger
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

If `attempts` is now `> 0` (even if some fail), the search is working and the fix moved the problem downstream.
