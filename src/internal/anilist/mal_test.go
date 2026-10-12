package anilist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// malItem is one load.json entry, as MAL sends it.
func malItem(id, status, watched, rewatching int) string {
	return fmt.Sprintf(`{"anime_id":%d,"anime_title":"Anime %d","status":%d,"num_watched_episodes":%d,"is_rewatching":%d}`,
		id, id, status, watched, rewatching)
}

// malFake answers both MAL (load.json pages by offset) and AniList (idMal_in mapping as
// id = malID + 1000, id_in hydration with a RELEASING media). unknown MAL ids are omitted by
// the mapping, like AniList does. It records every AniList query's ids.
type malFake struct {
	pages      map[int]string // offset -> JSON array body
	malStatus  int            // non-zero: MAL answers this status code
	unknown    map[int]bool
	failIDs    map[int]bool // id_in requests containing one of these answer 500
	malCalls   int
	mapCalls   [][]int
	hydrateIDs [][]int
	entries    string // AniList mediaList answer for per-account queries
}

func (f *malFake) do(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "myanimelist") {
		f.malCalls++
		if f.malStatus != 0 {
			return respond(f.malStatus, `{"errors":[{"message":"invalid request"}]}`, nil), nil
		}
		offset := 0
		fmt.Sscanf(req.URL.Query().Get("offset"), "%d", &offset)
		body, ok := f.pages[offset]
		if !ok {
			body = "[]"
		}
		return respond(http.StatusOK, body, nil), nil
	}

	raw, _ := io.ReadAll(req.Body)
	var payload struct {
		Query     string `json:"query"`
		Variables struct {
			IDs []int `json:"ids"`
		} `json:"variables"`
	}
	json.Unmarshal(raw, &payload)
	switch {
	case strings.Contains(payload.Query, "idMal_in"):
		f.mapCalls = append(f.mapCalls, payload.Variables.IDs)
		var media []string
		for _, id := range payload.Variables.IDs {
			if !f.unknown[id] {
				media = append(media, fmt.Sprintf(`{"id":%d,"idMal":%d}`, id+1000, id))
			}
		}
		return respond(http.StatusOK, `{"data":{"Page":{"media":[`+strings.Join(media, ",")+`]}}}`, nil), nil
	case strings.Contains(payload.Query, "id_in"):
		f.hydrateIDs = append(f.hydrateIDs, payload.Variables.IDs)
		for _, id := range payload.Variables.IDs {
			if f.failIDs[id] {
				return respond(http.StatusInternalServerError, `{"errors":[{"message":"boom"}]}`, nil), nil
			}
		}
		var media []string
		for _, id := range payload.Variables.IDs {
			media = append(media, fmt.Sprintf(`{"id":%d,"status":"RELEASING","title":{"romaji":"Anime %d"}}`, id, id))
		}
		return respond(http.StatusOK, `{"data":{"Page":{"media":[`+strings.Join(media, ",")+`]}}}`, nil), nil
	default:
		if f.entries == "" {
			return respond(http.StatusOK, `{"data":{"Page":{"mediaList":[]}}}`, nil), nil
		}
		return respond(http.StatusOK, `{"data":{"Page":{"mediaList":[`+f.entries+`]}}}`, nil), nil
	}
}

