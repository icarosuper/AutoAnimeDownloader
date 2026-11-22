# 46. `GetFrontendAnimeList` is cached for 60s — the `/api/v1/animes` poll is what blows AniList's quota

**Location:** `internal/anilist/anilist.go` — `ttlCache`, `frontendListCache`, `GetFrontendAnimeList`, `frontendListResponse`, `clearCaches` (called by `MockAniListDo`); tests in `anilist_test.go`.

**What it looks like:** a TTL cache on a getter that "just reads a list", returning a copy of the slice instead of the stored slice. Looks like premature optimization on a request that takes ~1s.

**Why it's right:** AniList today limits to **30 req/min** per IP. `Status.svelte` polls `/api/v1/animes` every 30s **per open tab**, and the handler does one fetch per configured account (`fetchAniListEntries` = `GetCustomListsMap` + `GetFrontendAnimeList`). With 2 accounts and 2 tabs that alone is ~16 req/min of idle UI — add an F5, the 10-minute verification cycle and orphan `GetAnimeInfo` and the 429 is guaranteed. Real diagnosis (`daemon.log`, 2026-08-10 11:09): 15 requests to `/api/v1/animes` in 2 minutes, followed by cascading 429s that also took down the daemon's `searchAnilist` (`verification.go`) — i.e. the open screen stopped automatic download from running. With the cache, N tabs cost at most 1 request per minute per account.

The 60s TTL is safe because this list only changes when the user edits AniList; downloaded-episode counts come from the local `episodes.json` and stay instant. The key includes username + statuses, so a config change produces a new key (no invalidation needed).

**A copy, not the stored slice:** `fetchAniListEntries` overwrites `ml.CustomLists` on the entries it receives. Returning the cache slice would make two concurrent `/api/v1/animes` requests write the same memory (real race, caught by `-race`) and leak one caller's overlay into the next.

**Don't "fix" by:**
- Caching `GetAllCurrentAnime` too — that is the daemon cycle, which runs every 10 min and **needs** fresh data to decide download/deletion.
- Increasing the frontend poll interval instead of the cache — does not solve F5, multiple tabs, or multiple devices on the LAN.
- Returning the cache slice directly "to save the copy".
- Caching the orphan refresh (`GetAnimeInfoByIDs`) too: that path is already off when the list fails (#42d), which is when it amplifies — and since it became a batch (#65) it costs 1 request per account per poll, the same order as the list fetch next to it, which is cached.
