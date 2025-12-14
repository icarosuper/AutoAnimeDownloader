package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"bufio"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// handleLogs retorna o handler para GET /api/v1/logs
// @Summary      Get logs
// @Description  Returns the last N lines from the daemon log file, optionally filtered by level
// @Tags         logs
// @Accept       json
// @Produce      json
// @Param        lines  query     int     false  "Number of lines to return (default: 100)"
// @Param        level  query     string  false  "Filter by log level (debug, info, warn, error)"
// @Success      200    {object}  SuccessResponse{data=object{lines=[]string,total=int}}
// @Failure      404    {object}  SuccessResponse
// @Failure      405    {object}  SuccessResponse
// @Failure      500    {object}  SuccessResponse
// @Router       /logs [get]
func handleLogs(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		
		logFilePath := logger.GetLogFilePath()
		if logFilePath == "" {
			JSONError(w, http.StatusNotFound, "LOG_FILE_NOT_FOUND", "Log file path is not configured")
			return
		}
		
		// Parse query parameters
		linesParam := r.URL.Query().Get("lines")
		levelParam := r.URL.Query().Get("level")
		
		lines := 100 // default
		if linesParam != "" {
			if parsedLines, err := strconv.Atoi(linesParam); err == nil && parsedLines > 0 {
				lines = parsedLines
			}
		}
		
		// Ler arquivo de log
		file, err := os.Open(logFilePath)
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to open log file")
			JSONError(w, http.StatusInternalServerError, "LOG_READ_ERROR", "Failed to read log file")
			return
		}
		defer file.Close()
		
		// Ler todas as linhas
		var allLines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}
		
		if err := scanner.Err(); err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to scan log file")
			JSONError(w, http.StatusInternalServerError, "LOG_READ_ERROR", "Failed to read log file")
			return
		}
		
		// Pegar últimas N linhas
		startIdx := 0
		if len(allLines) > lines {
			startIdx = len(allLines) - lines
		}
		resultLines := allLines[startIdx:]
		
		// Filtrar por nível se especificado
		if levelParam != "" {
			filteredLines := make([]string, 0)
			levelLower := strings.ToLower(levelParam)
			for _, line := range resultLines {
				lineLower := strings.ToLower(line)
				if strings.Contains(lineLower, `"level":"`+levelLower+`"`) ||
					strings.Contains(lineLower, "level="+levelLower) ||
					strings.Contains(lineLower, "["+levelLower+"]") {
					filteredLines = append(filteredLines, line)
				}
			}
			resultLines = filteredLines
		}
		
		JSONSuccess(w, http.StatusOK, map[string]interface{}{
			"lines": resultLines,
			"total": len(resultLines),
		})
	}
}

