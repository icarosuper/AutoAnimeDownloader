package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleDiagnosticsPassesTheParamsThrough(t *testing.T) {
	dir := t.TempDir()
	server := &Server{DataDir: dir}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics?titles=false&error=boom&context=GET+%2Fanimes%2F42%2Fepisodes+500&anime=42", nil)
	w := httptest.NewRecorder()
	handleDiagnostics(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			Bundle      string `json:"bundle"`
			Environment string `json:"environment"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	bundle := response.Data.Bundle
	for _, want := range []string{"Error: boom", "Request: GET /animes/<title-1>/episodes 500", "Anime media ID: <title-1>"} {
		if !strings.Contains(bundle, want) {
			t.Errorf("bundle misses %q:\n%s", want, bundle)
		}
	}
	if !strings.HasPrefix(response.Data.Environment, "Version: ") {
		t.Errorf("environment = %q", response.Data.Environment)
	}
}

func TestHandleDiagnosticsCrashIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "crash-pending.log"), []byte("panic: boom"), 0644)
	server := &Server{DataDir: dir}

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handleDiagnosticsCrash(server)(w, httptest.NewRequest(http.MethodDelete, "/api/v1/diagnostics/crash", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d: %s", i+1, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "crash-pending.log")); !os.IsNotExist(err) {
		t.Fatal("the pending crash should be archived")
	}
}

func TestHandleStatusReportsThePendingCrash(t *testing.T) {
	dir := t.TempDir()
	server := &Server{State: daemon.NewState(), FileManager: &mockFileManager{configs: &files.Config{}}, DataDir: dir}

	pendingCrashAt := func() any {
		w := httptest.NewRecorder()
		handleStatus(server)(w, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		var response struct {
			Data map[string]any `json:"data"`
		}
		json.NewDecoder(w.Body).Decode(&response)
		return response.Data["pending_crash_at"]
	}

	if got := pendingCrashAt(); got != nil {
		t.Fatalf("pending_crash_at = %v, want null", got)
	}
	os.WriteFile(filepath.Join(dir, "crash-pending.log"), []byte("panic: boom"), 0644)
	if got := pendingCrashAt(); got == nil {
		t.Fatal("pending_crash_at should be set")
	}
}
