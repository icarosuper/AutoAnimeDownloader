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

### 11. `GetCustomListsMap` — separate lightweight query + cache for `customLists`

**Location:** `internal/anilist/anilist.go` — `GetCustomListsMap`; called at the top of `searchAnilist` (`verification.go`) and `mergeCurrentAniListAnimes` (`api/endpoint_animes.go`) before `GetAllCurrentAnime`.

**What it looks like:** We call Anilist twice per verification cycle: once with a minimal `id + customLists` query, and once with the full `GetAllCurrentAnime` query. Then we overwrite `ml.CustomLists` from the first result when the second comes back with `null`. Looks like redundant work and a band-aid.

**Why it's right:** Anilist's GraphQL engine enforces a query-complexity budget. `GetAllCurrentAnime` includes `airingSchedule.nodes` (which is large for currently-releasing anime — it contains all future episode schedules) plus `relations`, `synonyms`, and `coverImage`. For users watching many currently-airing animes, this query exceeds Anilist's complexity threshold, causing the API to return `null` for the `customLists` scalar on every entry in the response — silently, with HTTP 200 and no error field. COMPLETED/DROPPED animes are unaffected because their `airingSchedule` is empty.

**Root-cause confirmed by:** back-to-back curl tests: Run 1 returned 0/20 null, Runs 2–5 returned 20/20 null (rate-limit exhausted). The user also ran a manual minimal GraphQL query (only `id` + `customLists`) which returned correct data including `{"AutoDownloader": true}` — confirming the API works; only the complex query suppresses the field.

**Effect when broken:** `customLists` is always `null` → `animeIsInExcludedList` never matches → blacklisted animes (e.g. in the "AutoDownloader" custom list used to block titles) are downloaded anyway and the frontend block icon is never shown.

**Cache:** `GetCustomListsMap` caches results for 5 minutes keyed by `username + statuses`. This prevents the API endpoint (`/animes`, called on every page load) from exhausting the rate limit on repeated minimal queries. The cache is only populated when the response contains at least one non-null `CustomLists` entry, so a rate-limited empty response never evicts a valid cached result.

**Overlay guard:** `if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 { ml.CustomLists = cl }` — the `len(cl) > 0` guard ensures that a rate-limited nil response from `GetCustomListsMap` (which would produce an empty map entry) never silently clears data that `GetAllCurrentAnime` might have returned correctly on a lucky call.

**Don't "fix" by:**
- Removing `GetCustomListsMap` and relying solely on `GetAllCurrentAnime` — `customLists` will be null whenever the complex query hits the complexity limit, breaking blacklist exclusions entirely.
- Removing the cache — rapid successive calls (frontend polling, concurrent goroutines) will exhaust rate limits, causing all calls to return null.
- Removing the `len(cl) > 0` guard — an empty map overwrites any valid data the complex query returned, flipping all animes to "not blacklisted."

---

### 13. `GetFrontendAnimeList` — separate lighter Anilist query for the API endpoint

**Location:** `internal/anilist/anilist.go` — `GetFrontendAnimeList`; called from `mergeCurrentAniListAnimes` (`api/endpoint_animes.go`).

**What it looks like:** There are now two functions that both return `*AniListResponse` and differ only in which GraphQL fields they request. Looks like duplication.

**Why it's right:** `GetAllCurrentAnime` (used by the daemon verification loop via `searchAnilist`) needs `synonyms`, `relations`, and `format` to match torrents and compute offsets, but does not need `coverImage`. `mergeCurrentAniListAnimes` (frontend `/animes` endpoint) needs `coverImage` for display but never touches `synonyms`, `relations`, or the `id` field on airingSchedule nodes. Keeping them separate lets each query stay within Anilist's complexity budget — `GetAllCurrentAnime` avoids the cost of fetching images, and `GetFrontendAnimeList` avoids the cost of fetching relations/synonyms. The return type is the same (`*AniListResponse`); unused fields simply remain at their zero values.

**Don't "fix" by:** merging back into one query. Requesting all fields from both call sites is what caused complexity-budget exhaustion (see decision 11).

---

### 12. Build logic lives in `scripts/build.sh`, not in Makefile targets

**What it looks like:** `build-linuxamd64` / `build-linuxarm64` / `build-windows` just delegate to `bash scripts/build.sh <platform> <version>` with no logic in the Makefile itself.

**Why it's right:** Make's autocomplete exposes every named target, including internal ones (`docker-build`, `docker-buildx-build`, `docker-build-classic`, `checksums`, `check-docker`) and file/directory targets (`build/linux-amd64`, `build/linux-amd64/autoanimedownloader-daemon`, etc.). Moving the implementation to a script eliminates all internal targets from the completion list without requiring per-machine shell configuration.

**Don't "fix" by:** inlining Docker logic back into Makefile targets or re-introducing named intermediate targets. That re-pollutes autocomplete on every machine.

