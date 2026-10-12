package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInterpolate(t *testing.T) {
	result := interpolate("Hello {{anime_name}}, ep {{episode}}", map[string]string{
		"anime_name": "Frieren",
		"episode":    "5",
	})
	if result != "Hello Frieren, ep 5" {
		t.Fatalf("got %q", result)
	}
}

func TestInterpolateMissingVarEmpty(t *testing.T) {
	result := interpolate("{{missing}}", map[string]string{})
	if result != "" {
		t.Fatalf("missing var should be empty string, got %q", result)
	}
}

func TestFireWebhookInterpolatesURLAndBody(t *testing.T) {
	var capturedBody string
	var capturedHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		capturedHeader = r.Header.Get("Title")
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name:    "test",
		URL:     srv.URL,
		Method:  "POST",
		Headers: map[string]string{"Title": "{{title}}"},
		Body:    "{{anime_name}} EP {{episode}}",
	}

	vars := buildVars("Frieren", 5, NewEpisode, "")
	fireWebhook(preset, vars)

	if capturedBody != "Frieren EP 5" {
		t.Fatalf("body not interpolated: %q", capturedBody)
	}
	if capturedHeader == "" {
		t.Fatal("Title header not set")
	}
}

func TestBuildVarsDownloadFailedReason(t *testing.T) {
	vars := buildVars("Frieren", 5, DownloadFailed, ReasonNotFound)
	if vars["reason"] != ReasonNotFound {
		t.Fatalf("reason var = %q", vars["reason"])
	}
	if want := "Frieren EP 5 failed: " + ReasonNotFound; vars["message"] != want {
		t.Fatalf("message = %q, want %q", vars["message"], want)
	}
}

func TestFireTestWebhookNotFound(t *testing.T) {
	cfg := &files.Config{}
	err := FireTestWebhook(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing webhook")
	}
}

func TestFireTestWebhookFires(t *testing.T) {
	var fired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "my-hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test"},
			},
		},
	}
	if err := FireTestWebhook(cfg, "my-hook"); err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Fatal("webhook not fired")
	}
}

func TestNotifyNoOp_WhenNoWebhooks(t *testing.T) {
	Notify(&files.Config{}, NewEpisode, "Frieren", 5, "")
}

func TestNotifyNoOp_WhenNilConfig(t *testing.T) {
	Notify(nil, NewEpisode, "Frieren", 5, "")
}

func TestNotify_SkipsWebhookWhenEventNotSubscribed(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"download_failed"}},
			},
		},
	}

	Notify(cfg, NewEpisode, "Frieren", 5, "")

	select {
	case <-fired:
		t.Fatal("webhook should not have fired for unsubscribed event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNotify_FiresWebhookWhenEventSubscribed(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	Notify(cfg, NewEpisode, "Frieren", 5, "")

	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("webhook should have fired for subscribed event")
	}
}

// cfgWithWindow builds a one-webhook config subscribed to every event.
func cfgWithWindow(url string, window int, headers map[string]string, body string) *files.Config {
	if headers == nil {
		headers = map[string]string{}
	}
	return &files.Config{
		Notifications: files.NotificationsConfig{
			BatchWindowSeconds: window,
			Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: url, Method: "POST", Headers: headers, Body: body,
				Events: []string{"new_episode", "download_failed", "download_completed"},
			}},
		},
	}
}

// collector returns a server that stacks the bodies it receives.
func collector(t *testing.T) (*httptest.Server, chan string) {
	t.Helper()
	bodies := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	t.Cleanup(srv.Close)
	return srv, bodies
}

// drain waits for `want` bodies or the timeout, and returns what arrived.
func drain(bodies chan string, want int, timeout time.Duration) []string {
	var got []string
	deadline := time.After(timeout)
	for len(got) < want {
		select {
		case b := <-bodies:
			got = append(got, b)
		case <-deadline:
			return got
		}
	}
	// Give it a beat to catch an extra request that should not have been sent.
	select {
	case b := <-bodies:
		got = append(got, b)
	case <-time.After(100 * time.Millisecond):
	}
	return got
}

// TestNotify_BatchesEventsWithinWindow: the reason batching exists. A library backfill fires
// one download_completed per torrent (77 in a real case) and ntfy.sh answers 429 past a certain
// volume — the window has to turn that into a single request.
func TestNotify_BatchesEventsWithinWindow(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, nil, "{{count}}|{{title}}|{{message}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "")
	Notify(cfg, DownloadCompleted, "Dandadan", 3, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("expected 1 request for the 3 events, got %d: %v", len(got), got)
	}
	want := "3|3 downloads completed|" +
		"Re:ZERO EP 12 downloaded successfully\n" +
		"Frieren EP 5 downloaded successfully\n" +
		"Dandadan EP 3 downloaded successfully"
	if got[0] != want {
		t.Errorf("batched body wrong:\n got: %q\nwant: %q", got[0], want)
	}
}

