package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
)

// handleAnimeDebug runs the --debug-anime pass inside the running daemon and answers its summary,
// so a daemon in a container or on another machine can be debugged without a shell next to it.
//
// Two differences from the flag: the trace (AniList response, every Nyaa row, what matched) goes to
// the daemon's own log instead of a .debug_<id>_N/debug.jsonl, and the summary comes back in the
// response instead of summary.json. Nothing is downloaded and episodes.json is not touched.
//
// It can take minutes (Nyaa paging plus the per-episode fallback), which is why the CLI calls it
// with a longer timeout — see Client.sendSlow.
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
			logger.Logger.Error().Err(err).Msg("Failed to load configs for the debug pass")
			JSONInternalError(w, err)
			return
		}

		logger.Logger.Info().Int("anime_id", id).Msg("Debug pass requested through the API")
		summary, err := daemon.RunAnimeDebug(id, configs, server.FileManager)
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Debug pass failed")
			JSONInternalError(w, err)
			return
		}
		JSONSuccess(w, http.StatusOK, summary)
	}
}