---

### 14. `nyaaSearcher` — dependency injection for Nyaa search in `processAnimeEpisodes`

**Location:** `internal/daemon/search.go` (`nyaaSearcher` struct + `defaultNyaaSearcher()`); `internal/daemon/episodes.go` (`processAnimeEpisodes`, `resolveSearchStrategy`).

**What it looks like:** `processAnimeEpisodes` receives a `nyaaSearcher` struct with function-valued fields instead of calling `searchNyaaForBatch`, `searchNyaaForMovie`, etc. directly. Looks like unnecessary indirection — these are pure functions with no state.

**Why it's right:** The `episodeInTorrents` hash-check fix prevents batch torrents from being re-downloaded in a loop. Without injection, it's impossible to write a test that proves the loop can't recur: a test that calls the real Nyaa makes a live HTTP request, is flaky, and can't observe whether `searchBatch` was invoked. Injection lets the regression test (`TestProcessAnimeEpisodes_BatchNoRedownload`) confirm both that `searchBatch` is never called and that `POST /add` is never sent when all episode hashes already match.

**Don't "fix" by:** removing the `nyaaSearcher` parameter and going back to direct package calls. That makes the regression test impossible to write, and the loop bug would be undetectable until it reappears in production.

---

### 10. "Cour N" treated as Part N, not as a distinct concept

**Location:** `internal/nyaa/nyaa_regex.go` — `rePartPatterns`; `internal/daemon/helpers.go` — `ExtractAnimeSeasonPart`

**What it looks like:** `Cour 2` is extracted by `ExtractPart`, not by a separate cour field. Looks like it conflates two different concepts.

**Why it's right:** Anilist uses "Cour" and "Part" interchangeably for split seasons (e.g. NieR:Automata Ver1.1a Cour 2). On Nyaa, release groups also use both terms for the same split. A separate cour field would require all search/filter logic to handle two parallel fields with identical semantics. Mapping cour→part keeps the filter logic and tests uniform and matches how the data actually appears in the wild.

**Don't "fix" by:** adding a separate `Cour *int` field to `TorrentResult` or a separate extraction path.

---

### 15. `atomic.Pointer[Priorities]` package-level global in `nyaa`

**Location:** `internal/nyaa/priorities.go` — `active atomic.Pointer[Priorities]`, `ActivePriorities()`, `SetPriorities()`.

**What it looks like:** A mutable global swapped at runtime instead of threading a `Priorities` value/config through every sort/filter function call (`SortTorrentResults`, `SortMovieResults`, `fansubPriority`, `ShouldIgnore`, etc.). Looks like hidden state that could cause data races or surprising cross-test pollution.

**Why it's right:** The sort/filter call sites (`ScrapNyaa*`, `SortTorrentResults`, `SortMovieResults`, and a dozen small `xPriority` helpers) are deep, numerous, and have no `*Config` or context parameter today — threading one through would touch most of `nyaa.go` for a value that changes rarely (only on config save/load). `atomic.Pointer` makes reads lock-free and the swap atomic, so the daemon's verification loop (running concurrently with API requests) never observes a torn read. `SetPriorities` returns a `restore func()`, mirroring the existing `MockNyaaHttpGet` convention (decision 1) — tests `defer restore()` instead of mutating shared state permanently. Package `init()` seeds the pointer with `DefaultPriorities()` so any code that calls the sort functions without ever calling `SetPriorities` (most unit tests) still gets correct, non-nil behavior.

**Don't "fix" by:** threading a `Priorities` parameter through every nyaa function, or replacing the atomic pointer with a mutex-guarded struct. The former is a large, low-value refactor; the latter adds lock contention to a hot path (every torrent comparison during sort) for no correctness benefit over `atomic.Pointer`.

---

### 16. `anime_id` is the AniList MediaList entry ID, not the AniList media ID

**Location:** `internal/anilist/anilist.go` (`MediaList.Id`, `GetAnimeInfo(mediaListId)`); `internal/api/endpoint_animes.go` and `endpoint_anime_episodes.go` (`AnimeID` fields).

**What it looks like:** `anime_id` is used everywhere as *the* identifier for an anime (settings files keyed by it, episode records, route params) — it would be natural to assume it's the AniList media ID and build an `anilist.co/anime/{id}` link directly from it.

