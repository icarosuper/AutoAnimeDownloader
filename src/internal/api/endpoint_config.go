package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
)

// @Summary      Get and update configuration
// @Description  Returns the current daemon configuration and allows updates
// @Tags         config
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /config [get]
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

// @Summary      Get configuration
// @Description  Returns the current daemon configuration
// @Tags         config
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=files.Config}
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /config [get]
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

// @Summary      Update configuration
// @Description  Updates the daemon configuration with the provided values
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        config  body      files.Config  true  "Configuration object"
// @Success      200     {object}  SuccessResponse
// @Failure      400     {object}  SuccessResponse
// @Failure      405     {object}  SuccessResponse
// @Failure      500     {object}  SuccessResponse
// @Router       /config [put]
func handleUpdateConfig(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config files.Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to decode config JSON")
			JSONError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON format")
			return
		}

		// Migrate legacy anilist_username → anilist_usernames (supports old CLI clients)
		if config.AnilistUsername != "" && len(config.AnilistUsernames) == 0 {
			config.AnilistUsernames = []string{config.AnilistUsername}
			config.AnilistUsername = ""
		}

		// save_path deixou de ser configuravel: o diretorio de download e derivado da
		// biblioteca. Zerar aqui e a vedacao que impede a API de reintroduzir o campo e
		// re-armar daemon.MigrateSavePath a cada boot. A migracao so pode ser disparada
		// por um config.json escrito por uma versao anterior, que e o caso de uso.
		config.SavePath = ""

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

		if config.MaxEpisodesPerAnime <= 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max episodes per anime must be greater than 0")
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

		if config.MaxBatchEpisodes < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max batch episodes must be non-negative")
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
