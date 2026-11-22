# 17. `--debug-anime` early-exit branch in `cmd/daemon/main.go`

**Location:** `cmd/daemon/main.go` — `runDebugAnime`, the `flag.Int("debug-anime", ...)` check at the top of `main()`.

**What it looks like:** `main()` parses a flag and, if set, runs a completely different code path (`runDebugAnime`) and returns — skipping the PID file, API server, tray, and daemon loop entirely. Looks like a debug hack that snuck into production entry point.

**Why it's right:** It's a deliberate one-shot diagnostic mode (`make debug-anime ID=<anilistId>` / `go run ./src/cmd/daemon --debug-anime <id>`) for the recurring "why didn't this anime download" problem. It reuses real production functions (`daemon.RunAnimeDebug` → `checkEpisode`, `partitionSearchResults`/`pickBatches`) so the debug output can't drift from actual verification-loop behavior, and it deliberately avoids touching the torrent client so it can run without the daemon up (the torrent client is embedded, so there is nothing external to start either way). See the `debug-anime` skill (`.claude/skills/debug-anime/SKILL.md`) and `daemon/debug.go`.

**Don't "fix" by:** moving this behind the HTTP API (it exists specifically to work without a running daemon) or deleting it as dead code (it's the primary entry point for the fast-path troubleshooting flow).
