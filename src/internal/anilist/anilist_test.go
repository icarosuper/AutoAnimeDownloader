package anilist

import (
	"encoding/json"
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
		t.Fatalf("first fetch failed: %v", err)
	}
	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 request to AniList, got %d", calls)
	}
	if len(second.Data.Page.MediaList) != 1 {
		t.Fatalf("cache returned %d entries, want 1", len(second.Data.Page.MediaList))
	}

	// Another account and another status combination are different keys: they must fetch.
	if _, err := GetFrontendAnimeList("outra", statuses); err != nil {
		t.Fatalf("fetch of another account failed: %v", err)
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
		t.Fatalf("first fetch failed: %v", err)
	}
	first.Data.Page.MediaList[0].CustomLists = CustomLists{"AutoDownloader": true}

	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if second.Data.Page.MediaList[0].CustomLists != nil {
		t.Errorf("cache contaminated by the previous caller: %v", second.Data.Page.MediaList[0].CustomLists)
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
		t.Fatalf("fetch failed: %v", err)
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

// mockPagedList answers each request with pages[page-1] (page read from the request variables),
// and hasNextPage while there are pages left. failPage > 0 answers that page with a 500.
func mockPagedList(t *testing.T, calls *int, pages []string, failPage int) func() {
	t.Helper()
	return MockAniListDo(func(r *http.Request) (*http.Response, error) {
		*calls++
		var req struct {
			Variables struct {
				Page int `json:"page"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		page := req.Variables.Page
		if page == failPage {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		if page < 1 || page > len(pages) {
			t.Errorf("unexpected page %d", page)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`))}, nil
		}
		body := fmt.Sprintf(`{"data":{"Page":{"pageInfo":{"hasNextPage":%t},"mediaList":[%s]}}}`, page < len(pages), pages[page-1])
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
}

// TestListFetchesFollowEveryPage locks the pagination: a Page without perPage stops at 50 entries,
// and an account with more anime than that in the queried statuses lost the rest silently.
func TestListFetchesFollowEveryPage(t *testing.T) {
	pages := []string{
		`{"id":1,"customLists":{"A":true},"media":{"id":10}}`,
		`{"id":2,"media":{"id":20}}`,
		`{"id":3,"customLists":{"B":true},"media":{"id":30}}`,
	}
	statuses := []string{"CURRENT"}

	t.Run("GetAllCurrentAnime", func(t *testing.T) {
		calls := 0
		defer mockPagedList(t, &calls, pages, 0)()
		resp, err := GetAllCurrentAnime("user", statuses)
		if err != nil {
			t.Fatalf("fetch failed: %v", err)
		}
		if calls != 3 || len(resp.Data.Page.MediaList) != 3 {
			t.Fatalf("want 3 requests and 3 entries, got %d requests and %d entries", calls, len(resp.Data.Page.MediaList))
		}
	})

	t.Run("GetFrontendAnimeList", func(t *testing.T) {
		calls := 0
		defer mockPagedList(t, &calls, pages, 0)()
		resp, err := GetFrontendAnimeList("user", statuses)
		if err != nil {
			t.Fatalf("fetch failed: %v", err)
		}
		if calls != 3 || len(resp.Data.Page.MediaList) != 3 {
			t.Fatalf("want 3 requests and 3 entries, got %d requests and %d entries", calls, len(resp.Data.Page.MediaList))
		}
	})

	t.Run("GetCustomListsMap", func(t *testing.T) {
		calls := 0
		defer mockPagedList(t, &calls, pages, 0)()
		m := GetCustomListsMap("user", statuses, PriorityCritical)
		if calls != 3 || len(m) != 3 {
			t.Fatalf("want 3 requests and 3 entries, got %d requests and %d entries", calls, len(m))
		}
		if !m[3]["B"] {
			t.Errorf("customLists of the last page lost: %v", m[3])
		}
	})
}

// TestListFetchFailsWholeOnAFailedPage: the pages that did arrive are not a list. Returning them
// would look complete — an anime past the failed page would read as "not in the list".
func TestListFetchFailsWholeOnAFailedPage(t *testing.T) {
	pages := []string{`{"id":1,"media":{"id":10}}`, `{"id":2,"media":{"id":20}}`}
	calls := 0
	defer mockPagedList(t, &calls, pages, 2)()

	resp, err := GetAllCurrentAnime("user", []string{"CURRENT"})
	if err == nil {
		t.Fatalf("want an error, got %d entries", len(resp.Data.Page.MediaList))
	}
}
