# 76. `searchNyaaWithVariants` stops at the first variant that returns any result — measured, and it stays that way

**Location:** `internal/daemon/search.go` (`searchNyaaWithVariants`, `buildTitleVariants`).

**What it looks like:** the title-variant loop only continues to the next when the current one
returned **nothing** (`result != nil` already ends it). A variant that returns 24 loose episodes and
zero packs ends the search — the English variant, which might bring the pack, does not run.

**Why it's right:** the failure described above is real on paper, and it even got scheduled as a
feature ("F3 · Title variants on the pack path"). It was **measured** after #75 and does not show
up in practice: running `ScrapNyaaForAnime` + `partitionSearchResults` + `pickBatches` for **each**
variant of the 8 animes in `scripts/robustness-animes.txt` plus `146065` and `166873` (29 Aug 2026),
there is not a single anime where variant 1 returns a result and a later variant finds a pack it
did not. Where a pack exists on Nyaa, romaji already finds it:

| id | v1 (romaji) packs/batches | best later variant |
|---|---|---|
| 1735 Naruto Shippuuden | 1 / 1 | same (different pack, same coverage) |
| 1 Cowboy Bebop | 9 / 1 | unique variant |
| 108465 Mushoku Tensei | 3 / 1 | same |
| 20594 Sword Art Online II | 4 / 1 | unique variant |
| 101922 Kimetsu no Yaiba | 3 / 2 | 14 / 1 (more packs, worse coverage) |
| 166873 Mushoku Tensei II Part 2 | 3 / 1 | same |
| 235 Meitantei Conan | 0 / 0 | **0 / 0** on all four |

`235` is the only anime in the corpus that falls on the loose-episode path, and no variant finds a
pack for it — Nyaa has no pack of Detective Conan's first episodes. It is absence of content, not
of a variant.

After #75, the cost of running the next variant stopped having a counterpart: it would be one more
Nyaa search round **per anime, on every pass** where there is no pack — which is most airing
animes, because a current-season pack only appears at the end. Combined with adaptive pagination
(#57), it multiplies.

**Reopen when:** `make debug-batch` shows an anime whose variant 1 returns a result, zero packs
survive the filter, and a manual search with the English variant shows a pack that covers the
window. Then the change is two lines: continue the loop when `partitionSearchResults` returns no
pack.

**Don't "fix" by:** always running every variant and unioning the results (pays N searches per
anime for the measured gain above, which is zero), nor inverting the variant order
(`GenerateSearchTitleVariants` puts romaji first because that is what matches most releases).
