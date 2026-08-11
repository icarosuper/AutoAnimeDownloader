package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubLibrarian is a files.Librarian for testing config validation (ProbePath behavior).
type stubLibrarian struct {
	probeErr error
}

func (s *stubLibrarian) Organize(files.OrganizeRequest) ([]string, error) { return nil, nil }
func (s *stubLibrarian) RemoveFromLibrary(string) error                   { return nil }
func (s *stubLibrarian) ProbePath(completedPath string) error             { return s.probeErr }

type mockFileManager struct {
	configs           *files.Config
	episodes          []files.EpisodeStruct
	blockedEpisodes   []files.EpisodeKey
	standaloneAnimes  []int
	addStandaloneErr  error
	loadErr           error
	saveErr           error
	loadEpisodesErr   error
	saveEpisodesErr   error
	deleteEpisodesErr error
}

func (m *mockFileManager) LoadConfigs() (*files.Config, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.configs == nil {
		return &files.Config{
			AnilistUsernames:      []string{"testuser"},
			SavePath:              "/tmp/test",
			CompletedAnimePath:    "/tmp/completed",
			CheckInterval:         10,
			MaxEpisodesPerAnime:   12,
			EpisodeRetryLimit:     5,
			DeleteWatchedEpisodes: true,
			ExcludedLists:         []string{},
			DownloadStatuses:      []string{"CURRENT", "REPEATING"},
			DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			DeleteStatuses:        []string{},
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

// UpsertEpisodes atualiza em lugar (casando por EpisodeID) e acrescenta o que e novo, como o
// FileManager de verdade — um mock que simplesmente substituisse a lista faria um teste de
// "marcar episodio" passar apagando todos os outros.
func (m *mockFileManager) UpsertEpisodes(episodes []files.EpisodeStruct) error {
	if m.saveEpisodesErr != nil {
		return m.saveEpisodesErr
	}
	for _, updated := range episodes {
		found := false
		for i, existing := range m.episodes {
			if existing.EpisodeNumber == updated.EpisodeNumber {
				m.episodes[i] = updated
				found = true
				break
			}
		}
		if !found {
			m.episodes = append(m.episodes, updated)
		}
	}
	return nil
}

func (m *mockFileManager) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	if m.deleteEpisodesErr != nil {
		return m.deleteEpisodesErr
	}
	// Simple implementation for testing
	newEpisodes := []files.EpisodeStruct{}
	for _, ep := range m.episodes {
		found := false
		for _, k := range keys {
			if ep.Key() == k {
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

func (m *mockFileManager) DeleteEmptyFolders(completedAnimeSaveFolder string) error {
	return nil
}

func (m *mockFileManager) LoadBlockedEpisodes() ([]files.EpisodeKey, error) {
	return m.blockedEpisodes, nil
}

func (m *mockFileManager) BlockEpisode(key files.EpisodeKey) error {
	m.blockedEpisodes = append(m.blockedEpisodes, key)
	return nil
}

func (m *mockFileManager) UnblockEpisode(key files.EpisodeKey) error {
	var filtered []files.EpisodeKey
	for _, k := range m.blockedEpisodes {
		if k != key {
			filtered = append(filtered, k)
		}
	}
	m.blockedEpisodes = filtered
	return nil
}

func (m *mockFileManager) UnmanageEpisode(key files.EpisodeKey) error {
	for i, ep := range m.episodes {
		if ep.Key() == key {
			m.episodes[i].ManuallyManaged = false
			break
		}
	}
	return nil
}

func (m *mockFileManager) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return map[int]files.AnimeSettings{}, nil
}

func (m *mockFileManager) LoadAnimeSettings(animeID int) (*files.AnimeSettings, error) {
	return &files.AnimeSettings{}, nil
}

func (m *mockFileManager) SaveAnimeSettings(animeID int, settings files.AnimeSettings) error {
	return nil
}

func (m *mockFileManager) LoadStandaloneAnimes() ([]int, error) {
	return m.standaloneAnimes, nil
}

func (m *mockFileManager) AddStandaloneAnime(mediaID int) error {
	if m.addStandaloneErr != nil {
		return m.addStandaloneErr
	}
	for _, id := range m.standaloneAnimes {
		if id == mediaID {
			return nil
		}
	}
	m.standaloneAnimes = append(m.standaloneAnimes, mediaID)
	return nil
}

func (m *mockFileManager) RemoveStandaloneAnime(mediaID int) error {
	var filtered []int
	for _, id := range m.standaloneAnimes {
		if id != mediaID {
			filtered = append(filtered, id)
		}
	}
	m.standaloneAnimes = filtered
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

		usernames, ok := config["anilist_usernames"].([]interface{})
		if !ok || len(usernames) == 0 || usernames[0].(string) != "testuser" {
			t.Errorf("Expected anilist_usernames ['testuser'], got %v", config["anilist_usernames"])
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
			AnilistUsernames:      []string{"newuser"},
			SavePath:              "/tmp/newpath",
			CompletedAnimePath:    "/tmp/newcompleted",
			CheckInterval:         15,
			MaxEpisodesPerAnime:   20,
			EpisodeRetryLimit:     3,
			DeleteWatchedEpisodes: false,
			ExcludedLists:         []string{"test"},
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

	t.Run("PUT applies max_concurrent_downloads to the backend right away", func(t *testing.T) {
		// Sem isso, mudar o limite so valeria no proximo passe de verificacao (10 min por
		// padrao) e o campo leria como quebrado.
		backend := torrents.NewFakeBackend()
		srv := &Server{State: state, FileManager: mockFM, Torrents: backend}

		config := files.Config{
			AnilistUsernames:       []string{"newuser"},
			CompletedAnimePath:     "/tmp/newcompleted",
			CheckInterval:          15,
			MaxEpisodesPerAnime:    20,
			EpisodeRetryLimit:      3,
			MaxConcurrentDownloads: 7,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		w := httptest.NewRecorder()
		handleUpdateConfig(srv)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}
		if backend.MaxActiveDownloads != 7 {
			t.Errorf("MaxActiveDownloads = %d, want 7", backend.MaxActiveDownloads)
		}
	})

	t.Run("PUT with negative max_concurrent_downloads returns 400", func(t *testing.T) {
		config := files.Config{
			AnilistUsernames:       []string{"newuser"},
			CompletedAnimePath:     "/tmp/newcompleted",
			CheckInterval:          15,
			MaxEpisodesPerAnime:    20,
			MaxConcurrentDownloads: -1,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("PUT with missing anilist_username returns 400", func(t *testing.T) {
		config := files.Config{
			SavePath:            "/tmp/test",
			CheckInterval:       10,
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
			AnilistUsernames:    []string{"testuser"},
			SavePath:            "/tmp/test",
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       0,
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
	})

	t.Run("PUT with missing completed_anime_path returns 400", func(t *testing.T) {
		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			SavePath:            "/tmp/test",
			CheckInterval:       10,
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
	})

	t.Run("PUT with cross-device paths returns 400", func(t *testing.T) {
		crossDeviceServer := &Server{
			State:       state,
			FileManager: mockFM,
			Librarian:   &stubLibrarian{probeErr: fmt.Errorf("save path and completed path are on different volumes")},
		}
		crossHandler := handleUpdateConfig(crossDeviceServer)

		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			SavePath:            "/vol1/save",
			CompletedAnimePath:  "/vol2/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		crossHandler(w, req)

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
			AnilistUsernames:      []string{"testuser"},
			SavePath:              "/tmp/test",
			CompletedAnimePath:    "/tmp/completed",
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

func TestHandleUpdateConfig_SavePath(t *testing.T) {
	// save_path deixou de ser exigido: o diretorio de download e derivado da biblioteca.
	t.Run("PUT sem save_path e aceito", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
		}
	})

	// A vedacao: um cliente antigo mandando save_path nao pode reintroduzir o campo, senao
	// daemon.MigrateSavePath re-arma a cada boot.
	t.Run("PUT com save_path persiste o campo vazio", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			SavePath:            "/tmp/legado",
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
		}
		if mockFM.configs == nil {
			t.Fatal("o config nao foi persistido")
		}
		if mockFM.configs.SavePath != "" {
			t.Errorf("SavePath deveria ter sido zerado, veio %q", mockFM.configs.SavePath)
		}
	})
}

// TestHandleUpdateConfig_NoAniListAccount: conta da AniList deixou de ser obrigatoria. Com
// animes avulsos o app funciona inteiro sem lista nenhuma; a biblioteca continua obrigatoria,
// porque sem ela nao ha para onde baixar.
func TestHandleUpdateConfig_NoAniListAccount(t *testing.T) {
	t.Run("PUT sem anilist_usernames e aceito", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{},
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("quero 200 sem conta da AniList, veio %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("PUT sem completed_anime_path continua 400", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{},
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("quero 400 sem biblioteca, veio %d", w.Code)
		}
	})
}
