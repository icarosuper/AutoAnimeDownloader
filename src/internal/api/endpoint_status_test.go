package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleStatus(t *testing.T) {
	state := daemon.NewState()
	state.SetStatus(daemon.StatusRunning)
	state.SetLastCheck(time.Now())
	state.SetLastCheckError(nil)

	fileManager := &files.FileManager{}
	server := &Server{
		State:       state,
		FileManager: fileManager,
	}

	handler := handleStatus(server)

	t.Run("GET method returns status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Error("Expected success=true")
		}

		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be an object")
		}

		if status, ok := data["status"].(string); !ok || status != "running" {
			t.Errorf("Expected status 'running', got %v", data["status"])
		}
	})

	t.Run("Non-GET method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response.Success {
			t.Error("Expected success=false")
		}

		if response.Error == nil {
			t.Fatal("Expected error field")
		}

		if response.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("Expected error code 'METHOD_NOT_ALLOWED', got %s", response.Error.Code)
		}
	})
}
