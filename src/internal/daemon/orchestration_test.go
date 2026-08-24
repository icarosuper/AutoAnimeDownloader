package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// orchestrationFM is a FileManager mock recording upserts/deletes for orchestration tests.
type orchestrationFM struct {
	mockFileManagerForEpisodes
	saved    []files.EpisodeStruct
	upserted [][]files.EpisodeStruct
	deleted  []files.EpisodeKey
	configs  *files.Config
}

func (m *orchestrationFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) { return m.saved, nil }
func (m *orchestrationFM) LoadConfigs() (*files.Config, error)               { return m.configs, nil }
func (m *orchestrationFM) UpsertEpisodes(eps []files.EpisodeStruct) error {
	m.upserted = append(m.upserted, eps)
	// reflect the write-back so subsequent LoadSavedEpisodes sees LibraryPaths.
	for _, up := range eps {
		for i := range m.saved {
			if m.saved[i].EpisodeNumber == up.EpisodeNumber {
				m.saved[i] = up
			}
		}
	}
	return nil
}
func (m *orchestrationFM) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	m.deleted = append(m.deleted, keys...)
	return nil
}

func makeTorrentDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "episode.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	return dir
}

func TestOrganizeTorrent_SingleEpisode(t *testing.T) {
	dataDir := makeTorrentDataDir(t)
	completed := t.TempDir()
	const hash = "0123456789abcdef0123456789abcdef01234567"

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)

	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 5},
		},
		configs: &files.Config{CompletedAnimePath: completed, RenameFilesForJellyfin: true},
	}
	lib := files.NewLibrarian(files.NewOSFileSystem())

	if ok := organizeTorrent(hash, backend, lib, fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}

	// Library link created with Jellyfin name.
	wantLink := filepath.Join(completed, "My Anime", "My Anime - E05.mkv")
	if _, err := os.Stat(wantLink); err != nil {
		t.Errorf("expected library link %s: %v", wantLink, err)
	}
	// LibraryPaths written back.
	if len(fm.upserted) != 1 || len(fm.saved[0].LibraryPaths) != 1 {
		t.Errorf("expected LibraryPaths written back, upserted=%v saved=%v", fm.upserted, fm.saved)
	}

	// Idempotent: second run does not re-organize (no new upsert).
	if ok := organizeTorrent(hash, backend, lib, fm, fm.configs); !ok {
		t.Fatal("second organizeTorrent should succeed (no-op)")
	}
	if len(fm.upserted) != 1 {
		t.Errorf("expected no second write-back, got %d upserts", len(fm.upserted))
	}
}

// O pack tambem passa RenameFilesForJellyfin adiante: dois episodios no mesmo hash marcam
// IsBatch, e cada arquivo do pack e renomeado pelo numero lido do proprio nome, na mesma pasta
// que os avulsos usariam.
func TestOrganizeTorrent_BatchRenamesEachFile(t *testing.T) {
	dataDir := t.TempDir()
	for _, name := range []string{"[Sub] My Anime - 01 [1080p].mkv", "[Sub] My Anime - 02 [1080p].mkv"} {
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte("video"), 0644); err != nil {
			t.Fatalf("write video: %v", err)
		}
	}
	completed := t.TempDir()
	const hash = "0123456789abcdef0123456789abcdef01234567"

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)

	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 1, IsBatch: true},
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 2, IsBatch: true},
		},
		configs: &files.Config{CompletedAnimePath: completed, RenameFilesForJellyfin: true},
	}

	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}

	for _, name := range []string{"My Anime - E01.mkv", "My Anime - E02.mkv"} {
		if _, err := os.Stat(filepath.Join(completed, "My Anime", name)); err != nil {
			t.Errorf("expected library link %s: %v", name, err)
		}
	}
}

func TestOrganizeTorrent_NotCompletedRetries(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatal(err)
	}
	fm := &orchestrationFM{
		saved:   []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}},
		configs: &files.Config{CompletedAnimePath: t.TempDir()},
	}
	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); ok {
		t.Error("organizeTorrent should return false (retry) when torrent not completed")
	}
}

func TestOrganizeTorrent_NoMatchingEpisodeRetries(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, t.TempDir())
	fm := &orchestrationFM{saved: nil, configs: &files.Config{CompletedAnimePath: t.TempDir()}}
	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); ok {
		t.Error("organizeTorrent should return false (retry) when no saved episode matches")
	}
}

