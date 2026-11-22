# 65. AniList budget is spent by the orphan refresh, not by the daemon pass — and a 200's `errors` field is not read

**Location:** `src/internal/anilist/anilist.go` (`sendAnilistRequest`, `ttlCache`); `src/internal/api/endpoint_animes.go` (`refreshOrphanAnimes`); `src/internal/daemon/verification.go` (`searchAnilist`).

**Official docs:** [errors](https://github.com/AniList/docs/blob/master/docs/guide/graphql/errors.md), [rate-limiting](https://github.com/AniList/docs/blob/master/docs/guide/rate-limiting.md), [considerations](https://github.com/AniList/docs/blob/master/docs/guide/considerations.md). **The numbers from there are not copied here on purpose** — the documented limit is 90/min but today it is degraded to 30/min, and the docs themselves call that temporary. A copied number rots in silence and someone sizes a cache by it six months later. The docs are the source; here stays only what is ours.

**What it looks like:** the daemon is what talks to AniList, so the verification pass looks like the consumer of the limit, and `ttlCache`'s TTLs look like generic caution.

**Why it's right:** measuring per minute, per configured account:

| Source | Requests | Note |
|---|---|---|
| Daemon pass | ~0.3/min | 3 per pass (`GetCustomListsMap` + download `GetAllCurrentAnime` + `GetListStatusesByMediaIDs` for the animes on disk), every `check_interval` (default 10 min). Each list query is one request **per 50 entries**: the delete-status lists fetched whole were 23 pages for one real account, which is why deletion now asks only about the animes on disk ([#130](130-status-deletion-asks-anilist-only-about-the-animes.md)) |
| `/animes` poll | ≤1.2/min | `GetFrontendAnimeList` (TTL 60s) + `GetCustomListsMap` (TTL 5min) |
| `refreshOrphanAnimes` | **1 per account every 50 orphans** | `GetAnimeInfoByIDs` (`mediaList(mediaId_in:)`), no cache. Used to be **1 per orphan anime per account**, with a semaphore of 5 concurrent that limited parallelism and not the total. Fixed 29 Aug 2026; the standalone fallback in the same function became a `GetMediaByIDs` |
| `appendStandaloneEntries` | **1 per batch of 50 standalones** | `GetMediaByIDs` (`Page(media(id_in:))`), on both sides (`api/standalone.go` and `daemon/standalone.go`). Used to be `GetMediaByID` in a loop: measured 28 Aug 2026, with 20 standalones they were 20 of the bucket's 30 requests, spent just to build a screen. Fixed 29 Aug 2026; the cache stays **per id**, so the batch and the standalone lookup reuse each other |

The daemon pass is noise. Who blew the limit was the orphan refresh: one request per downloaded anime the filtered fetch did not cover, fired **on every frontend poll**. In a library with tens of animes outside the configured statuses, that was a burst of tens of requests — and AniList has a *burst limiter* separate from the per-minute limit, which punishes exactly that shape. **Fixed on 29 Aug 2026 by batching**, together with the standalones: the two bold rows above started costing `ceil(N/50)` instead of `N`.

**`mediaId_in` exists and is what makes the batch possible** (introspection + measurement, 29 Aug 2026). `Page.mediaList` accepts `mediaId_in: [Int]` next to `userName`, returns **at most one entry per id** (so 50 ids never go past one page — there is no pagination to follow), silently **omits** the id the account does not track, and costs 1 unit like any query. Two measured traps: a nonexistent `userName` answers **500**, and a private account answers **404 "Private User"** — neither is 400, so a malformed query is not distinguishable from a bad account by status.

That is why `mergeFailed` skips the refresh (`handleAnimes`, `endpoint_animes.go`) instead of treating "not covered" as "needs refresh": with the list failed, every downloaded anime becomes an apparent orphan, and the resulting burst feeds the failure. The TTLs of `frontendListCache` and `customListsCache` are also not generic caution — they are what makes the poll cost **independent of the number of open tabs**. Without them, two tabs and two accounts were already ~16 req/min of idle frontend.

**A 200's `errors` field is never read.** The docs are explicit: a 200 can carry an error in the envelope. `sendAnilistRequest` deserializes straight into the data struct and discards `errors`, so that case today becomes an empty response with no diagnosis. **This is not the cause of decision #11's null `customLists`** — that was verified by curl and comes back 200 *without* an `errors` field, it is a complexity budget. They are two distinct problems with the same apparent symptom, and confusing them makes you look for the fix in the wrong place.

**Documented taxonomy** (`errors[].status` when present, otherwise HTTP):

| Code | Meaning | Recoverable on its own? |
|---|---|---|
| 429 | Rate limit. 1 min timeout, with `Retry-After` and `X-RateLimit-Reset` | Yes |
| 403 | API off for instability | Only when AniList comes back |
| 403 (custom message) | IP blocked by hand | No |
| 400 | Invalid query / missing field → the schema changed, it is our bug | No |
| 404 | ID does not exist | Not a failure — becomes `ErrNotFound` |
| 5xx | AniList internal error | Yes, transient |

`errors[].validation` only exists on mutation; the app is read-only and ignores it.

**Don't "fix" by:**
- Optimizing the daemon pass "because it talks to AniList" — it costs 0.3 req/min; the gain is zero and the risk is not.
- Making `refreshOrphanAnimes` run even with the list failed, "so the screen doesn't go stale" — that is the feedback loop that produced the original 429.
- Removing the TTLs because "the daemon only fetches every 10 minutes" — who fetches every 30 seconds is each frontend tab.
- Copying AniList's current limit here as a constant — it is temporary and changes without notice.
- Estimating consumption with a counter of ours instead of reading `X-RateLimit-Remaining` from the response — see #72, which measures the bucket and shows why an error also costs quota.
- Going back to one request per id where a list fits — `Page(media(id_in: [...]))` for loose media, `mediaList(userName:, mediaId_in: [...])` for a list entry. One query costs 1 unit independent of the number of ids (#72). Both paths are already batched: `GetMediaByIDs` and `GetAnimeInfoByIDs`.
