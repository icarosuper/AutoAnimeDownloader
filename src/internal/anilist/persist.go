package anilist

import (
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// The disk snapshot exists for ONE thing: keeping the screens and the pass working when AniList
// goes down, restart included. It is NOT a read front — nothing here is served while the API
// answers.
//
// How that's guaranteed: the MediaList collections (and the MAL lists) come back from disk with TTL
// ZERO, i.e. visible only to getStale, which only runs when the request failed. The happy path
// keeps going to the network exactly as before, and no screen starts showing older data than it
// did. The exceptions are seriesCache and the MAL id mapping, which come back with their full TTL
// on purpose: what goes into seriesCache is a link of a FINISHED anime with its episode count,
// immutable by construction (see recordLink), and a MAL -> AniList id mapping practically never
// changes — there's nothing to expire.
type snapshot struct {
	SavedAt time.Time `json:"saved_at"`
	// Keyed exactly as in memory (user + separator + statuses, or user + separator + media id). The
	// separator is a null byte, which Go's encoder writes as \u0000 and reads back the same - the key
	// survives the round-trip untouched.
	FrontendList map[string][]MediaList `json:"frontend_list,omitempty"`
	PassList     map[string][]MediaList `json:"pass_list,omitempty"`
	MediaEntries map[string][]MediaList `json:"media_entries,omitempty"`
	// CustomLists goes in because it's what carries the exclusion. Without it in the fallback, an
	// anime the user put in an excluded list looks non-excluded again while AniList is down — and the
	// pass downloads what the user said to ignore.
	CustomLists map[string]map[int]CustomLists `json:"custom_lists,omitempty"`
	// MediaByID covers the standalone animes, which show up in no account's list: without it the
	// screen's list keeps shrinking — the standalone part of it — with AniList down.
	MediaByID map[string]*MediaList `json:"media_by_id,omitempty"`
	Series    map[string]seriesLink `json:"series,omitempty"`
	// The MAL raw lists come back with TTL zero like the other lists; the MAL -> AniList id
	// mapping comes back with its full TTL, like Series: it practically never changes.
	MALLists map[string][]malRawEntry `json:"mal_lists,omitempty"`
	MALIDMap map[string]int           `json:"mal_id_map,omitempty"`
}

// persistDebounce coalesces the writes. Without it every opening of the detail screen would rewrite
// the whole snapshot, and a pass would rewrite it once per account plus once per series walk.
const persistDebounce = 5 * time.Second

var (
	persistMu    sync.Mutex
	persistSave  func([]byte) error
	persistTimer *time.Timer
	// snapshotAt is the SavedAt of what's on disk, in unix. Zero = never written on this install. The
	// frontend uses it to say how old the cache it's seeing is.
	snapshotAt atomic.Int64
)

// EnablePersistence turns the snapshot on and loads whatever is on disk. Called once at daemon
// boot; without it the package behaves exactly as before (every cache dies on restart), which is
// what the unit tests want.
//
// A read error or a corrupt snapshot aborts nothing: it starts without a cache and the next
// successful fetch rewrites the file.
func EnablePersistence(load func() ([]byte, error), save func([]byte) error) {
	persistMu.Lock()
	persistSave = save
	persistMu.Unlock()

	data, err := load()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to read the AniList cache snapshot; starting with an empty cache")
		return
	}
	if len(data) == 0 {
		return
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		logger.Logger.Warn().Err(err).Msg("Discarding a corrupt AniList cache snapshot; starting with an empty cache")
		return
	}

	frontendListCache.restore(snap.FrontendList, 0)
	passListCache.restore(snap.PassList, 0)
	mediaEntryCache.restore(snap.MediaEntries, 0)
	customListsCache.restore(snap.CustomLists, 0)
	mediaByIDCache.restore(snap.MediaByID, 0)
	seriesCache.restore(snap.Series, seriesTTL)
	malListCache.restore(snap.MALLists, 0)
	malIDMapCache.restore(snap.MALIDMap, malIDMapTTL)
	snapshotAt.Store(snap.SavedAt.Unix())

	logger.Logger.Info().
		Time("saved_at", snap.SavedAt).
		Int("frontend_lists", len(snap.FrontendList)).
		Int("pass_lists", len(snap.PassList)).
		Int("media_entries", len(snap.MediaEntries)).
		Int("series_links", len(snap.Series)).
		Msg("Loaded the AniList cache snapshot from disk")
}

// SnapshotSavedAt is the date of the disk snapshot, or zero if there's none. It's what answers
// "serving the cache from when?" on the screen.
func SnapshotSavedAt() time.Time {
	if unix := snapshotAt.Load(); unix > 0 {
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

// markCacheDirty schedules the write. Cheap to call too often: with a pending timer it does
// nothing, and with persistence off neither.
func markCacheDirty() {
	persistMu.Lock()
	defer persistMu.Unlock()
	if persistSave == nil || persistTimer != nil {
		return
	}
	persistTimer = time.AfterFunc(persistDebounce, flushCache)
}

func flushCache() {
	persistMu.Lock()
	persistTimer = nil
	save := persistSave
	persistMu.Unlock()
	if save == nil {
		return
	}

	now := time.Now()
	data, err := json.Marshal(snapshot{
		SavedAt:      now,
		FrontendList: frontendListCache.snapshot(),
		PassList:     passListCache.snapshot(),
		MediaEntries: mediaEntryCache.snapshot(),
		CustomLists:  customListsCache.snapshot(),
		MediaByID:    mediaByIDCache.snapshot(),
		Series:       seriesCache.snapshot(),
		MALLists:     malListCache.snapshot(),
		MALIDMap:     malIDMapCache.snapshot(),
	})
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to marshal the AniList cache snapshot")
		return
	}

	// A write failure doesn't propagate: the disk cache is a convenience, and the caller was serving a
	// request that has nothing to do with it.
	if err := save(data); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to write the AniList cache snapshot")
		return
	}
	snapshotAt.Store(now.Unix())
}

