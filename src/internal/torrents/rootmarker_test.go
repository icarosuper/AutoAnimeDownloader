package torrents

import (
	"os"
	"path/filepath"
	"testing"
)

// moveAway simulates what the user does in a file manager: the download folder (marker
// included) is moved somewhere else and an empty folder is left at the configured path —
// exactly the shape ProbePath recreates on the next pass.
func moveAway(t *testing.T, downloadPath string) string {
	t.Helper()
	moved := downloadPath + "-moved-by-the-user"
	if err := os.Rename(downloadPath, moved); err != nil {
		t.Fatalf("simulating the move: %v", err)
	}
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("recreating the empty download folder: %v", err)
	}
	return moved
}

func readMarker(t *testing.T, path string) string {
	t.Helper()
	id, err := readRootID(path)
	if err != nil {
		t.Fatalf("reading marker %s: %v", path, err)
	}
	return id
}

// The bug this whole mechanism exists for: rain holds open file descriptors, so a folder
// moved out from under it keeps receiving the bytes while the configured path stays empty
// and every hardlink into the library fails. Ensure must notice and rebuild the session.
func TestSessionManagerDetectsRootSwap(t *testing.T) {
	m, downloadPath, _ := newTestManager(t)

	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if m.ConsumeRootSwap() {
		t.Error("a first run has no recorded root, so it must not be reported as a swap")
	}
	first := m.session
	id := readMarker(t, filepath.Join(downloadPath, RootMarkerName))
	if id == "" {
		t.Fatal("Ensure must write the root marker inside the download folder")
	}
	if got := readMarker(t, m.idPath); got != id {
		t.Errorf("recorded id = %q, want the marker's %q", got, id)
	}

	moveAway(t, downloadPath)

	created, err := m.Ensure(downloadPath)
	if err != nil {
		t.Fatalf("Ensure after the move: %v", err)
	}
	if !m.ConsumeRootSwap() {
		t.Error("a download folder that lost its marker must be reported as swapped")
	}
	if !created || m.session == first {
		t.Error("a swap must replace the session, so rain reopens the files by path and redownloads them")
	}
	if readMarker(t, filepath.Join(downloadPath, RootMarkerName)) != id {
		t.Error("the recreated folder must be re-marked with the recorded id, so the swap is reported once and not on every pass")
	}
}

// The swap is edge-triggered: the daemon reacts by clearing library records, which must not
// happen again on the passes that follow.
func TestSessionManagerRootSwapReportedOnce(t *testing.T) {
	m, downloadPath, _ := newTestManager(t)

	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	moveAway(t, downloadPath)
	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("Ensure after the move: %v", err)
	}
	if !m.ConsumeRootSwap() {
		t.Fatal("the swap must be reported on the pass that detects it")
	}

	created, err := m.Ensure(downloadPath)
	if err != nil {
		t.Fatalf("Ensure after recovery: %v", err)
	}
	if m.ConsumeRootSwap() {
		t.Error("the swap must not be reported again once the folder has been re-marked")
	}
	if created {
		t.Error("a recovered root must not keep recreating the session")
	}
}

// The marker travels with the folder, which makes the legitimate move a no-op: a user who
// moves the library AND repoints the config at the new location keeps their data.
func TestSessionManagerMovedAndRepointedIsNotASwap(t *testing.T) {
	m, pathA, pathB := newTestManager(t)

	if _, err := m.Ensure(pathA); err != nil {
		t.Fatalf("Ensure(pathA): %v", err)
	}
	if err := os.Rename(pathA, pathB); err != nil {
		t.Fatalf("moving the folder: %v", err)
	}

	created, err := m.Ensure(pathB)
	if err != nil {
		t.Fatalf("Ensure(pathB): %v", err)
	}
	if m.ConsumeRootSwap() {
		t.Error("a folder moved with its marker is the same root; nothing should be redownloaded")
	}
	if !created {
		t.Error("the save path changed, so the session must still be recreated at the new location")
	}
}

// An install upgrading from a build without markers has downloads but no recorded id. There
// is nothing to compare against, so adopting it silently is the only safe reading — the
// alternative wipes a healthy library on the first boot after the upgrade.
func TestSessionManagerFirstRunAdoptsExistingFolder(t *testing.T) {
	m, downloadPath, _ := newTestManager(t)
	if err := os.MkdirAll(filepath.Join(downloadPath, "abc123"), 0755); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if m.ConsumeRootSwap() {
		t.Error("an existing download folder with no id on record must be adopted, not treated as swapped")
	}
}

// The manual-download endpoints call Ensure too. If the flag were returned by Ensure, one of
// those calls would swallow the swap and the daemon would never clear the stale library
// records. Latching it means only an explicit Consume clears it.
func TestSessionManagerRootSwapSurvivesAnotherEnsure(t *testing.T) {
	m, downloadPath, _ := newTestManager(t)

	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	moveAway(t, downloadPath)

	// The call that detects the swap — a manual download, say — does not consume it.
	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("Ensure after the move: %v", err)
	}
	// ...and neither does the one after it.
	if _, err := m.Ensure(downloadPath); err != nil {
		t.Fatalf("second Ensure after the move: %v", err)
	}

	if !m.ConsumeRootSwap() {
		t.Error("the swap must stay latched until something explicitly consumes it")
	}
	if m.ConsumeRootSwap() {
		t.Error("consuming the swap must clear it")
	}
}
