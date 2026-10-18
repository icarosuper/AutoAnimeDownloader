package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"encoding/json"
	"errors"
	"net/http"
)

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *ErrorInfo  `json:"error"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSONSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := SuccessResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func JSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := SuccessResponse{
		Success: false,
		Data:    nil,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode error response", http.StatusInternalServerError)
	}
}

func JSONInternalError(w http.ResponseWriter, err error) {
	JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

// JSONDownloadError maps a manual-download failure. Full disk is 409, not 500: the cause is
// known and the user can act on it.
func JSONDownloadError(w http.ResponseWriter, err error, code string) {
	if errors.Is(err, daemon.ErrInsufficientDiskSpace) {
		JSONError(w, http.StatusConflict, "INSUFFICIENT_DISK_SPACE", err.Error())
		return
	}
	JSONError(w, http.StatusInternalServerError, code, err.Error())
}
