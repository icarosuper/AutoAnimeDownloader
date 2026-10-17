package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
)

// handleAnimeDebug runs the daemon's search/match pipeline for one anime over HTTP and returns
// the machine-readable summary. It is the remote equivalent of the daemon's --debug-anime flag,
// so the CLI does not have to shell out to the daemon binary: `autoanimedownloader debug <id>`
// becomes one API call.
//
// Nothing is downloaded and nothing in downloaded_episodes is touched — the trace (AniList
// response, every Nyaa row, which matched) still goes through the logger, so it shows up in
// `logs` like any other daemon event.
func handleAnimeDebug(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
			return
		}

		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs for debug run")
			JSONInternalError(w, err)
			return
		}

		summary, err := daemon.RunAnimeDebug(id, configs, server.FileManager)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Debug run failed")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, summary)
	}
}