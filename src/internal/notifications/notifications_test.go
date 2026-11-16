package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

	vars := buildBatchVars(NewEpisode, []Episode{{AnimeName: "Frieren", Number: 5}})
	fireWebhook(preset, vars)

	if capturedBody != "Frieren EP 5" {
		t.Fatalf("body not interpolated: %q", capturedBody)
	}
	if capturedHeader == "" {
		t.Fatal("Title header not set")
	}
}

func TestBuildVarsDownloadFailedReason(t *testing.T) {
	vars := buildBatchVars(DownloadFailed, []Episode{{AnimeName: "Frieren", Number: 5, Reason: reasonStalled}})
	if vars["reason"] != reasonStalled {
		t.Fatalf("reason var = %q", vars["reason"])
	}
	if want := "Frieren EP 5 failed: " + reasonStalled; vars["message"] != want {
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
	Notify(&files.Config{}, NewEpisode, Episode{AnimeName: "Frieren", Number: 5})
}

func TestNotifyNoOp_WhenNilConfig(t *testing.T) {
	Notify(nil, NewEpisode, Episode{AnimeName: "Frieren", Number: 5})
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

	Notify(cfg, NewEpisode, Episode{AnimeName: "Frieren", Number: 5})

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

	Notify(cfg, NewEpisode, Episode{AnimeName: "Frieren", Number: 5})

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

	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Frieren", Number: 5})
	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Dandadan", Number: 3})

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("expected 1 request for the 3 events, got %d: %v", len(got), got)
	}
	want := "3|3 episodes downloaded|" +
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

	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, DownloadFailed, Episode{AnimeName: "Frieren", Number: 5, Reason: reasonStalled})

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

	Notify(cfgWithWindow(srv.URL, 0, nil, body), DownloadFailed, Episode{AnimeName: "Frieren", Number: 5, Reason: reasonStalled})
	unbatched := drain(bodies, 1, time.Second)

	Notify(cfgWithWindow(srv.URL, 1, nil, body), DownloadFailed, Episode{AnimeName: "Frieren", Number: 5, Reason: reasonStalled})
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

	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Frieren", Number: 5})

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

	Notify(cfg, NewEpisode, Episode{AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, NewEpisode, Episode{AnimeName: "Frieren", Number: 5})

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

	Notify(cfg, DownloadCompleted, Episode{AnimeName: `Re:"ZERO"`, Number: 12})
	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Frieren", Number: 5})

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

	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Frieren", Number: 5})

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
	fireWebhook(preset, buildBatchVars(DownloadCompleted, []Episode{
		{AnimeName: "Re:ZERO", Number: 12},
		{AnimeName: "Frieren", Number: 5},
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

// TestBatchLines_CollapsesConsecutiveEpisodes: a pack completing is one line per run, not one per
// episode. Organize finishes in completion order, so the episodes are sorted before collapsing; a
// gap opens a new run, another anime or another failure reason never merges into the same line.
func TestBatchLines_CollapsesConsecutiveEpisodes(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		items []Episode
		want  []string
	}{
		{
			name:  "out-of-order run with a gap and a duplicate",
			event: DownloadCompleted,
			items: []Episode{
				{AnimeID: 1, AnimeName: "Frieren", Number: 3},
				{AnimeID: 1, AnimeName: "Frieren", Number: 1},
				{AnimeID: 1, AnimeName: "Frieren", Number: 2},
				{AnimeID: 1, AnimeName: "Frieren", Number: 2},
				{AnimeID: 1, AnimeName: "Frieren", Number: 7},
			},
			want: []string{
				"Frieren EPs 1-3 downloaded successfully",
				"Frieren EP 7 downloaded successfully",
			},
		},
		{
			name:  "animes keep first-appearance order",
			event: NewEpisode,
			items: []Episode{
				{AnimeID: 2, AnimeName: "Dandadan", Number: 4},
				{AnimeID: 1, AnimeName: "Frieren", Number: 5},
				{AnimeID: 2, AnimeName: "Dandadan", Number: 5},
			},
			want: []string{
				"Dandadan EPs 4-5 detected, starting download",
				"Frieren EP 5 detected, starting download",
			},
		},
		{
			name:  "different reasons stay apart",
			event: DownloadFailed,
			items: []Episode{
				{AnimeID: 1, AnimeName: "Frieren", Number: 1, Reason: reasonStalled},
				{AnimeID: 1, AnimeName: "Frieren", Number: 2, Reason: ReasonDownloadRejected},
				{AnimeID: 1, AnimeName: "Frieren", Number: 3, Reason: ReasonDownloadRejected},
			},
			want: []string{
				"Frieren EP 1 failed: " + reasonStalled,
				"Frieren EPs 2-3 failed: " + ReasonDownloadRejected,
			},
		},
		{
			name:  "without an id the name keeps animes apart",
			event: EpisodeDeleted,
			items: []Episode{
				{AnimeName: "Frieren", Number: 1},
				{AnimeName: "Dandadan", Number: 2},
				{AnimeName: "Frieren", Number: 2},
			},
			want: []string{
				"Frieren EPs 1-2 were removed from the library",
				"Dandadan EP 2 was removed from the library",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batchLines(tt.event, tt.items)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("lines:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// TestBuildBatchVars_AnimeIDAndTorrentHash: filled for a single item, blank for N items (no single
// value), and blank when the caller has no id rather than a misleading "0".
func TestBuildBatchVars_AnimeIDAndTorrentHash(t *testing.T) {
	one := buildBatchVars(DownloadCompleted, []Episode{{AnimeID: 154587, AnimeName: "Frieren", Number: 5, TorrentHash: "abc"}})
	if one["anime_id"] != "154587" || one["torrent_hash"] != "abc" {
		t.Errorf("single item: anime_id=%q torrent_hash=%q", one["anime_id"], one["torrent_hash"])
	}

	many := buildBatchVars(DownloadCompleted, []Episode{
		{AnimeID: 154587, AnimeName: "Frieren", Number: 5, TorrentHash: "abc"},
		{AnimeID: 154587, AnimeName: "Frieren", Number: 6, TorrentHash: "def"},
	})
	if many["anime_id"] != "" || many["torrent_hash"] != "" {
		t.Errorf("N items must leave them blank: anime_id=%q torrent_hash=%q", many["anime_id"], many["torrent_hash"])
	}

	noID := buildBatchVars(DownloadFailed, []Episode{{AnimeName: "Frieren", Number: 5}})
	if noID["anime_id"] != "" {
		t.Errorf("unknown id must be blank, got %q", noID["anime_id"])
	}
}

// TestNotifyAll_PackIsOneWebhookNamingTheAnime: with batching off, a pack completing is still ONE
// request (one push, one Jellyfin scan). Its text names only the anime and {{episode}} is blank —
// the variable is a single number or nothing, never a range.
func TestNotifyAll_PackIsOneWebhookNamingTheAnime(t *testing.T) {
	srv, bodies := collector(t)
	cfg := cfgWithWindow(srv.URL, 0, nil, "{{title}}|{{anime_name}}|{{episode}}|{{count}}|{{message}}")

	Notify(cfg, DownloadCompleted, Episode{AnimeName: "Frieren", Number: 1, PackSize: 4})

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("expected 1 request for the pack, got %d: %v", len(got), got)
	}
	if want := "Download completed|Frieren||4|Frieren downloaded successfully"; got[0] != want {
		t.Errorf("body = %q, want %q", got[0], want)
	}
}

// TestNotifyAll_WindowZeroFiresOnePerItem: with batching off, items handed together are still one
// webhook each, so the single-value variables stay filled.
func TestNotifyAll_WindowZeroFiresOnePerItem(t *testing.T) {
	srv, bodies := collector(t)
	cfg := cfgWithWindow(srv.URL, 0, nil, "{{anime_id}}|{{torrent_hash}}")

	NotifyAll(cfg, DownloadCompleted, []Episode{
		{AnimeID: 1, AnimeName: "Frieren", Number: 3, TorrentHash: "a"},
		{AnimeID: 2, AnimeName: "Dandadan", Number: 1, PackSize: 12, TorrentHash: "b"},
	})

	got := drain(bodies, 2, 3*time.Second)
	slices.Sort(got)
	if want := []string{"1|a", "2|b"}; !slices.Equal(got, want) {
		t.Errorf("bodies = %v, want %v", got, want)
	}
}

// TestBatchLines_PackNamesOnlyTheAnime: in a window, a pack is one line naming the anime, next to
// the runs of the anime's single-episode torrents.
func TestBatchLines_PackNamesOnlyTheAnime(t *testing.T) {
	got := batchLines(DownloadCompleted, []Episode{
		{AnimeID: 1, AnimeName: "Frieren", Number: 1, PackSize: 12},
		{AnimeID: 1, AnimeName: "Frieren", Number: 13},
		{AnimeID: 2, AnimeName: "Dandadan", Number: 1, PackSize: 12},
	})
	want := []string{
		"Frieren downloaded successfully",
		"Frieren EP 13 downloaded successfully",
		"Dandadan downloaded successfully",
	}
	if !slices.Equal(got, want) {
		t.Errorf("lines:\n got: %q\nwant: %q", got, want)
	}
	if title := batchTitle(DownloadCompleted, []Episode{{AnimeName: "Frieren", Number: 1, PackSize: 12}, {AnimeName: "Frieren", Number: 13}}); title != "13 episodes downloaded" {
		t.Errorf("title counts the pack's episodes, got %q", title)
	}
}

// TestFireBatch_SourceFilter: a webhook with Sources only gets the items of those sources, and its
// template is built from that share (a window of 4 where 1 is alice's reads as that 1, plus the
// unknown one). A webhook without Sources still gets everything. An anime the resolver can't place
// (Re:ZERO, id 3) reaches every filtered webhook — fail-open — while one it places elsewhere (id 4,
// bob's only) reaches only bob's.
func TestFireBatch_SourceFilter(t *testing.T) {
	srvAll, all := collector(t)
	srvAlice, alice := collector(t)
	srvLoose, loose := collector(t)
	t.Cleanup(Flush)

	SetSourceResolver(func(_ *files.Config, ids []int) map[int][]string {
		return map[int][]string{1: {"anilist:alice", "anilist:bob"}, 2: {"standalone"}, 4: {"anilist:bob"}}
	})
	t.Cleanup(func() { SetSourceResolver(nil) })

	hook := func(name, url string, sources ...string) files.WebhookPreset {
		return files.WebhookPreset{Name: name, URL: url, Method: "POST", Body: "{{count}}|{{message}}",
			Events: []string{"download_completed"}, AnimeSources: sources}
	}
	cfg := &files.Config{Notifications: files.NotificationsConfig{BatchWindowSeconds: 1, Webhooks: []files.WebhookPreset{
		hook("all", srvAll.URL),
		hook("alice", srvAlice.URL, "anilist:alice"),
		hook("loose", srvLoose.URL, "standalone", "mal:nobody"),
	}}}

	Notify(cfg, DownloadCompleted, Episode{AnimeID: 1, AnimeName: "Frieren", Number: 5})
	Notify(cfg, DownloadCompleted, Episode{AnimeID: 2, AnimeName: "Dandadan", Number: 3})
	Notify(cfg, DownloadCompleted, Episode{AnimeID: 3, AnimeName: "Re:ZERO", Number: 12})
	Notify(cfg, DownloadCompleted, Episode{AnimeID: 4, AnimeName: "Mushishi", Number: 2})

	if got := drain(all, 1, 3*time.Second); len(got) != 1 || !strings.HasPrefix(got[0], "4|") {
		t.Errorf("the unfiltered webhook gets all 4, got %v", got)
	}
	if got := drain(alice, 1, 3*time.Second); len(got) != 1 || got[0] != "2|Frieren EP 5 downloaded successfully\nRe:ZERO EP 12 downloaded successfully" {
		t.Errorf("alice's webhook gets Frieren and the unknown Re:ZERO, got %v", got)
	}
	if got := drain(loose, 1, 3*time.Second); len(got) != 1 || got[0] != "2|Dandadan EP 3 downloaded successfully\nRe:ZERO EP 12 downloaded successfully" {
		t.Errorf("the standalone webhook gets Dandadan and the unknown Re:ZERO, got %v", got)
	}
}

// TestFireBatch_FilteredWebhookWithoutResolverFailsOpen: with no resolver every anime is unknown,
// and unknown reaches the filtered webhook too — a spare push beats a lost one.
func TestFireBatch_FilteredWebhookWithoutResolverFailsOpen(t *testing.T) {
	srvAll, all := collector(t)
	srvAlice, alice := collector(t)
	SetSourceResolver(nil)

	cfg := &files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{
		{Name: "all", URL: srvAll.URL, Method: "POST", Body: "{{title}}", Events: []string{"new_episode"}},
		{Name: "alice", URL: srvAlice.URL, Method: "POST", Body: "{{title}}", Events: []string{"new_episode"}, AnimeSources: []string{"anilist:alice"}},
	}}}
	Notify(cfg, NewEpisode, Episode{AnimeID: 1, AnimeName: "Frieren", Number: 5})

	if got := drain(all, 1, time.Second); len(got) != 1 {
		t.Errorf("the unfiltered webhook must fire, got %v", got)
	}
	if got := drain(alice, 1, time.Second); len(got) != 1 {
		t.Errorf("the filtered webhook must fire for an unknown anime, got %v", got)
	}
}

// A failed fire's error (logged, and shown by the Test button) must not carry the webhook URL's
// path: Discord and Telegram put the token there.
func TestFireWebhook_ErrorDoesNotLeakURLPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close() // connection refused from here on

	for _, raw := range []string{addr + "/api/webhooks/123/supersecrettoken", "http://[::1/botsupersecrettoken"} {
		err := fireWebhook(files.WebhookPreset{Name: "x", URL: raw, Method: "POST"}, nil)
		if err == nil {
			t.Fatalf("%s: expected an error", raw)
		}
		if strings.Contains(err.Error(), "supersecrettoken") {
			t.Errorf("error leaked the URL path: %v", err)
		}
	}
}

// An alert item's Text is its line, verbatim — no episode-run grouping — and the title is the
// event's own, never "N episodes ...".
func TestAlertItems_TextIsTheLineAndTitleIsTheEvent(t *testing.T) {
	items := []Episode{{Text: "/media/anime is gone or was replaced. 3 episodes will be downloaded again."}}
	vars := buildBatchVars(LibraryMissing, items)
	if vars["title"] != "Library folder is gone" {
		t.Errorf("title = %q", vars["title"])
	}
	if vars["message"] != items[0].Text {
		t.Errorf("message = %q, want the Text verbatim", vars["message"])
	}
	if vars["anime_id"] != "" || vars["episode"] != "" {
		t.Errorf("an alert has no anime/episode, got anime_id=%q episode=%q", vars["anime_id"], vars["episode"])
	}

	two := append(items, Episode{Text: "second line"})
	if got := buildBatchVars(LibraryMissing, two); got["message"] != items[0].Text+"\nsecond line" || got["title"] != "Library folder is gone" {
		t.Errorf("two alert items: title %q, message %q", got["title"], got["message"])
	}
}

// System events carry AnimeID 0, and a webhook filtered by anime_sources still gets them: a crash
// or a vanished library is not "from" any account, and filtering by account must not silence it.
func TestFireBatch_SourceFilterLetsSystemEventsThrough(t *testing.T) {
	srvAlice, alice := collector(t)
	SetSourceResolver(func(_ *files.Config, ids []int) map[int][]string { return map[int][]string{} })
	t.Cleanup(func() { SetSourceResolver(nil) })

	cfg := &files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{
		{Name: "alice", URL: srvAlice.URL, Method: "POST", Body: "{{message}}", Events: []string{"library_missing"}, AnimeSources: []string{"anilist:alice"}},
	}}}
	Notify(cfg, LibraryMissing, Episode{Text: "gone"})

	if got := drain(alice, 1, time.Second); len(got) != 1 || got[0] != "gone" {
		t.Errorf("a filtered webhook must get the system event, got %v", got)
	}
}

// The leftover_files title is the total of the lines the webhook gets, not of the whole digest: a
// webhook filtered to alice must not read bob's gigabytes in its title, whether bob's anime sorts
// first (and used to carry the title away) or not.
func TestFireBatch_LeftoverTitleIsTheFilteredTotal(t *testing.T) {
	srvAll, all := collector(t)
	srvAlice, alice := collector(t)
	SetSourceResolver(func(_ *files.Config, ids []int) map[int][]string {
		return map[int][]string{1: {"anilist:bob"}, 2: {"anilist:alice"}}
	})
	t.Cleanup(func() { SetSourceResolver(nil) })

	hook := func(name, url string, sources ...string) files.WebhookPreset {
		return files.WebhookPreset{Name: name, URL: url, Method: "POST", Body: "{{title}}|{{message}}",
			Events: []string{"leftover_files"}, AnimeSources: sources}
	}
	cfg := &files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{
		hook("all", srvAll.URL),
		hook("alice", srvAlice.URL, "anilist:alice"),
	}}}
	NotifyAll(cfg, LeftoverFiles, []Episode{
		{AnimeID: 1, AnimeName: "Dandadan", Text: "Dandadan line", Bytes: 3 << 30},
		{AnimeID: 2, AnimeName: "Frieren", Text: "Frieren line", Bytes: 1 << 29},
	})

	if got := drain(all, 1, time.Second); len(got) != 1 || got[0] != "3.5 GB of watched anime still on disk|Dandadan line\nFrieren line" {
		t.Errorf("unfiltered webhook, got %v", got)
	}
	if got := drain(alice, 1, time.Second); len(got) != 1 || got[0] != "0.5 GB of watched anime still on disk|Frieren line" {
		t.Errorf("alice's webhook counts only her line, got %v", got)
	}
}

// reasonStalled is the text HandleTorrentFailure passes for a stalled torrent — any free-text
// reason goes through the templates the same way.
const reasonStalled = "stalled: no peers for 1h0m"
