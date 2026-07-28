package torrents

import (
	"errors"
	"path/filepath"
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
	mu       sync.RWMutex
	dbPath   string
	idPath   string
	savePath string
	session  *Session
	// pendingSwap latches a detected root swap until the daemon consumes it.
	pendingSwap bool
	onComplete  func(hash string)
	onFailed    func(hash string, err error)
	// sessionOpts is the config the manager builds its sessions with. The zero value is
	// production; only the tests set it (see newTestManager).
	sessionOpts sessionOptions
}

var _ TorrentBackend = (*SessionManager)(nil)

// NewSessionManager creates a manager. dbPath is the resume database location, kept stable
// across SavePath changes so resume data survives. The download-root id file lives next to
// it, for the same reason: it must stay put while the download folder moves.
func NewSessionManager(dbPath string) *SessionManager {
	return &SessionManager{
		dbPath: dbPath,
		idPath: filepath.Join(filepath.Dir(dbPath), rootIDFileName),
	}
}

// Ensure creates the session if absent, and recreates it when savePath changed or when the
// download root was swapped. See ConsumeRootSwap for why the swap forces a new session.
func (m *SessionManager) Ensure(savePath string) (bool, error) {
	if savePath == "" {
		return false, ErrSessionNotReady
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	swapped, id, err := m.checkRoot(savePath)
	if err != nil {
		// The markers are a recovery aid, not a precondition: a session that cannot be
		// opened is a worse outcome than a swap that goes undetected for one pass.
		logger.Logger.Warn().Err(err).Str("save_path", savePath).
			Msg("Could not read the download root markers; skipping the swap check for this pass")
	}

	if swapped {
		m.pendingSwap = true
	}

	if m.session != nil && m.savePath == savePath && !swapped {
		return false, nil
	}
	if m.session != nil {
		if swapped {
			logger.Logger.Warn().Str("save_path", savePath).
				Msg("The download folder was moved, trashed or replaced; recreating the torrent session so the files are redownloaded at the configured path")
		} else {
			logger.Logger.Info().Str("old", m.savePath).Str("new", savePath).Msg("SavePath changed, recreating torrent session")
		}
		if err := m.session.Close(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Error closing previous torrent session")
		}
		m.session = nil
	}

	s, err := newSession(savePath, m.dbPath, m.sessionOpts)
	if err != nil {
		return false, err
	}
	s.SetCallbacks(m.onComplete, m.onFailed)
	m.session = s
	m.savePath = savePath

	m.writeRoot(savePath, id)

	return true, nil
}

// ConsumeRootSwap reports and clears the latched swap flag. See the interface docs.
func (m *SessionManager) ConsumeRootSwap() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	swapped := m.pendingSwap
	m.pendingSwap = false
	return swapped
}

// checkRoot compares the id recorded in the config folder with the one inside savePath and
// reports whether the download root was swapped. It also returns the id to persist.
//
// No expected id on record means a first run (or an upgrade from a build without the
// markers): there is nothing to compare against, so it is never a swap — an install that
// has downloads is adopted as-is instead of being wiped.
func (m *SessionManager) checkRoot(savePath string) (swapped bool, id string, err error) {
	expected, err := readRootID(m.idPath)
	if err != nil {
		return false, "", err
	}
	actual, err := readRootID(filepath.Join(savePath, RootMarkerName))
	if err != nil {
		return false, "", err
	}

	if expected == "" {
		fresh, err := newRootID()
		if err != nil {
			return false, "", err
		}
		return false, fresh, nil
	}
	// The marker travels with the folder: a user who moves the library AND repoints the
	// config at the new location finds their data intact, and nothing is redownloaded.
	return actual != expected, expected, nil
}

// writeRoot persists the id on both sides. Failures are logged, not fatal: the session is
// already up, and the worst case is that the next pass re-reports the same swap.
func (m *SessionManager) writeRoot(savePath, id string) {
	if id == "" {
		return
	}
	if err := writeRootID(m.idPath, id); err != nil {
		logger.Logger.Warn().Err(err).Str("path", m.idPath).Msg("Failed to record the download root id")
	}
	if err := writeRootID(filepath.Join(savePath, RootMarkerName), id); err != nil {
		logger.Logger.Warn().Err(err).Str("save_path", savePath).Msg("Failed to write the download root marker")
	}
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

func (m *SessionManager) Pause(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Pause(hash)
}

func (m *SessionManager) Resume(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Resume(hash)
}

func (m *SessionManager) Announce(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Announce(hash)
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
