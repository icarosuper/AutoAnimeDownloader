# 11. `GetCustomListsMap` — separate lightweight query + cache for `customLists`

**Location:** `internal/anilist/anilist.go` — `GetCustomListsMap`; called at the top of `searchAnilist` (`verification.go`) and `fetchAniListEntries` (`api/endpoint_animes.go`) before `GetAllCurrentAnime`.

**What it looks like:** We call Anilist twice per verification cycle: once with a minimal `id + customLists` query, and once with the full `GetAllCurrentAnime` query. Then we overwrite `ml.CustomLists` from the first result when the second comes back with `null`. Looks like redundant work and a band-aid.

**Why it's right:** Anilist's GraphQL engine enforces a query-complexity budget. `GetAllCurrentAnime` includes `airingSchedule.nodes` (which is large for currently-releasing anime — it contains all future episode schedules) plus `relations`, `synonyms`, and `coverImage`. For users watching many currently-airing animes, this query exceeds Anilist's complexity threshold, causing the API to return `null` for the `customLists` scalar on every entry in the response — silently, with HTTP 200 and no error field. COMPLETED/DROPPED animes are unaffected because their `airingSchedule` is empty.

**Root-cause confirmed by:** back-to-back curl tests: Run 1 returned 0/20 null, Runs 2–5 returned 20/20 null (rate-limit exhausted). The user also ran a manual minimal GraphQL query (only `id` + `customLists`) which returned correct data including `{"AutoDownloader": true}` — confirming the API works; only the complex query suppresses the field.

**Effect when broken:** `customLists` is always `null` → `animeIsInExcludedList` never matches → blacklisted animes (e.g. in the "AutoDownloader" custom list used to block titles) are downloaded anyway and the frontend block icon is never shown.

**Budget gate:** when the caller passes `PriorityDisposable` and the gate refuses, the function
serves the **stale** cache read instead of returning `nil` — see #72, "The poll pair degrades
together or not at all".

**Cache:** `GetCustomListsMap` caches results keyed by `username + statuses` (`ttlCache`, see
#46). A response with at least one non-null `CustomLists` stays 5 minutes
(`customListsTTL`); an empty response stays only 30 seconds (`customListsEmptyTTL`), because
"empty" can be either an account with no custom lists or a field AniList degraded. Empty used to
not be cached at all, and then an account with no custom lists generated one request per frontend
poll — exactly the amplifier that blew the limit. The cache read happens before the fetch, so no
degraded response gets to evict an still-valid entry.

**Overlay guard:** `if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 { ml.CustomLists = cl }` — the `len(cl) > 0` guard ensures that a rate-limited nil response from `GetCustomListsMap` (which would produce an empty map entry) never silently clears data that `GetAllCurrentAnime` might have returned correctly on a lucky call.

**Don't "fix" by:**
- Removing `GetCustomListsMap` and relying solely on `GetAllCurrentAnime` — `customLists` will be null whenever the complex query hits the complexity limit, breaking blacklist exclusions entirely.
- Removing the cache — rapid successive calls (frontend polling, concurrent goroutines) will exhaust rate limits, causing all calls to return null.
- Removing the `len(cl) > 0` guard — an empty map overwrites any valid data the complex query returned, flipping all animes to "not blacklisted."
