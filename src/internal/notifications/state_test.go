package notifications

import (
	"os"
	"path/filepath"
	"testing"
)

func TestState_PersistsOnlyWhatChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	t.Cleanup(ResetState)

	OpenState(path)
	UpdateState(func(s *AlertState) bool { return false })
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("no change must mean no file, stat err = %v", err)
	}

	UpdateState(func(s *AlertState) bool { s.CheckFailing.Streak = 3; return true })
	OpenState(path)
	var got int
	UpdateState(func(s *AlertState) bool { got = s.CheckFailing.Streak; return false })
	if got != 3 {
		t.Errorf("streak after reload = %d, want 3", got)
	}
}

func TestState_CorruptedFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	t.Cleanup(ResetState)
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	OpenState(path)
	UpdateState(func(s *AlertState) bool {
		if s.CheckFailing.Streak != 0 {
			t.Errorf("want an empty state, got %+v", s)
		}
		return false
	})
}
