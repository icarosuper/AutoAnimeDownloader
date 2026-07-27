package torrents

import (
	"errors"
	"sync"

	"AutoAnimeDownloader/src/internal/logger"
)

// ErrSessionNotReady is returned by SessionManager methods when no underlying session
// exists yet (config incomplete — no SavePath).
var ErrSessionNotReady = errors.New("torrent session not ready (incomplete config)")

// SessionManager owns the embedded torrent Session's lifecycle. It implements
// TorrentBackend by delegating to the current session, so every call site injects one
// uniform backend and never sees the create/recreate logic.
//
// The daemon can start without a complete config (no SavePath), in which case no session
// exists until Ensure is first called with a valid SavePath. Changing SavePath at runtime
// closes the old session and opens a new one at the new DataDir.
//
// The mutex is an RWMutex and is held for the whole duration of every delegated call
// (Add/List/Get/Remove), not just while reading the session pointer. Releasing it early
// would let a concurrent Ensure/Close tear the session down between the read and the
// delegated call — rain's Close() clears its internal torrent map, so the delegated call
// could panic on a nil map. Ensure/Close take the write lock; the delegating readers take
// the read lock, so concurrent Add/Get/List still run in parallel.
type SessionManager struct {
	mu         sync.RWMutex
	dbPath     string
	savePath   string
	session    *Session
	onComplete func(hash string)
	onFailed   func(hash string, err error)
}

var _ TorrentBackend = (*SessionManager)(nil)

// NewSessionManager creates a manager. dbPath is the resume database location, kept stable
// across SavePath changes so resume data survives.
func NewSessionManager(dbPath string) *SessionManager {
	return &SessionManager{dbPath: dbPath}
}

// Ensure creates the session if absent, or recreates it if savePath changed. It returns
// true when a new session was created (so the caller can run reconciliation).
func (m *SessionManager) Ensure(savePath string) (bool, error) {
	if savePath == "" {
		return false, ErrSessionNotReady
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.session != nil && m.savePath == savePath {
		return false, nil
	}
	if m.session != nil {
		logger.Logger.Info().Str("old", m.savePath).Str("new", savePath).Msg("SavePath changed, recreating torrent session")
		if err := m.session.Close(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Error closing previous torrent session")
		}
		m.session = nil
	}

	s, err := NewSession(savePath, m.dbPath)
	if err != nil {
		return false, err
	}
	s.SetCallbacks(m.onComplete, m.onFailed)
	m.session = s
	m.savePath = savePath
	return true, nil
}

// SetCallbacks stores completion/failure handlers and applies them to the current session.
func (m *SessionManager) SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onComplete = onComplete
	m.onFailed = onFailed
	if m.session != nil {
		// Same as in Ensure: delegate under the lock so the session cannot be swapped or
		// closed between the pointer read and the call.
		m.session.SetCallbacks(onComplete, onFailed)
	}
}

func (m *SessionManager) Add(magnet string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return "", ErrSessionNotReady
	}
	return m.session.Add(magnet)
}

func (m *SessionManager) List() []TorrentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil
	}
	return m.session.List()
}

func (m *SessionManager) Get(hash string) (TorrentInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return TorrentInfo{}, false
	}
	return m.session.Get(hash)
}

func (m *SessionManager) Remove(hash string, keepData bool) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Remove(hash, keepData)
}

func (m *SessionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		return nil
	}
	err := m.session.Close()
	m.session = nil
	return err
}
