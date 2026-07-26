//go:build !windows

package files

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIsCrossDeviceUnix(t *testing.T) {
	if !isCrossDevice(syscall.EXDEV) {
		t.Errorf("isCrossDevice(EXDEV) = false, want true")
	}
	if isCrossDevice(errors.New("some other error")) {
		t.Errorf("isCrossDevice(other) = true, want false")
	}
	if isCrossDevice(syscall.ENOENT) {
		t.Errorf("isCrossDevice(ENOENT) = true, want false")
	}
}

// exdevLink is a link func that always reports a cross-device error.
func exdevLink(oldname, newname string) error {
	return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: syscall.EXDEV}
}

func TestOrganizeCrossDeviceCleansUp(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "id")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "ep.mkv"), "x")

	lib := &organizer{fs: NewOSFileSystem(), link: exdevLink}
	_, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	})
	if err == nil {
		t.Fatalf("expected cross-device error")
	}
	if !isCrossDevice(errors.Unwrap(err)) {
		t.Errorf("error should wrap a cross-device error: %v", err)
	}
	// No orphan library folder left behind.
	if _, statErr := os.Stat(filepath.Join(completed, "A")); statErr == nil {
		t.Errorf("orphan library folder should have been cleaned up")
	}
}

func TestProbePathsCrossDevice(t *testing.T) {
	tmp := t.TempDir()
	save := filepath.Join(tmp, "save")
	completed := filepath.Join(tmp, "completed")

	lib := &organizer{fs: NewOSFileSystem(), link: exdevLink}
	if err := lib.ProbePaths(save, completed); err == nil {
		t.Fatalf("expected cross-device error from ProbePaths")
	}
	// Probe source cleaned up despite the failure.
	if _, statErr := os.Stat(filepath.Join(save, ".aad_link_probe")); statErr == nil {
		t.Errorf("probe source not cleaned up after cross-device failure")
	}
}
