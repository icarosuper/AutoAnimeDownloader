package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/notifications"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func TestTrackCheckFailing(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)

	t.Run("fires once after 30 min and 2 passes, then the recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")

		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(10*time.Minute))
		noneReceived(t, bodies) // 2 passes but only 10 min

		trackCheckFailing(cfg, PassErrAnilist, t0.Add(31*time.Minute))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") || !strings.Contains(got, "AniList is not responding") || !strings.Contains(got, "10:00") {
			t.Errorf("got %q", got)
		}

		trackCheckFailing(cfg, PassErrAnilist, t0.Add(60*time.Minute))
		noneReceived(t, bodies) // still the same run

		trackCheckFailing(cfg, "", t0.Add(90*time.Minute))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are back to normal|") || !strings.Contains(got, "1h 30m") {
			t.Errorf("got %q", got)
		}

		trackCheckFailing(cfg, "", t0.Add(100*time.Minute))
		noneReceived(t, bodies)
	})

	t.Run("a long single failure does not fire", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, "", t0.Add(2*time.Hour))
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(3*time.Hour))
		noneReceived(t, bodies)
	})

	t.Run("a blip that never fired sends no recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(time.Minute))
		trackCheckFailing(cfg, "", t0.Add(2*time.Minute))
		noneReceived(t, bodies)
	})

	t.Run("setup incomplete is neither failure nor recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrSetup, t0)
		trackCheckFailing(cfg, PassErrSetup, t0.Add(time.Hour))
		noneReceived(t, bodies)
	})

	t.Run("a config that cannot be read waits for a pass that can send", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(nil, PassErrConfig, t0)
		trackCheckFailing(nil, PassErrConfig, t0.Add(time.Hour))
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(2*time.Hour))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") {
			t.Errorf("got %q", got)
		}
	})
}

// The store survives a restart: a run of failures that already fired still sends its recovery
// after the daemon comes back, and does not fire again.
func TestTrackCheckFailing_SurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), notifications.StateFileName)
	t.Cleanup(notifications.ResetState)
	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	cfg, bodies := alertHook(t, "check_failing")

	notifications.OpenState(path)
	trackCheckFailing(cfg, PassErrAnilist, t0)
	trackCheckFailing(cfg, PassErrAnilist, t0.Add(time.Hour))
	if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") {
		t.Fatalf("got %q", got)
	}

	notifications.OpenState(path) // restart
	trackCheckFailing(cfg, PassErrAnilist, t0.Add(2*time.Hour))
	noneReceived(t, bodies)
	trackCheckFailing(cfg, "", t0.Add(3*time.Hour))
	if got := received(bodies); !strings.HasPrefix(got, "Checks are back to normal|") {
		t.Errorf("got %q", got)
	}
}
