package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
func (m *debugMockFileManager) UpsertEpisodes(episodes []files.EpisodeStruct) error     { return nil }
func (m *debugMockFileManager) DeleteEpisodesFromFile(keys []files.EpisodeKey) error    { return nil }
func (m *debugMockFileManager) DeleteEmptyFolders(a string) error                       { return nil }
func (m *debugMockFileManager) LoadBlockedEpisodes() ([]files.EpisodeKey, error)        { return nil, nil }
func (m *debugMockFileManager) BlockEpisode(key files.EpisodeKey) error                 { return nil }
func (m *debugMockFileManager) UnblockEpisode(key files.EpisodeKey) error               { return nil }
func (m *debugMockFileManager) UnmanageEpisode(key files.EpisodeKey) error              { return nil }
func (m *debugMockFileManager) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return nil, nil
}
func (m *debugMockFileManager) LoadAnimeSettings(id int) (*files.AnimeSettings, error) {
	return &files.AnimeSettings{}, nil
}
func (m *debugMockFileManager) SaveAnimeSettings(id int, s files.AnimeSettings) error { return nil }
func (m *debugMockFileManager) LoadStandaloneAnimes() ([]int, error)                  { return nil, nil }
func (m *debugMockFileManager) AddStandaloneAnime(int) error                          { return nil }
func (m *debugMockFileManager) RemoveStandaloneAnime(int) error                       { return nil }

