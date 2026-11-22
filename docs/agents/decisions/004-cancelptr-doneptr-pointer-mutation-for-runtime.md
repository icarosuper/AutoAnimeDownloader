# 4. `cancelPtr` / `donePtr` pointer mutation for runtime interval updates

**Location:** `internal/daemon/loop.go` — `StartLoop` / `UpdateInterval`

**What it looks like:** `cancelPtr := &cancel` — storing a pointer to a `context.CancelFunc` local variable, then reassigning the pointee in `UpdateInterval`. Looks like an unnecessary level of indirection.

**Why it's right:** `UpdateInterval` needs to cancel the running goroutine and start a new one with a different interval, all under the same mutex. Storing pointers to the cancel function and done channel lets `UpdateInterval` swap them atomically without exposing internal state or replacing the entire `LoopControl` struct returned to callers.

**Don't "fix" by:** removing the indirection or flattening into a channel-based command pattern. The current structure keeps `LoopControl` stable (callers hold the same pointer) while the internals are replaced underneath.
