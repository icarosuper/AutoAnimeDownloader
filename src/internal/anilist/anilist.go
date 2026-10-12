package anilist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// ttlCache holds AniList responses for a short time. AniList today limits to 30 req/min per IP and
// the frontend polls /api/v1/animes every 30s per open tab, with one fetch per configured account:
// without a cache, two tabs and two accounts blow the budget on their own and the resulting 429
// also takes down the daemon's verification cycle (see decisions.md #11 and #57).
type ttlCache[T any] struct {
	mu     sync.Mutex
	data   map[string]T
	expiry map[string]time.Time
}

func newTTLCache[T any]() *ttlCache[T] {
	return &ttlCache[T]{data: make(map[string]T), expiry: make(map[string]time.Time)}
}

func (c *ttlCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.expiry[key]) {
		return c.data[key], true
	}
	var zero T
	return zero, false
}

// getStale returns the entry IGNORING the TTL, and it's the ONLY door to what came from the disk
// snapshot: restore leaves those entries with an expiry in the past on purpose, so get never sees
// them (see persist.go).
//
// Only the FAILURE path calls it. Two failures, with different contracts: the budget gate's refusal
// serves the expired value without an error, because the data is seconds old (decisions.md #72);
// AniList being down serves it with ErrFromCache, because the age becomes unknown and the caller
// has to decide. Whoever reads through get expects fresh data and must not land here.
func (c *ttlCache[T]) getStale(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.data[key]
	return value, ok
}

func (c *ttlCache[T]) set(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	c.expiry[key] = time.Now().Add(ttl)
}

// size is the number of stored keys, EXPIRED ONES INCLUDED — nothing here removes an entry, expiry
// only hides it from get. Only the search cache uses it, the only one with a free-text key and
// therefore no natural ceiling.
func (c *ttlCache[T]) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.data)
}

// snapshot copies ALL entries, expired ones included — whoever persists wants precisely the expired
// ones, which are the ones that serve as fallback after the restart. It doesn't carry the expiry:
// the TTL on the way back is decided in restore, see the snapshot comment in persist.go.
func (c *ttlCache[T]) snapshot() map[string]T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) == 0 {
		return nil
	}
	out := make(map[string]T, len(c.data))
	for k, v := range c.data {
		out[k] = v
	}
	return out
}

// restore fills the cache from a snapshot. A zero ttl leaves the entries visible ONLY to getStale:
// that's how the disk data becomes available as fallback without ever being served as if it were
// fresh.
func (c *ttlCache[T]) restore(data map[string]T, ttl time.Duration) {
	if len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expiry := time.Now().Add(ttl)
	for k, v := range data {
		c.data[k] = v
		c.expiry[k] = expiry
	}
}

func (c *ttlCache[T]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.data)
	clear(c.expiry)
}

// ponytail: no stampede protection — N simultaneous polls that miss the cache fire N fetches. The
// tabs' timers are staggered, so the TTL alone holds the volume; if that's ever not enough,
// serializing fetches per key solves it.
var (
	customListsCache  = newTTLCache[map[int]CustomLists]()
	frontendListCache = newTTLCache[[]MediaList]()
	// passListCache and mediaEntryCache exist ONLY as fallback: written with TTL zero, never read
	// through get, read only when the request fails. There's no budget gain here and no stale data
	// starts being served on the happy path — the single goal is keeping the pass and the detail
	// screen up with AniList down (see persist.go).
	//
	// They're two, not one, because the three queries ask for DIFFERENT field sets: the frontend's
	// brings coverImage and doesn't bring relations nor the entry status; the pass's brings relations
	// and status and doesn't bring coverImage; the media-id one (mediaByIDFields) brings everything.
	// Serving the detail screen from the frontend cache would leave the manual download without
	// relations.
	passListCache   = newTTLCache[[]MediaList]()
	mediaEntryCache = newTTLCache[[]MediaList]()
)

const (
	customListsTTL = 5 * time.Minute
	// An empty response can be either "the account has no custom lists" or a field AniList degraded.
	// The short TTL limits both: the poll stops hitting the API on every request and a degraded field
	// corrects itself in half a minute.
	customListsEmptyTTL = 30 * time.Second
	frontendListTTL     = 60 * time.Second
)

var httpDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

var aniListAPIURL = "https://graphql.anilist.co"

// anilistMaxPerPage is the maximum perPage AniList accepts in a Page. It's the batch size of every
// fetch by a list of ids (mediaId_in, id_in, idMal_in): asking for more than that in one query
// would only return 50 and make the remaining ids look deleted.
const anilistMaxPerPage = 50

// ErrNotFound signals AniList answered 404: the queried object doesn't exist.
var ErrNotFound = errors.New("anilist: not found")

func init() {
	if url := os.Getenv("ANILIST_API_URL"); url != "" {
		aniListAPIURL = url
	}
}

// MockAniListDo swaps the HTTP transport (MyAnimeList's included — it goes through the same httpDo)
// and clears the caches at both ends: a test that installs a mock has to see its responses, not the
// previous test's.
func MockAniListDo(fn func(*http.Request) (*http.Response, error)) (restore func()) {
	prev := httpDo
	clearCaches()
	if fn != nil {
		httpDo = fn
	}
	return func() { httpDo = prev; clearCaches() }
}

func clearCaches() {
	customListsCache.clear()
	frontendListCache.clear()
	passListCache.clear()
	mediaEntryCache.clear()
	mediaByIDCache.clear()
	seriesCache.clear()
	searchCache.clear()
	malListCache.clear()
	malIDMapCache.clear()
	// Health is package state too: without resetting it here, a test that simulates a 403 would leave
	// the next test seeing AniList as down. Same for the budget reading: a test that simulates an
	// empty bucket would lock the next test's gate.
	health.Store(&Health{State: HealthOK})
	budget.Store(nil)
	// Persistence is package state too, and the write is scheduled: a pending snapshot that fires
	// after the test ends would write into another test's temp directory.
	disablePersistence()
}

type AniListResponse struct {
	Data struct {
		Page struct {
			MediaList []MediaList `json:"mediaList"`
		} `json:"Page"`
	} `json:"data"`
}

type MediaListStatus string

const (
	MediaListStatusCompleted MediaListStatus = "COMPLETED"
	MediaListStatusDropped   MediaListStatus = "DROPPED"
	MediaListStatusPaused    MediaListStatus = "PAUSED"
	MediaListStatusPlanning  MediaListStatus = "PLANNING"
	MediaListStatusRepeating MediaListStatus = "REPEATING"
	MediaListStatusCurrent   MediaListStatus = "CURRENT"
)

type MediaList struct {
	Id          int             `json:"id"`
	Status      MediaListStatus `json:"status"`
	Progress    int             `json:"progress"`
	CustomLists CustomLists     `json:"customLists"`
	Media       Media           `json:"media"`
}

type MediaStatus string

const (
	MediaStatusFinished       MediaStatus = "FINISHED"
	MediaStatusReleasing      MediaStatus = "RELEASING"
	MediaStatusNotYetReleased MediaStatus = "NOT_YET_RELEASED"
	MediaStatusCancelled      MediaStatus = "CANCELLED"
	MediaStatusHiatus         MediaStatus = "HIATUS"
)

func MediaStatusAllowed(allowedStatuses []string, status MediaStatus) bool {
	for _, s := range allowedStatuses {
		if s == string(status) {
			return true
		}
	}
	return false
}

type CoverImage struct {
	Large  string `json:"large"`
	Medium string `json:"medium"`
}

type Media struct {
	Id             int            `json:"id"`
	Status         MediaStatus    `json:"status"`
	Format         MediaFormat    `json:"format"`
	Title          Title          `json:"title"`
	Episodes       *int           `json:"episodes"`
	Synonyms       []string       `json:"synonyms"`
	Relations      MediaRelations `json:"relations"`
	CoverImage     CoverImage     `json:"coverImage"`
	AiringSchedule AiringSchedule `json:"airingSchedule"`
	// NextAiringEpisode is the source of "which was the last episode to air" for the animes whose
	// airingSchedule AniList already clipped (see EpisodeList and decisions.md #52). nil when the
	// anime ended or has no date set.
	NextAiringEpisode *AiringNode `json:"nextAiringEpisode"`
}

