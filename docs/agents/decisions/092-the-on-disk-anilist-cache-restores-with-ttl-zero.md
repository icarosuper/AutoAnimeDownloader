# 92. The on-disk AniList cache restores with TTL zero, and a value served from it comes with `ErrFromCache`

**Location:** `anilist/persist.go` (the whole snapshot), `anilist/anilist.go`
(`ttlCache.snapshot`/`restore`, the failure paths of `GetAllCurrentAnime`,
`GetFrontendAnimeList`, `getMediaListEntry`, `GetCustomListsMap`), `anilist/standalone.go`
(`GetMediaByID`/`GetMediaByIDs`), `anilist/health.go` (`ErrFromCache`, `Health.CacheSavedAt`),
`files/filemanager.go` (`LoadAnilistCache`/`SaveAnilistCache`, `anilist_cache` file).

Negative fact: **there is no** cache mode, "I am offline" flag, repository layer or second read
front. `ttlCache` remains the only one, and disk only makes it survive restart.

**What looks wrong, part one:** a persisted cache that `get` never sees. Entries restored from the
snapshot get an expiry in the past (`restore(..., 0)`), so every happy-path read misses the cache
and goes to the network as before — the on-disk file looks useless.

It is deliberate, and it is the invariant that separates "survives a crash" from "started showing
stale data". Restoring with a live TTL would make every screen serve yesterday's list without even
trying AniList, and would save requests nobody asked to save. The only path that reads the snapshot
is the **failure** one (`getStale`), so no screen got older than it was. The exception is
`seriesCache`, restored with the full TTL: only a `FINISHED` anime link with an episode count
enters it, immutable by construction (see `recordLink`) — there is nothing to expire.

**What looks wrong, part two:** functions that return a value **and** an error at the same time,
and call sites that do `if err != nil && !errors.Is(err, anilist.ErrFromCache)`.

`ErrFromCache` is not "it went wrong", it is "it went right with data from another time". It goes
as an error because that makes the old behaviour the **safe default**: every caller that does not
know the sentinel sees `err != nil` and behaves exactly as before the cache existed. Only whoever
checks `errors.Is` opts into using the value, and the choice stays visible at the call site instead
of hidden in a flag someone forgets to read. A boolean on the return would invert that — whoever
ignored the boolean would start treating days-old data as fresh without writing a line.

Two call sites deserve attention:

- `api/endpoint_animes.go` **refuses** the orphan refresh when the list came from cache. Coverage
  is known (the list came back whole), but each uncovered anime would become a `GetAnimeInfoByIDs`
  doomed to fail, on every 30s poll per tab — #65's burst, now without even a chance of succeeding.
- `daemon/verification.go` **accepts** the cached answer for status deletion (since [#130](130-status-deletion-asks-anilist-only-about-the-animes.md), `listStatusCache`'s per-(account, media id) statuses, not a cached list). It is an explicit
  decision of the project owner, and the risk has a name: an anime that left a deletion status
  *after* the snapshot (`COMPLETED` that became `REPEATING`, and the pass downloaded the season)
  shows up as deletable again, and the pass deletes what it just downloaded. Before the cache that
  was impossible — an account with no answer did not enter `listStatuses` and
  `allAccountsAgreeOnDelete` refused on its own. If it ever bites, the fix is one line: treat
  `ErrFromCache` as an account with no answer there.

**Why there are three `MediaList` caches, not one** (`frontendListCache`, `passListCache`,
`mediaEntryCache`): the three queries ask for **different** field sets. The frontend one brings
`coverImage` and does not bring `relations` or the entry's `status`; the pass one brings
`relations` and `status` and does not bring `coverImage`; the per-media-id one (`mediaByIDFields`)
brings everything. Serving the detail screen from the frontend cache would leave a manual download
without `relations` — and widening the 30s-poll query to cover that is exactly what #57 and #65
tightened.

**`stampAiringAt`/`rebaseAiring` exist because `timeUntilAiring` is relative to the fetch
instant.** Every "already aired" gate reads that field (`daemon/episodes.go`,
`anilist/episodes.go`, `daemon/manual_download.go`), so served raw from cache it freezes and the
pass concludes forever that nothing new premiered. The write converts to an absolute `airingAt`
(only where the API did not send it) and the read recomputes the countdown. That is what makes the
episode that was ten minutes away when AniList went down enter the search on its own, **with no
new rule** — the pass already searches when the countdown goes negative.

**Relation to #72:** the budget gate's refusal still serves the expired value **without** an
error. The data there is seconds old and the next poll comes back fresh; the screen gains nothing
from knowing. Only a real failure carries `ErrFromCache`.
