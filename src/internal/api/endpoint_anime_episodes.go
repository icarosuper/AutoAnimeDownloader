package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
	"time"
)

type AnimeEpisodeInfo struct {
	EpisodeNumber   int    `json:"episode_number"`
	AiringAt        int64  `json:"airing_at"`
	TimeUntilAiring int    `json:"time_until_airing"`
	IsAired         bool   `json:"is_aired"`
	IsWatched       bool   `json:"is_watched"`
	IsDownloaded    bool   `json:"is_downloaded"`
	DownloadDate    string `json:"download_date,omitempty"`
	EpisodeName     string `json:"episode_name,omitempty"`
}

type AnimeDetailResponse struct {
	AnimeID       int                `json:"anime_id"`
	TotalEpisodes int                `json:"total_episodes"`
	Progress      int                `json:"progress"`
	Status        string             `json:"status"`
	Episodes      []AnimeEpisodeInfo `json:"episodes"`
}

// @Summary      Get detail and episodes for a specific anime
// @Description  Returns anime info from AniList combined with downloaded episodes data
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id path int true "Anime ID (AniList MediaList ID)"
// @Success      200  {object}  SuccessResponse{data=AnimeDetailResponse}
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes [get]
func handleAnimeEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		detail, err := anilist.GetMediaListDetail(id)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to fetch anime detail from AniList")
			JSONInternalError(w, err)
			return
		}

		allEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		// Map downloaded episodes by their AiringNode ID
		downloadedByNodeID := make(map[int]struct {
			date string
			name string
		})
		for _, ep := range allEpisodes {
			if ep.AnimeID == id {
				downloadedByNodeID[ep.EpisodeID] = struct {
					date string
					name string
				}{
					date: ep.DownloadDate.Format(time.RFC3339),
					name: ep.EpisodeName,
				}
			}
		}

		mediaList := detail.Data.MediaList
		episodes := make([]AnimeEpisodeInfo, 0, len(mediaList.Media.AiringSchedule.Nodes))

		for _, node := range mediaList.Media.AiringSchedule.Nodes {
			info := AnimeEpisodeInfo{
				EpisodeNumber:   node.Episode,
				AiringAt:        node.AiringAt,
				TimeUntilAiring: node.TimeUntilAiring,
				IsAired:         node.TimeUntilAiring <= 0,
				IsWatched:       node.Episode <= mediaList.Progress,
			}

			if downloaded, ok := downloadedByNodeID[node.ID]; ok {
				info.IsDownloaded = true
				info.DownloadDate = downloaded.date
				info.EpisodeName = downloaded.name
			}

			episodes = append(episodes, info)
		}

		response := AnimeDetailResponse{
			AnimeID:       id,
			TotalEpisodes: mediaList.Media.Episodes,
			Progress:      mediaList.Progress,
			Status:        string(mediaList.Status),
			Episodes:      episodes,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
