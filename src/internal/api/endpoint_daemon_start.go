package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
)

// @Summary      Start daemon
// @Description  Starts the daemon loop for automatic anime checking
// @Tags         daemon
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /daemon/start [post]
func handleDaemonStart(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}
		
		currentStatus := server.State.GetStatus()
		if currentStatus == daemon.StatusRunning || currentStatus == daemon.StatusChecking {
			JSONError(w, http.StatusBadRequest, "ALREADY_RUNNING", "Daemon is already running")
			return
		}
		
		if err := server.StartDaemonLoop(); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to start daemon loop")
			JSONInternalError(w, err)
			return
		}
		
		JSONSuccess(w, http.StatusOK, map[string]string{
			"message": "Daemon started",
		})
	}
}