type Title struct {
	English *string `json:"english"`
	Romaji  *string `json:"romaji"`
	// Native is the title in the original language (kanji, in practice). Only the dashboard search
	// uses it — the daemon builds the Nyaa queries with romaji/english, see search.go.
	Native *string `json:"native"`
}

type MediaRelationNode struct {
	Title    Title    `json:"title"`
	Synonyms []string `json:"synonyms"`
	Episodes *int     `json:"episodes"`
	// Format separates the prequel that counts episodes from the one that doesn't: a recap movie or an
	// OVA also arrive as PREQUEL with Episodes filled in, and adding that to the offset throws the
	// search to an episode number that doesn't exist. See ComputeEpisodeOffset.
	Format MediaFormat `json:"format"`
}

type MediaRelationEdge struct {
	Node         MediaRelationNode `json:"node"`
	RelationType string            `json:"relationType"`
}

type MediaRelations struct {
	Edges []MediaRelationEdge `json:"edges"`
}

type AiringSchedule struct {
	Nodes []AiringNode `json:"nodes"`
}

type AiringNode struct {
	ID              int   `json:"id"`
	Episode         int   `json:"episode"`
	TimeUntilAiring int   `json:"timeUntilAiring"`
	AiringAt        int64 `json:"airingAt"`
}

type GraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type MediaFormat string

const (
	MediaFormatTV      MediaFormat = "TV"
	MediaFormatTVShort MediaFormat = "TV_SHORT"
	MediaFormatMovie   MediaFormat = "MOVIE"
	MediaFormatOVA     MediaFormat = "OVA"
	MediaFormatONA     MediaFormat = "ONA"
	MediaFormatMusic   MediaFormat = "MUSIC"
	MediaFormatManga   MediaFormat = "MANGA"
	MediaFormatNovel   MediaFormat = "NOVEL"
	MediaFormatOneShot MediaFormat = "ONE_SHOT"
)

type CustomLists map[string]bool

// DedupeByMedia collapses the same anime appearing across multiple linked accounts into a
// single entry. MediaList.Id is the per-account *entry* id, so one anime tracked by two
// accounts arrives as two entries with different Ids and the same Media.Id — which is why
// Media.Id, and not MediaList.Id, is this app's anime identity (see decisions.md #43).
//
// The entry with the LOWEST progress wins: every download/keep/delete decision is keyed by
// airing-schedule episode ID (shared across accounts), so duplicates with divergent progress
// would fight — the account further ahead deleting episodes another account hasn't watched
// yet. An episode is only "watched" once all accounts have seen it.
//
// Status is NOT resolved here: the winning entry's Status belongs to one arbitrary account,
// so nothing may branch on it. Status is a per-account question answered by
// downloadableMediaIDs / deletableMediaIDs, which apply the OR/AND rules across accounts.
func DedupeByMedia(list []MediaList) []MediaList {
	byMedia := make(map[int]int, len(list)) // media id -> index in result
	result := make([]MediaList, 0, len(list))
	for _, anime := range list {
		if anime.Media.Id == 0 { // no media id (shouldn't happen) — keep as-is
			result = append(result, anime)
			continue
		}
		if idx, ok := byMedia[anime.Media.Id]; ok {
			if anime.Progress < result[idx].Progress {
				result[idx] = anime
			}
			continue
		}
		byMedia[anime.Media.Id] = len(result)
		result = append(result, anime)
	}
	return result
}

type RequestVariables map[string]any

