package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockFileManager struct {
	configs  *files.Config
	episodes []files.EpisodeStruct
	loadErr  error
	saveErr  error
	loadEpisodesErr error
	saveEpisodesErr error
	deleteEpisodesErr error
}

func (m *mockFileManager) LoadConfigs() (*files.Config, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.configs == nil {
		return &files.Config{
			AnilistUsername:       "testuser",
			SavePath:              "/tmp/test",
			CompletedAnimePath:    "/tmp/completed",
			CheckInterval:         10,
			QBittorrentUrl:        "http://localhost:8080",
			MaxEpisodesPerAnime:   12,
			EpisodeRetryLimit:     5,
			DeleteWatchedEpisodes: true,
			ExcludedList:          "",
		}, nil
	}
	return m.configs, nil
}

func (m *mockFileManager) SaveConfigs(config *files.Config) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.configs = config
	return nil
}

func (m *mockFileManager) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	if m.loadEpisodesErr != nil {
		return nil, m.loadEpisodesErr
	}
	return m.episodes, nil
}

func (m *mockFileManager) SaveEpisodesToFile(episodes []files.EpisodeStruct) error {
	if m.saveEpisodesErr != nil {
		return m.saveEpisodesErr
	}
	m.episodes = episodes
	return nil
}

func (m *mockFileManager) DeleteEpisodesFromFile(ids []int) error {
	if m.deleteEpisodesErr != nil {
		return m.deleteEpisodesErr
	}
	// Simple implementation for testing
	newEpisodes := []files.EpisodeStruct{}
	for _, ep := range m.episodes {
		found := false
		for _, id := range ids {
			if ep.EpisodeID == id {
				found = true
				break
			}
		}
		if !found {
			newEpisodes = append(newEpisodes, ep)
		}
	}
	m.episodes = newEpisodes
	return nil
}

func (m *mockFileManager) DeleteEmptyFolders(savePath string, completedAnimeSaveFolder string) error {
	return nil
}

func TestHandleGetConfig(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleGetConfig(server)

	t.Run("GET returns config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
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

		config, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be an object")
		}

		if username, ok := config["anilist_username"].(string); !ok || username != "testuser" {
			t.Errorf("Expected anilist_username 'testuser', got %v", config["anilist_username"])
		}
	})
}

func TestHandleUpdateConfig(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleUpdateConfig(server)

	t.Run("PUT with valid config succeeds", func(t *testing.T) {
		config := files.Config{
			AnilistUsername:       "newuser",
			SavePath:              "/tmp/newpath",
			CompletedAnimePath:    "/tmp/newcompleted",
			CheckInterval:         15,
			QBittorrentUrl:        "http://localhost:8080",
			MaxEpisodesPerAnime:   20,
			EpisodeRetryLimit:     3,
			DeleteWatchedEpisodes: false,
			ExcludedList:          "test",
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
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
	})

	t.Run("PUT with missing anilist_username returns 400", func(t *testing.T) {
		config := files.Config{
			SavePath:           "/tmp/test",
			QBittorrentUrl:     "http://localhost:8080",
			CheckInterval:      10,
			MaxEpisodesPerAnime: 12,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response.Success {
			t.Error("Expected success=false")
		}

		if response.Error == nil || response.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("Expected VALIDATION_ERROR, got %v", response.Error)
		}
	})

	t.Run("PUT with invalid check_interval returns 400", func(t *testing.T) {
		config := files.Config{
			AnilistUsername:       "testuser",
			SavePath:              "/tmp/test",
			QBittorrentUrl:        "http://localhost:8080",
			CheckInterval:         0,
			MaxEpisodesPerAnime:   12,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("PUT with invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestHandleConfig(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleConfig(server)

	t.Run("GET method calls handleGetConfig", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("PUT method calls handleUpdateConfig", func(t *testing.T) {
		config := files.Config{
			AnilistUsername:       "testuser",
			SavePath:              "/tmp/test",
			QBittorrentUrl:        "http://localhost:8080",
			CheckInterval:         10,
			MaxEpisodesPerAnime:   12,
			EpisodeRetryLimit:     5,
			DeleteWatchedEpisodes: true,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Unsupported method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/config", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}
