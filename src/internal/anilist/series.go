package anilist

import (
	"strconv"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// Series e a posicao de um anime no eixo ABSOLUTO da sua serie — a numeracao continua que
// atravessa cour e part, e que boa parte dos grupos usa nos nomes de arquivo (SubsPlease e
// Erai numeram o Shingeki Final Season Part 2 como 76..87, nao 1..12).
//
// A AniList nao tem id de franquia: a cadeia de PREQUEL e a unica fonte, e busca por nome e
// armadilha (decisions.md #71). Key e o media id da RAIZ dessa cadeia — a identidade de serie
// que o schema nao oferece —, e Offset e a soma dos episodios de todos os ancestrais.
//
// O episodio absoluto de um episodio local ep e Offset + ep.
type Series struct {
	// Key e o media id do anime mais antigo alcancavel por PREQUEL. Dois animes da mesma serie
	// compartilham a Key; e por ela que "o pack ja baixado cobre este episodio?" faz sentido
	// entre anime_ids diferentes.
	Key int
	// Offset e quantos episodios vieram antes deste anime na serie. 0 na raiz.
	Offset int
}

// seriesLink e o fato imutavel sobre um media: quantos episodios ele tem e qual e o media id do
// prequel dele (0 = raiz). So e gravado para no cuja posicao na resposta era AUTORITATIVA — ver
// o comentario de walkSeries sobre o corte de 2 niveis.
type seriesLink struct {
	PrequelID int
	Episodes  int
}

// seriesCache guarda os links por media id. TTL longo porque o dado e imutavel (um anime
// terminado nao ganha episodio nem prequel novo) e a caminhada e monotonica: so ancestrais
// entram, e ancestral de anime terminado tambem esta terminado.
//
// Em memoria, sem arquivo: o warm-up inteiro de uma biblioteca custa ~4 requests
// (decisions.md #71/#72), entao persistir em disco seria manutencao e invalidacao a troco de
// quatro requisicoes por dia.
var seriesCache = newTTLCache[seriesLink]()

const seriesTTL = 24 * time.Hour

// seriesNode e um media na resposta da caminhada. E recursivo porque a query aninha relations,
// e Relations so tem conteudo confiavel nos dois primeiros niveis: no terceiro a AniList devolve
// edges vazio, que e INDISTINGUIVEL de "nao tem prequel" (decisions.md #71). Quem le precisa
// saber em que nivel esta — este tipo nao carrega essa informacao de proposito, para nao dar a
// impressao de que da para confiar nela em qualquer lugar.
type seriesNode struct {
	Id        int         `json:"id"`
	Episodes  *int        `json:"episodes"`
	Format    MediaFormat `json:"format"`
	Status    MediaStatus `json:"status"`
	Relations struct {
		Edges []struct {
			RelationType string     `json:"relationType"`
			Node         seriesNode `json:"node"`
		} `json:"edges"`
	} `json:"relations"`
}

type seriesResponse struct {
	Data struct {
		Page struct {
			Media []seriesNode `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

const seriesNodeFields = `id episodes format status`

// seriesQuery pede DOIS niveis de relations, que e o maximo que a AniList expande: pedir um
// terceiro so devolveria edges vazio (decisions.md #71). Um lote de ate 50 ids custa a mesma
// unidade de orcamento que um id so, entao a largura e de graca (decisions.md #72).
var seriesQuery = `
	query GetSeriesChains($ids: [Int]) {
		Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
			media(id_in: $ids, type: ANIME) {
				` + seriesNodeFields + `
				relations { edges { relationType node {
					` + seriesNodeFields + `
					relations { edges { relationType node {
						` + seriesNodeFields + `
					} } }
				} } }
			}
		}
	}
`

// GetSeriesIndex resolve o eixo absoluto de cada id pedido, caminhando a cadeia de PREQUEL para
// tras ate a raiz. Devolve um mapa id -> Series; um id AUSENTE do mapa e um id que a AniList nao
// devolveu (apagado, ou a busca falhou antes de chegar nele).
//
// A semente vem pronta de quem chama: este pacote nao pode importar files, e o conjunto certo
// (animes do passe mais os anime_id que ja tem episodio em disco) so o daemon conhece.
//
// O erro NAO invalida o mapa: o que ja foi resolvido antes dele continua la, pela mesma razao de
// GetMediaByIDs — meia resposta e melhor que nenhuma, e quem chama decide o que ela vale.
func GetSeriesIndex(ids []int, priority Priority) (map[int]Series, error) {
	known, err := walkSeries(ids, priority)

	result := make(map[int]Series, len(ids))
	for _, id := range ids {
		if series, ok := resolveSeries(known, id); ok {
			result[id] = series
		}
	}
	return result, err
}

// walkSeries e o BFS. Cada rodada consome uma fronteira de medias cuja cadeia ainda nao se
// conhece, e aprende DOIS elos por media buscada: o proprio e o do prequel dele, porque os
// niveis 0 e 1 da resposta vem em posicao autoritativa. O prequel do prequel (nivel 2) tem id e
// contagem de episodios confiaveis, mas relations CORTADO — ele volta para a fronteira em vez de
// ser tratado como raiz, que e a armadilha da decisions.md #71.
//
// O loop e agnostico a profundidade: o teto de 2 e comportamento observado, nao contrato. Ele
// consome o que vier e enfileira o que faltar, entao uma AniList que passe a expandir 3 niveis
// so faria a caminhada terminar em menos rodadas.
func walkSeries(seed []int, priority Priority) (map[int]seriesLink, error) {
	known := make(map[int]seriesLink, len(seed))
	seen := make(map[int]bool, len(seed)) // protege contra ciclo e contra buscar o mesmo id duas vezes
	var frontier []int

	var enqueue func(id int)
	enqueue = func(id int) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		if cached, ok := seriesCache.get(strconv.Itoa(id)); ok {
			known[id] = cached
			enqueue(cached.PrequelID) // a cadeia pode continuar em nos que ainda nao estao no cache
			return
		}
		frontier = append(frontier, id)
	}

	for _, id := range seed {
		enqueue(id)
	}

	for len(frontier) > 0 {
		batch := frontier
		frontier = nil

		for start := 0; start < len(batch); start += anilistMaxPerPage {
			chunk := batch[start:min(start+anilistMaxPerPage, len(batch))]

			resp, err := sendAnilistRequest[seriesResponse](seriesQuery, RequestVariables{"ids": chunk}, priority)
			if err != nil {
				logger.Logger.Warn().Err(err).Int("media_ids", len(chunk)).
					Msg("Failed to walk a batch of the AniList prequel chain")
				return known, err
			}

			for _, media := range resp.Data.Page.Media {
				// Nivel 0 e nivel 1: relations autoritativo, o elo pode ser gravado.
				prequel := recordLink(known, media)
				if prequel == nil {
					continue
				}
				seen[prequel.Id] = true
				grandparent := recordLink(known, *prequel)
				if grandparent == nil {
					continue
				}
				// Nivel 2: sabemos que ele existe e quantos episodios tem, mas nao o que vem
				// antes dele. Volta para a fronteira.
				enqueue(grandparent.Id)
			}
		}
	}

	return known, nil
}

// recordLink grava o elo de um no cuja posicao na resposta era autoritativa e devolve o prequel
// dele, ou nil se ele e raiz. So no FINISHED com contagem de episodios vai para o cache: um
// anime em exibicao ainda pode ganhar episodio, e um sem contagem (nao lancado) conta como 0
// hoje e como outra coisa amanha.
func recordLink(known map[int]seriesLink, node seriesNode) *seriesNode {
	link := seriesLink{}
	if node.Episodes != nil {
		link.Episodes = *node.Episodes
	}

	prequel := prequelOf(node)
	if prequel != nil {
		link.PrequelID = prequel.Id
	}

	known[node.Id] = link
	if node.Status == MediaStatusFinished && node.Episodes != nil {
		seriesCache.set(strconv.Itoa(node.Id), link, seriesTTL)
	}
	return prequel
}

// prequelOf devolve o prequel de TV/TV_SHORT do no. Filme de recap, OVA e ONA tambem chegam como
// PREQUEL e com Episodes preenchido, mas nao entram na contagem que os grupos seguem — mesma
// regra de daemon.ComputeEpisodeOffset (decisions.md #9 e #71).
//
// TODO(F8): esta regra de selecao esta duplicada aqui e em daemon.ComputeEpisodeOffset
// (daemon/helpers.go). Nao unificar antes do F8: o plano rebaixa ComputeEpisodeOffset a UMA das
// hipoteses de numeracao de pack, e as duas podem divergir de proposito no caminho. Se o F8 for
// descartado, unificar — extrair daqui a selecao e fazer o daemon chamar.
func prequelOf(node seriesNode) *seriesNode {
	for i := range node.Relations.Edges {
		edge := &node.Relations.Edges[i]
		if edge.RelationType != "PREQUEL" {
			continue
		}
		if edge.Node.Format != MediaFormatTV && edge.Node.Format != MediaFormatTVShort {
			continue
		}
		return &edge.Node
	}
	return nil
}

// resolveSeries soma a cadeia para tras a partir de id. Para no primeiro ancestral desconhecido:
// na duvida a caminhada resolve para o menor offset possivel, porque offset ausente cai na
// numeracao relativa (que boa parte dos grupos usa) e offset errado manda a busca para um
// episodio que nao existe (decisions.md #71).
func resolveSeries(known map[int]seriesLink, id int) (Series, bool) {
	link, ok := known[id]
	if !ok {
		return Series{}, false
	}

	series := Series{Key: id}
	visited := map[int]bool{id: true}
	for parent := link.PrequelID; parent != 0; {
		ancestor, ok := known[parent]
		if !ok || visited[parent] {
			break
		}
		visited[parent] = true
		series.Offset += ancestor.Episodes
		series.Key = parent
		parent = ancestor.PrequelID
	}
	return series, true
}
