package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mockAniListEmpty() func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		body := `{"data":{"Page":{"mediaList":[]}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
}

func TestHandleAnimes(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{
		episodes: []files.EpisodeStruct{
			{EpisodeID: 1, EpisodeName: "Test Anime - Episode 1", DownloadDate: time.Now()},
			{EpisodeID: 2, EpisodeName: "Test Anime - Episode 2", DownloadDate: time.Now()},
			{EpisodeID: 3, EpisodeName: "Another Anime - Episode 1", DownloadDate: time.Now()},
		},
	}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleAnimes(server)

	t.Run("GET returns animes list", func(t *testing.T) {
		defer mockAniListEmpty()()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Error("Expected success=true")
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 2 {
			t.Errorf("Expected 2 animes, got %d", len(animes))
		}
	})

	t.Run("Non-GET method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("Empty episodes list returns empty array", func(t *testing.T) {
		defer mockAniListEmpty()()
		mockFMEmpty := &mockFileManager{
			episodes: []files.EpisodeStruct{},
		}
		serverEmpty := &Server{
			State:       state,
			FileManager: mockFMEmpty,
		}
		handlerEmpty := handleAnimes(serverEmpty)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerEmpty(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 0 {
			t.Errorf("Expected 0 animes, got %d", len(animes))
		}
	})

	t.Run("CURRENT anime with no downloaded episodes still appears in list", func(t *testing.T) {
		english := "Vivy: Fluorite Eye's Song"
		totalEps := 13
		anilistBody := `{"data":{"Page":{"mediaList":[{"id":42,"status":"CURRENT","progress":0,"customLists":{},"media":{"format":"TV","status":"RELEASING","episodes":13,"title":{"english":"Vivy: Fluorite Eye's Song","romaji":"Vivy: Fluorite Eye's Song"},"airingSchedule":{"nodes":[]}}}]}}}`
		_ = english
		_ = totalEps
		defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(anilistBody)),
			}, nil
		})()

		mockFMNoEps := &mockFileManager{episodes: []files.EpisodeStruct{}}
		serverNoEps := &Server{State: state, FileManager: mockFMNoEps}
		handlerNoEps := handleAnimes(serverNoEps)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerNoEps(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 1 {
			t.Errorf("Expected 1 anime from AniList merge, got %d", len(animes))
		}

		animeData := animes[0].(map[string]interface{})
		if animeData["name"] != "Vivy: Fluorite Eye's Song" {
			t.Errorf("Expected anime name 'Vivy: Fluorite Eye's Song', got %v", animeData["name"])
		}
		if animeData["episodes_count"].(float64) != 0 {
			t.Errorf("Expected episodes_count=0, got %v", animeData["episodes_count"])
		}
		if animeData["anime_id"].(float64) != 42 {
			t.Errorf("Expected anime_id=42, got %v", animeData["anime_id"])
		}
	})
}
