package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
)

// mockAniListRoutes responde por nome da query: GetMediaByID e SearchMedia tem corpo proprio,
// todo o resto (lista do frontend, customLists) cai em listJSON. calls, quando nao nil, conta as
// requisicoes — e como os testes provam que um caminho NAO tocou a AniList.
func mockAniListRoutes(calls *int, listJSON, mediaJSON, searchJSON string) func() {
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if calls != nil {
			*calls++
		}
		payload := listJSON
		switch {
		case strings.Contains(string(body), "GetMediaByID"):
			payload = mediaJSON
		case strings.Contains(string(body), "SearchMedia"):
			payload = searchJSON
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})
}

// mockStandaloneAniList: a query por lista devolve vazio (o anime nao esta em lista nenhuma) e
// a query por media id devolve o anime. E exatamente a situacao de um avulso.
func mockStandaloneAniList(listJSON, mediaJSON string) func() {
	return mockAniListRoutes(nil, listJSON, mediaJSON, mediaJSON)
}

func mockStandaloneAniListCounting(calls *int, listJSON, mediaJSON string) func() {
	return mockAniListRoutes(calls, listJSON, mediaJSON, mediaJSON)
}

const emptyListJSON = `{"data":{"Page":{"mediaList":[]}}}`

const standaloneMediaJSON = `{"data":{"Media":{
	"id": 21, "episodes": 24, "format": "TV", "status": "RELEASING",
	"title": {"english": "Standalone Anime", "romaji": "Standalone Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "cover.jpg", "medium": ""},
	"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 111}]}
}}}`

func standaloneServer(fm *mockFileManager) *Server {
	return &Server{State: daemon.NewState(), FileManager: fm}
}

// TestHandleAnimeEpisodes_StandaloneAnimeIsNot404: sem o fallback de GetMediaByID a tela de
// detalhe de um avulso simplesmente nao abre.
func TestHandleAnimeEpisodes_StandaloneAnimeIsNot404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()

	handleAnimeEpisodes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200 para um avulso, veio %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data AnimeDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if resp.Data.AnimeID != 21 || len(resp.Data.Episodes) != 1 {
		t.Fatalf("detalhe do avulso incompleto: %+v", resp.Data)
	}
}

// TestHandleAnimeEpisodes_NonStandaloneUntrackedStill404: o fallback vale so para avulsos —
// um anime que nao esta em lista nenhuma nem no arquivo continua 404.
func TestHandleAnimeEpisodes_NonStandaloneUntrackedStill404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()

	handleAnimeEpisodes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("quero 404, veio %d", rec.Code)
	}
}

// TestHandleAnimes_StandaloneAppearsWithoutDownloadedEpisodes: sem isso o avulso so aparece na
// lista do Status depois de ter episodio baixado.
func TestHandleAnimes_StandaloneAppearsWithoutDownloadedEpisodes(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	fm := &mockFileManager{standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
	rec := httptest.NewRecorder()

	handleAnimes(standaloneServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []AnimeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("quero 1 anime na lista, veio %d: %+v", len(resp.Data), resp.Data)
	}
	got := resp.Data[0]
	if got.AnimeID != 21 {
		t.Fatalf("quero o anime 21, veio %+v", got)
	}
	if !got.IsStandalone {
		t.Fatal("o avulso precisa vir marcado com is_standalone, senao o chip nunca aparece")
	}
	if got.CoverImage != "cover.jpg" || got.TotalEpisodes != 24 {
		t.Fatalf("avulso sem capa/total: %+v", got)
	}
}

// TestHandleAnimes_ListedAnimeIsNotStandalone: is_standalone e origem, e um anime de lista nao
// pode ganhar o chip.
func TestHandleAnimes_ListedAnimeIsNotStandalone(t *testing.T) {
	listJSON := `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"customLists":{},"media":{
		"id": 21, "episodes": 24, "status": "RELEASING",
		"title": {"english": "Listed Anime", "romaji": "Listed Anime"},
		"coverImage": {"large": "cover.jpg", "medium": ""},
		"airingSchedule": {"nodes": []}
	}}]}}}`
	defer mockStandaloneAniList(listJSON, standaloneMediaJSON)()

	fm := &mockFileManager{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
	rec := httptest.NewRecorder()

	handleAnimes(standaloneServer(fm))(rec, req)

	var resp struct {
		Data []AnimeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].IsStandalone {
		t.Fatalf("anime de lista nao pode vir como avulso: %+v", resp.Data)
	}
}

var _ = files.EpisodeStruct{}
