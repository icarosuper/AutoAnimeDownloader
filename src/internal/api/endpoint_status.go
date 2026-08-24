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
	// DiskLow marca que o livre esta abaixo de min_free_disk_percent — ou seja, que o daemon
	// PAROU de adicionar torrents. Calculado no servidor de proposito: um limiar duplicado no
	// frontend acabaria discordando do que o daemon esta fazendo.
	DiskLow bool `json:"disk_low" example:"false"`
	// Anilist e o ultimo estado conhecido da AniList, gravado por QUALQUER chamada (passe do
	// daemon ou poll do frontend). Viaja aqui, e nao num endpoint proprio, porque /status ja e
	// polled e ja e o payload que o WebSocket empurra — ver decisions.md #66.
	Anilist anilist.Health `json:"anilist"`
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
		diskLow := false
		// Mesmo volume que o diretorio de download, por construcao (ver Config.DownloadPath).
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
