package torrents

import "testing"

const testMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"

func TestFakeBackendPauseSetsStoppedStatus(t *testing.T) {
	f := NewFakeBackend()
	hash, err := f.Add(testMagnet)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if err := f.Pause(hash); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}

	info, ok := f.Get(hash)
	if !ok {
		t.Fatal("torrent disappeared after Pause")
	}
	if info.Status != "stopped" {
		t.Errorf("Status = %q, want %q", info.Status, "stopped")
	}
}

func TestFakeBackendResumeSetsDownloadingStatus(t *testing.T) {
	f := NewFakeBackend()
	hash, _ := f.Add(testMagnet)
	_ = f.Pause(hash)

	if err := f.Resume(hash); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	info, _ := f.Get(hash)
	if info.Status != "downloading" {
		t.Errorf("Status = %q, want %q", info.Status, "downloading")
	}
}

func TestFakeBackendAnnounceRecordsCall(t *testing.T) {
	f := NewFakeBackend()
	hash, _ := f.Add(testMagnet)

	if err := f.Announce(hash); err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	calls := f.AnnounceCalls()
	if len(calls) != 1 || calls[0] != hash {
		t.Errorf("AnnounceCalls() = %v, want [%s]", calls, hash)
	}
}

func TestFakeBackendActionsOnUnknownHashError(t *testing.T) {
	f := NewFakeBackend()

	if err := f.Pause("deadbeef"); err == nil {
		t.Error("Pause on unknown hash should error")
	}
	if err := f.Resume("deadbeef"); err == nil {
		t.Error("Resume on unknown hash should error")
	}
	if err := f.Announce("deadbeef"); err == nil {
		t.Error("Announce on unknown hash should error")
	}
}
