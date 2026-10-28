package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/diagnostics"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"AutoAnimeDownloader/src/internal/version"
	"net/http"
	"time"
)

type StatusResponse struct {
	Status    string    `json:"status" example:"running"`
	LastCheck time.Time `json:"last_check" example:"2024-01-01T00:00:00Z"`
	HasError  bool      `json:"has_error" example:"false"`
	Version   string    `json:"version" example:"1.2.0"`
	DiskTotal uint64    `json:"disk_total" example:"500107862016"`
	DiskFree  uint64    `json:"disk_free" example:"128849018880"`
	// DiskLow marks that the daemon would refuse a new torrent of size 0: free minus what the
	// session still has to write, against min_free_disk_gb. Computed on the server on purpose: a
	// threshold duplicated in the frontend would end up disagreeing with what the daemon is doing.
	DiskLow bool `json:"disk_low" example:"false"`
	// Anilist is AniList's last known state, written by ANY call (the daemon's pass or the
	// frontend's poll). It travels here, and not in its own endpoint, because /status is already
	// polled and is already the payload the WebSocket pushes — see decisions.md #66.
	Anilist anilist.Health `json:"anilist"`
	// PendingCrashAt is when the daemon last crashed, while nobody has handled the report yet; null
	// otherwise. The UI opens the report screen (fresh page load) or toasts (tab already open).
	PendingCrashAt *time.Time `json:"pending_crash_at"`
}

// handleStatus returns the current status of the daemon, including last check time, error state,
// and disk space for the filesystem of completed_anime_path (the library)
func handleStatus(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		status, lastCheck, hasError := server.State.GetAll()

		var diskTotal, diskFree uint64
		diskLow := false
		// Same volume as the download directory, by construction (see Config.DownloadPath).
		if cfg, err := server.FileManager.LoadConfigs(); err == nil && cfg.CompletedAnimePath != "" {
			diskTotal, diskFree, _ = files.DiskSpace(cfg.CompletedAnimePath)
			var session []torrents.TorrentInfo
			if server.Torrents != nil {
				session = server.Torrents.List()
			}
			var saved []files.EpisodeStruct
			if eps, err := server.FileManager.LoadSavedEpisodes(); err == nil {
				saved = eps
			}
			diskLow = daemon.DiskLow(cfg, session, saved)
		}

		response := StatusResponse{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
			Version:   version.Version,
			DiskTotal: diskTotal,
			DiskFree:  diskFree,
			DiskLow:   diskLow,
			Anilist:   anilist.CurrentHealth(),
		}
		if server.DataDir != "" {
			response.PendingCrashAt = diagnostics.PendingCrashAt(server.DataDir)
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
