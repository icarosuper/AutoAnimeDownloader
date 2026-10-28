# Troubleshooting Failed Downloads

Use this guide when investigating why animes are failing to download, to identify bugs or gaps in the search/filter logic.

---

## Step 0 — Fast path: `make debug-anime`

Before doing anything manual, try:

```bash
make debug-anime ID=<anilistId>
```

`<anilistId>` is the AniList **media** ID (visible in the web UI URL for that anime, `#/status/<id>`, or via `GET /api/v1/animes`). This runs a one-shot pass — no running daemon required (the torrent client is embedded, nothing external to start) — that:

1. Fetches the anime from AniList and logs the full response (title, progress, status, synonyms, relations, airing schedule).
2. Picks the episodes that would be searched (same `checkEpisode` logic the real loop uses).
3. Runs the real search/match pipeline (same movie→pack→single priority as production: `resolveMovie`, then `partitionSearchResults`/`pickBatches` over the anime search, then `searchSingleEpisode` for whatever isn't covered) against live Nyaa.
4. Logs **every raw torrent row** Nyaa returned for the query (`"Raw Nyaa row"`), then the **matched subset** (`"Found ... results"` with `matched_torrents` — name, seeders/leechers, size and health score of each candidate, in ranking order), then a per-episode summary (`magnets_found`).

Output goes to `.debug_<anilistId>_<N>/` in the current directory — nothing is written to `~/.autoAnimeDownloader`:
- `debug.jsonl` has the full trace from steps 1-4 above (one JSON object per line — grep/`jq` it, or just read it directly)
- `summary.json` has the quick per-episode view (`would_search`, `magnets_found`)

Comparing `debug.jsonl`'s raw rows against its matched set tells you immediately whether the problem is upstream (Nyaa has nothing for this title/episode) or in filtering (Nyaa has it, but it got rejected — see Step 6 for why).

For a whole population at once, `make debug-batch` runs this same pass over `scripts/robustness-animes.txt` and writes a triaged `.debug_batch/report.md` — start there, and only open the `debug.jsonl` of the animes it flags (see [Commands](commands.md)).

**Known limitations** (logged as a warning at the start of every run): episodes are always treated as not-yet-downloaded (no `episodes.json` / torrent-backend check), and excluded-list membership is never evaluated (the single-anime AniList query doesn't fetch custom-list data). If the anime is being skipped for one of those two reasons, this tool won't show it — fall back to Steps 1-8 below.

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

**`attempts > 0`** → magnets were found but the embedded torrent client rejected all of them. With the embedded client the usual causes are:
- **`ErrSessionNotReady`** — `completed_anime_path` is empty/unset, so the download path (`Config.DownloadPath()`, derived from it) is empty and no torrent session exists yet. Complete the config.
- **Magnet parse failure** — a malformed magnet (`parseInfoHash` couldn't extract the info hash).

**No search log for the whole anime** → before blaming the search, look for:

```json
{"level":"info","anime":"...","episode":N,"absolute_episode":M,"owner_anime_id":X,"message":"Adopting episode already covered by a downloaded pack"}
```

That is **ownership by coverage** (decisions.md #78): the episode is already inside a pack on
disk, downloaded under another AniList entry (typically the previous cour), so it never reaches
Nyaa — and that is the correct behavior, not a failure. The new record points at the hash that
already exists. If the file is actually not there, the problem is the declared range (`BatchStart`/
`BatchEnd`, read from the torrent name), not the search.

**An episode reported as not found, with no per-episode Nyaa search in this pass** → look for:

```json
{"level":"debug","episode":"Anime Name - Episode N","earlier_episode_missing":true,"message":"Skipping per-episode search: a recent search came back empty"}
```

That's the miss backoff ([decisions.md #100](decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes)):
the episode's own search came back empty recently (`earlier_episode_missing: false`), or an earlier
episode of the same anime ended the pass without a torrent (`true`). The report row keeps the stats
of the episode's last real search; one never searched itself shows `awaiting_earlier_episode`
("Not searched yet: an earlier episode is still missing"). To search it right now, run `make debug-anime` (Step 0) or restart the
daemon — the backoff lives in memory.

**`disk_full` / "insufficient free disk space" while `df` shows plenty free** → read the error's
`reserved by torrents still downloading` number before suspecting statfs. The guard counts what
the torrents already in the session still have to write, plus what the same pass already added
([decisions.md #101](decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space)),
and keeps `min_free_disk_gb` of slack after that ([decisions.md #93](decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_gb--0)):
a full queue refuses new torrents long before the disk itself fills. Free space comes back as the
queue finishes or when queued torrents are deleted. Last-check copy names that reservation; `disk_low`
is the same add-gate (size 0, including reserved) so the Status banner tracks it.

A separate, new failure class is **"the torrent downloaded but nothing shows up in the library"** — the download succeeded but the `organize` job (hardlink into `completed_anime_path` + webhook) is failing or retrying. See the embedded-client section below.

---

## Embedded-client reality (no qBittorrent)

The BitTorrent client is embedded (`github.com/cenkalti/rain/v2`) — there is no external qBittorrent to connect to, and no `qbittorrent_url` config. A few things to know when downloads misbehave:

- **Resume database:** rain stores resume data (piece bitfields) in a bbolt DB at `~/.autoAnimeDownloader/session.db` (Windows: `%APPDATA%\.autoAnimeDownloader\session.db`), kept outside the download path so it survives a `completed_anime_path` change (which moves the derived download path along with it). If torrents don't resume after a restart, check this file exists and is writable.
- **Ports / connectivity:** rain listens on a default port range (20000–30000). There is **no UPnP/NAT-PMP** — inbound peers may require manual port forwarding. This is optional: DHT + PEX still find peers without it. If a well-seeded torrent gets zero peers, suspect a firewall blocking that range (or no outbound connectivity at all). There is no `torrent_port` config in this version.
- **"Downloaded but not in the library":** completion triggers an `organize` job that **hardlinks** the video files into `completed_anime_path`. Diagnose with:

  ```bash
  grep '"Organize:' ~/.autoAnimeDownloader/daemon.log
  cat ~/.autoAnimeDownloader/pending_jobs.json   # a stuck organize job sits here, retrying
  ```

  The download path is derived from `completed_anime_path` (a hidden subfolder inside it), so the two are always on the same volume by construction — the old "different volumes" cross-device hardlink failure is no longer possible. The most common cause today is `"no video files found in <dir>"` — the torrent has no recognized video file to link.

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

**6a. Ignore list** (`ShouldIgnore` in `nyaa/priorities.go`):
Any substring in `priorities.ignore_list` discards the release, case-insensitive — by default `[dub]`, `[raw]`, `[hardcoded]`, `[hc]`, `re-encode`. Check if the torrents Nyaa has for this anime are all in one of those categories. The list is **user-editable** (Priorities screen), so read the running `config.json`, not the defaults, before concluding a release should have passed.

**6b. Episode number extraction** (`reEpisodePatterns`):
For single-episode searches, a torrent is only kept if its extracted episode number matches the target. Check if the torrent names on Nyaa use an unusual pattern not covered by `reEpisodePatterns`.

**6c. Season/Part filtering**:
If `ExtractAnimeSeasonPart` returns a season or part, results are filtered to match. Check if the season/part extracted from the title is correct. A wrong extraction causes all results to be dropped. Watch for sequels titled with only a roman numeral (e.g. "... II") — `ExtractSeason` (`nyaa.go`) has a roman-numeral fallback (`reRomanSeason` in `nyaa_regex.go`, decision 20) precisely because fansub releases for these often carry no other season marker; if a new roman-numeral case still fails, check whether the numeral is spelled differently in the torrent name than in the AniList title.

**6d. Batch detection** (`reBatchPatterns`):
Single-episode searches drop batch torrents. If Nyaa only has a pack available, the single-episode search will return nothing (correct behavior — the pack search/selection path, run earlier over the same anime search, should have caught it first; see [decisions.md #59-61](decisions.md#59-batch-eligibility-stopped-being-metadata-and-became-a-result-filter)).

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

If it falls below the threshold, also check the second attempt before touching the allowlist: the
match repeats both checks on each `altTitleCandidates(<tail after the marker>)` (decisions.md
#75), so the name can pass via an alternative title even when the truncation fails.

If Jaccard < 0.8, add the offending torrent-side tokens to `titleTechnicalTokens` in `nyaa_match.go` (streaming service tags like `nf`/`amzn`/`cr`, codec fragments like `eac3`/`ddp2`, subtitle markers like `multisub`/`multi`/`subs`, etc.).

**6f. No pack survives — the anime downloads episode by episode even with a pack on Nyaa**

Anchor symptom in the log: there are `Attempting to download episode` lines for every episode, and
there is **no** `Using batch torrents to cover the pending window` line (`daemon/episodes.go`).
That means the search came back with results, but no pack reached `pickBatches` — the problem is
filtering, not a missing pack. Confirm on the same search: `Found anime torrents on Nyaa` with
`results: N` and every name a single episode.

Recipe to find **which** filter killed each pack (this is what closed the Mushoku Tensei II
Part 2 case, `166873`):

1. Extract the raw names from the search: the pass's `Raw Nyaa row` lines in
   `~/.autoAnimeDownloader/daemon.log` (or from Step 0's `debug.jsonl`).
2. Run, for **each** pack name, the `nyaa` package functions that decide the drop —
   `IsBatch`, `ExtractSeason`, `ExtractPart` and `titleMatchesQuery` — with the query the daemon
   actually used (the variant, not the AniList title).
3. Build the table `torrent × batch × part × match`. The column that is `false` is the guilty
   filter, and each row can die for a different reason.

The three culprits already seen, all in the same investigation:

- ~~**`titleMatchesQuery` false because `truncateAtFirstMarker` ate the alternative title.**~~
  **FIXED** (decisions.md #75): when the cut at the marker fails, `titleMatchesQuery` tries each
  alternative title that comes after it (parenthesized groups, slash-separated segments).
  If you still see a pack die on this filter, check whether the alternative title is in a format
  `altTitleCandidates` does not see — brackets and loose text stay out on purpose.
- ~~**Hard part filter killing a whole-season pack.**~~ **FIXED** (decisions.md #79): on the
  pack path, `part` only rejects a **declared conflict** — a pack with no marker passes, because a
  whole-season pack usually has no such marker even though it contains the requested episodes. See
  `sources.md`, "Pack granularity and numbering".
- ~~**`ExtractPart` reading `Part 1 + Part 2` as `1`.**~~ **FIXED** (decisions.md #79): a name that
  declares two parts returns `nil`, and what decides becomes the range coverage.
- **The pack's range does not match the entry's numbering.** The pack exists, survived the filters,
  and `pickBatches` still does not pick it: its name is on another ruler (continuous season,
  absolute across the series). Run `newPackAxis`/`localRange` (`daemon/episodes.go`) with the
  entry's episode total and see whether any hypothesis covers — if none covers, it is the heuristic
  that needs another one, and the tie-break is the pack's file count.

**The comparison that closes a fast diagnosis:** run the same step on the **sibling** entry with no
"Part" in the title (Part 1 of the same season). If it finds a pack and Part 2 does not, there is
still a part-dependent filter on the path — it does not run on the sibling.

Also check whether the right title variant was even tried: `searchNyaaWithVariants`
(`daemon/search.go`) **stops at the first variant that returns any result**, and
`GenerateSearchTitleVariants` puts romaji first. If romaji brought 24 single episodes, the
English variant — which might have matched the packs — never ran.

> This suspect was **measured and discarded** on the corpus (decisions.md #76): after #75 there is
> no anime where a later variant finds a pack that romaji did not. Confirm before touching the
> loop — if you have a real case, that is the evidence that reopens the decision.

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
build/autoanimedownloader check
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

**Rebuild before verifying** — changes only apply after a new binary is built:

```bash
go build -o build/autoanimedownloader ./src/cmd/daemon
```

Then trigger and check:

```bash
build/autoanimedownloader check
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

If `attempts` is now `> 0` (even if some fail), the search is working and the fix moved the problem downstream.
