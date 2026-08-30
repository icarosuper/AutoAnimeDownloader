package anilist

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSearchMedia_MapsResults: a barra de busca da tela #/add depende destes campos.
func TestSearchMedia_MapsResults(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[
		{"id":21,"title":{"english":"One Piece","romaji":"One Piece"},"format":"TV","status":"RELEASING",
		 "seasonYear":1999,"episodes":null,"coverImage":{"large":"big.jpg","medium":"small.jpg"}},
		{"id":5114,"title":{"english":null,"romaji":"Hagane no Renkinjutsushi"},"format":"TV","status":"FINISHED",
		 "seasonYear":2009,"episodes":64,"coverImage":{"large":"","medium":"fma.jpg"}}
	]}}}`)()

	results, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("quero 2 resultados, veio %d", len(results))
	}

	first := results[0]
	if first.Id != 21 || first.Format != MediaFormatTV || first.Status != MediaStatusReleasing {
		t.Fatalf("primeiro resultado mapeado errado: %+v", first)
	}
	if first.SeasonYear != 1999 {
		t.Fatalf("quero seasonYear 1999, veio %d", first.SeasonYear)
	}
	if first.Episodes != nil {
		t.Fatalf("episodes null deve virar nil, veio %v", *first.Episodes)
	}
	if first.CoverImage.Large != "big.jpg" {
		t.Fatalf("cover mapeada errado: %+v", first.CoverImage)
	}
	if results[1].Episodes == nil || *results[1].Episodes != 64 {
		t.Fatalf("quero 64 episodios no segundo resultado, veio %v", results[1].Episodes)
	}
}

// TestSearchMedia_UnreleasedFilterIsServerSide: o filtro precisa viajar NA QUERY. Filtrar depois
// de receber esvaziaria a busca (perPage e 20), e um teste que so olhasse o resultado passaria
// com as duas implementacoes — por isso este inspeciona o corpo enviado.
func TestSearchMedia_UnreleasedFilterIsServerSide(t *testing.T) {
	var sent string
	defer MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		sent = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"media":[]}}}`)),
		}, nil
	})()

	if _, err := SearchMedia("one", false); err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if !strings.Contains(sent, "status_not: NOT_YET_RELEASED") {
		t.Fatalf("o padrao precisa esconder os nao lancados na query, veio: %s", sent)
	}

	if _, err := SearchMedia("one", true); err != nil {
		t.Fatalf("SearchMedia: %v", err)
	}
	if strings.Contains(sent, "status_not") {
		t.Fatalf("com includeUnreleased o argumento nao pode existir, veio: %s", sent)
	}
}

// TestGetMediaByID_SyntheticEntry: o MediaList devolvido nao vem de lista nenhuma, entao
// Progress/Status/Id precisam ser os zeros — e nao lixo que o dedupe do loop leve a serio.
func TestGetMediaByID_SyntheticEntry(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":{
		"id":21,"episodes":12,"format":"TV","status":"RELEASING",
		"title":{"english":"My Anime","romaji":"Boku no Anime"},
		"synonyms":["MA"],
		"relations":{"edges":[{"node":{"title":{"english":"Prequel","romaji":"Prequel"},"synonyms":[],"episodes":13},"relationType":"PREQUEL"}]},
		"coverImage":{"large":"big.jpg","medium":"small.jpg"},
		"airingSchedule":{"nodes":[{"id":10,"episode":1,"timeUntilAiring":-100,"airingAt":123}]}
	}}}`)()

	ml, err := GetMediaByID(21, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if ml == nil {
		t.Fatal("quero um MediaList, veio nil")
	}
	if ml.Id != 0 || ml.Progress != 0 || ml.Status != "" {
		t.Fatalf("entrada sintetica precisa vir zerada, veio Id=%d Progress=%d Status=%q", ml.Id, ml.Progress, ml.Status)
	}
	if ml.Media.Id != 21 {
		t.Fatalf("quero media id 21, veio %d", ml.Media.Id)
	}
	// A busca por anime e searchNyaaForSingleEpisode dependem destes tres.
	if len(ml.Media.Synonyms) != 1 {
		t.Fatalf("synonyms perdido: %+v", ml.Media.Synonyms)
	}
	if len(ml.Media.Relations.Edges) != 1 || ml.Media.Relations.Edges[0].RelationType != "PREQUEL" {
		t.Fatalf("relations perdido: %+v", ml.Media.Relations)
	}
	if len(ml.Media.AiringSchedule.Nodes) != 1 || ml.Media.AiringSchedule.Nodes[0].ID != 10 {
		t.Fatalf("airingSchedule sem id: %+v", ml.Media.AiringSchedule.Nodes)
	}
}

// TestGetMediaByID_CachesWithinTTL: GET /animes tem poll de 30s por aba e passa a chamar isto
// por anime avulso — o mesmo motivo do cache de GetFrontendAnimeList.
func TestGetMediaByID_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":{"id":21,"title":{"romaji":"X"}}}}`)()

	if _, err := GetMediaByID(21, PriorityCritical); err != nil {
		t.Fatalf("primeira busca: %v", err)
	}
	if _, err := GetMediaByID(21, PriorityCritical); err != nil {
		t.Fatalf("segunda busca: %v", err)
	}
	if calls != 1 {
		t.Fatalf("quero 1 request, veio %d", calls)
	}

	if _, err := GetMediaByID(22, PriorityCritical); err != nil {
		t.Fatalf("outro id: %v", err)
	}
	if calls != 2 {
		t.Fatalf("id diferente e chave diferente: quero 2 requests, veio %d", calls)
	}
}

