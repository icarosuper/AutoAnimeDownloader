package anilist

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// memoryStore is the load/save pair EnablePersistence expects, without touching disk. What we
// want to test is the snapshot's semantics, not files.writeAtomic.
func memoryStore(initial []byte) (load func() ([]byte, error), save func([]byte) error, current *[]byte) {
	data := initial
	return func() ([]byte, error) { return data, nil },
		func(b []byte) error { data = b; return nil },
		&data
}

// frontendListBody builds the GetFrontendAnimeList response with an episode timeUntilAiring
// seconds away from airing.
func frontendListBody(mediaID, episode, timeUntilAiring int) string {
	return `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"media":{"id":` +
		strconv.Itoa(mediaID) + `,"title":{"romaji":"Test"},"status":"RELEASING","airingSchedule":{"nodes":[{"episode":` +
		strconv.Itoa(episode) + `,"timeUntilAiring":` + strconv.Itoa(timeUntilAiring) + `}]}}}]}}}`
}

// expire expires the entry without deleting it: that's what makes the next get miss the cache
// and go to the network, leaving the stored value there for getStale. Same trick as budget_test.
func expire(key string) {
	stored, _ := frontendListCache.getStale(key)
	frontendListCache.set(key, stored, -time.Second)
}

// TestOutageServesFrontendListFromCache is the heart of the feature: with AniList down the list
// still comes out, and the caller KNOWS it came from the cache. Without the second fact the
// /animes endpoint would fire the orphan refresh against an API that doesn't answer.
func TestOutageServesFrontendListFromCache(t *testing.T) {
	fail := false
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		if fail {
			return respond(503, `{}`, nil), nil
		}
		return respond(200, frontendListBody(7, 3, 600), nil), nil
	})
	t.Cleanup(restore)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err != nil {
		t.Fatalf("the first fetch had to succeed: %v", err)
	}

	// Expire the TTL so the next get misses the cache and goes to the network — which now fails.
	expire("user\x00CURRENT")
	fail = true

	resp, err := GetFrontendAnimeList("user", []string{"CURRENT"})
	if !errors.Is(err, ErrFromCache) {
		t.Fatalf("want ErrFromCache, got %v", err)
	}
	if resp == nil || len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 7 {
		t.Fatalf("the cache had to return anime 7, got %+v", resp)
	}
}

// TestOutageWithoutCacheStillFails: with no stored entry there's nothing to serve, and the
// error has to bubble up whole. A fallback that invents an empty list would make the screen
// say "you're not tracking anything" with AniList down.
func TestOutageWithoutCacheStillFails(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(503, `{}`, nil), nil
	})
	t.Cleanup(restore)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err == nil || errors.Is(err, ErrFromCache) {
		t.Fatalf("want the raw AniList failure, got %v", err)
	}
}

// TestCachedAiringCountdownIsRebased is why stampAiringAt exists. AniList's TimeUntilAiring is
// relative to the fetch instant: served raw from the cache it freezes, and the pass concludes
// forever that nothing new aired.
//
// Here the snapshot says "10 minutes left" and was written an hour ago. After serving, the
// countdown has to be NEGATIVE — the episode already aired, and that's exactly the case the
// cache needs to get right for the pass to keep downloading with AniList down.
func TestCachedAiringCountdownIsRebased(t *testing.T) {
	restore := MockAniListDo(nil)
	t.Cleanup(restore)

	savedAt := time.Now().Add(-time.Hour)
	stored := []MediaList{{Media: Media{
		Id:             7,
		AiringSchedule: AiringSchedule{Nodes: []AiringNode{{Episode: 3, TimeUntilAiring: 600}}},
	}}}
	// stampAiringAt at write time is what persist.go does via storeList.
	stampAiringAt(stored, savedAt)
	frontendListCache.set("k", stored, 0)

	served, ok := staleList(frontendListCache, "k")
	if !ok {
		t.Fatal("the stored entry had to be served")
	}

	got := served[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring
	if got > -3000 || got < -4200 {
		t.Fatalf("want the countdown rebased to about -3000s (10min left, 1h ago), got %d", got)
	}

	// The STORED entry must not have been rewritten: rebasing twice would walk the clock
	// twice and the next reader would see an even more negative countdown.
	if kept, _ := frontendListCache.getStale("k"); kept[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring != 600 {
		t.Fatalf("the stored entry was rewritten by the rebase: %d", kept[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring)
	}
}

// TestSnapshotSurvivesRestart: the snapshot comes back from disk as FALLBACK, never as fresh
// data. That's the invariant that separates "survives a crash" from "started showing old
// data": a restore that handed the entries to get would make every screen serve yesterday's
// cache without even trying the network.
func TestSnapshotSurvivesRestart(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, frontendListBody(7, 3, 600), nil), nil
	})
	t.Cleanup(restore)

	load, save, current := memoryStore(nil)
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err != nil {
		t.Fatalf("the fetch had to succeed: %v", err)
	}
	// The write is scheduled; the test doesn't wait five seconds for it.
	flushCache()
	if len(*current) == 0 {
		t.Fatal("the snapshot was not written")
	}

	// Simulates the restart: zeros all package state and reloads from "disk".
	clearCaches()
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	key := "user\x00CURRENT"
	if _, fresh := frontendListCache.get(key); fresh {
		t.Fatal("a restored entry can't appear as fresh: the happy path would stop going to the network")
	}
	if _, ok := frontendListCache.getStale(key); !ok {
		t.Fatal("a restored entry had to be available as fallback")
	}
	if SnapshotSavedAt().IsZero() {
		t.Fatal("the snapshot date had to come back, that's what the screen shows on the banner")
	}
}

// TestCorruptSnapshotIsDiscarded: an unreadable file starts without a cache instead of taking
// boot down. The cache exists so the app works worse instead of not working; a bad snapshot
// that blocked startup would invert that.
func TestCorruptSnapshotIsDiscarded(t *testing.T) {
	restore := MockAniListDo(nil)
	t.Cleanup(restore)

	load, save, _ := memoryStore([]byte("{not json"))
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	if !SnapshotSavedAt().IsZero() {
		t.Fatal("a corrupt snapshot can't register a date")
	}
	if _, ok := frontendListCache.getStale("user\x00CURRENT"); ok {
		t.Fatal("a corrupt snapshot can't populate the cache")
	}
}
