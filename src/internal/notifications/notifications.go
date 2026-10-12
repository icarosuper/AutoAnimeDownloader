package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type Event int

const (
	NewEpisode Event = iota
	DownloadFailed
	DownloadCompleted
	EpisodeDeleted
)

// Download-failure reasons, used as {{reason}} and in the default message.
const (
	ReasonNotFound         = "no torrent found"
	ReasonDownloadRejected = "torrent rejected"
	ReasonNoDiskSpace      = "insufficient disk space"
)

var reVar = regexp.MustCompile(`\{\{(\w+)\}\}`)

func eventString(e Event) string {
	switch e {
	case NewEpisode:
		return "new_episode"
	case DownloadFailed:
		return "download_failed"
	case DownloadCompleted:
		return "download_completed"
	case EpisodeDeleted:
		return "episode_deleted"
	}
	return ""
}

func interpolate(template string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(template, func(match string) string {
		key := reVar.FindStringSubmatch(match)[1]
		return vars[key]
	})
}

func buildVars(animeName string, episode int, event Event, reason string) map[string]string {
	return buildBatchVars(event, []item{{animeName: animeName, episode: episode, reason: reason}})
}

// buildBatchVars builds the template variables for the items of a window. With a single item the
// result is identical to before batching — which is why turning batching on does not change the
// look of notifications for anyone who still receives them one at a time.
//
// With N > 1, `anime_name`, `episode` and `reason` stay EMPTY: there is no single value for them
// and sending the first item's would make the template lie about the other N-1. Whoever needs
// to identify the episodes uses `message` (one line per item) or `count`.
func buildBatchVars(event Event, items []item) map[string]string {
	lines := make([]string, 0, len(items))
	for _, it := range items {
		_, message := eventStrings(it.animeName, it.episode, event, it.reason)
		lines = append(lines, message)
	}

	vars := map[string]string{
		"title":      batchTitle(event, items),
		"message":    strings.Join(lines, "\n"),
		"anime_name": "",
		"episode":    "",
		"reason":     "",
		"count":      fmt.Sprintf("%d", len(items)),
		"quality":    "",
		"file_path":  "",
		"timestamp":  time.Now().Format("2006-01-02 15:04"),
	}

	if len(items) == 1 {
		vars["anime_name"] = items[0].animeName
		vars["episode"] = fmt.Sprintf("%d", items[0].episode)
		vars["reason"] = items[0].reason
	}

	return vars
}

func batchTitle(event Event, items []item) string {
	if len(items) == 1 {
		title, _ := eventStrings(items[0].animeName, items[0].episode, event, items[0].reason)
		return title
	}
	switch event {
	case NewEpisode:
		return fmt.Sprintf("%d new episodes detected", len(items))
	case DownloadFailed:
		return fmt.Sprintf("%d download errors", len(items))
	case DownloadCompleted:
		return fmt.Sprintf("%d downloads completed", len(items))
	case EpisodeDeleted:
		return fmt.Sprintf("%d episodes deleted", len(items))
	}
	return ""
}

func eventStrings(animeName string, episode int, event Event, reason string) (title, message string) {
	switch event {
	case NewEpisode:
		return "New episode detected",
			fmt.Sprintf("%s EP %d detected, starting download", animeName, episode)
	case DownloadFailed:
		if reason == "" {
			reason = "all attempts failed"
		}
		return "Download failed",
			fmt.Sprintf("%s EP %d failed: %s", animeName, episode, reason)
	case DownloadCompleted:
		return "Download completed",
			fmt.Sprintf("%s EP %d downloaded successfully", animeName, episode)
	case EpisodeDeleted:
		return "Episode deleted",
			fmt.Sprintf("%s EP %d was removed from the library", animeName, episode)
	}
	return "", ""
}

// jsonEscape returns the value ready to be pasted INSIDE a JSON string (without the outer
// quotes). An anime name with quotes, a backslash, or the batching \n would break the body of
// presets that embed {{message}} in JSON (Discord, Slack, Telegram, Gotify, Pushover, Apprise):
// the service would answer 400 and the notification would vanish with no explanation in the log.
func jsonEscape(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil { // json.Marshal of a string does not fail; if it did, the raw value beats nothing
		return value
	}
	return string(encoded[1 : len(encoded)-1])
}

// escapeVarsForJSON applies jsonEscape to every variable. Used only on the body, and only when
// the preset declares itself JSON: the URL has its own escaping rules and a plain-text body
// (ntfy's is raw `{{message}}`) must receive a real \n.
func escapeVarsForJSON(vars map[string]string) map[string]string {
	escaped := make(map[string]string, len(vars))
	for k, v := range vars {
		escaped[k] = jsonEscape(v)
	}
	return escaped
}

func presetIsJSON(preset files.WebhookPreset) bool {
	for k, v := range preset.Headers {
		if strings.EqualFold(k, "Content-Type") && strings.Contains(strings.ToLower(v), "json") {
			return true
		}
	}
	return false
}

