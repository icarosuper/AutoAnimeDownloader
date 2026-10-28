package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
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

// TestHandleStatus_DiskLow: the "low disk" threshold lives on the server (min_free_disk_gb),
// and it is the same one that makes the daemon stop adding torrents.
func TestHandleStatus_DiskLow(t *testing.T) {
	below := t.TempDir()
	_, free, err := files.DiskSpace(below)
	if err != nil || free == 0 {
		t.Skipf("no free-space reading for the temp dir: %v", err)
	}
	above := t.TempDir()
	if free < 1<<30 {
		t.Skip("need at least 1 GiB free to assert the floor passing")
	}

	cases := []struct {
		name    string
		configs *files.Config
		want    bool
	}{
		{"below the cap", &files.Config{CompletedAnimePath: below, MinFreeDiskGB: int(free>>30) + 1}, true},
		{"above the cap", &files.Config{CompletedAnimePath: above, MinFreeDiskGB: 1}, false},
		{"guard off", &files.Config{CompletedAnimePath: t.TempDir(), MinFreeDiskGB: 0}, false},
		{"library not configured", &files.Config{MinFreeDiskGB: 100}, false},
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
				t.Errorf("disk_low: want %v, got %v (total=%v free=%v)", tc.want, got, data["disk_total"], data["disk_free"])
			}
		})
	}
}

func TestHandleStatus_DiskLowWhenReservedPushesUnderTheFloor(t *testing.T) {
	dir := t.TempDir()
	_, free, err := files.DiskSpace(dir)
	if err != nil || free < 3<<30 {
		t.Skipf("need a few GiB free to split between reserved and leftover: %v", err)
	}
	const giB = int64(1 << 30)
	minGB := 10
	if free < uint64(minGB)*uint64(giB)+2*uint64(giB) {
		minGB = max(1, int(free>>30)/2)
	}
	reserved := int64(free) - int64(minGB-1)*giB

	backend := torrents.NewFakeBackend()
	backend.AddPaused("queuedhash", "pack", 0, 0, false)
	fm := &mockFileManager{
		configs:  &files.Config{CompletedAnimePath: dir, MinFreeDiskGB: minGB},
		episodes: []files.EpisodeStruct{{EpisodeHash: "queuedhash", TorrentSize: reserved}},
	}
	server := &Server{State: daemon.NewState(), FileManager: fm, Torrents: backend}
	w := httptest.NewRecorder()
	handleStatus(server)(w, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data := response.Data.(map[string]interface{})
	if got := data["disk_low"].(bool); !got {
		t.Errorf("disk_low: want true when reserved alone trips the floor, got false (total=%v free=%v)", data["disk_total"], data["disk_free"])
	}
	// The card still reports the raw free bytes: reserved is why the flag flipped.
	if got := data["disk_free"].(float64); got < float64(minGB)*float64(giB) {
		t.Errorf("disk_free should still look healthy on its own, got %v", got)
	}
}

// TestHandleDownloadEpisode_DiskFullReturns409: the cause is known and the user can act on it,
// so 409 not 500.
func TestHandleDownloadEpisode_DiskFullReturns409(t *testing.T) {
	dir := t.TempDir()
	_, free, err := files.DiskSpace(dir)
	if err != nil {
		t.Fatal(err)
	}
	fm := &mockFileManager{configs: &files.Config{
		AnilistUsernames:    []string{"testuser"},
		CompletedAnimePath:  dir,
		MinFreeDiskGB:       int(free>>30) + 1,
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   1,
	}}
	if fm.configs.MinFreeDiskGB < 1 {
		fm.configs.MinFreeDiskGB = 1
	}
	server, _ := episodeActionServer(t, fm)

	restore := mockAnimeInfo()
	defer restore()

	rec := httptest.NewRecorder()
	handleDownloadEpisode(server)(rec, episodeRequest(http.MethodPost, "/api/v1/animes/7/episodes/42/download", ""))

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var response SuccessResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Error == nil || response.Error.Code != "INSUFFICIENT_DISK_SPACE" {
		t.Errorf("want INSUFFICIENT_DISK_SPACE, got %+v", response.Error)
	}
}
