package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// The crash file lifecycle: the runtime writes a fatal panic into crash.log (debug.SetCrashOutput);
// the next boot promotes a non-empty crash.log to crash-pending.log, which is what the report
// screen offers; once the user handles it, it moves to crashes/ and only the newest few are kept.
const (
	crashFileName    = "crash.log"
	pendingCrashName = "crash-pending.log"
	crashHistoryDir  = "crashes"
	crashesKept      = 5
)

// StartCrashCapture promotes the previous run's crash output to the pending slot and returns a
// fresh crash.log for debug.SetCrashOutput. An older pending crash nobody handled goes to the
// history first, so the pending one is always the latest and none is lost.
func StartCrashCapture(dir string) (*os.File, error) {
	crashPath := filepath.Join(dir, crashFileName)
	if info, err := os.Stat(crashPath); err == nil && info.Size() > 0 {
		if err := ArchivePendingCrash(dir); err != nil {
			return nil, err
		}
		if err := os.Rename(crashPath, filepath.Join(dir, pendingCrashName)); err != nil {
			return nil, fmt.Errorf("failed to promote the crash file: %w", err)
		}
	}

	f, err := os.Create(crashPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create the crash file: %w", err)
	}
	return f, nil
}

// PendingCrashAt is when the pending crash happened (the file's modification time), or nil.
func PendingCrashAt(dir string) *time.Time {
	info, err := os.Stat(filepath.Join(dir, pendingCrashName))
	if err != nil {
		return nil
	}
	at := info.ModTime()
	return &at
}

// ArchivePendingCrash moves the pending crash to crashes/ and prunes the history. No pending crash
// is not an error: the UI calls this on every handled report, and a second tab may have won.
func ArchivePendingCrash(dir string) error {
	pendingPath := filepath.Join(dir, pendingCrashName)
	info, err := os.Stat(pendingPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat the pending crash: %w", err)
	}

	historyDir := filepath.Join(dir, crashHistoryDir)
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return fmt.Errorf("failed to create the crash history folder: %w", err)
	}
	archived := filepath.Join(historyDir, "crash-"+info.ModTime().Format("20060102-150405")+".log")
	if err := os.Rename(pendingPath, archived); err != nil {
		return fmt.Errorf("failed to archive the pending crash: %w", err)
	}
	return pruneCrashHistory(historyDir)
}

func pruneCrashHistory(historyDir string) error {
	archived, err := filepath.Glob(filepath.Join(historyDir, "crash-*.log"))
	if err != nil {
		return err
	}
	// The timestamp in the name sorts chronologically.
	sort.Sort(sort.Reverse(sort.StringSlice(archived)))
	for i := crashesKept; i < len(archived); i++ {
		if err := os.Remove(archived[i]); err != nil {
			return fmt.Errorf("failed to prune the crash history: %w", err)
		}
	}
	return nil
}

func readPendingCrash(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, pendingCrashName))
	if err != nil {
		return ""
	}
	return string(data)
}
