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

func TestHandleDaemonStart(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{
		configs: &files.Config{
			CheckInterval: 10,
		},
	}
	
	server := &Server{
		State:         state,
		FileManager:   mockFM,
		StartLoopFunc: func(p daemon.StartLoopPayload) func(time.Duration) {
			return func(d time.Duration) {}
		},
	}

	handler := handleDaemonStart(server)

	t.Run("POST starts daemon when stopped", func(t *testing.T) {
		state.SetStatus(daemon.StatusStopped)
		
		req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/start", nil)
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

	t.Run("POST returns 400 when already running", func(t *testing.T) {
		state.SetStatus(daemon.StatusRunning)
		
		req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/start", nil)
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

		if response.Error == nil || response.Error.Code != "ALREADY_RUNNING" {
			t.Errorf("Expected error code 'ALREADY_RUNNING', got %v", response.Error)
		}
	})

	t.Run("POST returns 400 when checking", func(t *testing.T) {
		state.SetStatus(daemon.StatusChecking)
		
		req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/start", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Non-POST method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/start", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}