func TestRemoveEpisodesAndLinks_BatchGuard(t *testing.T) {
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: hash, IsBatch: true},
		{EpisodeNumber: 2, EpisodeHash: hash, IsBatch: true},
	}
	lib := files.NewLibrarian(files.NewOSFileSystem())

	// Delete only episode 1 → sibling survives → torrent kept.
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, t.TempDir())
	fm := &orchestrationFM{saved: saved}
	removeEpisodesAndLinks(fm, backend, lib, []files.EpisodeKey{{Episode: 1}}, saved, false)
	if _, ok := backend.Get(hash); !ok {
		t.Error("torrent should be kept while a batch sibling survives")
	}

	// Delete both → torrent removed.
	backend2 := torrents.NewFakeBackend()
	backend2.AddCompleted(hash, t.TempDir())
	fm2 := &orchestrationFM{saved: saved}
	removeEpisodesAndLinks(fm2, backend2, lib, []files.EpisodeKey{{Episode: 1}, {Episode: 2}}, saved, false)
	if _, ok := backend2.Get(hash); ok {
		t.Error("torrent should be removed when all batch siblings are deleted")
	}
}

// --- P5.1: torrent completion event → JobOrganize enqueued AND executed ---

// configWithCompletedWebhook returns a library config plus a webhook subscribed to
// download_completed, echoing the interpolated variables into the request body.
func configWithCompletedWebhook(completedPath, url string) *files.Config {
	return &files.Config{
		CompletedAnimePath:     completedPath,
		RenameFilesForJellyfin: true,
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{{
				Name:   "spy",
				URL:    url,
				Method: http.MethodPost,
				Body:   `{"anime":"{{anime_name}}","episode":"{{episode}}"}`,
				Events: []string{"download_completed"},
			}},
		},
	}
}

// queuedOrganizeHashes returns the hashes of the JobOrganize jobs currently in the queue.
func queuedOrganizeHashes(t *testing.T, q *JobQueue) []string {
	t.Helper()
	var out []string
	for _, j := range q.jobs {
		if j.Type != JobOrganize {
			continue
		}
		var p OrganizePayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatalf("unmarshal organize payload: %v", err)
		}
		out = append(out, p.Hash)
	}
	return out
}

// TestTorrentCompletion_EnqueuesAndRunsOrganize wires the *production* completion callback
// (the closure cmd/daemon/main.go installs on the backend) to a real JobQueue and drives a
// completion through the fake backend. Nothing is stubbed between the torrent event and the
// file landing in the library: the queue's own processDueJobs runs the job, the librarian
// creates a real hardlink on disk, LibraryPaths is written back and the webhook fires.
func TestTorrentCompletion_EnqueuesAndRunsOrganize(t *testing.T) {
	dataDir := makeTorrentDataDir(t)
	completed := t.TempDir()
	const hash = "0123456789abcdef0123456789abcdef01234567"

	spy := newWebhookSpy(t)
	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 5},
		},
		configs: configWithCompletedWebhook(completed, spy.server.URL),
	}

	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatalf("Add: %v", err)
	}

	q := NewJobQueue(fm, filepath.Join(t.TempDir(), "jobs.json"))
	q.SetOrchestration(backend, files.NewLibrarian(files.NewOSFileSystem()))
	// Exactly the wiring from cmd/daemon/main.go.
	backend.SetCallbacks(func(h string) { q.EnqueueOrganize(h) }, nil)

	// The torrent finishes: rain fires NotifyComplete → Session.onComplete → this callback.
	backend.CompleteTorrent(hash, dataDir)

	queued := queuedOrganizeHashes(t, q)
	if len(queued) != 1 || queued[0] != hash {
		t.Fatalf("completion should have enqueued one JobOrganize for %s, queue has %v", hash, queued)
	}

	// ...and the queue actually executes it. This leg had no coverage before.
	q.processDueJobs()

	if len(q.jobs) != 0 {
		t.Errorf("job should be drained after a successful run, %d left", len(q.jobs))
	}
	wantLink := filepath.Join(completed, "My Anime", "My Anime - E05.mkv")
	linkInfo, err := os.Stat(wantLink)
	if err != nil {
		t.Fatalf("expected the library hardlink %s to exist: %v", wantLink, err)
	}
	srcInfo, err := os.Stat(filepath.Join(dataDir, "episode.mkv"))
	if err != nil {
		t.Fatalf("stat seeded file: %v", err)
	}
	if !os.SameFile(srcInfo, linkInfo) {
		t.Error("library file must be a hardlink of the seeded file, not a copy")
	}
	if len(fm.saved[0].LibraryPaths) != 1 || fm.saved[0].LibraryPaths[0] != wantLink {
		t.Errorf("expected LibraryPaths written back to %s, got %v", wantLink, fm.saved[0].LibraryPaths)
	}
	body := spy.waitForCall(t)
	if !strings.Contains(body, `"anime":"My Anime"`) || !strings.Contains(body, `"episode":"5"`) {
		t.Errorf("download_completed webhook body = %s", body)
	}
}

