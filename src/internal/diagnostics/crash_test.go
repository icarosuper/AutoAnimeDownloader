package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func startCapture(t *testing.T, dir string) {
	t.Helper()
	f, err := StartCrashCapture(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestStartCrashCapturePromotesANonEmptyCrash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, crashFileName), "panic: boom")

	startCapture(t, dir)

	if got := readPendingCrash(dir); got != "panic: boom" {
		t.Fatalf("pending crash = %q", got)
	}
	if info, _ := os.Stat(filepath.Join(dir, crashFileName)); info == nil || info.Size() != 0 {
		t.Fatal("expected a fresh, empty crash.log")
	}
}

func TestStartCrashCaptureIgnoresAnEmptyCrashFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, crashFileName), "")

	startCapture(t, dir)

	if PendingCrashAt(dir) != nil {
		t.Fatal("an empty crash.log is not a crash")
	}
}

func TestStartCrashCaptureArchivesAnOlderPendingCrash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, pendingCrashName), "panic: first")
	writeFile(t, filepath.Join(dir, crashFileName), "panic: second")

	startCapture(t, dir)

	if got := readPendingCrash(dir); got != "panic: second" {
		t.Fatalf("pending crash = %q, want the latest", got)
	}
	archived, _ := filepath.Glob(filepath.Join(dir, crashHistoryDir, "crash-*.log"))
	if len(archived) != 1 {
		t.Fatalf("archived = %v, want the first crash", archived)
	}
}

func TestArchivePendingCrashKeepsTheNewestFive(t *testing.T) {
	dir := t.TempDir()
	historyDir := filepath.Join(dir, crashHistoryDir)
	os.MkdirAll(historyDir, 0755)
	for i := 1; i <= 5; i++ {
		writeFile(t, filepath.Join(historyDir, fmt.Sprintf("crash-2026010%d-000000.log", i)), "old")
	}
	pending := filepath.Join(dir, pendingCrashName)
	writeFile(t, pending, "panic: new")
	at := time.Date(2026, 10, 14, 12, 30, 0, 0, time.Local)
	os.Chtimes(pending, at, at)

	if err := ArchivePendingCrash(dir); err != nil {
		t.Fatal(err)
	}

	archived, _ := filepath.Glob(filepath.Join(historyDir, "crash-*.log"))
	if len(archived) != crashesKept {
		t.Fatalf("history has %d files, want %d", len(archived), crashesKept)
	}
	if _, err := os.Stat(filepath.Join(historyDir, "crash-20261014-123000.log")); err != nil {
		t.Fatal("the new crash should be named after its time")
	}
	if _, err := os.Stat(filepath.Join(historyDir, "crash-20260101-000000.log")); !os.IsNotExist(err) {
		t.Fatal("the oldest crash should be pruned")
	}
	if PendingCrashAt(dir) != nil {
		t.Fatal("the pending crash should be gone")
	}
}

func TestArchivePendingCrashWithNothingPendingIsANoOp(t *testing.T) {
	if err := ArchivePendingCrash(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
