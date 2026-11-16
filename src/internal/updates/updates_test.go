package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.11.0", "2.10.3", true},
		{"v2.10.10", "2.10.9", true},
		{"2.10.0", "2.10.0", false},
		{"2.9.9", "2.10.0", false},
		{"3.0.0", "2.99.99", true},
		{"2.10.0", "2.10.0-rc1", true},
		{"2.10.0-rc1", "2.10.0", false},
		{"garbage", "2.10.0", false},
		{"2.11.0", "dev", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestFetchLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != releasePath {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"tag_name":"v2.11.0","html_url":"https://github.com/icarosuper/AutoAnimeDownloader/releases/tag/v2.11.0","draft":false}`))
	}))
	defer srv.Close()

	version, url, err := fetchLatest(context.Background(), srv.URL)
	if err != nil || version != "2.11.0" || url == "" {
		t.Errorf("got %q %q %v", version, url, err)
	}
}

func TestFetchLatest_ErrorsDoNotPanic(t *testing.T) {
	for _, body := range []string{`{"tag_name":"nightly"}`, `not json`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		if _, _, err := fetchLatest(context.Background(), srv.URL); err == nil {
			t.Errorf("%s: expected an error", body)
		}
		srv.Close()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	if _, _, err := fetchLatest(context.Background(), srv.URL); err == nil {
		t.Error("a 403 (rate limit) must be an error")
	}
}

func TestStart_DevBuildNeverStarts(t *testing.T) {
	Start(context.Background(), "dev", func() bool { t.Error("enabled asked on a dev build"); return true }, func(string, string) {})
}

func TestStart_ReportsNewerAndKeepsLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v2.11.0","html_url":"https://example.invalid/v2.11.0"}`))
	}))
	defer srv.Close()
	t.Setenv("GITHUB_API_URL", srv.URL)
	firstCheckDelay, checkEvery = time.Millisecond, time.Hour
	t.Cleanup(func() { firstCheckDelay, checkEvery = 2*time.Minute, 24*time.Hour; setLatest("", "") })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan string, 1)
	Start(ctx, "2.10.0", func() bool { return true }, func(v, _ string) { got <- v })

	select {
	case v := <-got:
		if v != "2.11.0" {
			t.Errorf("onNewer(%q)", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onNewer never called")
	}
	if v, u := Latest(); v != "2.11.0" || u != "https://example.invalid/v2.11.0" {
		t.Errorf("Latest() = %q %q", v, u)
	}
}
