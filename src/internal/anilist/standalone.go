package anilist

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// mediaByIDCache stores the per-media-id reads of standalone animes, and is shared by
// GetMediaByID and GetMediaByIDs — keyed BY ID, never by batch, otherwise a new standalone
// would invalidate the read of every other one.
//
// The path it protects is GET /api/v1/animes, which is polled every 30s per open tab and
// reads the whole standalone list each time. Since the batch (decisions.md #65 and #72)
// that's ONE query per 50 ids, not one per standalone; the cache still pays off because
// with it the poll only goes to the network when some id expires — the same reason
// frontendListCache exists (decisions.md #57).
var mediaByIDCache = newTTLCache[*MediaList]()

const mediaByIDTTL = 60 * time.Second

// MediaSearchResult is a row of the search bar on the add-anime screen. It is deliberately
// poorer than Media: the search brings 20 results and none of them need synonyms,
// relations or airingSchedule — whoever adds the anime fetches those fields with GetMediaByID.
type MediaSearchResult struct {
	Id         int         `json:"id"`
	Title      Title       `json:"title"`
	Format     MediaFormat `json:"format"`
	Status     MediaStatus `json:"status"`
	SeasonYear int         `json:"seasonYear"`
	Episodes   *int        `json:"episodes"`
	CoverImage CoverImage  `json:"coverImage"`
}

// searchCache stores the result by term. It started paying off when the screen stopped
// searching on every keystroke (decisions.md #50 (f)): with submit, the keys are whole
// terms that repeat — the same user redoing the search, the unreleased toggle going back
// and forth, another tab searching the same anime. The key is lowercase because AniList's
// search is case-insensitive, so "One Piece" and "one piece" return the same thing and
// there's no reason to spend two requests.
//
// The cache lives in SearchMedia, NOT in the endpoint response, because block_reason is
// recomputed by standaloneGuard on every response: cached together, an anime added now
// would keep showing with the "Add" button until the TTL expired.
var searchCache = newTTLCache[[]MediaSearchResult]()

const (
	searchTTL = 10 * time.Minute
	// ponytail: cap by total flush, not LRU. This is the only cache with a free-text key —
	// nothing stops someone from typing a thousand different terms, and an expired entry
	// never leaves the map. A thousand search results are a few MB; if that ever bothers,
	// an insertion queue that drops the oldest swaps the flush for real eviction.
	searchCacheMaxEntries = 1000
)

func searchCacheKey(term string, includeUnreleased bool) string {
	return strings.ToLower(term) + "|" + strconv.FormatBool(includeUnreleased)
}

