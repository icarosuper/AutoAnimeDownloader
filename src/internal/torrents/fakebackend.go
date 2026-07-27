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
}

var _ TorrentBackend = (*FakeBackend)(nil)

func NewFakeBackend() *FakeBackend {
	return &FakeBackend{torrents: make(map[string]*TorrentInfo)}
}

// Ensure is a no-op for the fake (no real session to create).
func (f *FakeBackend) Ensure(savePath string) (bool, error) { return false, nil }

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
		f.torrents[hash] = &TorrentInfo{Hash: hash, Name: magnet, DataDir: "/fake/" + hash}
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
	f.torrents[hash] = &TorrentInfo{Hash: hash, DataDir: dataDir, Completed: true}
}

// CompleteTorrent marks a torrent seeding and fires the onComplete callback.
func (f *FakeBackend) CompleteTorrent(hash, dataDir string) {
	f.mu.Lock()
	if t, ok := f.torrents[hash]; ok {
		t.Completed = true
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
