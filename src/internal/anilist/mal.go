package anilist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// MyAnimeList is a LIST source, never a metadata source: it only answers "which anime, in which
// status, with what progress". Identity, episodes, airing and covers keep coming from AniList,
// reached through the idMal mapping below — see decisions.md #95.
//
// This lives in package anilist on purpose: it needs ttlCache, the disk snapshot,
// sendAnilistRequest and GetMediaByIDs, all internal to this package.

// malListURL is the endpoint MAL's own list page loads from. Unofficial: no key, public lists
// only, 300 entries per page. status=7 is "all statuses".
var malListURL = "https://myanimelist.net/animelist/%s/load.json?status=7&offset=%d"

const (
	malPageSize = 300
	// Same TTL as frontendListCache, for the same reason (decisions.md #46): /api/v1/animes is
	// polled every 30s per open tab.
	malListTTL = 60 * time.Second
	// A MAL -> AniList id mapping practically never changes.
	malIDMapTTL = 24 * time.Hour
)

var (
	// malListCache holds the raw entries per MAL username: MAL id, status, progress. No Media —
	// hydrating the whole list on every poll would burn the AniList budget (see GetMALList).
	malListCache = newTTLCache[[]malRawEntry]()
	// malIDMapCache maps MAL id -> AniList media id. 0 = AniList has no counterpart, cached too
	// so an unmappable entry doesn't cost a request on every pass.
	malIDMapCache = newTTLCache[int]()
)

// ErrMALEmpty means a MAL list came back with zero usable entries. Callers must treat it as a
// FAILED fetch, never as "this account tracks nothing": a private list, a format change on MAL's
// side (every field decodes to zero) and a total mapping failure all look like this, and as an
// empty answer the account would silently lose its veto in the deletion vote.
var ErrMALEmpty = errors.New("myanimelist: list is empty, private or unreadable")

type malRawEntry struct {
	MalID    int             `json:"mal_id"`
	Title    string          `json:"title"`
	Status   MediaListStatus `json:"status"`
	Progress int             `json:"progress"`
}

// MALEntry is one MAL list entry mapped to AniList. No Media: it costs nothing beyond the MAL
// fetch and the 24h mapping, which is all the status questions (deletion vote, guard) need.
type MALEntry struct {
	MediaID  int // AniList Media.Id
	Status   MediaListStatus
	Progress int
}

// malStatus maps load.json's numeric status. is_rewatching wins: MAL models a rewatch as a flag
// on top of the status, AniList as a status of its own.
func malStatus(status int, rewatching bool) (MediaListStatus, bool) {
	if rewatching {
		return MediaListStatusRepeating, true
	}
	switch status {
	case 1:
		return MediaListStatusCurrent, true
	case 2:
		return MediaListStatusCompleted, true
	case 3:
		return MediaListStatusPaused, true
	case 4:
		return MediaListStatusDropped, true
	case 6:
		return MediaListStatusPlanning, true
	}
	return "", false
}

// fetchMALList reads the whole list, page by page, until a page shorter than malPageSize.
func fetchMALList(user string) ([]malRawEntry, error) {
	type item struct {
		AnimeID            int `json:"anime_id"`
		AnimeTitle         any `json:"anime_title"` // MAL sometimes sends a number for numeric titles
		Status             int `json:"status"`
		NumWatchedEpisodes int `json:"num_watched_episodes"`
		// Usually 0/1, but measured as "" on some entries (12 of 300 on 2026-10-12).
		IsRewatching any `json:"is_rewatching"`
	}

	var entries []malRawEntry
	for offset := 0; ; offset += malPageSize {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(malListURL, url.PathEscape(user), offset), nil)
		if err != nil {
			return nil, fmt.Errorf("error creating MAL request: %w", err)
		}
		resp, err := httpDo(req)
		if err != nil {
			return nil, fmt.Errorf("error requesting MAL list: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("error reading MAL list: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			// 400 is what MAL answers for a user that doesn't exist.
			return nil, fmt.Errorf("myanimelist returned %d for user %q", resp.StatusCode, user)
		}

		var page []item
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("error decoding MAL list: %w", err)
		}
		for _, it := range page {
			status, ok := malStatus(it.Status, fmt.Sprint(it.IsRewatching) == "1")
			if !ok || it.AnimeID == 0 {
				logger.Logger.Debug().Str("mal_user", user).Int("mal_id", it.AnimeID).Int("status", it.Status).
					Msg("Skipping MAL entry with an unknown status")
				continue
			}
			entries = append(entries, malRawEntry{
				MalID:    it.AnimeID,
				Title:    fmt.Sprint(it.AnimeTitle),
				Status:   status,
				Progress: it.NumWatchedEpisodes,
			})
		}
		if len(page) < malPageSize {
			break
		}
	}

	if len(entries) == 0 {
		return nil, ErrMALEmpty
	}
	return entries, nil
}

