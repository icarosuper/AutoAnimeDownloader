package daemon

import (
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

func stalledFixture(hash, status string, peers int, completed bool) torrents.TorrentInfo {
	return torrents.TorrentInfo{Hash: hash, Name: hash, Status: status, PeersTotal: peers, Completed: completed, PiecesHave: 1, PiecesTotal: 10}
}

// A torrent is only dropped after a FULL stalledTorrentTimeout at zero peers: the first
// pass only stamps the time.
func TestDropStalledTorrents_OnlyAfterTheTimeout(t *testing.T) {
	stalledTorrents.since = map[string]time.Time{}

	const hash = "1111111111111111111111111111111111111111"
	backend := torrents.NewFakeBackend()
	backend.AddPaused(hash, "stalled", 1, 10, false)
	fm := &lifecycleFM{configs: &files.Config{}}

	snapshot := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 0, false)}
	start := time.Now()

	dropStalledTorrents(snapshot, backend, fm, start)
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("the first pass only stamps the time, it does not drop")
	}

	dropStalledTorrents(snapshot, backend, fm, start.Add(stalledTorrentTimeout-time.Minute))
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("before the threshold the torrent stays in the session")
	}

	dropStalledTorrents(snapshot, backend, fm, start.Add(stalledTorrentTimeout))
	if _, ok := backend.Get(hash); ok {
		t.Error("after the threshold the torrent must leave the session to free the slot")
	}
	// Reuse of the failure path: the bytes already downloaded stay (PiecesHave > 0).
	if !backend.RemovedKeepData[hash] {
		t.Error("the drop must keep the bytes already downloaded")
	}
}

// Zero peers is only a symptom in whoever is trying. Queued, paused and completed stay.
func TestDropStalledTorrents_IgnoresWhatIsNotTrying(t *testing.T) {
	stalledTorrents.since = map[string]time.Time{}

	backend := torrents.NewFakeBackend()
	fm := &lifecycleFM{configs: &files.Config{}}

	snapshot := []torrents.TorrentInfo{
		stalledFixture("2222222222222222222222222222222222222222", torrents.StatusQueued, 0, false),
		stalledFixture("3333333333333333333333333333333333333333", torrents.StatusStopped, 0, false),
		stalledFixture("4444444444444444444444444444444444444444", "seeding", 0, true),
		stalledFixture("5555555555555555555555555555555555555555", torrents.StatusDownloading, 3, false),
	}
	for _, tr := range snapshot {
		backend.AddPaused(tr.Hash, tr.Name, 1, 10, tr.Completed)
	}

	start := time.Now()
	dropStalledTorrents(snapshot, backend, fm, start)
	dropStalledTorrents(snapshot, backend, fm, start.Add(3*stalledTorrentTimeout))

	if len(stalledTorrents.since) != 0 {
		t.Errorf("none of these should be on the clock, map has %d", len(stalledTorrents.since))
	}
	for _, tr := range snapshot {
		if _, ok := backend.Get(tr.Hash); !ok {
			t.Errorf("%s (status %s, peers %d) should not have been dropped", tr.Hash, tr.Status, tr.PeersTotal)
		}
	}
}

// Peers coming back reset the clock: the next stall counts from zero, not from the old stamp.
func TestDropStalledTorrents_RecoveryResetsTheClock(t *testing.T) {
	stalledTorrents.since = map[string]time.Time{}

	const hash = "6666666666666666666666666666666666666666"
	backend := torrents.NewFakeBackend()
	backend.AddPaused(hash, "comes and goes", 1, 10, false)
	fm := &lifecycleFM{configs: &files.Config{}}

	start := time.Now()
	stalled := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 0, false)}
	alive := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 5, false)}

	dropStalledTorrents(stalled, backend, fm, start)
	dropStalledTorrents(alive, backend, fm, start.Add(30*time.Minute))
	dropStalledTorrents(stalled, backend, fm, start.Add(31*time.Minute))
	dropStalledTorrents(stalled, backend, fm, start.Add(31*time.Minute+stalledTorrentTimeout-time.Minute))

	if _, ok := backend.Get(hash); !ok {
		t.Error("the clock should have restarted when the peers came back")
	}
}
