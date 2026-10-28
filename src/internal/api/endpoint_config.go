package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
)

func handleConfig(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetConfig(server)(w, r)
		case http.MethodPut:
			handleUpdateConfig(server)(w, r)
		default:
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and PUT methods are allowed")
		}
	}
}

func handleGetConfig(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, configs)
	}
}

func handleUpdateConfig(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config files.Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to decode config JSON")
			JSONError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON format")
			return
		}

		// The anilist_username -> anilist_usernames migration is NOT repeated here: LoadConfigs
		// migrates once, on write (decisions.md #90). A body that still sends the singular
		// key is persisted and cured on the next read.

		// anilist_usernames is NOT validated: an install that only uses standalone animes
		// never configures an account (decisions.md #49).

		if config.CompletedAnimePath == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Completed anime path is required")
			return
		}

		// The library is built with hardlinks; not every filesystem supports them. Probe at
		// save time, with the same function the runtime uses.
		if server.Librarian != nil {
			if err := server.Librarian.ProbePath(config.CompletedAnimePath); err != nil {
				JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
		}

		if config.CheckInterval <= 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Check interval must be greater than 0")
			return
		}

		// max_episodes_per_anime accepts 0 = no cap, aligned with the project's other caps
		// (max_batch_torrent_size_gb, min_seeders, watched_episodes_to_keep).
		if config.MaxEpisodesPerAnime < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max episodes per anime must be non-negative")
			return
		}

		if config.EpisodeRetryLimit < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Episode retry limit must be non-negative")
			return
		}

		if config.WatchedEpisodesToKeep < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Watched episodes to keep must be non-negative")
			return
		}

		if config.MaxConcurrentDownloads < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max concurrent downloads must be non-negative")
			return
		}

		if config.MaxBatchTorrentSizeGB < 0 || config.MaxEpisodeTorrentSizeGB < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Torrent size limits must be non-negative")
			return
		}

		if config.MinSeeders < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Min seeders must be non-negative")
			return
		}

		if config.MaxSearchPages < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max search pages must be non-negative")
			return
		}

		if config.MinFreeDiskGB < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Min free disk space must be non-negative")
			return
		}

		if config.Notifications.BatchWindowSeconds < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Notification batch window must be non-negative")
			return
		}

		if err := server.FileManager.SaveConfigs(&config); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save configs")
			JSONInternalError(w, err)
			return
		}

		// Apply the limit immediately: without this, lowering concurrent downloads would
		// only take effect on the next check pass (10 min by default), which reads as
		// the field not working.
		if server.Torrents != nil {
			server.Torrents.SetMaxActiveDownloads(config.MaxConcurrentDownloads)
		}

		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Configuration updated successfully"})
	}
}
