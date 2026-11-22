# 5. State notifier called outside the mutex lock

**Location:** `internal/daemon/state.go` — `SetStatus`, `SetLastCheck`, `SetLastCheckError`

**What it looks like:** The code snapshots `notifier` and state values while holding `s.mu`, releases the lock, then calls `notifier.NotifyStateChange(...)`. Releasing before calling looks like a race.

**Why it's right:** `NotifyStateChange` triggers WebSocket broadcasts, which may acquire their own locks. Calling it while holding `s.mu` risks deadlock if any downstream code tries to read State. Snapshotting values under lock and calling the notifier after releasing is the standard Go pattern for callbacks that must not be called under a lock.

**Don't "fix" by:** moving `NotifyStateChange` inside the `s.mu.Lock()` block. That is the deadlock.
