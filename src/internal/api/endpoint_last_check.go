package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"net/http"
)

// handleLastCheck returns why the last automatic pass did not download episodes, aggregated per
// anime. `problems` are things that should have downloaded and did not; `limits` are the
// configuration working as configured. `pass_error` is non-empty when the pass itself aborted, and
// then both lists are empty. A clean pass answers 200 with two empty lists; a `finished_at` of
// zero means the daemon has not completed a pass yet. Manual downloads are out of scope — those
// report their failure in their own HTTP response.
func handleLastCheck(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		// Copy by value (see GetLastCheckReport): filling pass_error here must not write to the
		// shared report.
		report := server.State.GetLastCheckReport()
		// A nil list serializes as null — a pass with no limit, or a report cleared by
		// SetLastCheckError — and the Status screen reads .length of both: a null froze it on
		// "Loading status...".
		if report.Problems == nil {
			report.Problems = []daemon.Issue{}
		}
		if report.Limits == nil {
			report.Limits = []daemon.Issue{}
		}
		if err := server.State.GetLastCheckError(); err != nil {
			report.PassError = err.Error()
			report.PassErrorCode = server.State.GetLastCheckErrorCode()
		}

		JSONSuccess(w, http.StatusOK, report)
	}
}
