package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// @Summary      Manually download an episode
// @Description  Triggers an immediate download for an aired episode and marks it as manually managed
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeNumber path int true "Episode number (1-based, as aired)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeNumber}/download [post]
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

		episodeNumber, err := strconv.Atoi(r.PathValue("episodeNumber"))
		if err != nil || episodeNumber <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Invalid episode number")
			return
		}
		key := files.EpisodeKey{AnimeID: animeId, Episode: episodeNumber}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		// Unblock the episode in case it was previously manually deleted
		if err := server.FileManager.UnblockEpisode(key); err != nil {
			logger.Logger.Warn().Err(err).Int("episode", episodeNumber).Msg("Failed to unblock episode")
		}

		ep, err := daemon.ManualDownloadEpisode(server.FileManager, server.Torrents, animeId, episodeNumber, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to manually download episode")
			JSONDownloadError(w, err, "DOWNLOAD_FAILED")
			return
		}

		// UpsertEpisodes, never SaveEpisodesToFile: the latter is append-only with dedupe by
		// EpisodeKey, so an already-saved record makes it discard this one entirely — the new
		// hash would be lost (breaking JobOrganize's saved-episode ↔ torrent join by hash) and
		// the stale LibraryPaths would survive, making organizeTorrent think the episode was
		// already organized. Wholesale replacement is correct here because
		// ManualDownloadEpisode always returns ManuallyManaged: true and LibraryPaths nil, so
		// the record already carries what the daemon's mergeSavedEpisode would compute
		// (decision 27).
		if err := server.FileManager.UpsertEpisodes([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode", episodeNumber).Str("hash", ep.EpisodeHash).Msg("Manually downloaded episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode download started"})
	}
}

// @Summary      Manually delete a downloaded episode
// @Description  Deletes a downloaded episode (library hardlink + torrent) and blocks it from being re-downloaded automatically
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeNumber path int true "Episode number (1-based, as aired)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeNumber} [delete]
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

		episodeNumber, err := strconv.Atoi(r.PathValue("episodeNumber"))
		if err != nil || episodeNumber <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Invalid episode number")
			return
		}
		key := files.EpisodeKey{AnimeID: animeId, Episode: episodeNumber}

		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		found := false
		for _, ep := range savedEpisodes {
			if ep.Key() == key {
				found = true
				break
			}
		}

		if !found {
			JSONError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "Episode not found in downloaded list")
			return
		}

		if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, []files.EpisodeKey{key}); err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to remove episode")
			JSONInternalError(w, err)
			return
		}

		if err := server.FileManager.BlockEpisode(key); err != nil {
			logger.Logger.Warn().Err(err).Int("episode", episodeNumber).Msg("Failed to block episode")
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Manually deleted episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode deleted"})
	}
}

// @Summary      Release an episode from manual management
// @Description  Unblocks and unmanages an episode so the daemon can handle it automatically again
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeNumber path int true "Episode number (1-based, as aired)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeNumber}/release [post]
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

		episodeNumber, err := strconv.Atoi(r.PathValue("episodeNumber"))
		if err != nil || episodeNumber <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Invalid episode number")
			return
		}
		key := files.EpisodeKey{AnimeID: animeId, Episode: episodeNumber}

		if err := server.FileManager.UnblockEpisode(key); err != nil {
			logger.Logger.Warn().Err(err).Int("episode", episodeNumber).Msg("Failed to unblock episode")
		}

		if err := server.FileManager.UnmanageEpisode(key); err != nil {
			logger.Logger.Error().Err(err).Int("episode", episodeNumber).Msg("Failed to unmanage episode")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Released episode from manual management")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode released"})
	}
}

