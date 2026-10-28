package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"sync"
	"testing"
)

// budgetConfig points the guard at a real temp dir with the percentage off, and returns 60% of its
// free space: a torrent that size fits alone, two of them don't.
func budgetConfig(t *testing.T) (*files.Config, int64) {
	t.Helper()
	configs := limitsConfig()
	configs.CompletedAnimePath = t.TempDir()
	configs.MinFreeDiskGB = 0
	_, free, err := files.DiskSpace(configs.CompletedAnimePath)
	if err != nil || free == 0 {
		t.Skipf("no free-space reading for the temp dir: %v", err)
	}
	return configs, int64(free) * 6 / 10
}

// TestDiskBudget_TwoTorrentsThatOnlyFitAlone: the second one is refused because the first one
// reserved its bytes, even though the disk is still empty — the 130 GiB queued into 6 GiB.
func TestDiskBudget_TwoTorrentsThatOnlyFitAlone(t *testing.T) {
	configs, size := budgetConfig(t)
	backend := torrents.NewFakeBackend()
	disk := newDiskBudget(nil, nil)

	first, _ := attemptDownloadWithRetries(configs, backend, disk, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(1), Size: size}}, "ep 1")
	second, _ := attemptDownloadWithRetries(configs, backend, disk, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(2), Size: size}}, "ep 2")

	if first == "" || second != "" {
		t.Fatalf("want only the first added, got first=%q second=%q", first, second)
	}
}

// TestDiskBudget_PackPaysOnceAcrossItsEpisodes: the episodes of a pack add the same magnet, from
// parallel animes too; the pack reserves its size once, so every episode gets recorded.
func TestDiskBudget_PackPaysOnceAcrossItsEpisodes(t *testing.T) {
	configs, size := budgetConfig(t)
	backend := torrents.NewFakeBackend()
	disk := newDiskBudget(nil, nil)
	pack := []nyaa.TorrentResult{{MagnetLink: fakeMagnet(7), Size: size}}

	var wg sync.WaitGroup
	hashes := make([]string, 12)
	for i := range hashes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashes[i], _ = attemptDownloadWithRetries(configs, backend, disk, pack, "pack episode")
		}()
	}
	wg.Wait()

	for i, h := range hashes {
		if h == "" {
			t.Fatalf("episode %d of the pack was refused: the pack was paid more than once", i+1)
		}
	}
	if disk.reserved != size {
		t.Errorf("want the pack reserved once (%d), got %d", size, disk.reserved)
	}
}

// TestNewDiskBudget_QueuedTorrentReservesItsRecordedSize: a queued torrent never fetched its
// metadata (BytesTotal 0), so its size comes from the record — once per hash, not once per episode
// of the pack. A completed one reserves nothing, and a paused one only what it still lacks.
func TestNewDiskBudget_QueuedTorrentReservesItsRecordedSize(t *testing.T) {
	session := []torrents.TorrentInfo{
		{Hash: "queued"},
		{Hash: "seeding", Completed: true, BytesTotal: 500},
		{Hash: "paused", BytesTotal: 1000, PiecesHave: 3, PiecesTotal: 4},
	}
	saved := []files.EpisodeStruct{
		{EpisodeHash: "queued", EpisodeNumber: 1, TorrentSize: 100},
		{EpisodeHash: "queued", EpisodeNumber: 2, TorrentSize: 100},
		{EpisodeHash: "queued", EpisodeNumber: 3, TorrentSize: 100},
	}

	if got := newDiskBudget(session, saved).reserved; got != 100+250 {
		t.Errorf("want 350 reserved (queued pack once + paused remainder), got %d", got)
	}
}
