package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCheck(t *testing.T) {
	state := daemon.NewState()
	state.SetStatus(daemon.StatusStopped)
	mockFM := &mockFileManager{}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleCheck(server)

	t.Run("POST triggers check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/check", nil)
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

		if message, ok := data["message"].(string); !ok || message != "Verification started" {
			t.Errorf("Expected message 'Verification started', got %v", data["message"])
		}
	})

	t.Run("Non-POST method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/check", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}
