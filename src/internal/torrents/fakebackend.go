package torrents

import (
	"fmt"
	"sync"
)

// FakeBackend is an in-memory TorrentBackend for tests. It replaces the qBittorrent mock
// server: no HTTP, no goroutines, deterministic. Tests drive completion explicitly via
// CompleteTorrent / FailTorrent.
type FakeBackend struct {
	mu         sync.Mutex
	torrents   map[string]*TorrentInfo
	onComplete func(hash string)
	onFailed   func(hash string, err error)

	// AddErr, if set, is returned by Add (to simulate rejection).
	AddErr error
	// NextHash overrides the hash returned by Add for the next call (for magnets whose
	// hash the test does not control). When empty, the hash is derived from the magnet.
	NextHash string
	// announceCalls records every Announce(hash) for assertions.
	announceCalls []string
	// ensureCalls records every Ensure(savePath) for assertions.
	ensureCalls []string
}

var _ TorrentBackend = (*FakeBackend)(nil)

func NewFakeBackend() *FakeBackend {
	return &FakeBackend{torrents: make(map[string]*TorrentInfo)}
}

// Ensure is a no-op for the fake (no real session to create), but records the path so tests
// can assert which save path a caller opened the session at — MigrateSavePath opens the OLD
// path before moving the data, and that ordering is the whole point of the migration.
func (f *FakeBackend) Ensure(savePath string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCalls = append(f.ensureCalls, savePath)
	return false, nil
}

// EnsureCalls returns the save paths passed to Ensure, in order.
func (f *FakeBackend) EnsureCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ensureCalls...)
}

func (f *FakeBackend) Add(magnet string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.AddErr != nil {
		return "", f.AddErr
	}
	hash := f.NextHash
	f.NextHash = ""
	if hash == "" {
		var err error
		hash, err = parseInfoHash(magnet)
		if err != nil {
			return "", fmt.Errorf("fake: invalid magnet: %w", err)
		}
	}
	if _, ok := f.torrents[hash]; !ok {
		f.torrents[hash] = &TorrentInfo{
			Hash:    hash,
			Name:    magnet,
			DataDir: "/fake/" + hash,
			Status:  "downloading",
		}
	}
	return hash, nil
}

func (f *FakeBackend) List() []TorrentInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]TorrentInfo, 0, len(f.torrents))
	for _, t := range f.torrents {
		out = append(out, *t)
	}
	return out
}

func (f *FakeBackend) Get(hash string) (TorrentInfo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return TorrentInfo{}, false
	}
	return *t, true
}

// Remove drops the torrent. Removing a hash that is not in the session is NOT an error,
// matching the real Session.Remove: rain's Session.RemoveTorrent looks the id up in its
// map and returns (nil, nil) when it is absent, so the error is nil. Returning an error
// here would make tests assert on behaviour production never produces.
func (f *FakeBackend) Remove(hash string, keepData bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.torrents, hash)
	return nil
}

func (f *FakeBackend) Pause(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	t.Status = "stopped"
	return nil
}

func (f *FakeBackend) Resume(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	t.Status = "downloading"
	return nil
}

func (f *FakeBackend) Announce(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.torrents[hash]; !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	f.announceCalls = append(f.announceCalls, hash)
	return nil
}

// AnnounceCalls returns the hashes passed to Announce, in order.
func (f *FakeBackend) AnnounceCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.announceCalls...)
}

func (f *FakeBackend) SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error)) {
	f.mu.Lock()
	f.onComplete = onComplete
	f.onFailed = onFailed
	f.mu.Unlock()
}

func (f *FakeBackend) Close() error { return nil }

// --- test drivers ---

// AddCompleted injects an already-seeding torrent with a given data dir (for reconciliation tests).
func (f *FakeBackend) AddCompleted(hash, dataDir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrents[hash] = &TorrentInfo{Hash: hash, DataDir: dataDir, Completed: true, Status: "seeding"}
}

// AddPaused injects a torrent already in the paused ("stopped") state, with piece counts and
// Completed set directly — independent of Status. This is the state real Pause() leaves
// behind on a finished torrent: rain frees Bytes.Completed on Stop, but Completed is
// piece-derived and the piece bitfield survives, so a paused-but-complete torrent still
// reports Completed: true. Tests use this to build that fixture without needing a real
// Pause() call (which this fake models only as a status flip, per Pause/Resume's existing
// semantics — see TestFakeBackendPauseSetsStoppedStatus).
func (f *FakeBackend) AddPaused(hash, name string, piecesHave, piecesTotal uint32, completed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrents[hash] = &TorrentInfo{
		Hash:        hash,
		Name:        name,
		Status:      "stopped",
		PiecesHave:  piecesHave,
		PiecesTotal: piecesTotal,
		Completed:   completed,
	}
}

// CompleteTorrent marks a torrent seeding and fires the onComplete callback.
func (f *FakeBackend) CompleteTorrent(hash, dataDir string) {
	f.mu.Lock()
	if t, ok := f.torrents[hash]; ok {
		t.Completed = true
		t.Status = "seeding"
		if dataDir != "" {
			t.DataDir = dataDir
		}
	}
	cb := f.onComplete
	f.mu.Unlock()
	if cb != nil {
		cb(hash)
	}
}

// FailTorrent fires the onFailed callback.
func (f *FakeBackend) FailTorrent(hash string, err error) {
	f.mu.Lock()
	cb := f.onFailed
	f.mu.Unlock()
	if cb != nil {
		cb(hash, err)
	}
}
