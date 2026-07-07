package api

import (
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
}

// @Summary      Get daemon status
// @Description  Returns the current status of the daemon, including last check time, error state, and disk space for the configured save path
// @Tags         status
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=StatusResponse}
// @Failure      405  {object}  SuccessResponse
// @Router       /status [get]
func handleStatus(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		status, lastCheck, hasError := server.State.GetAll()

		var diskTotal, diskFree uint64
		if cfg, err := server.FileManager.LoadConfigs(); err == nil && cfg.SavePath != "" {
			diskTotal, diskFree, _ = files.DiskSpace(cfg.SavePath)
		}

		response := StatusResponse{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
			Version:   version.Version,
			DiskTotal: diskTotal,
			DiskFree:  diskFree,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
