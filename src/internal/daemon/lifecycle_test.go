package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"

	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lifecycleFM is a FileManagerInterface stub for the session-lifecycle / config-gate tests.
// It records whether the pass got past the gate (LoadSavedEpisodes is the first thing the
// pass does after the gate).
type lifecycleFM struct {
	mockFileManagerForEpisodes

	configs *files.Config
	saved   []files.EpisodeStruct

	mu               sync.Mutex
	loadedSaved      bool
	loadedSavedCount int
}

func (m *lifecycleFM) LoadConfigs() (*files.Config, error) { return m.configs, nil }

func (m *lifecycleFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	m.mu.Lock()
	m.loadedSaved = true
	m.loadedSavedCount++
	m.mu.Unlock()
	return m.saved, nil
}

func (m *lifecycleFM) passRan() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadedSaved
}

// webhookSpy is an httptest server that captures the bodies of the webhooks fired at it.
type webhookSpy struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
	got    chan struct{}
}

func newWebhookSpy(t *testing.T) *webhookSpy {
	t.Helper()
	spy := &webhookSpy{got: make(chan struct{}, 8)}
	spy.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		spy.mu.Lock()
		spy.bodies = append(spy.bodies, string(body))
		spy.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		select {
		case spy.got <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(spy.server.Close)
	return spy
}

// waitForCall waits for one webhook delivery (Notify fires in a goroutine).
func (s *webhookSpy) waitForCall(t *testing.T) string {
	t.Helper()
	select {
	case <-s.got:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the webhook to be fired")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[len(s.bodies)-1]
}

func (s *webhookSpy) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

// configWithFailedWebhook returns a config with a single webhook subscribed to
// download_failed, echoing the interpolated variables into the request body.
func configWithFailedWebhook(url string) *files.Config {
	return &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{{
				Name:   "spy",
				URL:    url,
				Method: http.MethodPost,
				Body:   `{"anime":"{{anime_name}}","episode":"{{episode}}","reason":"{{reason}}"}`,
				Events: []string{"download_failed"},
			}},
		},
	}
}

// P3.1 — a torrent stopped with an error must fire download_failed AND leave the session,
// so the next verification pass sees the episode as missing and re-adds the magnet.
func TestHandleTorrentFailure_NotifiesAndRemovesFromSession(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"

	spy := newWebhookSpy(t)
	fm := &lifecycleFM{
		configs: configWithFailedWebhook(spy.server.URL),
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 7},
		},
	}

	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Wire the production callback exactly as cmd/daemon/main.go does.
	backend.SetCallbacks(nil, func(h string, err error) {
		HandleTorrentFailure(h, err, backend, fm)
	})

	backend.FailTorrent(hash, errors.New("no peers left"))

	body := spy.waitForCall(t)
	if !strings.Contains(body, `"anime":"My Anime"`) {
		t.Errorf("webhook body should carry the anime name from the saved record, got %s", body)
	}
	if !strings.Contains(body, `"episode":"7"`) {
		t.Errorf("webhook body should carry the episode number from the saved record, got %s", body)
	}
	if !strings.Contains(body, "no peers left") {
		t.Errorf("webhook body should carry the failure reason, got %s", body)
	}

	if _, ok := backend.Get(hash); ok {
		t.Error("failed torrent must be removed from the session so the loop re-adds it next pass")
	}
	// episodeInTorrents is what the loop uses to decide "already downloaded".
	if episodeInTorrents(hash, buildTorrentsHashSet(backend.List())) {
		t.Error("episodeInTorrents must be false after a failure, otherwise the episode is never retried")
	}
}

// A failure for a hash with no saved episode still fires the webhook (with empty name) and
// still drops the torrent — the orphan must not linger in the session either.
func TestHandleTorrentFailure_NoSavedEpisode(t *testing.T) {
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	spy := newWebhookSpy(t)
	fm := &lifecycleFM{configs: configWithFailedWebhook(spy.server.URL)}

	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatalf("Add: %v", err)
	}

	HandleTorrentFailure(hash, errors.New("boom"), backend, fm)

	spy.waitForCall(t)
	if _, ok := backend.Get(hash); ok {
		t.Error("orphan failed torrent should still be removed from the session")
	}
}

