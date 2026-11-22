# 15. `atomic.Pointer[Priorities]` package-level global in `nyaa`

**Location:** `internal/nyaa/priorities.go` — `active atomic.Pointer[Priorities]`, `ActivePriorities()`, `SetPriorities()`.

**What it looks like:** A mutable global swapped at runtime instead of threading a `Priorities` value/config through every sort/filter function call (`SortTorrentResults`, `SortMovieResults`, `fansubPriority`, `ShouldIgnore`, etc.). Looks like hidden state that could cause data races or surprising cross-test pollution.

**Why it's right:** The sort/filter call sites (`ScrapNyaa*`, `SortTorrentResults`, `SortMovieResults`, and a dozen small `xPriority` helpers) are deep, numerous, and have no `*Config` or context parameter today — threading one through would touch most of `nyaa.go` for a value that changes rarely (only on config save/load). `atomic.Pointer` makes reads lock-free and the swap atomic, so the daemon's verification loop (running concurrently with API requests) never observes a torn read. `SetPriorities` returns a `restore func()`, mirroring the existing `MockNyaaHttpGet` convention (decision 1) — tests `defer restore()` instead of mutating shared state permanently. Package `init()` seeds the pointer with `DefaultPriorities()` so any code that calls the sort functions without ever calling `SetPriorities` (most unit tests) still gets correct, non-nil behavior.

**Don't "fix" by:** threading a `Priorities` parameter through every nyaa function, or replacing the atomic pointer with a mutex-guarded struct. The former is a large, low-value refactor; the latter adds lock contention to a hot path (every torrent comparison during sort) for no correctness benefit over `atomic.Pointer`.
