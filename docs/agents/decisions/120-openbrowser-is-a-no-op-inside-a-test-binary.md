# 120. `OpenBrowser` is a no-op inside a test binary

**Location:** `internal/daemon/helpers.go` — `OpenBrowser`, the `testing.Testing()` guard.

**What it looks like:** production code importing `testing` and checking whether it runs under
`go test`, instead of swapping the function out in the tests.

**Why it's right:** the verification pass opens the config page in a goroutine when the config is
incomplete, and several tests drive exactly that pass (e.g.
`TestAnimeVerification_IncompleteConfigStillReportsMissingConfig`). Every `go test ./...` ran
`xdg-open http://localhost:8091/#/config?missingConfig=true` in the developer's real browser. A
var-swap would need a stub in every package whose tests can reach a pass (daemon, api, …), and the
first new package that forgets it brings the tab back. `testing.Testing()` (Go 1.21+) is true only
in a binary built by `go test`, so one guard covers every caller — the pass, the first-run boot and
the tray, the CLI's `open` command — and costs nothing in the real binaries.
