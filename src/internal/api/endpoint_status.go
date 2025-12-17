package api

import (
	"net/http"
	"time"
)

type StatusResponse struct {
	Status    string    `json:"status" example:"running"`
	LastCheck time.Time `json:"last_check" example:"2024-01-01T00:00:00Z"`
	HasError  bool      `json:"has_error" example:"false"`
}

// @Summary      Get daemon status
// @Description  Returns the current status of the daemon, including last check time and error state
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
		
		response := StatusResponse{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
		}
		
		JSONSuccess(w, http.StatusOK, response)
	}
}