// Without a webhook subscribed to download_failed nothing is fired, but the torrent is
// still dropped from the session (the retry path must not depend on notifications).
func TestHandleTorrentFailure_NoWebhookStillRemoves(t *testing.T) {
	const hash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	spy := newWebhookSpy(t)
	fm := &lifecycleFM{configs: &files.Config{}} // no webhooks at all

	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatalf("Add: %v", err)
	}

	HandleTorrentFailure(hash, errors.New("boom"), backend, fm)

	if _, ok := backend.Get(hash); ok {
		t.Error("failed torrent should be removed even with no webhook configured")
	}
	if spy.callCount() != 0 {
		t.Error("no webhook is configured; nothing should have been fired")
	}
}

// exdevFS is an OSFileSystem whose Link always fails with EXDEV, simulating a filesystem
// that does not support hardlinks (exFAT, FAT32, some SMB/NFS mounts).
type exdevFS struct {
	*files.OSFileSystem
}

func (exdevFS) Link(oldname, newname string) error {
	return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: syscall.EXDEV}
}

// mockEmptyAniList makes searchAnilist succeed with an empty media list, so a pass that gets
// past the gate completes without touching the network.
func mockEmptyAniList(t *testing.T) {
	t.Helper()
	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(restore)
}

// P3.3 — a completed path whose filesystem does not support hardlinks must abort the pass
// with an actionable LastCheckError instead of downloading episodes that can never be
// hardlinked.
func TestAnimeVerification_NoHardlinkSupportAbortsPass(t *testing.T) {
	mockEmptyAniList(t)

	fm := &lifecycleFM{configs: &files.Config{
		AnilistUsernames:   []string{"tester"},
		SavePath:           t.TempDir(),
		CompletedAnimePath: t.TempDir(),
		CheckInterval:      10,
	}}
	state := NewState()
	librarian := files.NewLibrarian(exdevFS{files.NewOSFileSystem()})

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), librarian)

	err := state.GetLastCheckError()
	if err == nil {
		t.Fatal("expected LastCheckError to be set when the filesystem does not support hardlinks")
	}
	if !strings.Contains(err.Error(), "does not support hardlinks") {
		t.Errorf("LastCheckError should carry the same message the config endpoint returns, got %q", err)
	}
	if fm.passRan() {
		t.Error("verification must abort before running the pass when the hardlink probe fails")
	}
}

// P3.3 — valid (same-volume) paths keep the pass running.
func TestAnimeVerification_ValidPathsRunPass(t *testing.T) {
	mockEmptyAniList(t)

	base := t.TempDir()
	fm := &lifecycleFM{configs: &files.Config{
		AnilistUsernames:   []string{"tester"},
		SavePath:           base + "/downloads",
		CompletedAnimePath: base + "/library",
		CheckInterval:      10,
	}}
	state := NewState()
	librarian := files.NewLibrarian(files.NewOSFileSystem())

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), librarian)

	if err := state.GetLastCheckError(); err != nil {
		t.Fatalf("expected no LastCheckError for same-volume paths, got %v", err)
	}
	if !fm.passRan() {
		t.Error("verification should have run the pass with valid paths")
	}
	// The probe must not leave its scratch files behind in the library.
	if _, err := os.Stat(fm.configs.CompletedAnimePath + "/.aad_link_probe"); err == nil {
		t.Error("probe file leaked into the completed anime path")
	}
}

// The probe gate sits after the emptiness gate: an incomplete config still reports the
// "missing required configuration" error, not a probe error.
func TestAnimeVerification_IncompleteConfigStillReportsMissingConfig(t *testing.T) {
	fm := &lifecycleFM{configs: &files.Config{AnilistUsernames: []string{"tester"}}}
	state := NewState()

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	err := state.GetLastCheckError()
	if err == nil || !strings.Contains(err.Error(), "missing required configuration") {
		t.Fatalf("expected the missing-configuration error, got %v", err)
	}
	if fm.passRan() {
		t.Error("verification must not run the pass with an incomplete config")
	}
}
