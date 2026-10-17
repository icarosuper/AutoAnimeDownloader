package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"encoding/json"
	"errors"
	"fmt"
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
		// 502 e nao 200: o disparo que o servico recusou (token errado, URL fora do ar) precisa
		// chegar ao toast. Um "sucesso" aqui so provava que o preset existe.
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

func handleNotificationWebhookPreview(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
			return
		}
		preview := struct {
			Name string `json:"name"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&preview); err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
			return
		}
		cfg, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs for webhook preview")
			JSONInternalError(w, err)
			return
		}
		var preset *files.WebhookPreset
		for i := range cfg.Notifications.Webhooks {
			if cfg.Notifications.Webhooks[i].Name == preview.Name {
				preset = &cfg.Notifications.Webhooks[i]
				break
			}
		}
		if preset == nil {
			JSONError(w, http.StatusNotFound, "WEBHOOK_NOT_FOUND", fmt.Sprintf("webhook preset %q not found", preview.Name))
			return
		}
		url, method, headers, body := notifications.WebhookPreview(*preset)
		JSONSuccess(w, http.StatusOK, map[string]interface{}{
			"url":     url,
			"method":  method,
			"headers": headers,
			"body":    body,
		})
	}
}
