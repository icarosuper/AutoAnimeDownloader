package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"net/http"
	"os"
	"strconv"
)

func getQBittorrentURLForAPI(configURL string) string {
	if envURL := os.Getenv("QBITTORRENT_URL"); envURL != "" {
		return envURL
	}
	return configURL
}

// @Summary      Manually download an episode
// @Description  Triggers an immediate download for an aired episode and marks it as manually managed
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeId path int true "Episode ID (AniList AiringNode ID)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeId}/download [post]
func handleDownloadEpisode(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		animeId, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || animeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		episodeId, err := strconv.Atoi(r.PathValue("episodeId"))
		if err != nil || episodeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "Invalid episode ID")
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		// Unblock the episode in case it was previously manually deleted
		if err := server.FileManager.UnblockEpisode(episodeId); err != nil {
			logger.Logger.Warn().Err(err).Int("episode_id", episodeId).Msg("Failed to unblock episode")
		}

		ep, err := daemon.ManualDownloadEpisode(animeId, episodeId, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode_id", episodeId).Msg("Failed to manually download episode")
			JSONError(w, http.StatusInternalServerError, "DOWNLOAD_FAILED", err.Error())
			return
		}

		if err := server.FileManager.SaveEpisodesToFile([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode_id", episodeId).Str("hash", ep.EpisodeHash).Msg("Manually downloaded episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode download started"})
	}
}

// @Summary      Manually delete a downloaded episode
// @Description  Deletes a downloaded episode from qBittorrent and blocks it from being re-downloaded automatically
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeId path int true "Episode ID (AniList AiringNode ID)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeId} [delete]
func handleDeleteEpisode(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is allowed")
			return
		}

		animeId, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || animeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		episodeId, err := strconv.Atoi(r.PathValue("episodeId"))
		if err != nil || episodeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "Invalid episode ID")
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		var targetHash string
		for _, ep := range savedEpisodes {
			if ep.EpisodeID == episodeId && ep.AnimeID == animeId {
				targetHash = ep.EpisodeHash
				break
			}
		}

		if targetHash == "" {
			JSONError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "Episode not found in downloaded list")
			return
		}

		qBittorrentURL := getQBittorrentURLForAPI(configs.QBittorrentUrl)
		torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)

		if err := torrentsService.DeleteTorrents([]string{targetHash}); err != nil {
			logger.Logger.Warn().Err(err).Str("hash", targetHash).Msg("Failed to delete torrent from qBittorrent")
		}

		if err := server.FileManager.DeleteEpisodesFromFile([]int{episodeId}); err != nil {
			logger.Logger.Error().Err(err).Int("episode_id", episodeId).Msg("Failed to remove episode from file")
			JSONInternalError(w, err)
			return
		}

		if err := server.FileManager.BlockEpisode(episodeId); err != nil {
			logger.Logger.Warn().Err(err).Int("episode_id", episodeId).Msg("Failed to block episode")
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode_id", episodeId).Msg("Manually deleted episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode deleted"})
	}
}
