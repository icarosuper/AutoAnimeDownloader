package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The pass itself is covered in the daemon package; here only what the handler adds.
func TestHandleAnimeDebug_RejectsBeforeRunning(t *testing.T) {
	tests := []struct {
		name   string
		method string
		id     string
		want   int
	}{
		{"GET is not allowed", http.MethodGet, "21", http.StatusMethodNotAllowed},
		{"non-numeric id", http.MethodPost, "abc", http.StatusBadRequest},
		{"zero id", http.MethodPost, "0", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleAnimeDebug(&Server{FileManager: &mockFileManager{}})
			req := httptest.NewRequest(tt.method, "/api/v1/animes/"+tt.id+"/debug", nil)
			req.SetPathValue("id", tt.id)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, rec.Code)
			}
		})
	}
}
