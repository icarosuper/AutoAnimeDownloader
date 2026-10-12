package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
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
	// DiskLow marks that the free space is below min_free_disk_percent — that is, that the daemon
	// STOPPED adding torrents. Computed on the server on purpose: a threshold duplicated in the
	// frontend would end up disagreeing with what the daemon is doing.
	DiskLow bool `json:"disk_low" example:"false"`
	// Anilist is AniList's last known state, written by ANY call (the daemon's pass or the
	// frontend's poll). It travels here, and not in its own endpoint, because /status is already
	// polled and is already the payload the WebSocket pushes — see decisions.md #66.
	Anilist anilist.Health `json:"anilist"`
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
			if diskTotal > 0 && cfg.MinFreeDiskPercent > 0 {
				diskLow = float64(diskFree)/float64(diskTotal)*100 < float64(cfg.MinFreeDiskPercent)
			}
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

		JSONSuccess(w, http.StatusOK, response)
	}
}