// TestGetMediaByID_NotFound: id que nao existe devolve (nil, nil), o que o POST traduz em 404.
func TestGetMediaByID_NotFound(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Media":null}}`)()

	ml, err := GetMediaByID(999999, PriorityCritical)
	if err != nil {
		t.Fatalf("Media null nao e erro: %v", err)
	}
	if ml != nil {
		t.Fatalf("quero nil, veio %+v", ml)
	}
}

// mockCapturing devolve payloads em sequencia e guarda os corpos enviados — e como os testes do
// lote provam QUAIS ids foram pedidos, nao so quantos requests sairam.
func mockCapturing(sent *[]string, payloads ...string) func() {
	return MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		*sent = append(*sent, string(body))
		payload := payloads[len(payloads)-1]
		if len(*sent) <= len(payloads) {
			payload = payloads[len(*sent)-1]
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})
}

// TestGetMediaByIDs_SingleRequestForManyIDs e a razao de a funcao existir: o loop anterior
// gastava 1 request por avulso contra o limite de 30/min da AniList (decisions.md #65).
func TestGetMediaByIDs_SingleRequestForManyIDs(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[
		{"id":21,"title":{"romaji":"A"}},
		{"id":22,"title":{"romaji":"B"}},
		{"id":23,"title":{"romaji":"C"}}
	]}}}`)()

	medias, err := GetMediaByIDs([]int{21, 22, 23}, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByIDs: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("quero 1 request para 3 ids, veio %d", len(sent))
	}
	if !strings.Contains(sent[0], "id_in") {
		t.Fatalf("a query precisa buscar em lote com id_in, veio: %s", sent[0])
	}
	for _, id := range []int{21, 22, 23} {
		if medias[id] == nil || medias[id].Media.Id != id {
			t.Fatalf("id %d nao voltou no mapa: %+v", id, medias[id])
		}
	}
}

// TestGetMediaByIDs_AbsentIDIsNilNotMissing: id_in OMITE o media apagado da AniList em vez de
// devolver erro. Sem transformar a omissao em nil explicito, quem chama nao consegue distinguir
// "avulso morto" de "nao deu para buscar" e o anime some da tela sem aviso.
func TestGetMediaByIDs_AbsentIDIsNilNotMissing(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"A"}}]}}}`)()

	medias, err := GetMediaByIDs([]int{21, 999999}, PriorityCritical)
	if err != nil {
		t.Fatalf("id omitido nao e erro: %v", err)
	}
	ml, ok := medias[999999]
	if !ok {
		t.Fatal("o id omitido precisa estar no mapa com nil; ausente significa 'nao foi buscado'")
	}
	if ml != nil {
		t.Fatalf("quero nil para o id omitido, veio %+v", ml)
	}
}

// TestGetMediaByIDs_CachesPerID: o cache e por id, e nao por lote — chaveado pelo lote, um avulso
// novo invalidaria a leitura de todos os outros a cada adicao.
func TestGetMediaByIDs_CachesPerID(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent,
		`{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"A"}},{"id":22,"title":{"romaji":"B"}}]}}}`,
		`{"data":{"Page":{"media":[{"id":23,"title":{"romaji":"C"}}]}}}`,
	)()

	if _, err := GetMediaByIDs([]int{21, 22}, PriorityCritical); err != nil {
		t.Fatalf("primeira busca: %v", err)
	}
	if _, err := GetMediaByIDs([]int{21, 22, 23}, PriorityCritical); err != nil {
		t.Fatalf("segunda busca: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("quero 2 requests, veio %d", len(sent))
	}
	if strings.Contains(sent[1], "21") || !strings.Contains(sent[1], "23") {
		t.Fatalf("a segunda busca so pode pedir o id novo, veio: %s", sent[1])
	}

	// O cache e o mesmo de GetMediaByID: quem buscou em lote nao paga de novo no lookup avulso.
	if _, err := GetMediaByID(22, PriorityCritical); err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("GetMediaByID tinha que sair do cache do lote, veio %d requests", len(sent))
	}
}

