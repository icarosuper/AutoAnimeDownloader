package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"AutoAnimeDownloader/src/internal/api"
)

func TestTorrentsPrioritizeRoutesByHashCount(t *testing.T) {
	var method, path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: nil})
	}))
	defer srv.Close()

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "torrents", "prioritize", "abc"); err != nil {
		t.Fatalf("one hash: %v", err)
	}
	if method != http.MethodPost || path != "/api/v1/torrents/abc/prioritize" {
		t.Errorf("one hash: %s %s", method, path)
	}

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "torrents", "prioritize", "aaa", "bbb"); err != nil {
		t.Fatalf("two hashes: %v", err)
	}
	if method != http.MethodPost || path != "/api/v1/torrents/prioritize" {
		t.Errorf("two hashes: %s %s", method, path)
	}
	var req api.PrioritizeRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	if len(req.Hashes) != 2 || req.Hashes[0] != "aaa" || req.Hashes[1] != "bbb" {
		t.Errorf("hashes = %v", req.Hashes)
	}
}
