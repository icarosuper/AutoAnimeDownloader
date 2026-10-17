package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleWebhookPreview_NotFound returns 404 when the webhook preset does not exist.
func TestHandleWebhookPreview_NotFound(t *testing.T) {
	server := &Server{FileManager: &mockFileManager{}}
	handler := handleNotificationWebhookPreview(server)

	body, _ := json.Marshal(map[string]string{"name": "missing"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/missing/preview", bytes.NewReader(body))
	req.SetPathValue("name", "missing")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestHandleWebhookPreview_ReturnsVars returns the interpolated URL, method, headers, and body
// without firing the webhook.
func TestHandleWebhookPreview_ReturnsVars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
	}))
	defer srv.Close()

	fm := &mockFileManager{configs: &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "my-hook", URL: "https://example.com/webhook", Method: "POST", Headers: map[string]string{"X-Test": "{{title}}"}, Body: "{{message}}"},
			},
		},
	}}
	server := &Server{FileManager: fm}
	handler := handleNotificationWebhookPreview(server)

	body, _ := json.Marshal(map[string]string{"name": "my-hook"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/webhooks/my-hook/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	t.Logf("response data: %+v", resp.Data)

	url, _ := resp.Data["url"].(string)
	method, _ := resp.Data["method"].(string)
	headers, _ := resp.Data["headers"].(map[string]interface{})
	bodyVal, _ := resp.Data["body"].(string)

	if url == "" {
		t.Fatalf("expected non-empty URL, got %q", url)
	}
	if method != "POST" {
		t.Fatalf("expected POST method, got %s", method)
	}
	if _, ok := headers["X-Test"]; !ok {
		t.Fatal("expected X-Test header in response")
	}
	if bodyVal == "" {
		t.Fatal("expected non-empty body")
	}
}

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

// O 401 do Jellyfin com token errado tem de virar erro na tela. Antes este teste falhava: o
// handler devolvia 200 "fired successfully" para qualquer coisa que nao fosse nome inexistente.
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
