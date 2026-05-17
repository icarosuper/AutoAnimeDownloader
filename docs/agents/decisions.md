# Architectural Decisions

Patterns that look wrong but are intentional. Read before "fixing" anything.

---

### 1. Var-swap mocking for anilist and nyaa HTTP clients

**What it looks like:** Package-level `var httpDo` / `var httpGet` function variables instead of an `HTTPClient` interface.

**Why it's right:** `anilist` and `nyaa` are thin HTTP wrappers with no branching behavior — there's nothing to mock except the HTTP call itself. The var-swap pattern avoids creating a whole interface for one method, keeps zero test boilerplate, and the `MockXxx(fn) restore` helper makes tests self-cleaning.

**Don't "fix" by:** introducing `HTTPClient` interfaces in these packages. Interface injection is reserved for components that have real behavioral variation (see `FileManagerInterface`, torrent client).

---

### 2. `FileManagerInterface` declared twice

**What it looks like:** The same interface exists in both `internal/daemon/helpers.go` and `internal/api/server.go` instead of a shared `interfaces` package.

**Why it's right:** `api` already imports `daemon` (for `*State`, `LoopControl`, etc.). If `daemon` imported `api` (or a shared package that `api` also imports transitively), the import graph would cycle. Duplicating the interface keeps each package self-contained and Go's type system satisfies both interfaces structurally — no explicit coupling needed.

**Don't "fix" by:** extracting to a shared `interfaces` or `types` package without first tracing the full import graph. A seemingly neutral extraction will break the build.

---

### 3. JSONL format for `episodes.json` with full-file rewrite on save

**What it looks like:** The file is JSONL (one JSON object per line) but `saveEpisodesToFileJSON` rewrites the entire file, not just appends new lines.

**Why it's right:** JSONL allows line-by-line parsing and provides backward compatibility with the old plain-text episode format (fallback parser in `parser.go`). Full rewrite on save is intentional: it avoids partial-write corruption — if a true append fails mid-write, the file is left in a mixed state. Read-modify-write with `WriteFile` is atomic at the OS level on the platforms we target.

**Don't "fix" by:** switching to true file-append (`os.O_APPEND`) — that breaks deduplication logic and leaves no room for deletions. Don't switch to a binary format — JSONL is human-readable for debugging.

---

### 4. `cancelPtr` / `donePtr` pointer mutation for runtime interval updates

**Location:** `internal/daemon/loop.go` — `StartLoop` / `UpdateInterval`

**What it looks like:** `cancelPtr := &cancel` — storing a pointer to a `context.CancelFunc` local variable, then reassigning the pointee in `UpdateInterval`. Looks like an unnecessary level of indirection.

**Why it's right:** `UpdateInterval` needs to cancel the running goroutine and start a new one with a different interval, all under the same mutex. Storing pointers to the cancel function and done channel lets `UpdateInterval` swap them atomically without exposing internal state or replacing the entire `LoopControl` struct returned to callers.

**Don't "fix" by:** removing the indirection or flattening into a channel-based command pattern. The current structure keeps `LoopControl` stable (callers hold the same pointer) while the internals are replaced underneath.

---

### 5. State notifier called outside the mutex lock

**Location:** `internal/daemon/state.go` — `SetStatus`, `SetLastCheck`, `SetLastCheckError`

**What it looks like:** The code snapshots `notifier` and state values while holding `s.mu`, releases the lock, then calls `notifier.NotifyStateChange(...)`. Releasing before calling looks like a race.

**Why it's right:** `NotifyStateChange` triggers WebSocket broadcasts, which may acquire their own locks. Calling it while holding `s.mu` risks deadlock if any downstream code tries to read State. Snapshotting values under lock and calling the notifier after releasing is the standard Go pattern for callbacks that must not be called under a lock.

**Don't "fix" by:** moving `NotifyStateChange` inside the `s.mu.Lock()` block. That is the deadlock.

---

### 6. Non-blocking send on WebSocket broadcast channel

**Location:** `internal/api/websocket.go` — `NotifyStateChange`

