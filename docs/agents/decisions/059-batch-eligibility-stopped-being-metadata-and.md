# 59. Batch eligibility stopped being metadata and became a result filter

**Location:** `daemon/episodes.go` — `partitionSearchResults`, `pickBatches`, `packSet.covering`; `nyaa/nyaa.go` — `ExtractBatchInfo`, `ScrapNyaaForAnime`.

**What it looks like:** no `FINISHED` check, no check of the `Media` episode count before deciding whether an anime "becomes a pack". The whole decision lives inside the already-filtered search result.

**Why it's right:** what matters for pack eligibility is the torrent that is there — size, seeders, covered range — and none of that is known **before** the search. An episode-count cap (like the old `max_batch_episodes`) is a bad proxy for size: measured, 26 remux episodes go past 300 GB (would punch through any reasonable count cap) and 500 episodes in 480p sit at ~80 GB (a low count cap would block a pack that would fit comfortably). And the `FINISHED` gate excluded exactly who depends on packs the most — a long `RELEASING` series (One Piece, Naruto) never had another way not to accumulate 1000+ episode-by-episode records.

Today who decides is `partitionSearchResults` (splits packs from loose episodes and applies to EACH list its own size ceiling — `max_batch_torrent_size_gb`/`max_episode_torrent_size_gb` — and the seeders floor) followed by `pickBatches` (picks the fewest packs, among those that survived the filter, that cover the pending window).

**Known ceiling (ponytail, phantom-range guard):** `ExtractBatchInfo` only guarded the dominant phantom-range case — a resolution marker (`[720-1080p]`) being read as episodes 720-1080 — betting that other phantom ranges (date, bitrate) would fall into `EndEpisode == 0`, the safe fallback. **RESOLVED** (see #83): the bet was wrong for the year. `(2021-2022)` does not fall to zero, it becomes the range 2021..2022, and a complete top-seeded pack was discarded in silence for covering no episode.

**Known ceiling (ponytail, pagination floor):** `ScrapNyaaForAnime` fused pack and loose episode into one search, and the pagination floor (`enoughCandidates`, #57) started counting BOTH lists summed. A page 1 with 3 packs that the size filter later discards ends the descent without having gathered a single loose episode — before, with two separate searches, the second would descend on its own. **RESOLVED** (see #80): it showed up measured, on One Piece. The pack ceiling started being pushed into the `nyaa` package by `applyNyaaSettings` — the same push mechanism as `SetMaxSearchPages`, not a direct read of `files.Config` — and a pack above the ceiling leaves before counting toward the floor. The seeders filter stays only in the daemon: Nyaa returns ordered by seeders desc, so it does not truncate the descent the same way.

**Don't "fix" by:**
- Reintroducing an episode-count cap on the anime: count is a bad proxy for size (see above), and the `FINISHED` gate excluded who needs packs the most.
- Accepting `EndEpisode == 0` as "range 0-0" instead of "complete pack": a real pack whose range failed to parse would disappear from selection.
