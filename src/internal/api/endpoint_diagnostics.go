package api

import (
	"AutoAnimeDownloader/src/internal/diagnostics"
	"net/http"
)

// handleDiagnostics builds the bug report bundle the report screen previews and downloads. Nothing
// is sent anywhere: the user attaches the file by hand (decisions.md #102).
func handleDiagnostics(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		if server.DataDir == "" {
			JSONError(w, http.StatusInternalServerError, "DATA_DIR_UNKNOWN", "Data directory not available")
			return
		}

		q := r.URL.Query()
		report := diagnostics.Build(server.DataDir, diagnostics.Options{
			// Titles stay in unless explicitly turned off: the checkbox is checked by default.
			Titles:  q.Get("titles") != "false",
			Error:   q.Get("error"),
			Context: q.Get("context"),
			AnimeID: q.Get("anime"),
		})
		JSONSuccess(w, http.StatusOK, report)
	}
}

// handleDiagnosticsCrash marks the pending crash as handled (reported, downloaded or dismissed):
// it moves to the crashes/ history. Idempotent — a second tab may have handled it already.
func handleDiagnosticsCrash(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is allowed")
			return
		}
		if server.DataDir != "" {
			if err := diagnostics.ArchivePendingCrash(server.DataDir); err != nil {
				JSONInternalError(w, err)
				return
			}
		}
		JSONSuccess(w, http.StatusOK, nil)
	}
}
