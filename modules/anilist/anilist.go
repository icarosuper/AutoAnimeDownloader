package anilist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

var aniListAPIURL = "https://graphql.anilist.co"

func MockAniListDo(fn func(*http.Request) (*http.Response, error)) (restore func()) {
	prev := httpDo
	if fn == nil {
		return func() { httpDo = prev }
	}
	httpDo = fn
	return func() { httpDo = prev }
}

func MockAniListAPIURL(url string) (restore func()) {
	prev := aniListAPIURL
	if url == "" {
		return func() { aniListAPIURL = prev }
	}
	aniListAPIURL = url
	return func() { aniListAPIURL = prev }
}

type AniListResponse struct {
	Data struct {
		Page struct {
			MediaList []MediaListEntry `json:"mediaList"`
		} `json:"Page"`
	} `json:"data"`
}

type MediaListEntry struct {
	Progress    int         `json:"progress"`
	CustomLists CustomLists `json:"customLists"`
	Media       Media       `json:"media"`
}

type Media struct {
	Title          Title          `json:"title"`
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
	ID              int `json:"id"`
	Episode         int `json:"episode"`
	TimeUntilAiring int `json:"timeUntilAiring"`
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type CustomLists map[string]bool

func SearchAnimes(userName string) (*AniListResponse, error) {
	query := `
		query ExampleQuery($userName: String, $type: MediaType, $status: MediaListStatus) {
			Page {
				mediaList(userName: $userName, type: $type, status: $status) {
					progress
					customLists
					media {
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

	variables := map[string]interface{}{
		"userName": userName,
		"type":     "ANIME",
		"status":   "CURRENT",
	}

	requestBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(requestBody)
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

	var response AniListResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}
