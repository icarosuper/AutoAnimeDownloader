package api

import (
	"AutoAnimeDownloader/src/internal/nyaa"
	"net/http"
)

func handlePriorityDefaults(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		JSONSuccess(w, http.StatusOK, nyaa.DefaultPriorities())
	}
}
