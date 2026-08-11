package daemon

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

// mockAniListRouter responde a query de lista com listJSON e a query de midia por id com
// mediaJSON — searchAnilist com avulsos faz as duas.
func mockAniListRouter(t *testing.T, listJSON, mediaJSON string) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		payload := listJSON
		if strings.Contains(string(body), "GetMediaByID") {
			payload = mediaJSON
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     make(http.Header),
		}, nil
	})
}

// mockEmptyNyaa evita que os testes batam na rede: a lista de torrents volta vazia.
func mockEmptyNyaa() func() {
	return nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
}

func standaloneTestConfig() *files.Config {
	return &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
	}
}

const listWithAnime100 = `{"data": {"Page": {"mediaList": [
	{"id": 1, "status": "CURRENT", "progress": 7, "customLists": {}, "media": {
		"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
		"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
		"synonyms": [], "relations": {"edges": []},
		"airingSchedule": {"nodes": []}
	}}
]}}}`

const mediaForAnime100 = `{"data": {"Media": {
	"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
	"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

const mediaForAnime500 = `{"data": {"Media": {
	"id": 500, "format": "TV", "status": "RELEASING", "episodes": 24,
	"title": {"english": "Standalone Anime", "romaji": "Standalone Anime"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": [{"id": 900, "episode": 1, "timeUntilAiring": -100, "airingAt": 1}]}
}}}`

// TestSearchAnilist_StandaloneAppendedAfterDedupe: o avulso ausente das listas entra no
// universo do passe, senao ele nunca e verificado de novo depois da adicao.
func TestSearchAnilist_StandaloneAppended(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime500)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{500})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	byMedia := map[int]anilist.MediaList{}
	for _, ml := range resp.Data.Page.MediaList {
		byMedia[ml.Media.Id] = ml
	}
	if len(byMedia) != 2 {
		t.Fatalf("quero 2 animes (lista + avulso), veio %d", len(byMedia))
	}
	if byMedia[500].Media.Id != 500 {
		t.Fatal("avulso 500 nao entrou no passe")
	}
	if len(fm.removedStandalone) != 0 {
		t.Fatalf("avulso fora da lista nao pode ser removido do arquivo, removeu %v", fm.removedStandalone)
	}
}

// TestSearchAnilist_StandaloneAlsoInListDoesNotDuplicateNorResetProgress trava a ordem: o
// append acontece DEPOIS do dedupe. Se ele viesse antes, o Progress 0 sintetico venceria
// (DedupeByMedia mantem o menor) e o daemon rebaixaria episodios ja assistidos.
func TestSearchAnilist_StandaloneAlsoInListDoesNotDuplicateNorResetProgress(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime100)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{100}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{100})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("avulso que tambem esta na lista duplicou: %d entradas", len(resp.Data.Page.MediaList))
	}
	if got := resp.Data.Page.MediaList[0].Progress; got != 7 {
		t.Fatalf("a entrada real precisa vencer: quero Progress 7, veio %d", got)
	}
}

// TestSearchAnilist_StandaloneAdoptedByListIsRemovedFromFile: adotado por uma lista, o
// registro avulso deixa de ter funcao — e o arquivo para de crescer.
func TestSearchAnilist_StandaloneAdoptedByListIsRemovedFromFile(t *testing.T) {
	defer mockAniListRouter(t, listWithAnime100, mediaForAnime100)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{100}}
	if _, err := searchAnilist(fm, standaloneTestConfig(), []int{100}); err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if !containsID(fm.removedStandalone, 100) {
		t.Fatalf("avulso adotado pela lista precisa sair do arquivo, removidos: %v", fm.removedStandalone)
	}
	if len(fm.standaloneAnimes) != 0 {
		t.Fatalf("arquivo de avulsos deveria estar vazio, veio %v", fm.standaloneAnimes)
	}
}

// TestSearchAnilist_StandaloneOutsideDownloadStatusesStays: um anime em PLANNING nao aparece
// no conjunto que o daemon processa, entao o registro avulso continua sendo o unico motivo de
// ele ser verificado — e o caso de uso mais obvio da feature.
func TestSearchAnilist_StandaloneOutsideDownloadStatusesStays(t *testing.T) {
	// A lista (filtrada por download_statuses server-side) volta vazia: o anime esta em
	// PLANNING na AniList, mas o daemon so busca CURRENT.
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, standaloneTestConfig(), []int{500})
	if err != nil {
		t.Fatalf("searchAnilist: %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 500 {
		t.Fatalf("avulso fora de download_statuses precisa ser processado, veio %+v", resp.Data.Page.MediaList)
	}
	if len(fm.removedStandalone) != 0 {
		t.Fatalf("avulso fora de download_statuses nao pode sair do arquivo, removeu %v", fm.removedStandalone)
	}
}

// TestDownloadStandaloneAnime_DoesNotDeleteTheRestOfTheLibrary trava o motivo de
// DownloadStandaloneAnime nao chamar handleSavedEpisodes: com delete_watched_episodes ligado e
// os episodios de um anime so na mao, identifyEpisodesNotInWatching apagaria todo o resto.
func TestDownloadStandaloneAnime_DoesNotDeleteTheRestOfTheLibrary(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	defer mockEmptyNyaa()()

	configs := standaloneTestConfig()
	configs.DeleteWatchedEpisodes = true

	// Biblioteca com 3 animes ja salvos: nenhum deles pode sumir por causa da adicao.
	fm := &mockFileManagerForEpisodes{savedEpisodes: []files.EpisodeStruct{
		{EpisodeNumber: 1, AnimeID: 10, EpisodeHash: "h1"},
		{EpisodeNumber: 2, AnimeID: 20, EpisodeHash: "h2"},
		{EpisodeNumber: 3, AnimeID: 30, EpisodeHash: "h3"},
	}}
	backend := torrents.NewFakeBackend()

	if _, err := DownloadStandaloneAnime(fm, backend, configs, 500); err != nil {
		t.Fatalf("DownloadStandaloneAnime: %v", err)
	}

	if len(fm.deletedEpisodeKeys) != 0 {
		t.Fatalf("adicionar um avulso nao pode apagar episodio nenhum, apagou %v", fm.deletedEpisodeKeys)
	}
}

// TestDownloadStandaloneAnime_UnknownMedia: id que a AniList nao conhece nao pode virar um
// download silencioso.
func TestDownloadStandaloneAnime_UnknownMedia(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, `{"data": {"Media": null}}`)()

	fm := &mockFileManagerForEpisodes{}
	if _, err := DownloadStandaloneAnime(fm, torrents.NewFakeBackend(), standaloneTestConfig(), 999999); err == nil {
		t.Fatal("quero erro para media id desconhecido")
	}
}
