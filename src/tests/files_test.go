package tests

import (
	"AutoAnimeDownloader/src/internal/files"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempManager(t *testing.T, fn func(*files.FileManager)) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "aad_test_home_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	configsFolder := filepath.Join(tmp, ".autoAnimeDownloader")
	if err := os.MkdirAll(configsFolder, 0755); err != nil {
		t.Fatalf("failed to create configs folder: %v", err)
	}

	configPath := filepath.Join(configsFolder, ".config.json")
	episodesPath := filepath.Join(configsFolder, ".downloaded_episodes")

	fs := files.NewOSFileSystem()
	manager := files.NewManager(fs, configPath, episodesPath)

	fn(manager)
}

func TestFilesModule_CanSaveLoadAndDeleteEpisodes(t *testing.T) {
	withTempManager(t, func(manager *files.FileManager) {
		// initially no episodes
		eps, err := manager.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(eps) != 0 {
			t.Fatalf("expected 0 episodes, got %d", len(eps))
		}

		// save some episodes
		toSave := []files.EpisodeStruct{
			{EpisodeID: 1, EpisodeHash: "h1", EpisodeName: "Name1"},
			{EpisodeID: 2, EpisodeHash: "h2", EpisodeName: ""},
		}
		if err := manager.SaveEpisodesToFile(toSave); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := manager.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(loaded) != 2 {
			t.Fatalf("expected 2 episodes after save, got %d", len(loaded))
		}

		// delete one
		if err := manager.DeleteEpisodesFromFile([]int{1}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		afterDel, err := manager.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(afterDel) != 1 {
			t.Fatalf("expected 1 episode after delete, got %d", len(afterDel))
		}
		if afterDel[0].EpisodeID != 2 {
			t.Fatalf("expected remaining episode id 2, got %d", afterDel[0].EpisodeID)
		}
	})
}

func TestFilesModule_CanHandleDeleteEpisodes_WithNonExistentIDs(t *testing.T) {
	withTempManager(t, func(manager *files.FileManager) {
		// create file with one episode
		if err := manager.SaveEpisodesToFile([]files.EpisodeStruct{{EpisodeID: 10, EpisodeHash: "hh"}}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// delete non-existing id -> should be noop and not panic
		if err := manager.DeleteEpisodesFromFile([]int{999}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		loaded, err := manager.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(loaded) != 1 || loaded[0].EpisodeID != 10 {
			t.Fatalf("expected original episode to remain, got: %#v", loaded)
		}
	})
}

func TestFilesModule_CanDeleteEmptyFolders_WithMixedFolders(t *testing.T) {
	withTempManager(t, func(manager *files.FileManager) {
		tmp, err := os.MkdirTemp("", "save_test_")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmp)

		saveRoot := filepath.Join(tmp, "save-path")
		if err := os.MkdirAll(saveRoot, 0755); err != nil {
			t.Fatalf("mkdir save root: %v", err)
		}

		// create empty folder and non-empty folder
		empty := filepath.Join(saveRoot, "empty")
		nonEmpty := filepath.Join(saveRoot, "full")
		if err := os.MkdirAll(empty, 0755); err != nil {
			t.Fatalf("mkdir empty: %v", err)
		}
		if err := os.MkdirAll(nonEmpty, 0755); err != nil {
			t.Fatalf("mkdir nonEmpty: %v", err)
		}
		// create file inside nonEmpty
		f, err := os.Create(filepath.Join(nonEmpty, "f.txt"))
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		_ = f.Close()

		if err := manager.DeleteEmptyFolders(saveRoot, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// empty should be removed
		if _, err := os.Stat(empty); !os.IsNotExist(err) {
			t.Fatalf("expected empty folder removed, stat err: %v", err)
		}
		// nonEmpty should remain
		if _, err := os.Stat(nonEmpty); err != nil {
			t.Fatalf("expected non-empty folder to remain, stat err: %v", err)
		}
	})
}

func TestFilesModule_CanLoadAndSaveConfigs_WithDefaults(t *testing.T) {
	withTempManager(t, func(manager *files.FileManager) {
		cfg, err := manager.LoadConfigs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// defaults from implementation
		if cfg.CheckInterval != 10 {
			t.Fatalf("expected CheckInterval 10, got %d", cfg.CheckInterval)
		}
		if cfg.QBittorrentUrl != "http://127.0.0.1:8080" {
			t.Fatalf("unexpected default QBittorrentUrl: %s", cfg.QBittorrentUrl)
		}

		// modify and save
		cfg.CheckInterval = 42
		if err := manager.SaveConfigs(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// load again and verify saved value
		cfg2, err := manager.LoadConfigs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg2.CheckInterval != 42 {
			t.Fatalf("expected CheckInterval 42 after save, got %d", cfg2.CheckInterval)
		}
	})
}

// ============================================================================
// NOVOS TESTES COM MOCKS
// ============================================================================

func TestManager_LoadConfigs_WithNonExistentFile(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	config, err := manager.LoadConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deve retornar configuração padrão
	if config.CheckInterval != 10 {
		t.Errorf("expected default CheckInterval 10, got %d", config.CheckInterval)
	}
	if config.QBittorrentUrl != "http://127.0.0.1:8080" {
		t.Errorf("expected default QBittorrentUrl, got %s", config.QBittorrentUrl)
	}

	// Deve ter salvado a configuração padrão
	if !mockFS.FileExists("/config.json") {
		t.Error("expected config file to be created")
	}
}

func TestManager_LoadConfigs_WithExistingFile(t *testing.T) {
	mockFS := NewMockFileSystem()
	configJSON := `{
		"save_path": "/anime",
		"anilist_username": "testuser",
		"check_interval": 20,
		"qbittorrent_url": "http://localhost:9090",
		"max_episodes_per_animes": 5,
		"episode_retry_limit": 3,
		"delete_watched_episodes": false,
		"excluded_list": "test"
	}`
	mockFS.SetFile("/config.json", []byte(configJSON))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	config, err := manager.LoadConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.SavePath != "/anime" {
		t.Errorf("expected SavePath '/anime', got '%s'", config.SavePath)
	}
	if config.AnilistUsername != "testuser" {
		t.Errorf("expected AnilistUsername 'testuser', got '%s'", config.AnilistUsername)
	}
	if config.CheckInterval != 20 {
		t.Errorf("expected CheckInterval 20, got %d", config.CheckInterval)
	}
}

func TestManager_SaveConfigs_WithValidConfig(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	config := &files.Config{
		SavePath:              "/test",
		AnilistUsername:       "user123",
		CheckInterval:         15,
		QBittorrentUrl:        "http://test:8080",
		MaxEpisodesPerAnime:   10,
		EpisodeRetryLimit:     5,
		DeleteWatchedEpisodes: true,
		ExcludedList:          "excluded",
	}

	err := manager.SaveConfigs(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verificar que o arquivo foi escrito
	if !mockFS.FileExists("/config.json") {
		t.Fatal("expected config file to exist")
	}

	content, _ := mockFS.GetFile("/config.json")
	contentStr := string(content)

	// Verificar alguns campos no JSON
	if !strings.Contains(contentStr, `"save_path": "/test"`) {
		t.Error("expected save_path in JSON")
	}
	if !strings.Contains(contentStr, `"anilist_username": "user123"`) {
		t.Error("expected anilist_username in JSON")
	}
}

func TestManager_SaveConfigs_WithNilConfig(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.SaveConfigs(nil)
	if err == nil {
		t.Fatal("expected error when saving nil config")
	}

	if !strings.Contains(err.Error(), "cannot be nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_LoadSavedEpisodes_WithNonExistentFile(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	episodes, err := manager.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(episodes) != 0 {
		t.Errorf("expected empty episodes list, got %d episodes", len(episodes))
	}
}

func TestManager_LoadSavedEpisodes_WithValidContent(t *testing.T) {
	mockFS := NewMockFileSystem()
	content := "1:hash1:Episode 1\n2:hash2:Episode 2\n3:hash3\n"
	mockFS.SetFile("/episodes.txt", []byte(content))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	episodes, err := manager.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(episodes) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(episodes))
	}

	if episodes[0].EpisodeID != 1 || episodes[0].EpisodeHash != "hash1" || episodes[0].EpisodeName != "Episode 1" {
		t.Errorf("unexpected episode[0]: %+v", episodes[0])
	}
	if episodes[1].EpisodeID != 2 || episodes[1].EpisodeHash != "hash2" || episodes[1].EpisodeName != "Episode 2" {
		t.Errorf("unexpected episode[1]: %+v", episodes[1])
	}
	if episodes[2].EpisodeID != 3 || episodes[2].EpisodeHash != "hash3" || episodes[2].EpisodeName != "" {
		t.Errorf("unexpected episode[2]: %+v", episodes[2])
	}
}

func TestManager_SaveEpisodesToFile_WithNewEpisodes(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	episodes := []files.EpisodeStruct{
		{EpisodeID: 1, EpisodeHash: "abc", EpisodeName: "Test Episode"},
		{EpisodeID: 2, EpisodeHash: "def", EpisodeName: ""},
	}

	err := manager.SaveEpisodesToFile(episodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verificar que o arquivo foi criado
	if !mockFS.FileExists("/episodes.txt") {
		t.Fatal("expected episodes file to exist")
	}
}

func TestManager_SaveEpisodesToFile_WithEmptyList(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.SaveEpisodesToFile([]files.EpisodeStruct{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Não deve criar arquivo para lista vazia
	if mockFS.FileExists("/episodes.txt") {
		t.Error("did not expect episodes file to exist for empty list")
	}
}

func TestManager_DeleteEpisodesFromFile_WithValidIds(t *testing.T) {
	mockFS := NewMockFileSystem()
	content := "1:hash1:Ep1\n2:hash2:Ep2\n3:hash3:Ep3\n"
	mockFS.SetFile("/episodes.txt", []byte(content))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.DeleteEpisodesFromFile([]int{2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Carregar e verificar que apenas episódios 1 e 3 permanecem
	episodes, err := manager.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("unexpected error loading after delete: %v", err)
	}

	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes after delete, got %d", len(episodes))
	}

	if episodes[0].EpisodeID != 1 {
		t.Errorf("expected first episode ID 1, got %d", episodes[0].EpisodeID)
	}
	if episodes[1].EpisodeID != 3 {
		t.Errorf("expected second episode ID 3, got %d", episodes[1].EpisodeID)
	}
}

func TestManager_DeleteEpisodesFromFile_WithNonExistentIds(t *testing.T) {
	mockFS := NewMockFileSystem()
	content := "1:hash1:Ep1\n"
	mockFS.SetFile("/episodes.txt", []byte(content))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.DeleteEpisodesFromFile([]int{999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verificar que o episódio original ainda está lá
	episodes, err := manager.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("unexpected error loading after delete: %v", err)
	}

	if len(episodes) != 1 || episodes[0].EpisodeID != 1 {
		t.Errorf("expected original episode to remain: %+v", episodes)
	}
}

func TestManager_DeleteEpisodesFromFile_WithEmptyList(t *testing.T) {
	mockFS := NewMockFileSystem()
	content := "1:hash1:Ep1\n"
	mockFS.SetFile("/episodes.txt", []byte(content))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.DeleteEpisodesFromFile([]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Conteúdo não deve ter mudado
	newContent, _ := mockFS.GetFile("/episodes.txt")
	if string(newContent) != content {
		t.Error("expected content to remain unchanged")
	}
}

func TestManager_DeleteEmptyFolders_WithEmptyFolder(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.SetDir("/save")
	mockFS.SetDir("/save/empty")
	mockFS.SetDir("/save/nonempty")
	mockFS.SetFile("/save/nonempty/file.txt", []byte("content"))

	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.DeleteEmptyFolders("/save", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Pasta vazia deve ter sido removida
	if mockFS.DirExists("/save/empty") {
		t.Error("expected empty folder to be deleted")
	}

	// Pasta não vazia deve permanecer
	if !mockFS.DirExists("/save/nonempty") {
		t.Error("expected non-empty folder to remain")
	}
}

func TestManager_DeleteEmptyFolders_WithEmptySavePath(t *testing.T) {
	mockFS := NewMockFileSystem()
	manager := files.NewManager(mockFS, "/config.json", "/episodes.txt")

	err := manager.DeleteEmptyFolders("", "")
	if err == nil {
		t.Fatal("expected error with empty save path")
	}

	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================================
// TESTES PARA FUNÇÕES DE PARSING
// ============================================================================

func TestParseEpisodes_WithValidContent(t *testing.T) {
	content := "1:hash1:Episode 1\n2:hash2:Episode 2\n3:hash3\n"

	episodes, err := files.ParseEpisodes(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(episodes) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(episodes))
	}

	// Verificar primeiro episódio
	if episodes[0].EpisodeID != 1 {
		t.Errorf("expected episode ID 1, got %d", episodes[0].EpisodeID)
	}
	if episodes[0].EpisodeHash != "hash1" {
		t.Errorf("expected hash 'hash1', got '%s'", episodes[0].EpisodeHash)
	}
	if episodes[0].EpisodeName != "Episode 1" {
		t.Errorf("expected name 'Episode 1', got '%s'", episodes[0].EpisodeName)
	}

	// Verificar terceiro episódio (sem nome)
	if episodes[2].EpisodeID != 3 {
		t.Errorf("expected episode ID 3, got %d", episodes[2].EpisodeID)
	}
	if episodes[2].EpisodeName != "" {
		t.Errorf("expected empty name, got '%s'", episodes[2].EpisodeName)
	}
}

func TestParseEpisodes_WithEmptyContent(t *testing.T) {
	episodes, err := files.ParseEpisodes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(episodes) != 0 {
		t.Errorf("expected empty list, got %d episodes", len(episodes))
	}
}

func TestParseEpisodes_WithBlankLines(t *testing.T) {
	content := "1:hash1:Ep1\n\n2:hash2:Ep2\n   \n3:hash3\n"

	episodes, err := files.ParseEpisodes(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(episodes) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(episodes))
	}
}

func TestParseEpisodes_WithInvalidFormat(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"missing hash", "1\n"},
		{"missing colon", "1hash1\n"},
		{"invalid id", "abc:hash1\n"},
		{"empty hash", "1::name\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := files.ParseEpisodes(tc.content)
			if err == nil {
				t.Errorf("expected error for invalid format: %s", tc.name)
			}
		})
	}
}

func TestSerializeEpisode_WithName(t *testing.T) {
	episode := files.EpisodeStruct{
		EpisodeID:   42,
		EpisodeHash: "testhash",
		EpisodeName: "Test Name",
	}

	result := files.SerializeEpisode(episode)
	expected := "42:testhash:Test Name\n"

	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestSerializeEpisode_WithoutName(t *testing.T) {
	episode := files.EpisodeStruct{
		EpisodeID:   42,
		EpisodeHash: "testhash",
		EpisodeName: "",
	}

	result := files.SerializeEpisode(episode)
	expected := "42:testhash\n"

	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestSerializeEpisodes_WithMultipleEpisodes(t *testing.T) {
	episodes := []files.EpisodeStruct{
		{EpisodeID: 1, EpisodeHash: "h1", EpisodeName: "Name1"},
		{EpisodeID: 2, EpisodeHash: "h2", EpisodeName: ""},
		{EpisodeID: 3, EpisodeHash: "h3", EpisodeName: "Name3"},
	}

	result := files.SerializeEpisodes(episodes)
	expected := "1:h1:Name1\n2:h2\n3:h3:Name3\n"

	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestSerializeEpisodes_WithEmptyList(t *testing.T) {
	result := files.SerializeEpisodes([]files.EpisodeStruct{})
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

// Test round-trip: parse then serialize should give same result
func TestParseSerialize_RoundTrip(t *testing.T) {
	original := "1:hash1:Episode 1\n2:hash2\n3:hash3:Episode 3\n"

	episodes, err := files.ParseEpisodes(original)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	serialized := files.SerializeEpisodes(episodes)

	if serialized != original {
		t.Errorf("round-trip failed:\noriginal:   '%s'\nserialized: '%s'", original, serialized)
	}
}
