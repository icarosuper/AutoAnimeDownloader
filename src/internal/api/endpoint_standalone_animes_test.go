package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func standaloneConfig() *files.Config {
	return &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		CheckInterval:         10,
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
		ExcludedLists:         []string{},
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
	}
}

func standaloneWriteServer(fm *mockFileManager) *Server {
	return &Server{
		State:       daemon.NewState(),
		FileManager: fm,
		Torrents:    torrents.NewFakeBackend(),
		Librarian:   files.NewLibrarian(files.NewOSFileSystem()),
	}
}

// mockEmptyNyaa mantem os testes fora da rede: nenhum torrent encontrado.
func mockEmptyNyaa() func() {
	return nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
}

func postStandalone(t *testing.T, server *Server, mediaID int) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]int{"media_id": mediaID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/standalone-animes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleStandaloneAnimeAdd(server)(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("quero um erro, veio %s", rec.Body.String())
	}
	return resp.Error.Code
}

// TestStandaloneAdd_LibraryNotConfigured: a checagem vem primeiro porque nao depende de rede e
// porque, sem biblioteca, o Ensure falharia com um 500 opaco no lugar da causa.
func TestStandaloneAdd_LibraryNotConfigured(t *testing.T) {
	calls := 0
	defer mockStandaloneAniListCounting(&calls, emptyListJSON, standaloneMediaJSON)()

	config := standaloneConfig()
	config.CompletedAnimePath = ""
	fm := &mockFileManager{configs: config}

	rec := postStandalone(t, standaloneWriteServer(fm), 21)

	if rec.Code != http.StatusConflict {
		t.Fatalf("quero 409, veio %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "LIBRARY_NOT_CONFIGURED" {
		t.Fatalf("quero LIBRARY_NOT_CONFIGURED, veio %s", got)
	}
	if calls != 0 {
		t.Fatalf("biblioteca nao configurada nao pode tocar a AniList, fez %d chamadas", calls)
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("nada pode ir para o arquivo, veio %v", fm.standaloneAnimes)
	}
}

// TestStandaloneAdd_UnknownMediaIs404: GetMediaByID e tambem o que valida que o id existe.
func TestStandaloneAdd_UnknownMediaIs404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, `{"data":{"Media":null}}`)()

	fm := &mockFileManager{configs: standaloneConfig()}
	rec := postStandalone(t, standaloneWriteServer(fm), 999999)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("quero 404, veio %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("id inexistente nao pode entrar no arquivo, veio %v", fm.standaloneAnimes)
	}
}

// TestStandaloneAdd_BlockedNeverWritesTheFile: um bloqueio que grava antes de recusar seria
// pior que nenhum bloqueio.
func TestStandaloneAdd_BlockedNeverWritesTheFile(t *testing.T) {
	listWithAnime21 := `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"customLists":{"Blacklist":true},"media":{
		"id": 21, "episodes": 24, "status": "RELEASING",
		"title": {"english": "Listed Anime", "romaji": "Listed Anime"},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": []}
	}}]}}}`

	cases := []struct {
		name     string
		listJSON string
		fm       *mockFileManager
		wantCode string
	}{
		{
			name:     "ja avulso",
			listJSON: emptyListJSON,
			fm:       &mockFileManager{configs: standaloneConfig(), standaloneAnimes: []int{21}},
			wantCode: "ALREADY_STANDALONE",
		},
		{
			name:     "ja na lista processada",
			listJSON: strings.Replace(listWithAnime21, `"customLists":{"Blacklist":true}`, `"customLists":{}`, 1),
			fm:       &mockFileManager{configs: standaloneConfig()},
			wantCode: "ALREADY_TRACKED",
		},
		{
			name:     "em lista excluida",
			listJSON: listWithAnime21,
			fm: &mockFileManager{configs: func() *files.Config {
				c := standaloneConfig()
				c.ExcludedLists = []string{"Blacklist"}
				return c
			}()},
			wantCode: "ALREADY_BLACKLISTED",
		},
		{
			name:     "ja baixado por inteiro",
			listJSON: emptyListJSON,
			fm: &mockFileManager{
				configs:  standaloneConfig(),
				episodes: episodesForAnime(21, 24),
			},
			wantCode: "ALREADY_DOWNLOADED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer mockStandaloneAniList(tc.listJSON, standaloneMediaJSON)()

			before := len(tc.fm.standaloneAnimes)
			server := standaloneWriteServer(tc.fm)
			rec := postStandalone(t, server, 21)

			if rec.Code != http.StatusConflict {
				t.Fatalf("quero 409, veio %d: %s", rec.Code, rec.Body.String())
			}
			if got := errorCode(t, rec); got != tc.wantCode {
				t.Fatalf("quero %s, veio %s", tc.wantCode, got)
			}
			if len(tc.fm.standaloneAnimes) != before {
				t.Fatalf("bloqueio nao pode gravar no arquivo: %v", tc.fm.standaloneAnimes)
			}
			if backend, ok := server.Torrents.(*torrents.FakeBackend); ok && len(backend.EnsureCalls()) != 0 {
				t.Fatal("bloqueio nao pode chegar a DownloadStandaloneAnime")
			}
		})
	}
}

