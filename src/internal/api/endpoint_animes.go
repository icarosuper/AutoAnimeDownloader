package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type AnimeInfo struct {
	AnimeID          int    `json:"anime_id" example:"12345"`
	Name             string `json:"name" example:"Naruto"`
	EpisodesCount    int    `json:"episodes_count" example:"8"`
	TotalEpisodes    int    `json:"total_episodes" example:"12"`
	LatestEpisodeID  int    `json:"latest_episode_id" example:"12"`
	LastDownloadDate string `json:"last_download_date" example:"2026-02-24T10:30:00Z"`
}

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

		// Group episodes by anime
		animeMap := make(map[string]*AnimeInfo)

		for _, episode := range episodes {
			animeName := extractAnimeName(episode.EpisodeName)

			if animeInfo, exists := animeMap[animeName]; exists {
				animeInfo.EpisodesCount++
				if episode.EpisodeID > animeInfo.LatestEpisodeID {
					animeInfo.LatestEpisodeID = episode.EpisodeID
				}
				lastDownloadedTime, _ := time.Parse(time.RFC3339, animeInfo.LastDownloadDate)
				if episode.DownloadDate.After(lastDownloadedTime) {
					animeInfo.LastDownloadDate = episode.DownloadDate.Format(time.RFC3339)
				}
				if animeInfo.AnimeID == 0 && episode.AnimeID != 0 {
					animeInfo.AnimeID = episode.AnimeID
				}
				if animeInfo.TotalEpisodes == 0 && episode.AnimeTotalEpisodes != 0 {
					animeInfo.TotalEpisodes = episode.AnimeTotalEpisodes
				}
			} else {
				animeMap[animeName] = &AnimeInfo{
					AnimeID:          episode.AnimeID,
					Name:             animeName,
					EpisodesCount:    1,
					TotalEpisodes:    episode.AnimeTotalEpisodes,
					LatestEpisodeID:  episode.EpisodeID,
					LastDownloadDate: episode.DownloadDate.Format(time.RFC3339),
				}
			}
		}

		animes := make([]AnimeInfo, 0, len(animeMap))
		for _, animeInfo := range animeMap {
			animes = append(animes, *animeInfo)
		}

		JSONSuccess(w, http.StatusOK, animes)
	}
}
