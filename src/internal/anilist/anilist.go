package anilist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// ttlCache guarda respostas da AniList por um tempo curto. A AniList hoje limita a 30 req/min
// por IP e o frontend faz poll de /api/v1/animes a cada 30s por aba aberta, com uma busca por
// conta configurada: sem cache, duas abas e duas contas ja estouram o orcamento sozinhas e o
// 429 resultante derruba tambem o ciclo de verificacao do daemon (ver decisions.md #11 e #57).
type ttlCache[T any] struct {
	mu     sync.Mutex
	data   map[string]T
	expiry map[string]time.Time
}

func newTTLCache[T any]() *ttlCache[T] {
	return &ttlCache[T]{data: make(map[string]T), expiry: make(map[string]time.Time)}
}

func (c *ttlCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.expiry[key]) {
		return c.data[key], true
	}
	var zero T
	return zero, false
}

func (c *ttlCache[T]) set(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	c.expiry[key] = time.Now().Add(ttl)
}

func (c *ttlCache[T]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.data)
	clear(c.expiry)
}

// ponytail: sem protecao contra stampede — N polls simultaneos que erram o cache disparam N
// buscas. Os timers das abas sao desencontrados, entao o TTL sozinho ja segura o volume; se um
// dia isso nao bastar, serializar as buscas por chave resolve.
var (
	customListsCache  = newTTLCache[map[int]CustomLists]()
	frontendListCache = newTTLCache[[]MediaList]()
)

const (
	customListsTTL = 5 * time.Minute
	// Uma resposta vazia pode ser tanto "a conta nao tem custom lists" quanto um campo que a
	// AniList degradou. O TTL curto limita os dois: o poll para de bater na API a cada request
	// e um campo degradado se corrige em meio minuto.
	customListsEmptyTTL = 30 * time.Second
	frontendListTTL     = 60 * time.Second
)

var httpDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

var aniListAPIURL = "https://graphql.anilist.co"

// ErrNotFound sinaliza que a AniList respondeu 404: o objeto consultado nao existe.
var ErrNotFound = errors.New("anilist: not found")

func init() {
	if url := os.Getenv("ANILIST_API_URL"); url != "" {
		aniListAPIURL = url
	}
}

// MockAniListDo troca o transporte HTTP e limpa os caches nas duas pontas: um teste que instala
// um mock precisa ver as respostas dele, nao as do teste anterior.
func MockAniListDo(fn func(*http.Request) (*http.Response, error)) (restore func()) {
	prev := httpDo
	clearCaches()
	if fn != nil {
		httpDo = fn
	}
	return func() { httpDo = prev; clearCaches() }
}

func clearCaches() {
	customListsCache.clear()
	frontendListCache.clear()
	mediaByIDCache.clear()
	// A saude tambem e estado de pacote: sem zerar aqui, um teste que simula 403 deixaria o
	// proximo teste enxergando a AniList fora do ar.
	health.Store(&Health{State: HealthOK})
}

type AniListResponse struct {
	Data struct {
		Page struct {
			MediaList []MediaList `json:"mediaList"`
		} `json:"Page"`
	} `json:"data"`
}

type MediaListStatus string

const (
	MediaListStatusCompleted MediaListStatus = "COMPLETED"
	MediaListStatusDropped   MediaListStatus = "DROPPED"
	MediaListStatusPaused    MediaListStatus = "PAUSED"
	MediaListStatusPlanning  MediaListStatus = "PLANNING"
	MediaListStatusRepeating MediaListStatus = "REPEATING"
	MediaListStatusCurrent   MediaListStatus = "CURRENT"
)

type MediaList struct {
	Id          int             `json:"id"`
	Status      MediaListStatus `json:"status"`
	Progress    int             `json:"progress"`
	CustomLists CustomLists     `json:"customLists"`
	Media       Media           `json:"media"`
}

type MediaStatus string