**What it looks like:** `select { case wsm.broadcast <- message: default: /* drop */ }` — silently drops messages when the channel is full.

**Why it's right:** `NotifyStateChange` is called from the daemon loop (inside `SetStatus`). If the WebSocket consumers are slow, blocking here would stall the entire verification loop. WebSocket clients are UI-only; they get eventual consistency, not strict delivery. The channel has a 256-message buffer, so drops only happen under extreme backpressure.

**Don't "fix" by:** making the send blocking or adding a retry loop. That couples daemon throughput to UI client speed.

---

### 7. Never-closing channel as headless tray fallback

**Location:** `cmd/daemon/main.go` — shutdown select block

**What it looks like:** When no tray manager exists, a channel is created and never closed or signaled: `c := make(chan struct{}); trayShutdownChan = c`. Looks like a leak or forgotten close.

**Why it's right:** The main goroutine selects on both `sigChan` (OS signals) and `trayShutdownChan` (tray quit). A nil channel would panic in a select. A never-closed channel simply never fires, leaving OS signals as the only exit path — which is the correct behavior for headless/server deployments where there is no tray.

**Don't "fix" by:** using a nil check before the select or replacing with a boolean flag. The channel idiom keeps the select uniform and is idiomatic Go for "this path never triggers."

---

### 8. Hard part filter — nil-part torrents rejected when requestedPart is set

**Location:** `internal/nyaa/nyaa.go` — `ScrapNyaa`, `ScrapNyaaForBatch`, `ScrapNyaaForMultipleEpisodes`

**What it looks like:** When `requestedPart != nil`, torrents whose name has no part marker are rejected, even though they might be the right episode. Looks overly strict.

**Why it's right:** Split-season animes (e.g. Mushoku Tensei II Part 1 / Part 2) have the same episode numbers in both halves. Without a hard part filter, Part 1 torrents would be downloaded for Part 2 entries. A torrent with no part marker in its name is ambiguous and must be treated as the wrong release when the caller knows it wants a specific part. The false-negative cost (missing a valid torrent) is lower than the false-positive cost (wrong episode downloaded silently).

**Don't "fix" by:** accepting nil-part torrents as "probably correct." The offset fallback (see decision 9) handles the rare case where no part-marked torrent exists.

---

### 9. PREQUEL offset gate requires part ≥ 2

**Location:** `internal/daemon/helpers.go` — `ComputeEpisodeOffset`

**What it looks like:** `ComputeEpisodeOffset` always returns 0 unless `part >= 2`, even when a PREQUEL relation exists. Looks like a missing case.

**Why it's right:** The offset (PREQUEL episode count) is only needed for split-season Part 2 releases where the torrent site numbers episodes globally (e.g. episode 14 on Nyaa when Anilist calls it episode 1 of Part 2). Kaguya-sama Season 2, for example, also has a PREQUEL relation with 12 episodes — applying a +12 offset there would search for episode 13 when the correct episode is 1. The gate `part >= 2` restricts the offset to the specific case it was designed for, preventing all single-season animes with prior seasons from receiving a wrong offset.

**Don't "fix" by:** removing the gate or applying the offset whenever a PREQUEL exists. That breaks every multi-season anime that does not use split-part numbering.

---

### 10. "Cour N" treated as Part N, not as a distinct concept

**Location:** `internal/nyaa/nyaa_regex.go` — `rePartPatterns`; `internal/daemon/helpers.go` — `ExtractAnimeSeasonPart`

**What it looks like:** `Cour 2` is extracted by `ExtractPart`, not by a separate cour field. Looks like it conflates two different concepts.

**Why it's right:** Anilist uses "Cour" and "Part" interchangeably for split seasons (e.g. NieR:Automata Ver1.1a Cour 2). On Nyaa, release groups also use both terms for the same split. A separate cour field would require all search/filter logic to handle two parallel fields with identical semantics. Mapping cour→part keeps the filter logic and tests uniform and matches how the data actually appears in the wild.

**Don't "fix" by:** adding a separate `Cour *int` field to `TorrentResult` or a separate extraction path.
