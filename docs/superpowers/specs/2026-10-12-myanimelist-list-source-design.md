# MyAnimeList as a list source

**Date:** 2026-10-12
**Status:** implemented on 2026-10-12 — the final decision is [decisions.md #95](../../agents/decisions.md#95-myanimelist-is-a-list-source-not-a-metadata-source); see "What changed in the implementation" at the end
**Origin:** `v2.4.0` in [TODO.md](../../TODO.md) — "Add MyAnimeList integration"
**Expected scope:** `src/internal/anilist` (new `mal.go`, snapshot fields), `src/internal/files`
(config field), `src/internal/daemon` (pass + deletion vote), `src/internal/api` (anime list,
standalone guard, detail resolution), frontend Config + onboarding, docs.

---

## The idea in one sentence

A MAL account is **one more account that answers "which anime, in which status, with what
progress"**. Everything else — identity, episodes, airing schedule, series offset, covers, the
`tvshow.nfo`, the disk cache — keeps coming from AniList.

The anime identity is AniList's `Media.Id`
([decisions.md #43](../agents/decisions.md#43-a-identidade-de-um-anime-é-mediaid-status-é-uma-pergunta-por-conta-download--or-deleção--and)):
`episodes.json`, `anime_settings`, `blocked_episodes`, `standalone_animes`, the `/animes/{id}/*`
routes and the library folders are all keyed by it. Making MAL a second metadata source would mean
a second identity and a duplicate of most of the app. As a list source it never leaves the
`anilist` package: the rest of the code receives a `MediaList` exactly like the ones it already
handles.

## Reading the list: `load.json`

The official API v2 answers `403` without a client ID, and the client ID either has to be
embedded in the binary (public, with a shared rate limit, and the integration breaks for everyone if
it gets revoked) or registered by each user. Jikan v4 no longer has a user-list endpoint. The
choice is the endpoint MAL's own list page uses:

```
GET https://myanimelist.net/animelist/{user}/load.json?status=7&offset={n}
```

Measured on 2026-10-12 (user `Xinil`):

- No key, no login, and no special User-Agent needed: `Go-http-client/1.1` gets the same `200`
  as curl's. A nonexistent user answers **`400`** with `{"errors":[{"message":"invalid request"}]}`.
  The response for a **private** list was not measured — see Open questions
- `status`: `1` watching, `2` completed, `3` on hold, `4` dropped, `6` plan to watch, `7` all
- **300 items per page.** `offset=0` returned 300 and `offset=300` returned the rest, so the loop
  stops at the first page shorter than 300
- Fields used, per item: `anime_id` (MAL id), `status`, `num_watched_episodes`, `is_rewatching`.
  On completed entries `num_watched_episodes` equals the episode count

The whole list is always fetched (`status=7`) and filtered locally: the same list answers the
download filter, the deletion filter and the per-anime status question, so one cached fetch
serves all three (see "Call sites").

**Risk:** the endpoint is unofficial. If it breaks, the fallback is the official API v2 with a
**user-supplied** client ID: only `fetchMALPage` changes, everything downstream stays.

### Status mapping

| MAL | `MediaListStatus` |
|---|---|
| `is_rewatching = 1` (any status) | `REPEATING` |
| `1` watching | `CURRENT` |
| `2` completed | `COMPLETED` |
| `3` on hold | `PAUSED` |
| `4` dropped | `DROPPED` |
| `6` plan to watch | `PLANNING` |
| anything else | skip the entry, `Debug` log |

`is_rewatching` wins because MAL represents a rewatch as a flag on top of the status, while for
AniList it is a status of its own.

## MAL id → AniList id

Measured on 2026-10-12: `Page(perPage: 50){ media(idMal_in: $ids, type: ANIME){ id idMal } }`
returns the matching media and **silently omits** unknown ids (the probe with `99999999` returned
the other three). That's the same contract as the `id_in` in `GetMediaByIDs`, and it gets the same
treatment: "didn't come back" = AniList doesn't know it.

Two steps, both reusing what already exists, and **in this order with a filter between them**:

1. **Mapping** (`malIDMapCache`, new): only `id idMal`, 50 ids per query, over the **whole**
   list. A MAL→AniList mapping practically never changes → **24h** TTL, and a "not found" result
   is cached too (as `0`) so an anime with no counterpart doesn't cost a request on every pass
2. **Filter by list status** on the mapped raw entries (AniList id + status + progress)
3. **Media**, only for what survived the filter: `GetMediaByIDs(anilistIDs, priority)`. It
   already batches, caches per id (`mediaByIDCache`), persists to disk and falls back to stale
   data. Nothing new here

The filter has to come before the media step. `mediaByIDCache` lives 60s, and `/api/v1/animes`
is polled every 30s per tab: fetching media for the whole list (~300 entries, mostly `COMPLETED`)
would cost ~6 AniList queries **per minute per MAL account**, exactly the poll-driven burn that
[#46](../agents/decisions.md#46-getfrontendanimelist-é-cacheado-por-60s--o-poll-de-apiv1animes-é-o-que-estoura-a-anilist)
and [#85](../agents/decisions.md#85-o-custo-do-frontend-na-anilist-é-por-conta-e-por-órfão-não-por-aba) closed. Filtered to
`CURRENT`/`REPEATING` it's a handful of ids, so 1 query per minute — the same order as an AniList
account's list. The mapping costs ~6 queries once a day. The deletion vote (see Call sites)
needs only AniList id + status, so it never reaches step 3.

This goes through `sendAnilistRequest`, so the budget gate
([#72](../agents/decisions.md#72-o-orçamento-da-anilist-se-mede-pelos-headers-da-resposta-não-por-um-contador-nosso))
and the health tracking cover it without changes.

A MAL entry with no AniList counterpart gets a `Warn` log (with the MAL id and title) and is
skipped. It doesn't fail the pass.

## The resulting `MediaList`

```go
MediaList{Id: 0, Status: mapped, Progress: num_watched_episodes, Media: <from GetMediaByIDs>}
```

- `Id` (the AniList list-entry id) stays **0**: it doesn't exist for a MAL account. That's
  already safe: the synthetic `MediaList` of a standalone anime (`GetMediaByIDs`) has `Id == 0`
  too, and it already goes through the pass and the frontend list
- `CustomLists` stays empty, so `excluded_lists` **doesn't apply** to MAL accounts. This has to be
  documented in `config.md` and in the Config screen hint
- It's the same shape `GetMediaByIDs` + `withStandaloneProgress` already produce for standalone
  animes, so `shouldSkipEpisode`, `firstEpisodeToConsider`, pruning, `DedupeByMedia` (keeps the
  **lowest** progress across accounts) and `EpisodesWatched` handle it with no `isMAL` branch

## Cache and outage behavior

| cache | key | content | TTL | on disk |
|---|---|---|---|---|
| `malListCache` (new) | MAL username | raw entries (MAL id, status, progress) | 60s | yes |
| `malIDMapCache` (new) | MAL id | AniList id (`0` = none) | 24h | yes |

- 60s is the `frontendListCache` TTL, chosen for the same reason
  ([#46](../agents/decisions.md#46-getfrontendanimelist-é-cacheado-por-60s--o-poll-de-apiv1animes-é-o-que-estoura-a-anilist)):
  `GET /api/v1/animes` is polled every 30s per open tab. With the cache that's at most
  1 page fetch per minute per MAL account (2 for lists over 300)
- Both go into the `persist.go` snapshot as two new `omitempty` fields. An old snapshot without
  them loads fine, and they come back with TTL zero like the others
  ([#92](../agents/decisions.md#92-o-cache-em-disco-da-anilist-volta-com-ttl-zero-e-o-valor-servido-dele-vem-acompanhado-de-errfromcache))
- **MAL down**: serve the stale list and return `ErrFromCache`, the same contract as the AniList
  lists. Every caller already accepts `ErrFromCache` as an answer
- **AniList down**: the mapping and the media come from their persisted caches, so a MAL account
  keeps working in exactly the situations an AniList account does
- **No MAL health in `/api/v1/status`** in this version: a MAL failure is logged, and if it
  empties the pass it becomes a pass error with its own code (below). A MAL banner only gets added
  if this turns out to be a real problem

## Public API in `anilist`

The file goes **in the `anilist` package** (`anilist/mal.go`) and not in a new `internal/mal`: it
needs `ttlCache`, `persist.go`, `sendAnilistRequest` and `GetMediaByIDs`, all of them internal to
the package, and an `internal/mal` would either have to export all four or be imported by
`anilist` and still do the composition there. The package name stays `anilist` — the
rename to something like `lists` isn't worth the diff (21 files import it).

```go
// MALEntry is one MAL list entry already mapped to AniList: no Media, so it costs nothing beyond
// the MAL fetch and the 24h mapping.
type MALEntry struct {
	MediaID  int // AniList Media.Id
	Status   MediaListStatus
	Progress int
}

// GetMALEntries returns the user's whole MAL list (all statuses), mapped to AniList ids.
// Entries with no AniList counterpart are dropped. ErrFromCache when served stale.
// ErrMALEmpty when the full list has zero usable entries — see "An empty list is a failure".
func GetMALEntries(user string, priority Priority) ([]MALEntry, error)

// GetMALList is GetMALEntries filtered by list status, then hydrated with GetMediaByIDs into
// MediaLists carrying the MAL status and progress. Only the filtered ids reach AniList.
func GetMALList(user string, statuses []string, priority Priority) ([]MediaList, error)
```

No `Account` type and no interface: there are exactly two list sources.

### An empty list is a failure

A full fetch (`status=7`) that yields **zero usable entries** returns `ErrMALEmpty`, and every
caller treats it as a failed fetch for that account. Three things produce an empty list, and
none of them means "this account tracks nothing":

- a private list (if MAL answers it with `200 []` — see Open questions)
- a format change on MAL's side: Go decodes missing fields as zero, so every entry lands in the
  "anything else → skip" row of the status mapping
- every entry failing the AniList mapping

Treating that as an empty answer would be dangerous in the deletion vote. Example: an AniList
account has X in `COMPLETED`, and a MAL account is watching X. If the MAL list comes back empty,
MAL stops voting, the AniList account alone is unanimous, and X gets deleted. As a failed fetch,
`allAccountsAgreeOnDelete` refuses, which is the behavior it already has for an AniList account
that didn't answer. The cost is a genuinely empty MAL account blocking status deletion, which
is what an empty account *should* cost: it then has nothing to download either, so remove it from
the config.

## Config

```go
MALUsernames []string `json:"mal_usernames"`
```

- Default `[]`, normalized from `nil` in `LoadConfigs`, the same as `AnilistUsernames`
- **Not validated** (same as `anilist_usernames`, see config.md "Required Fields")
- Onboarding: the "at least one source" rule becomes AniList **or MAL** account **or**
  standalone anime (`onboarding.ts:40`, `Status.svelte:158`)
- Config screen: a second list field, the same component as `anilist_usernames`, with the hint
  "public list required; custom lists don't apply" (i18n `en` + `pt-BR`)
- CLI: one more row in the config table (`cmd/cli/main.go:303`)

## Call sites

There are seven places that iterate `AnilistUsernames` or receive the slice. Each one gets the MAL
accounts alongside, and they all reduce to `GetMALEntries` (status questions) or `GetMALList`
(anything that needs the `Media`). Every row treats `ErrMALEmpty` as a failed fetch:

| site | what it asks | MAL answer |
|---|---|---|
| `searchAnilist` (`daemon/verification.go:460`) | list in `DownloadStatuses` | `GetMALList(user, DownloadStatuses)`, then the same `MediaStatusAllowed` filter. Append **before** `DedupeByMedia` |
| delete fetch (`daemon/verification.go:180`) | list in `DeleteStatuses`, per account | `GetMALEntries` filtered by `DeleteStatuses` — no `Media` needed; key `inDeleteStatus` by **`"mal:" + user`** — an AniList and a MAL account with the same name would otherwise overwrite each other |
| `allAccountsAgreeOnDelete` (`verification.go:514`) | status of one media in one account | lookup in `GetMALEntries` (cached): absent = doesn't vote, present outside `DeleteStatuses` = veto. A failed fetch = no unanimity, the same as today |
| `/api/v1/animes` (`api/endpoint_animes.go:192`) | list in download statuses + media statuses | `GetMALList`, then `MediaStatusAllowed`, as in the pass. A MAL failure sets `mergeFailed`, a stale list sets `fromCache` — same orphan-refresh rules |
| `standaloneGuard` (`api/standalone_guard.go:99`) | which media are tracked | `tracked` from `fetchMALEntries` (as in `/animes`); never `blacklisted` (no custom lists) |
| `resolveAnimeDetails` / `resolveMediaList` (`GetAnimeInfo(id, usernames)`) | the entry with progress for one media | look the media id up in each MAL account's `GetMALEntries`, **before** the standalone fallback. When both an AniList and a MAL account track it, take the **lower** progress — the rule `DedupeByMedia` applies in the pass, so the detail screen and the pass agree on which episodes are watched |
| `refreshOrphanAnimes` (`GetAnimeInfoByIDs`) | entries for media no longer covered | look the orphan ids up in the MAL lists the same way |

The last two use the same lookup, so it's one helper next to them, not two.

**Pass error:** when **all** accounts fail and the list comes back empty, the code says which
source failed. A new `errCauseMAL` (`daemon/passerror.go`), with its own `pass_error_code` and
message in the frontend, so the banner doesn't blame AniList for a MAL outage. If both sources
failed, it stays `anilist`.

## Out of scope

- **Writing to MAL** (progress, status). The app doesn't write to AniList either
- **OAuth / private lists**
- **MAL health on the Status screen** — see Cache
- **Disk persistence beyond the two caches above**
- **Search / adding standalone anime by MAL**: search stays on AniList

## Tests

- `load.json` parsing and pagination (300 + rest, an empty page, a `400`) against `httptest`,
  in the style of the existing var-swap mock
  ([#1](../agents/decisions.md#1-var-swap-mocking-for-anilist-and-nyaa-http-clients))
- Status mapping, including `is_rewatching` over `COMPLETED`
- MAL id with no AniList counterpart: skipped, cached as `0`, no second request
- MAL down with a cached list → `ErrFromCache` + stale list
- `searchAnilist` with one AniList and one MAL account tracking the same media → one entry, the
  lower progress
- Deletion: an AniList account in `DeleteStatuses` + a MAL account with the same media in
  `CURRENT` → veto; the MAL account doesn't track it → deletes; the MAL list comes back empty
  (`ErrMALEmpty`) → no deletion
- Detail screen with an AniList and a MAL account at different progress → the lower one
- A `live_smoke_test.go`-style live test behind an env var (`AAD_LIVE_MAL=1`) that fetches a known
  public list, so a format change on MAL's side shows up as a test failure and not as a silent
  empty list

## Docs to update when implementing

- `config.md`: `mal_usernames`, plus the note that `excluded_lists` and `anilist_usernames`
  semantics don't extend to MAL
- `architecture.md`: `anilist/mal.go` lives in `anilist` on purpose; `MediaList.Id == 0` now
  means "standalone **or** MAL", never a way to tell them apart; the `"mal:"` key in
  `inDeleteStatus`; an empty MAL list is a failure, not an answer
- `decisions.md` **#95**: "MAL is a list source, not a metadata source: identity stays AniList's
  `Media.Id`, and the list comes from the unofficial `load.json`"

## Open questions

1. **What does `load.json` return for a private list?** It can't be measured without one. It
   doesn't change the design: a `400` is a failed fetch, and a `200 []` becomes `ErrMALEmpty`
   (see "An empty list is a failure"), which is also a failed fetch. What it changes is the log
   message: check with a real private list before shipping, so the `Warn` can say "the list is
   private" instead of a generic "empty or unreadable"
2. **MAL's rate limit** for `load.json` isn't documented. With the 60s cache it's 1-2 requests
   per minute per account; a `429` gets the same treatment as a network error (stale + log)

## What changed in the implementation

- **`is_rewatching` isn't always a number.** The live test (`TestLive_MALList`, `AAD_LIVE_MAL=1`)
  failed on its first run: 12 of Xinil's first 300 entries send `"is_rewatching": ""`. It's
  decoded as `any` and counts as rewatching only when it prints as `"1"`
- **`GetMALList` takes the statuses** (`GetMALList(user, statuses, priority)`) instead of being
  composed with a separate `FilterByListStatus`: the filter has to run before the hydration, so
  the function that hydrates is the one that filters. `FilterMALEntries` is still exported for
  the deletion vote, which filters raw entries
- **An empty fetch still falls back to the stale list.** `ErrMALEmpty` comes out of the fetch like
  any other failure, so a list that becomes private (or a format change) keeps serving the last
  good copy with `ErrFromCache`; `ErrMALEmpty` only reaches the callers when there's no copy
- **The standalone guard uses the hydrated list** (`fetchMALEntries`, the same as `/animes`)
  instead of `GetMALEntries`: the guard also filters by media status, which needs the `Media`, and
  the hydration is the one `/animes` already pays for in the same poll
- **UI text:** the Config group "Anilist" became "Lists", the AniList usernames field is now
  labeled "AniList usernames", and the AniList-only hints (download/delete statuses, the "already
  tracked" reason in Add anime, the onboarding button) were generalized
- **Open question 1 (private list) is still open.** Not measured; the `Warn` stays generic
- **A failed hydration keeps what came back** (`hydrateMALEntries`): one brand-new entry whose
  AniList fetch hits a 429 used to take the whole MAL account out of the pass — and an account
  missing from the pass has its downloaded episodes deleted by `identifyEpisodesNotInWatching`.
  The partial list goes out as `ErrFromCache`; only "nothing came back" is a failed account
- **New open question — an unmapped entry doesn't vote.** `GetMALEntries` drops the entries it
  couldn't map (AniList down and no cached mapping) and still returns no error, so in the deletion
  vote the MAL account counts as not tracking that anime and can't veto. It takes a brand-new MAL
  entry whose anime is already on disk through another account, during an AniList outage, in the
  same pass. Not worth a new sentinel until it's seen
