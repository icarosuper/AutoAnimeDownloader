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

// Um torrent so e derrubado depois de stalledTorrentTimeout INTEIRO a zero peers: o primeiro
// passe so carimba a hora.
func TestDropStalledTorrents_OnlyAfterTheTimeout(t *testing.T) {
	stalledTorrents.since = map[string]time.Time{}

	const hash = "1111111111111111111111111111111111111111"
	backend := torrents.NewFakeBackend()
	backend.AddPaused(hash, "travado", 1, 10, false)
	fm := &lifecycleFM{configs: &files.Config{}}

	snapshot := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 0, false)}
	start := time.Now()

	dropStalledTorrents(snapshot, backend, fm, start)
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("o primeiro passe so carimba a hora, nao derruba")
	}

	dropStalledTorrents(snapshot, backend, fm, start.Add(stalledTorrentTimeout-time.Minute))
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("antes do limiar o torrent continua na sessao")
	}

	dropStalledTorrents(snapshot, backend, fm, start.Add(stalledTorrentTimeout))
	if _, ok := backend.Get(hash); ok {
		t.Error("depois do limiar o torrent deve sair da sessao para liberar o slot")
	}
	// Reuso do caminho de falha: os bytes ja baixados ficam (PiecesHave > 0).
	if !backend.RemovedKeepData[hash] {
		t.Error("o drop deve preservar os bytes ja baixados")
	}
}

// Zero peers so e sintoma em quem esta tentando. Enfileirado, pausado e completo ficam.
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
		t.Errorf("nenhum desses devia estar sendo cronometrado, mapa tem %d", len(stalledTorrents.since))
	}
	for _, tr := range snapshot {
		if _, ok := backend.Get(tr.Hash); !ok {
			t.Errorf("%s (status %s, peers %d) nao devia ter sido derrubado", tr.Hash, tr.Status, tr.PeersTotal)
		}
	}
}

// Peers de volta zeram o relogio: o proximo travamento conta do zero, e nao do carimbo antigo.
func TestDropStalledTorrents_RecoveryResetsTheClock(t *testing.T) {
	stalledTorrents.since = map[string]time.Time{}

	const hash = "6666666666666666666666666666666666666666"
	backend := torrents.NewFakeBackend()
	backend.AddPaused(hash, "vai e volta", 1, 10, false)
	fm := &lifecycleFM{configs: &files.Config{}}

	start := time.Now()
	stalled := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 0, false)}
	alive := []torrents.TorrentInfo{stalledFixture(hash, torrents.StatusDownloading, 5, false)}

	dropStalledTorrents(stalled, backend, fm, start)
	dropStalledTorrents(alive, backend, fm, start.Add(30*time.Minute))
	dropStalledTorrents(stalled, backend, fm, start.Add(31*time.Minute))
	dropStalledTorrents(stalled, backend, fm, start.Add(31*time.Minute+stalledTorrentTimeout-time.Minute))

	if _, ok := backend.Get(hash); !ok {
		t.Error("o relogio devia ter reiniciado quando os peers voltaram")
	}
}