// disablePersistence turns it off and cancels the pending write. It exists for test isolation: a
// scheduled snapshot that fires after the test ends would write into another test's temp directory.
// In production EnablePersistence runs once at boot and nobody turns it off.
func disablePersistence() {
	persistMu.Lock()
	defer persistMu.Unlock()
	persistSave = nil
	if persistTimer != nil {
		persistTimer.Stop()
		persistTimer = nil
	}
	snapshotAt.Store(0)
}

// stampAiringAt converts TimeUntilAiring (relative to the fetch instant) into AiringAt (absolute)
// before storing, and it's what makes the cache usable to decide whether an episode already aired.
//
// Without it TimeUntilAiring freezes at the value of the fetch moment, and since EVERY "already
// aired" gate reads that field (daemon/episodes.go, anilist/episodes.go,
// daemon/manual_download.go), a pass served from the cache would conclude forever that nothing new
// premiered. With the absolute instant stored, rebaseAiring returns the right count at serving time
// — and the episode that was ten minutes away when AniList went down becomes "already aired" on its
// own, with no new rule.
//
// It only fills what's zero: the media-id query already asks for the real airingAt, and the API's
// is better than ours. The pass's and the frontend's don't ask for it, and that's where this
// matters.
func stampAiringAt(list []MediaList, now time.Time) {
	unix := now.Unix()
	for i := range list {
		media := &list[i].Media
		for j := range media.AiringSchedule.Nodes {
			node := &media.AiringSchedule.Nodes[j]
			if node.AiringAt == 0 {
				node.AiringAt = unix + int64(node.TimeUntilAiring)
			}
		}
		if media.NextAiringEpisode != nil && media.NextAiringEpisode.AiringAt == 0 {
			media.NextAiringEpisode.AiringAt = unix + int64(media.NextAiringEpisode.TimeUntilAiring)
		}
	}
}

// rebaseAiring is the inverse of stampAiringAt: it recomputes TimeUntilAiring from the absolute
// instant, so an entry stored yesterday doesn't claim the same six hours remain as yesterday.
// Called on EVERY path that serves from the cache.
//
// It works on copies of the fields it rewrites, never on the stored entry: rewriting the stored one
// would make the next reader rebase an already rebased value.
func rebaseAiring(list []MediaList, now time.Time) {
	unix := now.Unix()
	for i := range list {
		media := &list[i].Media
		nodes := make([]AiringNode, len(media.AiringSchedule.Nodes))
		copy(nodes, media.AiringSchedule.Nodes)
		for j := range nodes {
			if nodes[j].AiringAt != 0 {
				nodes[j].TimeUntilAiring = int(nodes[j].AiringAt - unix)
			}
		}
		media.AiringSchedule.Nodes = nodes

		if media.NextAiringEpisode != nil && media.NextAiringEpisode.AiringAt != 0 {
			next := *media.NextAiringEpisode
			next.TimeUntilAiring = int(next.AiringAt - unix)
			media.NextAiringEpisode = &next
		}
	}
}

// staleList returns the expired entry of a MediaList cache ready to use: a shallow copy (the caller
// overwrites CustomLists) and the premiere count recomputed.
func staleList(cache *ttlCache[[]MediaList], key string) ([]MediaList, bool) {
	stored, ok := cache.getStale(key)
	if !ok {
		return nil, false
	}
	out := append([]MediaList(nil), stored...)
	rebaseAiring(out, time.Now())
	return out, true
}

// storeList stores the list for the fallback: a shallow copy, so the caller's CustomLists overwrite
// doesn't reach what was stored, and AiringAt filled in. It stores an empty list too: "this account
// has nothing in these statuses" is a legitimate answer, and not storing it would make the 30s poll
// hit the network forever for whoever has an empty list.
func storeList(cache *ttlCache[[]MediaList], key string, list []MediaList, ttl time.Duration) {
	stored := append([]MediaList(nil), list...)
	stampAiringAt(stored, time.Now())
	cache.set(key, stored, ttl)
	markCacheDirty()
}

// staleMedia returns the expired standalone entry with the premiere count recomputed. It returns
// ok=true for a stored nil entry: nil there means "AniList doesn't know this id", which is an
// answer, not the absence of one.
func staleMedia(key string) (*MediaList, bool) {
	stored, ok := mediaByIDCache.getStale(key)
	if !ok {
		return nil, false
	}
	out := copyMediaList(stored)
	if out != nil {
		one := []MediaList{*out}
		rebaseAiring(one, time.Now())
		out = &one[0]
	}
	return out, true
}

// seriesKey is the seriesCache key. It exists so persist.go and series.go agree on the format
// without repeating the Itoa in several places.
func seriesKey(mediaID int) string { return strconv.Itoa(mediaID) }
