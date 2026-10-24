package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestManagerIn(t *testing.T, dir string) *FileManager {
	t.Helper()
	return NewManager(
		NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "episodes"),
		filepath.Join(dir, "blocked"),
		filepath.Join(dir, "settings"),
		filepath.Join(dir, "standalone"),
	)
}

func newTestManager(t *testing.T) *FileManager {
	t.Helper()
	return newTestManagerIn(t, t.TempDir())
}

// The old blocked_episodes was an array of AniList node ids. Those ids no longer exist
// anywhere, so the legacy file has to be discarded — and not become an error, which would
// stall LoadBlockedEpisodes (and with it the verification pass) forever.
func TestLoadBlockedEpisodes_DiscardsLegacyIntList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blocked"), []byte("[416348,416349]"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newTestManagerIn(t, dir)

	blocked, err := m.LoadBlockedEpisodes()
	if err != nil {
		t.Fatalf("legacy file must not become an error: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("want empty list, got %v", blocked)
	}

	// And the file works again in the new format.
	key := EpisodeKey{AnimeID: 21, Episode: 1123}
	if err := m.BlockEpisode(key); err != nil {
		t.Fatalf("BlockEpisode: %v", err)
	}
	blocked, err = m.LoadBlockedEpisodes()
	if err != nil || len(blocked) != 1 || blocked[0] != key {
		t.Fatalf("want [%v], got %v (err=%v)", key, blocked, err)
	}
}

func TestDeleteEmptyFolders(t *testing.T) {
	assertGone := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should have been deleted (%s): %v", what, path, err)
		}
	}
	assertKept := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should have been kept (%s): %v", what, path, err)
		}
	}

	t.Run("deletes an empty library folder", func(t *testing.T) {
		completed := t.TempDir()
		empty := filepath.Join(completed, "Empty Anime")
		if err := os.MkdirAll(empty, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		assertGone(t, empty, "empty folder")
	})

	t.Run("deletes a folder that only has nfo files", func(t *testing.T) {
		completed := t.TempDir()
		dir := filepath.Join(completed, "Prisma Illya")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tvshow.nfo"), []byte("show"), 0644); err != nil {
			t.Fatalf("write tvshow.nfo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Anime - E01.nfo"), []byte("ep"), 0644); err != nil {
			t.Fatalf("write episode nfo: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		assertGone(t, dir, "nfo-only folder")
	})

	t.Run("keeps a folder that has a video", func(t *testing.T) {
		completed := t.TempDir()
		dir := filepath.Join(completed, "Anime")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tvshow.nfo"), []byte("show"), 0644); err != nil {
			t.Fatalf("write nfo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "video.mkv"), []byte("video"), 0644); err != nil {
			t.Fatalf("write video: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		assertKept(t, dir, "folder with video")
		assertKept(t, filepath.Join(dir, "video.mkv"), "video")
	})

	t.Run("keeps a folder that has a poster", func(t *testing.T) {
		completed := t.TempDir()
		dir := filepath.Join(completed, "Anime")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tvshow.nfo"), []byte("show"), 0644); err != nil {
			t.Fatalf("write nfo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "poster.jpg"), []byte("art"), 0644); err != nil {
			t.Fatalf("write poster: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		assertKept(t, dir, "folder with poster")
		assertKept(t, filepath.Join(dir, "poster.jpg"), "poster")
	})

	// rain allocates <download>/<id> and only then writes into it. Without this guard the
	// library scan would delete freshly created torrent directories.
	t.Run("never enters the download directory", func(t *testing.T) {
		completed := t.TempDir()
		torrentDir := filepath.Join(completed, ".torrents", "abc123")
		if err := os.MkdirAll(torrentDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		assertKept(t, torrentDir, "empty torrent directory")
	})

	t.Run("rejects empty path", func(t *testing.T) {
		if err := newTestManager(t).DeleteEmptyFolders(""); err == nil {
			t.Error("want error for empty path, got nil")
		}
	})
}

// Regression: releasing several episodes at once from the UI fires a POST /release per
// episode in parallel, and each one does load -> mutate -> write the episodes file.
// Without a lock that loses updates and can even corrupt the file (two WriteFile
// truncating the same path at the same time), leaving LoadSavedEpisodes failing
// forever and taking /api/v1/animes down with 500.
func TestEpisodesFileConcurrency(t *testing.T) {
	seed := func(t *testing.T, m *FileManager, n int) {
		t.Helper()
		episodes := make([]EpisodeStruct, 0, n)
		for i := 0; i < n; i++ {
			episodes = append(episodes, EpisodeStruct{
				EpisodeNumber:   1 + i,
				EpisodeHash:     fmt.Sprintf("hash%04d", i),
				EpisodeName:     fmt.Sprintf("Very Long Anime Name To Fill The File - Episode %d", i),
				AnimeID:         488911345,
				AnimeName:       "Katainaka no Ossan, Kensei ni Naru II",
				DownloadDate:    time.Now(),
				ManuallyManaged: true,
			})
		}
		if err := m.SaveEpisodesToFile(episodes); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	t.Run("concurrent releases neither lose updates nor corrupt the file", func(t *testing.T) {
		const n = 30
		m := newTestManager(t)
		seed(t, m, n)

		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(ep int) {
				defer wg.Done()
				if err := m.UnmanageEpisode(EpisodeKey{AnimeID: 488911345, Episode: ep}); err != nil {
					t.Errorf("UnmanageEpisode(%d): %v", ep, err)
				}
			}(1 + i)
		}
		wg.Wait()

		got, err := m.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("episodes file corrupted: %v", err)
		}
		if len(got) != n {
			t.Fatalf("want %d saved episodes, got %d", n, len(got))
		}
		for _, ep := range got {
			if ep.ManuallyManaged {
				t.Errorf("episode %d is still manually managed: update lost", ep.EpisodeNumber)
			}
		}
	})

	// The daemon organizes torrents (UpsertEpisodes) while the UI polls
	// /api/v1/animes (LoadSavedEpisodes). A reader must never see the file
	// half-written.
	t.Run("concurrent read with write never sees a truncated file", func(t *testing.T) {
		const n = 40
		m := newTestManager(t)
		seed(t, m, n)

		done := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done)
			for i := 0; i < 50; i++ {
				if err := m.UpsertEpisodes([]EpisodeStruct{{
					AnimeID:       488911345,
					EpisodeNumber: 1,
					EpisodeHash:   "hash0000",
					EpisodeName:   fmt.Sprintf("rewrite %d", i),
					DownloadDate:  time.Now(),
				}}); err != nil {
					t.Errorf("UpsertEpisodes: %v", err)
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				eps, err := m.LoadSavedEpisodes()
				if err != nil {
					t.Errorf("read got a corrupted file: %v", err)
					return
				}
				if len(eps) != n {
					t.Errorf("read got an incomplete file: want %d episodes, got %d", n, len(eps))
					return
				}
			}
		}()

		wg.Wait()
	})

	t.Run("concurrent block and unblock do not corrupt the file", func(t *testing.T) {
		const n = 30
		m := newTestManager(t)

		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(ep int) {
				defer wg.Done()
				if err := m.BlockEpisode(EpisodeKey{AnimeID: 488911345, Episode: ep}); err != nil {
					t.Errorf("BlockEpisode(%d): %v", ep, err)
				}
			}(1 + i)
		}
		wg.Wait()

		blocked, err := m.LoadBlockedEpisodes()
		if err != nil {
			t.Fatalf("blocked file corrupted: %v", err)
		}
		if len(blocked) != n {
			t.Fatalf("want %d blocked episodes, got %d: update lost", n, len(blocked))
		}
	})
}
