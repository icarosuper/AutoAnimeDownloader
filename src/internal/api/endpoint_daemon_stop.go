package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
)

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
