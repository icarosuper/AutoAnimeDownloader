package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleWebhookTest_NotFound(t *testing.T) {
	server := &Server{FileManager: &mockFileManager{}}
	handler := handleNotificationWebhookTest(server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/missing/test", nil)
	req.SetPathValue("name", "missing")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleWebhookTest_MethodNotAllowed(t *testing.T) {
	server := &Server{FileManager: &mockFileManager{}}
	handler := handleNotificationWebhookTest(server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/webhooks/x/test", nil)
	req.SetPathValue("name", "x")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleWebhookTest_Fires(t *testing.T) {
	var fired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
	}))
	defer srv.Close()

	fm := &mockFileManager{configs: &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "my-hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test"},
			},
		},
	}}
	server := &Server{FileManager: fm}
	handler := handleNotificationWebhookTest(server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/my-hook/test", nil)
	req.SetPathValue("name", "my-hook")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	if !fired {
		t.Fatal("webhook not fired")
	}
}

// A Jellyfin 401 with a bad token has to become an error on screen. This test used to fail:
// the handler returned 200 "fired successfully" for anything that was not a missing name.
func TestHandleWebhookTest_ServiceRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	fm := &mockFileManager{configs: &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "jellyfin", URL: srv.URL, Method: "POST", Headers: map[string]string{}},
			},
		},
	}}
	handler := handleNotificationWebhookTest(&Server{FileManager: fm})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/jellyfin/test", nil)
	req.SetPathValue("name", "jellyfin")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d — body: %s", rec.Code, rec.Body.String())
	}
}
