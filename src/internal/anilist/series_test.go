package anilist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// fixtureMedia e um no do grafo falso da AniList usado pelos testes desta caminhada.
type fixtureMedia struct {
	episodes *int
	format   MediaFormat
	status   MediaStatus
	prequel  int
	sequel   int
}

func eps(n int) *int { return &n }

// tvChain monta uma cadeia de TV finalizada a partir de pares (id, episodios), do mais antigo
// para o mais novo, ligando PREQUEL/SEQUEL nos dois sentidos — como a AniList devolve.
func tvChain(graph map[int]*fixtureMedia, ids []int, counts []int) {
	for i, id := range ids {
		node := &fixtureMedia{episodes: eps(counts[i]), format: MediaFormatTV, status: MediaStatusFinished}
		if i > 0 {
			node.prequel = ids[i-1]
			graph[ids[i-1]].sequel = id
		}
		graph[id] = node
	}
}

// mockSeriesAPI responde a query da caminhada a partir do grafo, renderizando EXATAMENTE dois
// niveis de relations e devolvendo `edges: []` no terceiro — que e o que a AniList faz de
// verdade (decisions.md #71). E a parte que importa do mock: um fake que expandisse a
// profundidade toda deixaria passar justamente o bug de tratar o nivel cortado como raiz.
func mockSeriesAPI(t *testing.T, graph map[int]*fixtureMedia) *int {
	t.Helper()
	requests := 0
	restore := MockAniListDo(func(req *http.Request) (*http.Response, error) {
		requests++

		body, _ := io.ReadAll(req.Body)
		var parsed struct {
			Variables struct {
				Ids []int `json:"ids"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Errorf("corpo da requisicao ilegivel: %v", err)
		}
		if len(parsed.Variables.Ids) > anilistMaxPerPage {
			t.Errorf("lote de %d ids passa do perPage maximo da AniList", len(parsed.Variables.Ids))
		}

		rendered := make([]string, 0, len(parsed.Variables.Ids))
		for _, id := range parsed.Variables.Ids {
			if node, ok := graph[id]; ok {
				rendered = append(rendered, renderMedia(graph, id, node, 0))
			}
		}
		return respond(200, `{"data":{"Page":{"media":[`+strings.Join(rendered, ",")+`]}}}`, nil), nil
	})
	t.Cleanup(restore)
	return &requests
}

func renderMedia(graph map[int]*fixtureMedia, id int, node *fixtureMedia, depth int) string {
	episodes := "null"
	if node.episodes != nil {
		episodes = strconv.Itoa(*node.episodes)
	}

	edges := []string{}
	if depth < 2 { // no terceiro nivel a AniList para de expandir
		for _, rel := range []struct {
			kind string
			id   int
		}{{"PREQUEL", node.prequel}, {"SEQUEL", node.sequel}} {
			if child, ok := graph[rel.id]; ok {
				edges = append(edges, fmt.Sprintf(`{"relationType":%q,"node":%s}`,
					rel.kind, renderMedia(graph, rel.id, child, depth+1)))
			}
		}
	}

	return fmt.Sprintf(`{"id":%d,"episodes":%s,"format":%q,"status":%q,"relations":{"edges":[%s]}}`,
		id, episodes, node.format, node.status, strings.Join(edges, ","))
}

// goldenChains sao as tres cadeias medidas contra a AniList real em 28/ago/2026 (decisions.md
// #71). Elas moram aqui, e nao num markdown, porque markdown apodrece em silencio e teste
// quebra o CI.
//
// O Shingeki e a validacao forte: `76..87` para o Final Season Part 2 e EXATAMENTE a numeracao
// que SubsPlease e Erai-raws usam nos nomes de arquivo. A cadeia reproduz a convencao real sem
// nenhum ajuste.
var goldenChains = []struct {
	root   int
	ids    []int
	counts []int
	first  []int // primeiro episodio absoluto de cada id
}{
	{
		root:   108465,
		ids:    []int{108465, 127720, 146065, 166873},
		counts: []int{11, 12, 13, 12},
		first:  []int{1, 12, 24, 37},
	},
	{
		root:   16498,
		ids:    []int{16498, 20958, 99147, 104578, 110277, 131681},
		counts: []int{25, 12, 12, 10, 16, 12},
		first:  []int{1, 26, 38, 50, 60, 76},
	},
	{
		root:   15689,
		ids:    []int{15689, 5081, 11597, 17074, 21262, 20918, 21745},
		counts: []int{4, 15, 11, 26, 12, 4, 7},
		first:  []int{1, 5, 20, 31, 57, 69, 73},
	},
}

func goldenGraph() map[int]*fixtureMedia {
	graph := map[int]*fixtureMedia{}
	for _, chain := range goldenChains {
		tvChain(graph, chain.ids, chain.counts)
	}
	return graph
}

// TestGetSeriesIndex_GoldenChains e a validacao real do eixo absoluto: as tres cadeias medidas,
// resolvidas de ponta a ponta. Duas delas tem altura maior que os 2 niveis que uma query da
// AniList expande, entao passar aqui prova que a caminhada continua depois do corte em vez de
// tratar `edges: []` como raiz.
func TestGetSeriesIndex_GoldenChains(t *testing.T) {
	graph := goldenGraph()
	requests := mockSeriesAPI(t, graph)

	// A semente sao SO as pontas — o cour que alguem esta assistindo. Semear a cadeia inteira
	// deixaria todo ancestral chegar como nivel 0 da resposta, e o corte de 2 niveis nunca
	// seria exercitado.
	var tips []int
	for _, chain := range goldenChains {
		tips = append(tips, chain.ids[len(chain.ids)-1])
	}

	index, err := GetSeriesIndex(tips, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	for _, chain := range goldenChains {
		checkGolden(t, index, chain.ids[len(chain.ids)-1], chain.root, chain.first[len(chain.ids)-1], chain.counts[len(chain.counts)-1])
	}

	// A cadeia mais alta tem 7 niveis e cada query anda 2, entao 3 rodadas dao conta das tres
	// cadeias juntas. O numero e o ponto da feature: 17 medias por uns poucos requests. Uma
	// implementacao que buscasse um ancestral por request passaria em tudo acima e estouraria
	// o orcamento da AniList em producao.
	if *requests > 4 {
		t.Errorf("quero no maximo 4 requests para as tres cadeias, vieram %d", *requests)
	}

	// Agora a cadeia inteira, para as posicoes do meio tambem ficarem travadas. Sai do cache.
	var all []int
	for _, chain := range goldenChains {
		all = append(all, chain.ids...)
	}
	index, err = GetSeriesIndex(all, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex (cadeia inteira): %v", err)
	}
	for _, chain := range goldenChains {
		for i, id := range chain.ids {
			checkGolden(t, index, id, chain.root, chain.first[i], chain.counts[i])
		}
	}
}

func checkGolden(t *testing.T, index map[int]Series, id, wantKey, wantFirst, count int) {
	t.Helper()
	series, ok := index[id]
	if !ok {
		t.Fatalf("media %d ficou de fora do indice", id)
	}
	if series.Key != wantKey {
		t.Errorf("media %d: quero seriesKey %d, veio %d", id, wantKey, series.Key)
	}
	if got, want := series.Offset+1, wantFirst; got != want {
		t.Errorf("media %d: quero abs %d..%d, veio %d..%d", id, want, want+count-1, got, got+count-1)
	}
}

// TestGetSeriesIndex_CacheAvoidsSecondWalk: o dado e imutavel e a caminhada e monotonica, entao
// a segunda leitura nao pode ir a rede. E o que torna o warm-up por passe barato.
func TestGetSeriesIndex_CacheAvoidsSecondWalk(t *testing.T) {
	requests := mockSeriesAPI(t, goldenGraph())

	if _, err := GetSeriesIndex([]int{131681}, PriorityDisposable); err != nil {
		t.Fatalf("primeira caminhada: %v", err)
	}
	before := *requests

	index, err := GetSeriesIndex([]int{131681}, PriorityDisposable)
	if err != nil {
		t.Fatalf("segunda caminhada: %v", err)
	}
	if *requests != before {
		t.Errorf("a segunda caminhada foi a rede %d vezes; o cache de 24h deveria cobrir tudo", *requests-before)
	}
	if index[131681].Offset != 75 {
		t.Errorf("a resposta do cache tem que ser a mesma: quero offset 75, veio %d", index[131681].Offset)
	}
}

// TestGetSeriesIndex_UnreleasedCountsZeroAndIsNotCached: anime sem contagem de episodios conta
// como 0 hoje e como outra coisa amanha. Persistir isso congelaria o zero por 24h e deslocaria
// toda a serie para baixo.
func TestGetSeriesIndex_UnreleasedCountsZeroAndIsNotCached(t *testing.T) {
	graph := map[int]*fixtureMedia{}
	tvChain(graph, []int{100, 200}, []int{12, 12})
	graph[100].episodes = nil
	graph[100].status = MediaStatusNotYetReleased

	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{200}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[200].Offset != 0 {
		t.Errorf("prequel sem contagem tem que somar 0, veio offset %d", index[200].Offset)
	}
	if _, cached := seriesCache.get("100"); cached {
		t.Error("prequel sem contagem de episodios nao pode entrar no cache de 24h")
	}
	if _, cached := seriesCache.get("200"); !cached {
		t.Error("o anime finalizado com contagem devia ter sido cacheado")
	}
}

// TestGetSeriesIndex_SkipsNonTVPrequel: filme de recap, OVA e ONA tambem chegam como PREQUEL e
// com episodes preenchido, e somar isso manda a busca para um episodio que nao existe. Mesma
// regra de ComputeEpisodeOffset (decisions.md #9 e #71).
func TestGetSeriesIndex_SkipsNonTVPrequel(t *testing.T) {
	graph := map[int]*fixtureMedia{
		300: {episodes: eps(4), format: MediaFormatOVA, status: MediaStatusFinished, sequel: 400},
		400: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 300},
	}
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{400}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[400].Offset != 0 || index[400].Key != 400 {
		t.Errorf("prequel de OVA nao pode entrar na cadeia: veio %+v", index[400])
	}
}

// TestGetSeriesIndex_MissingMediaIsAbsent: id que a AniList nao devolve nao vira offset 0 no
// mapa — vira ausencia. Confundir os dois faria um anime apagado da AniList parecer raiz de
// serie, com numeracao relativa silenciosamente errada.
func TestGetSeriesIndex_MissingMediaIsAbsent(t *testing.T) {
	graph := map[int]*fixtureMedia{}
	tvChain(graph, []int{500}, []int{12})
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{500, 999}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if _, ok := index[999]; ok {
		t.Error("id desconhecido da AniList tem que ficar AUSENTE do mapa, nao virar offset 0")
	}
	if _, ok := index[500]; !ok {
		t.Error("o id conhecido continua no mapa")
	}
}

// TestGetSeriesIndex_CycleTerminates: um grafo com PREQUEL circular (dado torto da AniList) nao
// pode pendurar o passe do daemon.
func TestGetSeriesIndex_CycleTerminates(t *testing.T) {
	graph := map[int]*fixtureMedia{
		600: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 700, sequel: 700},
		700: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 600, sequel: 600},
	}
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{600}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[600].Offset != 12 {
		t.Errorf("o ciclo tem que parar depois de um salto: quero offset 12, veio %d", index[600].Offset)
	}
}