// fireWebhook fires a preset and returns what went wrong. Automatic delivery ignores the
// return (the log already records it); the "Test" button needs it, or it would report success
// while the service refused the token — see decisions.md #86.
func fireWebhook(preset files.WebhookPreset, vars map[string]string) error {
	bodyVars := vars
	if presetIsJSON(preset) {
		bodyVars = escapeVarsForJSON(vars)
	}

	url := interpolate(preset.URL, vars)
	body := interpolate(preset.Body, bodyVars)

	req, err := http.NewRequest(preset.Method, url, strings.NewReader(body))
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Failed to build webhook request")
		return err
	}
	for k, v := range preset.Headers {
		// A header with \n makes net/http reject the whole request. ntfy uses
		// `Title: {{title}}`, and nothing stops someone from putting {{message}} in a header.
		req.Header.Set(k, strings.NewReplacer("\n", " ", "\r", " ").Replace(interpolate(v, vars)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Webhook request failed")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Logger.Warn().
			Str("webhook", preset.Name).
			Int("status", resp.StatusCode).
			Msg("Webhook returned error status")
		return fmt.Errorf("webhook %q returned status %d", preset.Name, resp.StatusCode)
	}
	return nil
}

// item is an event waiting in the batching window.
type item struct {
	animeName string
	episode   int
	reason    string
}

// batch accumulates the items of one event's window. The timer is stored with it so Flush can
// cancel it: without that, the shutdown flush and the timer would race on the same queue.
type batch struct {
	items   []item
	timer   *time.Timer
	webhook []files.WebhookPreset
}

var (
	batchMu      sync.Mutex
	pendingBatch = make(map[Event]*batch)
)

// Notify fires webhooks subscribed to the given event in background goroutines.
// No-op if cfg is nil or has no webhooks.
//
// With cfg.Notifications.BatchWindowSeconds > 0 the event enters a per-event-type queue and
// leaves with the others of the same window, as a single webhook. A library backfill fires one
// DownloadCompleted per torrent (77 in a real case) and services such as ntfy.sh answer 429
// past a certain volume — see decisions.md #47.
func Notify(cfg *files.Config, event Event, animeName string, episode int, reason string) {
	if cfg == nil || len(cfg.Notifications.Webhooks) == 0 {
		return
	}

	it := item{animeName: animeName, episode: episode, reason: reason}
	window := time.Duration(cfg.Notifications.BatchWindowSeconds) * time.Second
	if window <= 0 {
		fireBatch(event, []item{it}, cfg.Notifications.Webhooks, false)
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	if b, ok := pendingBatch[event]; ok {
		b.items = append(b.items, it)
		// The window's webhooks are those of the last config seen: if the user edited the
		// presets mid-window, the latest one wins.
		b.webhook = cfg.Notifications.Webhooks
		return
	}

	b := &batch{items: []item{it}, webhook: cfg.Notifications.Webhooks}
	pendingBatch[event] = b
	b.timer = time.AfterFunc(window, func() { flushEvent(event, false) })
}

// Flush fires everything currently waiting in a window and ONLY returns once the requests have
// finished. Called on daemon shutdown: without it a whole window of notifications would die
// with the process — and firing in a goroutine on shutdown would be the same as not firing,
// because the process exits before they run.
func Flush() {
	batchMu.Lock()
	events := make([]Event, 0, len(pendingBatch))
	for event := range pendingBatch {
		events = append(events, event)
	}
	batchMu.Unlock()

	for _, event := range events {
		flushEvent(event, true)
	}
}

func flushEvent(event Event, wait bool) {
	batchMu.Lock()
	b, ok := pendingBatch[event]
	if !ok {
		batchMu.Unlock()
		return
	}
	delete(pendingBatch, event)
	if b.timer != nil {
		b.timer.Stop()
	}
	batchMu.Unlock()

	fireBatch(event, b.items, b.webhook, wait)
}

func fireBatch(event Event, items []item, webhooks []files.WebhookPreset, wait bool) {
	if len(items) == 0 {
		return
	}
	eventStr := eventString(event)
	vars := buildBatchVars(event, items)

	var wg sync.WaitGroup
	for _, preset := range webhooks {
		if !slices.Contains(preset.Events, eventStr) {
			continue
		}
		wg.Add(1)
		go func(p files.WebhookPreset) {
			defer wg.Done()
			fireWebhook(p, vars)
		}(preset)
	}
	if wait {
		wg.Wait()
	}
}

// ErrWebhookNotFound distinguishes "this preset does not exist" (404) from "the preset exists
// and delivery failed" (502). Without the distinction the handler would return 404 for a token
// the service refused.
var ErrWebhookNotFound = errors.New("webhook not found")

// FireTestWebhook fires one named webhook with sample variables. Returns ErrWebhookNotFound if the
// preset does not exist, or the delivery error if the service refused it.
func FireTestWebhook(cfg *files.Config, name string) error {
	for _, preset := range cfg.Notifications.Webhooks {
		if preset.Name == name {
			vars := buildVars("Frieren Beyond Journey's End", 5, DownloadCompleted, "")
			return fireWebhook(preset, vars)
		}
	}
	return fmt.Errorf("%w: %q", ErrWebhookNotFound, name)
}
