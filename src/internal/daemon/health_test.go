package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// alertHook returns a config whose one webhook (no batch window) posts "{{title}}|{{message}}" for
// the given events, and the channel the bodies arrive on.
func alertHook(t *testing.T, events ...string) (*files.Config, chan string) {
	t.Helper()
	bodies := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	t.Cleanup(srv.Close)
	return &files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
		Name: "hook", URL: srv.URL, Method: "POST", Body: "{{title}}|{{message}}", Events: events,
	}}}}, bodies
}

// received waits for one body; "" when none arrived in time.
func received(bodies chan string) string {
	select {
	case b := <-bodies:
		return b
	case <-time.After(time.Second):
		return ""
	}
}

// noneReceived fails if a body arrives within a short wait.
func noneReceived(t *testing.T, bodies chan string) {
	t.Helper()
	select {
	case b := <-bodies:
		t.Errorf("expected no notification, got %q", b)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestNotifyPreviousCrash(t *testing.T) {
	cfg, bodies := alertHook(t, "daemon_crashed")
	at := time.Date(2026, 11, 14, 3, 12, 0, 0, time.Local)

	NotifyPreviousCrash(&orchestrationFM{configs: cfg}, at)

	got := received(bodies)
	if !strings.HasPrefix(got, "AutoAnimeDownloader crashed|") || !strings.Contains(got, "2026-11-14 03:12") {
		t.Errorf("got %q", got)
	}
}
