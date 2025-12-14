package api

import (
	"AutoAnimeDownloader/src/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
)

// @Summary      Stop daemon
// @Description  Stops the daemon loop
// @Tags         daemon
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Router       /daemon/stop [post]
func handleDaemonStop(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}
		
		currentStatus := server.State.GetStatus()
		if currentStatus == daemon.StatusStopped {
			JSONError(w, http.StatusBadRequest, "ALREADY_STOPPED", "Daemon is already stopped")
			return
		}
		
		server.StopDaemonLoop()
		
		logger.Logger.Info().Msg("Daemon stop requested via API")
		
		JSONSuccess(w, http.StatusOK, map[string]string{
			"message": "Daemon stopped",
		})
	}
}

