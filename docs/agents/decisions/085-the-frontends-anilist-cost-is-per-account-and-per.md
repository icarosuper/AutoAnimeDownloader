# 85. The frontend's AniList cost is per ACCOUNT and per ORPHAN, not per tab

**Location:** `api/endpoint_animes.go` — `refreshOrphanAnimes`, `refreshStandaloneOrphans`;
`anilist/anilist.go` — `GetAnimeInfoByIDs`, `GetCustomListsMap`, `frontendListCache`,
`customListsEmptyTTL`. Complements [#72](072-anilist-budget-is-measured-from-the-response.md)
(how to know how much is left) and [#73](073-the-frontend-doesnt-query-anilist-directly-even.md)
(why not move the fetch to the browser).

**What it looks like:** each open tab polls `GET /animes` every 30s, so N tabs cost N times more
budget — that was the reading that motivated the idea of the frontend talking to AniList directly
(#73), and it is what the `ttlCache` comment described before the cache existed.

**Measured on 30 Aug 2026** (isolated daemon, real account, `NYAA_URL` dead so nothing downloads,
three 5-minute phases with the `/animes` poll at 30s reproducing `Status.svelte`):

| phase | tabs | orphans | AniList req per `/animes` call | req/min |
|---|---|---|---|---|
| A | 1 | 0 | 1.50 | 2.4 |
| B | 2 | 0 | **0.94** | 3.0 |
| D | 2 | 100 | **3.00** | 7.8 |

Three consequences:

1. **A tab does not multiply — the TTL absorbs.** Doubling the tabs CHEAPENED the cost per call
   (1.50 → 0.94): `frontendListTTL` (60s) and `customListsTTL` hold, and the second tab almost
   always hits a cache the first just filled. The ceiling of the list+customLists pair is the TTL,
   not the number of tabs.
2. **An orphan multiplies, and per tab.** `refreshOrphanAnimes` → `GetAnimeInfoByIDs` **has no
   cache at all**: it fires `ceil(orphans/50) × accounts` requests on EVERY `/animes` call. With
   100 orphans the cost per call tripled (0.94 → 3.00) and started scaling with tabs. It is the
   only `/animes` path outside the TTL, and it is what decides consumption of an install with a
   large library.
3. **Empty `customLists` is the largest consumer in normal regime.** An account with no custom
   list at all falls into the 30s `customListsEmptyTTL` instead of the 5 min `customListsTTL` — 2
   of the ~3 req/min of phases A and B, two thirds of the frontend cost when there is no orphan.

**The gate never needed to enter:** 0 refusals by `ErrBudgetLow` and 0 429 responses in 22
minutes, with a peak of 11 req/min in a bucket of 30. The only two failures were AniList's own
30s timeout — latency, not budget.

**The count that matters:** `(1 + ceil(orphans/50) × accounts) × tabs × 2/min`, plus ~2/min per
account of empty `customLists`. 2 tabs + 100 orphans + 1 account = 7.8/min (measured). 4 tabs +
200 orphans + 2 accounts ≈ 50/min, and then the gate starts refusing for real.

**Don't "fix" by:** raising `frontendListTTL` thinking a tab is the problem — the measurement says
it is not. If the budget tightens again, the target is putting a TTL on `GetAnimeInfoByIDs`, the
only path with no cache, and then looking at `customListsEmptyTTL`.
