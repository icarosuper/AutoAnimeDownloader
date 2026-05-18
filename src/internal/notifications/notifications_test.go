package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInterpolate(t *testing.T) {
	result := interpolate("Hello {{anime_name}}, ep {{episode}}", map[string]string{
		"anime_name": "Frieren",
		"episode":    "5",
	})
	if result != "Hello Frieren, ep 5" {
		t.Fatalf("got %q", result)
	}
}

func TestInterpolateMissingVarEmpty(t *testing.T) {
	result := interpolate("{{missing}}", map[string]string{})
	if result != "" {
		t.Fatalf("missing var should be empty string, got %q", result)
	}
}

func TestFireWebhookInterpolatesURLAndBody(t *testing.T) {
	var capturedBody string
	var capturedHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		capturedHeader = r.Header.Get("Title")
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name:    "test",
		URL:     srv.URL,
		Method:  "POST",
		Headers: map[string]string{"Title": "{{title}}"},
		Body:    "{{anime_name}} EP {{episode}}",
	}

	vars := buildVars("Frieren", 5, NewEpisode)
	fireWebhook(preset, vars)

	if capturedBody != "Frieren EP 5" {
		t.Fatalf("body not interpolated: %q", capturedBody)
	}
	if capturedHeader == "" {
		t.Fatal("Title header not set")
	}
}

func TestFireTestWebhookNotFound(t *testing.T) {
	cfg := &files.Config{}
	err := FireTestWebhook(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing webhook")
	}
}

func TestFireTestWebhookFires(t *testing.T) {
	var fired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "my-hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test"},
			},
		},
	}
	if err := FireTestWebhook(cfg, "my-hook"); err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Fatal("webhook not fired")
	}
}

func TestNotifyNoOp_WhenNoWebhooks(t *testing.T) {
	Notify(&files.Config{}, NewEpisode, "Frieren", 5)
}

func TestNotifyNoOp_WhenNilConfig(t *testing.T) {
	Notify(nil, NewEpisode, "Frieren", 5)
}
