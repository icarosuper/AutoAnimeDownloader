# 84. A pack without a range in its name gets its coverage from the file list, not from assuming it covers everything

**Location:** `nyaa/nyaa.go` — `PackFileRange`, `TorrentResult.DetailURL`, `detailURL`, `fileListEntryName`; `daemon/episodes.go` — `packCandidate`, `packSet`, `maxPackDetailFetches`, `pickBatches`, `assignBatches`; `daemon/search.go` — `nyaaSearcher.packRange`.

**What it looks like:** pack choice makes one extra HTTP request, in the middle of a function that used to be pure, and a pack's range started being resolved lazily by a stateful type (`packSet`) instead of coming from a call to `ExtractBatchInfo`.

**Why it's right:** the pack name almost never carries the range. Measured on 29 Aug 2026 on the first 8 rows of `?q=<anime>+batch&s=seeders` for three animes: **19 of 24 packs declare no range at all**. That was not the rare case — it was the common case, and the path it followed stored wrong data in `episodes.json`:

`EndEpisode == 0` counted as "complete pack", `pickBatches` stopped the cursor there, `assignBatches` wrote `1..total` and **every** episode of the window became an `EpisodeStruct` pointing at the pack's hash. The ones the pack does not have stay recorded as downloaded forever: the Librarian links only the files that exist and nothing reconciles record with disk afterwards, so they never return to the search.

The case that forced the change, measured at `nyaa.si/view/1891610`: `[EMBER] … (Season 2 | Part 2) … (Batch)` declares no range and brings the files **13..24**. Under the Part 1 entry the daemon recorded 1..12 — intersection **zero** with what the torrent contains.

The detail-page file list answers before downloading, and it answers well: on the 7 packs measured, 574/574, 24/24, 23/23, 12/12 and 103/111 files gave the number (the 8 that missed are OP/ED, which should stay out). It comes from the `.torrent` itself, so it works on old content — unlike AnimeTosho's `files[]` (see `sources.md`).

**The files' range enters through the same `packAxis` as the name's range**, because groups use the same four numbering conventions in both places (`sources.md`, "Pack granularity and numbering"). On the EMBER case: 13..24, delta 12 of the "continuous across the season" hypothesis, becomes 1..12 local.

**Why on demand and with a budget.** The detail page costs ~30 KB (measured on 7 packs) against ~125 KB of a listing — it is not bandwidth, it is number of requests against nyaa.si. `packSet.rangeOf` only fetches when the range is asked for a pack with no range in the name, and memos including failure:

- resolving everything at once would spend 3 requests per anime per pass even when the first pack already declares the range and covers the window — the most common case;
- without a memory of failure, `assignBatches` (which walks the packs once **per episode**) would become one request per episode.

`maxPackDetailFetches = 3` because packs come ordered by quality and `pickBatches` rarely goes past the first ones to close the window.

**An unresolved range stores ZERO, not `1..total`.** Reaching `assignBatches` with no range means both sources failed (detail page down, single-file torrent, no readable name). The pack is still used — refusing it would make a down Nyaa zero everyone's packs — but `1..total` was a guess that `hasDeclaredRange` read as a **declared** range, and from there ownership by coverage ([#78](078-a-torrents-unit-of-ownership-is-coverage-on-the.md)) adopted, under another cour of the series, an episode the pack might not have. "I don't know" is the only honest record.

**Accepted trade-off:** with a zero range [#78](078-a-torrents-unit-of-ownership-is-coverage-on-the.md)'s `hasUnclaimedContent` has no span to compare and the deletion guard falls back to the old behaviour on that group. That is the right trade: losing a protection against premature deletion in a rare case is worth less than spreading an invented coverage to the series' other cours.

**Known ceiling, and MEASURED on 30 Aug 2026: it stays as-is.** The range is the `min..max` of the numbers read, so a pack whose files **restart** numbering per season (S01-S04, one folder each) gets the largest season's range instead of the total. The proposed tie-break was the per-folder file count, which [#79](079-pack-choice-asks-does-it-cover-the-window-not-is.md) already wanted. The measurement (`nyaa/live_pack_measure_test.go`, 61 real packs in 12 searches) says **not** to do it:

- **Frequency:** 3 of 48 packs (6%) in the neutral sample `?q=<anime>+batch&s=seeders`, 2 of 13 in the sample that aims at multi-season on purpose. The 5 cases: Vinland Saga (24/24), Jujutsu Kaisen (24/23), Steins;Gate Trix (25/24/4), EMBER SnK (25/12/12/10/16), Haikyuu Complete (25/25/10).
- **No download is lost.** Running the 5 through `packAxis.localRange` under each season's entry, the pack is chosen in all 8 combinations — delta 0 already hits, because a multi-season pack does contain the episodes of every season it brings, in each one's local numbering.
- **The tie-break would change none of the 8 results**, and would count an extras folder as an episode: `Creditless Opening 1` → 1, `OAD - 01` → 1, `Movie 01` → 1 (measured; on 61 packs those folders never stretched `min..max` past the main content, but they would enter a per-folder count directly).
- **The residue is another, and smaller:** the STORED range exceeds the entry's total in 4 of the 8 combinations — Haikyuu S3 stores 1..25 for a 10-episode entry, SnK S2 stores 1..25 for 12, SnK S3 +3, JJK S2 +1. That is over-claim for [#78](078-a-torrents-unit-of-ownership-is-coverage-on-the.md)'s ownership by coverage, and the fix for it is clamping the end at `totalEpisodes`, not reading folders.

The min-max of **restart** packs is lifted by [#116](116-restart-per-season-packs-flatten-onto-the.md): overlapping season groups flatten onto the concatenated axis. The per-folder file-count tie-break stays rejected.

**Gap measured together, this one with no fix:** `ExtractEpisodeNumber` returns `nil` for a file name with a loose number followed by a title — `My Hero Academia 39 Game Start [1080p].mkv`. They were the only 3 packs in the sample where neither the name nor the files resolved the range, all three of the same MHA release.

**Don't "fix" by:**
- Fetching the detail inside the searches' `parseRow`: it becomes one request **per listing row**. That is why `DetailURL` is only captured there and the read happens after the filter and the sort.
- Resolving every pack's range at once "to simplify `packSet`": spends the whole budget on packs that would never be chosen.
- Refusing a pack whose range was not resolved: a down Nyaa would download no pack at all, swapping a wrong record for no download.
- Going back to storing `1..total` "because the pack was chosen as complete": that was exactly the guess that fed ownership-by-coverage adoption with an invented range.
- Filtering files by extension in a list of the `files` package's own: `nyaa.IsVideoFile` is the same list the Librarian uses after download, and `files` imports `nyaa` (the other way would cycle).

**Amendment (see [#123](123-a-one-episode-entry-covers-episode-1-even-when-the.md)):** "an unresolved range stores ZERO" still holds for every
entry with more than one episode. For an AniList entry with exactly **one** episode the standalone
magnet path records episode 1 instead — there is no range to invent.
