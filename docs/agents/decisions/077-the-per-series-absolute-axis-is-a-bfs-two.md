# 77. The per-series absolute axis is a BFS two generations at a time, and the cut level goes back to the queue

**Location:** `src/internal/anilist/series.go` — `Series`, `GetSeriesIndex`, `walkSeries`,
`recordLink`, `prequelOf`, `resolveSeries`, `seriesCache`. Pinned by `series_test.go`.
Implements what #71 measured; #9 is the offset *gate* of a single part.

**What it looks like:** to know a cour's absolute episode it is enough to read the anime's
`relations` and sum the `PREQUEL`'s episodes — which is what `daemon.ComputeEpisodeOffset` does.

**Why that is not enough.** `ComputeEpisodeOffset` walks **one** hop and only when the title
declares `Part >= 2`. That covers `Shingeki no Kyojin: The Final Season Part 2` → `+16`, but not
the series: that cour's real absolute is `76..87`, which requires summing the **five** previous
entries. Because an AniList query expands at most 2 levels of `relations` (#71), resolving a
7-level chain is not a read — it is a walk.

**The design, in three rules:**

1. **Each request walks two generations, not one.** A media's response brings its `relations`
   (level 0) and its prequel's (level 1), both authoritative. From that come **two** recorded
   links and **three** episode counts per media fetched. `Page(media(id_in: [...]))` with up to 50
   ids costs the same budget unit as a single id (#72), so width is free and the only dimension
   that costs is height: `ceil(height/2)` rounds.
2. **The grandfather goes back to the queue.** The level-2 node comes with a reliable id, format
   and count, but its `relations` arrives as `edges: []` — the cut, indistinguishable from "has no
   prequel" (#71). Recording it as root produces a **silently wrong** offset: on the Shingeki
   fixture, Final Season Part 2 would give `38..49` instead of `76..87`. That is why `recordLink`
   is only called on levels 0 and 1, and level 2 is re-enqueued. Only a node recorded by
   `recordLink` enters the cache — a level-2 node knows how many episodes it has, but not who
   comes before it, and a half-fact is not a fact.
3. **The loop is depth-agnostic.** The ceiling of 2 is observed behaviour, not a contract: it
   consumes what comes, enqueues what is missing. An AniList that started expanding 3 levels would
   only make the walk finish in fewer rounds, without changing a line.

**The cache is 24h because the data is immutable and the walk is monotonic.** Only ancestors
enter, and an ancestor of a finished anime also finished: a `FINISHED` with an episode count does
not gain an episode or a new prequel. A `RELEASING` node or one with no count (`episodes: null`,
unreleased) counts as 0 and is **not** persisted — freezing that zero for 24h would shift the
whole series down on the day the count appeared. In memory, no file: the whole warm-up costs a
few requests, so persisting to disk would be invalidation and migration for nothing.

**Absent ≠ root, in both directions.** An id AniList does not return stays **out** of the map
(same contract as `GetMediaByIDs`), and an unknown ancestor in the middle of the chain stops the
sum there. Both choices err toward the **smaller** offset on purpose: a missing offset falls back
to relative numbering, which a good part of the groups use; a wrong offset sends the search to an
episode that does not exist (#71).

**The measured chains became a test fixture, not markdown** — Mushoku Tensei (4 levels),
Shingeki no Kyojin (6) and Monogatari (7). The strong validation is Shingeki: `76..87` is exactly
the numbering SubsPlease and Erai-raws use on the files. The test seeds **only the ends** of each
chain: seeding every id would make each ancestor arrive as level 0 and the cut would never be
exercised — verified by mutation, the previous version of the test passed with rule 2's bug.

**Don't "fix" by:** treating `edges: []` as root; assuming depth 2 as a contract; fetching one
ancestor per request (height becomes the cost, and AniList's budget is 30/min); caching a level-2,
`RELEASING` or no-count node; persisting the offset to disk or on the episode record (see the
discarded alternative in #78 — a persisted value rots when AniList corrects a count, and the
derived one self-corrects on the next TTL); replacing the walk with `search:` and the franchise
name (#71).
