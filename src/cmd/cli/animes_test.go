package main

import (
	"encoding/json"
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
