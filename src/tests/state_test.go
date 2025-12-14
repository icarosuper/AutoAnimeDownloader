package tests

import (
	"AutoAnimeDownloader/src/daemon"
	"sync"
	"testing"
	"time"
)

// mockNotifier é um mock da interface StateNotifier para testes
type mockNotifier struct {
	mu              sync.Mutex
	notifications   []notificationCall
	notifyCallCount int
}

type notificationCall struct {
	status    daemon.Status
	lastCheck time.Time
	hasError  bool
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{
		notifications: make([]notificationCall, 0),
	}
}

func (m *mockNotifier) NotifyStateChange(status daemon.Status, lastCheck time.Time, hasError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, notificationCall{
		status:    status,
		lastCheck: lastCheck,
		hasError:  hasError,
	})
	m.notifyCallCount++
}

func (m *mockNotifier) GetNotifications() []notificationCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]notificationCall, len(m.notifications))
	copy(result, m.notifications)
	return result
}

func (m *mockNotifier) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifyCallCount
}

func (m *mockNotifier) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = make([]notificationCall, 0)
	m.notifyCallCount = 0
}

func TestNewState(t *testing.T) {
	state := daemon.NewState()
	if state == nil {
		t.Fatal("NewState() returned nil")
	}
	if state.GetStatus() != daemon.StatusStopped {
		t.Errorf("Expected initial status to be %s, got %s", daemon.StatusStopped, state.GetStatus())
	}
	if !state.GetLastCheck().IsZero() {
		t.Error("Expected initial lastCheck to be zero time")
	}
	if state.HasLastCheckError() {
		t.Error("Expected initial hasError to be false")
	}
}

func TestState_GetStatus(t *testing.T) {
	state := daemon.NewState()
	if state.GetStatus() != daemon.StatusStopped {
		t.Errorf("Expected status %s, got %s", daemon.StatusStopped, state.GetStatus())
	}
}

func TestState_SetStatus(t *testing.T) {
	state := daemon.NewState()
	notifier := newMockNotifier()
	state.SetNotifier(notifier)

	// Test setting status
	state.SetStatus(daemon.StatusRunning)
	if state.GetStatus() != daemon.StatusRunning {
		t.Errorf("Expected status %s, got %s", daemon.StatusRunning, state.GetStatus())
	}

	// Test that notification was called
	if notifier.GetCallCount() != 1 {
		t.Errorf("Expected 1 notification, got %d", notifier.GetCallCount())
	}

	notifications := notifier.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].status != daemon.StatusRunning {
		t.Errorf("Expected notification status %s, got %s", daemon.StatusRunning, notifications[0].status)
	}

	// Test that setting same status doesn't trigger notification
	state.SetStatus(daemon.StatusRunning)
	if notifier.GetCallCount() != 1 {
		t.Errorf("Expected 1 notification after setting same status, got %d", notifier.GetCallCount())
	}

	// Test setting different status
	state.SetStatus(daemon.StatusChecking)
	if state.GetStatus() != daemon.StatusChecking {
		t.Errorf("Expected status %s, got %s", daemon.StatusChecking, state.GetStatus())
	}
	if notifier.GetCallCount() != 2 {
		t.Errorf("Expected 2 notifications, got %d", notifier.GetCallCount())
	}
}

func TestState_GetLastCheck(t *testing.T) {
	state := daemon.NewState()
	if !state.GetLastCheck().IsZero() {
		t.Error("Expected initial lastCheck to be zero time")
	}
}

func TestState_SetLastCheck(t *testing.T) {
	state := daemon.NewState()
	notifier := newMockNotifier()
	state.SetNotifier(notifier)

	now := time.Now()
	state.SetLastCheck(now)

	retrieved := state.GetLastCheck()
	if !retrieved.Equal(now) {
		t.Errorf("Expected lastCheck %v, got %v", now, retrieved)
	}

	// Test that notification was called
	if notifier.GetCallCount() != 1 {
		t.Errorf("Expected 1 notification, got %d", notifier.GetCallCount())
	}
}

func TestState_GetLastCheckError(t *testing.T) {
	state := daemon.NewState()
	if state.GetLastCheckError() != nil {
		t.Error("Expected initial error to be nil")
	}
}

func TestState_SetLastCheckError(t *testing.T) {
	state := daemon.NewState()
	notifier := newMockNotifier()
	state.SetNotifier(notifier)

	err := &testError{msg: "test error"}
	state.SetLastCheckError(err)

	retrieved := state.GetLastCheckError()
	if retrieved != err {
		t.Errorf("Expected error %v, got %v", err, retrieved)
	}

	if !state.HasLastCheckError() {
		t.Error("Expected HasLastCheckError to return true")
	}

	// Test that notification was called
	if notifier.GetCallCount() != 1 {
		t.Errorf("Expected 1 notification, got %d", notifier.GetCallCount())
	}

	notifications := notifier.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(notifications))
	}
	if !notifications[0].hasError {
		t.Error("Expected notification hasError to be true")
	}

	// Test clearing error
	state.SetLastCheckError(nil)
	if state.HasLastCheckError() {
		t.Error("Expected HasLastCheckError to return false after clearing error")
	}
	if notifier.GetCallCount() != 2 {
		t.Errorf("Expected 2 notifications, got %d", notifier.GetCallCount())
	}
}