// TestNotify_SeparateBatchPerEvent: a preset subscribed only to download_failed must not
// receive success text. The queue is per event precisely so they don't mix.
func TestNotify_SeparateBatchPerEvent(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, nil, "{{title}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "")
	Notify(cfg, DownloadFailed, "Frieren", 5, ReasonNotFound)

	got := drain(bodies, 2, 3*time.Second)
	if len(got) != 2 {
		t.Fatalf("expected 2 requests (one per event), got %d: %v", len(got), got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen["Download completed"] || !seen["Download failed"] {
		t.Errorf("expected one title per event, got %v", got)
	}
}

// TestNotify_SingleItemBatchMatchesUnbatched: turning batching on must not change the look of
// a notification that arrived alone in the window — that is what keeps existing presets
// working the same.
func TestNotify_SingleItemBatchMatchesUnbatched(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	const body = "{{title}}|{{message}}|{{anime_name}}|{{episode}}|{{reason}}"

	Notify(cfgWithWindow(srv.URL, 0, nil, body), DownloadFailed, "Frieren", 5, ReasonNotFound)
	unbatched := drain(bodies, 1, time.Second)

	Notify(cfgWithWindow(srv.URL, 1, nil, body), DownloadFailed, "Frieren", 5, ReasonNotFound)
	batched := drain(bodies, 1, 3*time.Second)

	if len(unbatched) != 1 || len(batched) != 1 {
		t.Fatalf("expected 1 request of each, got %d and %d", len(unbatched), len(batched))
	}
	if unbatched[0] != batched[0] {
		t.Errorf("a 1-item window changed the body:\n without batch: %q\n with batch: %q", unbatched[0], batched[0])
	}
}

// TestNotify_WindowZeroFiresImmediately: 0 turns batching off, without waiting for any timer.
func TestNotify_WindowZeroFiresImmediately(t *testing.T) {
	srv, bodies := collector(t)
	cfg := cfgWithWindow(srv.URL, 0, nil, "{{title}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "")

	got := drain(bodies, 2, time.Second)
	if len(got) != 2 {
		t.Fatalf("with window 0 expected 2 immediate requests, got %d", len(got))
	}
}

// TestFlush_SendsPendingBatch: without this a whole window of notifications would die with
// the process on shutdown.
func TestFlush_SendsPendingBatch(t *testing.T) {
	srv, bodies := collector(t)
	// Long window: only Flush can make this leave during the test.
	cfg := cfgWithWindow(srv.URL, 3600, nil, "{{count}}")

	Notify(cfg, NewEpisode, "Re:ZERO", 12, "")
	Notify(cfg, NewEpisode, "Frieren", 5, "")

	if got := drain(bodies, 1, 200*time.Millisecond); len(got) != 0 {
		t.Fatalf("nothing should leave before Flush, got %v", got)
	}

	Flush() // synchronous: if it returns before the request, the check below catches it
	select {
	case b := <-bodies:
		if b != "2" {
			t.Errorf("Flush should send the 2 items together, got %q", b)
		}
	default:
		t.Error("Flush returned without the request arriving")
	}
}

// TestFireWebhook_EscapesJSONBody: a name with quotes and the batching \n would break the body
// of presets that embed {{message}} in JSON (Discord, Slack, Telegram...), and the service
// would answer 400 with the notification vanishing unexplained.
func TestFireWebhook_EscapesJSONBody(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1,
		map[string]string{"Content-Type": "application/json"},
		`{"content":"{{message}}"}`)

	Notify(cfg, DownloadCompleted, `Re:"ZERO"`, 12, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got))
	}

	var payload struct{ Content string }
	if err := json.Unmarshal([]byte(got[0]), &payload); err != nil {
		t.Fatalf("body is not valid JSON (%v): %s", err, got[0])
	}
	if !strings.Contains(payload.Content, `Re:"ZERO" EP 12`) {
		t.Errorf("quotes in the name lost in the escape: %q", payload.Content)
	}
	if !strings.Contains(payload.Content, "\n") {
		t.Errorf("batching newline lost: %q", payload.Content)
	}
}

// TestFireWebhook_DoesNotEscapePlainTextBody: ntfy's preset sends raw `{{message}}`, so the
// \n has to arrive as a real \n — escaping there would send the literal backslash-n.
func TestFireWebhook_DoesNotEscapePlainTextBody(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, map[string]string{"Title": "{{title}}"}, "{{message}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got))
	}
	if strings.Contains(got[0], `\n`) {
		t.Errorf("plain-text body must not be escaped: %q", got[0])
	}
	if !strings.Contains(got[0], "\n") {
		t.Errorf("expected the two lines separated by \\n: %q", got[0])
	}
}

// TestFireWebhook_StripsNewlinesFromHeaders: a header with \n makes net/http reject the whole
// request, and ntfy uses `Title: {{title}}`.
func TestFireWebhook_StripsNewlinesFromHeaders(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Title")
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{"Title": "{{message}}"},
		Body:    "x", Events: []string{"download_completed"},
	}
	fireWebhook(preset, buildBatchVars(DownloadCompleted, []item{
		{animeName: "Re:ZERO", episode: 12},
		{animeName: "Frieren", episode: 5},
	}))

	select {
	case title := <-received:
		if title == "" {
			t.Error("empty header: the request was rejected instead of sanitized")
		}
		if strings.ContainsAny(title, "\n\r") {
			t.Errorf("header still has a newline: %q", title)
		}
	case <-time.After(time.Second):
		t.Fatal("request never arrived — net/http rejected the header")
	}
}