// malRawList serves the raw list from the cache, falling back to the stale copy (memory or disk
// snapshot) with ErrFromCache when MAL fails — the same contract as the AniList lists.
func malRawList(user string) ([]malRawEntry, error) {
	if cached, ok := malListCache.get(user); ok {
		return cached, nil
	}
	entries, err := fetchMALList(user)
	if err != nil {
		logger.Logger.Warn().Err(err).Str("mal_user", user).Msg("Failed to fetch MyAnimeList list")
		if stale, ok := malListCache.getStale(user); ok {
			return stale, ErrFromCache
		}
		return nil, err
	}
	malListCache.set(user, entries, malListTTL)
	markCacheDirty()
	return entries, nil
}

// mapMALIDs resolves MAL ids to AniList ids through idMal_in, 50 per query. The result holds
// only the ids it could answer for; 0 = AniList has no counterpart. On failure the stale mapping
// is used where there is one, and the error comes back as ErrFromCache if anything was served
// from it.
func mapMALIDs(malIDs []int, priority Priority) (map[int]int, error) {
	mapped := make(map[int]int, len(malIDs))
	var missing []int
	for _, id := range malIDs {
		if mediaID, ok := malIDMapCache.get(strconv.Itoa(id)); ok {
			mapped[id] = mediaID
			continue
		}
		missing = append(missing, id)
	}

	query := `
		query MapMALIDs($ids: [Int]) {
			Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
				media(idMal_in: $ids, type: ANIME) { id idMal }
			}
		}
	`
	type response struct {
		Data struct {
			Page struct {
				Media []struct {
					Id    int `json:"id"`
					IdMal int `json:"idMal"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	for start := 0; start < len(missing); start += anilistMaxPerPage {
		chunk := missing[start:min(start+anilistMaxPerPage, len(missing))]
		resp, err := sendAnilistRequest[response](query, RequestVariables{"ids": chunk}, priority)
		if err != nil {
			fromCache := false
			for _, id := range missing[start:] {
				if mediaID, ok := malIDMapCache.getStale(strconv.Itoa(id)); ok {
					mapped[id] = mediaID
					fromCache = true
				}
			}
			if fromCache {
				return mapped, ErrFromCache
			}
			return mapped, err
		}
		for _, m := range resp.Data.Page.Media {
			mapped[m.IdMal] = m.Id
			malIDMapCache.set(strconv.Itoa(m.IdMal), m.Id, malIDMapTTL)
		}
		// idMal_in silently omits the ids AniList doesn't know, the same as id_in.
		for _, id := range chunk {
			if _, ok := mapped[id]; !ok {
				mapped[id] = 0
				malIDMapCache.set(strconv.Itoa(id), 0, malIDMapTTL)
			}
		}
		markCacheDirty()
	}
	return mapped, nil
}

// GetMALEntries returns the user's whole MAL list (all statuses), mapped to AniList ids. Entries
// with no AniList counterpart are dropped with a Warn. ErrFromCache when anything came from a
// stale copy; ErrMALEmpty when nothing usable is left (see ErrMALEmpty — it is a failure).
func GetMALEntries(user string, priority Priority) ([]MALEntry, error) {
	raw, err := malRawList(user)
	fromCache := errors.Is(err, ErrFromCache)
	if err != nil && !fromCache {
		return nil, err
	}

	ids := make([]int, len(raw))
	for i, e := range raw {
		ids[i] = e.MalID
	}
	mapped, mapErr := mapMALIDs(ids, priority)
	switch {
	case errors.Is(mapErr, ErrFromCache):
		fromCache = true
	case mapErr != nil:
		logger.Logger.Warn().Err(mapErr).Str("mal_user", user).Msg("Failed to map some MAL ids to AniList")
	}

	entries := make([]MALEntry, 0, len(raw))
	for _, e := range raw {
		mediaID, ok := mapped[e.MalID]
		if !ok {
			continue // couldn't be asked: AniList failed and there's no stale mapping
		}
		if mediaID == 0 {
			logger.Logger.Warn().Str("mal_user", user).Int("mal_id", e.MalID).Str("title", e.Title).
				Msg("MAL entry has no AniList counterpart, skipping")
			continue
		}
		entries = append(entries, MALEntry{MediaID: mediaID, Status: e.Status, Progress: e.Progress})
	}

	if len(entries) == 0 {
		if mapErr != nil && !errors.Is(mapErr, ErrFromCache) {
			return nil, mapErr
		}
		return nil, ErrMALEmpty
	}
	if fromCache {
		return entries, ErrFromCache
	}
	return entries, nil
}

// FilterMALEntries keeps the entries whose status is in statuses.
func FilterMALEntries(entries []MALEntry, statuses []string) []MALEntry {
	var out []MALEntry
	for _, e := range entries {
		for _, s := range statuses {
			if string(e.Status) == s {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// GetMALList is GetMALEntries filtered by list status, then hydrated with GetMediaByIDs into
// MediaLists carrying the MAL status and progress. The filter comes BEFORE the hydration on
// purpose: mediaByIDCache lives 60s and the frontend polls every 30s, so hydrating the whole list
// (mostly COMPLETED) would cost several AniList queries per minute per account — the burn
// decisions.md #46 and #85 closed. Filtered, it's the handful of anime actually being watched.
//
// Id stays 0 (MAL has no AniList list-entry id) and CustomLists empty, so excluded_lists doesn't
// apply — the same shape as a standalone anime's synthetic MediaList.
func GetMALList(user string, statuses []string, priority Priority) ([]MediaList, error) {
	entries, err := GetMALEntries(user, priority)
	fromCache := errors.Is(err, ErrFromCache)
	if err != nil && !fromCache {
		return nil, err
	}
	return hydrateMALEntries(FilterMALEntries(entries, statuses), priority, fromCache)
}

func hydrateMALEntries(entries []MALEntry, priority Priority, fromCache bool) ([]MediaList, error) {
	ids := make([]int, len(entries))
	for i, e := range entries {
		ids[i] = e.MediaID
	}
	// On error the map still holds what came back (GetMediaByIDs' contract). Keep it, the same as
	// appendStandaloneAnimes does: discarding it would make ONE brand-new entry whose fetch hit a
	// 429 take the whole account down — and an account missing from the pass has its downloaded
	// episodes deleted by identifyEpisodesNotInWatching. The partial answer goes out as
	// ErrFromCache, which the pass accepts and /animes reads as "skip the orphan refresh".
	medias, err := GetMediaByIDs(ids, priority)
	switch {
	case errors.Is(err, ErrFromCache):
		fromCache = true
	case err != nil:
		logger.Logger.Warn().Err(err).Int("media_ids", len(ids)).
			Msg("Failed to fetch some MAL animes from AniList; skipping the missing ones")
	}

	list := make([]MediaList, 0, len(entries))
	for _, e := range entries {
		ml := medias[e.MediaID]
		if ml == nil {
			continue
		}
		ml.Status = e.Status
		ml.Progress = e.Progress
		list = append(list, *ml)
	}
	if err != nil && !errors.Is(err, ErrFromCache) {
		if len(list) == 0 {
			return nil, err // nothing came back: a failed account, not an empty one
		}
		fromCache = true
	}
	if fromCache {
		return list, ErrFromCache
	}
	return list, nil
}

// malEntriesFor returns, across the MAL accounts, the entries for the given media ids, hydrated.
// It's the MAL half of GetAnimeInfo / GetAnimeInfoByIDs. A failed account is reported through
// the error and skipped; ErrFromCache when anything came from a stale copy.
func malEntriesFor(mediaIDs []int, malUsers []string, priority Priority) ([]MediaList, error) {
	wanted := make(map[int]bool, len(mediaIDs))
	for _, id := range mediaIDs {
		wanted[id] = true
	}

	var list []MediaList
	var lastErr error
	fromCache := false
	for _, user := range malUsers {
		entries, err := GetMALEntries(user, priority)
		switch {
		case errors.Is(err, ErrFromCache):
			fromCache = true
		case err != nil:
			lastErr = err
			continue
		}
		var matched []MALEntry
		for _, e := range entries {
			if wanted[e.MediaID] {
				matched = append(matched, e)
			}
		}
		if len(matched) == 0 {
			continue
		}
		hydrated, err := hydrateMALEntries(matched, priority, false)
		switch {
		case errors.Is(err, ErrFromCache):
			fromCache = true
		case err != nil:
			lastErr = err
			continue
		}
		list = append(list, hydrated...)
	}

	if lastErr != nil {
		return list, lastErr
	}
	if fromCache {
		return list, ErrFromCache
	}
	return list, nil
}