**Why it's right:** The value actually comes from the top-level `id` field of AniList's `MediaList` GraphQL type, i.e. the list-entry ID (unique per user-entry), not `media.id` (the actual anime). It's stable and available from every query that returns a user's list, which is why it was chosen as the app-wide key. But it is not a valid AniList media URL component — `anilist.co/anime/{mediaListEntryId}` 404s. The real media ID is only fetched where needed: `AnimeDetailResponse.AnilistID` (from `GetAnimeInfo`'s `media { id }`), used solely to build the "view on AniList" link.

**Don't "fix" by:** assuming `anime_id` can be used to build AniList URLs, or replacing `anime_id` app-wide with the media ID — that would require re-keying settings files and episode records, a much larger and unrelated change.

---

### 17. `--debug-anime` early-exit branch in `cmd/daemon/main.go`

**Location:** `cmd/daemon/main.go` — `runDebugAnime`, the `flag.Int("debug-anime", ...)` check at the top of `main()`.

**What it looks like:** `main()` parses a flag and, if set, runs a completely different code path (`runDebugAnime`) and returns — skipping the PID file, API server, tray, and daemon loop entirely. Looks like a debug hack that snuck into production entry point.

**Why it's right:** It's a deliberate one-shot diagnostic mode (`make debug-anime ID=<anilistId>` / `go run ./src/cmd/daemon --debug-anime <id>`) for the recurring "why didn't this anime download" problem. It reuses real production functions (`daemon.RunAnimeDebug` → `checkEpisode`, `resolveSearchStrategy`) so the debug output can't drift from actual verification-loop behavior, and it deliberately avoids touching qBittorrent so it can run without the daemon or qBittorrent up. See `docs/agents/troubleshooting-downloads.md` Step 0 and `daemon/debug.go`.

**Don't "fix" by:** moving this behind the HTTP API (it exists specifically to work without a running daemon) or deleting it as dead code (it's the primary entry point for the fast-path troubleshooting flow).

---

### 18. `extractTitleTokens` truncates the torrent name at the first episode/season marker before tokenizing

**Location:** `internal/nyaa/nyaa_match.go` — `truncateAtFirstMarker`, `reLeadingBracket`, called at the top of `extractTitleTokens`.

**What it looks like:** Tokenizing only the substring before the first `S01E05`/`- 05`/`Episode 3`-style marker, and separately stripping a leading `[Group]`/`(Group)` tag, instead of just tokenizing the whole torrent name. Looks like it would drop legitimate title words.

**Why it's right:** Real torrent names often carry the episode's plot title, streaming-service tag, or a duplicate alt-title in parentheses *after* the episode marker (e.g. `KAIJU GIRL CARAMELISE S01E01 The Kaiju Girl Appears in Tokyo 1080p CR WEB-DL ... (Otome Kaijuu Caramelise, Multi-Subs)`). None of that belongs to the anime's core title, but it isn't covered by the fixed `titleTechnicalTokens` allowlist (unknown fansub tags like `varyg`/`ironclad`, stray split tokens like `h` from `H 264`, episode-title prose). Tokenizing it anyway inflates the Jaccard union enough that `jaccardThreshold` (0.8 for ≤3-token queries) rejects genuine matches — this caused two real currently-airing anime to never download (see debug session that produced this decision). Truncating at the marker (reusing `reEpisodePatterns`/`reSeasonPatterns`, already used by `extractEpisodeNumber`/`extractSeason`) removes exactly the noise while preserving genuine extra title words that appear *before* the marker (e.g. "Alternative Gun Gale Online" in a Sword Art Online spinoff), so the existing spin-off-rejection tests still pass.

**Don't "fix" by:** lowering `jaccardThreshold` instead. The two failure modes overlap: the real Kaiju Girl Caramelise match scores ~0.33 Jaccard, but the SAO-spinoff torrent that must stay rejected scores ~0.5 — no single threshold accepts one without accepting the other. The marker-truncation approach fixes the union inflation at its source instead.

---

### 19. Disk space is read via OS stat on `SavePath`, not qBittorrent's API

**Location:** `internal/files/diskspace_unix.go`, `internal/files/diskspace_windows.go`; `internal/api/endpoint_status.go` (`handleStatus`).

**What it looks like:** Reading disk space with a raw filesystem syscall on a local path, when qBittorrent's WebUI already exposes `free_space_on_disk` via `/api/v2/sync/maindata` — reaching for a platform-specific syscall pair looks like reinventing something the existing HTTP dependency already provides.

**Why it's right:** qBittorrent's endpoint only reports free space, not total capacity — the dashboard needs both ("tamanho total, tamanho disponível" per `docs/TODO.md`). The syscall approach is the only one of the two that can answer that. The known trade-off: if qBittorrent's actual save directory isn't the same filesystem the daemon process sees (e.g. daemon and qBittorrent split across hosts/containers with misaligned mounts), the numbers reflect the daemon's view of `SavePath`, not necessarily qBittorrent's real disk. Accepted for now since this project targets the common single-box setup; `handleStatus` swallows stat errors (empty/unreadable `SavePath`) rather than surfacing them, so a bad path just hides the disk card instead of breaking `/api/v1/status`.

**Don't "fix" by:** switching to qBittorrent's `free_space_on_disk` alone (loses total capacity) or trying to detect cross-host mount mismatches — no reliable way to do that from the daemon's side without an out-of-band signal from qBittorrent itself.
