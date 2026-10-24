package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func standaloneConfig() *files.Config {
	return &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		CheckInterval:         10,
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
		ExcludedLists:         []string{},
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
	}
}

func standaloneWriteServer(fm *mockFileManager) *Server {
	return &Server{
		State:       daemon.NewState(),
		FileManager: fm,
		Torrents:    torrents.NewFakeBackend(),
		Librarian:   files.NewLibrarian(files.NewOSFileSystem()),
	}
}

// mockEmptyNyaa keeps the tests off the network: no torrent found.
func mockEmptyNyaa() func() {
	return nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
}

func postStandalone(t *testing.T, server *Server, mediaID int) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]int{"media_id": mediaID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/standalone-animes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleStandaloneAnimeAdd(server)(rec, req)
	return rec
}

const standaloneMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"

const finished24MediaJSON = `{"data":{"Media":{
	"id": 21, "episodes": 24, "format": "TV", "status": "FINISHED",
	"title": {"english": "Standalone Anime", "romaji": "Standalone Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

func postStandaloneMagnet(t *testing.T, server *Server, body StandaloneAnimeRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/standalone-animes", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handleStandaloneAnimeAdd(server)(rec, req)
	return rec
}

func decodeAdded(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	var resp struct {
		Data struct {
			Added int `json:"added"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	return resp.Data.Added
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("want an error, got %s", rec.Body.String())
	}
	return resp.Error.Code
}

// TestStandaloneAdd_LibraryNotConfigured: the check comes first because it does not depend on
// the network, and because without a library Ensure would fail with an opaque 500 instead of
// the cause.
func TestStandaloneAdd_LibraryNotConfigured(t *testing.T) {
	calls := 0
	defer mockStandaloneAniListCounting(&calls, emptyListJSON, standaloneMediaJSON)()

	config := standaloneConfig()
	config.CompletedAnimePath = ""
	fm := &mockFileManager{configs: config}

	rec := postStandalone(t, standaloneWriteServer(fm), 21)

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "LIBRARY_NOT_CONFIGURED" {
		t.Fatalf("want LIBRARY_NOT_CONFIGURED, got %s", got)
	}
	if calls != 0 {
		t.Fatalf("an unconfigured library must not hit AniList, made %d calls", calls)
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("nothing may go into the file, got %v", fm.standaloneAnimes)
	}
}

// TestStandaloneAdd_UnknownMediaIs404: GetMediaByID is also what validates that the id exists.
func TestStandaloneAdd_UnknownMediaIs404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, `{"data":{"Media":null}}`)()

	fm := &mockFileManager{configs: standaloneConfig()}
	rec := postStandalone(t, standaloneWriteServer(fm), 999999)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("a missing id must not enter the file, got %v", fm.standaloneAnimes)
	}
}

// TestStandaloneAdd_BlockedNeverWritesTheFile: a block that writes before refusing would be
// worse than no block.
func TestStandaloneAdd_BlockedNeverWritesTheFile(t *testing.T) {
	listWithAnime21 := `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"customLists":{"Blacklist":true},"media":{
		"id": 21, "episodes": 24, "status": "RELEASING",
		"title": {"english": "Listed Anime", "romaji": "Listed Anime"},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": []}
	}}]}}}`

	cases := []struct {
		name     string
		listJSON string
		fm       *mockFileManager
		wantCode string
	}{
		{
			name:     "already standalone",
			listJSON: emptyListJSON,
			fm:       &mockFileManager{configs: standaloneConfig(), standaloneAnimes: []int{21}},
			wantCode: "ALREADY_STANDALONE",
		},
		{
			name:     "already on the processed list",
			listJSON: strings.Replace(listWithAnime21, `"customLists":{"Blacklist":true}`, `"customLists":{}`, 1),
			fm:       &mockFileManager{configs: standaloneConfig()},
			wantCode: "ALREADY_TRACKED",
		},
		{
			name:     "on an excluded list",
			listJSON: listWithAnime21,
			fm: &mockFileManager{configs: func() *files.Config {
				c := standaloneConfig()
				c.ExcludedLists = []string{"Blacklist"}
				return c
			}()},
			wantCode: "ALREADY_BLACKLISTED",
		},
		{
			name:     "already fully downloaded",
			listJSON: emptyListJSON,
			fm: &mockFileManager{
				configs:  standaloneConfig(),
				episodes: episodesForAnime(21, 24),
			},
			wantCode: "ALREADY_DOWNLOADED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer mockStandaloneAniList(tc.listJSON, standaloneMediaJSON)()

			before := len(tc.fm.standaloneAnimes)
			server := standaloneWriteServer(tc.fm)
			rec := postStandalone(t, server, 21)

			if rec.Code != http.StatusConflict {
				t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := errorCode(t, rec); got != tc.wantCode {
				t.Fatalf("want %s, got %s", tc.wantCode, got)
			}
			if len(tc.fm.standaloneAnimes) != before {
				t.Fatalf("a block must not write to the file: %v", tc.fm.standaloneAnimes)
			}
			if backend, ok := server.Torrents.(*torrents.FakeBackend); ok && len(backend.EnsureCalls()) != 0 {
				t.Fatal("a block must not reach DownloadStandaloneAnime")
			}
		})
	}
}

