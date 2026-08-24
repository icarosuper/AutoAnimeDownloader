package torrents

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// newTestManager returns a SessionManager whose resume database lives in a temp dir, plus a
// pair of distinct save paths to exercise the recreate-on-change branch.
//
// The sessions it opens are real rain sessions, with one deviation: DHT is off. It is the
// only thing that binds a fixed port (7246/udp), so leaving it on made every test here fail
// with "address already in use" while the daemon was running — and put the test run on the
// network. Nothing these tests assert on (session lifecycle, root markers, delegation)
// involves peer discovery.
func newTestManager(t *testing.T) (*SessionManager, string, string) {
	t.Helper()
	base := t.TempDir()
	m := NewSessionManager(filepath.Join(base, "session.db"))
	m.sessionOpts = sessionOptions{disableDHT: true}
	t.Cleanup(func() { _ = m.Close() })
	return m, filepath.Join(base, "downloads-a"), filepath.Join(base, "downloads-b")
}

// Lazy creation: the daemon boots with an incomplete config (no library), so no session
// exists and every delegated call reports ErrSessionNotReady instead of panicking.
func TestSessionManagerLazyCreation(t *testing.T) {
	m, _, _ := newTestManager(t)

	created, err := m.Ensure("")
	if created {
		t.Error("Ensure(\"\") must not create a session")
	}
	if !errors.Is(err, ErrSessionNotReady) {
		t.Errorf("Ensure(\"\") = %v, want ErrSessionNotReady", err)
	}
	if m.session != nil {
		t.Fatal("no session should exist after Ensure(\"\")")
	}

	if _, err := m.Add("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"); !errors.Is(err, ErrSessionNotReady) {
		t.Errorf("Add without session = %v, want ErrSessionNotReady", err)
	}
	if err := m.Remove("0123456789abcdef0123456789abcdef01234567", false); !errors.Is(err, ErrSessionNotReady) {
		t.Errorf("Remove without session = %v, want ErrSessionNotReady", err)
	}
	if got := m.List(); got != nil {
		t.Errorf("List without session = %v, want nil", got)
	}
	if _, ok := m.Get("0123456789abcdef0123456789abcdef01234567"); ok {
		t.Error("Get without session should report not found")
	}
	// Closing a manager that never had a session is a no-op, not an error.
	if err := m.Close(); err != nil {
		t.Errorf("Close without session = %v, want nil", err)
	}
}

// Ensure is idempotent for the same save path and recreates the session when it changes
// (the resume database path stays put, so resume data survives the swap).
func TestSessionManagerEnsureCreatesReusesAndRecreates(t *testing.T) {
	m, pathA, pathB := newTestManager(t)

	created, err := m.Ensure(pathA)
	if err != nil {
		t.Fatalf("Ensure(pathA): %v", err)
	}
	if !created {
		t.Error("first Ensure should report that it created the session (caller runs reconciliation)")
	}
	first := m.session
	if first == nil {
		t.Fatal("session should exist after Ensure")
	}

	created, err = m.Ensure(pathA)
	if err != nil {
		t.Fatalf("second Ensure(pathA): %v", err)
	}
	if created {
		t.Error("Ensure with an unchanged save path must not recreate the session")
	}
	if m.session != first {
		t.Error("Ensure with an unchanged save path must keep the same session instance")
	}

	created, err = m.Ensure(pathB)
	if err != nil {
		t.Fatalf("Ensure(pathB): %v", err)
	}
	if !created {
		t.Error("changing the save path must recreate the session")
	}
	if m.session == first {
		t.Error("changing the save path must replace the session instance")
	}
	if m.savePath != pathB {
		t.Errorf("savePath = %q, want %q", m.savePath, pathB)
	}
}

// Callbacks registered before the session exists must be applied when it is created — that
// is the whole point of registering them once in main() at startup.
func TestSessionManagerCallbacksAppliedOnCreate(t *testing.T) {
	m, pathA, _ := newTestManager(t)

	m.SetCallbacks(func(string) {}, func(string, error) {})

	if _, err := m.Ensure(pathA); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	m.session.mu.RLock()
	onComplete, onFailed := m.session.onComplete, m.session.onFailed
	m.session.mu.RUnlock()
	if onComplete == nil || onFailed == nil {
		t.Error("callbacks stored before the session existed must be applied on creation")
	}
}

// Close tears the session down and returns the manager to the not-ready state; a second
// Close is a no-op.
func TestSessionManagerClose(t *testing.T) {
	m, pathA, _ := newTestManager(t)

	if _, err := m.Ensure(pathA); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if m.session != nil {
		t.Error("Close must drop the session pointer")
	}
	if _, err := m.Add("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"); !errors.Is(err, ErrSessionNotReady) {
		t.Errorf("Add after Close = %v, want ErrSessionNotReady", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}

	// The manager is reusable: Ensure after Close creates a fresh session.
	created, err := m.Ensure(pathA)
	if err != nil || !created {
		t.Errorf("Ensure after Close = (%v, %v), want (true, nil)", created, err)
	}
}

// Delegated calls hold the lock for the whole call, so a concurrent Ensure/Close can never
// swap the session out from under one of them. Run this with -race to exercise the window
// that existed when current() released the lock before delegating.
func TestSessionManagerConcurrentEnsureAndDelegation(t *testing.T) {
	m, pathA, pathB := newTestManager(t)

	if _, err := m.Ensure(pathA); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				m.List()
				m.Get("0123456789abcdef0123456789abcdef01234567")
				_ = m.Remove("0123456789abcdef0123456789abcdef01234567", false)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 5; j++ {
			if _, err := m.Ensure(pathB); err != nil {
				t.Errorf("Ensure(pathB): %v", err)
				return
			}
			if _, err := m.Ensure(pathA); err != nil {
				t.Errorf("Ensure(pathA): %v", err)
				return
			}
		}
	}()
	wg.Wait()
}
