package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"AutoAnimeDownloader/src/internal/api"
)

func TestAnimesSearchShortTermReturnsEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/anilist/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: []api.AniListSearchResult{}})
	}))
	defer srv.Close()

	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "search", "ab")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	got := decodeJSON(t, stdout)
	list, ok := got.([]any)
	if !ok {
		t.Fatalf("got %#v, want array", got)
	}
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

func TestAnimesCandidatesHitsTorrentCandidates(t *testing.T) {
	var method, path string
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{
			Success: true,
			Data:    api.TorrentCandidatesResponse{Query: "One Piece", Items: []api.TorrentCandidate{}},
		})
	}))
	defer srv.Close()

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "candidates", "21"); err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if method != http.MethodGet || path != "/api/v1/animes/21/torrent-candidates" {
		t.Errorf("got %s %s, want GET /api/v1/animes/21/torrent-candidates", method, path)
	}
	if query != "" {
		t.Errorf("query = %q, want empty", query)
	}
}

func TestAnimesCandidatesWithQuery(t *testing.T) {
	var path, rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{
			Success: true,
			Data:    api.TorrentCandidatesResponse{Query: "foo", Items: []api.TorrentCandidate{}},
		})
	}))
	defer srv.Close()

	// Flags before the id: urfave/cli v2 (stdlib flag) stops parsing at the first positional.
	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "candidates", "--query", "foo", "21"); err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if path != "/api/v1/animes/21/torrent-candidates" {
		t.Errorf("path = %s", path)
	}
	if rawQuery != "q=foo" {
		t.Errorf("query = %q, want q=foo", rawQuery)
	}
}

func TestAnimesAddMagnet(t *testing.T) {
	var method, path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{
			Success: true,
			Data:    api.StandaloneAnimeAddResponse{Added: 1},
		})
	}))
	defer srv.Close()

	// Flags before the id: urfave/cli v2 (stdlib flag) stops parsing at the first positional.
	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "add", "--magnet", "magnet:?xt=urn:btih:abc", "21"); err != nil {
		t.Fatalf("add magnet: %v", err)
	}
	if method != http.MethodPost || path != "/api/v1/standalone-animes" {
		t.Errorf("got %s %s, want POST /api/v1/standalone-animes", method, path)
	}
	var req api.StandaloneAnimeRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	if req.MediaID != 21 || req.Magnet != "magnet:?xt=urn:btih:abc" {
		t.Errorf("req = %+v", req)
	}
}
