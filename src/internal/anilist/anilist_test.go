package anilist

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func mockList(calls *int, body string) func() {
	return MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
}

// TestGetFrontendAnimeList_CachesWithinTTL locks the 429 fix: /api/v1/animes is polled by the
// frontend every 30s per tab, and without a cache each open tab became one request per account
// against AniList's 30/min limit — the resulting 429 took the daemon's download cycle down with it.
func TestGetFrontendAnimeList_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":10}}]}}}`)()

	statuses := []string{"CURRENT"}

	if _, err := GetFrontendAnimeList("user", statuses); err != nil {
		t.Fatalf("primeira busca falhou: %v", err)
	}
	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("segunda busca falhou: %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 request to AniList, got %d", calls)
	}
	if len(second.Data.Page.MediaList) != 1 {
		t.Fatalf("cache returned %d entries, want 1", len(second.Data.Page.MediaList))
	}

	// Another account and another status combination are different keys: they must fetch.
	if _, err := GetFrontendAnimeList("outra", statuses); err != nil {
		t.Fatalf("busca de outra conta falhou: %v", err)
	}
	if calls != 2 {
		t.Fatalf("want 2 requests after switching account, got %d", calls)
	}
}

// TestGetFrontendAnimeList_CacheHandsOutCopies makes sure the customLists overlay
// fetchAniListEntries applies to the result doesn't leak to the next caller (nor become a race
// between two concurrent /api/v1/animes requests).
func TestGetFrontendAnimeList_CacheHandsOutCopies(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"media":{"id":10}}]}}}`)()

	statuses := []string{"CURRENT"}
	first, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("primeira busca falhou: %v", err)
	}
	first.Data.Page.MediaList[0].CustomLists = CustomLists{"AutoDownloader": true}

	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("segunda busca falhou: %v", err)
	}
	if second.Data.Page.MediaList[0].CustomLists != nil {
		t.Errorf("cache contaminado pelo chamador anterior: %v", second.Data.Page.MediaList[0].CustomLists)
	}
}

// TestGetCustomListsMap_CachesEmptyResult: an account with no custom lists can't generate one
// request per frontend poll. The short TTL (customListsEmptyTTL) is what limits that.
func TestGetCustomListsMap_CachesEmptyResult(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"customLists":null}]}}}`)()

	statuses := []string{"CURRENT"}
	GetCustomListsMap("user", statuses, PriorityCritical)
	GetCustomListsMap("user", statuses, PriorityCritical)

	if calls != 1 {
		t.Fatalf("want 1 request to AniList, got %d", calls)
	}
}

// TestMockAniListDo_ClearsCaches: without this a test would inherit the previous test's responses.
func TestMockAniListDo_ClearsCaches(t *testing.T) {
	firstCalls := 0
	restore := mockList(&firstCalls, `{"data":{"Page":{"mediaList":[{"id":1,"media":{"id":10}}]}}}`)
	GetFrontendAnimeList("user", []string{"CURRENT"})
	restore()

	secondCalls := 0
	defer mockList(&secondCalls, `{"data":{"Page":{"mediaList":[]}}}`)()
	resp, err := GetFrontendAnimeList("user", []string{"CURRENT"})
	if err != nil {
		t.Fatalf("busca falhou: %v", err)
	}
	if secondCalls != 1 {
		t.Fatalf("want the new mock to be queried, requests=%d", secondCalls)
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Errorf("response came from the previous test's cache: %+v", resp.Data.Page.MediaList)
	}
}

// TestGetAnimeInfoByIDs_ChunksPerAccount: the GET /animes orphan refresh runs on every 30s poll per
// tab. One request per anime per account was the burst of decisions.md #65; the contract here is
// ceil(ids/50) per account, and nothing more.
func TestGetAnimeInfoByIDs_ChunksPerAccount(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[]}}}`)()

	ids := make([]int, 0, 60)
	for i := range 60 {
		ids = append(ids, 1000+i)
	}

	if _, err := GetAnimeInfoByIDs(ids, []string{"a", "b"}, nil); err != nil {
		t.Fatalf("GetAnimeInfoByIDs: %v", err)
	}
	if calls != 4 {
		t.Fatalf("60 ids in 2 accounts with perPage 50 are 4 requests, got %d", calls)
	}
}

// TestGetAnimeInfoByIDs_KeepsLowestProgressAcrossAccounts: the same rule as GetAnimeInfo and
// DedupeByMedia — an episode is only "watched" when ALL accounts watched it. Without this the
// orphan refresh would lower the progress of the account that's behind.
func TestGetAnimeInfoByIDs_KeepsLowestProgressAcrossAccounts(t *testing.T) {
	call := 0
	defer MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		call++
		progress := 10
		if call == 2 {
			progress = 3
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"data":{"Page":{"mediaList":[{"id":1,"progress":%d,"media":{"id":21,"title":{"romaji":"X"}}}]}}}`, progress))),
		}, nil
	})()

	byMedia, err := GetAnimeInfoByIDs([]int{21}, []string{"a", "b"}, nil)
	if err != nil {
		t.Fatalf("GetAnimeInfoByIDs: %v", err)
	}
	if byMedia[21] == nil || byMedia[21].Progress != 3 {
		t.Fatalf("want the LOWEST progress of the two accounts (3), got %+v", byMedia[21])
	}
}

// TestGetAnimeInfoByIDs_UntrackedIDIsAbsent: an id no account tracks isn't in the map — it's the
// normal state of an anime taken off the list with episodes still on disk, not an error.
func TestGetAnimeInfoByIDs_UntrackedIDIsAbsent(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":21}}]}}}`)()

	byMedia, err := GetAnimeInfoByIDs([]int{21, 99}, []string{"a"}, nil)
	if err != nil {
		t.Fatalf("an id with no entry is not an error: %v", err)
	}
	if byMedia[21] == nil {
		t.Fatal("the tracked id must be in the map")
	}
	if _, ok := byMedia[99]; ok {
		t.Fatalf("the id nobody tracks can't be in the map, got %+v", byMedia[99])
	}
}

// TestGetAnimeInfoByIDs_PartialFailureKeepsWhatCame: one account's error can't wipe what the other
// answered — the screen would lose cover and name across the whole list because of one account.
func TestGetAnimeInfoByIDs_PartialFailureKeepsWhatCame(t *testing.T) {
	call := 0
	defer MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		call++
		if call == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":21}}]}}}`)),
		}, nil
	})()

	byMedia, err := GetAnimeInfoByIDs([]int{21}, []string{"a", "b"}, nil)
	if err == nil {
		t.Fatal("one account's failure must be reported")
	}
	if byMedia[21] == nil {
		t.Fatal("the error can't invalidate what the other account answered")
	}
}
