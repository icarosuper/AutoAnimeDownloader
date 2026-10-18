package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
)

// mockAniListRoutes responds by query name: GetMediaByID and SearchMedia have their own body,
// everything else (frontend list, customLists) falls back to listJSON. calls, when not nil,
// counts the requests — how the tests prove a path did NOT hit AniList.
func mockAniListRoutes(calls *int, listJSON, mediaJSON, searchJSON string) func() {
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if calls != nil {
			*calls++
		}
		payload := listJSON
		switch {
		case strings.Contains(string(body), "GetMediaByIDs"):
			payload = batchMediaJSON(mediaJSON)
		case strings.Contains(string(body), "GetMediaByID"):
			payload = mediaJSON
		case strings.Contains(string(body), "SearchMedia"):
			payload = searchJSON
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})
}

// batchMediaJSON converts a GetMediaByID fixture (single Media) into the response GetMediaByIDs
// expects (Page.media[]) — both queries ask for the same fields, so the tests do not need two
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

// mockStandaloneAniList: the list query returns empty (the anime is on no list) and the media-id
// query returns the anime. Exactly the situation of a standalone.
func mockStandaloneAniList(listJSON, mediaJSON string) func() {
	return mockAniListRoutes(nil, listJSON, mediaJSON, mediaJSON)
}

func mockStandaloneAniListCounting(calls *int, listJSON, mediaJSON string) func() {
	return mockAniListRoutes(calls, listJSON, mediaJSON, mediaJSON)
}

const emptyListJSON = `{"data":{"Page":{"mediaList":[]}}}`

const standaloneMediaJSON = `{"data":{"Media":{
	"id": 21, "episodes": 24, "format": "TV", "status": "RELEASING",
	"title": {"english": "Standalone Anime", "romaji": "Standalone Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "cover.jpg", "medium": ""},
	"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 111}]}
}}}`

func standaloneServer(fm *mockFileManager) *Server {
	return &Server{State: daemon.NewState(), FileManager: fm}
}

// TestHandleAnimeEpisodes_StandaloneAnimeIsNot404: without the GetMediaByID fallback the
// detail screen of a standalone simply does not open.
func TestHandleAnimeEpisodes_StandaloneAnimeIsNot404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()

	handleAnimeEpisodes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for a standalone, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data AnimeDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp.Data.AnimeID != 21 || len(resp.Data.Episodes) != 1 {
		t.Fatalf("incomplete standalone detail: %+v", resp.Data)
	}
}

// TestHandleAnimeEpisodes_NonStandaloneUntrackedStill404: the fallback applies only to
// standalones — an anime on no list and not in the file is still 404.
func TestHandleAnimeEpisodes_NonStandaloneUntrackedStill404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()

	handleAnimeEpisodes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// TestHandleAnimes_StandaloneAppearsWithoutDownloadedEpisodes: without this the standalone only
// shows up in the Status list after an episode has been downloaded.
func TestHandleAnimes_StandaloneAppearsWithoutDownloadedEpisodes(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
	rec := httptest.NewRecorder()

	handleAnimes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []AnimeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("want 1 anime in the list, got %d: %+v", len(resp.Data), resp.Data)
	}
	got := resp.Data[0]
	if got.AnimeID != 21 {
		t.Fatalf("want anime 21, got %+v", got)
	}
	if !got.IsStandalone {
		t.Fatal("the standalone must come flagged with is_standalone, or the chip never shows")
	}
	if got.CoverImage != "cover.jpg" || got.TotalEpisodes != 24 {
		t.Fatalf("standalone missing cover/total: %+v", got)
	}
}

// TestHandleAnimes_StandaloneUsesSavedProgress: without injecting the saved progress, the screen
// would show 0 watched for a standalone whose progress the user just saved in AnimeSettings.
func TestHandleAnimes_StandaloneUsesSavedProgress(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{
		standaloneAnimes: []int{21},
		animeSettings:    map[int]files.AnimeSettings{21: {Progress: 48}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
	rec := httptest.NewRecorder()

	handleAnimes(standaloneServer(fm))(rec, req)

	var resp struct {
		Data []AnimeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].EpisodesWatched != 48 {
		t.Fatalf("want episodes_watched 48, got %+v", resp.Data)
	}
}

// TestHandleAnimes_ListedAnimeIsNotStandalone: is_standalone is origin, and a listed anime
// must not get the chip.
func TestHandleAnimes_ListedAnimeIsNotStandalone(t *testing.T) {
	listJSON := `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"customLists":{},"media":{
		"id": 21, "episodes": 24, "status": "RELEASING",
		"title": {"english": "Listed Anime", "romaji": "Listed Anime"},
		"coverImage": {"large": "cover.jpg", "medium": ""},
		"airingSchedule": {"nodes": []}
	}}]}}}`
	defer mockStandaloneAniList(listJSON, standaloneMediaJSON)()

	fm := &mockFileManager{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
	rec := httptest.NewRecorder()

	handleAnimes(standaloneServer(fm))(rec, req)

	var resp struct {
		Data []AnimeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].IsStandalone {
		t.Fatalf("a listed anime must not come as standalone: %+v", resp.Data)
	}
}

var _ = files.EpisodeStruct{}
