package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockAnimeDetailResponse builds a minimal AniList list-entry JSON body with two airing nodes:
// episode 1 (id 1) and episode 2 (id 2). Used to drive handleAnimeEpisodes without a real
// network call. The route id is the MEDIA id (21), not the entry id (1).
const mockAnimeDetailResponse = `{
	"data": {
		"Page": {
			"mediaList": [
				{
					"id": 1,
					"status": "CURRENT",
					"progress": 0,
					"customLists": {},
					"media": {
						"id": 21,
						"episodes": 24,
						"format": "TV",
						"status": "RELEASING",
						"title": {"english": "Test Anime", "romaji": "Test Anime"},
						"synonyms": [],
						"relations": {"edges": []},
						"coverImage": {"large": "", "medium": ""},
						"airingSchedule": {
							"nodes": [
								{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 111},
								{"id": 2, "episode": 2, "timeUntilAiring": -50, "airingAt": 222}
							]
						}
					}
				}
			]
		}
	}
}`

func mockAnimeDetail() func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockAnimeDetailResponse)),
		}, nil
	})
}

// TestHandleAnimeEpisodes_EpisodeHash covers the episode_hash field added for the inline
// torrent-progress join on the anime detail screen: present (from the saved episode record)
// for a downloaded episode, and omitted from the serialized JSON (via omitempty) for one that
// isn't downloaded. Asserting on the decoded JSON, not the struct, is the point — omitempty
// only has an observable effect on the wire.
func TestHandleAnimeEpisodes_EpisodeHash(t *testing.T) {
	defer mockAnimeDetail()()

	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{
			EpisodeNumber: 1,
			AnimeID:       21,
			EpisodeHash:   "0123456789abcdef0123456789abcdef01234567",
			EpisodeName:   "Test Anime - Episode 1",
			DownloadDate:  time.Now(),
		},
	}}
	server := &Server{FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	w := httptest.NewRecorder()
	handleAnimeEpisodes(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be an object, got %T", response.Data)
	}
	episodes, ok := data["episodes"].([]interface{})
	if !ok || len(episodes) != 2 {
		t.Fatalf("Expected 2 episodes, got %v", data["episodes"])
	}

	ep1, ok := episodes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected episode 1 to be an object")
	}
	if ep1["episode_hash"] != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("episode_hash = %v, want the downloaded episode's hash", ep1["episode_hash"])
	}

	ep2, ok := episodes[1].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected episode 2 to be an object")
	}
	if _, present := ep2["episode_hash"]; present {
		t.Errorf("episode_hash should be omitted (omitempty) for a non-downloaded episode, got %v", ep2["episode_hash"])
	}
}

// An anime whose schedule AniList already clipped (One Piece: page 1 starts at 1123, and
// nextAiringEpisode is the only hint of what has already aired) must show on screen with ALL
// episodes — otherwise there is nowhere to click to download the missing ones. See decisions.md #52.
func TestHandleAnimeEpisodes_ListsEpisodesOutsideTheAniListWindow(t *testing.T) {
	const body = `{"data":{"Page":{"mediaList":[{
		"id": 1, "status": "CURRENT", "progress": 3, "customLists": {},
		"media": {
			"id": 21, "episodes": null, "format": "TV", "status": "RELEASING",
			"title": {"english": "ONE PIECE", "romaji": "ONE PIECE"},
			"synonyms": [], "relations": {"edges": []}, "coverImage": {"large": "", "medium": ""},
			"airingSchedule": {"nodes": [{"id": 416348, "episode": 10, "timeUntilAiring": -100, "airingAt": 111}]},
			"nextAiringEpisode": {"episode": 12, "timeUntilAiring": 3600, "airingAt": 999}
		}
	}]}}}`

	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})()

	server := &Server{FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	w := httptest.NewRecorder()
	handleAnimeEpisodes(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}

	var response struct {
		Data AnimeDetailResponse `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// 1..11 aired (nextAiringEpisode 12) + the future 12, even with the schedule only bringing 10.
	if len(response.Data.Episodes) != 12 {
		t.Fatalf("want 12 episodes on screen, got %d", len(response.Data.Episodes))
	}
	for i, ep := range response.Data.Episodes {
		if ep.EpisodeNumber != i+1 {
			t.Fatalf("episode %d out of order: %+v", i+1, ep)
		}
	}
	if !response.Data.Episodes[0].IsWatched || response.Data.Episodes[3].IsWatched {
		t.Error("is_watched must follow progress (3) even on synthesized episodes")
	}
	if response.Data.Episodes[11].IsAired {
		t.Error("episode 12 has not aired yet")
	}
}

// The pack range reaches the screen from the saved record, not from min/max of listed episodes:
// with 5 watched, a pack 01-11 only has records for 6..11 and min/max would lie. Omitted
// (omitempty) for a standalone episode, which has no range.
func TestHandleAnimeEpisodes_BatchRange(t *testing.T) {
	defer mockAnimeDetail()()

	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{
			EpisodeNumber: 1, AnimeID: 21, EpisodeHash: "pack", EpisodeName: "Test Anime 1-11",
			IsBatch: true, BatchStart: 1, BatchEnd: 11, DownloadDate: time.Now(),
		},
		{
			EpisodeNumber: 2, AnimeID: 21, EpisodeHash: "solo", EpisodeName: "Test Anime - Episode 2",
			DownloadDate: time.Now(),
		},
	}}
	server := &Server{FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	w := httptest.NewRecorder()
	handleAnimeEpisodes(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	episodes := response.Data.(map[string]interface{})["episodes"].([]interface{})
	ep1 := episodes[0].(map[string]interface{})
	if ep1["batch_start"] != float64(1) || ep1["batch_end"] != float64(11) {
		t.Errorf("want batch_start=1 batch_end=11, got %v-%v", ep1["batch_start"], ep1["batch_end"])
	}

	ep2 := episodes[1].(map[string]interface{})
	if _, present := ep2["batch_start"]; present {
		t.Errorf("batch_start must be omitted for a standalone episode, got %v", ep2["batch_start"])
	}
}

// TestHandleAnimeEpisodes_OpensFromCacheDuringOutage is the item that motivated the local cache:
// this screen was the only one that became ENTIRELY unreachable with AniList down, even though
// everything it does besides listing episodes (block, delete, re-download, mark as manual) is
// already local. A 500 here hid all those actions with it.
//
// The test is both halves: the first call populates the cache, the second is AniList already
// answering 503 — and the screen must open with the same episodes.
func TestHandleAnimeEpisodes_OpensFromCacheDuringOutage(t *testing.T) {
	fail := false
	restore := anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		if fail {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockAnimeDetailResponse)),
		}, nil
	})
	defer restore()

	server := &Server{FileManager: &mockFileManager{}}
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
		req.SetPathValue("id", "21")
		w := httptest.NewRecorder()
		handleAnimeEpisodes(server)(w, req)
		return w
	}

	if w := call(); w.Code != http.StatusOK {
		t.Fatalf("the first call had to pass: %d (%s)", w.Code, w.Body.String())
	}

	fail = true
	w := call()
	if w.Code != http.StatusOK {
		t.Fatalf("with AniList down the screen had to open from cache, got %d (%s)", w.Code, w.Body.String())
	}

	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be an object, got %T", response.Data)
	}
	if episodes, ok := data["episodes"].([]interface{}); !ok || len(episodes) != 2 {
		t.Fatalf("want the 2 cached episodes, got %v", data["episodes"])
	}
}
