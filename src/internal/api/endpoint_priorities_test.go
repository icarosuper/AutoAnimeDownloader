package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePriorityDefaults(t *testing.T) {
	server := &Server{}
	handler := handlePriorityDefaults(server)

	t.Run("GET method returns defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/config/priorities/defaults", nil)
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

		fansubs, ok := data["fansubs"].([]interface{})
		if !ok || len(fansubs) == 0 || fansubs[0] != "subsplease" {
			t.Errorf("Expected fansubs to start with 'subsplease', got %v", data["fansubs"])
		}
	})

	t.Run("Non-GET method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/config/priorities/defaults", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}