const (
	MediaStatusFinished       MediaStatus = "FINISHED"
	MediaStatusReleasing      MediaStatus = "RELEASING"
	MediaStatusNotYetReleased MediaStatus = "NOT_YET_RELEASED"
	MediaStatusCancelled      MediaStatus = "CANCELLED"
	MediaStatusHiatus         MediaStatus = "HIATUS"
)

func MediaStatusAllowed(allowedStatuses []string, status MediaStatus) bool {
	for _, s := range allowedStatuses {
		if s == string(status) {
			return true
		}
	}
	return false
}

type CoverImage struct {
	Large  string `json:"large"`
	Medium string `json:"medium"`
}

type Media struct {
	Id             int            `json:"id"`
	Status         MediaStatus    `json:"status"`
	Format         MediaFormat    `json:"format"`
	Title          Title          `json:"title"`
	Episodes       *int           `json:"episodes"`
	Synonyms       []string       `json:"synonyms"`
	Relations      MediaRelations `json:"relations"`
	CoverImage     CoverImage     `json:"coverImage"`
	AiringSchedule AiringSchedule `json:"airingSchedule"`
	// NextAiringEpisode e a fonte de "qual foi o ultimo episodio no ar" para os animes cujo
	// airingSchedule a AniList ja clipou (ver EpisodeList e decisions.md #52). nil quando o
	// anime terminou ou nao tem data marcada.
	NextAiringEpisode *AiringNode `json:"nextAiringEpisode"`
}

type Title struct {
	English *string `json:"english"`
	Romaji  *string `json:"romaji"`
	// Native e o titulo no idioma original (kanji, na pratica). So a busca da dashboard usa —
	// o daemon monta as queries do Nyaa com romaji/english, ver search.go.
	Native *string `json:"native"`
}

type MediaRelationNode struct {
	Title    Title    `json:"title"`
	Synonyms []string `json:"synonyms"`
	Episodes *int     `json:"episodes"`
	// Format separa a prequel que conta episodios da que nao conta: um filme de recap ou uma
	// OVA tambem chegam como PREQUEL e com Episodes preenchido, e somar isso no offset joga a
	// busca para um numero de episodio que nao existe. Ver ComputeEpisodeOffset.
	Format MediaFormat `json:"format"`
}

type MediaRelationEdge struct {
	Node         MediaRelationNode `json:"node"`
	RelationType string            `json:"relationType"`
}

type MediaRelations struct {
	Edges []MediaRelationEdge `json:"edges"`
}

type AiringSchedule struct {
	Nodes []AiringNode `json:"nodes"`
}

type AiringNode struct {
	ID              int   `json:"id"`
	Episode         int   `json:"episode"`
	TimeUntilAiring int   `json:"timeUntilAiring"`
	AiringAt        int64 `json:"airingAt"`
}

type GraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type MediaFormat string

const (
	MediaFormatTV      MediaFormat = "TV"
	MediaFormatTVShort MediaFormat = "TV_SHORT"
	MediaFormatMovie   MediaFormat = "MOVIE"
	MediaFormatOVA     MediaFormat = "OVA"
	MediaFormatONA     MediaFormat = "ONA"
	MediaFormatMusic   MediaFormat = "MUSIC"
	MediaFormatManga   MediaFormat = "MANGA"
	MediaFormatNovel   MediaFormat = "NOVEL"
	MediaFormatOneShot MediaFormat = "ONE_SHOT"
)

type CustomLists map[string]bool

