package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// animeIDMigrationFM holds episodes and settings in memory so the test can see what the
// migration rewrote.
type animeIDMigrationFM struct {
	mockFileManagerForEpisodes
	configs  *files.Config
	episodes []files.EpisodeStruct
	settings map[int]files.AnimeSettings
}

func (m *animeIDMigrationFM) LoadConfigs() (*files.Config, error) { return m.configs, nil }
func (m *animeIDMigrationFM) SaveConfigs(c *files.Config) error   { m.configs = c; return nil }
func (m *animeIDMigrationFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	return m.episodes, nil
}
func (m *animeIDMigrationFM) UpsertEpisodes(eps []files.EpisodeStruct) error {
	for _, up := range eps {
		for i := range m.episodes {
			if m.episodes[i].EpisodeNumber == up.EpisodeNumber {
				m.episodes[i] = up
			}
		}
	}
	return nil
}
func (m *animeIDMigrationFM) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return m.settings, nil
}
func (m *animeIDMigrationFM) SaveAnimeSettings(id int, s files.AnimeSettings) error {
	if m.settings == nil {
		m.settings = map[int]files.AnimeSettings{}
	}
	m.settings[id] = s
	return nil
}

// mockEntryToMedia answers the entry->media query with the given map. A missing entry
// becomes 404, which is what AniList returns for an entry the user deleted.
func mockEntryToMedia(t *testing.T, byEntry map[int]int) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Variables struct {
				MediaListId int `json:"mediaListId"`
			} `json:"variables"`
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unexpected body: %s", body)
		}
		mediaID, ok := byEntry[payload.Variables.MediaListId]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("Not Found"))}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"MediaList":{"media":{"id":%d}}}}`, mediaID))),
		}, nil
	})
}

func TestMigrateAnimeIDsToMedia_RewritesEpisodesAndSettings(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer mockEntryToMedia(t, map[int]int{488911345: 194829, 583631757: 194829})()

	fm := &animeIDMigrationFM{
		configs: &files.Config{},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 583631757},
			{EpisodeNumber: 2, AnimeID: 488911345},
		},
		settings: map[int]files.AnimeSettings{583631757: {Progress: 42}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Both entries point to the SAME media: that is exactly the case that used to duplicate the anime.
	for _, ep := range fm.episodes {
		if ep.AnimeID != 194829 {
			t.Errorf("episode %d should have the media id, got %d", ep.EpisodeNumber, ep.AnimeID)
		}
	}
	if fm.settings[194829].Progress != 42 {
		t.Errorf("settings should have been copied to the media id, got %+v", fm.settings)
	}
	if !fm.configs.AnimeIDsAreMediaIDs {
		t.Error("the migration should mark completion in the config")
	}
}

// Idempotency: with the flag set there must be neither query nor rewrite — running again
// would treat MEDIA ids as entry ids and scramble everything.
func TestMigrateAnimeIDsToMedia_SkipsWhenAlreadyMigrated(t *testing.T) {
	called := false
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})()

	fm := &animeIDMigrationFM{
		configs:  &files.Config{AnimeIDsAreMediaIDs: true},
		episodes: []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 194829}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if called {
		t.Error("must not query AniList when the migration is already done")
	}
}

// A mid-run network failure must not leave half converted: nothing is written and the flag
// stays off, so the next pass restarts from scratch.
func TestMigrateAnimeIDsToMedia_FailureWritesNothing(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})()

	fm := &animeIDMigrationFM{
		configs:  &files.Config{},
		episodes: []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 583631757}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err == nil {
		t.Fatal("want an error when AniList fails")
	}
	if fm.episodes[0].AnimeID != 583631757 {
		t.Errorf("nothing should have been rewritten, got %d", fm.episodes[0].AnimeID)
	}
	if fm.configs.AnimeIDsAreMediaIDs {
		t.Error("the completion flag must not be set after a failure")
	}
}

// An entry deleted from AniList (404) does not stall the migration: the record keeps the old
// id and the rest converts normally.
func TestMigrateAnimeIDsToMedia_UnresolvedEntryDoesNotBlock(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer mockEntryToMedia(t, map[int]int{111: 500})()

	fm := &animeIDMigrationFM{
		configs: &files.Config{},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 111},
			{EpisodeNumber: 2, AnimeID: 222}, // no longer exists
		},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if fm.episodes[0].AnimeID != 500 {
		t.Errorf("a resolvable entry should become 500, got %d", fm.episodes[0].AnimeID)
	}
	if fm.episodes[1].AnimeID != 222 {
		t.Errorf("an unmatched entry must stay as it was, got %d", fm.episodes[1].AnimeID)
	}
	if !fm.configs.AnimeIDsAreMediaIDs {
		t.Error("the migration should complete even with unresolved entries")
	}
}