func sendAnilistRequest[T any](query string, variables RequestVariables, priority Priority) (*T, error) {
	if !budgetAllows(priority) {
		logger.Logger.Debug().Msg("Skipping a disposable Anilist request: the rate limit budget is low")
		return nil, ErrBudgetLow
	}

	jsonData, err := json.Marshal(GraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", aniListAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logger.Logger.Debug().Str("url", aniListAPIURL).Msg("Sending Anilist request")

	resp, err := httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	recordBudget(resp.Header)

	if resp.StatusCode == http.StatusNotFound {
		// AniList answers 404 when the requested object doesn't exist (for example, a list entry the user
		// deleted). It's a valid answer, not a failure — whoever queries by id needs to tell it apart
		// from "AniList is down".
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		// The body carries the real reason (rate limit vs. API turned off vs. invalid query); without it
		// the log becomes a number with no diagnosis. The message also feeds the banner: a blocked-IP 403
		// explains the reason in writing, and only AniList knows which it is.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_, message := firstError(snippet)
		setHealth(classify(resp.StatusCode), message, retryAfter(resp.Header.Get("Retry-After")))
		logger.Logger.Warn().Int("status_code", resp.StatusCode).Str("body", string(snippet)).Msg("Anilist returned non-200 status")
		// The extracted message, not the raw body: the error goes up to the user's screen, and a JSON
		// dump with locations and column says nothing to anyone. The whole body stays in the log above,
		// which is where it's useful.
		if message == "" {
			message = strings.TrimSpace(string(snippet))
		}
		return nil, fmt.Errorf("anilist returned %d: %s", resp.StatusCode, message)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	// A 200 can carry an error in the envelope — AniList's docs are explicit about it. Without this
	// read the case becomes an empty response with no diagnosis. It's not the same problem as the null
	// customLists of decisions.md #11: that one comes back 200 WITHOUT an errors field. See #65.
	if status, message := firstError(body); status != 0 {
		setHealth(classify(status), message, time.Time{})
		logger.Logger.Warn().Int("graphql_status", status).Str("message", message).Msg("Anilist returned an error inside a 200 response")
		return nil, fmt.Errorf("API returned error %d: %s", status, message)
	}
	clearHealth()

	logger.Logger.Debug().Int("status_code", resp.StatusCode).Int("body_size", len(body)).Msg("Anilist response received")

	var response T
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}

// GetCustomListsMap fetches a lightweight map of MediaList ID → CustomLists via a minimal query.
// Results are cached for 5 minutes so repeated calls (e.g. from the API endpoint) don't hit
// Anilist's rate limit. Only a response with at least one non-null CustomLists entry is cached.
func GetCustomListsMap(userName string, statuses []string, priority Priority) map[int]CustomLists {
	key := userName + "\x00" + strings.Join(statuses, "\x00")

	if result, ok := customListsCache.get(key); ok {
		return result
	}

	type miniEntry struct {
		Id          int         `json:"id"`
		CustomLists CustomLists `json:"customLists"`
	}
	type miniResponse struct {
		Data struct {
			Page struct {
				MediaList []miniEntry `json:"mediaList"`
			} `json:"Page"`
		} `json:"data"`
	}

	query := `query($u:String,$t:MediaType,$s:[MediaListStatus]){Page{mediaList(userName:$u,type:$t,status_in:$s){id customLists}}}`
	resp, err := sendAnilistRequest[miniResponse](query, RequestVariables{
		"u": userName,
		"t": "ANIME",
		"s": statuses,
	}, priority)
	if err != nil {
		// Refused by the budget gate: serve the expired read, the same as its pair in
		// GetFrontendAnimeList (decisions.md #72). Without this the gate degrades BOTH sides
		// asymmetrically — the list comes from the old cache and answers successfully, customLists comes
		// back nil, and the GET /animes merge concludes "no anime is blacklisted". The screen shows a
		// blacklisted anime as normal and the standalone guard (standalone_guard.go) lets the user add
		// what it should refuse. Any failure, not only the gate's refusal: returning nil here makes the
		// exclusion disappear, and an anime in an excluded list looks downloadable again. Expired is
		// better than absent — the exclusion changes when the user edits AniList, not every minute.
		if stale, ok := customListsCache.getStale(key); ok {
			return stale
		}
		logger.Logger.Warn().Err(err).Str("username", userName).Msg("Failed to fetch customLists map")
		return nil
	}

	m := make(map[int]CustomLists, len(resp.Data.Page.MediaList))
	hasData := false
	for _, ml := range resp.Data.Page.MediaList {
		m[ml.Id] = ml.CustomLists
		if len(ml.CustomLists) > 0 {
			hasData = true
		}
	}

	if hasData {
		customListsCache.set(key, m, customListsTTL)
	} else {
		customListsCache.set(key, m, customListsEmptyTTL)
	}
	markCacheDirty()

	return m
}

func GetAllCurrentAnime(userName string, statuses []string) (*AniListResponse, error) {
	query := `
		query GetAllCurrentAnime($userName: String, $type: MediaType, $statuses: [MediaListStatus]) {
			Page {
				mediaList(userName: $userName, type: $type, status_in: $statuses) {
					id
					status
					progress
					customLists
					media {
						id
						format
						status
						episodes
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
						airingSchedule {
							nodes {
								id
								episode
								timeUntilAiring
							}
						}
						nextAiringEpisode {
							episode
							airingAt
							timeUntilAiring
						}
					}
				}
			}
		}
	`

	if len(statuses) == 0 {
		return &AniListResponse{}, nil
	}

	variables := RequestVariables{
		"userName": userName,
		"type":     "ANIME",
		"statuses": statuses,
	}

	key := userName + "\x00" + strings.Join(statuses, "\x00")
	resp, err := sendAnilistRequest[AniListResponse](query, variables, PriorityCritical)
	if err != nil {
		if stale, ok := staleList(passListCache, key); ok {
			return mediaListResponse(stale), ErrFromCache
		}
		return nil, err
	}

	storeList(passListCache, key, resp.Data.Page.MediaList, 0)
	return resp, nil
}

// GetFrontendAnimeList feeds /api/v1/animes, which the frontend polls every 30s per tab. The result
// is cached for frontendListTTL: without it each open tab multiplied the pressure on AniList's
// limit up to the 429, which in turn made the daemon's download cycle fail too. The list only
// changes when the user edits AniList, so a minute of delay on the screen is cheap next to taking
// the daemon down.
func GetFrontendAnimeList(userName string, statuses []string) (*AniListResponse, error) {
	query := `
		query GetFrontendAnimeList($userName: String, $type: MediaType, $statuses: [MediaListStatus]) {
			Page {
				mediaList(userName: $userName, type: $type, status_in: $statuses) {
					id
					progress
					customLists
					media {
						id
						title {
							english
							romaji
							native
						}
						synonyms
						episodes
						status
						coverImage {
							large
							medium
						}
						airingSchedule {
							nodes {
								episode
								timeUntilAiring
							}
						}
						nextAiringEpisode {
							episode
							airingAt
							timeUntilAiring
						}
					}
				}
			}
		}
	`

	if len(statuses) == 0 {
		return &AniListResponse{}, nil
	}

	key := userName + "\x00" + strings.Join(statuses, "\x00")
	if list, ok := frontendListCache.get(key); ok {
		return mediaListResponse(list), nil
	}

	variables := RequestVariables{
		"userName": userName,
		"type":     "ANIME",
		"statuses": statuses,
	}

	resp, err := sendAnilistRequest[AniListResponse](query, variables, PriorityDisposable)
	if err != nil {
		if stale, ok := staleList(frontendListCache, key); ok {
			// Refused by the budget gate: this list changes when the user edits AniList, not every 30s.
			// Serving the expired read keeps the screen working and hands the bucket back to the daemon's
			// pass, the only one that can't be postponed. See decisions.md #72. No error on purpose: the
			// refusal was OURS, the data is seconds old and the next poll comes back fresh — the screen
			// gains nothing from knowing.
			if errors.Is(err, ErrBudgetLow) {
				return mediaListResponse(stale), nil
			}
			// Any other failure is AniList being down, and the data's age becomes unknown (it may come from
			// the disk snapshot, days old). Here the caller HAS to know: /animes uses it to not fire the
			// orphan refresh against an API that doesn't answer.
			return mediaListResponse(stale), ErrFromCache
		}
		return nil, err
	}

	storeList(frontendListCache, key, resp.Data.Page.MediaList, frontendListTTL)
	return mediaListResponse(resp.Data.Page.MediaList), nil
}

// mediaListResponse wraps a list in an AniListResponse. It was born for GetFrontendAnimeList and
// today serves the three list fetches plus the media-id fallback — that's why the name doesn't say
// frontend.
//
// The copy is the point: callers overwrite CustomLists in the entries, and returning the slice
// stored in the cache would leave two concurrent requests writing to the same memory.
func mediaListResponse(list []MediaList) *AniListResponse {
	resp := &AniListResponse{}
	resp.Data.Page.MediaList = append([]MediaList(nil), list...)
	return resp
}

// GetAnimeInfoByIDs is the batch twin of GetAnimeInfo: ONE query per account per 50 media ids,
// instead of one query per anime per account. It is what makes the orphan refresh of
// GET /animes affordable — that path used to fire one request per downloaded anime the filtered
// list fetch didn't cover, on EVERY frontend poll, which is the burst decisions.md #65 measured
// as the dominant consumer of the AniList budget.
//
// Same collapse rule as GetAnimeInfo (DedupeByMedia: media fields from whichever account tracks
// the anime, LOWEST progress among them), applied across accounts AND chunks.
//
// A media id no account tracks is simply ABSENT from the map — the normal state of an anime
// removed from the lists whose episodes are still on disk, not an error. The error is returned
// when at least one account's request failed, and it does NOT invalidate the map: whatever came
// back is already in it. The caller decides what a partial answer is worth; it is the same
// distinction GetAnimeInfo draws between "not tracked" and "AniList is down", just at batch
// granularity.
//
// No cache: this feeds a path already fronted by frontendListCache's poll rhythm, and one query
// per poll per account is the same order as the list fetch beside it.
//
// malUsers are looked up in their cached MAL lists (GetMALEntries) and folded into the same
// collapse, so a MAL account's lower progress wins here exactly as it does in the pass.
func GetAnimeInfoByIDs(mediaIds []int, usernames, malUsers []string) (map[int]*MediaList, error) {
	if len(mediaIds) == 0 || len(usernames)+len(malUsers) == 0 {
		return map[int]*MediaList{}, nil
	}

	// mediaId_in returns at most one entry per id, so a batch of anilistMaxPerPage ids never goes past
	// one page — there's no pagination to follow here.
	query := `
		query GetAnimeInfoByIDs($userName: String, $mediaIds: [Int]) {
			Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
				mediaList(userName: $userName, mediaId_in: $mediaIds, type: ANIME) {
					id
					status
					progress
					customLists
					media {` + mediaByIDFields + `}
				}
			}
		}
	`

	var entries []MediaList
	var lastErr error
	for _, username := range usernames {
		for start := 0; start < len(mediaIds); start += anilistMaxPerPage {
			chunk := mediaIds[start:min(start+anilistMaxPerPage, len(mediaIds))]

			resp, err := sendAnilistRequest[AniListResponse](query, RequestVariables{
				"userName": username,
				"mediaIds": chunk,
			}, PriorityDisposable)
			if err != nil {
				logger.Logger.Warn().Err(err).Str("username", username).Int("media_ids", len(chunk)).
					Msg("Failed to fetch a batch of anime info for account")
				lastErr = err
				continue
			}
			entries = append(entries, resp.Data.Page.MediaList...)
		}
	}

	malList, err := malEntriesFor(mediaIds, malUsers, PriorityDisposable)
	if err != nil && !errors.Is(err, ErrFromCache) {
		logger.Logger.Warn().Err(err).Int("media_ids", len(mediaIds)).Msg("Failed to fetch anime info from a MAL account")
		lastErr = err
	}
	entries = append(entries, malList...)

	deduped := DedupeByMedia(entries)
	byMedia := make(map[int]*MediaList, len(deduped))
	for i := range deduped {
		byMedia[deduped[i].Media.Id] = &deduped[i]
	}

	if lastErr != nil {
		return byMedia, fmt.Errorf("failed to fetch anime info for %d media ids: %w", len(mediaIds), lastErr)
	}
	return byMedia, nil
}

// GetAnimeInfo returns one anime's data by MEDIA id, collapsed across every configured account:
// the media fields come from whichever account tracks it and Progress is the LOWEST among them
// (same rule as DedupeByMedia — an episode is only "watched" once every account has seen it).
//
// Returns (nil, nil) when no account has this media in its list: that is a normal state (the
// anime was removed from the lists but its episodes are still on disk), not an error. An error
// is only returned when every account's request failed, so the caller can tell "not tracked"
// from "AniList is down".
//
// malUsers count as accounts too: their entry is found in the cached MAL list and joins the
// same collapse, so the detail screen and the pass agree on the lowest progress.
func GetAnimeInfo(mediaId int, usernames, malUsers []string, priority Priority) (*MediaList, error) {
	var entries []MediaList
	var lastErr error
	fromCache := false
	for _, username := range usernames {
		resp, err := getMediaListEntry(username, mediaId, priority)
		switch {
		case errors.Is(err, ErrFromCache):
			// The account answered from the local cache. The entry counts — it's what keeps the detail
			// screen and the manual download up — but the fact travels with the return, so no caller
			// concludes on its own that it talked to AniList.
			fromCache = true
		case err != nil:
			logger.Logger.Warn().Err(err).Str("username", username).Int("media_id", mediaId).
				Msg("Failed to fetch anime info for account")
			lastErr = err
			continue
		}
		entries = append(entries, resp.Data.Page.MediaList...)
	}

	malList, err := malEntriesFor([]int{mediaId}, malUsers, priority)
	switch {
	case errors.Is(err, ErrFromCache):
		fromCache = true
	case err != nil:
		logger.Logger.Warn().Err(err).Int("media_id", mediaId).Msg("Failed to fetch anime info from a MAL account")
		lastErr = err
	}
	entries = append(entries, malList...)

	if len(entries) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to fetch anime info for media %d: %w", mediaId, lastErr)
		}
		return nil, nil
	}

	deduped := DedupeByMedia(entries)
	if fromCache {
		return &deduped[0], ErrFromCache
	}
	return &deduped[0], nil
}