// TestStandaloneAdd_NoAiredEpisodeIsStillTracked: added == 0 e resposta normal, nao erro — um
// anime que ainda vai estrear e adicionado e fica acompanhado.
func TestStandaloneAdd_NoAiredEpisodeIsStillTracked(t *testing.T) {
	unairedMedia := `{"data":{"Media":{
		"id": 21, "episodes": 12, "format": "TV", "status": "NOT_YET_RELEASED",
		"title": {"english": "Future Anime", "romaji": "Future Anime"},
		"synonyms": [], "relations": {"edges": []},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": 86400, "airingAt": 999}]}
	}}}`
	defer mockStandaloneAniList(emptyListJSON, unairedMedia)()
	defer mockEmptyNyaa()()

	fm := &mockFileManager{configs: standaloneConfig()}
	rec := postStandalone(t, standaloneWriteServer(fm), 21)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Added int `json:"added"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if resp.Data.Added != 0 {
		t.Fatalf("quero added 0, veio %d", resp.Data.Added)
	}
	if len(fm.standaloneAnimes) != 1 || fm.standaloneAnimes[0] != 21 {
		t.Fatalf("o anime precisa ficar acompanhado mesmo sem episodio no ar, arquivo: %v", fm.standaloneAnimes)
	}
}

// TestStandaloneRemove_KeepEpisodesMarksThemManuallyManaged: e o ManuallyManaged que fecha a
// janela em que os arquivos correriam risco ao deixar de acompanhar por este endpoint.
func TestStandaloneRemove_KeepEpisodesMarksThemManuallyManaged(t *testing.T) {
	fm := &mockFileManager{
		configs:          standaloneConfig(),
		standaloneAnimes: []int{21},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 21, EpisodeHash: "h1"},
			{EpisodeNumber: 2, AnimeID: 21, EpisodeHash: "h2"},
			{EpisodeNumber: 3, AnimeID: 99, EpisodeHash: "h3"},
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/standalone-animes/21", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()
	handleStandaloneAnimeRemove(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("o id precisa sair do arquivo, veio %v", fm.standaloneAnimes)
	}
	if len(fm.episodes) != 3 {
		t.Fatalf("sem delete_episodes nada pode ser apagado, veio %d episodios", len(fm.episodes))
	}
	for _, ep := range fm.episodes {
		if ep.AnimeID == 21 && !ep.ManuallyManaged {
			t.Fatalf("episodio %d do avulso precisa virar ManuallyManaged", ep.EpisodeNumber)
		}
		if ep.AnimeID == 99 && ep.ManuallyManaged {
			t.Fatal("episodio de outro anime nao pode ser tocado")
		}
	}
}

// TestStandaloneRemove_DeleteEpisodes: com delete_episodes=true os registros somem.
func TestStandaloneRemove_DeleteEpisodes(t *testing.T) {
	fm := &mockFileManager{
		configs:          standaloneConfig(),
		standaloneAnimes: []int{21},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 21, EpisodeHash: "h1"},
			{EpisodeNumber: 3, AnimeID: 99, EpisodeHash: "h3"},
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/standalone-animes/21?delete_episodes=true", nil)
	req.SetPathValue("id", "21")
	rec := httptest.NewRecorder()
	handleStandaloneAnimeRemove(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d: %s", rec.Code, rec.Body.String())
	}
	if len(fm.episodes) != 1 || fm.episodes[0].AnimeID != 99 {
		t.Fatalf("so os episodios do avulso podiam sair, veio %+v", fm.episodes)
	}
}

// TestAniListSearch_FillsBlockReason e o teste que trava front e back na mesma funcao: o
// block_reason do resultado da busca tem de ser o mesmo valor que o POST usa para o 409.
func TestAniListSearch_FillsBlockReason(t *testing.T) {
	searchJSON := `{"data":{"Page":{"media":[
		{"id":21,"title":{"english":"Already Standalone","romaji":"x"},"format":"TV","status":"RELEASING","seasonYear":2016,"episodes":24,"coverImage":{"large":"a.jpg","medium":""}},
		{"id":22,"title":{"english":null,"romaji":"Addable"},"format":"TV","status":"FINISHED","seasonYear":2020,"episodes":12,"coverImage":{"large":"","medium":"b.jpg"}}
	]}}}`
	defer mockAniListRoutes(nil, emptyListJSON, standaloneMediaJSON, searchJSON)()

	fm := &mockFileManager{configs: standaloneConfig(), standaloneAnimes: []int{21}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anilist/search?q=test", nil)
	rec := httptest.NewRecorder()
	handleAniListSearch(standaloneWriteServer(fm))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []AniListSearchResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta invalida: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("quero 2 resultados, veio %d", len(resp.Data))
	}
	if resp.Data[0].BlockReason != blockReasonStandalone {
		t.Fatalf("quero %q, veio %q", blockReasonStandalone, resp.Data[0].BlockReason)
	}
	if resp.Data[1].BlockReason != "" {
		t.Fatalf("resultado adicionavel precisa vir com block_reason vazio, veio %q", resp.Data[1].BlockReason)
	}
	if resp.Data[1].Title != "Addable" {
		t.Fatalf("titulo sem english deve cair no romaji, veio %q", resp.Data[1].Title)
	}
	if resp.Data[1].Cover != "b.jpg" {
		t.Fatalf("cover deve cair no medium quando nao ha large, veio %q", resp.Data[1].Cover)
	}
	if resp.Data[0].Year != 2016 || resp.Data[0].Episodes != 24 {
		t.Fatalf("meta do resultado mapeada errado: %+v", resp.Data[0])
	}
}

// TestAniListSearch_ShortTermIsEmptyList: termo curto devolve lista vazia, nao 400 — o front
// digita letra a letra.
func TestAniListSearch_ShortTermIsEmptyList(t *testing.T) {
	calls := 0
	defer mockStandaloneAniListCounting(&calls, emptyListJSON, emptyListJSON)()

	fm := &mockFileManager{configs: standaloneConfig()}
	for _, term := range []string{"", "a", "ab"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anilist/search?q="+term, nil)
		rec := httptest.NewRecorder()
		handleAniListSearch(standaloneWriteServer(fm))(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("termo %q: quero 200, veio %d", term, rec.Code)
		}
		var resp struct {
			Data []AniListSearchResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("resposta invalida: %v", err)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("termo %q devia devolver lista vazia, veio %d", term, len(resp.Data))
		}
	}
	if calls != 0 {
		t.Fatalf("termo curto nao pode tocar a AniList, fez %d chamadas", calls)
	}
}

func episodesForAnime(animeID, n int) []files.EpisodeStruct {
	eps := make([]files.EpisodeStruct, 0, n)
	for i := 1; i <= n; i++ {
		eps = append(eps, files.EpisodeStruct{EpisodeNumber: i, AnimeID: animeID})
	}
	return eps
}
