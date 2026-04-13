package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
	"strconv"
)

type animeSettingsRequest struct {
	CustomSearchQuery string `json:"custom_search_query"`
}

// @Summary      Get or update anime-specific settings
// @Description  GET returns current settings; PUT updates them
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id   path int true "Anime ID (AniList MediaList ID)"
// @Param        body body animeSettingsRequest false "Settings"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/settings [get]
// @Router       /animes/{id}/settings [put]
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

			settings := files.AnimeSettings{
				CustomSearchQuery: req.CustomSearchQuery,
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