// A completion whose episode record is not persisted yet must not be lost: the job stays in
// the queue with an incremented attempt count (bounded retry), it is not silently dropped.
func TestTorrentCompletion_JobRetriesWhenEpisodeNotSavedYet(t *testing.T) {
	dataDir := makeTorrentDataDir(t)
	const hash = "0123456789abcdef0123456789abcdef01234567"

	fm := &orchestrationFM{configs: &files.Config{CompletedAnimePath: t.TempDir()}}
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatalf("Add: %v", err)
	}

	q := NewJobQueue(fm, filepath.Join(t.TempDir(), "jobs.json"))
	q.SetOrchestration(backend, files.NewLibrarian(files.NewOSFileSystem()))
	backend.SetCallbacks(func(h string) { q.EnqueueOrganize(h) }, nil)

	backend.CompleteTorrent(hash, dataDir)
	q.processDueJobs()

	if len(q.jobs) != 1 {
		t.Fatalf("job should survive for a retry, queue has %d jobs", len(q.jobs))
	}
	if q.jobs[0].Attempts != 1 {
		t.Errorf("expected attempts=1 after the failed run, got %d", q.jobs[0].Attempts)
	}
	if !q.jobs[0].NextRun.After(q.jobs[0].CreatedAt) {
		t.Error("failed job should have been rescheduled with a backoff")
	}
}

// --- P5.2: deleting an episode frees BOTH links, with real files on disk ---

// makeTorrentDir writes the given file names into a fresh "torrent data dir" under root.
func makeTorrentDir(t *testing.T, root string, names ...string) string {
	t.Helper()
	dir := filepath.Join(root, "torrent")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir torrent dir: %v", err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return dir
}

// linkIntoLibrary hardlinks a torrent's files into the library through the real Librarian and
// returns the created paths, so LibraryPaths in these tests are actual links on disk.
func linkIntoLibrary(t *testing.T, lib files.Librarian, dataDir, completed, animeName string, episodeNumber *int, isBatch bool) []string {
	t.Helper()
	created, err := lib.Organize(files.OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      animeName,
		CompletedPath:  completed,
		EpisodeNumber:  episodeNumber,
		IsBatch:        isBatch,
		RenameJellyfin: episodeNumber != nil,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	for _, p := range created {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("library link %s was not created: %v", p, err)
		}
	}
	return created
}

func assertGone(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s should be gone (%s), stat error = %v", what, path, err)
	}
}

func assertExists(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s should still exist (%s): %v", what, path, err)
	}
}

