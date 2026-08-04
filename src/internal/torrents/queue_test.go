package torrents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// queueFake is a minimal queueOps double. It is deliberately NOT the FakeBackend: the
// queue drives the unexported pause/resume, and a double that models only the status flip
// keeps the assertions about the queue's decisions, not about a fake's fidelity.
type queueFake struct {
	torrents []TorrentInfo
	pauseErr map[string]error
	// noSession makes list() return nil — "there is no session", which is NOT the same as
	// "the session has no torrents". See enforce's step 0.
	noSession bool
	calls     []string
}

// list returns a NON-NIL empty slice for an empty session. This matters more than it looks:
// nil is enforce's "no session at all" signal, so a double that returned nil here would turn
// every empty-session test into a no-op that passes while measuring nothing.
func (f *queueFake) list() []TorrentInfo {
	if f.noSession {
		return nil
	}
	return append(make([]TorrentInfo, 0, len(f.torrents)), f.torrents...)
}

func (f *queueFake) find(hash string) *TorrentInfo {
	for i := range f.torrents {
		if f.torrents[i].Hash == hash {
			return &f.torrents[i]
		}
	}
	return nil
}

func (f *queueFake) pause(hash string) error {
	if err := f.pauseErr[hash]; err != nil {
		return err
	}
	t := f.find(hash)
	if t == nil {
		return fmt.Errorf("not found: %s", hash)
	}
	f.calls = append(f.calls, "pause:"+hash)
	t.Status = StatusStopped
	return nil
}

func (f *queueFake) resume(hash string) error {
	t := f.find(hash)
	if t == nil {
		return fmt.Errorf("not found: %s", hash)
	}
	f.calls = append(f.calls, "resume:"+hash)
	t.Status = "downloading"
	return nil
}

func (f *queueFake) statuses() map[string]string {
	out := make(map[string]string, len(f.torrents))
	for _, t := range f.torrents {
		out[t.Hash] = t.Status
	}
	return out
}

func (f *queueFake) hashesWithStatus(status string) []string {
	var out []string
	for _, t := range f.torrents {
		if t.Status == status {
			out = append(out, t.Hash)
		}
	}
	sort.Strings(out)
	return out
}

// downloading builds an active torrent with the given piece progress.
func downloading(hash string, have, total uint32) TorrentInfo {
	return TorrentInfo{Hash: hash, Status: "downloading", PiecesHave: have, PiecesTotal: total}
}

func stopped(hash string) TorrentInfo {
	return TorrentInfo{Hash: hash, Status: StatusStopped, PiecesTotal: 10}
}

// withAddOrder carimba AddedAt crescente na ordem dos argumentos. AddedAt e a chave de
// ordenacao da fila, e um TorrentInfo zerado empata com todos os outros.
func withAddOrder(infos ...TorrentInfo) []TorrentInfo {
	base := time.Now()
	for i := range infos {
		infos[i].AddedAt = base.Add(time.Duration(i) * time.Second)
	}
	return infos
}

func seeding(hash string) TorrentInfo {
	return TorrentInfo{Hash: hash, Status: "seeding", Completed: true, PiecesHave: 10, PiecesTotal: 10}
}

// newQueue builds a queue with the boot latch already consumed — the default for every test
// that is not about the upgrade path.
func newQueue(limit int) *queue {
	return &queue{limit: limit}
}

func assertOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestQueueCapsActiveDownloads(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(
		downloading("a", 9, 10),
		downloading("b", 7, 10),
		downloading("c", 5, 10),
		downloading("d", 3, 10),
		downloading("e", 1, 10),
	)}
	q := newQueue(3)

	q.enforce(f)

	if got := f.hashesWithStatus("downloading"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("expected the first three of the order active, got %v", got)
	}
	if got := f.hashesWithStatus(StatusStopped); len(got) != 2 || got[0] != "d" || got[1] != "e" {
		t.Fatalf("expected d and e queued, got %v", got)
	}
	assertOrder(t, q.order, "a", "b", "c", "d", "e")
	if q.queued["d"] != 1 || q.queued["e"] != 2 {
		t.Fatalf("expected d=#1 and e=#2, got %v", q.queued)
	}
}

