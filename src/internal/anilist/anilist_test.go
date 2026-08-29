package anilist

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func mockList(calls *int, body string) func() {
	return MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
}

// TestGetFrontendAnimeList_CachesWithinTTL trava o conserto do 429: /api/v1/animes tem poll do
// frontend a cada 30s por aba, e sem cache cada aba aberta virava um request por conta contra o
// limite de 30/min da AniList — o 429 resultante derrubava o ciclo de download do daemon junto.
func TestGetFrontendAnimeList_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":10}}]}}}`)()

	statuses := []string{"CURRENT"}

	if _, err := GetFrontendAnimeList("user", statuses); err != nil {
		t.Fatalf("primeira busca falhou: %v", err)
	}
	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("segunda busca falhou: %v", err)
	}
	if calls != 1 {
		t.Fatalf("esperava 1 request para a AniList, obteve %d", calls)
	}
	if len(second.Data.Page.MediaList) != 1 {
		t.Fatalf("cache devolveu %d entradas, esperava 1", len(second.Data.Page.MediaList))
	}

	// Outra conta e outra combinacao de status sao chaves diferentes: precisam buscar.
	if _, err := GetFrontendAnimeList("outra", statuses); err != nil {
		t.Fatalf("busca de outra conta falhou: %v", err)
	}
	if calls != 2 {
		t.Fatalf("esperava 2 requests apos trocar de conta, obteve %d", calls)
	}
}

// TestGetFrontendAnimeList_CacheHandsOutCopies garante que o overlay de customLists feito por
// fetchAniListEntries no resultado nao vaza para o proximo chamador (nem vira corrida entre
// dois requests concorrentes de /api/v1/animes).
func TestGetFrontendAnimeList_CacheHandsOutCopies(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"media":{"id":10}}]}}}`)()

	statuses := []string{"CURRENT"}
	first, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("primeira busca falhou: %v", err)
	}
	first.Data.Page.MediaList[0].CustomLists = CustomLists{"AutoDownloader": true}

	second, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("segunda busca falhou: %v", err)
	}
	if second.Data.Page.MediaList[0].CustomLists != nil {
		t.Errorf("cache contaminado pelo chamador anterior: %v", second.Data.Page.MediaList[0].CustomLists)
	}
}

// TestGetCustomListsMap_CachesEmptyResult: uma conta sem custom lists nao pode gerar um request
// por poll do frontend. O TTL curto (customListsEmptyTTL) e o que limita isso.
func TestGetCustomListsMap_CachesEmptyResult(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"customLists":null}]}}}`)()

	statuses := []string{"CURRENT"}
	GetCustomListsMap("user", statuses)
	GetCustomListsMap("user", statuses)

	if calls != 1 {
		t.Fatalf("esperava 1 request para a AniList, obteve %d", calls)
	}
}

// TestMockAniListDo_ClearsCaches: sem isso um teste herdaria as respostas do teste anterior.
func TestMockAniListDo_ClearsCaches(t *testing.T) {
	firstCalls := 0
	restore := mockList(&firstCalls, `{"data":{"Page":{"mediaList":[{"id":1,"media":{"id":10}}]}}}`)
	GetFrontendAnimeList("user", []string{"CURRENT"})
	restore()

	secondCalls := 0
	defer mockList(&secondCalls, `{"data":{"Page":{"mediaList":[]}}}`)()
	resp, err := GetFrontendAnimeList("user", []string{"CURRENT"})
	if err != nil {
		t.Fatalf("busca falhou: %v", err)
	}
	if secondCalls != 1 {
		t.Fatalf("esperava que o novo mock fosse consultado, requests=%d", secondCalls)
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Errorf("resposta veio do cache do teste anterior: %+v", resp.Data.Page.MediaList)
	}
}

// TestGetAnimeInfoByIDs_ChunksPerAccount: o refresh de orfaos de GET /animes roda a cada poll de
// 30s por aba. Um request por anime por conta era a rajada da decisions.md #65; o contrato aqui e
// ceil(ids/50) por conta, e nada mais.
func TestGetAnimeInfoByIDs_ChunksPerAccount(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[]}}}`)()

	ids := make([]int, 0, 60)
	for i := range 60 {
		ids = append(ids, 1000+i)
	}

	if _, err := GetAnimeInfoByIDs(ids, []string{"a", "b"}); err != nil {
		t.Fatalf("GetAnimeInfoByIDs: %v", err)
	}
	if calls != 4 {
		t.Fatalf("60 ids em 2 contas com perPage 50 sao 4 requests, veio %d", calls)
	}
}

// TestGetAnimeInfoByIDs_KeepsLowestProgressAcrossAccounts: mesma regra de GetAnimeInfo e de
// DedupeByMedia — episodio so e "assistido" quando TODAS as contas o assistiram. Sem isso o
// refresh de orfaos rebaixaria o progresso da conta atrasada.
func TestGetAnimeInfoByIDs_KeepsLowestProgressAcrossAccounts(t *testing.T) {
	call := 0
	defer MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		call++
		progress := 10
		if call == 2 {
			progress = 3
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"data":{"Page":{"mediaList":[{"id":1,"progress":%d,"media":{"id":21,"title":{"romaji":"X"}}}]}}}`, progress))),
		}, nil
	})()

	byMedia, err := GetAnimeInfoByIDs([]int{21}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("GetAnimeInfoByIDs: %v", err)
	}
	if byMedia[21] == nil || byMedia[21].Progress != 3 {
		t.Fatalf("quero o MENOR progresso das duas contas (3), veio %+v", byMedia[21])
	}
}

// TestGetAnimeInfoByIDs_UntrackedIDIsAbsent: id que nenhuma conta acompanha nao esta no mapa —
// e o estado normal do anime tirado da lista com episodios ainda em disco, nao um erro.
func TestGetAnimeInfoByIDs_UntrackedIDIsAbsent(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":21}}]}}}`)()

	byMedia, err := GetAnimeInfoByIDs([]int{21, 99}, []string{"a"})
	if err != nil {
		t.Fatalf("id sem entrada nao e erro: %v", err)
	}
	if byMedia[21] == nil {
		t.Fatal("o id acompanhado precisa estar no mapa")
	}
	if _, ok := byMedia[99]; ok {
		t.Fatalf("o id que ninguem acompanha nao pode estar no mapa, veio %+v", byMedia[99])
	}
}

// TestGetAnimeInfoByIDs_PartialFailureKeepsWhatCame: o erro de uma conta nao pode zerar o que a
// outra respondeu — a tela ficaria sem capa e sem nome em toda a lista por causa de uma conta.
func TestGetAnimeInfoByIDs_PartialFailureKeepsWhatCame(t *testing.T) {
	call := 0
	defer MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		call++
		if call == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[{"id":1,"progress":2,"media":{"id":21}}]}}}`)),
		}, nil
	})()

	byMedia, err := GetAnimeInfoByIDs([]int{21}, []string{"a", "b"})
	if err == nil {
		t.Fatal("a falha de uma conta precisa ser reportada")
	}
	if byMedia[21] == nil {
		t.Fatal("o erro nao pode invalidar o que a outra conta respondeu")
	}
}
