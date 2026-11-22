# 72. AniList budget is measured from the response headers, not from a counter of ours

**Location:** `src/internal/anilist/anilist.go` — `sendAnilistRequest`; `health.go` — `Priority`,
`ErrBudgetLow`, `recordBudget`, `budgetAllows`, `retryAfter`, `HealthRateLimited` classification.
Pinned by `budget_test.go`. Complements #65, which measures **who** spends; here is **how to know
how much is left**.

**What it looks like:** to not take a 429, the natural path is a client-side limiter
(`golang.org/x/time/rate`) or a queue that holds a request when estimated consumption passes the
ceiling.

**Why it's wrong.** AniList returns the bucket state on **every** response, and the header is
strictly more precise than any internal counter. Measured on 28 Aug 2026:

```
minimal media           HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 29
normal 200               HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 28
404 (media does not exist) HTTP 404  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 27
invalid query            HTTP 400  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 26
heavy nested query       HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 25
```

Four consequences, all with design effect:

1. **An error consumes quota.** A 404 and a malformed query cost 1 each. Blind retry on top of an
   error burns budget, and a counter that only counted successes would be optimistic exactly when
   things are already going badly.
2. **Complexity costs nothing.** The 33 KB nested query decremented 1, same as the minimal query.
   There is no charge for complexity or for number of ids: **maximizing work per request is
   strictly correct, there is no trade-off.** That is why `Page(media(id_in: [...]))` (up to 50
   ids, `perPage` max 50) is worth it whenever there is a fetch-by-id loop. Applied in
   `GetMediaByIDs` (standalones, `media(id_in:)`) and `GetAnimeInfoByIDs` (orphan refresh,
   `mediaList(mediaId_in:)`), both on 29 Aug 2026.
3. **The header is authoritative because it sees the other consumers.** It already sums the daemon
   pass, the frontend poll and the standalone searches — the three compete for the same bucket per
   IP, with no coordination. No internal counter comes close, because none of them sees the
   others' errors.
4. **The bucket resets at once, it does not slide.** Measured: `Remaining` went 24 → 20 in a
   burst, sat at 19 at 21s and came back to **29** at 41s. It is a fixed bucket with a discrete
   reset, not a sliding window with gradual refill — in practice, **the budget recovers whole in
   ≤ 60s**.

**`X-RateLimit-Reset` cannot be used.** It shows up advertised in
`Access-Control-Expose-Headers`, but it is **not present** on any observed 200 response. #65's
table cites it because the official docs cite it; measurement did not find it. A 429 was not
provoked on purpose (it blocks the IP for ~1 min and the user's daemon shares that IP), so
**which headers a 429 carries remains unverified** — what is known is that `health.go` already
reads `Retry-After`.

**The design that follows: reserve by priority, not a queue.** Record `X-RateLimit-Remaining` and
the instant of the read in an atomic next to `health`, on every response **including error**, and
compare against a floor before emitting. Blocking and waiting is the wrong answer — the frontend
poll runs inside an HTTP handler and would hang the screen. The right answer is to degrade what is
cheap to degrade:

| Caller | Criticality | Action with budget low |
|---|---|---|
| Daemon pass | high — losing the pass is an episode not downloaded | always passes |
| Frontend poll (`GetFrontendAnimeList` **and** `GetCustomListsMap`) | low — they already have cache (60s and 5min) | refuse and serve stale cache — **both**, or the degrade is asymmetric |
| Standalone search from the UI | low | refuse, with an error visible to whoever asked |
| Warm-up / prefetch work | low — has a fallback | postpone to the next pass |

**The mandatory escape hatch:** if everyone is refused, nobody emits a request and the process
never discovers that the bucket reset. The read needs a validity — 60s after the last observation,
treat the budget as full again. That is what the reset measurement is for, and it is what stops
the gate from deadlocking itself.

**How it ended up implemented** (29 Aug 2026). `recordBudget` stores `X-RateLimit-Remaining` and
the instant in an `atomic.Pointer`, right after `httpDo` — before any status branch, so the 404
and the 400 count. `budgetAllows` is the gate, at the top of `sendAnilistRequest`: critical always
passes; disposable passes if the read is expired (60s) or if `remaining >= budgetFloor` (10 of
30). The refusal is `ErrBudgetLow` immediately — it never waits.

`Priority` is a **required parameter** of `sendAnilistRequest` and rises to the four mixed-caller
public functions (`GetCustomListsMap`, `GetAnimeInfo`, `GetMediaByID`, `GetMediaByIDs`), which the
daemon and the frontend call with opposite criticalities. The others pin theirs in the body. There
is no implicit default, and that is the feature: a new query does not compile without someone
deciding. The discarded alternative was a wrapper with `sendAnilistRequest` becoming "normal
priority" — smaller diff, but it creates exactly #69's inert-config class.

**The dividing line is not "daemon vs. frontend", it is "repeats on its own vs. someone is
waiting".** Disposable is recurring traffic with a fallback: the `/animes` poll
(`GetFrontendAnimeList`, `GetCustomListsMap`, `GetAnimeInfoByIDs`, and the `GetMediaByIDs` of
`appendStandaloneEntries` and `refreshStandaloneOrphans`) and the per-keystroke search
(`SearchMedia`). Critical is the daemon pass **and** every click action — opening the detail
screen, adding a standalone, downloading an episode by hand: they are rare requests and expensive
to refuse, and sacrificing them returns no budget. A refused `GetFrontendAnimeList` serves the
**expired** cache (`ttlCache.getStale`) instead of dropping the poll; the other disposables
already treat a partial failure as missing data.

**The poll pair degrades together or not at all.** `fetchAniListEntries` is `GetCustomListsMap` +
`GetFrontendAnimeList`, and refusing only one of the two is worse than refusing both: the list
comes from the expired cache and answers **successfully**, `customLists` comes back `nil`, and the
merge concludes that no anime is on an excluded list. `GET /animes` shows a blacklisted one as
normal — in a response that looks healthy. That is why `GetCustomListsMap` also serves `getStale` when the gate
refuses (`TestCustomListsServesStaleCacheWhenRefused`). What remains is the window where there is
no expired read at all (process just up with the bucket already empty); then `nil` comes back and
#11's overlay guard keeps what the complex query brought. Nulling the whole list in that case
would be worse: `GetCustomListsMap`'s `nil` also happens on a network failure, and it would take
the screen down because of the cheap query even with the face answering.

No warm-up caller exists yet. `GetSeriesIndex` (#77) already receives the `Priority`, but nobody
calls it in production: the warm-up comes in together with ownership by coverage, which is who
builds the seed.

**Don't "fix" by:** adding `x/time/rate` or any client-side token bucket (guesses instead of
measuring, and does not account for the other consumers' errors); counting only 200 responses;
depending on `X-RateLimit-Reset`; blocking the goroutine waiting for the bucket to fill inside an
HTTP handler; refusing *every* caller when the budget is low, without the 60s validity; giving
`Priority` a default (wrapper, a "safe" zero value, or deriving it from the query —
`GetAllCurrentAnime` and `GetFrontendAnimeList` can be the same query with opposite
criticalities); using `getStale` outside the gate's fallback.