// TestGetMediaByIDs_ChunksAtPageSize: perPage maximo da AniList e 50 — pedir 60 ids numa query so
// devolveria 50 e os outros 10 pareceriam apagados.
func TestGetMediaByIDs_ChunksAtPageSize(t *testing.T) {
	var sent []string
	defer mockCapturing(&sent, `{"data":{"Page":{"media":[]}}}`)()

	ids := make([]int, 0, 60)
	for i := range 60 {
		ids = append(ids, 1000+i)
	}

	medias, err := GetMediaByIDs(ids, PriorityCritical)
	if err != nil {
		t.Fatalf("GetMediaByIDs: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("60 ids com perPage 50 sao 2 requests, veio %d", len(sent))
	}
	if len(medias) != 60 {
		t.Fatalf("todo id pedido precisa ter desfecho no mapa, veio %d", len(medias))
	}
}

// TestSearchMedia_CachesWithinTTL: a tela busca no submit, entao a mesma chave se repete —
// usuario refazendo a busca, toggle de nao lancados indo e voltando, outra aba com o mesmo
// termo. Case-insensitive porque a busca da AniList tambem e.
func TestSearchMedia_CachesWithinTTL(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"One Piece"}}]}}}`)()

	for _, term := range []string{"one piece", "one piece", "One Piece", "ONE PIECE"} {
		if _, err := SearchMedia(term, false); err != nil {
			t.Fatalf("SearchMedia(%q): %v", term, err)
		}
	}
	if calls != 1 {
		t.Fatalf("quero 1 request para o mesmo termo em qualquer caixa, veio %d", calls)
	}

	if _, err := SearchMedia("one piece", true); err != nil {
		t.Fatalf("SearchMedia com includeUnreleased: %v", err)
	}
	if calls != 2 {
		t.Fatalf("includeUnreleased faz parte da chave: quero 2 requests, veio %d", calls)
	}
}

// TestSearchMedia_CachedSliceIsNotShared: a fatia guardada e servida a todos os requests que
// acertarem a chave — quem chama nao pode cortar nem reordenar a memoria do cache.
func TestSearchMedia_CachedSliceIsNotShared(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[
		{"id":21,"title":{"romaji":"A"}},
		{"id":22,"title":{"romaji":"B"}}
	]}}}`)()

	first, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("primeira busca: %v", err)
	}
	first[0].Id = 999

	second, err := SearchMedia("one", false)
	if err != nil {
		t.Fatalf("segunda busca: %v", err)
	}
	if second[0].Id != 21 {
		t.Fatalf("o cache foi corrompido por quem chamou: %+v", second[0])
	}
}

// TestSearchMedia_ServesStaleWhenRefused: recusada pelo gate de orcamento, a busca serve o
// resultado vencido em vez de virar "a busca falhou" na tela. Ver decisions.md #72.
func TestSearchMedia_ServesStaleWhenRefused(t *testing.T) {
	defer MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"media":[{"id":21,"title":{"romaji":"One Piece"}}]}}}`,
			map[string]string{"X-RateLimit-Remaining": "29"}), nil
	})()

	if _, err := SearchMedia("one piece", false); err != nil {
		t.Fatalf("primeira busca: %v", err)
	}

	// Vence o cache e afunda o orcamento: a proxima chamada e recusada pelo gate.
	searchCache.set(searchCacheKey("one piece", false), []MediaSearchResult{{Id: 21}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	results, err := SearchMedia("one piece", false)
	if err != nil {
		t.Fatalf("esperava o cache vencido, veio erro: %v", err)
	}
	if len(results) != 1 || results[0].Id != 21 {
		t.Fatalf("esperava a entrada vencida, veio %+v", results)
	}
}

// TestSearchMedia_FlushesWhenFull: unico cache com chave de texto livre, e entrada vencida nunca
// sai do mapa — sem teto, o mapa so cresce enquanto o daemon roda.
func TestSearchMedia_FlushesWhenFull(t *testing.T) {
	calls := 0
	defer mockList(&calls, `{"data":{"Page":{"media":[]}}}`)()

	for i := range searchCacheMaxEntries {
		if _, err := SearchMedia("termo"+strconv.Itoa(i), false); err != nil {
			t.Fatalf("busca %d: %v", i, err)
		}
	}
	if searchCache.size() != searchCacheMaxEntries {
		t.Fatalf("quero o cache cheio com %d, veio %d", searchCacheMaxEntries, searchCache.size())
	}

	if _, err := SearchMedia("estoura", false); err != nil {
		t.Fatalf("busca que estoura o teto: %v", err)
	}
	if searchCache.size() != 1 {
		t.Fatalf("passado o teto o cache e descartado inteiro: quero 1, veio %d", searchCache.size())
	}
}
