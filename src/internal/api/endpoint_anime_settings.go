package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
	"strconv"
)

// Progress e PONTEIRO porque o PUT e parcial: ausente tem de ser distinguivel de zero, ou um
// corpo sem o campo zeraria o progresso salvo.
type animeSettingsRequest struct {
	Progress *int `json:"progress"`
}

func handleAnimeSettings(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		switch r.Method {
		case http.MethodGet:
			settings, err := server.FileManager.LoadAnimeSettings(id)
			if err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to load anime settings")
				JSONInternalError(w, err)
				return
			}
			JSONSuccess(w, http.StatusOK, settings)

		case http.MethodPut:
			var req animeSettingsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
				return
			}
			if req.Progress != nil && *req.Progress < 0 {
				JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Progress must be non-negative")
				return
			}

			existing, err := server.FileManager.LoadAnimeSettings(id)
			if err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to load anime settings")
				JSONInternalError(w, err)
				return
			}

			settings := files.AnimeSettings{}
			if existing != nil {
				settings = *existing
			}
			if req.Progress != nil {
				settings.Progress = *req.Progress
			}

			if err := server.FileManager.SaveAnimeSettings(id, settings); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to save anime settings")
				JSONInternalError(w, err)
				return
			}

			JSONSuccess(w, http.StatusOK, nil)

		default:
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and PUT methods are allowed")
		}
	}
}
