package anilist

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSearchMedia_MapsResults: the search bar of the #/add screen depends on these fields.
func TestSearchMedia_MapsResults(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[
		{"id":21,"title":{"english":"One Piece","romaji":"One Piece"},"format":"TV","status":"RELEASING",
		 "seasonYear":1999,"episodes":null,"coverImage":{"large":"big.jpg","medium":"small.jpg"}},
		{"id":5114,"title":{"english":null,"romaji":"Hagane no Renkinjutsushi"},"format":"TV","status":"FINISHED",
		 "seasonYear":2009,"episodes":64,"coverImage":{"large":"","medium":"fma.jpg"}}
	]}}}`)()

	results, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}

	first := results[0]
	if first.Id != 21 || first.Format != MediaFormatTV || first.Status != MediaStatusReleasing {
		t.Fatalf("first result mapped wrong: %+v", first)
	}
	if first.SeasonYear != 1999 {
		t.Fatalf("want seasonYear 1999, got %d", first.SeasonYear)
	}
	if first.Episodes != nil {
		t.Fatalf("null episodes should become nil, got %v", *first.Episodes)
	}
	if first.CoverImage.Large != "big.jpg" {
		t.Fatalf("cover mapped wrong: %+v", first.CoverImage)
	}
	if results[1].Episodes == nil || *results[1].Episodes != 64 {
		t.Fatalf("want 64 episodes on the second result, got %v", results[1].Episodes)
	}
}

// TestSearchMedia_UnreleasedFilterIsServerSide: the filter has to travel IN THE QUERY. Filtering
// after receiving would empty the search (perPage is 20), and a test that only looked at the
// result would pass with both implementations — that's why this one inspects the sent body.
func TestSearchMedia_UnreleasedFilterIsServerSide(t *testing.T) {
	var sent string
	defer MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		sent = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"media":[]}}}`)),
		}, nil
	})()

	if _, err := SearchMedia("one", false); err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if !strings.Contains(sent, "status_not: NOT_YET_RELEASED") {
		t.Fatalf("the default needs to hide unreleased in the query, got: %s", sent)
	}

	if _, err := SearchMedia("one", true); err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if strings.Contains(sent, "status_not") {
		t.Fatalf("with includeUnreleased the argument can't exist, got: %s", sent)
	}
}

// TestGetMediaByID_SyntheticEntry: the returned MediaList doesn't come from any list, so
// Progress/Status/Id need to be zeros — not junk the loop's dedupe would take seriously.
func TestGetMediaByID_SyntheticEntry(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":{
		"id":21,"episodes":12,"format":"TV","status":"RELEASING",
		"title":{"english":"My Anime","romaji":"Boku no Anime"},
		"synonyms":["MA"],
		"relations":{"edges":[{"node":{"title":{"english":"Prequel","romaji":"Prequel"},"synonyms":[],"episodes":13},"relationType":"PREQUEL"}]},
		"coverImage":{"large":"big.jpg","medium":"small.jpg"},
		"airingSchedule":{"nodes":[{"id":10,"episode":1,"timeUntilAiring":-100,"airingAt":123}]}
	}}}`)()

	ml, err := GetMediaByID(21, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if ml == nil {
		t.Fatal("want a MediaList, got nil")
	}
	if ml.Id != 0 || ml.Progress != 0 || ml.Status != "" {
		t.Fatalf("synthetic entry needs to come zeroed, got Id=%d Progress=%d Status=%q", ml.Id, ml.Progress, ml.Status)
	}
	if ml.Media.Id != 21 {
		t.Fatalf("want media id 21, got %d", ml.Media.Id)
	}
	// Anime search and searchNyaaForSingleEpisode depend on these three.
	if len(ml.Media.Synonyms) != 1 {
		t.Fatalf("synonyms lost: %+v", ml.Media.Synonyms)
	}
	if len(ml.Media.Relations.Edges) != 1 || ml.Media.Relations.Edges[0].RelationType != "PREQUEL" {
		t.Fatalf("relations lost: %+v", ml.Media.Relations)
	}
	if len(ml.Media.AiringSchedule.Nodes) != 1 || ml.Media.AiringSchedule.Nodes[0].ID != 10 {
		t.Fatalf("airingSchedule without id: %+v", ml.Media.AiringSchedule.Nodes)
	}
}

// TestGetMediaByID_CachesWithinTTL: GET /animes has a 30s poll per tab and starts calling this
// per standalone anime — the same reason GetFrontendAnimeList is cached.
func TestGetMediaByID_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":{"id":21,"title":{"romaji":"X"}}}}`)()

	if _, err := GetMediaByID(21, PriorityCritical); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := GetMediaByID(21, PriorityCritical); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 request, got %d", calls)
	}

	if _, err := GetMediaByID(22, PriorityCritical); err != nil {
		t.Fatalf("other id: %v", err)
	}
	if calls != 2 {
		t.Fatalf("a different id is a different key: want 2 requests, got %d", calls)
	}
}

