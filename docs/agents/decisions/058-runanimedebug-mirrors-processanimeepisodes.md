# 58. `RunAnimeDebug` mirrors `processAnimeEpisodes`: enumeration, pack resolution and single fallback

**Location:** `daemon/debug.go` — `RunAnimeDebug`; mirrors `daemon/episodes.go` — `processAnimeEpisodes`.

**What it looks like:** debug of an anime does not iterate `anime.Media.AiringSchedule.Nodes`; it builds the list with `anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`, runs the same per-anime search + `partitionSearchResults`/`pickBatches` to decide whether a pack covers the window (and, if so, re-runs `selectEpisodes` with the ceiling lifted), and falls into `searchSingleEpisode` when no pending episode received a pack/movie magnet — three duplicated stretches of the production loop. `willBatchAnime`/`resolveSearchStrategy` left: pack eligibility is the filtered search result, not AniList metadata ([#59](059-batch-eligibility-stopped-being-metadata-and.md)).

**Why it's right:**

**(a) The raw schedule describes another pipeline.** On One Piece schedule page 1 goes from 1123 to 1147; production enumerates 1..1147 and takes the first 12 by the ceiling. Iterating the raw nodes, debug reported having searched "1123 to 1147" — no clipped-schedule anime was measured correctly, and that is exactly the class the synthetic `EpisodeList` (#52) exists to cover.

**(b) The pack↔limit rule was never evaluated.** `pickBatches`/`assignBatches` lift the per-anime ceiling in `processAnimeEpisodes` when a pack covers the window; without running the same path, debug was blind to the very rule the standalone-anime feature declares as a prerequisite ("an 1100-episode anime added by hand falls on the limited one-by-one path, unless a pack covers the window").

**(c) Without the fallback, a long series was a guaranteed false positive.** The multi search does not put an episode number in the query (`q=one piece`), and the first pages ordered by seeders are only recent episodes — measured: 750 raw rows, none of episode 1-12. Who finds the old episode is `searchSingleEpisode`, which carries the zero-padding (#56). Without it the `debug-batch` report marked One Piece and Naruto Shippuden as "0 magnets" for **not having searched**, not for Nyaa not having them.

**(d) `savedEpisodes = nil` is intentional.** `firstEpisodeToConsider` only uses that argument to walk the start back below `Progress + 1`, and debug already declares — in the `ponytail:` and the `Warn` it emits — that it treats every episode as not-downloaded. With `nil` it resolves to `Progress + 1`, which on a standalone is 1.

**Accepted trade-off:** `summary.Episodes` now has one entry per enumerated episode (~1147 on One Piece, most with `would_search: false`), and a short FINISHED enters batch, lengthening the run. That is the cost of fidelity; `debug-batch`'s `report.md` aggregates.

**Don't "fix" by:**
- Going back to iterating `AiringSchedule.Nodes` "because it's simpler" — the simplicity produces a diagnosis of a pipeline the daemon does not run.
- Stopping at pack resolution "because that's the step that decides the strategy" — it is only half the search; the other half is the per-episode fallback.
- Truncating `summary.Episodes` to those with `would_search: true` — the report's "nothing searched" count depends on seeing both sides.
