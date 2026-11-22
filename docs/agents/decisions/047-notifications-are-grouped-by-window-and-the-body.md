# 47. Notifications are grouped by window, and the body is escaped only when the preset is JSON

**Location:** `internal/notifications/notifications.go` — `Notify`, `Flush`, `flushEvent`, `fireBatch`, `buildBatchVars`, `jsonEscape`/`escapeVarsForJSON`/`presetIsJSON`; `NotificationsConfig.BatchWindowSeconds` in `files/filemanager.go`; `notifications.Flush()` on shutdown in `cmd/daemon/main.go`.

**What it looks like:** three things that look optional on a webhook firer: a timer queue, a synchronous `Flush()` on shutdown, and JSON escaping conditioned on a header.

**Why it's right:**

**(a) The window.** A library backfill fires one `JobOrganize` per torrent, and each one closes with a `download_completed`. Real case (`daemon.log`, 2026-08-09 17:49→18:22): 67 organizes, the first 24 went through and ntfy.sh answered **429 on the remaining 53** — the quota blew and the notifications were lost with no retry. Because the median interval between organizes was 25s, a short window does not help: measured on the same log, 10s would take 67 → 53, and 60s takes 67 → 24. That is why the default is 60s and the field is configurable.

The queue is **per event**, not global: a preset subscribed only to `download_failed` cannot receive success text, and the per-event filter that already existed (`slices.Contains(preset.Events, ...)`) still holds.

**(b) `len(items) == 1` comes out identical to ungrouped.** `buildVars` became a wrapper of `buildBatchVars` with one item. That is what makes turning grouping on not change the look of notifications for whoever receives them one at a time — and it is what the `TestNotify_SingleItemBatchMatchesUnbatched` tests pin.

**(c) `Flush()` is synchronous.** `fireBatch` normally fires in a goroutine, but on shutdown that is the same as not firing: the process exits before the goroutines run. Hence the `wait` parameter and `wg.Wait()`.

**(d) JSON escaping depends on `Content-Type`.** Of the 9 built-in presets, 6 embed `{{message}}` inside a JSON string (Discord, Slack, Telegram, Gotify, Pushover, Apprise) and 1 sends the body raw (ntfy: `body: '{{message}}'`; the Jellyfin/Plex scan presets have an empty body, which passes through). Grouping's `\n` is invalid JSON inside a string — and escaping ntfy's body would send a literal backslash-n instead of a newline. So escaping is applied **only on the body, and only when the preset declares `Content-Type: application/json`**. As a bonus that fixes a latent bug that existed before grouping: an anime with quotes or a backslash in the name already broke those 6 presets' body, and the service answered 400 with the notification vanishing (the log only says "Webhook returned error status").

The URL stays out because URL escaping has its own rules, and headers go through a `\n`/`\r` strip: a header with a newline makes `net/http` **refuse the whole request**, and ntfy's preset uses `Title: {{title}}`.

**Don't "fix" by:**
- Replacing the per-event queue with a global queue "to send even fewer messages" — breaks the preset's `Events` filter.
- Sending `{{anime_name}}`/`{{episode}}` from the first item when N > 1: the template starts lying about the other N-1.
- Always escaping, or escaping the URL/headers too: ntfy's plain-text body gets a literal backslash-n.
- Dropping `wait` from `Flush` "because a goroutine is faster".
- Adding retry with backoff on 429 hoping to solve the log case: it was a service quota, not a transient failure. What solves it is sending fewer messages.
