package anilist

import (
	"errors"
	"strconv"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// mediaByIDCache guarda as leituras por media id de animes avulsos. GET /api/v1/animes sofre
// poll de 30s por aba aberta e passa a chamar GetMediaByID uma vez por avulso — sem cache
// isso vira o mesmo caminho para o 429 que frontendListCache fechou (ver decisions.md #57).
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

// SearchMedia busca animes na AniList por termo. Nao e cacheado: cada tecla digitada e uma
// chave diferente, entao um cache so guardaria prefixos que ninguem repete — quem segura o
// volume aqui e o debounce do frontend.
//
// includeUnreleased=false (o padrao da tela) esconde os NOT_YET_RELEASED. O filtro e
// SERVER-SIDE de proposito: perPage e 20, e filtrar depois de receber devolveria buscas com 4
// resultados uteis em temporada de anuncios. So NOT_YET_RELEASED sai — CANCELLED e HIATUS ficam,
// porque um anime interrompido no meio da exibicao tem episodios baixaveis.
func SearchMedia(term string, includeUnreleased bool) ([]MediaSearchResult, error) {
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

	resp, err := sendAnilistRequest[response](query, RequestVariables{"q": term})
	if err != nil {
		return nil, err
	}
	return resp.Data.Page.Media, nil
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
func GetMediaByID(mediaID int) (*MediaList, error) {
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

	resp, err := sendAnilistRequest[response](query, RequestVariables{"id": mediaID})
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
func GetMediaByIDs(ids []int) (map[int]*MediaList, error) {
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

		resp, err := sendAnilistRequest[response](query, RequestVariables{"ids": chunk})
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
