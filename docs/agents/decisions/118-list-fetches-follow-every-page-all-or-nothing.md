# 118. List fetches follow every page, all or nothing

**Location:** `internal/anilist/anilist.go` — `fetchMediaListPages`, used by `GetAllCurrentAnime`,
`GetFrontendAnimeList` and `GetCustomListsMap`. Pinned by `TestListFetchesFollowEveryPage` and
`TestListFetchFailsWholeOnAFailedPage` (`anilist_test.go`).

**What it looks like:** a loop on `pageInfo.hasNextPage` around queries that used to be one
request, plus a hard cap (`maxListPages`) that errors out instead of returning what it has.

**Why it's right:** a `Page { mediaList(...) }` without paging returns **50 entries** and no
error. An account with more than 50 anime in the queried statuses lost the rest silently: past
the 50th, an anime in `download_statuses` was never downloaded, one in `delete_statuses` was
never deleted, and its `customLists` never reached the exclusion check. The three queries page
together — paging the list but not `GetCustomListsMap` would leave entries past the 50th with no
exclusion data.

**All or nothing:** a failed page fails the whole fetch, so the caller falls back to its stale
copy. The pages that arrived are not a list: handed back, they would read as complete and an
anime past the failed page would look removed from AniList.

**Cost:** each page is one request against the budget ([#72](072-anilist-budget-is-measured-from-the-response.md)),
so a list costs `ceil(N/50)` per fetch. Cheap for watching lists; a big `COMPLETED` in
`delete_statuses` is where it shows up.

**Don't "fix" by:** returning the partial list on a failed page; asking for `perPage` above 50
(AniList caps it at 50); dropping the cap to "trust hasNextPage".