// SearchMedia searches AniList animes by term, with a searchTTL cache.
//
// includeUnreleased=false (the screen default) hides NOT_YET_RELEASED. The filter is
// SERVER-SIDE on purpose: perPage is 20, and filtering after receiving would return searches
// with 4 useful results in an announcement season. Only NOT_YET_RELEASED is dropped —
// CANCELLED and HIATUS stay, because an anime interrupted mid-airing has downloadable episodes.
func SearchMedia(term string, includeUnreleased bool) ([]MediaSearchResult, error) {
	key := searchCacheKey(term, includeUnreleased)
	if cached, ok := searchCache.get(key); ok {
		return slices.Clone(cached), nil
	}

	// Concatenation, not a GraphQL variable with a null value: AniList ignores null arguments
	// today, but depending on that would make the filter die silently if the semantics changed.
	// Here, when includeUnreleased is true, the argument does not exist in the query.
	statusFilter := ", status_not: NOT_YET_RELEASED"
	if includeUnreleased {
		statusFilter = ""
	}

	query := `
		query SearchMedia($q: String) {
			Page(perPage: 20) {
				media(search: $q, type: ANIME, sort: SEARCH_MATCH` + statusFilter + `) {
					id
					title {
						english
						romaji
					}
					format
					status
					seasonYear
					episodes
					coverImage {
						large
						medium
					}
				}
			}
		}
	`

	type response struct {
		Data struct {
			Page struct {
				Media []MediaSearchResult `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"q": term}, PriorityDisposable)
	if err != nil {
		// Refused by the budget gate: AniList's catalog doesn't change in ten minutes, so
		// serving the expired result keeps the screen working and gives the bucket back to
		// the daemon pass, which is the only one that can't be postponed. See decisions.md #72.
		if errors.Is(err, ErrBudgetLow) {
			if stale, ok := searchCache.getStale(key); ok {
				return slices.Clone(stale), nil
			}
		}
		return nil, err
	}

	if searchCache.size() >= searchCacheMaxEntries {
		searchCache.clear()
	}
	searchCache.set(key, resp.Data.Page.Media, searchTTL)
	// Copy: the stored slice is shared by every request that hits this key, and the caller
	// must not reorder or cut the cache's memory.
	return slices.Clone(resp.Data.Page.Media), nil
}

// mediaByIDFields are the fields GetMediaByID and GetMediaByIDs ask for — the SAME as
// getMediaListEntry (including synonyms, relations and the id of each airingSchedule node):
// anime search and searchNyaaForSingleEpisode depend on synonyms and relations (split-season
// offset via PREQUEL), and the rest of the app keys episodes by the node id.
const mediaByIDFields = `
	id
	episodes
	format
	status
	title {
		english
		romaji
		native
	}
	synonyms
	relations {
		edges {
			node {
				title {
					english
					romaji
				}
				synonyms
				episodes
				format
			}
			relationType
		}
	}
	coverImage {
		large
		medium
	}
	airingSchedule {
		nodes {
			airingAt
			timeUntilAiring
			episode
			id
		}
	}
	nextAiringEpisode {
		episode
		airingAt
		timeUntilAiring
	}
`

// GetMediaByID reads an anime by media id WITHOUT going through any account list — the
// primitive that was missing for a standalone anime to exist for the app (GetAnimeInfo
// returns nil when no account tracks it).
//
// Returns a SYNTHETIC MediaList: only Media filled, Progress 0, Status "" and Id 0. The
// zeros are part of the contract — DedupeByMedia keeps the LOWEST progress, so a Progress
// invented here would beat the real entry and the daemon would lower already-watched episodes.
//
// The SYNTHETIC MediaList is the same contract as GetMediaByIDs, which is who serves the
// whole standalone list; this one stays for the lookup of a single id.
//
// (nil, nil) when AniList doesn't know the id.
func GetMediaByID(mediaID int, priority Priority) (*MediaList, error) {
	key := strconv.Itoa(mediaID)
	if cached, ok := mediaByIDCache.get(key); ok {
		return copyMediaList(cached), nil
	}

	query := `
		query GetMediaByID($id: Int) {
			Media(id: $id, type: ANIME) {` + mediaByIDFields + `}
		}
	`

	type response struct {
		Data struct {
			Media *Media `json:"Media"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"id": mediaID}, priority)
	if errors.Is(err, ErrNotFound) {
		mediaByIDCache.set(key, nil, mediaByIDTTL)
		markCacheDirty()
		return nil, nil
	}
	if err != nil {
		if stale, ok := staleMedia(key); ok {
			return stale, ErrFromCache
		}
		return nil, err
	}
	if resp.Data.Media == nil {
		logger.Logger.Debug().Int("media_id", mediaID).Msg("AniList does not know this media id")
		mediaByIDCache.set(key, nil, mediaByIDTTL)
		markCacheDirty()
		return nil, nil
	}

	ml := &MediaList{Media: *resp.Data.Media}
	mediaByIDCache.set(key, ml, mediaByIDTTL)
	markCacheDirty()
	return copyMediaList(ml), nil
}

// GetMediaByIDs reads several animes by media id in a single query. It replaces the
// GetMediaByID loop in the two places that walk the whole standalone list — the dominant
// AniList budget consumer (decisions.md #65), which spent 1 request per standalone on every
// daemon pass and every 30s frontend poll.
//
// Returns a map id -> MediaList, with three distinct outcomes per id:
//
//   - entry with a value: the anime came (from cache or network);
//   - entry with nil: AniList does NOT know this id. id_in silently omits the deleted
//     media, so "didn't come back" is the only way to discover that — and it matches the
//     (nil, nil) of GetMediaByID;
//   - id absent from the map: couldn't fetch (network, 429). Only this case comes with an error.
//
// The returned error does NOT invalidate the map: pages that passed before it are already there.
func GetMediaByIDs(ids []int, priority Priority) (map[int]*MediaList, error) {
	// The cache stays per id, not per batch: keyed by the batch, a new standalone would
	// invalidate the read of every other one.
	found := make(map[int]*MediaList, len(ids))
	seen := make(map[int]bool, len(ids))
	missing := make([]int, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if cached, ok := mediaByIDCache.get(strconv.Itoa(id)); ok {
			found[id] = copyMediaList(cached)
			continue
		}
		missing = append(missing, id)
	}

	query := `
		query GetMediaByIDs($ids: [Int]) {
			Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
				media(id_in: $ids, type: ANIME) {` + mediaByIDFields + `}
			}
		}
	`

	type response struct {
		Data struct {
			Page struct {
				Media []Media `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	for start := 0; start < len(missing); start += anilistMaxPerPage {
		chunk := missing[start:min(start+anilistMaxPerPage, len(missing))]

		resp, err := sendAnilistRequest[response](query, RequestVariables{"ids": chunk}, priority)
		if err != nil {
			// Whatever the local cache can serve, serve: without this the standalones
			// disappear from the screen's list while AniList is down, which is half of
			// "the list doesn't shrink". Ids with no stored entry stay absent from the
			// map, which is the contract documented above for "couldn't fetch".
			fromCache := false
			for _, id := range missing[start:] {
				if stale, ok := staleMedia(strconv.Itoa(id)); ok {
					found[id] = stale
					fromCache = true
				}
			}
			if fromCache {
				return found, ErrFromCache
			}
			return found, err
		}

		for i := range resp.Data.Page.Media {
			ml := &MediaList{Media: resp.Data.Page.Media[i]}
			mediaByIDCache.set(strconv.Itoa(ml.Media.Id), ml, mediaByIDTTL)
			found[ml.Media.Id] = copyMediaList(ml)
		}

		for _, id := range chunk {
			if _, ok := found[id]; !ok {
				logger.Logger.Debug().Int("media_id", id).Msg("AniList does not know this media id")
				mediaByIDCache.set(strconv.Itoa(id), nil, mediaByIDTTL)
				found[id] = nil
			}
		}
		markCacheDirty()
	}

	return found, nil
}

// copyMediaList returns a shallow copy of the entry stored in the cache. Same reason as
// mediaListResponse: the caller overwrites fields of the entry (CustomLists, Progress), and
// handing out the stored pointer would leave two concurrent requests writing the same memory.
func copyMediaList(ml *MediaList) *MediaList {
	if ml == nil {
		return nil
	}
	copied := *ml
	return &copied
}