// DedupeByMedia collapses the same anime appearing across multiple linked accounts into a
// single entry. MediaList.Id is the per-account *entry* id, so one anime tracked by two
// accounts arrives as two entries with different Ids and the same Media.Id — which is why
// Media.Id, and not MediaList.Id, is this app's anime identity (see decisions.md #43).
//
// The entry with the LOWEST progress wins: every download/keep/delete decision is keyed by
// airing-schedule episode ID (shared across accounts), so duplicates with divergent progress
// would fight — the account further ahead deleting episodes another account hasn't watched
// yet. An episode is only "watched" once all accounts have seen it.
//
// Status is NOT resolved here: the winning entry's Status belongs to one arbitrary account,
// so nothing may branch on it. Status is a per-account question answered by
// downloadableMediaIDs / deletableMediaIDs, which apply the OR/AND rules across accounts.
func DedupeByMedia(list []MediaList) []MediaList {
	byMedia := make(map[int]int, len(list)) // media id -> index in result
	result := make([]MediaList, 0, len(list))
	for _, anime := range list {
		if anime.Media.Id == 0 { // no media id (shouldn't happen) — keep as-is
			result = append(result, anime)
			continue
		}
		if idx, ok := byMedia[anime.Media.Id]; ok {
			if anime.Progress < result[idx].Progress {
				result[idx] = anime
			}
			continue
		}
		byMedia[anime.Media.Id] = len(result)
		result = append(result, anime)
	}
	return result
}

type RequestVariables map[string]any

