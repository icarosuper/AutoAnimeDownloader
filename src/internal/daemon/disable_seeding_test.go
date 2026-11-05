package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// disable_seeding, library half: once organized, the torrent leaves the session with its download
// copy, the record is marked released BEFORE the removal, and the library keeps the hardlink.
func TestOrganizeTorrent_DisableSeedingReleasesTheTorrent(t *testing.T) {
	dataDir := makeTorrentDataDir(t)
	completed := t.TempDir()
	const hash = "0123456789abcdef0123456789abcdef01234567"

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)
	fm := &orchestrationFM{
		saved:   []files.EpisodeStruct{{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 5}},
		configs: &files.Config{CompletedAnimePath: completed, RenameFilesForJellyfin: true, DisableSeeding: true},
	}

	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}

	if _, ok := backend.Get(hash); ok {
		t.Error("the organized torrent must leave the session")
	}
	if keep, removed := backend.RemovedKeepData[hash]; !removed || keep {
		t.Errorf("the download copy must go with it (keepData=false), got removed=%v keepData=%v", removed, keep)
	}
	if !fm.saved[0].TorrentReleased || len(fm.saved[0].LibraryPaths) != 1 {
		t.Errorf("record must be organized AND released: %+v", fm.saved[0])
	}
	// The hardlink survives the download copy going away.
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(completed, "My Anime", "My Anime - E05.mkv")); err != nil {
		t.Errorf("library file must survive the download copy: %v", err)
	}
}

// A torrent organized while the setting was off (or a release a crash interrupted) is already
// organized: reconcileLibrary sends it back under disable_seeding, and organize releases it.
func TestDisableSeeding_ReleasesTorrentsOrganizedBefore(t *testing.T) {
	const hash = "1111111111111111111111111111111111111111"
	saved := []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{"/lib/a.mkv"}}}
	seeding := []torrents.TorrentInfo{{Hash: hash, Completed: true}}

	q := NewJobQueue(&orchestrationFM{}, filepath.Join(t.TempDir(), "jobs.json"))
	reconcileLibrary(seeding, saved, q, false)
	if got := queuedOrganizeHashes(t, q); len(got) != 0 {
		t.Fatalf("without disable_seeding an organized torrent stays put, got %v", got)
	}
	reconcileLibrary(seeding, saved, q, true)
	if got := queuedOrganizeHashes(t, q); len(got) != 1 || got[0] != hash {
		t.Fatalf("with disable_seeding the organized torrent must be sent back, got %v", got)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, t.TempDir())
	fm := &orchestrationFM{saved: saved, configs: &files.Config{DisableSeeding: true}}
	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}
	if _, ok := backend.Get(hash); ok || !fm.saved[0].TorrentReleased {
		t.Errorf("an already-organized torrent must be released: inSession=%v record=%+v", ok, fm.saved[0])
	}
}

// The torrent missing from the session is not a loss for a released record: the pass must not
// download it again. A record that just lost its torrent still is one.
func TestSelectEpisodes_ReleasedTorrentIsNotMissing(t *testing.T) {
	anime := animeWithEpisodes(2, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 1)
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 2)
	released := savedFull[files.EpisodeKey{AnimeID: anime.Media.Id, Episode: 1}]
	released.TorrentReleased = true
	savedFull[released.Key()] = released

	sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{}, nil, nil)

	if len(sel.toDownload) != 1 || sel.toDownload[0].Episode != 2 {
		t.Errorf("only the episode that lost its torrent is redownloaded, got %+v", sel.toDownload)
	}
}

// A root swap means the library the released episodes lived in is gone: the flag goes too, or the
// pass would treat them as present forever.
func TestClearLibraryPathsAfterRootSwap_ClearsReleased(t *testing.T) {
	fm := &orchestrationFM{saved: []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: "2222222222222222222222222222222222222222", LibraryPaths: []string{"/gone/a.mkv"}, TorrentReleased: true},
	}}

	clearLibraryPathsAfterRootSwap(fm, t.TempDir())

	if fm.saved[0].TorrentReleased || len(fm.saved[0].LibraryPaths) != 0 {
		t.Errorf("record must be back to not-downloaded: %+v", fm.saved[0])
	}
}

// Deleting a released episode removes its library file and record, and does not try to remove a
// torrent that is no longer there.
func TestRemoveEpisodesAndLinks_ReleasedSkipsTheTorrent(t *testing.T) {
	const hash = "3333333333333333333333333333333333333333"
	link := filepath.Join(t.TempDir(), "My Anime - E01.mkv")
	if err := os.WriteFile(link, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	fm := &orchestrationFM{saved: []files.EpisodeStruct{
		{AnimeID: 7, EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{link}, TorrentReleased: true},
	}}
	backend := torrents.NewFakeBackend()

	key := files.EpisodeKey{AnimeID: 7, Episode: 1}
	if err := RemoveEpisodesWithLinks(fm, backend, files.NewLibrarian(files.NewOSFileSystem()), []files.EpisodeKey{key}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Errorf("library file must be removed, stat err=%v", err)
	}
	if _, tried := backend.RemovedKeepData[hash]; tried {
		t.Error("a released torrent is not in the session; there is nothing to remove")
	}
	if len(fm.deleted) != 1 {
		t.Errorf("record must be deleted, got %v", fm.deleted)
	}
}
