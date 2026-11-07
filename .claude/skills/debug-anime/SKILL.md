---
name: debug-anime
description: Diagnose why an anime (or one of its episodes) did not download in AutoAnimeDownloader — runs the real search/match pipeline against live AniList/Nyaa with `make debug-anime`, reads the trace, and walks down to the filter or search bug that dropped it. Use when the user says an anime/episode isn't downloading, "não baixou", downloads one by one instead of the pack, picks the wrong torrent, or asks to run debug-anime / debug-batch. Takes an AniList media ID as argument when known.
---

# Debug an anime that didn't download

Goal: say **where** the episode is lost — upstream (Nyaa has nothing), in the search query, in
result filtering, in pack selection, or after the download (torrent client / organize) — and, when
it is a logic bug, pin it with a failing test before fixing.

Argument: `$ARGUMENTS` (AniList **media** ID — not the MediaList entry ID; see
[decisions.md #16](../../../docs/agents/decisions.md#16-anime_id-is-the-anilist-media-id-it-was-the-medialist-entry-id-until-the-migration)).
No ID? It is in the web UI URL (`#/status/<id>`), in `GET /api/v1/animes`, or on the anime's
anilist.co page (`anilist.co/anime/<id>`).

**What touches the user's installation:** `--debug-anime` *reads* `~/.autoAnimeDownloader/config.json`
(usernames, priorities, limits) and hits live AniList and Nyaa, but writes nothing there and never
starts the torrent client. Reading `~/.autoAnimeDownloader/daemon.log` is fine. **Never** trigger a
pass on the user's running daemon (`autoanimedownloader check`) or edit their config without asking —
that downloads for real.

## 1. Run the debug pass

```bash
make debug-anime ID=<mediaId>        # = go run ./src/cmd/daemon --debug-anime <mediaId>
```

No running daemon needed. It:

1. Fetches the anime from AniList and logs the full response (title, progress, status, synonyms,
   relations, airing schedule).
2. Picks the episodes that would be searched (the same `selectEpisodes` the real loop uses).
3. Runs the real search/match pipeline — same movie → pack → single priority as production:
   `resolveMovie`, then `partitionSearchResults`/`pickBatches` over the anime search, then
   `searchSingleEpisode` for whatever isn't covered — against live Nyaa.
4. Logs **every raw torrent row** Nyaa returned (`"Raw Nyaa row"`), then the **matched subset**
   (`"Found ... results"` with `matched_torrents` — name, seeders/leechers, size and health score,
   in ranking order), then a per-episode summary (`magnets_found`).

Output goes to `.debug_<mediaId>_<N>/` in the current directory (`N` increments per run, so a
before/after comparison is two folders):
- `debug.jsonl` — the full trace, one JSON object per line (`jq`/grep it)
- `summary.json` — per-episode quick view (`would_search`, `magnets_found`)

**Raw rows vs. matched set is the first fork:** no raw rows → upstream or query (step 4); raw rows
that look right but nothing matched → filtering (step 5); matched but the wrong one won → ranking
or pack selection (step 5, 6f).

**Known blind spots** (also warned at the start of every run): every episode is treated as
not-yet-downloaded (no `episodes.json` / torrent check), and `excluded_lists` membership is never
evaluated (the single-anime query doesn't fetch custom lists). If the anime is skipped for one of
those reasons, go to the logs (step 2). Also: if the anime is on one of the user's lists, debug
uses the real entry (real `Progress`); otherwise it builds a standalone one.

The daemon running elsewhere (Docker, another machine)? `autoanimedownloader animes debug <mediaId>`
runs the same pass inside it — summary on the terminal, trace in the daemon log.

### Many animes at once — `make debug-batch`

Runs the same pass over `scripts/robustness-animes.txt` (live network, takes minutes) and writes a
triaged `.debug_batch/report.md`: a table per anime plus SUSPECTS (searched, 0 magnets), NOTHING
SEARCHED, ERRORS (exit ≠ 0) and NO GOOD SOURCE ON NYAA (best matched torrent under `WEAK_SEEDERS`,
default 10). Start from the report and open only the `debug.jsonl` it points at.
`bash scripts/debug-batch.sh --report-only` rebuilds the report from disk with no network.
The list's IDs must be animes the user does **not** track (see the file's header). Every anime that
fails in real life becomes a line there.

## 2. Read the real logs — when debug-anime doesn't reproduce it

```bash
grep -E '"level":"(warn|error)"' ~/.autoAnimeDownloader/daemon.log
zcat ~/.autoAnimeDownloader/daemon-*.log.gz | grep -E '"level":"(warn|error)"'   # archived
```

Classify the failure:

- **`"Failed to download episode after all attempts"` with `attempts: 0`** → search returned zero
  magnets; the download loop never ran. Upstream/search/filter — steps 3–5.
- **Same, `attempts > 0`** → magnets found, the embedded client rejected them all. Usually
  `ErrSessionNotReady` (`completed_anime_path` empty, so `Config.DownloadPath()` is empty and there
  is no session — complete the config) or a magnet `parseInfoHash` couldn't read.
- **No search log for the anime at all**, but `"Adopting episode already covered by a downloaded pack"`
  → ownership by coverage ([decisions.md #78](../../../docs/agents/decisions.md#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)): the episode is
  inside a pack on disk owned by another entry (usually the previous cour). Correct behavior. If
  the file really isn't there, suspect the declared range (`BatchStart`/`BatchEnd` from the name).
- **Reported not found, no per-episode search this pass**, with
  `"Skipping per-episode search: a recent search came back empty"` → the miss backoff
  ([decisions.md #100](../../../docs/agents/decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes)).
  `earlier_episode_missing: true` means an earlier episode is the one missing. The backoff lives in
  memory: debug-anime bypasses it.
- **`disk_full` while `df` shows space** → read `reserved by torrents still downloading` in the
  error. The guard counts what queued torrents still have to write plus `min_free_disk_gb`
  ([decisions.md #101](../../../docs/agents/decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space),
  [#93](../../../docs/agents/decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_gb--0)).
- **Downloaded but not in the library** → the `organize` job (hardlink into
  `completed_anime_path` + webhook) is failing; see [embedded-client.md](embedded-client.md).

## 3. Rule out Nyaa being down

```bash
grep -i "Error searching Nyaa\|status 50" ~/.autoAnimeDownloader/daemon.log
```

`502`/`503` is a Nyaa outage, retried next cycle — not a bug. The same anime failing on several
days is a logic problem.

## 4–6. Query, filters, pack selection

The step-by-step for "Nyaa has it but we don't find/keep it" — the exact search URL, title
variants, ignore list, episode/season/part extraction, batch detection, Jaccard title match, and
"no pack survives" — is in [filtering.md](filtering.md). Read it when step 1 points at the search
or at filtering.

## 7. Pin it with a failing test

Before fixing:

- search query issue → case in `src/tests/unit/nyaa_test.go` with the failing title, asserting the
  right variant is tried
- regex/filter issue → the torrent name in the relevant table in `nyaa_test.go`, asserting the
  right episode/season/part (or that it isn't dropped)
- `ExtractAnimeSeasonPart` → case in `src/tests/unit/season_part_test.go`

Watch it fail, fix, then `go test ./...`.

## 8. Verify with the same pass

```bash
make debug-anime ID=<mediaId>
```

Compare the new `.debug_<mediaId>_<N+1>/` against the first run: the episodes that had
`magnets_found: 0` should now have a magnet, and nothing that had one should lose it. If it came
from `debug-batch`, re-run the batch and check no other anime moved into SUSPECTS. The user's
running daemon only gets the fix after they rebuild and restart it — say so; don't trigger it.

If the anime is a new kind of failure, offer to add its ID to `scripts/robustness-animes.txt`.
If the fix is non-obvious, record it with the `record-decision` skill.

## Report

Tell the user: where the episode was lost (step and filter), the evidence (the torrent name and
the value that failed), and what was changed or what is missing upstream. Delete the
`.debug_*` folders you created, unless the user wants to keep them.
