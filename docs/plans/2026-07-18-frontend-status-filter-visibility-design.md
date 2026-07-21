# Frontend Anime Listing — Status Filter Visibility — Design

Amends [2026-07-17-download-media-status-filter-design.md](2026-07-17-download-media-status-filter-design.md), which scoped `DownloadMediaStatuses` as download-only and explicitly left `endpoint_animes.go` untouched. Product decision changed: both `DownloadStatuses` (list status) and `DownloadMediaStatuses` (media status) should now also govern which animes appear in the frontend's anime listing (`GET /animes`).

## Current behavior (before this change)

`handleAnimes` → `mergeCurrentAniListAnimes` (`endpoint_animes.go`) already partially filters the listing by list status:

- `GetFrontendAnimeList(username, config.DownloadStatuses)` filters server-side via GraphQL `status_in` — only animes whose **list** status is in `DownloadStatuses` come back.
- Any existing `animeMap` entry with a known `AnimeID` (i.e., already has downloaded episodes, tracked via `episodes.json`) gets **deleted** if its `AnimeID` isn't in that filtered response's ID set. This means: if a user's list status for an anime changes away from `DownloadStatuses` (e.g., moves it to `COMPLETED` when `COMPLETED` isn't selected), the anime disappears from the frontend entirely — even with episodes on disk.
- `DownloadMediaStatuses` (media status) isn't consulted at all here — an anime in `HIATUS` or `CANCELLED` still shows and gets merged in normally.

## New behavior

1. **Media status now also filters the listing**, same visibility rule as list status: only animes whose `Media.Status` is in `DownloadMediaStatuses` are eligible to be freshly merged in.
2. **Already-downloaded animes are never removed from the listing** by either filter. Once an anime has a downloaded episode (`AnimeID != 0` in `episodes.json`), it stays visible regardless of its current list/media status.
3. **Already-downloaded animes whose current status falls outside the allowed sets still get their AniList-derived fields refreshed** (cover image, episodes released, episodes watched, blacklist) via a per-anime lookup, not silently left stale/blank.

## mediaStatusAllowed → anilist.MediaStatusAllowed

`mediaStatusAllowed` currently lives in `src/internal/daemon/helpers.go` (unexported), used only by `searchAnilist` (`verification.go`). It moves to `src/internal/anilist/anilist.go` as exported `MediaStatusAllowed(allowedStatuses []string, status MediaStatus) bool`, so both `daemon.searchAnilist` and `api.mergeCurrentAniListAnimes` share one implementation. `daemon/helpers_test.go`'s three unit tests move with it (as `anilist` package tests, or stay as call-site tests against `anilist.MediaStatusAllowed` — implementer's call, same assertions).

## endpoint_animes.go changes

**`mergeCurrentAniListAnimes`**:

- Right after the existing `customLists` overlay loop (which restores `ml.CustomLists` from the lightweight query when the complex query nulls it out), filter `resp.Data.Page.MediaList` down to entries where `anilist.MediaStatusAllowed(config.DownloadMediaStatuses, ml.Media.Status)` is true. Everything downstream (`validIDs`, `knownByID`, the merge loop) operates on this filtered slice — list-status filtering already happened server-side via `GetFrontendAnimeList`'s `status_in`, so after this the slice is filtered by both.
- **Remove** the block that deletes `animeMap` entries whose known `AnimeID` isn't in `validIDs`. Downloaded animes are never removed by this function anymore.
- The function needs to report which `AnimeID`s it saw (were present in the filtered slice), so the caller knows which already-downloaded animes were *not* covered and need a refresh. Simplest: change return type from nothing to `map[int]bool` (the covered ID set) or accept a `map[int]bool` to populate — either way, `handleAnimes` needs this set across all `config.AnilistUsernames` iterations (union across accounts).

**`handleAnimes`**:

- After the `for _, username := range config.AnilistUsernames { mergeCurrentAniListAnimes(...) }` loop, with the unioned covered-ID set in hand: collect every `animeMap` entry with `AnimeID != 0` **not** in that set — these are "orphans" (downloaded, but current status fell outside both filters, or the AniList fetch for that account failed).
- Refresh each orphan concurrently, bounded to 5 in flight (mirrors `maxConcurrentAnimes` in `daemon/verification.go`), via a new `refreshOrphanAnime(info *AnimeInfo, excludedLists []string)` that:
  - Calls `anilist.GetAnimeInfo(info.AnimeID)`.
  - On error: log a warning, leave `info` untouched (episodes.json-derived fields stand, AniList-derived fields stay at their zero value for this response — not fatal, request still succeeds).
  - On success: update `Name`, `TotalEpisodes`, `EpisodesReleased`, `EpisodesWatched`, `CoverImage`, `IsBlacklisted` on `info` in place, same computation as the existing merge loop (episodes-released-from-airing-schedule-or-finished, blacklist-from-customLists-membership). Each goroutine touches a distinct `*AnimeInfo` pointer — no shared mutable state, no lock needed.
- Since the enrichment computation (name/cover/episodesReleased/isBlacklisted from a `Media`+`CustomLists`+`Progress`+`AiringSchedule` tuple) is now needed for both the batch merge loop and the single-orphan refresh, factor it into a shared unexported function, e.g. `computeAnimeFields(mediaId int, status anilist.MediaStatus, episodes *int, title anilist.Title, cover anilist.CoverImage, schedule anilist.AiringSchedule, progress int, customLists anilist.CustomLists, excludedLists []string) (name string, totalEpisodes, episodesReleased int, coverImage string, isBlacklisted bool)`. Both call sites adapt their slightly different source types (`anilist.MediaList` vs `anilist.MediaListDetail`) to this signature.

## anilist.go changes

- `MediaListDetail` gains a top-level `CustomLists CustomLists `json:"customLists"`` field (mirroring how `MediaList.CustomLists` is top-level, not nested under `.Media`). Add `customLists` to the `GetAnimeInfo` GraphQL query's top-level `MediaList { ... }` selection (alongside existing `id status progress`). Existing callers (`debug.go`, `manual_download.go`) ignore the new field — harmless.
- Add exported `MediaStatusAllowed(allowedStatuses []string, status MediaStatus) bool` (moved from `daemon/helpers.go`, same body).

## Out of scope

- No change to how `DownloadStatuses`/`DownloadMediaStatuses` affect the *download* pipeline (`searchAnilist`) — that part is already correct from the prior design.
- No change to `dedupeAnimesByMedia` or multi-account episode-tracking semantics.
- No caching of orphan-refresh results across requests — every `GET /animes` re-fetches orphans fresh. Acceptable: orphans are expected to be rare (anime whose status just changed), not the common case.

## Testing

- `anilist` package: unit tests for `MediaStatusAllowed` (moved from `daemon`).
- `daemon` package: existing `searchAnilist` media-status tests updated to call `anilist.MediaStatusAllowed` (behavior unchanged).
- `api` package (`endpoint_animes_test.go`):
  - Anime with downloaded episodes whose list/media status is no longer allowed still appears in `GET /animes` response (doesn't disappear).
  - Such an orphan gets its `cover_image`/`episodes_released`/`is_blacklisted` refreshed via a mocked second AniList call (`GetAnimeInfo`).
  - An orphan refresh that errors doesn't fail the whole request — anime still appears, just without the refreshed fields.
  - An anime with no downloaded episodes and a disallowed media status is NOT merged in (closes the original gap).
  - Existing tests (blacklist, rate-limit overlay, multi-account) stay green.
- `go test ./...` green.
