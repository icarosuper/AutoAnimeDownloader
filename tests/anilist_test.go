package tests

import (
	"AutoAnimeDownloader/modules/anilist"
	"io"
	"net/http"
	"strings"
	"testing"
)

func mockAniListResponse(body string, status int) func() {
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
}

func TestAniList_SearchAnimes_Success(t *testing.T) {
	json := `{"data": {"Page": {"mediaList": [{"progress": 3, "customLists": {"AutoAnimeDownloader": true}, "media": {"title": {"english": "My Anime", "romaji": "Boku no Anime"}, "airingSchedule": {"nodes": [{"id": 1, "episode": 4, "timeUntilAiring": 3600}]}}}]}}}`
	restore := mockAniListResponse(json, 200)
	defer restore()

	resp, err := anilist.SearchAnimes("icarosuper")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("expected 1 mediaList entry, got %d", len(resp.Data.Page.MediaList))
	}

	entry := resp.Data.Page.MediaList[0]
	if entry.Progress != 3 {
		t.Fatalf("expected progress 3, got %d", entry.Progress)
	}
	if !entry.CustomLists["AutoAnimeDownloader"] {
		t.Fatalf("expected custom list flag true")
	}
	if entry.Media.Title.English == nil || *entry.Media.Title.English != "My Anime" {
		t.Fatalf("unexpected english title: %v", entry.Media.Title.English)
	}
	if len(entry.Media.AiringSchedule.Nodes) != 1 {
		t.Fatalf("expected 1 airing node, got %d", len(entry.Media.AiringSchedule.Nodes))
	}
	node := entry.Media.AiringSchedule.Nodes[0]
	if node.Episode != 4 || node.TimeUntilAiring != 3600 {
		t.Fatalf("unexpected node values: %+v", node)
	}
}

func TestAniList_SearchAnimes_HTTPErrorStatus(t *testing.T) {
	restore := mockAniListResponse("error", 500)
	defer restore()

	resp, err := anilist.SearchAnimes("icarosuper")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on error status")
	}
}

func TestAniList_SearchAnimes_InvalidJSON(t *testing.T) {
	restore := mockAniListResponse("{invalid", 200)
	defer restore()

	resp, err := anilist.SearchAnimes("icarosuper")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on invalid JSON")
	}
}
