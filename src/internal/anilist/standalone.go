package anilist

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// mediaByIDCache guarda as leituras por media id de animes avulsos, e e compartilhado por
// GetMediaByID e GetMediaByIDs — chaveado POR ID, nunca por lote, senao um avulso novo
// invalidaria a leitura de todos os outros.
//
// O caminho que ele protege e o de GET /api/v1/animes, que sofre poll de 30s por aba aberta e
// le a lista inteira de avulsos a cada vez. Desde o lote (decisions.md #65 e #72) isso e UMA
// query por 50 ids, e nao mais uma por avulso; o cache continua valendo porque com ele o poll
// so vai a rede quando algum id vence — o mesmo motivo pelo qual frontendListCache existe
// (decisions.md #57).
var mediaByIDCache = newTTLCache[*MediaList]()

const mediaByIDTTL = 60 * time.Second

// MediaSearchResult e uma linha da barra de busca da tela de adicionar anime. E deliberadamente
// mais pobre que Media: a busca traz 20 resultados e nenhum deles precisa de synonyms,
// relations ou airingSchedule — quem adiciona o anime busca esses campos com GetMediaByID.
type MediaSearchResult struct {
	Id         int         `json:"id"`
	Title      Title       `json:"title"`
	Format     MediaFormat `json:"format"`
	Status     MediaStatus `json:"status"`
	SeasonYear int         `json:"seasonYear"`
	Episodes   *int        `json:"episodes"`
	CoverImage CoverImage  `json:"coverImage"`
}

// searchCache guarda o resultado por termo. Passou a valer quando a tela deixou de buscar a
// cada tecla (decisions.md #50 (f)): com submit, as chaves sao termos inteiros que se repetem —
// o mesmo usuario refazendo a busca, o toggle de nao lancados indo e voltando, outra aba
// buscando o mesmo anime. A chave e minuscula porque a busca da AniList e case-insensitive,
// entao "One Piece" e "one piece" devolvem a mesma coisa e nao ha por que gastar dois requests.
//
// O cache fica em SearchMedia, e NAO na resposta do endpoint, porque block_reason e recalculado
// pelo standaloneGuard a cada resposta: cacheado junto, um anime adicionado agora continuaria
// aparecendo com o botao "Adicionar" ate o TTL vencer.
var searchCache = newTTLCache[[]MediaSearchResult]()

const (
	searchTTL = 10 * time.Minute
	// ponytail: teto por descarte total, nao por LRU. Este e o unico cache com chave de texto
	// livre — nada impede alguem de digitar mil termos diferentes, e entrada vencida nunca sai
	// do mapa. Mil resultados de busca sao poucos MB; se um dia isso incomodar, uma fila de
	// insercao com descarte do mais antigo troca o flush por eviction de verdade.
	searchCacheMaxEntries = 1000
)

func searchCacheKey(term string, includeUnreleased bool) string {
	return strings.ToLower(term) + "|" + strconv.FormatBool(includeUnreleased)
}

