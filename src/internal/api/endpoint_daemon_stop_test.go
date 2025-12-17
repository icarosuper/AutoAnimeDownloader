package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDaemonStop(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleDaemonStop(server)

	t.Run("POST stops daemon when running", func(t *testing.T) {
		state.SetStatus(daemon.StatusRunning)
		
		req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/stop", nil)
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
	})

	t.Run("POST returns 400 when already stopped", func(t *testing.T) {
		state.SetStatus(daemon.StatusStopped)
		
		req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/stop", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response.Success {
			t.Error("Expected success=false")
		}

		if response.Error == nil || response.Error.Code != "ALREADY_STOPPED" {
			t.Errorf("Expected error code 'ALREADY_STOPPED', got %v", response.Error)
		}
	})

	t.Run("Non-POST method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/stop", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}
