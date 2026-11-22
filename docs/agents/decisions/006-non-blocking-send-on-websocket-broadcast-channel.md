# 6. Non-blocking send on WebSocket broadcast channel

**Location:** `internal/api/websocket.go` — `NotifyStateChange`

**What it looks like:** `select { case wsm.broadcast <- message: default: /* drop */ }` — silently drops messages when the channel is full.

**Why it's right:** `NotifyStateChange` is called from the daemon loop (inside `SetStatus`). If the WebSocket consumers are slow, blocking here would stall the entire verification loop. WebSocket clients are UI-only; they get eventual consistency, not strict delivery. The channel has a 256-message buffer, so drops only happen under extreme backpressure.

**Don't "fix" by:** making the send blocking or adding a retry loop. That couples daemon throughput to UI client speed.
