package unit

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"fmt"
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

func TestAniListModule_GetAnimeInfo_ParsesSynonymsAndRelations(t *testing.T) {
	json := `{"data": {"Page": {"mediaList": [{"id": 1, "status": "CURRENT", "progress": 2, "media": {
		"id": 21, "episodes": 12, "format": "TV", "status": "RELEASING",
		"title": {"english": "My Anime", "romaji": "Boku no Anime"},
		"synonyms": ["My Anime Season 2"],
		"relations": {"edges": [{"relationType": "PREQUEL", "node": {"title": {"romaji": "Prequel"}, "episodes": 12}}]},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [{"id": 10, "episode": 3, "timeUntilAiring": 0}]}
	}}]}}}`
	restore := mockAniListResponse(json, 200)
	defer restore()

	ml, err := anilist.GetAnimeInfo(21, []string{"user"}, anilist.PriorityCritical)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ml == nil {
		t.Fatal("expected an entry, got nil")
	}
	if len(ml.Media.Synonyms) != 1 || ml.Media.Synonyms[0] != "My Anime Season 2" {
		t.Fatalf("expected synonyms to be parsed, got %v", ml.Media.Synonyms)
	}
	if len(ml.Media.Relations.Edges) != 1 || ml.Media.Relations.Edges[0].RelationType != "PREQUEL" {
		t.Fatalf("expected relations to be parsed, got %v", ml.Media.Relations)
	}
}

func TestAniListModule_SearchAnimes_Success(t *testing.T) {
	json := `{"data": {"Page": {"mediaList": [{"progress": 3, "customLists": {"AutoAnimeDownloader": true}, "media": {"title": {"english": "My Anime", "romaji": "Boku no Anime"}, "airingSchedule": {"nodes": [{"id": 1, "episode": 4, "timeUntilAiring": 3600}]}}}]}}}`
	restore := mockAniListResponse(json, 200)
	defer restore()

	resp, err := anilist.GetAllCurrentAnime("icarosuper", []string{"CURRENT", "REPEATING"})
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

func TestAniListModule_SearchAnimes_HTTPErrorStatus(t *testing.T) {
	restore := mockAniListResponse("error", 500)
	defer restore()

	resp, err := anilist.GetAllCurrentAnime("icarosuper", []string{"CURRENT"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on error status")
	}
}

func TestAniListModule_SearchAnimes_InvalidJSON(t *testing.T) {
	restore := mockAniListResponse("{invalid", 200)
	defer restore()

	resp, err := anilist.GetAllCurrentAnime("icarosuper", []string{"CURRENT"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on invalid JSON")
	}
}

func TestAniListModule_GetAnimeInfo_ParsesMediaID(t *testing.T) {
	json := `{"data": {"Page": {"mediaList": [{"id": 12345, "status": "CURRENT", "progress": 3, "media": {"id": 21, "episodes": 12, "title": {"english": "My Anime", "romaji": "Boku no Anime"}, "airingSchedule": {"nodes": [{"id": 1, "episode": 4, "timeUntilAiring": 3600}]}}}]}}}`
	restore := mockAniListResponse(json, 200)
	defer restore()

	ml, err := anilist.GetAnimeInfo(21, []string{"user"}, anilist.PriorityCritical)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ml.Media.Id != 21 {
		t.Fatalf("expected media id 21, got %d", ml.Media.Id)
	}
}

// GetAnimeInfo colapsa as contas: o anime aparece nas duas listas com entradas diferentes e
// deve voltar como UM registro, com o MENOR progresso (a conta mais atrasada manda).
func TestAniListModule_GetAnimeInfo_CollapsesAccountsKeepingLowestProgress(t *testing.T) {
	var calls int
	restore := anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		calls++
		progress := 7
		entryID := 111
		if calls > 1 {
			progress, entryID = 2, 222
		}
		body := fmt.Sprintf(`{"data": {"Page": {"mediaList": [{"id": %d, "status": "CURRENT", "progress": %d, "media": {"id": 21, "episodes": 12, "title": {"english": "My Anime"}, "airingSchedule": {"nodes": []}}}]}}}`, entryID, progress)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	defer restore()

	ml, err := anilist.GetAnimeInfo(21, []string{"accountA", "accountB"}, anilist.PriorityCritical)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ml.Progress != 2 {
		t.Errorf("esperava o menor progresso entre as contas (2), obteve %d", ml.Progress)
	}
}

// Anime fora de todas as listas nao e erro: os episodios podem seguir em disco.
func TestAniListModule_GetAnimeInfo_NotTrackedReturnsNil(t *testing.T) {
	restore := mockAniListResponse(`{"data": {"Page": {"mediaList": []}}}`, 200)
	defer restore()

	ml, err := anilist.GetAnimeInfo(21, []string{"user"}, anilist.PriorityCritical)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ml != nil {
		t.Errorf("esperava nil para anime fora das listas, obteve %+v", ml)
	}
}

func TestAniListModule_GetAnimeInfo_ParsesCustomLists(t *testing.T) {
	json := `{"data": {"Page": {"mediaList": [{"id": 1, "status": "CURRENT", "progress": 2, "customLists": {"Blacklist": true}, "media": {
		"id": 21, "episodes": 12, "format": "TV", "status": "RELEASING",
		"title": {"english": "My Anime", "romaji": "Boku no Anime"},
		"airingSchedule": {"nodes": []}
	}}]}}}`
	restore := mockAniListResponse(json, 200)
	defer restore()

	ml, err := anilist.GetAnimeInfo(21, []string{"user"}, anilist.PriorityCritical)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ml.CustomLists["Blacklist"] {
		t.Fatalf("expected CustomLists[Blacklist]=true, got %v", ml.CustomLists)
	}
}

func TestMediaStatusAllowed_AllowedStatusPasses(t *testing.T) {
	allowed := []string{"RELEASING", "FINISHED"}
	if !anilist.MediaStatusAllowed(allowed, anilist.MediaStatusReleasing) {
		t.Error("expected RELEASING to be allowed")
	}
}

func TestMediaStatusAllowed_DisallowedStatusSkipped(t *testing.T) {
	allowed := []string{"RELEASING", "FINISHED"}
	if anilist.MediaStatusAllowed(allowed, anilist.MediaStatusNotYetReleased) {
		t.Error("expected NOT_YET_RELEASED to be disallowed")
	}
}

func TestMediaStatusAllowed_EmptySliceAllowsNothing(t *testing.T) {
	if anilist.MediaStatusAllowed([]string{}, anilist.MediaStatusReleasing) {
		t.Error("expected empty allowed list to allow nothing")
	}
}

func TestAniListModule_SearchAnimes_EmptyStatuses(t *testing.T) {
	resp, err := anilist.GetAllCurrentAnime("icarosuper", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Fatalf("expected empty mediaList, got %d", len(resp.Data.Page.MediaList))
	}
}
