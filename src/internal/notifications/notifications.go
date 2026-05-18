package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Event int

const (
	NewEpisode                  Event = iota
	DownloadFailed
	QBittorrentDownloadCompleted
)

var reVar = regexp.MustCompile(`\{\{(\w+)\}\}`)

func interpolate(template string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(template, func(match string) string {
		key := reVar.FindStringSubmatch(match)[1]
		return vars[key]
	})
}

func buildVars(animeName string, episode int, event Event) map[string]string {
	title, message := eventStrings(animeName, episode, event)
	return map[string]string{
		"title":      title,
		"message":    message,
		"anime_name": animeName,
		"episode":    fmt.Sprintf("%d", episode),
		"quality":    "",
		"file_path":  "",
		"timestamp":  time.Now().Format("2006-01-02 15:04"),
	}
}

func eventStrings(animeName string, episode int, event Event) (title, message string) {
	switch event {
	case NewEpisode:
		return "Novo episódio detectado",
			fmt.Sprintf("%s EP %d detectado, iniciando download", animeName, episode)
	case DownloadFailed:
		return "Erro no download",
			fmt.Sprintf("%s EP %d falhou após todas as tentativas", animeName, episode)
	case QBittorrentDownloadCompleted:
		return "Download concluído",
			fmt.Sprintf("%s EP %d foi baixado com sucesso", animeName, episode)
	}
	return "", ""
}

func fireWebhook(preset files.WebhookPreset, vars map[string]string) {
	url := interpolate(preset.URL, vars)
	body := interpolate(preset.Body, vars)

	req, err := http.NewRequest(preset.Method, url, strings.NewReader(body))
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Failed to build webhook request")
		return
	}
	for k, v := range preset.Headers {
		req.Header.Set(k, interpolate(v, vars))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Webhook request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Logger.Warn().
			Str("webhook", preset.Name).
			Int("status", resp.StatusCode).
			Msg("Webhook returned error status")
	}
}

// Notify fires all configured webhooks for an event in background goroutines.
// No-op if cfg is nil or has no webhooks.
func Notify(cfg *files.Config, event Event, animeName string, episode int) {
	if cfg == nil || len(cfg.Notifications.Webhooks) == 0 {
		return
	}
	vars := buildVars(animeName, episode, event)
	for _, preset := range cfg.Notifications.Webhooks {
		go func(p files.WebhookPreset) {
			fireWebhook(p, vars)
		}(preset)
	}
}

// FireTestWebhook fires one named webhook with sample variables. Returns error if not found.
func FireTestWebhook(cfg *files.Config, name string) error {
	for _, preset := range cfg.Notifications.Webhooks {
		if preset.Name == name {
			vars := buildVars("Frieren Beyond Journey's End", 5, QBittorrentDownloadCompleted)
			fireWebhook(preset, vars)
			return nil
		}
	}
	return fmt.Errorf("webhook %q not found", name)
}
