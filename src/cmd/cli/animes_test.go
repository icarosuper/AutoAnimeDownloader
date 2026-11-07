package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/api"
	"AutoAnimeDownloader/src/internal/daemon"
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

func TestAnimesCandidatesQueryAfterID(t *testing.T) {
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

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "candidates", "21", "--query", "foo"); err != nil {
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

func TestAnimesAddMagnetAfterID(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{
			Success: true,
			Data:    api.StandaloneAnimeAddResponse{Added: 1},
		})
	}))
	defer srv.Close()

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "add", "21", "--magnet", "magnet:?xt=urn:btih:abc"); err != nil {
		t.Fatalf("add magnet after id: %v", err)
	}
	var req api.StandaloneAnimeRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	if req.MediaID != 21 || req.Magnet != "magnet:?xt=urn:btih:abc" {
		t.Errorf("req = %+v", req)
	}
}

func TestAnimesDebugPrintsSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/animes/21/debug" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: daemon.DebugSummary{
			AnimeID: 21, AnimeName: "One Piece",
			Episodes: []daemon.EpisodeDebugResult{{Episode: 1100, WouldSearch: true, MagnetsFound: 3}},
		}})
	}))
	defer srv.Close()

	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "debug", "21")
	if err != nil {
		t.Fatalf("debug: %v", err)
	}
	if !strings.Contains(stdout, "One Piece") || !strings.Contains(stdout, "1100") {
		t.Errorf("summary missing from output:\n%s", stdout)
	}
}

// TestAnimesDeleteAndRedownloadTakeSeveralEpisodes: one episode keeps the single endpoint, several
// go through the batch one; a batch redownload with failures exits non-zero.
func TestAnimesDeleteAndRedownloadTakeSeveralEpisodes(t *testing.T) {
	var paths, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		paths = append(paths, r.Method+" "+r.URL.Path)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		var data any
		if strings.HasSuffix(r.URL.Path, "/batch-redownload") {
			data = map[string]any{"redownloaded": []int{3}, "failed": []map[string]any{{"episode": 4, "error": "no torrent found"}}}
		}
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: data})
	}))
	defer srv.Close()

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "delete", "21", "3", "--yes"); err != nil {
		t.Fatalf("single delete: %v", err)
	}
	if _, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "delete", "21", "3", "4", "--yes"); err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "redownload", "21", "3", "4", "--yes")
	if err == nil {
		t.Error("a batch redownload with failures must exit non-zero")
	}
	if !strings.Contains(stdout, "episode 4: no torrent found") {
		t.Errorf("failure missing from output:\n%s", stdout)
	}

	want := []string{
		"DELETE /api/v1/animes/21/episodes/3",
		"POST /api/v1/animes/21/episodes/batch-delete",
		"POST /api/v1/animes/21/episodes/batch-redownload",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n got: %q\nwant: %q", paths, want)
	}
	if bodies[1] != `{"episodes":[3,4]}` {
		t.Errorf("batch body = %q", bodies[1])
	}
}

// A magnet copied from `candidates` goes out with the row's name / nyaa_id / size_bytes, like the
// web picker sends them: without the name the daemon cannot read a pack's range and records no
// episode. A magnet that is not among the candidates still adds with the magnet alone.
func TestAnimesAddMagnetSendsTheMatchingCandidate(t *testing.T) {
	const hash = "1443c9af27a6e0c7d8d1d51f5ece2e04205269f9"
	picked := "magnet:?xt=urn:btih:" + hash + "&dn=Pack"
	other := "magnet:?xt=urn:btih:2443c9af27a6e0c7d8d1d51f5ece2e04205269f9"

	var body, candidatesQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/animes/21/torrent-candidates" {
			candidatesQuery = r.URL.Query().Get("q")
			_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: api.TorrentCandidatesResponse{
				Items: []api.TorrentCandidate{
					{Name: "Other", Magnet: other, NyaaID: 1, SizeBytes: 10},
					{Name: "[Group] Show [1080p]", Magnet: "magnet:?xt=urn:btih:" + strings.ToUpper(hash) + "&dn=Pack&tr=udp%3A%2F%2Ftracker", NyaaID: 1866380, SizeBytes: 1447034880},
				},
			}})
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: api.StandaloneAnimeAddResponse{Added: 1}})
	}))
	defer srv.Close()

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "add", "21", "--magnet", picked, "--query", "show s01"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if candidatesQuery != "show s01" {
		t.Errorf("candidates q = %q, want the --query", candidatesQuery)
	}
	var req api.StandaloneAnimeRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	if req.Name != "[Group] Show [1080p]" || req.NyaaID != 1866380 || req.SizeBytes != 1447034880 || req.Magnet != picked {
		t.Errorf("req = %+v, want the matching candidate's name/nyaa_id/size", req)
	}

	unknown := "magnet:?xt=urn:btih:3443c9af27a6e0c7d8d1d51f5ece2e04205269f9"
	if _, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "add", "21", "--magnet", unknown); err != nil {
		t.Fatalf("add unknown: %v", err)
	}
	req = api.StandaloneAnimeRequest{}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	if req.Magnet != unknown || req.Name != "" || req.NyaaID != 0 {
		t.Errorf("req = %+v, want the magnet alone", req)
	}
}
