package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
)

type debugMockFileManager struct{}

func (m *debugMockFileManager) LoadConfigs() (*files.Config, error)                     { return nil, nil }
func (m *debugMockFileManager) SaveConfigs(config *files.Config) error                  { return nil }
func (m *debugMockFileManager) LoadSavedEpisodes() ([]files.EpisodeStruct, error)       { return nil, nil }
func (m *debugMockFileManager) SaveEpisodesToFile(episodes []files.EpisodeStruct) error { return nil }
func (m *debugMockFileManager) DeleteEpisodesFromFile(ids []int) error                  { return nil }
func (m *debugMockFileManager) DeleteEmptyFolders(a, b string) error                    { return nil }
func (m *debugMockFileManager) LoadBlockedEpisodes() ([]int, error)                     { return nil, nil }
func (m *debugMockFileManager) BlockEpisode(id int) error                               { return nil }
func (m *debugMockFileManager) UnblockEpisode(id int) error                             { return nil }
func (m *debugMockFileManager) UnmanageEpisode(id int) error                            { return nil }
func (m *debugMockFileManager) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return nil, nil
}
func (m *debugMockFileManager) LoadAnimeSettings(id int) (*files.AnimeSettings, error) {
	return &files.AnimeSettings{}, nil
}
func (m *debugMockFileManager) SaveAnimeSettings(id int, s files.AnimeSettings) error { return nil }

func TestRunAnimeDebug_NoNyaaResults_NoError(t *testing.T) {
	anilistJSON := `{"data": {"MediaList": {"id": 1, "status": "CURRENT", "progress": 0, "media": {
		"episodes": 12, "format": "TV", "status": "RELEASING",
		"title": {"english": "My Anime", "romaji": "Boku no Anime"},
		"synonyms": [], "relations": {"edges": []},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [{"id": 10, "episode": 1, "timeUntilAiring": 0}]}
	}}}}`
	restoreAnilist := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restoreAnilist()

	restoreNyaa := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<html><body><table class=\"torrent-list\"><tbody></tbody></table></body></html>")), Header: make(http.Header)}, nil
	})
	defer restoreNyaa()

	summary, err := RunAnimeDebug(1, &files.Config{MaxEpisodesPerAnime: 12, EpisodeRetryLimit: 3}, &debugMockFileManager{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if summary == nil {
		t.Fatal("expected a non-nil summary")
	}
	if summary.AnimeID != 1 {
		t.Errorf("expected AnimeID 1, got %d", summary.AnimeID)
	}
	if len(summary.Episodes) != 1 {
		t.Fatalf("expected 1 episode in summary, got %d", len(summary.Episodes))
	}
	if !summary.Episodes[0].WouldSearch {
		t.Error("expected episode 1 to be marked WouldSearch=true (it aired)")
	}
	if summary.Episodes[0].MagnetsFound != 0 {
		t.Errorf("expected 0 magnets found, got %d", summary.Episodes[0].MagnetsFound)
	}
}

func TestNextDebugDir(t *testing.T) {
	tmpDir := t.TempDir()

	dir, err := NextDebugDir(tmpDir, 123)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dir != ".debug_123_1" {
		t.Errorf("expected .debug_123_1 for empty dir, got %q", dir)
	}

	// Create _1 and _2 for anime 123, and _1 for a different anime.
	if err := os.Mkdir(filepath.Join(tmpDir, ".debug_123_1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".debug_123_2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".debug_999_1"), 0755); err != nil {
		t.Fatal(err)
	}

	dir, err = NextDebugDir(tmpDir, 123)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dir != ".debug_123_3" {
		t.Errorf("expected .debug_123_3, got %q", dir)
	}

	// A different anime ID is unaffected by 123's existing dirs.
	dir, err = NextDebugDir(tmpDir, 999)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dir != ".debug_999_2" {
		t.Errorf("expected .debug_999_2, got %q", dir)
	}
}

func TestWriteDebugSummary(t *testing.T) {
	tmpDir := t.TempDir()
	summary := &DebugSummary{
		AnimeID:   123,
		AnimeName: "Test Anime",
		Episodes: []EpisodeDebugResult{
			{Episode: 1, WouldSearch: true, MagnetsFound: 2},
		},
	}

	if err := WriteDebugSummary(tmpDir, summary); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "summary.json"))
	if err != nil {
		t.Fatalf("expected summary.json to exist: %v", err)
	}

	var got DebugSummary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if got.AnimeID != 123 || got.AnimeName != "Test Anime" {
		t.Errorf("unexpected summary contents: %+v", got)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].MagnetsFound != 2 {
		t.Errorf("unexpected episodes: %+v", got.Episodes)
	}
}
