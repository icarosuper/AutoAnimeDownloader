package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// waitForChecks blocks until every manual-check goroutine started through handleCheck has
// returned. It is declared here, not in server.go, because only the tests need it: the
// endpoint's whole contract is that it answers before the verification finishes.
//
// Every test that POSTs to /check must call this before returning. See Server.checks.
func (s *Server) waitForChecks() { s.checks.Wait() }

func TestHandleCheck(t *testing.T) {
	// handleCheck runs a REAL AnimeVerification in the background: without these two guards it
	// resolves "testuser" against the live AniList API and lets files.Librarian probe (and
	// create) the configured paths on the developer's actual disk.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`)),
		}, nil
	})()

	tmp := t.TempDir()
	state := daemon.NewState()
	state.SetStatus(daemon.StatusStopped)
	mockFM := &mockFileManager{configs: &files.Config{
		AnilistUsernames:      []string{"testuser"},
		CompletedAnimePath:    filepath.Join(tmp, "completed"),
		CheckInterval:         10,
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING"},
	}}
	server := &Server{
		State:       state,
		FileManager: mockFM,
		Torrents:    torrents.NewFakeBackend(),
		Librarian:   files.NewLibrarian(files.NewOSFileSystem()),
	}

	handler := handleCheck(server)

	t.Run("POST triggers check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/check", nil)
		w := httptest.NewRecorder()

		handler(w, req)
		// The response is asserted while the verification is still in flight — that is the
		// point of the endpoint. Waiting is deferred so the goroutine cannot outlive the
		// test even when an assertion below calls t.Fatal.
		defer server.waitForChecks()

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

	// A rejected request must not have started anything in the background.
	server.waitForChecks()
}
