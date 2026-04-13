package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
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

		animeSettings, err := server.FileManager.LoadAnimeSettings(animeId)
		if err != nil {
			logger.Logger.Warn().Err(err).Int("anime_id", animeId).Msg("Failed to load anime settings")
			animeSettings = &files.AnimeSettings{}
		}

		ep, err := daemon.ManualDownloadEpisode(animeId, episodeId, configs, animeSettings.CustomSearchQuery)
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

// @Summary      Release an episode from manual management
// @Description  Unblocks and unmanages an episode so the daemon can handle it automatically again
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeId path int true "Episode ID (AniList AiringNode ID)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeId}/release [post]
func handleReleaseEpisode(server *Server) http.HandlerFunc {
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

		if err := server.FileManager.UnblockEpisode(episodeId); err != nil {
			logger.Logger.Warn().Err(err).Int("episode_id", episodeId).Msg("Failed to unblock episode")
		}

		if err := server.FileManager.UnmanageEpisode(episodeId); err != nil {
			logger.Logger.Error().Err(err).Int("episode_id", episodeId).Msg("Failed to unmanage episode")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode_id", episodeId).Msg("Released episode from manual management")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode released"})
	}
}

// @Summary      Redownload an episode from Nyaa
// @Description  Deletes the existing torrent (if any) and searches Nyaa again for a fresh download
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
// @Router       /animes/{id}/episodes/{episodeId}/redownload [post]
func handleRedownloadEpisode(server *Server) http.HandlerFunc {
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

		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		var existingHash string
		for _, ep := range savedEpisodes {
			if ep.EpisodeID == episodeId && ep.AnimeID == animeId {
				existingHash = ep.EpisodeHash
				break
			}
		}

		qBittorrentURL := getQBittorrentURLForAPI(configs.QBittorrentUrl)
		torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)

		if existingHash != "" {
			if err := torrentsService.DeleteTorrents([]string{existingHash}); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", existingHash).Msg("Failed to delete existing torrent")
			}
			if err := server.FileManager.DeleteEpisodesFromFile([]int{episodeId}); err != nil {
				logger.Logger.Error().Err(err).Int("episode_id", episodeId).Msg("Failed to remove episode from file")
				JSONInternalError(w, err)
				return
			}
		}

		if err := server.FileManager.UnblockEpisode(episodeId); err != nil {
			logger.Logger.Warn().Err(err).Int("episode_id", episodeId).Msg("Failed to unblock episode")
		}

		animeSettings, err := server.FileManager.LoadAnimeSettings(animeId)
		if err != nil {
			logger.Logger.Warn().Err(err).Int("anime_id", animeId).Msg("Failed to load anime settings")
			animeSettings = &files.AnimeSettings{}
		}

		ep, err := daemon.ManualDownloadEpisode(animeId, episodeId, configs, animeSettings.CustomSearchQuery)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode_id", episodeId).Msg("Failed to redownload episode")
			JSONError(w, http.StatusInternalServerError, "REDOWNLOAD_FAILED", err.Error())
			return
		}

		if err := server.FileManager.SaveEpisodesToFile([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode_id", episodeId).Str("hash", ep.EpisodeHash).Msg("Redownloaded episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode redownload started"})
	}
}

// @Summary      Replace a downloaded episode with a user-supplied magnet link
// @Description  Deletes the existing torrent (if any) and downloads the episode using the provided magnet link
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeId path int true "Episode ID (AniList AiringNode ID)"
// @Param        body      body object true "Magnet link" example({"magnet":"magnet:?xt=..."})
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeId}/replace [post]
func handleReplaceEpisodeWithMagnet(server *Server) http.HandlerFunc {
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

		var body struct {
			Magnet string `json:"magnet"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !strings.HasPrefix(body.Magnet, "magnet:") {
			JSONError(w, http.StatusBadRequest, "INVALID_MAGNET", "Invalid or missing magnet link")
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		// Delete existing torrent if already downloaded
		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		var existingHash string
		for _, ep := range savedEpisodes {
			if ep.EpisodeID == episodeId && ep.AnimeID == animeId {
				existingHash = ep.EpisodeHash
				break
			}
		}

		qBittorrentURL := getQBittorrentURLForAPI(configs.QBittorrentUrl)
		torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)

		if existingHash != "" {
			if err := torrentsService.DeleteTorrents([]string{existingHash}); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", existingHash).Msg("Failed to delete existing torrent")
			}
			if err := server.FileManager.DeleteEpisodesFromFile([]int{episodeId}); err != nil {
				logger.Logger.Error().Err(err).Int("episode_id", episodeId).Msg("Failed to remove episode from file")
				JSONInternalError(w, err)
				return
			}
		}

		if err := server.FileManager.UnblockEpisode(episodeId); err != nil {
			logger.Logger.Warn().Err(err).Int("episode_id", episodeId).Msg("Failed to unblock episode")
		}

		ep, err := daemon.ManualDownloadEpisodeWithMagnet(animeId, episodeId, body.Magnet, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode_id", episodeId).Msg("Failed to replace episode with magnet")
			JSONError(w, http.StatusInternalServerError, "REPLACE_FAILED", err.Error())
			return
		}

		if err := server.FileManager.SaveEpisodesToFile([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode_id", episodeId).Str("hash", ep.EpisodeHash).Msg("Replaced episode with user magnet")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode replacement started"})
	}
}

// @Summary      Replace all downloaded episodes of an anime with a user-supplied batch magnet link
// @Description  Deletes all existing torrents for the anime and downloads using the provided magnet link, marking all aired episodes as downloaded
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id   path int true "Anime ID (AniList MediaList ID)"
// @Param        body body object true "Magnet link" example({"magnet":"magnet:?xt=..."})
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/replace [post]
func handleReplaceAnimeWithMagnet(server *Server) http.HandlerFunc {
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

		var body struct {
			Magnet string `json:"magnet"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !strings.HasPrefix(body.Magnet, "magnet:") {
			JSONError(w, http.StatusBadRequest, "INVALID_MAGNET", "Invalid or missing magnet link")
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

		// Collect hashes and IDs for all existing episodes of this anime
		var hashesToDelete []string
		var idsToDelete []int
		seenHashes := make(map[string]bool)
		for _, ep := range savedEpisodes {
			if ep.AnimeID == animeId {
				idsToDelete = append(idsToDelete, ep.EpisodeID)
				if !seenHashes[ep.EpisodeHash] {
					seenHashes[ep.EpisodeHash] = true
					hashesToDelete = append(hashesToDelete, ep.EpisodeHash)
				}
			}
		}

		qBittorrentURL := getQBittorrentURLForAPI(configs.QBittorrentUrl)
		torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)

		if len(hashesToDelete) > 0 {
			if err := torrentsService.DeleteTorrents(hashesToDelete); err != nil {
				logger.Logger.Warn().Err(err).Msg("Failed to delete existing anime torrents")
			}
		}
		if len(idsToDelete) > 0 {
			if err := server.FileManager.DeleteEpisodesFromFile(idsToDelete); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Msg("Failed to remove anime episodes from file")
				JSONInternalError(w, err)
				return
			}
		}

		episodes, err := daemon.ManualDownloadAnimeWithMagnet(animeId, body.Magnet, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Msg("Failed to replace anime with magnet")
			JSONError(w, http.StatusInternalServerError, "REPLACE_FAILED", err.Error())
			return
		}

		if err := server.FileManager.SaveEpisodesToFile(episodes); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episodes to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episodes", len(episodes)).Msg("Replaced anime with user magnet")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Anime replacement started"})
	}
}
