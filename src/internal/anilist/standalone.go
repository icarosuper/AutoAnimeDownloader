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

// GetMediaByID le um anime pelo media id SEM passar por lista de conta nenhuma — o primitivo
// que faltava para um anime avulso existir para o app (GetAnimeInfo devolve nil quando nenhuma
// conta o acompanha).
//
// Devolve um MediaList SINTETICO: so Media preenchido, Progress 0, Status "" e Id 0. Os zeros
// sao parte do contrato — DedupeByMedia mantem o MENOR progresso, entao um Progress inventado
// aqui venceria a entrada real e o daemon rebaixaria episodios ja assistidos.
//
// Os campos pedidos sao os MESMOS de getMediaListEntry (inclusive synonyms, relations e o id
// de cada no do airingSchedule): a busca por anime e searchNyaaForSingleEpisode dependem
// de synonyms e relations (offset de temporada dividida via PREQUEL), e todo o resto do app
// chaveia episodio pelo id do no.
//
// (nil, nil) quando a AniList nao conhece o id.
func GetMediaByID(mediaID int) (*MediaList, error) {
	key := strconv.Itoa(mediaID)
	if cached, ok := mediaByIDCache.get(key); ok {
		return copyMediaList(cached), nil
	}

	query := `
		query GetMediaByID($id: Int) {
			Media(id: $id, type: ANIME) {
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
			}
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
