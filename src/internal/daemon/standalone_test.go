package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

// mockAniListRouter answers the list query with listJSON and the media-by-id query with
// mediaJSON — searchAnilist with standalones does both.
func mockAniListRouter(t *testing.T, listJSON, mediaJSON string) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		payload := listJSON
		switch {
		case strings.Contains(string(body), "GetMediaByIDs"):
			payload = batchMediaJSON(mediaJSON)
		case strings.Contains(string(body), "GetMediaByID"):
			payload = mediaJSON
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     make(http.Header),
		}, nil
	})
}

// batchMediaJSON converts a GetMediaByID fixture (single Media) into the response GetMediaByIDs
// expects (Page.media[]) — both queries ask for the same fields, so the tests don't need two
// copies of each anime. Null Media becomes an empty list, which is how id_in returns a missing id.
func batchMediaJSON(mediaJSON string) string {
	var single struct {
		Data struct {
			Media json.RawMessage `json:"Media"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(mediaJSON), &single); err != nil || string(single.Data.Media) == "null" || len(single.Data.Media) == 0 {
		return `{"data":{"Page":{"media":[]}}}`
	}
	return `{"data":{"Page":{"media":[` + string(single.Data.Media) + `]}}}`
}

// mockEmptyNyaa keeps the tests off the network: the torrent list comes back empty.
func mockEmptyNyaa() func() {
	return nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
}

func standaloneTestConfig() *files.Config {
	return &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
	}
}

const listWithAnime100 = `{"data": {"Page": {"mediaList": [
	{"id": 1, "status": "CURRENT", "progress": 7, "customLists": {}, "media": {
		"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
		"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
		"synonyms": [], "relations": {"edges": []},
		"airingSchedule": {"nodes": []}
	}}
]}}}`

const mediaForAnime100 = `{"data": {"Media": {
	"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
	"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

const mediaForAnime500 = `{"data": {"Media": {
	"id": 500, "format": "TV", "status": "RELEASING", "episodes": 24,
	"title": {"english": "Standalone Anime", "romaji": "Standalone Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": [{"id": 900, "episode": 1, "timeUntilAiring": -100, "airingAt": 1}]}
}}}`

// TestSearchAnilist_StandaloneAppendedAfterDedupe: a standalone missing from the lists enters
// the pass's universe, otherwise it is never verified again after being added.
func TestSearchAnilist_StandaloneAppended(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime500)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{500})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	byMedia := map[int]anilist.MediaList{}
	for _, ml := range resp.Data.Page.MediaList {
		byMedia[ml.Media.Id] = ml
	}
	if len(byMedia) != 2 {
		t.Fatalf("want 2 animes (list + standalone), got %d", len(byMedia))
	}
	if byMedia[500].Media.Id != 500 {
		t.Fatal("standalone 500 did not enter the pass")
	}
	if len(fm.removedStandalone) != 0 {
		t.Fatalf("a standalone outside the list must not be removed from the file, removed %v", fm.removedStandalone)
	}
}

// TestSearchAnilist_StandaloneAlsoInListDoesNotDuplicateNorResetProgress locks the order: the
// append happens AFTER the dedupe. If it came first, the synthetic Progress 0 would win
// (DedupeByMedia keeps the lowest) and the daemon would re-download already-watched episodes.
func TestSearchAnilist_StandaloneAlsoInListDoesNotDuplicateNorResetProgress(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime100)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{100}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{100})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("a standalone that is also on the list duplicated: %d entries", len(resp.Data.Page.MediaList))
	}
	if got := resp.Data.Page.MediaList[0].Progress; got != 7 {
		t.Fatalf("the real entry must win: want Progress 7, got %d", got)
	}
}

// TestSearchAnilist_StandaloneAdoptedByListIsRemovedFromFile: adopted by a list, the
// standalone record no longer has a purpose — and the file stops growing.
func TestSearchAnilist_StandaloneAdoptedByListIsRemovedFromFile(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime100)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{100}}
	if _, err := searchAnilist(fm, standaloneTestConfig(), []int{100}); err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if !containsID(fm.removedStandalone, 100) {
		t.Fatalf("a standalone adopted by the list must leave the file, removed: %v", fm.removedStandalone)
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("the standalones file should be empty, got %v", fm.standaloneAnimes)
	}
}