// TestGetMediaByID_NotFound: an id that doesn't exist returns (nil, nil), which the POST
// translates into 404.
func TestGetMediaByID_NotFound(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":null}}`)()

	ml, err := GetMediaByID(999999, PriorityCritical)
	if err != nil {
		t.Fatalf("null Media is not an error: %v", err)
	}
	if ml != nil {
		t.Fatalf("want nil, got %+v", ml)
	}
}

// mockCapturing returns payloads in sequence and stores the sent bodies — that's how the batch
// tests prove WHICH ids were requested, not just how many requests went out.
func mockCapturing(sent *[]string, payloads ...string) func() {
	return MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		*sent = append(*sent, string(body))
		payload := payloads[len(payloads)-1]
		if len(*sent) <= len(payloads) {
			payload = payloads[len(*sent)-1]
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})
}

// TestGetMediaByIDs_SingleRequestForManyIDs is why the function exists: the previous loop
// spent 1 request per standalone against AniList's 30/min limit (decisions.md #65).
func TestGetMediaByIDs_SingleRequestForManyIDs(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[
		{"id":21,"title":{"romaji":"A"}},
		{"id":22,"title":{"romaji":"B"}},
		{"id":23,"title":{"romaji":"C"}}
	]}}}`)()

	medias, err := GetMediaByIDs([]int{21, 22, 23}, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByIDs: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("want 1 request for 3 ids, got %d", len(sent))
	}
	if !strings.Contains(sent[0], "id_in") {
		t.Fatalf("the query needs to fetch in batch with id_in, got: %s", sent[0])
	}
	for _, id := range []int{21, 22, 23} {
		if medias[id] == nil || medias[id].Media.Id != id {
			t.Fatalf("id %d didn't come back in the map: %+v", id, medias[id])
		}
	}
}

// TestGetMediaByIDs_AbsentIDIsNilNotMissing: id_in OMITS the deleted AniList media instead of
// returning an error. Without turning the omission into an explicit nil, the caller can't
// tell "dead standalone" from "couldn't fetch" and the anime vanishes from the screen with
// no warning.
func TestGetMediaByIDs_AbsentIDIsNilNotMissing(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"A"}}]}}}`)()

	medias, err := GetMediaByIDs([]int{21, 999999}, PriorityCritical)
	if err != nil {
		t.Fatalf("an omitted id is not an error: %v", err)
	}
	ml, ok := medias[999999]
	if !ok {
		t.Fatal("the omitted id needs to be in the map as nil; absent means 'wasn't fetched'")
	}
	if ml != nil {
		t.Fatalf("want nil for the omitted id, got %+v", ml)
	}
}

// TestGetMediaByIDs_CachesPerID: the cache is per id, not per batch — keyed by the batch, a
// new standalone would invalidate the read of every other one on each addition.
func TestGetMediaByIDs_CachesPerID(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent,
		`{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"A"}},{"id":22,"title":{"romaji":"B"}}]}}}`,
		`{"data":{"Page":{"media":[{"id":23,"title":{"romaji":"C"}}]}}}`,
	)()

	if _, err := GetMediaByIDs([]int{21, 22}, PriorityCritical); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := GetMediaByIDs([]int{21, 22, 23}, PriorityCritical); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("want 2 requests, got %d", len(sent))
	}
	if strings.Contains(sent[1], "21") || !strings.Contains(sent[1], "23") {
		t.Fatalf("the second fetch can only ask for the new id, got: %s", sent[1])
	}

	// The cache is the same as GetMediaByID: whoever fetched in batch doesn't pay again on a
	// standalone lookup.
	if _, err := GetMediaByID(22, PriorityCritical); err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("GetMediaByID had to come from the batch cache, got %d requests", len(sent))
	}
}

