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
	// lastCheckReport e o relatorio do ULTIMO passe. So memoria: createStartFunc chama
	// AnimeVerification imediatamente ao iniciar (loop.go, antes do primeiro time.After),
	// entao apos um restart ele se reconstroi em segundos. Um arquivo custaria persistencia,
	// migracao e a possibilidade de mostrar um relatorio de dias atras como se fosse do ultimo
	// passe.
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

func (s *State) SetLastCheckError(err error) {
	s.mu.Lock()
	s.lastCheckError = err
	// Limpar aqui, e nao em cada saida antecipada: as sete saidas de AnimeVerification ja
	// chamam esta funcao, entao nenhuma delas precisa de linha nova. E a semantica e a certa —
	// um passe que abortou antes de olhar anime nenhum nao tem relatorio por anime, tem
	// pass_error. Consequencia deliberada: SetLastCheckError(nil) no fim do passe tambem limpa,
	// entao SetLastCheckReport TEM de vir depois dele (ver AnimeVerification e decisions.md).
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

// GetLastCheckReport devolve VALOR, nao ponteiro: o handler precisa preencher pass_error na
// resposta sem escrever no objeto compartilhado. As slices continuam sendo as mesmas, e isso e
// seguro porque um CheckReport publicado nunca e mutado — o passe seguinte publica um novo.
func (s *State) GetLastCheckReport() CheckReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckReport
}
