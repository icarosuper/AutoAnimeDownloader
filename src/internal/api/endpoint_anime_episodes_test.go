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

// mockAnimeDetailResponse builds a minimal AniList MediaListDetailResponse JSON body with two
// airing nodes: episode 1 (id 1) and episode 2 (id 2). Used to drive handleAnimeEpisodes without
// a real network call.
const mockAnimeDetailResponse = `{
	"data": {
		"MediaList": {
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
			EpisodeID:    1,
			AnimeID:      1,
			EpisodeHash:  "0123456789abcdef0123456789abcdef01234567",
			EpisodeName:  "Test Anime - Episode 1",
			DownloadDate: time.Now(),
		},
	}}
	server := &Server{FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/1/episodes", nil)
	req.SetPathValue("id", "1")
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