// TestRemoveEpisodesAndLinks_RealHardlinks covers the "free BOTH links" contract with genuine
// hardlinks on disk (t.TempDir() + OSFileSystem), which the mock-only batch-guard test never
// exercised: it never populated LibraryPaths, so the library half was untested.
func TestRemoveEpisodesAndLinks_RealHardlinks(t *testing.T) {
	five := 5
	lib := files.NewLibrarian(files.NewOSFileSystem())

	t.Run("single episode: library link and torrent both go", func(t *testing.T) {
		root := t.TempDir()
		dataDir := makeTorrentDir(t, root, "episode.mkv")
		completed := filepath.Join(root, "library")
		const hash = "0123456789abcdef0123456789abcdef01234567"

		created := linkIntoLibrary(t, lib, dataDir, completed, "My Anime", &five, false)

		saved := []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 5, LibraryPaths: created},
		}
		backend := torrents.NewFakeBackend()
		backend.AddCompleted(hash, dataDir)
		fm := &orchestrationFM{saved: saved}

		if err := removeEpisodesAndLinks(fm, backend, lib, []files.EpisodeKey{{Episode: 5}}, saved, false); err != nil {
			t.Fatalf("removeEpisodesAndLinks: %v", err)
		}

		assertGone(t, created[0], "library hardlink")
		if _, ok := backend.Get(hash); ok {
			t.Error("torrent (seeding copy) should have been removed")
		}
		if !containsID(fm.deleted, files.EpisodeKey{Episode: 5}) {
			t.Errorf("episode record should have been deleted, got %v", fm.deleted)
		}
	})

	t.Run("surviving sibling: this episode's link goes, the torrent stays", func(t *testing.T) {
		// Two non-batch episodes sharing one infohash: the batch guard must keep the torrent
		// alive for the survivor while still freeing the deleted episode's library link.
		root := t.TempDir()
		dataDir := makeTorrentDir(t, root, "ep01.mkv", "ep02.mkv")
		completed := filepath.Join(root, "library")
		const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		created := linkIntoLibrary(t, lib, dataDir, completed, "My Anime", nil, false)
		if len(created) != 2 {
			t.Fatalf("expected 2 library links, got %v", created)
		}

		saved := []files.EpisodeStruct{
			{EpisodeNumber: 1, EpisodeHash: hash, AnimeName: "My Anime", LibraryPaths: created[:1]},
			{EpisodeNumber: 2, EpisodeHash: hash, AnimeName: "My Anime", LibraryPaths: created[1:]},
		}
		backend := torrents.NewFakeBackend()
		backend.AddCompleted(hash, dataDir)
		fm := &orchestrationFM{saved: saved}

		if err := removeEpisodesAndLinks(fm, backend, lib, []files.EpisodeKey{{Episode: 1}}, saved, false); err != nil {
			t.Fatalf("removeEpisodesAndLinks: %v", err)
		}

		assertGone(t, created[0], "deleted episode's library hardlink")
		assertExists(t, created[1], "surviving sibling's library hardlink")
		if _, ok := backend.Get(hash); !ok {
			t.Error("torrent must be kept while a sibling still references it")
		}
		// The seeding copies are untouched: only the library name was dropped.
		assertExists(t, filepath.Join(dataDir, "ep01.mkv"), "seeded file of the deleted episode")
		assertExists(t, filepath.Join(dataDir, "ep02.mkv"), "seeded file of the survivor")
	})

	t.Run("batch with a surviving sibling keeps both the torrent and its library files", func(t *testing.T) {
		// Batch library files keep raw release names, which cannot be mapped back to a single
		// episode, so they are only freed when the whole torrent goes (see the contract in
		// removeEpisodesAndLinks). This pins that deliberate asymmetry with real files.
		root := t.TempDir()
		dataDir := makeTorrentDir(t, root, "ep01.mkv", "ep02.mkv")
		completed := filepath.Join(root, "library")
		const hash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

		created := linkIntoLibrary(t, lib, dataDir, completed, "My Anime", nil, true)

		saved := []files.EpisodeStruct{
			{EpisodeNumber: 1, EpisodeHash: hash, AnimeName: "My Anime", IsBatch: true, LibraryPaths: created},
			{EpisodeNumber: 2, EpisodeHash: hash, AnimeName: "My Anime", IsBatch: true, LibraryPaths: created},
		}
		backend := torrents.NewFakeBackend()
		backend.AddCompleted(hash, dataDir)
		fm := &orchestrationFM{saved: saved}

		if err := removeEpisodesAndLinks(fm, backend, lib, []files.EpisodeKey{{Episode: 1}}, saved, false); err != nil {
			t.Fatalf("removeEpisodesAndLinks: %v", err)
		}

		if _, ok := backend.Get(hash); !ok {
			t.Error("torrent must be kept while a batch sibling survives")
		}
		for _, p := range created {
			assertExists(t, p, "batch library file with a surviving sibling")
		}
	})

	t.Run("batch fully deleted: every library file and the torrent go", func(t *testing.T) {
		root := t.TempDir()
		dataDir := makeTorrentDir(t, root, "ep01.mkv", "ep02.mkv")
		completed := filepath.Join(root, "library")
		const hash = "cccccccccccccccccccccccccccccccccccccccc"

		created := linkIntoLibrary(t, lib, dataDir, completed, "My Anime", nil, true)

		saved := []files.EpisodeStruct{
			{EpisodeNumber: 1, EpisodeHash: hash, AnimeName: "My Anime", IsBatch: true, LibraryPaths: created},
			{EpisodeNumber: 2, EpisodeHash: hash, AnimeName: "My Anime", IsBatch: true, LibraryPaths: created},
		}
		backend := torrents.NewFakeBackend()
		backend.AddCompleted(hash, dataDir)
		fm := &orchestrationFM{saved: saved}

		if err := removeEpisodesAndLinks(fm, backend, lib, []files.EpisodeKey{{Episode: 1}, {Episode: 2}}, saved, false); err != nil {
			t.Fatalf("removeEpisodesAndLinks: %v", err)
		}

		for _, p := range created {
			assertGone(t, p, "batch library file after full deletion")
		}
		if _, ok := backend.Get(hash); ok {
			t.Error("torrent should be removed once no sibling references it")
		}
	})
}

