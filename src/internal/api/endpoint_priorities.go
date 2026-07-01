package api

import (
	"AutoAnimeDownloader/src/internal/nyaa"
	"net/http"
)

// @Summary      Get default torrent priorities
// @Description  Returns the built-in default priority lists (for the "reset to default" button)
// @Tags         config
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=nyaa.Priorities}
// @Failure      405  {object}  SuccessResponse
// @Router       /config/priorities/defaults [get]
func handlePriorityDefaults(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		JSONSuccess(w, http.StatusOK, nyaa.DefaultPriorities())
	}
}
