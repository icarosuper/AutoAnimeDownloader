package daemon

import (
	"sync"
	"time"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusRunning  Status = "running"
	StatusChecking Status = "checking"
)

type StateNotifier interface {
	NotifyStateChange(status Status, lastCheck time.Time, hasError bool)
}

type State struct {
	mu sync.RWMutex

	status Status

	lastCheck      time.Time
	lastCheckError error
	// lastCheckReport is the LAST pass's report. Memory only: createStartFunc calls
	// AnimeVerification immediately on start (loop.go, before the first time.After),
	// so after a restart it rebuilds itself in seconds. A file would cost persistence,
	// migration, and the chance of showing a days-old report as if it were from the last
	// pass.
	lastCheckReport CheckReport

	notifier StateNotifier
}

func NewState() *State {
	return &State{
		status: StatusStopped,
	}
}

func (s *State) SetNotifier(notifier StateNotifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = notifier
}

// Should always return the current state values
// The notifier null-check should be done by caller
func (s *State) notifyChange() (Status, time.Time, bool) {
	return s.status, s.lastCheck, s.lastCheckError != nil
}

func (s *State) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *State) SetStatus(status Status) {
	s.mu.Lock()
	changed := s.status != status
	s.status = status
	notifier := s.notifier
	statusSnapshot, lastCheckSnapshot, hasErrorSnapshot := s.notifyChange()
	s.mu.Unlock()

	if changed && notifier != nil {
		notifier.NotifyStateChange(statusSnapshot, lastCheckSnapshot, hasErrorSnapshot)
	}
}

func (s *State) GetLastCheck() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheck
}

func (s *State) SetLastCheck(t time.Time) {
	s.mu.Lock()
	s.lastCheck = t
	notifier := s.notifier
	statusSnapshot, lastCheckSnapshot, hasErrorSnapshot := s.notifyChange()
	s.mu.Unlock()

	if notifier != nil {
		notifier.NotifyStateChange(statusSnapshot, lastCheckSnapshot, hasErrorSnapshot)
	}
}

func (s *State) GetLastCheckError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckError
}

// GetLastCheckErrorCode returns the classified cause of the last abort, "" when there was none.
func (s *State) GetLastCheckErrorCode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return passErrorCode(s.lastCheckError)
}

func (s *State) SetLastCheckError(err error) {
	s.mu.Lock()
	s.lastCheckError = err
	// Clear here, not on every early return: AnimeVerification's seven exits already
	// call this function, so none of them needs a new line. And the semantics are the
	// right ones — a pass that aborted before looking at any anime has no per-anime
	// report, it has pass_error. Deliberate consequence: SetLastCheckError(nil) at the
	// end of the pass also clears, so SetLastCheckReport MUST come after it (see
	// AnimeVerification and decisions.md).
	s.lastCheckReport = CheckReport{}
	notifier := s.notifier
	statusSnapshot, lastCheckSnapshot, hasErrorSnapshot := s.notifyChange()
	s.mu.Unlock()

	if notifier != nil {
		notifier.NotifyStateChange(statusSnapshot, lastCheckSnapshot, hasErrorSnapshot)
	}
}

func (s *State) HasLastCheckError() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckError != nil
}

func (s *State) GetAll() (status Status, lastCheck time.Time, hasError bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status, s.lastCheck, s.lastCheckError != nil
}

func (s *State) SetLastCheckReport(r CheckReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCheckReport = r
}

// GetLastCheckReport returns a VALUE, not a pointer: the handler needs to fill pass_error in
// the response without writing to the shared object. The slices remain the same, and that is
// safe because a published CheckReport is never mutated — the next pass publishes a new one.
func (s *State) GetLastCheckReport() CheckReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckReport
}
