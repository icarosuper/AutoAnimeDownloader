package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
)

// handleConfig retorna o handler para GET e PUT /api/v1/config
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

// handleGetConfig implementa GET /api/v1/config
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

// handleUpdateConfig implementa PUT /api/v1/config
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

		// Validar campos obrigatórios
		if config.AnilistUsername == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Anilist username is required")
			return
		}

		if config.SavePath == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Save path is required")
			return
		}

		if config.QBittorrentUrl == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "qBittorrent URL is required")
			return
		}

		// Validar valores numéricos
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

		// Salvar configurações
		if err := server.FileManager.SaveConfigs(&config); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to save configs")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Configuration updated successfully"})
	}
}
