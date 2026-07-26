package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// orchestrationFM is a FileManager mock recording upserts/deletes for orchestration tests.
type orchestrationFM struct {
	mockFileManagerForEpisodes
	saved     []files.EpisodeStruct
	upserted  [][]files.EpisodeStruct
	deleted   []int
	configs   *files.Config
}

func (m *orchestrationFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) { return m.saved, nil }
func (m *orchestrationFM) LoadConfigs() (*files.Config, error)               { return m.configs, nil }
func (m *orchestrationFM) UpsertEpisodes(eps []files.EpisodeStruct) error {
	m.upserted = append(m.upserted, eps)
	// reflect the write-back so subsequent LoadSavedEpisodes sees LibraryPaths.
	for _, up := range eps {
		for i := range m.saved {
			if m.saved[i].EpisodeID == up.EpisodeID {
				m.saved[i] = up
			}
		}
	}
	return nil
}
func (m *orchestrationFM) DeleteEpisodesFromFile(ids []int) error {
	m.deleted = append(m.deleted, ids...)
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
			{EpisodeID: 1, EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 5},
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

func TestOrganizeTorrent_NotCompletedRetries(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add("magnet:?xt=urn:btih:" + hash); err != nil {
		t.Fatal(err)
	}
	fm := &orchestrationFM{
		saved:   []files.EpisodeStruct{{EpisodeID: 1, EpisodeHash: hash}},
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
		{EpisodeID: 1, EpisodeHash: hash, IsBatch: true},
		{EpisodeID: 2, EpisodeHash: hash, IsBatch: true},
	}
	lib := files.NewLibrarian(files.NewOSFileSystem())

	// Delete only episode 1 → sibling survives → torrent kept.
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, t.TempDir())
	fm := &orchestrationFM{saved: saved}
	removeEpisodesAndLinks(fm, backend, lib, []int{1}, saved)
	if _, ok := backend.Get(hash); !ok {
		t.Error("torrent should be kept while a batch sibling survives")
	}

	// Delete both → torrent removed.
	backend2 := torrents.NewFakeBackend()
	backend2.AddCompleted(hash, t.TempDir())
	fm2 := &orchestrationFM{saved: saved}
	removeEpisodesAndLinks(fm2, backend2, lib, []int{1, 2}, saved)
	if _, ok := backend2.Get(hash); ok {
		t.Error("torrent should be removed when all batch siblings are deleted")
	}
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
		{EpisodeID: 1, EpisodeHash: organized, LibraryPaths: []string{"/lib/a.mkv"}},
		{EpisodeID: 2, EpisodeHash: pending},
		{EpisodeID: 3, EpisodeHash: notComplete},
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
