# 57. Adaptive pagination: sequential, with a candidate floor, no per-anime budget

**Location:** `nyaa/nyaa.go` — `fetchSearchPages`, `parsePagesWith`, `enoughCandidates`, `SetMaxSearchPages`/`ActiveMaxSearchPages`; `files/filemanager.go` — `Config.MaxSearchPages`, `applyNyaaSettings`.

**What it looks like:** one pagination helper used by the three searches (`ScrapNyaa`, `ScrapNyaaForAnime`, `ScrapNyaaForMovie` — at the time of this decision there were four, `ScrapNyaaForBatch` and `ScrapNyaaForMultipleEpisodes` later fused into `ScrapNyaaForAnime`), which descends page by page **sequentially** while it has fewer than 3 accepted candidates, with a configurable ceiling.

**Why it's right:**

**(a) Page 2 was fetched ALWAYS.** On searches that resolve on page 1 — most of them — that was a wasted fetch. That saving is what pays for the 5-page ceiling without raising average traffic: a fixed ceiling of 5 would cost a burst of fetches per pass against nyaa.si, where 429/Cloudflare is expected.

**(b) Floor of accepted candidates, not of rows.** Nyaa returns ordered by seeders desc, so past a handful of candidates the next page only brings a worse option; 3 already gives the ranking a choice. An **empty** page also ends it: it means the query is over, and insisting up to the ceiling would be a guaranteed useless fetch.

**(c) Sequential, no parallel burst.** The plan called for 2-3 concurrent "when it actually needs to descend". Left out on purpose: each page is only asked for because the previous was not enough (parallelizing would require asking for pages that might not be needed, undoing (a)), the gain is only latency on a rare path, and a parallel burst is exactly what causes 429/Cloudflare — the problem the rest of the decision avoids. With no client throttle, this is the worst place to add concurrency.

**(d) Batch and movie entered the same path.** Both used to read only page 1 with a direct `httpGet`, without going through `fetchNyaaPage` — they did not paginate and **did not appear in the log**. Now they use the same helper and get the page log with it.

**(e) There is NO per-anime per-pass fetch budget.** The plan asked for one, so an unsolvable anime would not burn the ceiling every pass. Measured: the worst case is a search that never meets the floor and goes to 5 pages, ~2.3x today's fetches, and **only** on an anime that finds nothing. The budget would require per-anime state crossing `nyaaSearcher` and nyaa's exported functions — plumbing out of proportion to the risk. If the traffic bothers, the shortest path is lowering `max_search_pages`.

**Don't "fix" by:**
- Fetching pages 1..N in parallel "to make it faster" (see (a) and (c)).
- Swapping the floor for "always fetch the ceiling" (see (a)).
- Keeping descending after an empty page (see (b)).
