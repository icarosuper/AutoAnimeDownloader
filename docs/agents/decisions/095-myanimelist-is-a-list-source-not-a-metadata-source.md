# 95. MyAnimeList is a list source, not a metadata source

**Location:** `anilist/mal.go` (`GetMALEntries`, `GetMALList`, `malEntriesFor`, `ErrMALEmpty`);
the MAL halves of `GetAnimeInfo`/`GetAnimeInfoByIDs` (`anilist/anilist.go`); the call sites in
`daemon/verification.go` (`searchAnilist`, the delete fetch, `allAccountsAgreeOnDelete`,
`malAccountKey`) and `api/endpoint_animes.go` (`fetchMALEntries`) / `api/standalone_guard.go`.

A MAL account only answers "which anime, in which status, with what progress". Each MAL id is
mapped to an AniList `Media.Id` through `idMal_in`, and from there the entry is the same synthetic
`MediaList` a standalone anime gets (`Id == 0`, empty `CustomLists`, Media from `GetMediaByIDs`)
with the MAL status and progress on top. Identity stays AniList's
([#43](043-an-animes-identity-is-mediaid-status-is-a-per.md)),
so `episodes.json`, `anime_settings`, the routes and the library don't know MAL exists.

**What looks wrong, part one: the list comes from `myanimelist.net/animelist/{user}/load.json`**,
an unofficial endpoint, and not from the official API v2. The v2 answers `403` without a client ID,
and a client ID either ships in the binary (public, shared rate limit, one revocation breaks every
install) or makes every user register an app. `load.json` needs nothing, but only reads **public**
lists. If it breaks, the fallback is v2 with a user-supplied client ID: only `fetchMALList` changes.
`TestLive_MALList` (`AAD_LIVE_MAL=1`) is the alarm — it already caught `is_rewatching` arriving as
`""` on some entries.

**What looks wrong, part two: an empty MAL list is an error (`ErrMALEmpty`), not an answer.**
A private list, a format change on MAL's side (every field decodes to zero) and a total mapping
failure all look like an empty list. As an empty answer the account would silently lose its veto:
AniList account with X in `COMPLETED`, MAL account watching X, MAL comes back empty → the AniList
account alone is unanimous and X is deleted. As a failure, `allAccountsAgreeOnDelete` refuses,
the same as for an AniList account that didn't answer.

**What looks wrong, part three: the status filter runs before the AniList hydration.** The MAL
list is fetched whole (`status=7`) and cached raw (MAL id, status, progress); only the entries in
the requested statuses go to `GetMediaByIDs`. `mediaByIDCache` lives 60s and `/api/v1/animes` is
polled every 30s per tab, so hydrating a ~300-entry list (mostly `COMPLETED`) would cost ~6 AniList
queries per minute per MAL account — the burn
[#46](046-getfrontendanimelist-is-cached-for-60s-the.md)
and [#85](085-the-frontends-anilist-cost-is-per-account-and-per.md) closed. The deletion
vote never hydrates at all: it only needs AniList id + status.

**Caches:** `malListCache` (raw list, 60s) and `malIDMapCache` (MAL→AniList id, 24h, `0` = no
counterpart, cached so an unmappable entry doesn't cost a request per pass). Both go into the disk
snapshot: the list comes back with TTL zero like the other lists
([#92](092-the-on-disk-anilist-cache-restores-with-ttl-zero.md)),
the mapping with its full TTL like `Series`. MAL down → stale list + `ErrFromCache`.

**Don't confuse:** the `listStatuses` key of a MAL account is `"mal:" + user` (`malAccountKey`),
so an AniList and a MAL account with the same name don't overwrite each other. And
`MediaList.Id == 0` means "standalone **or** MAL" — never use it to tell them apart.

**Don't "fix" by:** making MAL a metadata source (a second identity and most of the app
duplicated); putting the MAL code in its own package (it needs `ttlCache`, the snapshot,
`sendAnilistRequest` and `GetMediaByIDs`, all internal to `anilist`); treating `ErrMALEmpty` as an
empty list; or hydrating before filtering.