func TestRunAnimeDebug_NoNyaaResults_NoError(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [{"id": 1, "status": "CURRENT", "progress": 0, "media": {
		"id": 21, "episodes": 12, "format": "TV", "status": "RELEASING",
		"title": {"english": "My Anime", "romaji": "Boku no Anime"},
		"synonyms": [], "relations": {"edges": []},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [{"id": 10, "episode": 1, "timeUntilAiring": 0}]}
	}}]}}}`
	restoreAnilist := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restoreAnilist()

	restoreNyaa := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<html><body><table class=\"torrent-list\"><tbody></tbody></table></body></html>")), Header: make(http.Header)}, nil
	})
	defer restoreNyaa()

	summary, err := RunAnimeDebug(21, &files.Config{MaxEpisodesPerAnime: 12, EpisodeRetryLimit: 3, AnilistUsernames: []string{"user"}}, &debugMockFileManager{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if summary == nil {
		t.Fatal("expected a non-nil summary")
	}
	if summary.AnimeID != 21 {
		t.Errorf("expected AnimeID 21, got %d", summary.AnimeID)
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

// debugAnilistJSON builds a one-media AniList page response with an airing schedule covering
// firstNode..lastNode (all aired). firstNode == 0 means an empty schedule.
func debugAnilistJSON(status string, totalEpisodes, firstNode, lastNode int) string {
	var nodes []string
	for ep := firstNode; ep >= 1 && ep <= lastNode; ep++ {
		nodes = append(nodes, fmt.Sprintf(`{"id": %d, "episode": %d, "timeUntilAiring": -1}`, ep, ep))
	}
	return fmt.Sprintf(`{"data": {"Page": {"mediaList": [{"id": 1, "status": "CURRENT", "progress": 0, "media": {
		"id": 21, "episodes": %d, "format": "TV", "status": %q,
		"title": {"english": "My Anime", "romaji": "Boku no Anime"},
		"synonyms": [], "relations": {"edges": []},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [%s]}
	}}]}}}`, totalEpisodes, status, strings.Join(nodes, ","))
}

// runDebugWithMocks runs RunAnimeDebug against a stubbed AniList response and an empty Nyaa.
func runDebugWithMocks(t *testing.T, anilistJSON string, configs *files.Config) *DebugSummary {
	t.Helper()

	restoreAnilist := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restoreAnilist()

	restoreNyaa := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<html><body><table class=\"torrent-list\"><tbody></tbody></table></body></html>")), Header: make(http.Header)}, nil
	})
	defer restoreNyaa()

	summary, err := RunAnimeDebug(21, configs, &debugMockFileManager{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	return summary
}

func countWouldSearch(summary *DebugSummary) int {
	n := 0
	for _, ep := range summary.Episodes {
		if ep.WouldSearch {
			n++
		}
	}
	return n
}

// A agenda da AniList e clipada (One Piece: pagina 1 vai do 1123 ao 1147), e a producao enumera
// a partir do progresso+1. O debug tem que fazer o mesmo, senao reporta ter buscado o 1123.
func TestRunAnimeDebug_ClippedSchedule_StartsAtEpisodeOne(t *testing.T) {
	summary := runDebugWithMocks(t, debugAnilistJSON("RELEASING", 1147, 1123, 1147),
		&files.Config{MaxEpisodesPerAnime: 12, MaxBatchEpisodes: 30, EpisodeRetryLimit: 3, AnilistUsernames: []string{"user"}})

	if len(summary.Episodes) != 1147 {
		t.Fatalf("expected 1147 episodes enumerated, got %d", len(summary.Episodes))
	}
	if summary.Episodes[0].Episode != 1 {
		t.Errorf("expected enumeration to start at episode 1, got %d", summary.Episodes[0].Episode)
	}
	if got := countWouldSearch(summary); got != 12 {
		t.Errorf("expected the per-anime limit (12) to apply, got %d searched", got)
	}
}

// FINISHED dentro de max_batch_episodes vira batch, e em batch o teto por anime nao se aplica.
func TestRunAnimeDebug_BatchAnime_IgnoresPerAnimeLimit(t *testing.T) {
	summary := runDebugWithMocks(t, debugAnilistJSON("FINISHED", 26, 0, 0),
		&files.Config{MaxEpisodesPerAnime: 12, MaxBatchEpisodes: 30, EpisodeRetryLimit: 3, AnilistUsernames: []string{"user"}})

	if len(summary.Episodes) != 26 {
		t.Fatalf("expected 26 episodes enumerated, got %d", len(summary.Episodes))
	}
	if got := countWouldSearch(summary); got != 26 {
		t.Errorf("expected all 26 episodes searched (batch disables the limit), got %d", got)
	}
}

// O par do anterior: acima de max_batch_episodes willBatch e falsa e o teto volta a valer.
func TestRunAnimeDebug_FinishedAboveBatchLimit_KeepsPerAnimeLimit(t *testing.T) {
	summary := runDebugWithMocks(t, debugAnilistJSON("FINISHED", 500, 0, 0),
		&files.Config{MaxEpisodesPerAnime: 12, MaxBatchEpisodes: 30, EpisodeRetryLimit: 3, AnilistUsernames: []string{"user"}})

	if len(summary.Episodes) != 500 {
		t.Fatalf("expected 500 episodes enumerated, got %d", len(summary.Episodes))
	}
	if got := countWouldSearch(summary); got != 12 {
		t.Errorf("expected the per-anime limit (12) to apply, got %d searched", got)
	}
}

// Numa serie longa a busca multipla nao leva numero de episodio na query e volta vazia; quem
// acha o episodio e searchSingleEpisode (com zero-padding, decisions.md #56). Sem esse fallback
// o debug reportava "0 magnets" por nao ter buscado — e nao por o Nyaa nao ter.
func TestRunAnimeDebug_FallsBackToSingleEpisodeSearch(t *testing.T) {
	// So a query que carrega o numero do episodio devolve linha; a busca multipla (titulo puro)
	// volta vazia, como no One Piece real.
	epInQuery := regexp.MustCompile(`q=my\+anime\+0*(\d+)`)
	restoreNyaa := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		body := `<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`
		if m := epInQuery.FindStringSubmatch(url); m != nil {
			body = fmt.Sprintf(`<!doctype html><html><body><table class="torrent-list"><tbody><tr>
			  <td></td>
			  <td><a title="[Sub] My Anime - %s [1080p]">[Sub] My Anime - %s [1080p]</a></td>
			  <td><a></a><a href="magnet:?xt=urn:btih:abc%s">magnet</a></td>
			  <td>1.2 GiB</td>
			  <td>2020-01-01 10:00</td>
			  <td>100</td>
			</tr></tbody></table></body></html>`, m[1], m[1], m[1])
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	defer restoreNyaa()

	restoreAnilist := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		json := debugAnilistJSON("RELEASING", 1147, 1123, 1147)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(json)), Header: make(http.Header)}, nil
	})
	defer restoreAnilist()

	summary, err := RunAnimeDebug(21, &files.Config{MaxEpisodesPerAnime: 3, MaxBatchEpisodes: 30, EpisodeRetryLimit: 3, MinSeeders: 1, AnilistUsernames: []string{"user"}}, &debugMockFileManager{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, ep := range summary.Episodes[:3] {
		if !ep.WouldSearch {
			t.Fatalf("expected episode %d to be searched", ep.Episode)
		}
		if ep.MagnetsFound == 0 {
			t.Errorf("episode %d: expected the single-episode fallback to find a magnet, got 0", ep.Episode)
		}
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