func TestQueueKeepsEpisodeOrderWhenTorrentsArriveOneByOne(t *testing.T) {
	// Como acontece de verdade: SessionManager.Add roda enforce a cada episodio. Dois animes
	// seguidos, 5 episodios cada — a fila tem de sair em ordem de adicao, nao invertida.
	base := time.Now()
	f := &queueFake{}
	q := newQueue(2)

	hashes := []string{"a1", "a2", "a3", "a4", "a5", "b1", "b2", "b3", "b4", "b5"}
	for i, hash := range hashes {
		info := downloading(hash, 0, 0)
		info.AddedAt = base.Add(time.Duration(i) * time.Second)
		f.torrents = append(f.torrents, info)
		q.enforce(f)
	}

	if got := f.hashesWithStatus("downloading"); len(got) != 2 || got[0] != "a1" || got[1] != "a2" {
		t.Fatalf("expected a1 and a2 downloading, got %v", got)
	}
	assertOrder(t, q.order, hashes...)
	for i, h := range hashes[2:] {
		if q.queued[h] != i+1 {
			t.Fatalf("expected %s at #%d, got %v", h, i+1, q.queued)
		}
	}
}

func TestQueuePrioritizePreempts(t *testing.T) {
	// O bug que esta spec existe para matar: com o limite cheio, priorizar nao fazia nada.
	f := &queueFake{torrents: withAddOrder(
		downloading("a", 5, 10),
		downloading("b", 5, 10),
		stopped("c"),
	)}
	q := newQueue(2)
	q.enforce(f)

	q.prioritize([]string{"c"})
	q.enforce(f)

	if f.statuses()["c"] != "downloading" {
		t.Fatalf("the prioritized torrent must start immediately, got %v", f.statuses())
	}
	// Sai o ULTIMO dos ativos em ordem de fila, nao o menos adiantado.
	if f.statuses()["b"] != StatusStopped {
		t.Fatalf("expected b (last in queue order) to be demoted, got %v", f.statuses())
	}
	assertOrder(t, q.order, "c", "a", "b")
	if q.queued["b"] != 1 {
		t.Fatalf("expected the demoted torrent at #1, got %v", q.queued)
	}
}

func TestQueuePrioritizeBatchLargerThanTheLimit(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(
		downloading("x", 5, 10),
		stopped("e1"), stopped("e2"), stopped("e3"),
	)}
	q := newQueue(2)
	q.enforce(f)

	q.prioritize([]string{"e1", "e2", "e3"})
	q.enforce(f)

	// Os `limit` primeiros iniciam, o excedente fica no TOPO da fila, em ordem.
	if got := f.hashesWithStatus("downloading"); len(got) != 2 || got[0] != "e1" || got[1] != "e2" {
		t.Fatalf("expected e1 and e2 downloading, got %v", got)
	}
	assertOrder(t, q.order, "e1", "e2", "e3", "x")
	if q.queued["e3"] != 1 || q.queued["x"] != 2 {
		t.Fatalf("expected e3=#1 and x=#2, got %v", q.queued)
	}
}

func TestQueuePrioritizeRespectsTheOrderReceived(t *testing.T) {
	// A ordem recebida manda, nao AddedAt: quem decide e o frontend, que sabe o que o usuario
	// clicou. Aqui os hashes chegam em ordem inversa a de adicao.
	f := &queueFake{torrents: withAddOrder(stopped("e1"), stopped("e2"), stopped("e3"))}
	q := newQueue(0)

	q.prioritize([]string{"e3", "e1", "e2"})
	q.enforce(f)

	assertOrder(t, q.order, "e3", "e1", "e2")
}

func TestQueuePrioritizeInsertsAHashMissingFromOrder(t *testing.T) {
	// Sessao recem-criada, nenhum enforce ainda: o hash e incompleto e nao esta em order.
	// Tratar so o caso "ja esta la" faria o priorizado sumir da fila.
	f := &queueFake{torrents: withAddOrder(downloading("a", 5, 10), stopped("new"))}
	q := newQueue(1)

	q.prioritize([]string{"new"})
	if len(q.order) != 1 || q.order[0] != "new" {
		t.Fatalf("expected the unknown hash inserted at the front, got %v", q.order)
	}

	q.enforce(f)
	if f.statuses()["new"] != "downloading" {
		t.Fatalf("expected the inserted hash to start, got %v", f.statuses())
	}
}

func TestQueueManualPauseSurvivesEnforceAndFreesASlot(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(downloading("a", 5, 10), downloading("b", 5, 10), stopped("c"))}
	q := newQueue(2)
	q.enforce(f)

	q.markPaused("a")
	_ = f.pause("a")
	q.enforce(f)

	if f.statuses()["a"] != StatusStopped {
		t.Fatalf("a manual pause must not be undone by enforce: %v", f.statuses())
	}
	// O pausado nao ocupa slot, entao o proximo da ordem sobe.
	if f.statuses()["c"] != "downloading" {
		t.Fatalf("expected c promoted into the freed slot, got %v", f.statuses())
	}
	// Manteve a posicao em order, mas sem numero na tela: pausado nunca sobe sozinho.
	assertOrder(t, q.order, "a", "b", "c")
	if _, ok := q.queued["a"]; ok {
		t.Fatalf("a paused torrent must not carry a queue position, got %v", q.queued)
	}

	// Retomar manda para o FIM.
	q.pushBack("a")
	q.enforce(f)
	assertOrder(t, q.order, "b", "c", "a")
}

