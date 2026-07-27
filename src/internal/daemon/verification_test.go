package daemon

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

func TestSearchAnilist_FiltersByMediaStatus(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}},
		{"id": 2, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 200, "format": "TV", "status": "NOT_YET_RELEASED", "episodes": 12,
			"title": {"english": "Unreleased Anime", "romaji": "Unreleased Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		SavePath:              "/tmp/save",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
	}

	resp, err := searchAnilist(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("expected 1 anime after media-status filter, got %d", len(resp.Data.Page.MediaList))
	}
	if resp.Data.Page.MediaList[0].Media.Id != 100 {
		t.Errorf("expected surviving anime to be media id 100 (RELEASING), got %d", resp.Data.Page.MediaList[0].Media.Id)
	}
}

func TestSearchAnilist_EmptyMediaStatusesAllowsNothing(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		SavePath:              "/tmp/save",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{},
	}

	resp, err := searchAnilist(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Fatalf("expected 0 animes with empty DownloadMediaStatuses, got %d", len(resp.Data.Page.MediaList))
	}
}
