# 7. Never-closing channel as headless tray fallback

**Location:** `cmd/daemon/main.go` — shutdown select block

**What it looks like:** When no tray manager exists, a channel is created and never closed or signaled: `c := make(chan struct{}); trayShutdownChan = c`. Looks like a leak or forgotten close.

**Why it's right:** The main goroutine selects on both `sigChan` (OS signals) and `trayShutdownChan` (tray quit). A nil channel would panic in a select. A never-closed channel simply never fires, leaving OS signals as the only exit path — which is the correct behavior for headless/server deployments where there is no tray.

**Don't "fix" by:** using a nil check before the select or replacing with a boolean flag. The channel idiom keeps the select uniform and is idiomatic Go for "this path never triggers."