func sendAnilistRequest[T any](query string, variables RequestVariables) (*T, error) {
	jsonData, err := json.Marshal(GraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", aniListAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logger.Logger.Debug().Str("url", aniListAPIURL).Msg("Sending Anilist request")

	resp, err := httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// A AniList responde 404 quando o objeto pedido nao existe (por exemplo, uma entrada de
		// lista que o usuario apagou). E uma resposta valida, nao uma falha — quem consulta por
		// id precisa distinguir isso de "a AniList caiu".
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		// O corpo carrega o motivo real (rate limit vs. API desligada vs. query invalida);
		// sem ele o log vira um numero sem diagnostico. A mensagem tambem alimenta o banner:
		// um 403 de IP bloqueado explica o motivo por escrito, e so a AniList sabe qual e.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_, message := firstError(snippet)
		setHealth(classify(resp.StatusCode), message, retryAfter(resp.Header.Get("Retry-After")))
		logger.Logger.Warn().Int("status_code", resp.StatusCode).Str("body", string(snippet)).Msg("Anilist returned non-200 status")
		// A mensagem extraida, e nao o corpo cru: o erro sobe ate a tela do usuario, e um dump
		// de JSON com locations e column nao diz nada a ninguem. O corpo inteiro continua no log
		// acima, que e onde ele serve para alguma coisa.
		if message == "" {
			message = strings.TrimSpace(string(snippet))
		}
		return nil, fmt.Errorf("anilist returned %d: %s", resp.StatusCode, message)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	// Um 200 pode carregar erro no envelope — a doc da AniList e explicita quanto a isso. Sem
	// esta leitura o caso vira resposta vazia sem diagnostico. Nao e o mesmo problema do
	// customLists null da decisions.md #11: aquele volta 200 SEM campo errors. Ver #65.
	if status, message := firstError(body); status != 0 {
		setHealth(classify(status), message, time.Time{})
		logger.Logger.Warn().Int("graphql_status", status).Str("message", message).Msg("Anilist returned an error inside a 200 response")
		return nil, fmt.Errorf("API returned error %d: %s", status, message)
	}
	clearHealth()

	logger.Logger.Debug().Int("status_code", resp.StatusCode).Int("body_size", len(body)).Msg("Anilist response received")

	var response T
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}

// GetCustomListsMap fetches a lightweight map of MediaList ID → CustomLists via a minimal query.
// Results are cached for 5 minutes so repeated calls (e.g. from the API endpoint) don't hit
// Anilist's rate limit. Only a response with at least one non-null CustomLists entry is cached.
func GetCustomListsMap(userName string, statuses []string) map[int]CustomLists {
	key := userName + "\x00" + strings.Join(statuses, "\x00")

	if result, ok := customListsCache.get(key); ok {
		return result
	}

	type miniEntry struct {
		Id          int         `json:"id"`
		CustomLists CustomLists `json:"customLists"`
	}
	type miniResponse struct {
		Data struct {
			Page struct {
				MediaList []miniEntry `json:"mediaList"`
			} `json:"Page"`
		} `json:"data"`
	}

	query := `query($u:String,$t:MediaType,$s:[MediaListStatus]){Page{mediaList(userName:$u,type:$t,status_in:$s){id customLists}}}`
	resp, err := sendAnilistRequest[miniResponse](query, RequestVariables{
		"u": userName,
		"t": "ANIME",
		"s": statuses,
	})
	if err != nil {
		logger.Logger.Warn().Err(err).Str("username", userName).Msg("Failed to fetch customLists map")
		return nil
	}

	m := make(map[int]CustomLists, len(resp.Data.Page.MediaList))
	hasData := false
	for _, ml := range resp.Data.Page.MediaList {
		m[ml.Id] = ml.CustomLists
		if len(ml.CustomLists) > 0 {
			hasData = true
		}
	}

	if hasData {
		customListsCache.set(key, m, customListsTTL)
	} else {
		customListsCache.set(key, m, customListsEmptyTTL)
	}

	return m
}

func GetAllCurrentAnime(userName string, statuses []string) (*AniListResponse, error) {
	query := `
		query GetAllCurrentAnime($userName: String, $type: MediaType, $statuses: [MediaListStatus]) {
			Page {
				mediaList(userName: $userName, type: $type, status_in: $statuses) {
					id
					status
					progress
					customLists
					media {
						id
						format
						status
						episodes
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
						airingSchedule {
							nodes {
								id
								episode
								timeUntilAiring
							}
						}
						nextAiringEpisode {
							episode
							airingAt
							timeUntilAiring
						}
					}
				}
			}
		}
	`

	if len(statuses) == 0 {
		return &AniListResponse{}, nil
	}

	variables := RequestVariables{
		"userName": userName,
		"type":     "ANIME",
		"statuses": statuses,
	}

	return sendAnilistRequest[AniListResponse](query, variables)
}

// GetFrontendAnimeList alimenta /api/v1/animes, que o frontend faz poll a cada 30s por aba.
// O resultado fica em cache por frontendListTTL: sem isso cada aba aberta multiplicava a
// pressao sobre o limite da AniList ate o 429, que por sua vez fazia o ciclo de download do
// daemon falhar junto. A lista so muda quando o usuario mexe na AniList, entao um minuto de
// atraso na tela e barato perto de derrubar o daemon.
func GetFrontendAnimeList(userName string, statuses []string) (*AniListResponse, error) {
	query := `
		query GetFrontendAnimeList($userName: String, $type: MediaType, $statuses: [MediaListStatus]) {
			Page {
				mediaList(userName: $userName, type: $type, status_in: $statuses) {
					id
					progress
					customLists
					media {
						id
						title {
							english
							romaji
							native
						}
						synonyms
						episodes
						status
						coverImage {
							large
							medium
						}
						airingSchedule {
							nodes {
								episode
								timeUntilAiring
							}
						}
						nextAiringEpisode {
							episode
							airingAt
							timeUntilAiring
						}
					}
				}
			}
		}
	`

	if len(statuses) == 0 {
		return &AniListResponse{}, nil
	}

	key := userName + "\x00" + strings.Join(statuses, "\x00")
	if list, ok := frontendListCache.get(key); ok {
		return frontendListResponse(list), nil
	}

	variables := RequestVariables{
		"userName": userName,
		"type":     "ANIME",
		"statuses": statuses,
	}

	resp, err := sendAnilistRequest[AniListResponse](query, variables)
	if err != nil {
		return nil, err
	}

	frontendListCache.set(key, resp.Data.Page.MediaList, frontendListTTL)
	// Copia: quem chama sobrescreve CustomLists nas entradas, e devolver a fatia guardada
	// deixaria dois requests concorrentes escrevendo na mesma memoria.
	return frontendListResponse(resp.Data.Page.MediaList), nil
}

func frontendListResponse(list []MediaList) *AniListResponse {
	resp := &AniListResponse{}
	resp.Data.Page.MediaList = append([]MediaList(nil), list...)
	return resp
}

// GetAnimeInfo returns one anime's data by MEDIA id, collapsed across every configured account:
// the media fields come from whichever account tracks it and Progress is the LOWEST among them
// (same rule as DedupeByMedia — an episode is only "watched" once every account has seen it).
//
// Returns (nil, nil) when no account has this media in its list: that is a normal state (the
// anime was removed from the lists but its episodes are still on disk), not an error. An error
// is only returned when every account's request failed, so the caller can tell "not tracked"
// from "AniList is down".
func GetAnimeInfo(mediaId int, usernames []string) (*MediaList, error) {
	var entries []MediaList
	var lastErr error
	for _, username := range usernames {
		resp, err := getMediaListEntry(username, mediaId)
		if err != nil {
			logger.Logger.Warn().Err(err).Str("username", username).Int("media_id", mediaId).
				Msg("Failed to fetch anime info for account")
			lastErr = err
			continue
		}
		entries = append(entries, resp.Data.Page.MediaList...)
	}

	if len(entries) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to fetch anime info for media %d: %w", mediaId, lastErr)
		}
		return nil, nil
	}

	deduped := DedupeByMedia(entries)
	return &deduped[0], nil
}

