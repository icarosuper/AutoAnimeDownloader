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

	tmpDir := t.TempDir()
	fileManager := &mockFileManager{configs: &files.Config{CompletedAnimePath: tmpDir}}
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

		diskTotal, ok := data["disk_total"].(float64)
		if !ok || diskTotal <= 0 {
			t.Errorf("Expected disk_total > 0, got %v", data["disk_total"])
		}
		diskFree, ok := data["disk_free"].(float64)
		if !ok || diskFree < 0 {
			t.Errorf("Expected disk_free >= 0, got %v", data["disk_free"])
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

// TestHandleStatus_DiskLow: o limiar de "disco baixo" vive no servidor (min_free_disk_percent), e
// e o mesmo que faz o daemon parar de adicionar torrents.
func TestHandleStatus_DiskLow(t *testing.T) {
	cases := []struct {
		name    string
		configs *files.Config
		want    bool
	}{
		// 100% livre e impossivel num volume em uso.
		{"abaixo do teto", &files.Config{CompletedAnimePath: t.TempDir(), MinFreeDiskPercent: 100}, true},
		{"acima do teto", &files.Config{CompletedAnimePath: t.TempDir(), MinFreeDiskPercent: 1}, false},
		{"guarda desligada", &files.Config{CompletedAnimePath: t.TempDir(), MinFreeDiskPercent: 0}, false},
		// Biblioteca nao configurada: disk_total == 0, nada a afirmar.
		{"biblioteca nao configurada", &files.Config{MinFreeDiskPercent: 100}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{State: daemon.NewState(), FileManager: &mockFileManager{configs: tc.configs}}
			w := httptest.NewRecorder()
			handleStatus(server)(w, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

			var response SuccessResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("decode: %v", err)
			}
			data := response.Data.(map[string]interface{})
			if got := data["disk_low"].(bool); got != tc.want {
				t.Errorf("disk_low: esperava %v, obteve %v (total=%v free=%v)", tc.want, got, data["disk_total"], data["disk_free"])
			}
		})
	}
}

// TestHandleDownloadEpisode_DiskFullReturns409: a causa e conhecida e acionavel pelo usuario, logo
// 409 e nao 500.
func TestHandleDownloadEpisode_DiskFullReturns409(t *testing.T) {
	fm := &mockFileManager{configs: &files.Config{
		AnilistUsernames:    []string{"testuser"},
		CompletedAnimePath:  t.TempDir(),
		MinFreeDiskPercent:  100,
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   1,
	}}
	server, _ := episodeActionServer(t, fm)

	restore := mockAnimeInfo()
	defer restore()

	rec := httptest.NewRecorder()
	handleDownloadEpisode(server)(rec, episodeRequest(http.MethodPost, "/api/v1/animes/7/episodes/42/download", ""))

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperava 409, obteve %d: %s", rec.Code, rec.Body.String())
	}
	var response SuccessResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Error == nil || response.Error.Code != "INSUFFICIENT_DISK_SPACE" {
		t.Errorf("esperava INSUFFICIENT_DISK_SPACE, obteve %+v", response.Error)
	}
}