// TestGetMediaByIDs_ChunksAtPageSize: AniList's max perPage is 50 — asking for 60 ids in a
// single query would return 50 and the other 10 would look deleted.
func TestGetMediaByIDs_ChunksAtPageSize(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[]}}}`)()

	ids := make([]int, 0, 60)
	for i := range 60 {
		ids = append(ids, 1000+i)
	}

	medias, err := GetMediaByIDs(ids, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByIDs: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("60 ids with perPage 50 are 2 requests, got %d", len(sent))
	}
	if len(medias) != 60 {
		t.Fatalf("every requested id needs an outcome in the map, got %d", len(medias))
	}
}

// TestSearchMedia_CachesWithinTTL: the screen searches on submit, so the same key repeats —
// user redoing the search, unreleased toggle going back and forth, another tab with the same
// term. Case-insensitive because AniList's search is too.
func TestSearchMedia_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"One Piece"}}]}}}`)()

	for _, term := range []string{"one piece", "one piece", "One Piece", "ONE PIECE"} {
		if _, err := SearchMedia(term, false); err != nil {
			t.Fatalf("SearchMedia(%q): %v", term, err)
		}
	}
	if calls != 1 {
		t.Fatalf("want 1 request for the same term in any case, got %d", calls)
	}

	if _, err := SearchMedia("one piece", true); err != nil {
		t.Fatalf("SearchMedia with includeUnreleased: %v", err)
	}
	if calls != 2 {
		t.Fatalf("includeUnreleased is part of the key: want 2 requests, got %d", calls)
	}
}

// TestSearchMedia_CachedSliceIsNotShared: the stored slice is served to every request that
// hits the key — the caller must not cut or reorder the cache's memory.
func TestSearchMedia_CachedSliceIsNotShared(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[
		{"id":21,"title":{"romaji":"A"}},
		{"id":22,"title":{"romaji":"B"}}
	]}}}`)()

	first, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	first[0].Id = 999

	second, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if second[0].Id != 21 {
		t.Fatalf("the cache was corrupted by the caller: %+v", second[0])
	}
}

// TestSearchMedia_ServesStaleWhenRefused: refused by the budget gate, the search serves the
// expired result instead of becoming "the search failed" on the screen. See decisions.md #72.
func TestSearchMedia_ServesStaleWhenRefused(t *testing.T) {
	defer MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"One Piece"}}]}}}`,
			map[string]string{"X-RateLimit-Remaining": "29"}), nil
	})()

	if _, err := SearchMedia("one piece", false); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Expire the cache and sink the budget: the next call is refused by the gate.
	searchCache.set(searchCacheKey("one piece", false), []MediaSearchResult{{Id: 21}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	results, err := SearchMedia("one piece", false)
	if err != nil {
		t.Fatalf("want the expired cache, got error: %v", err)
	}
	if len(results) != 1 || results[0].Id != 21 {
		t.Fatalf("want the expired entry, got %+v", results)
	}
}

// TestSearchMedia_FlushesWhenFull: only cache with a free-text key, and an expired entry never
// leaves the map — without a cap, the map only grows while the daemon runs.
func TestSearchMedia_FlushesWhenFull(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[]}}}`)()

	for i := range searchCacheMaxEntries {
		if _, err := SearchMedia("term"+strconv.Itoa(i), false); err != nil {
			t.Fatalf("search %d: %v", i, err)
		}
	}
	if searchCache.size() != searchCacheMaxEntries {
		t.Fatalf("want the cache full at %d, got %d", searchCacheMaxEntries, searchCache.size())
	}

	if _, err := SearchMedia("overflow", false); err != nil {
		t.Fatalf("search that overflows the cap: %v", err)
	}
	if searchCache.size() != 1 {
		t.Fatalf("past the cap the cache is discarded whole: want 1, got %d", searchCache.size())
	}
}