// TestSearchAnilist_StandaloneOutsideDownloadStatusesStays: an anime in PLANNING does not
// appear in the set the daemon processes, so the standalone record remains the only reason
// it is verified — and the most obvious use case of the feature.
func TestSearchAnilist_StandaloneOutsideDownloadStatusesStays(t *testing.T) {
	// The list (filtered by download_statuses server-side) comes back empty: the anime is in
	// PLANNING on AniList, but the daemon only searches CURRENT.
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{500})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 500 {
		t.Fatalf("a standalone outside download_statuses must be processed, got %+v", resp.Data.Page.MediaList)
	}
	if len(fm.removedStandalone) != 0 {
		t.Fatalf("a standalone outside download_statuses must not leave the file, removed %v", fm.removedStandalone)
	}
}

// TestDownloadStandaloneAnime_DoesNotDeleteTheRestOfTheLibrary locks why
// DownloadStandaloneAnime does not call handleSavedEpisodes: with delete_watched_episodes on
// and only one anime's episodes in hand, identifyEpisodesNotInWatching would delete the rest.
func TestDownloadStandaloneAnime_DoesNotDeleteTheRestOfTheLibrary(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	defer mockEmptyNyaa()()

	configs := standaloneTestConfig()
	configs.DeleteWatchedEpisodes = true

	// Library with 3 animes already saved: none of them can vanish because of the add.
	fm := &mockFileManagerForEpisodes{savedEpisodes: []files.EpisodeStruct{
		{EpisodeNumber: 1, AnimeID: 10, EpisodeHash: "h1"},
		{EpisodeNumber: 2, AnimeID: 20, EpisodeHash: "h2"},
		{EpisodeNumber: 3, AnimeID: 30, EpisodeHash: "h3"},
	}}
	backend := torrents.NewFakeBackend()

	if _, err := DownloadStandaloneAnime(fm, backend, configs, 500); err != nil {
		t.Fatalf("DownloadStandaloneAnime: %v", err)
	}

	if len(fm.deletedEpisodeKeys) != 0 {
		t.Fatalf("adding a standalone must not delete any episode, deleted %v", fm.deletedEpisodeKeys)
	}
}

// TestAppendStandaloneAnimes_InjectsSavedProgress: without progress, after the first pack the
// standalone stops forever — nothing is ever "watched", pruning never runs and the rotation
// has nothing to move it.
func TestAppendStandaloneAnimes_InjectsSavedProgress(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	fm := &mockFileManagerForEpisodes{settings: map[int]files.AnimeSettings{500: {Progress: 48}}}

	merged := appendStandaloneAnimes(fm, nil, []int{500})

	if len(merged) != 1 {
		t.Fatalf("want 1 anime, got %d", len(merged))
	}
	if merged[0].Progress != 48 {
		t.Errorf("want Progress 48, got %d", merged[0].Progress)
	}
	if got := firstEpisodeToConsider(merged[0], nil); got != 49 {
		t.Errorf("the list must start at 49, got %d", got)
	}
}

// TestAppendStandaloneAnimes_NoSettingsMeansZeroProgress: with no AnimeSettings saved,
// Progress 0 — the previous behavior.
func TestAppendStandaloneAnimes_NoSettingsMeansZeroProgress(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	merged := appendStandaloneAnimes(&mockFileManagerForEpisodes{}, nil, []int{500})

	if len(merged) != 1 || merged[0].Progress != 0 {
		t.Errorf("want Progress 0, got %+v", merged)
	}
}

// TestDownloadStandaloneAnime_UnknownMedia: an id AniList does not know must not become a
// silent download.
func TestDownloadStandaloneAnime_UnknownMedia(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, `{"data": {"Media": null}}`)()

	fm := &mockFileManagerForEpisodes{}
	if _, err := DownloadStandaloneAnime(fm, torrents.NewFakeBackend(), standaloneTestConfig(), 999999); err == nil {
		t.Fatal("want an error for an unknown media id")
	}
}
