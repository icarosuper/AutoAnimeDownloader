package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"bufio"
	"net/http"
	"os"
	"strconv"
)

type LogsResponse struct {
	Lines []string `json:"lines"`
}

// @Summary      Get daemon logs
// @Description  Returns the last N lines from the daemon log file
// @Tags         logs
// @Accept       json
// @Produce      json
// @Param        lines  query     int  false  "Number of lines to return (default: 1000, max: 10000)"
// @Success      200    {object}  SuccessResponse{data=LogsResponse}
// @Failure      500    {object}  ErrorResponse
// @Router       /logs [get]
func handleLogs(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		// Get number of lines from query parameter (default: 1000, max: 10000)
		lines := 1000
		if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
			var err error
			lines, err = parseInt(linesParam)
			if err != nil || lines < 1 {
				JSONError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid lines parameter")
				return
			}
			if lines > 10000 {
				lines = 10000
			}
		}

		logFilePath := logger.GetLogFilePath()
		if logFilePath == "" {
			// Try to get expected log file path
			expectedPath, err := logger.GetExpectedLogFilePath()
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "LOG_FILE_NOT_FOUND", "Log file path not available")
				return
			}
			logFilePath = expectedPath
		}

		// Read log file
		file, err := os.Open(logFilePath)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "LOG_FILE_READ_ERROR", "Failed to read log file")
			return
		}
		defer file.Close()

		// Read all lines
		var allLines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			JSONError(w, http.StatusInternalServerError, "LOG_FILE_SCAN_ERROR", "Failed to scan log file")
			return
		}

		// Get last N lines
		start := 0
		if len(allLines) > lines {
			start = len(allLines) - lines
		}
		resultLines := allLines[start:]

		response := LogsResponse{
			Lines: resultLines,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}

// Helper function to parse integer from string
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