// TestStandaloneAdd_NoAiredEpisodeIsStillTracked: added == 0 is a normal response, not an
// error — an anime that has not premiered yet is added and stays tracked.
func TestStandaloneAdd_NoAiredEpisodeIsStillTracked(t *testing.T) {
	unairedMedia := `{"data":{"Media":{
		"id": 21, "episodes": 12, "format": "TV", "status": "NOT_YET_RELEASED",
		"title": {"english": "Future Anime", "romaji": "Future Anime"},
		"synonyms": [], "relations": {"edges": []},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": 86400, "airingAt": 999}]}
	}}}`
	defer mockStandaloneAniList(emptyListJSON, unairedMedia)()
	defer mockEmptyNyaa()()

	fm := &mockFileManager{configs: standaloneConfig()}
	rec := postStandalone(t, standaloneWriteServer(fm), 21)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Added int `json:"added"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp.Data.Added != 0 {
		t.Fatalf("want added 0, got %d", resp.Data.Added)
	}
	if len(fm.standaloneAnimes) != 1 || fm.standaloneAnimes[0] != 21 {
		t.Fatalf("the anime must stay tracked even with no episode aired, file: %v", fm.standaloneAnimes)
	}
}

// TestStandaloneRemove_KeepEpisodesMarksThemManuallyManaged: ManuallyManaged is what closes the
// window in which the files would be at risk after this endpoint stops tracking.
func TestStandaloneRemove_KeepEpisodesMarksThemManuallyManaged(t *testing.T) {
	fm := &mockFileManager{
		configs:          standaloneConfig(),
		standaloneAnimes: []int{21},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 21, EpisodeHash: "h1"},
			{EpisodeNumber: 2, AnimeID: 21, EpisodeHash: "h2"},
			{EpisodeNumber: 3, AnimeID: 99, EpisodeHash: "h3"},
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/standalone-animes/21", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()
	handleStandaloneAnimeRemove(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("the id must leave the file, got %v", fm.standaloneAnimes)
	}
	if len(fm.episodes) != 3 {
		t.Fatalf("without delete_episodes nothing may be deleted, got %d episodes", len(fm.episodes))
	}
	for _, ep := range fm.episodes {
		if ep.AnimeID == 21 && !ep.ManuallyManaged {
			t.Fatalf("standalone episode %d must become ManuallyManaged", ep.EpisodeNumber)
		}
		if ep.AnimeID == 99 && ep.ManuallyManaged {
			t.Fatal("an episode of another anime must not be touched")
		}
	}
}

// TestStandaloneRemove_DeleteEpisodes: with delete_episodes=true the records disappear.
func TestStandaloneRemove_DeleteEpisodes(t *testing.T) {
	fm := &mockFileManager{
		configs:          standaloneConfig(),
		standaloneAnimes: []int{21},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 21, EpisodeHash: "h1"},
			{EpisodeNumber: 3, AnimeID: 99, EpisodeHash: "h3"},
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/standalone-animes/21?delete_episodes=true", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()
	handleStandaloneAnimeRemove(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.episodes) != 1 || fm.episodes[0].AnimeID != 99 {
		t.Fatalf("only the standalone's episodes could leave, got %+v", fm.episodes)
	}
}

// TestAniListSearch_FillsBlockReason is the test that pins front and back to the same function:
// the search result's block_reason must be the same value the POST uses for the 409.
func TestAniListSearch_FillsBlockReason(t *testing.T) {
	searchJSON := `{"data":{"Page":{"media":[
		{"id":21,"title":{"english":"Already Standalone","romaji":"x"},"format":"TV","status":"RELEASING","seasonYear":2016,"episodes":24,"coverImage":{"large":"a.jpg","medium":""}},
		{"id":22,"title":{"english":null,"romaji":"Addable"},"format":"TV","status":"FINISHED","seasonYear":2020,"episodes":12,"coverImage":{"large":"","medium":"b.jpg"}}
	]}}}`
	defer mockAniListRoutes(nil, emptyListJSON, standaloneMediaJSON, searchJSON)()

	fm := &mockFileManager{configs: standaloneConfig(), standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anilist/search?q=test", nil)
	rec := httptest.NewRecorder()
	handleAniListSearch(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []AniListSearchResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("want 2 results, got %d", len(resp.Data))
	}
	if resp.Data[0].BlockReason != blockReasonStandalone {
		t.Fatalf("want %q, got %q", blockReasonStandalone, resp.Data[0].BlockReason)
	}
	if resp.Data[1].BlockReason != "" {
		t.Fatalf("an addable result must come with empty block_reason, got %q", resp.Data[1].BlockReason)
	}
	if resp.Data[1].Title != "Addable" {
		t.Fatalf("a title without english must fall back to romaji, got %q", resp.Data[1].Title)
	}
	if resp.Data[1].Cover != "b.jpg" {
		t.Fatalf("cover must fall back to medium when there is no large, got %q", resp.Data[1].Cover)
	}
	if resp.Data[0].Year != 2016 || resp.Data[0].Episodes != 24 {
		t.Fatalf("result meta mapped wrong: %+v", resp.Data[0])
	}
}

// TestAniListSearch_ShortTermIsEmptyList: a short term returns an empty list, not 400 — the
// front types letter by letter.
func TestAniListSearch_ShortTermIsEmptyList(t *testing.T) {
	calls := 0
	defer mockStandaloneAniListCounting(&calls, emptyListJSON, emptyListJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	for _, term := range []string{"", "a", "ab"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anilist/search?q="+term, nil)
		rec := httptest.NewRecorder()
		handleAniListSearch(standaloneWriteServer(fm))(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("term %q: want 200, got %d", term, rec.Code)
		}
		var resp struct {
			Data []AniListSearchResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid response: %v", err)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("term %q should return an empty list, got %d", term, len(resp.Data))
		}
	}
	if calls != 0 {
		t.Fatalf("a short term must not hit AniList, made %d calls", calls)
	}
}

func episodesForAnime(animeID, n int) []files.EpisodeStruct {
	eps := make([]files.EpisodeStruct, 0, n)
	for i := 1; i <= n; i++ {
		eps = append(eps, files.EpisodeStruct{EpisodeNumber: i, AnimeID: animeID})
	}
	return eps
}

func TestStandaloneAdd_WithMagnet_WritesOnlyCoveredEpisodes(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, finished24MediaJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	server := standaloneWriteServer(fm)
	rec := postStandaloneMagnet(t, server, StandaloneAnimeRequest{
		MediaID: 21,
		Magnet:  standaloneMagnet,
		Name:    "[SubsPlease] Standalone Anime (01-12) [1080p]",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := decodeAdded(t, rec); got != 12 {
		t.Fatalf("want added 12 of 24, got %d", got)
	}
	if len(fm.standaloneAnimes) != 1 || fm.standaloneAnimes[0] != 21 {
		t.Fatalf("standalone id 21 missing, got %v", fm.standaloneAnimes)
	}
	if len(fm.episodes) != 12 {
		t.Fatalf("want 12 saved episodes, got %d", len(fm.episodes))
	}
	for _, ep := range fm.episodes {
		if ep.EpisodeNumber < 1 || ep.EpisodeNumber > 12 {
			t.Errorf("uncovered episode %d was saved", ep.EpisodeNumber)
		}
	}
	backend, ok := server.Torrents.(*torrents.FakeBackend)
	if !ok {
		t.Fatal("want FakeBackend")
	}
	if len(backend.List()) != 1 {
		t.Fatalf("want torrent in session, got %d", len(backend.List()))
	}
}

func TestStandaloneAdd_WithMagnet_UnknownRangeWritesNoEpisodes(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, finished24MediaJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	server := standaloneWriteServer(fm)
	rec := postStandaloneMagnet(t, server, StandaloneAnimeRequest{
		MediaID: 21,
		Magnet:  standaloneMagnet,
		Name:    "[SubsPlease] Standalone Anime [1080p]",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := decodeAdded(t, rec); got != 0 {
		t.Fatalf("want added 0 for an unknown range, got %d", got)
	}
	if len(fm.standaloneAnimes) != 1 || fm.standaloneAnimes[0] != 21 {
		t.Fatalf("standalone id 21 missing, got %v", fm.standaloneAnimes)
	}
	if len(fm.episodes) != 0 {
		t.Fatalf("unknown range must write no episode records, got %v", fm.episodes)
	}
	backend := server.Torrents.(*torrents.FakeBackend)
	if len(backend.List()) != 1 {
		t.Fatalf("want torrent in session, got %d", len(backend.List()))
	}
}

func TestStandaloneAdd_WithMagnet_AddFailureDoesNotWriteID(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, finished24MediaJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	server := standaloneWriteServer(fm)
	backend := server.Torrents.(*torrents.FakeBackend)
	backend.AddErr = errors.New("refused")

	rec := postStandaloneMagnet(t, server, StandaloneAnimeRequest{
		MediaID: 21,
		Magnet:  standaloneMagnet,
		Name:    "[SubsPlease] Standalone Anime (01-12) [1080p]",
	})

	if rec.Code == http.StatusOK {
		t.Fatalf("want an error when the torrent add is refused, got 200: %s", rec.Body.String())
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("standalone id must stay absent, got %v", fm.standaloneAnimes)
	}
	if len(fm.episodes) != 0 {
		t.Fatalf("want no episodes, got %v", fm.episodes)
	}
	if len(backend.List()) != 0 {
		t.Fatalf("refused add must leave the session empty, got %d torrents", len(backend.List()))
	}
}

func TestStandaloneAdd_WithMagnet_InvalidMagnet(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, finished24MediaJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	server := standaloneWriteServer(fm)
	rec := postStandaloneMagnet(t, server, StandaloneAnimeRequest{
		MediaID: 21,
		Magnet:  "http://example.com/not-a-magnet",
		Name:    "[SubsPlease] Standalone Anime (01-12) [1080p]",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "INVALID_MAGNET" {
		t.Fatalf("want INVALID_MAGNET, got %s", got)
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("invalid magnet must not write the id, got %v", fm.standaloneAnimes)
	}
}

func TestStandaloneAdd_WithMagnet_BlockedNeverWritesTheFile(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, finished24MediaJSON)()

	fm := &mockFileManager{configs: standaloneConfig(), standaloneAnimes: []int{21}}
	server := standaloneWriteServer(fm)
	rec := postStandaloneMagnet(t, server, StandaloneAnimeRequest{
		MediaID: 21,
		Magnet:  standaloneMagnet,
		Name:    "[SubsPlease] Standalone Anime (01-12) [1080p]",
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "ALREADY_STANDALONE" {
		t.Fatalf("want ALREADY_STANDALONE, got %s", got)
	}
	if len(fm.standaloneAnimes) != 1 {
		t.Fatalf("a block must not change the file: %v", fm.standaloneAnimes)
	}
	backend := server.Torrents.(*torrents.FakeBackend)
	if len(backend.List()) != 0 {
		t.Fatal("a block must not add a torrent")
	}
	if len(backend.EnsureCalls()) != 0 {
		t.Fatal("a block must not reach DownloadStandaloneAnimeWithMagnet")
	}
}