// @Summary      Redownload an episode from Nyaa
// @Description  Deletes the existing torrent (if any) and searches Nyaa again for a fresh download
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeNumber path int true "Episode number (1-based, as aired)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeNumber}/redownload [post]
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

		episodeNumber, err := strconv.Atoi(r.PathValue("episodeNumber"))
		if err != nil || episodeNumber <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Invalid episode number")
			return
		}
		key := files.EpisodeKey{AnimeID: animeId, Episode: episodeNumber}

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

		alreadyDownloaded := false
		for _, ep := range savedEpisodes {
			if ep.Key() == key {
				alreadyDownloaded = true
				break
			}
		}

		if alreadyDownloaded {
			// Abort on failure: adding a new torrent while the stale record survives would
			// leave the new download untracked (JobOrganize joins saved episodes by hash).
			if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, []files.EpisodeKey{key}); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to remove episode before redownload")
				JSONInternalError(w, err)
				return
			}
		}

		if err := server.FileManager.UnblockEpisode(key); err != nil {
			logger.Logger.Warn().Err(err).Int("episode", episodeNumber).Msg("Failed to unblock episode")
		}

		ep, err := daemon.ManualDownloadEpisode(server.FileManager, server.Torrents, animeId, episodeNumber, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to redownload episode")
			JSONDownloadError(w, err, "REDOWNLOAD_FAILED")
			return
		}

		// UpsertEpisodes so the write does not depend on RemoveEpisodesWithLinks having deleted
		// the old record first: append-only SaveEpisodesToFile would silently drop this update if
		// any record survived. Replacement is safe — ManualDownloadEpisode sets
		// ManuallyManaged: true and leaves LibraryPaths nil (decision 27).
		if err := server.FileManager.UpsertEpisodes([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode", episodeNumber).Str("hash", ep.EpisodeHash).Msg("Redownloaded episode")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Episode redownload started"})
	}
}

// @Summary      Replace a downloaded episode with a user-supplied magnet link
// @Description  Deletes the existing torrent (if any) and downloads the episode using the provided magnet link
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id        path int true "Anime ID (AniList MediaList ID)"
// @Param        episodeNumber path int true "Episode number (1-based, as aired)"
// @Param        body      body object true "Magnet link" example({"magnet":"magnet:?xt=..."})
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes/{episodeNumber}/replace [post]
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

		episodeNumber, err := strconv.Atoi(r.PathValue("episodeNumber"))
		if err != nil || episodeNumber <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Invalid episode number")
			return
		}
		key := files.EpisodeKey{AnimeID: animeId, Episode: episodeNumber}

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

		alreadyDownloaded := false
		for _, ep := range savedEpisodes {
			if ep.Key() == key {
				alreadyDownloaded = true
				break
			}
		}

		if alreadyDownloaded {
			// Abort on failure: the replacement torrent would otherwise be untracked.
			if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, []files.EpisodeKey{key}); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to remove episode before replacement")
				JSONInternalError(w, err)
				return
			}
		}

		if err := server.FileManager.UnblockEpisode(key); err != nil {
			logger.Logger.Warn().Err(err).Int("episode", episodeNumber).Msg("Failed to unblock episode")
		}

		ep, err := daemon.ManualDownloadEpisodeWithMagnet(server.FileManager, server.Torrents, animeId, episodeNumber, body.Magnet, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", episodeNumber).Msg("Failed to replace episode with magnet")
			JSONDownloadError(w, err, "REPLACE_FAILED")
			return
		}

		// UpsertEpisodes: same reasoning as the redownload handler — the update must not depend
		// on the old record having been deleted. ManualDownloadEpisodeWithMagnet sets
		// ManuallyManaged: true and leaves LibraryPaths nil (decision 27).
		if err := server.FileManager.UpsertEpisodes([]files.EpisodeStruct{ep}); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episode", episodeNumber).Str("hash", ep.EpisodeHash).Msg("Replaced episode with user magnet")
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

		var keysToDelete []files.EpisodeKey
		for _, ep := range savedEpisodes {
			if ep.AnimeID == animeId {
				keysToDelete = append(keysToDelete, ep.Key())
			}
		}

		if len(keysToDelete) > 0 {
			// Abort on failure: the replacement batch torrent would otherwise be untracked.
			if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, keysToDelete); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Msg("Failed to remove episodes before replacement")
				JSONInternalError(w, err)
				return
			}
		}

		episodes, err := daemon.ManualDownloadAnimeWithMagnet(server.FileManager, server.Torrents, animeId, body.Magnet, configs)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Msg("Failed to replace anime with magnet")
			JSONDownloadError(w, err, "REPLACE_FAILED")
			return
		}

		// UpsertEpisodes: the batch covers every aired episode of the anime, so a single record
		// that survived the removal above would otherwise be left pointing at the old torrent
		// hash. ManualDownloadAnimeWithMagnet sets ManuallyManaged: true on every record and
		// leaves LibraryPaths nil (decision 27).
		if err := server.FileManager.UpsertEpisodes(episodes); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save episodes to file")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episodes", len(episodes)).Msg("Replaced anime with user magnet")
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Anime replacement started"})
	}
}
