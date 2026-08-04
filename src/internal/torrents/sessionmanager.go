package torrents

import (
	"errors"
	"fmt"
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
	// queue caps how many incomplete torrents run at once. It lives on the manager, not on
	// the Session, so it survives the session being torn down and rebuilt by Ensure.
	queue queue
}

// queue.mu is taken BEFORE m.mu (queue.enforce calls List/pause/resume). Every exported
// method below therefore releases m.mu before touching the queue — see the ordering note
// on the queue type.
var _ queueOps = (*SessionManager)(nil)

var _ TorrentBackend = (*SessionManager)(nil)

// NewSessionManager creates a manager. dbPath is the resume database location, kept stable
// across SavePath changes so resume data survives. The download-root id file lives next to
// it, for the same reason: it must stay put while the download folder moves.
func NewSessionManager(dbPath string) *SessionManager {
	m := &SessionManager{
		dbPath: dbPath,
		idPath: filepath.Join(filepath.Dir(dbPath), rootIDFileName),
	}
	// queue.json fica ao lado do banco de resume pelo mesmo motivo do download_root.id: e
	// estado do torrent client, e precisa acompanhar o banco.
	m.queue.load(filepath.Join(filepath.Dir(dbPath), queueFileName))
	return m
}

// Ensure creates the session if absent, and recreates it when savePath changed or when the
// download root was swapped. See ConsumeRootSwap for why the swap forces a new session.
func (m *SessionManager) Ensure(savePath string) (bool, error) {
	created, err := m.ensure(savePath)
	if created {
		// Troca de save_path nao zera a fila: os hashes sao os mesmos, e a poda/anexacao do
		// enforce acerta qualquer diferenca. Fora do lock, que ensure ja soltou (ver a nota
		// de ordem de lock no tipo).
		m.queue.enforce(m)
	}
	return created, err
}

func (m *SessionManager) ensure(savePath string) (bool, error) {
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
	s.SetCallbacks(m.wrapComplete(m.onComplete), m.onFailed)
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
		m.session.SetCallbacks(m.wrapComplete(onComplete), m.onFailed)
	}
}

// wrapComplete runs the queue before the caller's completion handler. A torrent finishing
// is the moment a download slot frees, so it is the queue's main trigger — the caller
// (which enqueues JobOrganize) neither knows nor needs to know about it. The raw handler
// stays in m.onComplete so Ensure can re-wrap it for each new session instead of stacking
// wrappers.
func (m *SessionManager) wrapComplete(cb func(hash string)) func(hash string) {
	return func(hash string) {
		m.queue.enforce(m)
		if cb != nil {
			cb(hash)
		}
	}
}

func (m *SessionManager) Add(magnet string) (string, error) {
	hash, err := func() (string, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		if m.session == nil {
			return "", ErrSessionNotReady
		}
		return m.session.Add(magnet)
	}()
	if err != nil {
		return "", err
	}
	m.queue.enforce(m)
	return hash, nil
}

func (m *SessionManager) List() []TorrentInfo {
	infos := m.list()
	if infos == nil {
		return nil
	}
	return m.queue.markQueued(infos)
}

// list is the raw snapshot, without the queue's status rewrite. enforce reads through it:
// going via List would re-enter queue.mu, which enforce already holds.
func (m *SessionManager) list() []TorrentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil
	}
	return m.session.List()
}

func (m *SessionManager) Get(hash string) (TorrentInfo, bool) {
	m.mu.RLock()
	var (
		info TorrentInfo
		ok   bool
	)
	if m.session != nil {
		info, ok = m.session.Get(hash)
	}
	m.mu.RUnlock()
	if !ok {
		return TorrentInfo{}, false
	}
	return m.queue.markQueued([]TorrentInfo{info})[0], true
}

func (m *SessionManager) Remove(hash string, keepData bool) error {
	m.mu.RLock()
	err := error(nil)
	if m.session == nil {
		err = ErrSessionNotReady
	} else {
		err = m.session.Remove(hash, keepData)
	}
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	m.queue.drop(hash)
	m.queue.enforce(m)
	return nil
}

// pause/resume are the raw delegations the queue drives. They deliberately do NOT touch
// the queue: a pause decided by the queue that re-entered the queue would not terminate.
func (m *SessionManager) pause(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Pause(hash)
}

func (m *SessionManager) resume(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Resume(hash)
}

func (m *SessionManager) Pause(hash string) error {
	t, ok := m.Get(hash)
	if !ok {
		return fmt.Errorf("torrent %s not found", hash)
	}
	// A completed torrent is seeding: it never held a download slot, so the queue has no
	// bookkeeping for it in either direction.
	if t.Completed {
		return m.pause(hash)
	}
	// Into `paused` first: between the pause and the bookkeeping, an enforce racing from a
	// completion callback could promote the very torrent the user is pausing.
	m.queue.markPaused(hash)
	if err := m.pause(hash); err != nil {
		return err
	}
	m.queue.enforce(m)
	return nil
}

func (m *SessionManager) Resume(hash string) error {
	t, ok := m.Get(hash)
	if !ok {
		return fmt.Errorf("torrent %s not found", hash)
	}
	// A completed torrent is seeding, not downloading — it never occupied a slot, so the
	// queue has no say in restarting it.
	if t.Completed {
		return m.resume(hash)
	}
	m.queue.pushBack(hash)
	m.queue.enforce(m)
	return nil
}

// Prioritize is the single-hash form, and it validates: an unknown or already-completed hash
// is an error here, while the batch below ignores both (see PrioritizeAll).
//
// The Get happens BEFORE delegating, never inside PrioritizeAll: Get goes through markQueued,
// which takes queue.mu — calling it from under that lock is the deadlock decision 41 already
// warns about for enforce.
func (m *SessionManager) Prioritize(hash string) error {
	t, ok := m.Get(hash)
	if !ok {
		return fmt.Errorf("torrent %s not found", hash)
	}
	if t.Completed {
		return fmt.Errorf("torrent %s already completed", hash)
	}
	return m.PrioritizeAll([]string{hash})
}

func (m *SessionManager) PrioritizeAll(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	m.queue.prioritize(hashes)
	m.queue.enforce(m)
	return nil
}

func (m *SessionManager) SetMaxActiveDownloads(n int) {
	m.queue.setLimit(n)
	m.queue.enforce(m)
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
