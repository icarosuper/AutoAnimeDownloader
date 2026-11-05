package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestHandleWebhookPreview_RendersBodyPresetWithoutSending: the preset comes in the body (the UI
// previews an unsaved form), it is rendered with the sample values and the URL is never called.
func TestHandleWebhookPreview_RendersBodyPresetWithoutSending(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()

	handler := handleNotificationWebhookPreview(&Server{FileManager: &mockFileManager{}})
	body := `{"name":"unsaved","url":"` + srv.URL + `/{{anime_id}}","method":"POST",` +
		`"headers":{"Content-Type":"application/json","Title":"{{title}}"},"body":"{\"text\":\"{{message}}\"}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/preview", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.URL != srv.URL+"/154587" {
		t.Errorf("url = %q", resp.Data.URL)
	}
	if resp.Data.Headers["Title"] != "Download completed" {
		t.Errorf("Title header = %q", resp.Data.Headers["Title"])
	}
	// JSON preset: the sample message has to come out escaped, as valid JSON.
	if !json.Valid([]byte(resp.Data.Body)) {
		t.Errorf("body is not valid JSON: %q", resp.Data.Body)
	}
	if called {
		t.Error("preview sent the request")
	}
}

func TestHandleWebhookPreview_InvalidBody(t *testing.T) {
	handler := handleNotificationWebhookPreview(&Server{FileManager: &mockFileManager{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/preview", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
