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

func (s *State) notifyChange() (Status, time.Time, bool) {
	if s.notifier != nil {
		return s.status, s.lastCheck, s.lastCheckError != nil
	}
	return "", time.Time{}, false
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

func (s *State) SetLastCheckError(err error) {
	s.mu.Lock()
	s.lastCheckError = err
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