func TestState_HasLastCheckError(t *testing.T) {
	state := daemon.NewState()
	if state.HasLastCheckError() {
		t.Error("Expected initial HasLastCheckError to be false")
	}

	state.SetLastCheckError(&testError{msg: "test"})
	if !state.HasLastCheckError() {
		t.Error("Expected HasLastCheckError to be true after setting error")
	}
}

func TestState_GetAll(t *testing.T) {
	state := daemon.NewState()
	now := time.Now()
	err := &testError{msg: "test error"}

	state.SetStatus(daemon.StatusRunning)
	state.SetLastCheck(now)
	state.SetLastCheckError(err)

	status, lastCheck, hasError := state.GetAll()

	if status != daemon.StatusRunning {
		t.Errorf("Expected status %s, got %s", daemon.StatusRunning, status)
	}
	if !lastCheck.Equal(now) {
		t.Errorf("Expected lastCheck %v, got %v", now, lastCheck)
	}
	if !hasError {
		t.Error("Expected hasError to be true")
	}
}

func TestState_SetNotifier(t *testing.T) {
	state := daemon.NewState()
	notifier1 := newMockNotifier()
	notifier2 := newMockNotifier()

	// Set first notifier
	state.SetNotifier(notifier1)
	state.SetStatus(daemon.StatusRunning)
	if notifier1.GetCallCount() != 1 {
		t.Errorf("Expected notifier1 to be called 1 time, got %d", notifier1.GetCallCount())
	}
	if notifier2.GetCallCount() != 0 {
		t.Error("Expected notifier2 to not be called")
	}

	// Replace with second notifier
	state.SetNotifier(notifier2)
	state.SetStatus(daemon.StatusChecking)
	if notifier1.GetCallCount() != 1 {
		t.Errorf("Expected notifier1 to still be called 1 time, got %d", notifier1.GetCallCount())
	}
	if notifier2.GetCallCount() != 1 {
		t.Errorf("Expected notifier2 to be called 1 time, got %d", notifier2.GetCallCount())
	}
}

func TestState_ThreadSafety(t *testing.T) {
	state := daemon.NewState()
	notifier := newMockNotifier()
	state.SetNotifier(notifier)

	var wg sync.WaitGroup
	numGoroutines := 100
	operationsPerGoroutine := 10

	// Test concurrent reads and writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// Read operations
				_ = state.GetStatus()
				_ = state.GetLastCheck()
				_ = state.GetLastCheckError()
				_ = state.HasLastCheckError()
				_, _, _ = state.GetAll()

				// Write operations
				state.SetStatus(daemon.StatusRunning)
				state.SetLastCheck(time.Now())
				state.SetLastCheckError(nil)
				state.SetStatus(daemon.StatusChecking)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent
	status, lastCheck, _ := state.GetAll()
	if status != daemon.StatusChecking && status != daemon.StatusRunning {
		t.Errorf("Unexpected final status: %s", status)
	}
	if lastCheck.IsZero() {
		t.Error("Expected lastCheck to be set")
	}
	// hasError can be true or false depending on timing, that's OK (ignored with _)

	// Verify no race conditions were detected (this test should pass with -race flag)
}

func TestState_NotificationWithoutNotifier(t *testing.T) {
	state := daemon.NewState()
	// No notifier set

	// These should not panic
	state.SetStatus(daemon.StatusRunning)
	state.SetLastCheck(time.Now())
	state.SetLastCheckError(&testError{msg: "test"})

	// Verify state was updated
	if state.GetStatus() != daemon.StatusRunning {
		t.Error("Expected status to be updated")
	}
}

func TestState_NotificationContent(t *testing.T) {
	state := daemon.NewState()
	notifier := newMockNotifier()
	state.SetNotifier(notifier)

	now := time.Now()
	state.SetStatus(daemon.StatusRunning)
	state.SetLastCheck(now)
	state.SetLastCheckError(&testError{msg: "test error"})

	// Trigger another notification by setting status again
	state.SetStatus(daemon.StatusChecking)

	notifications := notifier.GetNotifications()
	if len(notifications) < 2 {
		t.Fatalf("Expected at least 2 notifications, got %d", len(notifications))
	}

	// Check last notification
	lastNotif := notifications[len(notifications)-1]
	if lastNotif.status != daemon.StatusChecking {
		t.Errorf("Expected notification status %s, got %s", daemon.StatusChecking, lastNotif.status)
	}
	if !lastNotif.lastCheck.Equal(now) {
		t.Errorf("Expected notification lastCheck %v, got %v", now, lastNotif.lastCheck)
	}
	if !lastNotif.hasError {
		t.Error("Expected notification hasError to be true")
	}
}

// testError é um tipo de erro simples para testes
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