func TestQueuePromotesOnCompletion(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(downloading("a", 9, 10), downloading("b", 5, 10), downloading("c", 1, 10))}
	q := newQueue(2)
	q.enforce(f)

	if f.statuses()["c"] != StatusStopped {
		t.Fatalf("expected c queued, got %v", f.statuses())
	}

	f.find("a").Status = "seeding"
	f.find("a").Completed = true
	q.enforce(f)

	if f.statuses()["c"] != "downloading" {
		t.Fatalf("expected c promoted after a completed, got %v", f.statuses())
	}
	assertOrder(t, q.order, "b", "c")
	if len(q.queued) != 0 {
		t.Fatalf("expected nothing waiting, got %v", q.queued)
	}
}

func TestQueueEnforceIsIdempotent(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(downloading("a", 5, 10), downloading("b", 5, 10), downloading("c", 5, 10))}
	q := newQueue(2)
	q.enforce(f)

	f.calls = nil
	q.enforce(f)

	if len(f.calls) != 0 {
		t.Fatalf("a second pass must not touch anything, got %v", f.calls)
	}
}

func TestQueueSeedingIsNeverTouched(t *testing.T) {
	// O teste que pega a iteracao sobre a SESSAO em vez de sobre `order`: completo nunca entra
	// em order, entao varrer a sessao pausaria todo seeder.
	f := &queueFake{torrents: withAddOrder(seeding("s1"), seeding("s2"), seeding("s3"))}
	q := newQueue(1)

	q.enforce(f)

	if len(f.calls) != 0 {
		t.Fatalf("a session of seeders must not be touched, got %v", f.calls)
	}
	if len(q.order) != 0 {
		t.Fatalf("completed torrents must stay out of the queue, got %v", q.order)
	}
}

func TestQueueSeedingDoesNotCountAgainstTheLimit(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(
		seeding("s1"), seeding("s2"), seeding("s3"),
		downloading("a", 1, 10), downloading("b", 2, 10),
	)}
	q := newQueue(2)

	q.enforce(f)

	if got := f.hashesWithStatus(StatusStopped); len(got) != 0 {
		t.Fatalf("seeding torrents must not push downloads out of the queue, got %v stopped", got)
	}
}

func TestQueueStoppingIsNeitherPausedNorResumed(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(
		TorrentInfo{Hash: "a", Status: StatusStopping, PiecesTotal: 10},
		TorrentInfo{Hash: "b", Status: StatusStopping, PiecesTotal: 10},
	)}
	q := newQueue(1)

	q.enforce(f)

	if len(f.calls) != 0 {
		t.Fatalf("a stopping torrent must be left alone until it settles, got %v", f.calls)
	}
}

func TestQueueWithoutASessionPrunesNothingAndSavesNothing(t *testing.T) {
	// A regressao mais cara possivel — a fila persistida sumindo em todo boot — e a mais
	// silenciosa, porque tudo continua "funcionando".
	path := filepath.Join(t.TempDir(), "queue.json")
	q := &queue{limit: 2, order: []string{"a", "b"}, paused: []string{"a"}, path: path}

	q.enforce(&queueFake{noSession: true})

	assertOrder(t, q.order, "a", "b")
	assertOrder(t, q.paused, "a")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("enforce without a session must not write the queue file")
	}
}

func TestQueueEmptySessionPrunesNormally(t *testing.T) {
	q := &queue{limit: 2, order: []string{"a"}, paused: []string{"a"}}

	q.enforce(&queueFake{})

	if len(q.order) != 0 || len(q.paused) != 0 {
		t.Fatalf("a genuinely empty session must prune, got order=%v paused=%v", q.order, q.paused)
	}
}

func TestQueueUnlimitedRunsEverythingButThePaused(t *testing.T) {
	f := &queueFake{torrents: withAddOrder(stopped("a"), stopped("b"), stopped("c"))}
	q := newQueue(0)
	q.markPaused("c")
	q.order = []string{"a", "b", "c"}

	q.enforce(f)

	if got := f.hashesWithStatus(StatusStopped); len(got) != 1 || got[0] != "c" {
		t.Fatalf("limit 0 must run everything but the manually paused, got %v stopped", got)
	}
	// Sem espera, ninguem tem posicao para numerar.
	if len(q.queued) != 0 {
		t.Fatalf("expected no positions with an unlimited queue, got %v", q.queued)
	}
	// order continua sendo mantido, para quando o limite voltar.
	assertOrder(t, q.order, "a", "b", "c")
}