func TestGetMALEntries_PaginatesAndMapsStatuses(t *testing.T) {
	// A full first page means there is a second one; the loop stops at the short page.
	first := make([]string, malPageSize)
	for i := range first {
		first[i] = malItem(i+1, 2, 12, 0)
	}
	first[0] = malItem(1, 1, 3, 0) // watching
	first[1] = malItem(2, 2, 5, 1) // completed + rewatching
	first[2] = malItem(3, 3, 0, 0) // on hold
	first[3] = malItem(4, 4, 0, 0) // dropped
	first[4] = malItem(5, 6, 0, 0) // plan to watch
	first[5] = malItem(6, 5, 0, 0) // unknown status: skipped
	// Measured live: is_rewatching sometimes comes as "" instead of 0.
	first[6] = `{"anime_id":7,"anime_title":"Anime 7","status":2,"num_watched_episodes":12,"is_rewatching":""}`
	fake := &malFake{pages: map[int]string{
		0:           "[" + strings.Join(first, ",") + "]",
		malPageSize: "[" + malItem(9999, 1, 7, 0) + "]",
	}}
	defer MockAniListDo(fake.do)()

	entries, err := GetMALEntries("user", PriorityCritical)
	if err != nil {
		t.Fatalf("GetMALEntries: %v", err)
	}
	if fake.malCalls != 2 {
		t.Fatalf("want 2 MAL pages fetched, got %d", fake.malCalls)
	}
	if len(entries) != malPageSize { // 300 - 1 unknown status + 1 on page two
		t.Fatalf("want %d entries, got %d", malPageSize, len(entries))
	}

	byMedia := map[int]MALEntry{}
	for _, e := range entries {
		byMedia[e.MediaID] = e
	}
	want := map[int]MediaListStatus{
		1001: MediaListStatusCurrent, 1002: MediaListStatusRepeating, 1003: MediaListStatusPaused,
		1004: MediaListStatusDropped, 1005: MediaListStatusPlanning, 1007: MediaListStatusCompleted,
		10999: MediaListStatusCurrent,
	}
	for id, status := range want {
		if byMedia[id].Status != status {
			t.Errorf("media %d: want %s, got %q", id, status, byMedia[id].Status)
		}
	}
	if _, ok := byMedia[1006]; ok {
		t.Error("an entry with an unknown MAL status must be skipped")
	}
	if byMedia[10999].Progress != 7 {
		t.Errorf("want progress 7 from num_watched_episodes, got %d", byMedia[10999].Progress)
	}
	if len(fake.mapCalls) != 6 { // 300 ids / 50 per query
		t.Errorf("want 6 mapping queries, got %d", len(fake.mapCalls))
	}
}

