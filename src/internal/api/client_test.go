package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
)

func TestClientMethodsHitExpectedPaths(t *testing.T) {
	type tc struct {
		name   string
		method string
		path   string
		query  map[string]string
		body   string
		data   string
		call   func(*Client) error
	}

	tests := []tc{
		{"GetLastCheck", http.MethodGet, "/api/v1/last-check", nil, "", "{}", func(c *Client) error {
			_, err := c.GetLastCheck()
			return err
		}},
		{"GetPriorityDefaults", http.MethodGet, "/api/v1/config/priorities/defaults", nil, "", "{}", func(c *Client) error {
			_, err := c.GetPriorityDefaults()
			return err
		}},
		{"SearchAniList", http.MethodGet, "/api/v1/anilist/search", map[string]string{"q": "frieren", "include_unreleased": "true"}, "", "[]", func(c *Client) error {
			_, err := c.SearchAniList("frieren", true)
			return err
		}},
		{"AddStandaloneAnime", http.MethodPost, "/api/v1/standalone-animes", nil, `{"media_id":21}`, "{}", func(c *Client) error {
			_, err := c.AddStandaloneAnime(StandaloneAnimeRequest{MediaID: 21})
			return err
		}},
		{"AddStandaloneAnimeWithMagnet", http.MethodPost, "/api/v1/standalone-animes", nil, `{"media_id":21,"magnet":"magnet:?xt=urn:btih:abc"}`, "{}", func(c *Client) error {
			_, err := c.AddStandaloneAnime(StandaloneAnimeRequest{MediaID: 21, Magnet: "magnet:?xt=urn:btih:abc"})
			return err
		}},
		{"GetTorrentCandidates", http.MethodGet, "/api/v1/animes/21/torrent-candidates", nil, "", `{"query":"","items":[]}`, func(c *Client) error {
			_, err := c.GetTorrentCandidates(21, "")
			return err
		}},
		{"GetTorrentCandidatesWithQuery", http.MethodGet, "/api/v1/animes/21/torrent-candidates", map[string]string{"q": "foo"}, "", `{"query":"foo","items":[]}`, func(c *Client) error {
			_, err := c.GetTorrentCandidates(21, "foo")
			return err
		}},
		{"RemoveStandaloneAnime", http.MethodDelete, "/api/v1/standalone-animes/21", map[string]string{"delete_episodes": "true"}, "", "null", func(c *Client) error {
			return c.RemoveStandaloneAnime(21, true)
		}},
		{"GetAnimeSettings", http.MethodGet, "/api/v1/animes/21/settings", nil, "", "{}", func(c *Client) error {
			_, err := c.GetAnimeSettings(21)
			return err
		}},
		{"UpdateAnimeSettings", http.MethodPut, "/api/v1/animes/21/settings", nil, `{"progress":8}`, "null", func(c *Client) error {
			return c.UpdateAnimeSettings(21, 8)
		}},
		{"GetAnimeEpisodes", http.MethodGet, "/api/v1/animes/21/episodes", nil, "", "{}", func(c *Client) error {
			_, err := c.GetAnimeEpisodes(21)
			return err
		}},
		{"DownloadEpisode", http.MethodPost, "/api/v1/animes/21/episodes/3/download", nil, "", "null", func(c *Client) error {
			return c.DownloadEpisode(21, 3)
		}},
		{"RedownloadEpisode", http.MethodPost, "/api/v1/animes/21/episodes/3/redownload", nil, "", "null", func(c *Client) error {
			return c.RedownloadEpisode(21, 3)
		}},
		{"DeleteEpisode", http.MethodDelete, "/api/v1/animes/21/episodes/3", nil, "", "null", func(c *Client) error {
			return c.DeleteEpisode(21, 3)
		}},
		{"ReleaseEpisode", http.MethodPost, "/api/v1/animes/21/episodes/3/release", nil, "", "null", func(c *Client) error {
			return c.ReleaseEpisode(21, 3)
		}},
		{"ReplaceAnime", http.MethodPost, "/api/v1/animes/21/replace", nil, `{"magnet":"magnet:?xt=urn:btih:abc"}`, "null", func(c *Client) error {
			return c.ReplaceAnime(21, "magnet:?xt=urn:btih:abc")
		}},
		{"ReplaceEpisode", http.MethodPost, "/api/v1/animes/21/episodes/3/replace", nil, `{"magnet":"magnet:?xt=urn:btih:abc"}`, "null", func(c *Client) error {
			return c.ReplaceEpisode(21, 3, "magnet:?xt=urn:btih:abc")
		}},
		{"GetTorrents", http.MethodGet, "/api/v1/torrents", nil, "", "[]", func(c *Client) error {
			_, err := c.GetTorrents()
			return err
		}},
		{"GetTorrentFiles", http.MethodGet, "/api/v1/torrents/deadbeef/files", nil, "", "[]", func(c *Client) error {
			_, err := c.GetTorrentFiles("deadbeef")
			return err
		}},
		{"PauseTorrent", http.MethodPost, "/api/v1/torrents/deadbeef/pause", nil, "", "null", func(c *Client) error {
			return c.PauseTorrent("deadbeef")
		}},
		{"ResumeTorrent", http.MethodPost, "/api/v1/torrents/deadbeef/resume", nil, "", "null", func(c *Client) error {
			return c.ResumeTorrent("deadbeef")
		}},
		{"AnnounceTorrent", http.MethodPost, "/api/v1/torrents/deadbeef/announce", nil, "", "null", func(c *Client) error {
			return c.AnnounceTorrent("deadbeef")
		}},
		{"PrioritizeTorrent", http.MethodPost, "/api/v1/torrents/deadbeef/prioritize", nil, "", "null", func(c *Client) error {
			return c.PrioritizeTorrent("deadbeef")
		}},
		{"PrioritizeTorrents", http.MethodPost, "/api/v1/torrents/prioritize", nil, `{"hashes":["aaa","bbb"]}`, "null", func(c *Client) error {
			return c.PrioritizeTorrents([]string{"aaa", "bbb"})
		}},
		{"DeleteTorrent", http.MethodDelete, "/api/v1/torrents/deadbeef", map[string]string{"keep_data": "true", "block": "true"}, "", "null", func(c *Client) error {
			return c.DeleteTorrent("deadbeef", true, true)
		}},
		{"TestWebhook", http.MethodPost, "/api/v1/notifications/webhooks/discord/test", nil, "", "null", func(c *Client) error {
			return c.TestWebhook("discord")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotQuery = r.URL.Query()
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"data":` + tt.data + `,"error":null}`))
			}))
			defer srv.Close()

			if err := tt.call(NewClient(srv.URL)); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotMethod != tt.method {
				t.Errorf("method = %s, want %s", gotMethod, tt.method)
			}
			if gotPath != tt.path {
				t.Errorf("path = %s, want %s", gotPath, tt.path)
			}
			if tt.query == nil && len(gotQuery) > 0 {
				t.Errorf("query = %v, want empty", gotQuery)
			}
			for key, want := range tt.query {
				if got := gotQuery.Get(key); got != want {
					t.Errorf("query[%s] = %q, want %q", key, got, want)
				}
			}
			if tt.query != nil && len(gotQuery) != len(tt.query) {
				t.Errorf("query = %v, want %v", gotQuery, tt.query)
			}
			if strings.TrimSpace(tt.body) == "" {
				if strings.TrimSpace(gotBody) != "" {
					t.Errorf("body = %q, want empty", gotBody)
				}
			} else if !jsonEqual(t, gotBody, tt.body) {
				t.Errorf("body = %s, want %s", gotBody, tt.body)
			}
		})
	}
}

func TestClientPropagatesErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(SuccessResponse{
			Success: false,
			Error:   &ErrorInfo{Code: "ALREADY_TRACKED", Message: "already in a list"},
		})
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).AddStandaloneAnime(StandaloneAnimeRequest{MediaID: 21})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v (%T), want APIError", err, err)
	}
	if apiErr.Code != "ALREADY_TRACKED" {
		t.Errorf("code = %q", apiErr.Code)
	}
	if apiErr.Message != "already in a list" {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestExistingClientMethodsStillParseEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"running","has_error":false,"version":"test"},"error":null}`))
		case "/api/v1/config":
			raw, _ := json.Marshal(SuccessResponse{Success: true, Data: files.Config{AnilistUsernames: []string{"a"}}})
			_, _ = w.Write(raw)
		case "/api/v1/animes":
			_, _ = w.Write([]byte(`{"success":true,"data":[],"error":null}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":null,"error":null}`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.GetStatus(); err != nil {
		t.Errorf("GetStatus: %v", err)
	}
	if _, err := c.GetConfig(); err != nil {
		t.Errorf("GetConfig: %v", err)
	}
	if _, err := c.GetAnimes(); err != nil {
		t.Errorf("GetAnimes: %v", err)
	}
	if err := c.TriggerCheck(); err != nil {
		t.Errorf("TriggerCheck: %v", err)
	}
	if err := c.StartLoop(); err != nil {
		t.Errorf("StartLoop: %v", err)
	}
	if err := c.StopLoop(); err != nil {
		t.Errorf("StopLoop: %v", err)
	}
}

func jsonEqual(t *testing.T, got, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(got), &a); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("want JSON: %v", err)
	}
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	return string(ga) == string(gb)
}
