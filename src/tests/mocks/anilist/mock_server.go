package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type AniListResponse struct {
	Data struct {
		Page struct {
			MediaList []MediaList `json:"mediaList"`
		} `json:"Page"`
	} `json:"data"`
}

type MediaList struct {
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	CustomLists map[string]bool `json:"customLists"`
	Media       Media  `json:"media"`
}

type Media struct {
	Status         string        `json:"status"`
	Title          Title         `json:"title"`
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleGraphQL)
	log.Printf("Mock Anilist API server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req GraphQLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Generate mock response based on scenario
	scenario := os.Getenv("SCENARIO")
	if scenario == "empty" {
		response := AniListResponse{
			Data: struct {
				Page struct {
					MediaList []MediaList `json:"mediaList"`
				} `json:"Page"`
			}{
				Page: struct {
					MediaList []MediaList `json:"mediaList"`
				}{
					MediaList: []MediaList{},
				},
			},
		}
		sendResponse(w, response)
		return
	}

	// Default: return mock data with one anime
	englishTitle := "Test Anime"
	romajiTitle := "Test Anime"
	response := AniListResponse{
		Data: struct {
			Page struct {
				MediaList []MediaList `json:"mediaList"`
			} `json:"Page"`
		}{
			Page: struct {
				MediaList []MediaList `json:"mediaList"`
			}{
				MediaList: []MediaList{
					{
						Status:   "CURRENT",
						Progress: 5,
						CustomLists: map[string]bool{},
						Media: Media{
							Status: "RELEASING",
							Title: Title{
								English: &englishTitle,
								Romaji:  &romajiTitle,
							},
							AiringSchedule: AiringSchedule{
								Nodes: []AiringNode{
									{
										ID:              1,
										Episode:         6,
										TimeUntilAiring: 86400,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	sendResponse(w, response)
}

func sendResponse(w http.ResponseWriter, response AniListResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