// GetMediaIDForEntry resolves a legacy MediaList *entry* id to its media id, and is the only
// place left that keys anything by entry id: the one-shot AnimeID migration (decisions.md #43)
// needs it to rewrite records written before Media.Id became the identity.
// Returns 0 when the entry no longer exists — the anime was removed from that account's list.
func GetMediaIDForEntry(mediaListId int) (int, error) {
	query := `
		query GetMediaIDForEntry($mediaListId: Int) {
			MediaList(id: $mediaListId) {
				media {
					id
				}
			}
		}
	`

	type response struct {
		Data struct {
			MediaList *struct {
				Media struct {
					Id int `json:"id"`
				} `json:"media"`
			} `json:"MediaList"`
		} `json:"data"`
	}

	resp, err := sendAnilistRequest[response](query, RequestVariables{"mediaListId": mediaListId})
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if resp.Data.MediaList == nil {
		return 0, nil
	}
	return resp.Data.MediaList.Media.Id, nil
}

// GetMediaListStatus returns the account's list status for one media. The bool is false when the
// account does not track this media at all — a distinction the delete rule depends on: an account
// that never had the anime cannot veto its deletion, but one holding it in a non-delete status can.
func GetMediaListStatus(username string, mediaId int) (MediaListStatus, bool, error) {
	query := `
		query GetMediaListStatus($userName: String, $mediaId: Int) {
			Page {
				mediaList(userName: $userName, mediaId: $mediaId) {
					status
				}
			}
		}
	`

	resp, err := sendAnilistRequest[AniListResponse](query, RequestVariables{
		"userName": username,
		"mediaId":  mediaId,
	})
	if err != nil {
		return "", false, err
	}
	if len(resp.Data.Page.MediaList) == 0 {
		return "", false, nil
	}
	return resp.Data.Page.MediaList[0].Status, true, nil
}

func getMediaListEntry(userName string, mediaId int) (*AniListResponse, error) {
	query := `
		query GetAnimeEpisodes($userName: String, $mediaId: Int) {
			Page {
				mediaList(userName: $userName, mediaId: $mediaId) {
					id
					status
					progress
					customLists
					media {
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
					}
				}
			}
		}
	`

	return sendAnilistRequest[AniListResponse](query, RequestVariables{
		"userName": userName,
		"mediaId":  mediaId,
	})
}
