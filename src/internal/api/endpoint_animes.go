package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"regexp"
	"strings"
)

// AnimeInfo representa informações agregadas sobre um anime
type AnimeInfo struct {
	Name            string `json:"name" example:"Naruto"`
	EpisodesCount   int    `json:"episodes_count" example:"12"`
	LatestEpisodeID int    `json:"latest_episode_id" example:"12"`
}

// extractAnimeName tenta extrair o nome do anime do nome do episódio
// Formato esperado: "Anime Name - Episode 01" ou "Anime Name 01" ou similar
func extractAnimeName(episodeName string) string {
	if episodeName == "" {
		return "Unknown"
	}
	
	// Remove padrões comuns de numeração de episódios
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\s*-\s*[Ee]pisode\s*\d+.*$`),
		regexp.MustCompile(`\s*-\s*[Ee]p\s*\d+.*$`),
		regexp.MustCompile(`\s*-\s*\d+.*$`),
		regexp.MustCompile(`\s+\d+.*$`),
		regexp.MustCompile(`\s*\(.*\)\s*$`),
	}
	
	result := episodeName
	for _, pattern := range patterns {
		result = pattern.ReplaceAllString(result, "")
	}
	
	result = strings.TrimSpace(result)
	if result == "" {
		return episodeName
	}
	
	return result
}

// handleAnimes retorna o handler para GET /api/v1/animes
// @Summary      Get animes list
// @Description  Returns a list of animes with aggregated episode information
// @Tags         animes
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]AnimeInfo}
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes [get]
func handleAnimes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		
		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}
		
		// Agrupar episódios por anime
		animeMap := make(map[string]*AnimeInfo)
		
		for _, episode := range episodes {
			animeName := extractAnimeName(episode.EpisodeName)
			
			if animeInfo, exists := animeMap[animeName]; exists {
				animeInfo.EpisodesCount++
				if episode.EpisodeID > animeInfo.LatestEpisodeID {
					animeInfo.LatestEpisodeID = episode.EpisodeID
				}
			} else {
				animeMap[animeName] = &AnimeInfo{
					Name:            animeName,
					EpisodesCount:   1,
					LatestEpisodeID: episode.EpisodeID,
				}
			}
		}
		
		// Converter map para slice
		animes := make([]AnimeInfo, 0, len(animeMap))
		for _, animeInfo := range animeMap {
			animes = append(animes, *animeInfo)
		}
		
		JSONSuccess(w, http.StatusOK, animes)
	}
}

