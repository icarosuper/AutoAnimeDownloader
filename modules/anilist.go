package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AniListResponse struct {
	Data struct {
		Page struct {
			MediaList []MediaListEntry `json:"mediaList"`
		} `json:"Page"`
	} `json:"data"`
}

type MediaListEntry struct {
	Progress int   `json:"progress"`
	Media    Media `json:"media"`
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
	ID      int `json:"id"`
	Episode int `json:"episode"`
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func SearchAnimes(userName string) (*AniListResponse, error) {
	query := `
		query ExampleQuery($userName: String, $type: MediaType, $status: MediaListStatus, $notYetAired: Boolean) {
			Page {
				mediaList(userName: $userName, type: $type, status: $status) {
					media {
						title {
							english
							romaji
						}
						airingSchedule(notYetAired: $notYetAired) {
							nodes {
								id
								episode
							}
						}
					}
					progress
				}
			}
		}
	`

	variables := map[string]interface{}{
		"userName":    userName,
		"type":        "ANIME",
		"status":      "CURRENT",
		"notYetAired": false,
	}

	requestBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("POST", "https://graphql.anilist.co", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
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

func PrintAnimeResults(response *AniListResponse) {
	if response == nil || len(response.Data.Page.MediaList) == 0 {
		fmt.Println("No anime found.")
		return
	}

	fmt.Printf("Found %d anime:\n\n", len(response.Data.Page.MediaList))

	for i, entry := range response.Data.Page.MediaList {
		media := entry.Media
		title := ""
		if media.Title.English != nil {
			title = *media.Title.English
		} else if media.Title.Romaji != nil {
			title = *media.Title.Romaji
		}

		fmt.Printf("%d. %s\n", i+1, title)
		fmt.Printf("   Progress: %d episodes\n", entry.Progress)

		if len(media.AiringSchedule.Nodes) > 0 {
			fmt.Println("Aired episodes:")

			for _, node := range media.AiringSchedule.Nodes {
				fmt.Printf("   - Episode %d (ID: %d)\n", node.Episode, node.ID)
			}
		}

		fmt.Println()
	}
}