func TestGetMALEntries_UnmappedIDIsCachedAndSkipped(t *testing.T) {
	fake := &malFake{
		pages:   map[int]string{0: "[" + malItem(1, 1, 0, 0) + "," + malItem(2, 1, 0, 0) + "]"},
		unknown: map[int]bool{2: true},
	}
	defer MockAniListDo(fake.do)()

	entries, err := GetMALEntries("user", PriorityCritical)
	if err != nil {
		t.Fatalf("GetMALEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].MediaID != 1001 {
		t.Fatalf("want only the mapped entry, got %+v", entries)
	}

	// Expire the list so it's fetched again: the "no counterpart" answer must stay cached.
	malListCache.clear()
	if _, err := GetMALEntries("user", PriorityCritical); err != nil {
		t.Fatalf("second GetMALEntries: %v", err)
	}
	if len(fake.mapCalls) != 1 {
		t.Errorf("an unmappable id must not be asked again, got %d mapping queries", len(fake.mapCalls))
	}
}

// An empty list is a failure, never "this account tracks nothing": that is what keeps a private
// list or a format change from silently dropping the account's veto in the deletion vote.
func TestGetMALEntries_EmptyIsAFailure(t *testing.T) {
	cases := map[string]string{
		"empty list": "[]",
		// A format change on MAL's side: every field decodes to zero.
		"unrecognized format": `[{"id":1,"state":"watching"},{"id":2,"state":"completed"}]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			defer MockAniListDo((&malFake{pages: map[int]string{0: body}}).do)()
			if _, err := GetMALEntries("user", PriorityCritical); !errors.Is(err, ErrMALEmpty) {
				t.Fatalf("want ErrMALEmpty, got %v", err)
			}
		})
	}
}

func TestGetMALEntries_MALDownServesStale(t *testing.T) {
	fake := &malFake{pages: map[int]string{0: "[" + malItem(1, 1, 4, 0) + "]"}}
	defer MockAniListDo(fake.do)()
	if _, err := GetMALEntries("user", PriorityCritical); err != nil {
		t.Fatalf("GetMALEntries: %v", err)
	}

	malListCache.restore(malListCache.snapshot(), 0) // expired, visible only to getStale
	fake.malStatus = http.StatusServiceUnavailable

	entries, err := GetMALEntries("user", PriorityCritical)
	if !errors.Is(err, ErrFromCache) {
		t.Fatalf("want ErrFromCache, got %v", err)
	}
	if len(entries) != 1 || entries[0].Progress != 4 {
		t.Fatalf("want the stale entry, got %+v", entries)
	}
}

func TestGetMALEntries_UnknownUserFails(t *testing.T) {
	defer MockAniListDo((&malFake{malStatus: http.StatusBadRequest}).do)()
	_, err := GetMALEntries("nobody", PriorityCritical)
	if err == nil || errors.Is(err, ErrFromCache) {
		t.Fatalf("want a plain error for an unknown user, got %v", err)
	}
}

// Hydration must only reach AniList for the entries that survive the status filter: hydrating a
// whole list (mostly COMPLETED) on every poll would burn the budget decisions.md #46 protects.
func TestGetMALList_FiltersBeforeHydrating(t *testing.T) {
	fake := &malFake{pages: map[int]string{0: "[" +
		malItem(1, 1, 2, 0) + "," + malItem(2, 2, 12, 0) + "," + malItem(3, 2, 12, 0) + "]"}}
	defer MockAniListDo(fake.do)()

	list, err := GetMALList("user", []string{"CURRENT"}, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMALList: %v", err)
	}
	if len(fake.hydrateIDs) != 1 || len(fake.hydrateIDs[0]) != 1 || fake.hydrateIDs[0][0] != 1001 {
		t.Fatalf("want one hydration query for media 1001 only, got %v", fake.hydrateIDs)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 entry, got %d", len(list))
	}
	got := list[0]
	if got.Media.Id != 1001 || got.Status != MediaListStatusCurrent || got.Progress != 2 || got.Id != 0 {
		t.Fatalf("want media 1001 CURRENT progress 2 with Id 0, got %+v", got)
	}
}

// A failed hydration of ONE new entry must not take the whole account down: an account missing
// from the pass has its downloaded episodes deleted. What came back is kept, as ErrFromCache.
func TestGetMALList_PartialHydrationKeepsWhatCameBack(t *testing.T) {
	fake := &malFake{pages: map[int]string{0: "[" + malItem(1, 1, 2, 0) + "," + malItem(2, 1, 0, 0) + "]"}}
	defer MockAniListDo(fake.do)()

	// 1001 is fresh in mediaByIDCache, as the frontend poll keeps it; 1002 is brand new.
	if _, err := GetMediaByIDs([]int{1001}, PriorityCritical); err != nil {
		t.Fatalf("warming the cache: %v", err)
	}
	fake.failIDs = map[int]bool{1002: true}

	list, err := GetMALList("user", []string{"CURRENT"}, PriorityCritical)
	if !errors.Is(err, ErrFromCache) {
		t.Fatalf("want ErrFromCache for a partial answer, got %v", err)
	}
	if len(list) != 1 || list[0].Media.Id != 1001 {
		t.Fatalf("want media 1001 kept, got %+v", list)
	}
}

func TestGetMALList_HydrationWithNothingBackFails(t *testing.T) {
	fake := &malFake{
		pages:   map[int]string{0: "[" + malItem(1, 1, 2, 0) + "]"},
		failIDs: map[int]bool{1001: true},
	}
	defer MockAniListDo(fake.do)()

	if _, err := GetMALList("user", []string{"CURRENT"}, PriorityCritical); err == nil || errors.Is(err, ErrFromCache) {
		t.Fatalf("want a plain error when nothing came back, got %v", err)
	}
}

// The detail screen and the pass must agree: with an AniList and a MAL account on the same
// anime, the LOWER progress wins in GetAnimeInfo too.
func TestGetAnimeInfo_MALAccountLowerProgressWins(t *testing.T) {
	fake := &malFake{
		pages:   map[int]string{0: "[" + malItem(1, 1, 3, 0) + "]"},
		entries: `{"id":77,"status":"CURRENT","progress":5,"media":{"id":1001,"status":"RELEASING"}}`,
	}
	defer MockAniListDo(fake.do)()

	ml, err := GetAnimeInfo(1001, []string{"anilistUser"}, []string{"malUser"}, PriorityCritical)
	if err != nil {
		t.Fatalf("GetAnimeInfo: %v", err)
	}
	if ml == nil || ml.Progress != 3 {
		t.Fatalf("want progress 3 from the MAL account, got %+v", ml)
	}
}

func TestGetAnimeInfo_OnlyMALAccountTracksIt(t *testing.T) {
	fake := &malFake{pages: map[int]string{0: "[" + malItem(1, 1, 3, 0) + "]"}}
	defer MockAniListDo(fake.do)()

	ml, err := GetAnimeInfo(1001, nil, []string{"malUser"}, PriorityCritical)
	if err != nil || ml == nil || ml.Media.Id != 1001 {
		t.Fatalf("want the MAL entry, got %+v, %v", ml, err)
	}
	if ml, err := GetAnimeInfo(4242, nil, []string{"malUser"}, PriorityCritical); ml != nil || err != nil {
		t.Fatalf("an anime no account tracks is (nil, nil), got %+v, %v", ml, err)
	}
}

// Live check against the real MyAnimeList and AniList. load.json is unofficial, and no mock
// proves its format still holds: a change on MAL's side must show up here as a failure, not as a
// silent empty list. Only runs with AAD_LIVE_MAL=1, so `go test ./...` doesn't depend on the
// network. Xinil is MAL's founder, with a large public list.
func TestLive_MALList(t *testing.T) {
	if os.Getenv("AAD_LIVE_MAL") != "1" {
		t.Skip("set AAD_LIVE_MAL=1 to run")
	}
	entries, err := GetMALEntries("Xinil", PriorityCritical)
	if err != nil {
		t.Fatalf("GetMALEntries: %v", err)
	}
	counts := map[MediaListStatus]int{}
	for _, e := range entries {
		counts[e.Status]++
	}
	t.Logf("%d entries mapped to AniList: %v", len(entries), counts)
	if len(entries) < malPageSize {
		t.Errorf("want more than one page of entries, got %d", len(entries))
	}

	list, err := GetMALList("Xinil", []string{"CURRENT"}, PriorityCritical)
	if err != nil || len(list) == 0 {
		t.Fatalf("GetMALList CURRENT: %d entries, %v", len(list), err)
	}
	t.Logf("first CURRENT: media %d, progress %d", list[0].Media.Id, list[0].Progress)
}

// The persisted MAL list is what keeps a MAL-only anime in the pass when MAL is down after a
// restart: without it the account drops out and identifyEpisodesNotInWatching deletes its
// downloads. The mapping must come back too, or AniList would be asked again.
func TestMALCachesSurviveRestart(t *testing.T) {
	fake := &malFake{pages: map[int]string{0: "[" + malItem(1, 1, 4, 0) + "]"}}
	defer MockAniListDo(fake.do)()

	load, save, current := memoryStore(nil)
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)
	if _, err := GetMALEntries("user", PriorityCritical); err != nil {
		t.Fatalf("GetMALEntries: %v", err)
	}
	flushCache()
	if len(*current) == 0 {
		t.Fatal("the snapshot was not written")
	}

	clearCaches() // the restart
	EnablePersistence(load, save)
	fake.malStatus = http.StatusServiceUnavailable
	mapCalls := len(fake.mapCalls)

	entries, err := GetMALEntries("user", PriorityCritical)
	if !errors.Is(err, ErrFromCache) {
		t.Fatalf("want ErrFromCache after the restart, got %v", err)
	}
	if len(entries) != 1 || entries[0].MediaID != 1001 || entries[0].Progress != 4 {
		t.Fatalf("want the restored entry, got %+v", entries)
	}
	if len(fake.mapCalls) != mapCalls {
		t.Error("the restored mapping must not ask AniList again")
	}
}