// GetMediaIDForEntry resolves a legacy MediaList *entry* id to its media id, and is the only
// place left that keys anything by entry id: the one-shot AnimeID migration (decisions.md #43)
// needs it to rewrite records written before Media.Id became the identity.
// Returns 0 when the entry no longer exists — the anime was removed from that account's list.
func GetMediaIDForEntry(mediaListId int) (int, error) {
	query := `
		query GetMediaIDForEntry($mediaListId: Int) {
			MediaList(id: $mediaListId) {
				media {
					id
				}
			}
		}
	`

	type response struct {
		Data struct {
			MediaList *struct {
				Media struct {
					Id int `json:"id"`
				} `json:"media"`
			} `json:"MediaList"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"mediaListId": mediaListId}, PriorityCritical)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if resp.Data.MediaList == nil {
		return 0, nil
	}
	return resp.Data.MediaList.Media.Id, nil
}

// GetMediaListStatus returns the account's list status for one media. The bool is false when the
// account does not track this media at all — a distinction the delete rule depends on: an account
// that never had the anime cannot veto its deletion, but one holding it in a non-delete status can.
func GetMediaListStatus(username string, mediaId int) (MediaListStatus, bool, error) {
	query := `
		query GetMediaListStatus($userName: String, $mediaId: Int) {
			Page {
				mediaList(userName: $userName, mediaId: $mediaId) {
					status
				}
			}
		}
	`

	resp, err := sendAnilistRequest[AniListResponse](query, RequestVariables{
		"userName": username,
		"mediaId":  mediaId,
	}, PriorityCritical)
	if err != nil {
		return "", false, err
	}
	if len(resp.Data.Page.MediaList) == 0 {
		return "", false, nil
	}
	return resp.Data.Page.MediaList[0].Status, true, nil
}

func getMediaListEntry(userName string, mediaId int, priority Priority) (*AniListResponse, error) {
	query := `
		query GetAnimeEpisodes($userName: String, $mediaId: Int) {
			Page {
				mediaList(userName: $userName, mediaId: $mediaId) {
					id
					status
					progress
					customLists
					media {` + mediaByIDFields + `}
				}
			}
		}
	`

	key := userName + "\x00" + strconv.Itoa(mediaId)
	resp, err := sendAnilistRequest[AniListResponse](query, RequestVariables{
		"userName": userName,
		"mediaId":  mediaId,
	}, priority)
	if err != nil {
		if stale, ok := staleList(mediaEntryCache, key); ok {
			return mediaListResponse(stale), ErrFromCache
		}
		return nil, err
	}

	storeList(mediaEntryCache, key, resp.Data.Page.MediaList, 0)
	return resp, nil
}
