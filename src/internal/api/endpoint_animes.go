package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// reStripEpisodeNumber strips common episode-numbering suffixes from an episode name.
var reStripEpisodeNumber = []*regexp.Regexp{
	regexp.MustCompile(`\s*-\s*[Ee]pisode\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*[Ee]p\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*\d+.*$`),
	regexp.MustCompile(`\s+\d+.*$`),
	regexp.MustCompile(`\s*\(.*\)\s*$`),
}

type AnimeInfo struct {
	AnimeID            int    `json:"anime_id" example:"12345"`
	Name               string `json:"name" example:"Naruto"`
	EpisodesDownloaded int    `json:"episodes_downloaded" example:"8"`
	EpisodesReleased   int    `json:"episodes_released" example:"10"`
	EpisodesWatched    int    `json:"episodes_watched" example:"5"`
	TotalEpisodes      int    `json:"total_episodes" example:"12"`
	LatestEpisodeID    int    `json:"latest_episode_id" example:"12"`
	LastDownloadDate   string `json:"last_download_date" example:"2026-02-24T10:30:00Z"`
	CoverImage         string `json:"cover_image,omitempty"`
	IsBlacklisted      bool   `json:"is_blacklisted,omitempty"`
}

func extractAnimeName(episodeName string) string {
	if episodeName == "" {
		return "Unknown"
	}

	result := episodeName
	for _, pattern := range reStripEpisodeNumber {
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

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		// Group episodes by anime.
		// Key: "id:<AnimeID>" when AnimeID is set, otherwise the extracted name (backward compat).
		animeMap := make(map[string]*AnimeInfo)

		for _, episode := range episodes {
			var key string
			if episode.AnimeID != 0 {
				key = fmt.Sprintf("id:%d", episode.AnimeID)
			} else {
				key = extractAnimeName(episode.EpisodeName)
			}

			// Prefer the persisted AnimeName; fall back to extracting from EpisodeName.
			displayName := episode.AnimeName
			if displayName == "" {
				displayName = extractAnimeName(episode.EpisodeName)
			}

			if animeInfo, exists := animeMap[key]; exists {
				animeInfo.EpisodesDownloaded++
				if episode.EpisodeID > animeInfo.LatestEpisodeID {
					animeInfo.LatestEpisodeID = episode.EpisodeID
				}
				lastDownloadedTime, _ := time.Parse(time.RFC3339, animeInfo.LastDownloadDate)
				if episode.DownloadDate.After(lastDownloadedTime) {
					animeInfo.LastDownloadDate = episode.DownloadDate.Format(time.RFC3339)
					// Update name from the most recent episode that has AnimeName set.
					if episode.AnimeName != "" {
						animeInfo.Name = episode.AnimeName
					}
				}
				if animeInfo.AnimeID == 0 && episode.AnimeID != 0 {
					animeInfo.AnimeID = episode.AnimeID
				}
				if animeInfo.TotalEpisodes == 0 && episode.AnimeTotalEpisodes != 0 {
					animeInfo.TotalEpisodes = episode.AnimeTotalEpisodes
				}
			} else {
				animeMap[key] = &AnimeInfo{
					AnimeID:            episode.AnimeID,
					Name:               displayName,
					EpisodesDownloaded: 1,
					TotalEpisodes:      episode.AnimeTotalEpisodes,
					LatestEpisodeID:    episode.EpisodeID,
					LastDownloadDate:   episode.DownloadDate.Format(time.RFC3339),
				}
			}
		}

		// Merge CURRENT animes from AniList so they remain visible even with 0 downloaded episodes
		for _, username := range config.AnilistUsernames {
			mergeCurrentAniListAnimes(animeMap, username, config.ExcludedLists, config.DownloadStatuses)
		}

		animes := make([]AnimeInfo, 0, len(animeMap))
		for _, animeInfo := range animeMap {
			animes = append(animes, *animeInfo)
		}

		JSONSuccess(w, http.StatusOK, animes)
	}
}

func mergeCurrentAniListAnimes(animeMap map[string]*AnimeInfo, username string, excludedLists []string, statuses []string) {
	// Fetch customLists via cached minimal query before the complex query that may null it out.
	clMap := anilist.GetCustomListsMap(username, statuses)

	resp, err := anilist.GetFrontendAnimeList(username, statuses)
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to fetch AniList current animes, skipping merge")
		return
	}

	for i := range resp.Data.Page.MediaList {
		ml := &resp.Data.Page.MediaList[i]
		if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 {
			ml.CustomLists = cl
		}
	}

	// Build set of valid AnimeIDs from AniList response (only statuses in DownloadStatuses)
	validIDs := make(map[int]bool, len(resp.Data.Page.MediaList))
	for _, ml := range resp.Data.Page.MediaList {
		validIDs[ml.Id] = true
	}

	// Remove animes whose AnimeID is known but not in the valid set
	for key, info := range animeMap {
		if info.AnimeID != 0 && !validIDs[info.AnimeID] {
			delete(animeMap, key)
		}
	}

	// Build map from AnimeID → *AnimeInfo pointer so we can update existing entries
	knownByID := make(map[int]*AnimeInfo)
	for _, info := range animeMap {
		if info.AnimeID != 0 {
			knownByID[info.AnimeID] = info
		}
	}

	for _, ml := range resp.Data.Page.MediaList {
		name := ""
		if ml.Media.Title.English != nil && *ml.Media.Title.English != "" {
			name = *ml.Media.Title.English
		} else if ml.Media.Title.Romaji != nil {
			name = *ml.Media.Title.Romaji
		}
		if name == "" {
			continue
		}
		totalEpisodes := 0
		if ml.Media.Episodes != nil {
			totalEpisodes = *ml.Media.Episodes
		}

		isBlacklisted := false
		if len(excludedLists) > 0 {
			excludedSet := make(map[string]bool, len(excludedLists))
			for _, name := range excludedLists {
				excludedSet[name] = true
			}
			for listName, inList := range ml.CustomLists {
				if excludedSet[listName] && inList {
					isBlacklisted = true
					break
				}
			}
		}

		coverImage := ml.Media.CoverImage.Large
		if coverImage == "" {
			coverImage = ml.Media.CoverImage.Medium
		}

		episodesReleased := 0
		for _, node := range ml.Media.AiringSchedule.Nodes {
			if node.TimeUntilAiring <= 0 && node.Episode > episodesReleased {
				episodesReleased = node.Episode
			}
		}
		if episodesReleased == 0 && ml.Media.Status == anilist.MediaStatusFinished {
			episodesReleased = totalEpisodes
		}

		if existing, ok := knownByID[ml.Id]; ok {
			existing.Name = name
			if existing.TotalEpisodes == 0 {
				existing.TotalEpisodes = totalEpisodes
			}
			existing.EpisodesReleased = episodesReleased
			existing.EpisodesWatched = ml.Progress
			existing.CoverImage = coverImage
			existing.IsBlacklisted = isBlacklisted
			continue
		}

		animeMap[name] = &AnimeInfo{
			AnimeID:          ml.Id,
			Name:             name,
			EpisodesReleased: episodesReleased,
			EpisodesWatched:  ml.Progress,
			TotalEpisodes:    totalEpisodes,
			CoverImage:       coverImage,
			IsBlacklisted:    isBlacklisted,
		}
	}
}