func TestReconcileLibrary(t *testing.T) {
	const organized = "1111111111111111111111111111111111111111"
	const pending = "2222222222222222222222222222222222222222"
	const notComplete = "3333333333333333333333333333333333333333"

	downloaded := []torrents.TorrentInfo{
		{Hash: organized, Completed: true},
		{Hash: pending, Completed: true},
		{Hash: notComplete, Completed: false},
	}
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: organized, LibraryPaths: []string{"/lib/a.mkv"}},
		{EpisodeNumber: 2, EpisodeHash: pending},
		{EpisodeNumber: 3, EpisodeHash: notComplete},
	}

	q := NewJobQueue(&orchestrationFM{}, filepath.Join(t.TempDir(), "jobs.json"))
	reconcileLibrary(downloaded, saved, q)

	// Only the pending (completed, no LibraryPaths) torrent should be enqueued.
	if len(q.jobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(q.jobs))
	}
	var p OrganizePayload
	if err := json.Unmarshal(q.jobs[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Hash != pending {
		t.Errorf("expected enqueue for %s, got %s", pending, p.Hash)
	}
}

// --- P5.3: startup reconciliation ---

// TestReconcileLibrary_Marker pins down what the startup reconciliation treats as "not yet
// organized". The marker is an EMPTY LibraryPaths on the saved record — deliberately NOT
// "the hardlink is missing on disk", which is what the original design doc asked for. Keying
// off the record means a library file the user deleted on purpose is not silently recreated
// on every pass; keying off the disk would resurrect it forever.
func TestReconcileLibrary_Marker(t *testing.T) {
	newQueue := func(t *testing.T) *JobQueue {
		t.Helper()
		return NewJobQueue(&orchestrationFM{}, filepath.Join(t.TempDir(), "jobs.json"))
	}

	t.Run("seeding torrent without LibraryPaths is enqueued", func(t *testing.T) {
		const hash = "1111111111111111111111111111111111111111"
		q := newQueue(t)
		reconcileLibrary(
			[]torrents.TorrentInfo{{Hash: hash, Completed: true}},
			[]files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}},
			q,
		)
		got := queuedOrganizeHashes(t, q)
		if len(got) != 1 || got[0] != hash {
			t.Errorf("expected a JobOrganize for the unorganized seeding torrent, got %v", got)
		}
	})

	t.Run("seeding torrent with LibraryPaths is not enqueued even if the file is gone", func(t *testing.T) {
		const hash = "2222222222222222222222222222222222222222"
		// A path that provably does not exist: the marker is the record, not the disk.
		missing := filepath.Join(t.TempDir(), "deleted-by-the-user.mkv")
		assertGone(t, missing, "fixture path")

		q := newQueue(t)
		reconcileLibrary(
			[]torrents.TorrentInfo{{Hash: hash, Completed: true}},
			[]files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{missing}}},
			q,
		)
		if got := queuedOrganizeHashes(t, q); len(got) != 0 {
			t.Errorf("an already-organized record must not be re-enqueued (deleted files stay deleted), got %v", got)
		}
	})

	t.Run("not-completed torrent is never enqueued", func(t *testing.T) {
		const hash = "3333333333333333333333333333333333333333"
		q := newQueue(t)
		reconcileLibrary(
			[]torrents.TorrentInfo{{Hash: hash, Completed: false}},
			[]files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}},
			q,
		)
		if got := queuedOrganizeHashes(t, q); len(got) != 0 {
			t.Errorf("a torrent that is still downloading must not be organized, got %v", got)
		}
	})

	t.Run("batch is enqueued once when any sibling is unorganized", func(t *testing.T) {
		const hash = "4444444444444444444444444444444444444444"
		q := newQueue(t)
		reconcileLibrary(
			[]torrents.TorrentInfo{{Hash: hash, Completed: true}},
			[]files.EpisodeStruct{
				{EpisodeNumber: 1, EpisodeHash: hash, IsBatch: true, LibraryPaths: []string{"/lib/a.mkv"}},
				{EpisodeNumber: 2, EpisodeHash: hash, IsBatch: true},
			},
			q,
		)
		if got := queuedOrganizeHashes(t, q); len(got) != 1 || got[0] != hash {
			t.Errorf("a batch with one unorganized sibling should be enqueued exactly once, got %v", got)
		}
	})

	t.Run("orphan torrent with no saved episode is not enqueued", func(t *testing.T) {
		q := newQueue(t)
		reconcileLibrary(
			[]torrents.TorrentInfo{{Hash: "5555555555555555555555555555555555555555", Completed: true}},
			nil,
			q,
		)
		if got := queuedOrganizeHashes(t, q); len(got) != 0 {
			t.Errorf("an orphan torrent has nothing to organize, got %v", got)
		}
	})

	t.Run("repeated passes do not pile up duplicate jobs", func(t *testing.T) {
		const hash = "6666666666666666666666666666666666666666"
		downloaded := []torrents.TorrentInfo{{Hash: hash, Completed: true}}
		saved := []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}}

		q := newQueue(t)
		reconcileLibrary(downloaded, saved, q)
		reconcileLibrary(downloaded, saved, q)
		reconcileLibrary(downloaded, saved, q)

		if got := queuedOrganizeHashes(t, q); len(got) != 1 {
			t.Errorf("reconciliation runs every pass; enqueue must dedupe, got %v", got)
		}
	})
}

