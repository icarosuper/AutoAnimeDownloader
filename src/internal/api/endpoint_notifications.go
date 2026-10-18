package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"errors"
	"net/http"
)

func handleNotificationWebhookTest(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
			return
		}
		name := r.PathValue("name")
		cfg, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs for webhook test")
			JSONInternalError(w, err)
			return
		}
		// 502, not 200: a fire the service refused (bad token, URL down) has to reach the
		// toast. A "success" here only proved the preset exists.
		if err := notifications.FireTestWebhook(cfg, name); err != nil {
			if errors.Is(err, notifications.ErrWebhookNotFound) {
				JSONError(w, http.StatusNotFound, "WEBHOOK_NOT_FOUND", err.Error())
				return
			}
			JSONError(w, http.StatusBadGateway, "WEBHOOK_FAILED", err.Error())
			return
		}
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Webhook fired successfully"})
	}
}
