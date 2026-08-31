package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"net/http"
)

func handleCheck(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		// Execute verification in separate goroutine
		server.checks.Add(1)
		go func() {
			defer server.checks.Done()

			// Save current status to restore it after verification
			currentStatus := server.State.GetStatus()

			// Set status to checking before starting verification
			server.State.SetStatus(daemon.StatusChecking)

			ctx := context.Background()
			daemon.AnimeVerification(ctx, server.FileManager, server.State, server.JobQueue, server.Torrents, server.Librarian)

			// Restore status after verification completes
			// Only restore to running if it was running before, otherwise keep it as is
			switch currentStatus {
			case daemon.StatusRunning:
				server.State.SetStatus(daemon.StatusRunning)
			case daemon.StatusStopped:
				server.State.SetStatus(daemon.StatusStopped)
			case daemon.StatusChecking:
				// If it was checking, it means another check started, so leave it as checking
				server.State.SetStatus(daemon.StatusChecking)
			}
		}()

		logger.Logger.Info().Msg("Manual check triggered via API")

		JSONSuccess(w, http.StatusOK, map[string]string{
			"message": "Verification started",
		})
	}
}