// TestClearLibraryPathsAfterRootSwap covers the documented exception to decision #29: the
// records are cleared when the WHOLE download root was swapped, so the library is rebuilt at
// the configured path instead of staying empty forever with the records claiming otherwise.
func TestClearLibraryPathsAfterRootSwap(t *testing.T) {
	t.Run("organized records are cleared and become reconcilable again", func(t *testing.T) {
		const hash = "7777777777777777777777777777777777777777"
		gone := filepath.Join(t.TempDir(), "moved-away", "Anime", "E01.mkv")
		fm := &orchestrationFM{
			saved: []files.EpisodeStruct{
				{EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{gone}},
			},
		}

		clearLibraryPathsAfterRootSwap(fm, t.TempDir())

		if len(fm.upserted) != 1 {
			t.Fatalf("expected one write-back, got %d", len(fm.upserted))
		}
		if paths := fm.saved[0].LibraryPaths; len(paths) != 0 {
			t.Errorf("LibraryPaths = %v, want empty so reconciliation picks the episode up again", paths)
		}

		q := NewJobQueue(&orchestrationFM{}, filepath.Join(t.TempDir(), "jobs.json"))
		reconcileLibrary([]torrents.TorrentInfo{{Hash: hash, Completed: true}}, fm.saved, q)
		if got := queuedOrganizeHashes(t, q); len(got) != 1 || got[0] != hash {
			t.Errorf("a cleared record must be re-enqueued for organizing, got %v", got)
		}
	})

	t.Run("records with nothing organized are left alone", func(t *testing.T) {
		fm := &orchestrationFM{
			saved: []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: "8888888888888888888888888888888888888888"}},
		}

		clearLibraryPathsAfterRootSwap(fm, t.TempDir())

		if len(fm.upserted) != 0 {
			t.Errorf("nothing to clear must mean no write to the episodes file, got %v", fm.upserted)
		}
	})
}

// O AnimeTotalEpisodes do registro chega ao Librarian: sem ele, um pack de season 2 com
// numeracao continua entraria na biblioteca como E13/E14 numa pasta de 2 episodios.
func TestOrganizeTorrent_BatchContinuousNumbering(t *testing.T) {
	dataDir := t.TempDir()
	for _, name := range []string{"[Sub] My Anime S2 - 13 [1080p].mkv", "[Sub] My Anime S2 - 14 [1080p].mkv"} {
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte(name), 0644); err != nil {
			t.Fatalf("write video: %v", err)
		}
	}
	completed := t.TempDir()
	const hash = "0123456789abcdef0123456789abcdef01234567"

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)

	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime Season 2", AnimeTotalEpisodes: 2, EpisodeNumber: 1, IsBatch: true},
			{EpisodeHash: hash, AnimeName: "My Anime Season 2", AnimeTotalEpisodes: 2, EpisodeNumber: 2, IsBatch: true},
		},
		configs: &files.Config{CompletedAnimePath: completed, RenameFilesForJellyfin: true},
	}

	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}
	for _, name := range []string{"My Anime Season 2 - E01.mkv", "My Anime Season 2 - E02.mkv"} {
		if _, err := os.Stat(filepath.Join(completed, "My Anime Season 2", name)); err != nil {
			t.Errorf("expected library link %s: %v", name, err)
		}
	}
}