func TestQueueFailedPauseDropsThePositionButKeepsTheOrder(t *testing.T) {
	f := &queueFake{
		torrents: withAddOrder(downloading("a", 9, 10), downloading("b", 1, 10)),
		pauseErr: map[string]error{"b": fmt.Errorf("boom")},
	}
	q := newQueue(1)

	q.enforce(f)

	if _, ok := q.queued["b"]; ok {
		t.Fatalf("a failed pause must not leave the row showing a queue position, got %v", q.queued)
	}
	assertOrder(t, q.order, "a", "b")
	if f.statuses()["b"] != "downloading" {
		t.Fatalf("expected b to stay running after the failed pause, got %v", f.statuses())
	}
}

func TestQueueMarkQueuedWritesSlugAndPosition(t *testing.T) {
	q := &queue{queued: map[string]int{"b": 1, "c": 2}}
	got := q.markQueued([]TorrentInfo{
		{Hash: "a", Status: "downloading"},
		{Hash: "b", Status: StatusStopped},
		{Hash: "c", Status: StatusStopped},
		{Hash: "d", Status: StatusStopped}, // pausado pelo usuario: fica stopped, sem posicao
	})

	if got[0].Status != "downloading" || got[0].QueuePosition != 0 {
		t.Fatalf("an active torrent keeps its status and gets no position, got %+v", got[0])
	}
	if got[1].Status != StatusQueued || got[1].QueuePosition != 1 {
		t.Fatalf("expected queued #1, got %+v", got[1])
	}
	if got[2].QueuePosition != 2 {
		t.Fatalf("expected queued #2, got %+v", got[2])
	}
	if got[3].Status != StatusStopped || got[3].QueuePosition != 0 {
		t.Fatalf("a paused torrent must report stopped with no position, got %+v", got[3])
	}
}

func TestQueuePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	f := &queueFake{torrents: withAddOrder(stopped("a"), stopped("b"), stopped("c"))}

	q := &queue{limit: 1, path: path}
	q.markPaused("b")
	q.enforce(f)

	var reloaded queue
	reloaded.load(path)
	assertOrder(t, reloaded.order, "a", "b", "c")
	assertOrder(t, reloaded.paused, "b")
	if reloaded.seedPaused {
		t.Fatalf("an existing queue file must not arm the seeding latch")
	}
}

func TestQueueLoadMissingFileArmsTheLatch(t *testing.T) {
	var q queue
	q.load(filepath.Join(t.TempDir(), "queue.json"))
	if !q.seedPaused {
		t.Fatalf("a missing queue file must arm the seeding latch")
	}
}

func TestQueueLoadCorruptedFileDoesNotPropagate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	var q queue
	q.load(path)

	if len(q.order) != 0 || len(q.paused) != 0 {
		t.Fatalf("a corrupted file must yield an empty queue, got %v / %v", q.order, q.paused)
	}
}

func TestQueueUpgradeBootSeedsPausedInsteadOfResuming(t *testing.T) {
	// O boot do upgrade: sem queue.json, os torrents que a rain reporta parados foram parados
	// por alguem (o usuario ou a fila antiga, que era so memoria). Congelar e a troca certa.
	path := filepath.Join(t.TempDir(), "queue.json")
	f := &queueFake{torrents: withAddOrder(stopped("a"), stopped("b"), downloading("c", 1, 10))}

	q := &queue{limit: 5}
	q.load(path) // arquivo ausente: arma o latch

	q.enforce(f)

	if got := f.hashesWithStatus(StatusStopped); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("the stopped torrents must stay stopped, got %v", got)
	}
	assertOrder(t, q.paused, "a", "b")

	// Latch de uma vez so: um Play depois disso nao e desfeito pelo passe seguinte.
	q.pushBack("a")
	q.enforce(f)
	if f.statuses()["a"] != "downloading" {
		t.Fatalf("expected a to start after the user resumed it, got %v", f.statuses())
	}
}

func TestQueueSaveOnlyWhenChangedAndRetriesAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	f := &queueFake{torrents: withAddOrder(downloading("a", 1, 10))}

	q := &queue{limit: 5, path: path}
	q.enforce(f)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the queue file to be written: %v", err)
	}

	// Nada mudou: a segunda passada nao regrava (o arquivo apagado continua apagado).
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	q.enforce(f)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("an unchanged queue must not be rewritten")
	}
	// Falha de escrita nao atualiza lastSaved, entao a gravacao seguinte com o MESMO estado
	// tenta de novo — senao uma falha transitoria congelaria o arquivo para sempre.
	q.path = filepath.Join(dir, "missing-dir", "queue.json")
	f.torrents = append(f.torrents, withAddOrder(downloading("b", 1, 10))...)
	q.enforce(f)

	q.path = path
	q.enforce(f)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the retry to write the queue file: %v", err)
	}
}
