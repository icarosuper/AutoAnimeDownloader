# 25. Seeding is created at startup and is independent of the daemon loop

**Location:** `cmd/daemon/main.go` — `ensureStartupSession`, called right after `jobQueue.Start()`; `internal/daemon/verification.go` — the `backend.Ensure(configs.DownloadPath())` call inside the verification pass.

**What it looks like:** `Ensure` is called from **two** places — once at startup and once on every verification pass — which reads like a redundant call that could be dropped from one side.

**Why it's right:** The two calls serve different purposes and both are needed.

- **Startup call:** torrents keep **seeding** from the derived download directory (`Config.DownloadPath()`, nested under `completed_anime_path`) after they complete. If the session were created only by the verification pass, stopping the daemon loop from the WebUI (or booting with the loop stopped) would mean no session at all, i.e. **seeding stops** — a behavior regression against the external-qBittorrent setup, which kept seeding regardless of what the daemon was doing. That matters for private trackers with ratio requirements. Creating the session at startup makes seeding a property of the *process*, not of the loop.
- **Verification-pass call:** the daemon must be able to boot with an incomplete config (no `completed_anime_path`, and therefore no derivable download path). In that case the startup call deliberately does nothing and the session stays lazy; the pass's `Ensure` is what creates it once the user saves a config. It also handles `completed_anime_path` changing at runtime (recreate, since the derived download path moves with it) and is where the `created == true` return feeds startup reconciliation.

Ordering detail: `ensureStartupSession` runs **after** `jobQueue.Start()`. Creating the session arms the resume listeners, which can fire a completion immediately, and `Start()` loads the persisted job list *over* whatever is in memory — enqueueing before it would silently discard the job. The shutdown defers keep their LIFO order (`jobQueue.Stop()` drains organize jobs that still need the session, then `torrentManager.Close()` flushes bbolt); do not reorder them.

**Don't "fix" by:** removing the `Ensure` from the verification pass (breaks the incomplete-config boot and reacting to `completed_anime_path` changes), removing the startup call (stops seeding whenever the loop is stopped), moving startup reconciliation out of the verification pass, or calling `ensureStartupSession` before `jobQueue.Start()`.

**Amendment (see #31):** `daemon.MigrateSavePath` used to run in `main.go` between `jobQueue.Start()` and `ensureStartupSession`, and `ensureStartupSession` was skipped for the whole boot when it failed. Both are **gone** (2026-08-24, see the amendment on #31): `ensureStartupSession` now runs unconditionally right after `jobQueue.Start()`.
