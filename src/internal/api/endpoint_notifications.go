package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"net/http"
)

// @Summary      Test a webhook preset
// @Description  Fires a named webhook with sample variables to verify connectivity
// @Tags         notifications
// @Produce      json
// @Param        name  path      string  true  "Webhook preset name"
// @Success      200   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Failure      500   {object}  SuccessResponse
// @Router       /notifications/webhooks/{name}/test [post]
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
		if err := notifications.FireTestWebhook(cfg, name); err != nil {
			JSONError(w, http.StatusNotFound, "WEBHOOK_NOT_FOUND", err.Error())
			return
		}
		JSONSuccess(w, http.StatusOK, map[string]string{"message": "Webhook fired successfully"})
	}
}
