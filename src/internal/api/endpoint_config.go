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

		// A migracao de anilist_username -> anilist_usernames NAO e repetida aqui: quem migra e
		// LoadConfigs, uma vez, e gravando (decisions.md #90). Um corpo que ainda mande a chave
		// singular e persistido e curado na leitura seguinte.

		// anilist_usernames NAO e validado: uma instalacao que so usa animes avulsos nunca
		// configura conta nenhuma (decisions.md #49).

		if config.CompletedAnimePath == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Completed anime path is required")
			return
		}

		// A biblioteca e montada com hardlinks; nem todo filesystem suporta. Verifica no
		// momento do save, com a mesma funcao que o runtime usa.
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

		// max_episodes_per_anime aceita 0 = sem teto, alinhado com os outros tetos do projeto
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

		// 100 bloquearia todo download para sempre.
		if config.MinFreeDiskPercent < 0 || config.MinFreeDiskPercent > 99 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Min free disk percent must be between 0 and 99")
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

		// Aplica o limite na hora: sem isso, baixar o numero de downloads simultaneos so
		// valeria no proximo passe de verificacao (10 min por padrao), o que le como o
		// campo nao ter funcionado.
		if server.Torrents != nil {
			server.Torrents.SetMaxActiveDownloads(config.MaxConcurrentDownloads)
		}

		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Configuration updated successfully"})
	}
}
