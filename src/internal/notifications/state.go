package notifications

import (
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// StateFileName is the data-dir file that remembers what the alert events already said, so a state
// that holds on every pass ("checks are failing") is announced once — and a restart neither
// repeats the push nor forgets to send the recovery. Disposable: deleting it costs at most one
// round of repeated notifications.
const StateFileName = "notification_state"

// AlertState is the content of notification_state. Each alert event owns its fields; the daemon
// reads and changes them through UpdateState.
type AlertState struct {
	CheckFailing CheckFailingState `json:"check_failing"`
}

// CheckFailingState tracks a run of aborted passes: since when, how many in a row, and whether
// check_failing already went out for this run (which is what makes the next finished pass send
// the recovery).
type CheckFailingState struct {
	Since  time.Time `json:"since,omitzero"`
	Code   string    `json:"code,omitempty"`
	Streak int       `json:"streak,omitempty"`
	Fired  bool      `json:"fired,omitempty"`
}

var (
	stateMu   sync.Mutex
	statePath string // "" = memory only (tests, or before OpenState)
	state     AlertState
)

// OpenState points the store at its file and loads it. A missing file is a fresh start; a corrupted
// one is a fresh start with a warning — never fatal, same rule as queue.json.
func OpenState(path string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	statePath = path
	state = AlertState{}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err == nil {
		err = json.Unmarshal(data, &state)
	}
	if err != nil {
		state = AlertState{}
		logger.Logger.Warn().Err(err).Str("path", path).Msg("Failed to read the notification state; starting empty (some alerts may repeat once)")
	}
}

// ResetState empties the store and detaches it from any file. For tests.
func ResetState() {
	stateMu.Lock()
	defer stateMu.Unlock()
	statePath = ""
	state = AlertState{}
}

// UpdateState runs fn on the state under the store's mutex and writes the file when fn reports a
// change. Notify never touches the store, so calling it from fn is safe; the daemon still sends
// after UpdateState returns, to keep the lock short.
func UpdateState(fn func(s *AlertState) (changed bool)) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if !fn(&state) || statePath == "" {
		return
	}
	if err := writeState(statePath, state); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to save the notification state")
	}
}

func writeState(path string, s AlertState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to rename %s: %w", tmp, err)
	}
	return nil
}