// SearchMedia busca animes na AniList por termo, com cache de searchTTL.
//
// includeUnreleased=false (o padrao da tela) esconde os NOT_YET_RELEASED. O filtro e
// SERVER-SIDE de proposito: perPage e 20, e filtrar depois de receber devolveria buscas com 4
// resultados uteis em temporada de anuncios. So NOT_YET_RELEASED sai — CANCELLED e HIATUS ficam,
// porque um anime interrompido no meio da exibicao tem episodios baixaveis.
func SearchMedia(term string, includeUnreleased bool) ([]MediaSearchResult, error) {
	key := searchCacheKey(term, includeUnreleased)
	if cached, ok := searchCache.get(key); ok {
		return slices.Clone(cached), nil
	}

	// Concatenacao, e nao uma variavel GraphQL com valor nulo: a AniList ignora argumentos nulos
	// hoje, mas depender disso faria o filtro morrer em silencio se a semantica mudasse. Aqui,
	// quando includeUnreleased e true, o argumento nao existe na query.
	statusFilter := ", status_not: NOT_YET_RELEASED"
	if includeUnreleased {
		statusFilter = ""
	}

	query := `
		query SearchMedia($q: String) {
			Page(perPage: 20) {
				media(search: $q, type: ANIME, sort: SEARCH_MATCH` + statusFilter + `) {
					id
					title {
						english
						romaji
					}
					format
					status
					seasonYear
					episodes
					coverImage {
						large
						medium
					}
				}
			}
		}
	`

	type response struct {
		Data struct {
			Page struct {
				Media []MediaSearchResult `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"q": term}, PriorityDisposable)
	if err != nil {
		// Recusado pelo gate de orcamento: o catalogo da AniList nao muda em dez minutos, entao
		// servir o resultado vencido deixa a tela funcionando e devolve o balde para o passe do
		// daemon, que e o unico que nao pode ser adiado. Ver decisions.md #72.
		if errors.Is(err, ErrBudgetLow) {
			if stale, ok := searchCache.getStale(key); ok {
				return slices.Clone(stale), nil
			}
		}
		return nil, err
	}

	if searchCache.size() >= searchCacheMaxEntries {
		searchCache.clear()
	}
	searchCache.set(key, resp.Data.Page.Media, searchTTL)
	// Copia: a fatia guardada e compartilhada por todos os requests que acertarem esta chave, e
	// quem chama nao pode reordenar nem cortar a memoria do cache.
	return slices.Clone(resp.Data.Page.Media), nil
}

// mediaByIDFields sao os campos que GetMediaByID e GetMediaByIDs pedem — os MESMOS de
// getMediaListEntry (inclusive synonyms, relations e o id de cada no do airingSchedule): a busca
// por anime e searchNyaaForSingleEpisode dependem de synonyms e relations (offset de temporada
// dividida via PREQUEL), e todo o resto do app chaveia episodio pelo id do no.
const mediaByIDFields = `
	id
	episodes
	format
	status
	title {
		english
		romaji
		native
	}
	synonyms
	relations {
		edges {
			node {
				title {
					english
					romaji
				}
				synonyms
				episodes
				format
			}
			relationType
		}
	}
	coverImage {
		large
		medium
	}
	airingSchedule {
		nodes {
			airingAt
			timeUntilAiring
			episode
			id
		}
	}
	nextAiringEpisode {
		episode
		airingAt
		timeUntilAiring
	}
`

// GetMediaByID le um anime pelo media id SEM passar por lista de conta nenhuma — o primitivo
// que faltava para um anime avulso existir para o app (GetAnimeInfo devolve nil quando nenhuma
// conta o acompanha).
//
// Devolve um MediaList SINTETICO: so Media preenchido, Progress 0, Status "" e Id 0. Os zeros
// sao parte do contrato — DedupeByMedia mantem o MENOR progresso, entao um Progress inventado
// aqui venceria a entrada real e o daemon rebaixaria episodios ja assistidos.
//
// O MediaList SINTETICO e o mesmo contrato de GetMediaByIDs, que e quem serve a lista inteira
// de avulsos; este aqui fica para o lookup de um id so.
//
// (nil, nil) quando a AniList nao conhece o id.
func GetMediaByID(mediaID int, priority Priority) (*MediaList, error) {
	key := strconv.Itoa(mediaID)
	if cached, ok := mediaByIDCache.get(key); ok {
		return copyMediaList(cached), nil
	}

	query := `
		query GetMediaByID($id: Int) {
			Media(id: $id, type: ANIME) {` + mediaByIDFields + `}
		}
	`

	type response struct {
		Data struct {
			Media *Media `json:"Media"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"id": mediaID}, priority)
	if errors.Is(err, ErrNotFound) {
		mediaByIDCache.set(key, nil, mediaByIDTTL)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if resp.Data.Media == nil {
		logger.Logger.Debug().Int("media_id", mediaID).Msg("AniList does not know this media id")
		mediaByIDCache.set(key, nil, mediaByIDTTL)
		return nil, nil
	}

	ml := &MediaList{Media: *resp.Data.Media}
	mediaByIDCache.set(key, ml, mediaByIDTTL)
	return copyMediaList(ml), nil
}

// GetMediaByIDs le varios animes por media id numa unica query. Substitui o loop de
// GetMediaByID nos dois lugares que percorrem a lista de avulsos inteira — o consumidor
// dominante do orcamento da AniList (decisions.md #65), que gastava 1 request por avulso a cada
// passe do daemon e a cada poll de 30s do frontend.
//
// Devolve um mapa id -> MediaList, com tres desfechos distintos por id:
//
//   - entrada com valor: o anime veio (do cache ou da rede);
//   - entrada com nil: a AniList NAO conhece esse id. id_in omite silenciosamente o media
//     apagado, entao "nao voltou" e a unica forma de descobrir isso — e equivale ao (nil, nil)
//     de GetMediaByID;
//   - id ausente do mapa: nao deu para buscar (rede, 429). So esse caso vem com erro.
//
// O erro devolvido NAO invalida o mapa: as paginas que passaram antes dele ja estao la.
func GetMediaByIDs(ids []int, priority Priority) (map[int]*MediaList, error) {
	// O cache continua por id, e nao por lote: chaveado pelo lote, um avulso novo invalidaria
	// a leitura de todos os outros.
	found := make(map[int]*MediaList, len(ids))
	seen := make(map[int]bool, len(ids))
	missing := make([]int, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if cached, ok := mediaByIDCache.get(strconv.Itoa(id)); ok {
			found[id] = copyMediaList(cached)
			continue
		}
		missing = append(missing, id)
	}

	query := `
		query GetMediaByIDs($ids: [Int]) {
			Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
				media(id_in: $ids, type: ANIME) {` + mediaByIDFields + `}
			}
		}
	`

	type response struct {
		Data struct {
			Page struct {
				Media []Media `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	for start := 0; start < len(missing); start += anilistMaxPerPage {
		chunk := missing[start:min(start+anilistMaxPerPage, len(missing))]

		resp, err := sendAnilistRequest[response](query, RequestVariables{"ids": chunk}, priority)
		if err != nil {
			return found, err
		}

		for i := range resp.Data.Page.Media {
			ml := &MediaList{Media: resp.Data.Page.Media[i]}
			mediaByIDCache.set(strconv.Itoa(ml.Media.Id), ml, mediaByIDTTL)
			found[ml.Media.Id] = copyMediaList(ml)
		}

		for _, id := range chunk {
			if _, ok := found[id]; !ok {
				logger.Logger.Debug().Int("media_id", id).Msg("AniList does not know this media id")
				mediaByIDCache.set(strconv.Itoa(id), nil, mediaByIDTTL)
				found[id] = nil
			}
		}
	}

	return found, nil
}

// copyMediaList devolve uma copia rasa da entrada guardada no cache. Mesma razao de
// frontendListResponse: quem chama sobrescreve campos da entrada (CustomLists, Progress), e
// entregar o ponteiro guardado deixaria dois requests concorrentes escrevendo na mesma memoria.
func copyMediaList(ml *MediaList) *MediaList {
	if ml == nil {
		return nil
	}
	copied := *ml
	return &copied
}
