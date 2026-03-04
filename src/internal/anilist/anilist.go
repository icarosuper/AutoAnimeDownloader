package anilist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var httpDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

var aniListAPIURL = "https://graphql.anilist.co"

func init() {
	if url := os.Getenv("ANILIST_API_URL"); url != "" {
		aniListAPIURL = url
	}
}

func MockAniListDo(fn func(*http.Request) (*http.Response, error)) (restore func()) {
	prev := httpDo
	if fn == nil {
		return func() { httpDo = prev }
	}
	httpDo = fn
	return func() { httpDo = prev }
}

func doGraphQLRequest[T any](query string, variables map[string]any) (*T, error) {
	jsonData, err := json.Marshal(GraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", aniListAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var response T
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
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

type Media struct {
	Status         MediaStatus    `json:"status"`
	Format         MediaFormat    `json:"format"`
	Title          Title          `json:"title"`
	Episodes       *int           `json:"episodes"`
	AiringSchedule AiringSchedule `json:"airingSchedule"`
}

type Title struct {
	English *string `json:"english"`
	Romaji  *string `json:"romaji"`
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

type MediaListDetailResponse struct {
	Data struct {
		MediaList MediaListDetail `json:"MediaList"`
	} `json:"data"`
}

type MediaListDetail struct {
	Id       int             `json:"id"`
	Status   MediaListStatus `json:"status"`
	Progress int             `json:"progress"`
	Media    struct {
		Episodes       int            `json:"episodes"`
		AiringSchedule AiringSchedule `json:"airingSchedule"`
	} `json:"media"`
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

func SearchAnimes(userName string) (*AniListResponse, error) {
	query := `
		query GetAllCurrentAnime($userName: String, $type: MediaType, $statuses: [MediaListStatus]) {
			Page {
				mediaList(userName: $userName, type: $type, status_in: $statuses) {
					id
					status
					progress
					customLists
					media {
						format
						status
						episodes
						title {
							english
							romaji
						}
						airingSchedule {
							nodes {
								id
								episode
								timeUntilAiring
							}
						}
					}
				}
			}
		}
	`

	variables := map[string]any{
		"userName": userName,
		"type":     "ANIME",
		"statuses": []string{
			string(MediaListStatusCurrent),
			string(MediaListStatusRepeating),
		},
	}

	return doGraphQLRequest[AniListResponse](query, variables)
}

func GetMediaListDetail(mediaListId int) (*MediaListDetailResponse, error) {
	query := `
		query GetAnimeEpisodes($mediaListId: Int) {
			MediaList(id: $mediaListId) {
				id
				status
				progress
				media {
					episodes
					airingSchedule {
						nodes {
							airingAt
							timeUntilAiring
							episode
							id
						}
					}
				}
			}
		}
	`

	variables := map[string]any{
		"mediaListId": mediaListId,
	}

	return doGraphQLRequest[MediaListDetailResponse](query, variables)
}
