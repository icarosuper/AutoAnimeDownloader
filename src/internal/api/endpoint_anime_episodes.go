package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
	"time"
)

type AnimeEpisodeInfo struct {
	EpisodeID         int    `json:"episode_id"`
	EpisodeNumber     int    `json:"episode_number"`
	AiringAt          int64  `json:"airing_at"`
	TimeUntilAiring   int    `json:"time_until_airing"`
	IsAired           bool   `json:"is_aired"`
	IsWatched         bool   `json:"is_watched"`
	IsDownloaded      bool   `json:"is_downloaded"`
	DownloadDate      string `json:"download_date,omitempty"`
	EpisodeName       string `json:"episode_name,omitempty"`
	IsManuallyManaged bool   `json:"is_manually_managed,omitempty"`
	IsBlocked         bool   `json:"is_blocked,omitempty"`
	// EpisodeHash is the saved episode's torrent info hash. It is what lets the anime detail
	// screen join in live torrent progress exactly: a batch torrent's saved episodes all carry
	// the same hash but episode_number is meaningless for matching a torrent to them (the
	// torrent itself has no single episode number), so joining by number would miss batches
	// entirely. Joining by hash instead, single and batch episodes use the same path. Empty
	// (and omitted via omitempty) when the episode has no saved record.
	EpisodeHash string `json:"episode_hash,omitempty"`
}

type AnimeDetailResponse struct {
	// AnimeID e o id da MIDIA na AniList — a mesma chave usada em episodes.json e nas rotas
	// /animes/{id}/*, e tambem o id do link anilist.co/anime/{id} (ver decisions.md #43).
	AnimeID           int                `json:"anime_id"`
	TotalEpisodes     int                `json:"total_episodes"`
	Progress          int                `json:"progress"`
	Status            string             `json:"status"`
	CoverImage        string             `json:"cover_image,omitempty"`
	Episodes          []AnimeEpisodeInfo `json:"episodes"`
	CustomSearchQuery string             `json:"custom_search_query,omitempty"`
}

// @Summary      Get detail and episodes for a specific anime
// @Description  Returns anime info from AniList combined with downloaded episodes data
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id path int true "Anime ID (AniList media ID)"
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

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		mediaList, err := anilist.GetAnimeInfo(id, config.AnilistUsernames)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to fetch anime detail from AniList")
			JSONInternalError(w, err)
			return
		}
		if mediaList == nil {
			JSONError(w, http.StatusNotFound, "ANIME_NOT_FOUND", "Anime is not in any configured AniList account")
			return
		}

		allEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		blockedIDs, err := server.FileManager.LoadBlockedEpisodes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to load blocked episodes")
			blockedIDs = []int{}
		}
		blockedSet := make(map[int]bool, len(blockedIDs))
		for _, bid := range blockedIDs {
			blockedSet[bid] = true
		}

		// Map downloaded episodes by their AiringNode ID
		type downloadedInfo struct {
			date            string
			name            string
			manuallyManaged bool
			hash            string
		}
		downloadedByNodeID := make(map[int]downloadedInfo)
		for _, ep := range allEpisodes {
			if ep.AnimeID == id {
				downloadedByNodeID[ep.EpisodeID] = downloadedInfo{
					date:            ep.DownloadDate.Format(time.RFC3339),
					name:            ep.EpisodeName,
					manuallyManaged: ep.ManuallyManaged,
					hash:            ep.EpisodeHash,
				}
			}
		}

		episodes := make([]AnimeEpisodeInfo, 0, len(mediaList.Media.AiringSchedule.Nodes))

		for _, node := range mediaList.Media.AiringSchedule.Nodes {
			info := AnimeEpisodeInfo{
				EpisodeID:       node.ID,
				EpisodeNumber:   node.Episode,
				AiringAt:        node.AiringAt,
				TimeUntilAiring: node.TimeUntilAiring,
				IsAired:         node.TimeUntilAiring <= 0,
				IsWatched:       node.Episode <= mediaList.Progress,
				IsBlocked:       blockedSet[node.ID],
			}

			if downloaded, ok := downloadedByNodeID[node.ID]; ok {
				info.IsDownloaded = true
				info.DownloadDate = downloaded.date
				info.EpisodeName = downloaded.name
				info.IsManuallyManaged = downloaded.manuallyManaged
				info.EpisodeHash = downloaded.hash
			}

			episodes = append(episodes, info)
		}

		animeSettings, err := server.FileManager.LoadAnimeSettings(id)
		if err != nil {
			logger.Logger.Warn().Err(err).Int("anime_id", id).Msg("Failed to load anime settings")
			animeSettings = &files.AnimeSettings{}
		}

		totalEpisodes := 0
		if mediaList.Media.Episodes != nil {
			totalEpisodes = *mediaList.Media.Episodes
		}

		coverImage := mediaList.Media.CoverImage.Large
		if coverImage == "" {
			coverImage = mediaList.Media.CoverImage.Medium
		}

		response := AnimeDetailResponse{
			AnimeID:           id,
			TotalEpisodes:     totalEpisodes,
			Progress:          mediaList.Progress,
			Status:            string(mediaList.Status),
			CoverImage:        coverImage,
			Episodes:          episodes,
			CustomSearchQuery: animeSettings.CustomSearchQuery,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
