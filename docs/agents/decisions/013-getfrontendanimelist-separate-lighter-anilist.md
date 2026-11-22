# 13. `GetFrontendAnimeList` — separate lighter Anilist query for the API endpoint

**Location:** `internal/anilist/anilist.go` — `GetFrontendAnimeList`; called from `fetchAniListEntries` (`api/endpoint_animes.go`).

**What it looks like:** There are now two functions that both return `*AniListResponse` and differ only in which GraphQL fields they request. Looks like duplication.

**Why it's right:** `GetAllCurrentAnime` (used by the daemon verification loop via `searchAnilist`) needs `synonyms`, `relations`, and `format` to match torrents and compute offsets, but does not need `coverImage`. `GetFrontendAnimeList` (frontend `/animes` endpoint) needs `coverImage` for display but never touches `synonyms`, `relations`, or the `id` field on airingSchedule nodes. Keeping them separate lets each query stay within Anilist's complexity budget — `GetAllCurrentAnime` avoids the cost of fetching images, and `GetFrontendAnimeList` avoids the cost of fetching relations/synonyms. The return type is the same (`*AniListResponse`); unused fields simply remain at their zero values.

**Don't "fix" by:** merging back into one query. Requesting all fields from both call sites is what caused complexity-budget exhaustion (see decision 11).
