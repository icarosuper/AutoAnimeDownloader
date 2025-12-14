package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"context"
	"net/http"
)

// @Summary      Trigger manual check
// @Description  Triggers a manual anime verification check
// @Tags         daemon
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Router       /check [post]
func handleCheck(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}
		
		// Execute verification in separate goroutine
		go func() {
			ctx := context.Background()
			daemon.AnimeVerification(ctx, server.FileManager, server.State)
		}()
		
		JSONSuccess(w, http.StatusOK, map[string]string{
			"message": "Verification started",
		})
	}
}

