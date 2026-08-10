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

**Location:** `internal/anilist/anilist.go` — `GetCustomListsMap`; called at the top of `searchAnilist` (`verification.go`) and `fetchAniListEntries` (`api/endpoint_animes.go`) before `GetAllCurrentAnime`.

**What it looks like:** We call Anilist twice per verification cycle: once with a minimal `id + customLists` query, and once with the full `GetAllCurrentAnime` query. Then we overwrite `ml.CustomLists` from the first result when the second comes back with `null`. Looks like redundant work and a band-aid.

**Why it's right:** Anilist's GraphQL engine enforces a query-complexity budget. `GetAllCurrentAnime` includes `airingSchedule.nodes` (which is large for currently-releasing anime — it contains all future episode schedules) plus `relations`, `synonyms`, and `coverImage`. For users watching many currently-airing animes, this query exceeds Anilist's complexity threshold, causing the API to return `null` for the `customLists` scalar on every entry in the response — silently, with HTTP 200 and no error field. COMPLETED/DROPPED animes are unaffected because their `airingSchedule` is empty.

**Root-cause confirmed by:** back-to-back curl tests: Run 1 returned 0/20 null, Runs 2–5 returned 20/20 null (rate-limit exhausted). The user also ran a manual minimal GraphQL query (only `id` + `customLists`) which returned correct data including `{"AutoDownloader": true}` — confirming the API works; only the complex query suppresses the field.

**Effect when broken:** `customLists` is always `null` → `animeIsInExcludedList` never matches → blacklisted animes (e.g. in the "AutoDownloader" custom list used to block titles) are downloaded anyway and the frontend block icon is never shown.

**Cache:** `GetCustomListsMap` caches results keyed by `username + statuses` (`ttlCache`, ver decisions.md #46). Uma resposta com pelo menos um `CustomLists` não-nulo fica 5 minutos (`customListsTTL`); uma resposta vazia fica só 30 segundos (`customListsEmptyTTL`), porque "vazio" tanto pode ser uma conta sem custom lists quanto um campo que a AniList degradou. Antes o vazio não era cacheado de jeito nenhum, e aí uma conta sem custom lists gerava um request por poll do frontend — exatamente o amplificador que estourava o limite. A leitura do cache acontece antes da busca, então nenhuma resposta degradada chega a evictar uma entrada ainda válida.

**Overlay guard:** `if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 { ml.CustomLists = cl }` — the `len(cl) > 0` guard ensures that a rate-limited nil response from `GetCustomListsMap` (which would produce an empty map entry) never silently clears data that `GetAllCurrentAnime` might have returned correctly on a lucky call.

**Don't "fix" by:**
- Removing `GetCustomListsMap` and relying solely on `GetAllCurrentAnime` — `customLists` will be null whenever the complex query hits the complexity limit, breaking blacklist exclusions entirely.
- Removing the cache — rapid successive calls (frontend polling, concurrent goroutines) will exhaust rate limits, causing all calls to return null.
- Removing the `len(cl) > 0` guard — an empty map overwrites any valid data the complex query returned, flipping all animes to "not blacklisted."

---

### 13. `GetFrontendAnimeList` — separate lighter Anilist query for the API endpoint

**Location:** `internal/anilist/anilist.go` — `GetFrontendAnimeList`; called from `fetchAniListEntries` (`api/endpoint_animes.go`).

**What it looks like:** There are now two functions that both return `*AniListResponse` and differ only in which GraphQL fields they request. Looks like duplication.

**Why it's right:** `GetAllCurrentAnime` (used by the daemon verification loop via `searchAnilist`) needs `synonyms`, `relations`, and `format` to match torrents and compute offsets, but does not need `coverImage`. `GetFrontendAnimeList` (frontend `/animes` endpoint) needs `coverImage` for display but never touches `synonyms`, `relations`, or the `id` field on airingSchedule nodes. Keeping them separate lets each query stay within Anilist's complexity budget — `GetAllCurrentAnime` avoids the cost of fetching images, and `GetFrontendAnimeList` avoids the cost of fetching relations/synonyms. The return type is the same (`*AniListResponse`); unused fields simply remain at their zero values.

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

### 16. `anime_id` é o AniList **media** ID (foi o MediaList entry ID até a migração)

**Location:** `internal/anilist/anilist.go` (`Media.Id`, `GetAnimeInfo(mediaId, usernames)`); `internal/api/endpoint_animes.go` e `endpoint_anime_episodes.go` (campos `AnimeID`); `daemon/migration.go` (`MigrateAnimeIDsToMedia`).

**What it looks like:** `anime_id` é *o* identificador de anime em todo lugar (arquivo de settings, registros de episódio, parâmetros de rota) e também serve de componente da URL `anilist.co/anime/{id}` — não existe mais um `anilist_id` separado na API.

**Why it's right:** até a migração, o valor vinha do campo `id` do tipo `MediaList` da AniList, que é o id da **entrada de lista** e portanto **por conta**. Com mais de uma conta configurada, o mesmo anime tinha dois `anime_id`, e o app tratava como dois animes. Ver a decisão #43 para o bug concreto e o desenho atual. Instalações antigas são convertidas uma única vez por `MigrateAnimeIDsToMedia`, marcada pelo campo `anime_ids_are_media_ids` do config.

**Don't "fix" by:** voltar a chavear qualquer coisa por `MediaList.Id` (é por conta — ver #43); rodar a migração de novo depois da marca ligada (interpretaria ids de mídia como ids de entrada).

---

### 17. `--debug-anime` early-exit branch in `cmd/daemon/main.go`

**Location:** `cmd/daemon/main.go` — `runDebugAnime`, the `flag.Int("debug-anime", ...)` check at the top of `main()`.

**What it looks like:** `main()` parses a flag and, if set, runs a completely different code path (`runDebugAnime`) and returns — skipping the PID file, API server, tray, and daemon loop entirely. Looks like a debug hack that snuck into production entry point.

**Why it's right:** It's a deliberate one-shot diagnostic mode (`make debug-anime ID=<anilistId>` / `go run ./src/cmd/daemon --debug-anime <id>`) for the recurring "why didn't this anime download" problem. It reuses real production functions (`daemon.RunAnimeDebug` → `checkEpisode`, `resolveSearchStrategy`) so the debug output can't drift from actual verification-loop behavior, and it deliberately avoids touching the torrent client so it can run without the daemon up (the torrent client is embedded, so there is nothing external to start either way). See `docs/agents/troubleshooting-downloads.md` Step 0 and `daemon/debug.go`.

**Don't "fix" by:** moving this behind the HTTP API (it exists specifically to work without a running daemon) or deleting it as dead code (it's the primary entry point for the fast-path troubleshooting flow).

---

### 18. `extractTitleTokens` truncates the torrent name at the first episode/season marker before tokenizing

**Location:** `internal/nyaa/nyaa_match.go` — `truncateAtFirstMarker`, `reLeadingBracket`, called at the top of `extractTitleTokens`.

**What it looks like:** Tokenizing only the substring before the first `S01E05`/`- 05`/`Episode 3`-style marker, and separately stripping a leading `[Group]`/`(Group)` tag, instead of just tokenizing the whole torrent name. Looks like it would drop legitimate title words.

**Why it's right:** Real torrent names often carry the episode's plot title, streaming-service tag, or a duplicate alt-title in parentheses *after* the episode marker (e.g. `KAIJU GIRL CARAMELISE S01E01 The Kaiju Girl Appears in Tokyo 1080p CR WEB-DL ... (Otome Kaijuu Caramelise, Multi-Subs)`). None of that belongs to the anime's core title, but it isn't covered by the fixed `titleTechnicalTokens` allowlist (unknown fansub tags like `varyg`/`ironclad`, stray split tokens like `h` from `H 264`, episode-title prose). Tokenizing it anyway inflates the Jaccard union enough that `jaccardThreshold` (0.8 for ≤3-token queries) rejects genuine matches — this caused two real currently-airing anime to never download (see debug session that produced this decision). Truncating at the marker (reusing `reEpisodePatterns`/`reSeasonPatterns`, already used by `extractEpisodeNumber`/`extractSeason`) removes exactly the noise while preserving genuine extra title words that appear *before* the marker (e.g. "Alternative Gun Gale Online" in a Sword Art Online spinoff), so the existing spin-off-rejection tests still pass.

**Don't "fix" by:** lowering `jaccardThreshold` instead. The two failure modes overlap: the real Kaiju Girl Caramelise match scores ~0.33 Jaccard, but the SAO-spinoff torrent that must stay rejected scores ~0.5 — no single threshold accepts one without accepting the other. The marker-truncation approach fixes the union inflation at its source instead.

---

### 19. Disk space is read via OS stat on `CompletedAnimePath`

**Location:** `internal/files/diskspace_unix.go`, `internal/files/diskspace_windows.go`; `internal/api/endpoint_status.go` (`handleStatus`).

**What it looks like:** Reading disk space with a raw platform-specific filesystem syscall on a local path, when a portable Go library might seem cleaner.

**Why it's right:** The dashboard needs both total capacity **and** free space ("tamanho total, tamanho disponível" per `docs/TODO.md`); the syscall pair (`Statfs` / `GetDiskFreeSpaceEx`) is the direct way to get both. `handleStatus` swallows stat errors (empty/unreadable `CompletedAnimePath`) rather than surfacing them, so a bad path just hides the disk card instead of breaking `/api/v1/status`.

> **Revision (embedded-client refactor):** The original rationale compared this against qBittorrent's `free_space_on_disk` API (which only reported free space, not total). That comparison is now moot — the torrent client is embedded and there is no qBittorrent API to read from. The old cross-host mount-mismatch caveat no longer applies either. The OS-stat approach stands as the only and correct source.

> **Amendment (see #31):** disk space is now measured on `CompletedAnimePath`, not the legacy `save_path` field. Since the download directory is derived from `CompletedAnimePath` (`Config.DownloadPath`), the two are always on the same filesystem by construction, so measuring the library path also correctly reports free space for downloads-in-progress.

**Don't "fix" by:** trying to route disk stats through the torrent library — it has no such API, and `CompletedAnimePath` is on the same filesystem as the download directory the daemon writes to.

---

### 20. `extractSeason` has a roman-numeral fallback kept out of `reSeasonPatterns`

**Location:** `internal/nyaa/nyaa_regex.go` — `reRomanSeason`, `romanSeasonValues`; `internal/nyaa/nyaa.go` — `extractSeason`.

**What it looks like:** A second, separate regex (`\b(II|III|IV|V|VI|VII|VIII|IX|X)\b`) tried only after the whole `reSeasonPatterns` loop has failed, instead of just adding it as one more entry in that slice.

**Why it's right:** Some sequels are titled with only a roman numeral (e.g. Anilist id 194829, "Katainaka no Ossan, Kensei ni Naru II") — no "Season 2"/"S2" appears anywhere in the AniList title, and fansub groups (Erai-raws, Ironclad) release episodes using that exact title verbatim, with no separate season marker either. Before this fix, `extractSeason("...Naru II - 01 [1080p]...")` returned `nil` while `ExtractAnimeSeasonPart` (via the `"...2nd Season"` synonym) correctly resolved `requestedSeason=2`, so every real torrent got rejected by the hard season filter in `ScrapNyaaForMultipleEpisodes` (`season == nil` vs `requestedSeason=2`) even though `titleMatchesQuery` already matched the full title including "II" — the anime failed to download every cycle. `reSeasonPatterns` is also used by `truncateAtFirstMarker` (decision 18) to decide where to cut a torrent name before tokenizing; if the roman-numeral pattern were merged into that slice, it would truncate `"...Naru II"` right before "II", dropping it from the Jaccard title tokens and silently changing match behavior for every title ending in a numeral. Keeping it as a separate, lower-priority fallback used only by `extractSeason` avoids that cross-effect.

**Don't "fix" by:** merging `reRomanSeason` into `reSeasonPatterns`, or matching lowercase roman numerals — lowercase risks false positives from unrelated fansub/codec tokens, and uppercase-only matches how anime titles actually format them.

---

### 21. Embedded torrent client + hardlink-into-library model (replaces qBittorrent)

**Location:** `internal/torrents/` (`TorrentBackend`, `Session`/`SessionManager`, `FakeBackend`); `internal/files/librarian.go`; `internal/daemon/jobs.go` (`JobOrganize`), `episodes.go`, `verification.go`.

**What it looks like:** The app embeds a full BitTorrent client (`github.com/cenkalti/rain/v2`) and hardlinks completed files into a second directory, instead of the more familiar "talk to a running qBittorrent over its WebUI and let it manage files."

**Why it's right:** Embedding makes the daemon a single self-contained binary — no external qBittorrent to install, run, secure, or keep reachable, and no `qbittorrent_url`/`QBITTORRENT_URL` to configure. Torrents download to `save_path` and keep **seeding** there; on completion the video files are **hardlinked** into `completed_anime_path` (the Jellyfin library). Hardlinking (not copy/move) means:
- No wasted space — the library name and the seeded file share the same bytes.
- Seeding is never interrupted — the seeded file is never moved or renamed (renaming would break the torrent). The Jellyfin name (`"Anime Name - E05.mkv"` for single episodes; raw filenames for batches/movies) lives only on the library hardlink.
- The hard constraint: `save_path` and `completed_anime_path` **must be on the same filesystem/volume** (hardlinks can't cross devices). `completed_anime_path` is therefore now **required**, and the config-save endpoint validates the pair with a real hardlink probe (`Librarian.ProbePaths`), rejecting cross-device paths with HTTP 400.

Torrent logic sits behind the `TorrentBackend` interface (`Add`/`List`/`Get`/`Remove`/`Ensure`/`SetCallbacks`/`Close`) so the daemon injects one uniform backend — rain-backed `SessionManager` in production, in-memory `FakeBackend` in tests (the qBittorrent mock server is gone). Resume data lives in a bbolt DB at `~/.autoAnimeDownloader/session.db`, deliberately **outside** `save_path` so it survives a `save_path` change. rain listens on a default port range (20000–30000) with no UPnP/NAT-PMP; inbound peers may need manual forwarding, but DHT+PEX work without it.

**Don't "fix" by:** re-introducing an external torrent client or `qbittorrent_url`; renaming the seeded file to the Jellyfin name (breaks seeding); copying/moving into the library instead of hardlinking (wastes space, stops seeding); or moving `session.db` into `save_path` (loses resume data on a path change). Note the webhook event key string is still `download_completed` — the Go constant was renamed `QBittorrentDownloadCompleted → DownloadCompleted`, but that is not user-visible; don't "fix" a webhook that isn't broken.

**Amendment (see #31):** `save_path` is no longer a user-configured field — the download directory is now derived from `completed_anime_path` (`Config.DownloadPath`). What was previously a constraint on the user's configuration ("these two fields must be on the same filesystem, and the endpoint validates it") is now an architectural invariant that holds by construction: there is only one path, so there is nothing to be on a different filesystem from. `Librarian.ProbePaths` (two arguments) is gone; the surviving probe is `Librarian.ProbePath` (one argument, #26).

---

### 22. Organize everything to `completed_anime_path`, and the batch-hygiene deletion limitation

**Location:** `internal/daemon/jobs.go` (`organizeTorrent`), `internal/daemon/episodes.go` (`removeEpisodesAndLinks`, `allEpisodesInDeleteSet`). Pinned with real files on disk by `TestRemoveEpisodesAndLinks_RealHardlinks` (`internal/daemon/orchestration_test.go`), whose subtests separate the batch asymmetry (torrent *and* library files kept while a sibling survives) from the non-batch shared-hash case (library link removed, torrent kept).

**What it looks like:** (a) *Every* completed torrent — ongoing or finished — is hardlinked into `completed_anime_path`, not just finished animes. (b) When deleting a watched episode that belongs to a **batch** torrent, its library hardlink is **kept** until the entire batch is deleted, which looks like a leak.

**Why it's right (behavior change):** Previously only FINISHED animes were moved to `completed_anime_path`, while ongoing episodes were renamed in place inside `save_path`. Now `save_path` is purely the download/seeding working directory and `completed_anime_path` is the Jellyfin library — so *every* completed torrent gets organized into the library, uniformly, via the single idempotent `organize` job. This removes the ongoing-vs-finished special case and gives Jellyfin one consistent library path to watch.

**Why it's right (batch guard):** Deletion frees space by removing both the library hardlink and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). For a **single-episode** torrent that maps cleanly to one library file, both links are removed. For a **batch** torrent shared by many episodes, the raw batch filenames can't be safely mapped back to one specific episode — removing "the file for episode 5" risks deleting the wrong file. So per-episode library removal is **skipped for batches**; the batch's library files (and the seeding torrent) are only removed once **all** of that torrent's episodes are in the delete set (`allEpisodesInDeleteSet`). While any sibling episode survives, the batch torrent and its library links stay. The small space cost of keeping a shared batch around a bit longer is preferred over the correctness risk of deleting the wrong episode's file.

**Don't "fix" by:** trying to delete individual episode files out of a batch torrent's library folder (raw filenames aren't reliably episode-addressable), or removing a batch torrent while siblings still reference it (breaks the survivors' library links and stops seeding for episodes still wanted).

**Amendment (see #31):** `save_path` no longer exists as a config field; "everything organizes to `completed_anime_path`" now also covers the download/seeding directory itself, which lives at `<completed_anime_path>/.torrents` (`Config.DownloadPath`).

---

### 23. Integration tests skip unless `DAEMON_URL` is set explicitly

**Location:** `src/tests/integration/integration_test.go` (`requireDaemon`, `defaultTestPath`, `testSavePath`/`testCompletedPath`); `docker/docker-compose.test.yml` (`test` service env).

**What it looks like:** `go test ./...` reports the entire integration suite as SKIP on a developer machine, even when a daemon *is* running on `localhost:8091`. The obvious-looking "fix" is to drop the `DAEMON_URL` check and let `probeDaemon()` decide, since a reachable daemon seems like exactly the condition the tests want.

**Why it's right:** These tests are not read-only. `TestAPIEndpoints` and `TestFullDownloadFlow` both `PUT /api/v1/config`, overwriting `save_path`, `completed_anime_path`, `anilist_username`, `check_interval` and `excluded_lists` on whatever daemon answers. Gating on reachability alone meant that following `CLAUDE.md`'s "run `go test ./...` after any change" silently reconfigured the developer's own daemon: the save path became `/tmp/test`, and on distros where `/tmp` is a tmpfs the daemon then downloaded real torrents into RAM until the filesystem filled. A live daemon is not consent to reconfigure it, so the opt-in is an explicit env var. `docker-compose.test.yml` sets `DAEMON_URL=http://daemon:8091`, so Docker and CI runs are unaffected.

The paths written into the config are likewise no longer hardcoded: they come from `TEST_SAVE_PATH`/`TEST_COMPLETED_PATH`, defaulting to `~/aad-test/downloads` and `~/aad-test/library`. Two constraints shape that default — it must not be under `/tmp` (tmpfs), and both paths must share one filesystem, because the config endpoint runs a real hardlink probe (`Librarian.ProbePaths`) and rejects cross-volume pairs with HTTP 400. Docker overrides both to `/app/data/aad-test/*`, inside the daemon container's mounted volume; note the strings are interpreted by the **daemon** container, not the test container.

**Don't "fix" by:** removing the `DAEMON_URL` gate so the tests "work" during `go test ./...`; hardcoding the config paths again; putting the default paths back under `/tmp`; or splitting save and completed paths across different volumes (the hardlink probe rejects it).

**Amendment (see #31):** `testSavePath`/`TEST_SAVE_PATH` were removed — the integration test no longer sends `save_path` at all, since the field doesn't exist anymore. `Librarian.ProbePaths` (the two-argument save/completed variant) was replaced by `Librarian.ProbePath(completedPath string)`.

---

### 24. A failed torrent is dropped from the session and re-added by the next pass — no blacklist

**Location:** `internal/daemon/helpers.go` — `HandleTorrentFailure`; wired as the `onFailed` callback in `cmd/daemon/main.go`.

**What it looks like:** When the embedded client stops a torrent with an error, we fire the `download_failed` webhook and then **remove the torrent from the session** (`Remove(hash, false)`, discarding partial data). Since nothing records that the hash failed, the very next verification pass finds the episode missing from the session and re-adds the same magnet. For a torrent that is genuinely dead (no seeds at all) this is an unbounded re-add loop, one attempt per `check_interval`. That looks like a bug we forgot to guard.

**Why it's right:** The alternative is worse. rain leaves a failed torrent in state Stopped **inside** the session and never restarts it, and the per-torrent listener goroutine exits after `NotifyStop` and is not re-armed (`torrents/session.go`). If we only logged, `episodeInTorrents` (`daemon/helpers.go`) would keep seeing the hash and the daemon would believe the episode was downloaded — forever. No retry, no organize, no notification, and the episode silently never reaches the library. Dropping the torrent converts a permanent silent failure into an automatic retry that reuses the machinery already in place (the loop re-searches Nyaa and may pick a *different*, healthier release the second time). `EpisodeRetryLimit` does not bound this, because it resets each pass.

**Cost of the accepted risk:** a dead torrent costs one Nyaa search plus one magnet add per `check_interval` — cheap, visible in the logs, and each retry fires a `download_failed` webhook so the user is not left guessing.

**Don't "fix" by:** adding a speculative per-hash blacklist with a TTL. That is the correct fix *if* the re-add churn is ever observed to be a real problem, but it introduces new state to persist, expire, and expose in the UI (otherwise a permanently blacklisted episode becomes the new silent failure). Build it against a real report, not preemptively. Also don't go back to log-only in `onFailed`, and don't pass `keepData=true` — partial data from a torrent that errored out is not reusable and only occupies the save path.

---

### 25. Seeding is created at startup and is independent of the daemon loop

**Location:** `cmd/daemon/main.go` — `ensureStartupSession`, called right after `jobQueue.Start()`; `internal/daemon/verification.go` — the `backend.Ensure(configs.DownloadPath())` call inside the verification pass.

**What it looks like:** `Ensure` is called from **two** places — once at startup and once on every verification pass — which reads like a redundant call that could be dropped from one side.

**Why it's right:** The two calls serve different purposes and both are needed.

- **Startup call:** torrents keep **seeding** from the derived download directory (`Config.DownloadPath()`, nested under `completed_anime_path`) after they complete. If the session were created only by the verification pass, stopping the daemon loop from the WebUI (or booting with the loop stopped) would mean no session at all, i.e. **seeding stops** — a behavior regression against the external-qBittorrent setup, which kept seeding regardless of what the daemon was doing. That matters for private trackers with ratio requirements. Creating the session at startup makes seeding a property of the *process*, not of the loop.
- **Verification-pass call:** the daemon must be able to boot with an incomplete config (no `completed_anime_path`, and therefore no derivable download path). In that case the startup call deliberately does nothing and the session stays lazy; the pass's `Ensure` is what creates it once the user saves a config. It also handles `completed_anime_path` changing at runtime (recreate, since the derived download path moves with it) and is where the `created == true` return feeds startup reconciliation.

Ordering detail: `ensureStartupSession` runs **after** `jobQueue.Start()`. Creating the session arms the resume listeners, which can fire a completion immediately, and `Start()` loads the persisted job list *over* whatever is in memory — enqueueing before it would silently discard the job. The shutdown defers keep their LIFO order (`jobQueue.Stop()` drains organize jobs that still need the session, then `torrentManager.Close()` flushes bbolt); do not reorder them.

**Don't "fix" by:** removing the `Ensure` from the verification pass (breaks the incomplete-config boot and reacting to `completed_anime_path` changes), removing the startup call (stops seeding whenever the loop is stopped), moving startup reconciliation out of the verification pass, or calling `ensureStartupSession` before `jobQueue.Start()`.

**Amendment (see #31):** `daemon.MigrateSavePath` now runs in `main.go` between `jobQueue.Start()` and `ensureStartupSession`, converting a legacy `save_path` installation to the single-folder model by moving its data under the derived download path. If migration fails, `ensureStartupSession` is **skipped entirely** for that boot: the data may still be sitting at the legacy path, so opening the session at the new derived `DownloadPath()` would resume every torrent against an empty directory and re-download the whole library. The verification pass keeps retrying the migration on each tick until it succeeds; only then does a session get created.

---

### 26. The hardlink probe runs on every verification pass, not just on config save

**Location:** `internal/daemon/verification.go` — the `librarian.ProbePaths` gate right after `isConfigComplete`; also `internal/api/endpoint_config.go` (`PUT /config`).

**What it looks like:** Every pass writes a probe file to `save_path`, hardlinks it into `completed_anime_path` and deletes both — real disk I/O on a hot loop, duplicating a validation the config endpoint already performs.

**Why it's right:** `isConfigComplete` only checks that the fields are non-empty, and the endpoint probe only covers configs saved **through the API after** the embedded-client upgrade. Two populations bypass it entirely: users who configured a `completed_anime_path` on a different volume back when the app *moved* files (perfectly legal then — `rename` crosses devices, `link` does not), and any deployment where `docker/entrypoint.sh` writes `config.json` straight from env vars. For them the daemon would download happily while every `JobOrganize` failed with `EXDEV`, retried 20 times over ~2.5h, and was dropped — with `LastCheckError` never set, so the WebUI showed a healthy daemon and an empty library.

The probe aborts the pass instead of merely warning: downloading episodes that provably cannot be organized only fills the disk. It reuses `Librarian.ProbePaths`, so the message the user sees in the UI is identical to the one `PUT /config` returns. Cost is one small file write, one link and two unlinks per `check_interval` (default 10 min) — negligible next to the Anilist and Nyaa requests in the same pass.

**Don't "fix" by:** removing the gate because "the endpoint already validates it" (it does not, for pre-upgrade and entrypoint-written configs), or downgrading it to a warning that lets the pass continue. Caching the result per path pair is a legitimate optimization if the I/O ever shows up in a profile — but it must be invalidated on config change, and the current cost does not justify the extra state.

**Amendment (see #31):** the probe is now single-path. `Librarian.ProbePaths(save, completed)` was replaced by `Librarian.ProbePath(completedPath)`, which writes a probe file directly under `<completedPath>/.torrents` and hardlinks it in place — there is no second user-supplied path to compare against, since the download directory is derived, not configured. The check still exists for the same reason: exFAT/FAT32 and some SMB shares don't support hardlinks at all, so the invariant "the download dir and the library share a filesystem" is guaranteed by construction, but "the filesystem supports hardlinks" is not.

---

### 27. `saveEpisodesToFile` merges by hand — it uses neither `SaveEpisodesToFile` nor a bare `UpsertEpisodes`

**Location:** `internal/daemon/episodes.go` — `saveEpisodesToFile` and `mergeSavedEpisode`; the two `FileManager` primitives in `internal/files/filemanager.go`.

**What it looks like:** The daemon loads every saved episode, merges field by field, and only then calls `UpsertEpisodes` — when the `FileManager` already exposes two perfectly good one-liners. Either `SaveEpisodesToFile(eps)` or `UpsertEpisodes(eps)` would compile and look cleaner.

**Why it's right:** The three functions have genuinely different semantics and each is wrong on its own here.

- `SaveEpisodesToFile` is **append-only with dedupe by `EpisodeID`**: for an ID that already exists it drops the incoming record *entirely*. That was the original bug. On the upgrade path to the embedded client, the rain session boots empty, so `episodeInTorrents` is false for every saved episode and the loop re-downloads all of them — and every one of those updates was silently discarded. The stale record kept `EpisodeNumber: 0` (a field that did not exist before the upgrade, so it deserialises as zero for all pre-existing records) and the stale `EpisodeHash`. The first caused every episode of an anime to be organized as `Anime - E00.mkv`; the second broke `JobOrganize`'s saved-episode ↔ torrent join by hash, so the job retried 20 times over ~2.5h and gave up, forever.
- `UpsertEpisodes` **replaces the record wholesale**. That is exactly what `organizeTorrent` needs when writing `LibraryPaths` back, and it is what makes it wrong here: it would clobber `ManuallyManaged`, a user flag the automatic loop must never clear (clearing it lets the loop delete an episode the user pinned).

The merge is the only place that knows which fields belong to *this* download (hash, number, `IsBatch`, names, totals, date — all taken from the new record), which belongs to the *user* (`ManuallyManaged`, OR-ed so a manual download can still set it), and which is *derived* (`LibraryPaths`, reset to nil because the old hardlinks point at the previous release — leaving it set would make `organizeTorrent` think the episode was already organized and never create the new link).

The API handlers in `endpoint_episode_actions.go` reach the same requirement by a shorter route: they call `UpsertEpisodes` directly, never `SaveEpisodesToFile`. Wholesale replacement is safe there — and only there — because every `daemon.ManualDownload*` constructor returns a record with `ManuallyManaged: true` and `LibraryPaths` unset, so the record already carries what `mergeSavedEpisode` would have computed. `handleDownloadEpisode` is the call site that actually needed this: unlike redownload and replace, it does **not** delete the old record first, so append-only dedupe silently dropped the whole update whenever a record already existed. Pinned by `TestHandleDownloadEpisode_UpdatesExistingRecord`, which uses a real `FileManager` over a temp dir — the api package's `mockFileManager` implements both primitives identically and cannot see the difference.

**Don't "fix" by:** collapsing the daemon's merge into either primitive; "simplifying" `mergeSavedEpisode` to `merged.ManuallyManaged = existing.ManuallyManaged` (breaks manual download setting the flag on a fresh record); or switching the API handlers to `SaveEpisodesToFile` "for consistency with the daemon". If a future `ManualDownload*` variant stops setting `ManuallyManaged`, the handlers must move to the daemon's merge rather than keep the bare upsert.

---

### 28. `Organize` replaces a library file that has the same name but different bytes

**Location:** `internal/files/librarian.go` — the `os.SameFile` branch in `Organize`; the `*req.EpisodeNumber > 0` term in the `useJellyfin` condition.

**What it looks like:** When the destination path already exists, `Organize` deletes it and creates a fresh hardlink. Destroying a file that is already in the user's library looks reckless — the safe-looking options are to skip it (as the code originally did, on a bare `Stat(dest) == nil`) or to fail loudly.

**Why it's right:** A bare existence check cannot tell "our hardlink, already created" from "a different file that happens to collide on name", and treating the second as the first is silent data loss: the caller is told the episode was organized, `LibraryPaths` is written pointing at someone else's bytes, and the `download_completed` webhook fires. `os.SameFile` (inode on Unix, file index on Windows — both `Stat` calls go through the injectable `FileSystem`, so fakes still work) splits the two cases. Same file is a true no-op, which is what reconciliation and job retries need. Different file only arises from redownload and replace-episode, where the user explicitly asked for the swap, so the new release wins; the replacement is logged at `Info` with source and destination so it stays auditable. Failing instead would deadlock those flows in `JobOrganize` retries. The seeded file is never touched — only the library name is recycled — so seeding is unaffected.

The related `*req.EpisodeNumber > 0` guard is defence in depth for the same failure: AniList has no episode 0, so a zero unambiguously means *missing data* rather than a real episode. Falling back to the raw filename (unique per release) is strictly better than emitting `Anime - E00.mkv`, which collides across every episode of the anime and, before the `SameFile` fix, made them all resolve to the same destination. Decision 27 fixes the cause; this guard makes sure no future path reintroduces it.

**Don't "fix" by:** restoring the bare `Stat`-and-skip, turning the different-inode case into an error, or dropping the `> 0` term because "the persistence layer guarantees a real episode number now" — the guard exists precisely because that guarantee is not enforceable at this layer.

---

### 29. Startup reconciliation keys off empty `LibraryPaths`, not off the hardlink missing from disk

**Location:** `internal/daemon/verification.go` — `reconcileLibrary`, called at the top of every verification pass. Pinned by `TestReconcileLibrary_Marker` in `internal/daemon/orchestration_test.go`.

**What it looks like:** Reconciliation decides whether an episode still needs organizing by looking only at the **record** (`LibraryPaths` empty → enqueue `JobOrganize`). It never stats the paths, so a record whose `LibraryPaths` point at files the user has since deleted is treated as already organized and skipped. The embedded-client design (§2) actually specified the opposite — check whether the hardlink exists on disk — so this reads like an unfinished implementation.

**Why it's right:** Statting the paths would turn every pass into a repair loop that **resurrects files the user deleted on purpose**. Deleting from the library is a legitimate user action (freeing the library while leaving the torrent seeding, or pruning something they no longer want), and the seeded file is still there for the hardlink to be recreated from — so a disk-based check would recreate it on the next pass, every pass, forever, with no way to opt out short of deleting the episode record too. The record is the daemon's own statement of "I have organized this"; the disk belongs to the user. Keeping them separate means reconciliation only ever fixes work the daemon failed to finish (a crash between torrent completion and organize, or a session restarted before `JobOrganize` ran), which is exactly its purpose.

The deviation is intentional and better than the design. It is also what makes reconciliation idempotent and cheap: `EnqueueOrganize` dedupes by hash, so three consecutive passes over the same unorganized torrent produce one job.

**Don't "fix" by:** adding a `Stat` on `LibraryPaths` to "detect" missing library files, or clearing `LibraryPaths` when the paths no longer resolve — both reintroduce the resurrection loop. A user who wants the link back should redownload or replace the episode, which resets `LibraryPaths` through the merge in decision 27.

**One exception, see decision 34:** when the *entire download root* is detected as swapped (moved/trashed/replaced folder), `clearLibraryPathsAfterRootSwap` wipes the records once. That is a different event from a per-file deletion and is edge-triggered, so it cannot become the loop this decision guards against.

---

### 30. Progress data comes from one `Stats()` per torrent, pulled only while a screen is open

**Location:** `internal/torrents/session.go` (`toInfo`); `internal/api/endpoint_torrents.go`; `frontend/src/routes/Downloads.svelte`; `frontend/src/routes/Status.svelte`.

**What it looks like:** `TorrentInfo` carries a dozen progress fields filled from a single
`t.Stats()` call, and the WebUI polls `GET /api/v1/torrents` on a plain client-side timer
while a screen that needs it is mounted — instead of the more familiar "push updates over
the existing WebSocket". Two screens poll it independently, at different rates: `Downloads.svelte`
every 2s (`setInterval(load, 2000)`) while the downloads screen is open, and `Status.svelte`
every 5s (`torrentsPollInterval = setInterval(loadTorrents, 5000)`) for the global speed
card, since Status is the default `#/` landing route. Both intervals are cleared on
unmount, so the endpoint is only hit while at least one of those two screens is on screen.

**Why it's right:** rain's `Stats()` is not a getter — it is a blocking round-trip into that
torrent's goroutine (`torrent/torrent_commands.go:141`), and so are `Peers()`, `Trackers()`
and `Webseeds()`. Reading fields one at a time, or on a server-side ticker, would hammer
every torrent's goroutine forever, including when nobody is looking. Pulling on demand costs
exactly zero when no screen is open. The same reasoning keeps `Peers()`/`Trackers()` out of
the list route entirely.

**Don't "fix" by:** adding a parallel `Stats()`-per-field accessor; moving the poll to a
server-side ticker or the WebSocket without gating it on `len(wsm.clients) > 0`; or calling
`Peers()`/`Trackers()`/`FileStats()` from the list endpoint.

**Related:** the API exposes a hand-written status slug (`statusSlug`) rather than rain's
`Status.String()`, which returns display text with a space (`"Downloading Metadata"`) and
can be reworded by any library upgrade. And `Session.Resume` re-arms the per-torrent listener,
because pausing makes the one-shot `NotifyStop` fire and kills the goroutine — without the
re-arm a resumed torrent would complete without ever enqueuing `JobOrganize`. Finally,
`Completed` is computed by `completedFromStats` from piece counts (`st.Pieces.Have >=
st.Pieces.Total`), not from `Status`: pausing a fully-downloaded torrent takes it out of
`Seeding`, and both `jobs.go`'s retry gate and `verification.go`'s reconciliation gate on
`Completed`, so deriving it from `Status` would make a paused-but-finished torrent invisible
to organization — the plan originally specified `Completed: st.Status == torrent.Seeding`,
and this was corrected during review specifically because `Pause` made the bug reachable.
The same lesson applies one layer up, to bytes rather than pieces: `Torrent.Stop()` frees
rain's piece data (`closeData` nils `t.pieces`), which zeroes `Bytes.Completed` while
`Bytes.Total` survives, so `buildTorrentResponse` falls back to the piece ratio
(`PiecesHave`/`PiecesTotal`) whenever `BytesCompleted` reads 0 — otherwise a torrent paused
mid-download renders an empty progress bar for its whole paused lifetime.

---

### 31. Diretório de download derivado da biblioteca

**Location:** `internal/files/filemanager.go` (`Config.DownloadPath`), `internal/files/librarian.go` (`ProbePath`), `internal/daemon/migration.go`.

**What it looks like:** `save_path` sumiu da configuração; o diretório de download é `<completed_anime_path>/.torrents`, calculado a cada uso.

**Why it's right:** a restrição de mesmo-filesystem do `#21` era uma armadilha que só aparecia como erro no save; derivando o caminho ela vira invariante. Dois campos obrigatórios sem diferença clara confundiam o usuário, para quem só a biblioteca importa. `ProbePath` continua existindo porque exFAT/FAT32/alguns SMB não têm hardlink nenhum. O `.ignore` + o prefixo com ponto mantêm o Jellyfin fora da pasta de download.

**Don't "fix" by:** reintroduzir `save_path` como campo de config ou variável de ambiente (re-arma a migração a cada boot); tirar a zeragem de `SavePath` no `PUT /config`; deixar `MigrateSavePath` seguir em frente quando o rename falha (rebaixa tudo em silêncio); tirar a guarda que faz `DeleteEmptyFolders` pular `.torrents`.

---

### 32. `DELETE /torrents/{hash}`: default é apagar + bloquear, `keep_data` é binário, não há endpoint de lote

**Location:** `internal/api/endpoint_torrents.go` (`handleTorrentDelete`, `parseBoolQueryParam`), `internal/daemon/episodes.go` (`RemoveTorrentWithEpisodes`, `RemoveTorrentOptions`, `removeEpisodesAndLinks`), `frontend/src/components/TorrentDeleteDialog.svelte`.

**What it looks like:** a UI de exclusão da tela de Downloads (linha única ou seleção em massa) abre um diálogo com dois checkboxes — "apagar arquivos" e "não baixar de novo" — **ambos marcados por padrão**. Desmarcar o primeiro vira `keep_data=true` (mantém a cópia semeada **e** o hardlink da biblioteca — nunca só um dos dois). Desmarcar o segundo vira `block=false` e a UI mostra um aviso em texto amarelo em vez de impedir a combinação. Não existe `DELETE /torrents/bulk`; o cliente dispara N requisições ao endpoint por hash já existente.

**Why it's right:**

- **Delete + block por padrão:** é a ação que corresponde à intenção mais comum de quem clica em "excluir" — livrar-se do torrent de vez. Deixar os dois desmarcados por padrão faria o caso comum exigir dois cliques extras; deixar só "apagar" marcado por padrão recriaria silenciosamente o torrent no próximo passo do loop (ver próximo ponto).
- **"Excluir sem bloquear" é um rebaixar-do-zero, não um cancelar:** `handleAlreadySavedEpisode` (`daemon/daemon.go`) rebaixa (redownloada) qualquer episódio cujo registro em `episodes.json` exista mas cujo hash não bata com nenhum torrent vivo na sessão — é assim que o daemon se recupera de um torrent removido por fora (crash, limpeza manual do rain). `DELETE /torrents/{hash}` com `block=false` produz exatamente esse estado: registro sobrevive (a menos que `keep_data` também tire os hardlinks), torrent não. Não há como o backend distinguir "usuário quer parar de baixar isso" de "torrent morreu e precisa recomeçar" só olhando o registro — por isso a UI avisa por texto ("sem isso, o daemon pode baixar de novo") em vez do backend proibir ou forçar a combinação. Bloquear é a única forma de expressar "não, mesmo" para esse fluxo.
- **`keep_data` não separa biblioteca de cópia semeada:** o hardlink da biblioteca e o arquivo semeado pelo torrent são o mesmo inode (decisão 21). Apagar um e manter o outro não libera espaço nenhum — só quebra a exibição no Jellyfin ou para de semear sem ganhar disco. `removeEpisodesAndLinks` trata isso como um parâmetro binário único (`keepData`): ambos ficam ou ambos vão.
- **Sem endpoint de lote:** o pior caso realista é dezenas de torrents selecionados, não milhares — N requisições HTTP em paralelo (`Promise.all` no frontend) resolve isso sem exigir semântica de erro parcial no backend (o que falhou? o quê retry?) nem um novo formato de payload. O custo de implementar e manter um body `{hashes: []string}` com resposta por-item não compensa para essa escala.
- **404 do DELETE decidido só pela sessão:** igual a `pause`/`resume`/`announce` (`torrentAction`), a existência do torrent é checada só via `server.Torrents.Get(hash)`. Se o hash não está na sessão mas ainda existem registros de episódio com aquele hash (órfãos — torrent já removido por fora), a rota responde 404 e **não mexe** nesses registros; limpar um registro órfão é responsabilidade de `DELETE /animes/{id}/episodes/{episodeId}`, não desta rota. Misturar as duas responsabilidades tornaria o contrato de erro ambíguo (404 significa "torrent não existe" ou "nada para fazer"?).

**Fora de escopo (investigado, não esquecido):**

- **Limite de downloads simultâneos:** exigiria um campo de config novo, uma fila real no daemon, e a persistência do *motivo* de um torrent estar pausado — a rain persiste o estado pausado (decisão 25), mas não o motivo, então sem esse campo extra o scheduler retomaria por engano um torrent que o usuário pausou de propósito. Vira spec própria.
- **Prioridade por torrent:** `github.com/cenkalti/rain/v2` não tem nenhum conceito de prioridade, nem por torrent nem por arquivo — só `SpeedLimitDownload`/`SpeedLimitUpload` globais. Implementar throttling próprio em cima disso mexeria no caminho quente da lib por um ganho que não compensa. Se a fila de simultâneos acima vier a existir, a ordem da fila já é a prioridade, e um botão "baixar primeiro" que move o item para o topo da fila entrega o valor sem exigir scheduler nenhum.

**Don't "fix" by:** fazer o backend recusar `block=false` sozinho (a combinação é válida e intencional — só não é o padrão); tentar separar biblioteca de cópia semeada adicionando um terceiro parâmetro (mesmo inode, ver decisão 21); adicionar um endpoint de lote "porque é óbvio" sem antes confirmar que a escala realista (dezenas, não milhares) justifica a complexidade extra de erro parcial.

---

### 33. daisyUI fica travado na v4 enquanto o Tailwind for v3 (e o inline de progresso não pode olhar `is_downloaded`)

**Location:** `frontend/package.json` (`daisyui: ^4`), `frontend/tailwind.config.js`, `frontend/src/components/Layout.svelte` (tooltip do WebSocket), `frontend/src/routes/AnimeDetail.svelte` (inline de progresso do torrent), `internal/api/endpoint_anime_episodes.go`.

**What it looks like:** o projeto usa Tailwind 3.4 + daisyUI **4**, não a 5. E a barra de progresso inline na tela do anime aparece com base só em `!torrent.completed` — nunca em `!ep.is_downloaded`.

**Why it's right:**

- **daisyUI 5 é CSS-first e só suporta Tailwind 4.** Rodando pelo plugin JS no Tailwind 3, ela emite todo o CSS de componente dentro de `@layer daisyui.l1.l2.l3` (cascade layers reais), enquanto o preflight do Tailwind 3 sai **sem layer nenhum**. CSS sem layer sempre ganha de CSS em layer, independente de especificidade — então `button { background-color: transparent; padding: 0 }` e `*,::before,::after { border-width: 0 }` do preflight atropelavam `.btn` e `.checkbox`. Resultado visível: botões daisyUI viravam texto puro sem fundo/borda/padding, e checkboxes ficavam literalmente invisíveis (só o ✓ aparecia quando marcado). Nada disso é ajustável por config: não dá para pôr o preflight do Tailwind 3 numa layer (`@layer tw-base { @tailwind base }` faz o Tailwind 3 estourar "no matching @tailwind directive"), nem para tirar a daisyUI 5 das layers.
- **A v4 é a versão feita para o Tailwind 3** e emite CSS sem layer, então convive com o preflight pela cascata normal. Todas as classes em uso aqui (`btn*`, `checkbox*`, `badge*`, `progress*`, `card*`, `menu`, `dropdown*`, `modal*`, `join`, `alert*`, `input-bordered`, `select-bordered`, `table*`, `base-100/200/300`, `base-content/NN`) existem na v4 — inclusive `input-bordered`/`select-bordered`/`btn-outline`, que são idioma v4 e denunciam que o código foi escrito para ela antes de alguém bumpar a major.
- **O histórico do repo é feito de remendos deste mesmo bug**: `--btn-shadow: 0` global, "substituir btn DaisyUI por classes Tailwind diretas" (é daí que vêm os `bg-blue-600`/`bg-red-600` crus em `Status.svelte` e `AnimeDetail.svelte`), e a troca de `data-tip` por `.tooltip-content`. Esse último é API só da v5 e, na v4, renderiza o texto do tooltip **sempre visível** ao lado do ponto de conexão na navbar — por isso o `Layout.svelte` voltou para `data-tip`.
- **`episode_hash != "" ⟺ is_downloaded`.** `handleAnimeEpisodes` preenche `IsDownloaded` e `EpisodeHash` no mesmo `if` (existe registro salvo para aquele nó), e o daemon grava o registro salvo no instante em que o torrent é **adicionado**, não quando termina (`daemon/episodes.go`, `daemon/manual_download.go`). Ou seja, um episódio baixando **já** vem com `is_downloaded: true`. A condição antiga do inline (`torrent && !torrent.completed && !ep.is_downloaded`) exigia uma combinação que a API nunca emite, então a barra jamais aparecia em produção — e o smoke test não pegou porque a fixture montava justamente esse estado impossível.

**Don't "fix" by:** subir a daisyUI para 5 "porque é a mais nova" sem migrar o Tailwind para 4 junto (volta o bug de layer inteiro, e de forma silenciosa: só componentes cujo preflight conflita quebram); usar `.tooltip-content` no lugar de `data-tip`; reintroduzir classes Tailwind cruas no lugar de `btn` "porque o btn não pinta" — isso era sintoma do problema de layer, não limitação da daisyUI; voltar a filtrar o inline de progresso por `!ep.is_downloaded`; escrever fixture de teste com `is_downloaded: false` + `episode_hash` preenchido.

---

### 34. Troca da pasta de download é detectada por marcador duplo, e derruba a sessão da rain

**Location:** `internal/torrents/rootmarker.go`, `internal/torrents/sessionmanager.go` (`Ensure`, `checkRoot`, `writeRoot`), `internal/daemon/verification.go` (`clearLibraryPathsAfterRootSwap`), `internal/daemon/migration.go` (o marcador viaja junto no rename). Fixado por `TestSessionManagerDetectsRootSwap`, `TestSessionManagerRootSwapReportedOnce`, `TestSessionManagerMovedAndRepointedIsNotASwap`, `TestSessionManagerFirstRunAdoptsExistingFolder` e `TestClearLibraryPathsAfterRootSwap`.

**What it looks like:** existem dois arquivos com o mesmo id aleatório — `.aad_root` **dentro** da pasta de download e `download_root.id` na pasta de config. `Ensure` compara os dois a cada passe e, quando divergem, fecha e recria a sessão da rain.

**Why it's right:** a rain segura *file descriptors* abertos. Quando o usuário move a pasta de download (ou manda pra lixeira) com o daemon rodando, os bytes continuam caindo no inode que foi embora enquanto o caminho configurado fica vazio — a UI mostra tudo semeando e **todo** `JobOrganize` falha com `no such file or directory`, queimando as 20 tentativas em silêncio. Nada no processo percebe, porque o caminho em string não mudou: só o inode por trás dele mudou.

O par de marcadores é o que torna isso detectável sem depender de inode (que não existe no Windows):

- o marcador **de dentro** viaja com a pasta, então some do caminho configurado exatamente quando a pasta é movida;
- o id **de fora** fica onde o usuário não mexe, então sobrevive para servir de referência — inclusive quando a pasta é movida com o daemon **parado**.

Recriar a sessão é o conserto completo porque a rain já faz a parte difícil: em `torrent_allocation.go`, se os arquivos não existem (`al.HasMissing`), ela **descarta o bitfield do resume** e rebaixa. Não é preciso mexer no resume db nem remover torrent nenhum.

**Por que o sinal é latch e não retorno do `Ensure`:** os endpoints de download manual também chamam `Ensure`. Se a flag viesse no retorno, uma chamada dessas engoliria a troca — a sessão seria recriada, mas os registros de biblioteca órfãos nunca seriam limpos, e a recuperação ficaria pela metade sem nenhum erro visível. `Ensure` marca `pendingSwap`; só `ConsumeRootSwap`, chamado no passe de verificação, limpa. Fixado por `TestSessionManagerRootSwapSurvivesAnotherEnsure`.

Três propriedades que caíram de graça e são intencionais:

- **Mover a pasta E repontar o config** (`completed_anime_path` para o novo lugar) **não** é troca: o marcador viajou junto, os ids batem, nada é rebaixado. É o caminho "certo" de mover a biblioteca.
- **Primeiro boot / upgrade de uma versão sem marcador** não é troca: sem id de referência não há o que comparar, e adotar a pasta como está é a única leitura segura — a alternativa apaga os registros de uma biblioteca saudável.
- **É edge-triggered:** `Ensure` reporta a troca uma vez e já regrava o marcador, então o passe seguinte volta ao normal.

**Emenda à decisão 29:** `clearLibraryPathsAfterRootSwap` zera `LibraryPaths` — justamente o que a #29 proíbe. A proibição continua valendo para arquivo faltando: o que ela protege é a exclusão deliberada de **um** episódio da biblioteca, que um cheque por `Stat` a cada passe ressuscitaria para sempre. Uma troca de raiz é outro evento: sumiu a pasta inteira para onde os registros apontam, o daemon já está rebaixando o conteúdo dela, e a detecção dispara **uma vez por troca** — nunca vira o laço de ressurreição da #29. Sem essa limpeza a recuperação fica pela metade: os torrents rebaixam, mas os episódios organizados antes da troca ficam com `LibraryPaths` órfão e nunca voltam para a biblioteca.

**Don't "fix" by:** devolver a flag no retorno do `Ensure` em vez do latch (ver acima); trocar os marcadores por comparação de inode (`syscall.Stat_t.Ino` não existe no Windows); tratar erro de leitura do marcador como "sumiu" (uma falha de permissão passaria a apagar os registros da biblioteca — por isso `readRootID` só engole `IsNotExist`); fazer a limpeza de `LibraryPaths` a cada passe em vez de só na troca (aí sim vira a violação da #29); tirar o rename do marcador em `MigrateSavePath` (a migração preserva os hardlinks por rename e passaria a parecer uma troca).

---

### 35. `os.SameFile` no Windows resolve o arquivo tarde, então FileInfo não é snapshot em teste

**Location:** `internal/files/librarian_test.go` (`TestOrganizeReplacesDifferentFileAtDestination`, o `staleAlias`). O uso em produção fica em `internal/files/librarian.go:156`.

**What it looks like:** o teste cria um hardlink extra (`staleAlias`) para o arquivo velho **antes** de chamar `Organize`, e depois compara `os.Stat(staleAlias)` contra o destino — em vez de guardar um `os.Stat(dest)` antes da troca, que seria o jeito óbvio.

**Why it's right:** no Linux `os.Stat` grava dev+inode na hora, então um `FileInfo` é um retrato do arquivo daquele instante. No Windows não: `saveInfoFromPath` (`os/types_windows.go`) guarda **só o caminho**, e `SameFile` chama `loadFileId`, que abre esse caminho de novo no momento da comparação. Um `FileInfo` tirado de `dest` antes da substituição passa a apontar para o arquivo **novo** depois dela, e `os.SameFile(staleInfo, destInfo)` vira sempre `true` — o teste falhava só no job `test-backend-windows`, com o código de produção correto. Manter um segundo nome para o arquivo velho fixa a identidade dele em qualquer plataforma, porque `Organize` substitui com `Remove(dest)` + `link(src, dest)` e o hardlink alternativo sobrevive ao `Remove`.

Em `librarian.go:156` o mesmo `os.SameFile` está correto: os dois `Stat` são feitos na hora da comparação, com os dois caminhos existindo.

**Don't "fix" by:** trocar o alias por um `os.Stat(dest)` guardado antes da troca ("é a mesma coisa e lê melhor" — não é, e só quebra no Windows); marcar o teste como `t.Skip` no Windows (era exatamente o cross-device/hardlink que esse job existe para cobrir); apagar a asserção do arquivo velho por parecer redundante com a de conteúdo (uma mutação que pula a substituição faz as duas falharem, mas a de identidade é a que distingue "relinkou" de "sobrescreveu por cima").

---

### 36. `AppShell` escolhe rail vs tab bar em JS (`matchMedia`), não com `hidden md:flex`/`md:hidden`

**Location:** `frontend/src/components/shell/AppShell.svelte`, `NavRail.svelte`, `NavTabBar.svelte`, `MoreMenu.svelte` (Fase 1 do redesign de UI, spec §5).

**What it looks like:** `AppShell` guarda um booleano `isDesktop` (inicializado sincronamente de `window.matchMedia('(min-width: 768px)')`, atualizado por um listener de `change`) e monta `{#if isDesktop}<NavRail>{:else}<NavTabBar>{/if}` — só um dos dois existe no DOM a qualquer momento. O padrão Tailwind mais comum para "rail no desktop, tab bar no mobile" seria montar os dois sempre e escondê-los por classe (`hidden md:flex` / `md:hidden`), como o `Layout.svelte` antigo fazia para seu bloco desktop/mobile.

**Why it's right:** `NavRail` e `NavTabBar` cada um monta seu próprio `MoreMenu`, que no mobile hospeda um `<select id="theme-select-mobile">` (o desktop tem o seu próprio `<select id="theme-select">`, sempre visível no rodapé do rail). Com os dois blocos sempre no DOM e só escondidos por CSS, ambos existiriam ao mesmo tempo — dois elementos com o mesmo propósito de controle, potencialmente dois ids duplicados se algum dia convergirem, e dois menus "Mais" interativos simultâneos para ferramentas que não respeitam `display:none` puramente por visibilidade (ex. `getByLabelText`/`getByRole` do Testing Library encontram elementos ocultos por CSS a menos que a query filtre por `hidden: true`, e podem falhar com "found multiple elements"). Montar só um dos dois em JS elimina a classe inteira desse problema, ao custo de um listener de `matchMedia` — inicializado de forma síncrona no `<script>` do componente (antes do primeiro render), então não há flash de layout errado.

**Don't "fix" by:** voltar para `hidden md:flex`/`md:hidden` "porque é mais simples e é o padrão Tailwind" — funciona visualmente, mas reintroduz o duplo-DOM que motivou a decisão; inicializar `isDesktop` só dentro de `onMount` (mostraria o layout errado por um frame antes do primeiro paint, já que a leitura síncrona de `matchMedia` no `<script>` é o que evita isso).

---

### 37. Tabelas de largura fixa só a partir de `lg`, e `min-w-0` obrigatório em item de grid que contém faixa rolável

**Location:** `frontend/src/routes/Status.svelte` (`LIST_GRID` e o card herói), `Downloads.svelte` (`ROW_GRID`/`ROW_INDENT`), `AnimeDetail.svelte` (`EP_GRID`). Coberto por `frontend/tests/smoke/layout.spec.ts`.

**What it looks like:** duas coisas que parecem redundantes e não são.

1. A faixa "Downloads ativos" do card herói tem `overflow-x-auto` **e** a `<section>` em volta tem `min-w-0`. Parece cinto e suspensório — o `overflow-x-auto` sozinho deveria bastar para a faixa rolar.
2. As três tabelas trocam para cards empilhados em `lg` (1024px), não em `md` (768px), mesmo o app inteiro usando `md` como breakpoint desktop/mobile (inclusive o `matchMedia` da decisão 36, que continua em 768px).

**Why it's right:**

1. Item de grid tem `min-width: auto`, ou seja, seu piso é o tamanho intrínseco do conteúdo, não zero. Com muitos downloads a `<section>` era esticada até o max-content e o `overflow-x-auto` **nunca ativava**: medido num grid de 1280px, a section ia a 2494px, a faixa ficava com `clientWidth == scrollWidth` e a coluna da direita saía da tela. Com `min-w-0` a mesma section volta a 677px e a faixa rola de verdade (639px visíveis para 2456px de conteúdo). O `overflow-x-auto` descreve a intenção; o `min-w-0` é o que permite que ela aconteça.

2. É aritmética, não gosto: as trilhas fixas somam ~754px (Status), ~740px (Downloads) e ~710px (AnimeDetail), enquanto a largura útil em `md` é `768 − 92 (rail) − 48 (padding do main) ≈ 628px`. O corte em `md` estava errado desde sempre — a faixa 768–880px caía em rolagem horizontal — e só não incomodava porque ninguém abre o app nessa largura. `lg` é o primeiro breakpoint em que as colunas cabem. O breakpoint rail-vs-tab-bar continua em 768px de propósito: é sobre o **shell**, não sobre a largura que uma tabela exige.

**Don't "fix" by:** tirar o `min-w-0` "porque já tem `overflow-x-auto`" (é a ordem invertida: sem `min-w-0` o overflow é decorativo); tirar o `overflow-x-auto` "porque o `min-w-0` já resolve" (aí a faixa corta o conteúdo em vez de rolar); baixar as tabelas de volta para `md` por consistência com o resto do app (refaça a conta acima antes — a largura útil em `md` não cobre nenhuma das três); acrescentar uma coluna de largura fixa sem revisar o comentário de aritmética que mora junto de cada constante de grid.

---

### 38. Em cabeçalho `flex-wrap`, o item `flex-1` leva `min-w-[240px]` — e não `min-w-0`

**Location:** `frontend/src/routes/Downloads.svelte` (bloco do título, no topo do template). Coberto por `frontend/tests/smoke/layout.spec.ts` ("subtitle is not squeezed to one word per line").

**What it looks like:** o bloco de título/subtítulo é `min-w-[240px] flex-1`, contrariando o `min-w-0` que a decisão 37 exige nos itens de grid/flex do resto do app.

**Why it's right:** são problemas opostos. `min-w-0` serve para item que **contém uma faixa rolável** e por isso precisa poder encolher abaixo do conteúdo. Aqui o item é vizinho de uma caixa larga (a barra de banda ↓/↑, 433px de max-content) dentro de um container `flex-wrap`, e a quebra de linha de um container flex decide pelo **tamanho principal hipotético** de cada item — que para `flex-1` (flex-basis 0%) é ~0, e `min-width` é justamente o que entra nesse cálculo. Com `min-w-0` o título contribuía ~0, a barra nunca ia para a linha de baixo e o título ficava com as sobras: medido, 7 linhas de subtítulo em 500px e 4 em 768px, uma palavra por linha. O piso de 240px empurra a barra para a próxima linha em qualquer largura em que ela não caiba.

**Don't "fix" by:** trocar por `min-w-0` "para ficar igual ao resto do app" (é o bug de volta — a regra da decisão 37 vale para item que hospeda scroll, não para item vizinho de conteúdo largo); resolver com breakpoint (`flex-col sm:flex-row`) — conserta 375px e deixa a faixa dos 500–768px espremida, que é exatamente onde o sintoma era pior; assumir que `flex-wrap` sozinho já quebra a linha (ele quebra pelo tamanho hipotético, não pelo tamanho final depois de flexionar).

---

### 39. As pills de filtro de Downloads quebram linha (`flex-wrap`), ao contrário da faixa do card herói

**Location:** `frontend/src/components/DownloadsToolbar.svelte`. Coberto por `frontend/tests/smoke/layout.spec.ts` ("every downloads filter pill is visible without horizontal scroll").

**What it looks like:** a fileira de pills (Todos/Baixando/Semeando/Problemas) é `flex flex-wrap`, sem `overflow-x-auto` e sem `min-w-0` — enquanto a faixa "Downloads ativos" do card herói (decisão 37) e o índice lateral do Config no mobile continuam sendo faixas roláveis.

**Why it's right:** o que a rolagem esconde muda de peso em cada caso. A pill existe para responder "tem algo ali?" pela **contagem**; com `overflow-x-auto` as duas últimas ("Semeando", "Problemas") ficavam fora da vista em tela estreita e a informação só aparecia depois de arrastar — o usuário nem sabia que havia mais filtros. Já a faixa do herói e o índice do Config são listas longas e homogêneas, onde rolar é o degrade correto. Também não leva `min-w-0`: o mínimo automático deste item é o min-content (a pill mais larga), que é exatamente o piso desejado — com `min-w-0` uma pill voltaria a estourar em vez de quebrar.

**Don't "fix" by:** devolver `overflow-x-auto` "por consistência com a faixa do herói" (as duas faixas resolvem problemas diferentes); tirar o `shrink-0` das pills (elas passariam a encolher e o rótulo quebraria no meio, em vez de a pill inteira descer de linha).

---

### 40. O `z-30` do `NavRail` não é redundante com o `z-50` do painel do `MoreMenu`

**Location:** `frontend/src/components/shell/NavRail.svelte` (classe do `<nav>`), `MoreMenu.svelte` (painel `absolute z-50`, backdrop `fixed inset-0 z-40`). Coberto por `frontend/tests/smoke/layout.spec.ts` ("the More menu paints above the page content", "a modal still paints above the nav rail").

**What it looks like:** o painel do menu já é `z-50`, e ainda assim o `<nav>` em volta precisa de `z-30` — parece número sobrando, e a tentação é remover um dos dois.

**Why it's right:** `position: sticky` cria contexto de empilhamento **sempre**, independente de z-index. Sem z-index no `<nav>`, aquele `z-50` só ordena coisas **dentro** do rail, e o rail inteiro pinta na camada z-auto da raiz, em ordem de árvore — perdendo para qualquer elemento posicionado que venha depois no DOM. Foi o que acontecia na tela de Prioridades, cujos cards usam a classe `.card` do daisyUI (`position: relative`): os cards apareciam na frente do menu aberto. Com `z-30` (o mesmo valor do `NavTabBar`, que é `fixed`), o rail inteiro sobe junto. O `z-40` do backdrop e o `z-50` do painel continuam necessários para ordenar o menu **contra o próprio rail**; Modal e Toasts ficam em `z-50` na raiz e por isso continuam acima dos 30 do rail.

**Don't "fix" by:** subir o rail para `z-50` "para ficar igual ao painel" (empataria com Modal/Toasts e o diálogo passaria a depender de ordem de árvore); tirar o `z-30` porque "sticky já fica na frente" (ele fica na frente do conteúdo **não posicionado** — qualquer `position: relative` depois no DOM ganha dele); tirar o `z-50` do painel achando que o `z-30` do nav basta (dentro do rail o painel precisa vencer o backdrop `z-40`).

---

### 41. A fila de downloads contém TODOS os incompletos, é persistida, e `queued` é o único slug que a rain não produz

**Location:** `torrents/queue.go` (`queue`, `enforce`, `markQueued`, `load`/`save`), `torrents/sessionmanager.go`, `torrents/status.go` (`StatusQueued`), `files.Config.MaxConcurrentDownloads`. Frontend: `lib/utils/torrentStatus.ts`, `torrentFilters.ts` (`KNOWN_STATUSES`, `isProblemTorrent`, `prioritizeOrder`), `routes/Downloads.svelte` (`canPause`/`canResume`/`canPrioritize`).

**What it looks like:** um monte de coisa que parece descuido, e é toda deliberada.

**(a) A fila é uma lista de TUDO, não só de quem ela pausou.** `queue.order` contém todo torrent incompleto que a sessão conhece; os `limit` primeiros que não estão em `queue.paused` baixam. A versão anterior guardava só os hashes que a fila tinha pausado, e os três defeitos que isso produzia eram estruturais: priorizar com o limite cheio não privava ninguém (não havia excesso a rebaixar nem folga a promover, então o priorizado só começava quando um slot vagasse por conta própria); priorizar em lote invertia a ordem (cada `pushFront` passava na frente do anterior); e não havia posição estável para mostrar na tela. Com a fila contendo tudo, nada é inserido no meio — a posição só muda por ação do usuário — e some a pergunta "onde este hash entra". Isso apagou `insertByAddedAt`, `leastProgressed` e o desempate por progresso. `AddedAt` sobrevive por um motivo só: dar ordem inicial aos torrents que a fila ainda não conhece (passo 2 do `enforce`).

**(b) `enforce` é reconciliação, não reação a evento — e o passo 0 é o que impede a fila de morrer em todo boot.** `SessionManager.list()` devolve `nil` enquanto não há sessão e slice vazio **não-nil** para uma sessão sem torrents. `nil` significa "não sei nada", não "não há nada": sem a guarda, o passo 1 podaria `order`/`paused` inteiros e o passo 5 gravaria o arquivo vazio por cima — em **todo** boot, porque `verification.go` chama `SetMaxActiveDownloads` (que roda `enforce`) **antes** do `Ensure`. É também o que garante que uma falha se conserta sozinha: todo passe de verificação roda `SetMaxActiveDownloads`, então um `pause` que a rain recusou ou um `stopping` que ficou no meio do caminho se resolve no ciclo seguinte, sem ticker novo nenhum.

**(c) O passo 4 itera `order`, NUNCA a sessão.** Torrent completo não entra em `order` (seeding não ocupa slot), então varrer a sessão marcaria todo seeder como "não deveria estar ativo" e **pausaria a biblioteca inteira**. `order` é a lista dos incompletos e é exatamente o conjunto sobre o qual a fila tem autoridade; o que está fora dela a fila não toca, nem para pausar nem para retomar. Por isso `Pause`/`Resume` de um torrent completo **bypassam a fila nos dois sentidos**, delegando direto para a rain — sem o bypass, e com o passo 4 restrito a `order` (como deve ser), pausar um seeder não pararia nada. Consequência: `paused` guarda só hashes incompletos.

**(d) `stopping` não é alvo de nada.** A pausa da rain é assíncrona (`Torrent.Stop()` volta na hora, o status só chega a `stopped` até ~5s depois). Tratá-lo como parado faria o `enforce` mandar `resume` por cima de um stop em andamento; tratá-lo como rodando faria o passo 4 pausá-lo de novo. Fica de fora dos dois lados e o passe seguinte decide sobre um estado estável.

**(e) A preempção escolhe por POSIÇÃO, não por progresso.** Sai o último dos ativos em ordem de fila — um download em 95% pode ser pausado se estiver depois na fila, coisa que o `leastProgressed` antigo evitava. Aceito: posição é o eixo que o usuário controla, e a alternativa é ressuscitar a peça que (a) apagou. Nada é perdido de qualquer forma: a rain zera `Bytes.Completed` ao parar, mas o bitfield das peças sobrevive.

**(f) `Resume` põe no FIM da fila, não inicia na hora.** Com limite de downloads simultâneos, um resume que iniciasse incondicionalmente furaria o limite em silêncio — e um limite que a UI mostra sendo violado é um limite em que ninguém confia. Quem quer "começa agora" chama `Prioritize`, que vai para a frente e preempta. Por isso a linha de um enfileirado tem **três** botões, cada um numa direção: Priorizar sobe, Play desce para o fim, Pause tira da rotação. Downloads manuais usam `daemon.addAndPrioritize` — furam a fila, não o limite.

**(g) `queued` é slug de status, não campo booleano — e a posição sai do mesmo mapa.** É o único valor de `TorrentInfo.Status` que não sai de `statusSlug()`: para a rain um enfileirado é só mais um torrent parado. A fonte é `q.queued`, o mapa que o passo 3 acabou de calcular, **não** `order` — marcar por pertencer a `order` pintaria a tela inteira de `queued`, já que agora ela contém os ativos também. Quem está em `paused` mostra `stopped` (o que a rain reporta), e `queued` fica significando só "a fila vai chegar nele". `QueuePosition` 0 = não enfileirado, o que inclui todo pausado: ele tem posição em `order`, mas ela não prevê nada, e publicar um número que não vai acontecer é pior do que não publicar nenhum. Quando um `pause` falha, o hash **sai de `q.queued`** mantendo a posição em `order` — a tela mostra o que está acontecendo, ao custo de um buraco na numeração que dura até o próximo passe.

**(h) A fila é persistida em `queue.json`, ao lado do banco de resume.** Mesmo lugar e mesmo motivo do `download_root.id`: é estado do torrent client, não configuração do usuário, e precisa acompanhar o banco de resume. Isso mata o teto que a versão anterior marcava com `ponytail:` — a pausa manual agora sobrevive a restart. A "adoção de boot", que rodava a cada sessão criada e retomava por engano o que o usuário tinha pausado, encolheu para um **latch de uma vez só**: quando não existe `queue.json`, a leitura arma `seedPaused` e o primeiro `enforce` **com sessão** marca como `paused` os incompletos que a rain reporta parados. Não dá para semear na leitura — em `NewSessionManager` ainda não existe sessão para consultar. O preço, real e inevitável: nesse boot de upgrade os torrents que a fila antiga (só memória) pausou são indistinguíveis dos que o usuário pausou, então os dois grupos congelam e precisam de um clique em Play ou Priorizar. É a troca certa — congelar é visível na tela e custa um clique; retomar sozinho é invisível e desfaz uma decisão do usuário gastando banda. Acontece uma vez na vida da instalação, e o boot loga quantos hashes foram semeados.

**(i) O lote ignora o que o individual rejeita.** `POST /torrents/prioritize` aplica **na ordem recebida** (quem decide é o frontend, que sabe o que o usuário clicou) e ignora hash desconhecido ou já completo: uma lista de 12 episódios não pode falhar inteira porque um deles terminou de baixar entre o render e o clique. `POST /torrents/{hash}/prioritize` mantém os erros de sempre — a validação vive em `SessionManager.Prioritize(hash)`, que checa **antes** de delegar para `PrioritizeAll`. Não é o mesmo que N chamadas ao endpoint individual: aquelas se atropelariam e inverteriam o lote. A mensagem de resultado no frontend também foge do `{success} of {total}` das outras ações em lote — é **uma** requisição, então não existe sucesso parcial para contar, e o denominador nem é verificável do lado do cliente.

**Don't "fix" by:** iterar a sessão em vez de `order` no passo 4 (pausa **todo seeder** — ver (c)); chamar `m.Get`/`m.List` de dentro de `PrioritizeAll` (passam por `markQueued`, que pega `queue.mu`: **deadlock**, o mesmo que a linha abaixo descreve para `enforce`); fazer `enforce` ler via `SessionManager.List()` em vez de `list()` (idem — foi assim que a primeira versão travou `TestSessionManagerDetectsRootSwap`); fazer a fila chamar `Pause`/`Resume` públicos em vez de `pause`/`resume` (aí sim recursão infinita); atualizar `lastSaved` sem o `Rename` ter dado certo (uma falha transitória de disco congelaria o arquivo na versão velha para sempre, porque o "só grava quando muda" nunca mais dispararia); inverter a ordem `paused`-antes-do-`pause` em `SessionManager.Pause` (um `enforce` disparado por uma conclusão promoveria justamente o torrent que o usuário está parando); tratar `all == nil` como sessão vazia no passo 0 (apaga a fila persistida em todo boot, silenciosamente); tirar `queued` de `isProblemTorrent` (0 peers é a definição de estar enfileirado, e a pill de problemas ficaria vermelha toda vez que o limite funcionasse); deixar torrent completo contar contra o limite (seeding não custa banda de download e parar de semear é o oposto do que se quer); pôr a posição da fila dentro de `statusLabel()` (a mesma função monta o filtro de status da toolbar, onde "Na fila #7" não significa nada); reordenar ou renumerar as linhas por grupo para os números ficarem contíguos (é justamente a não-contiguidade que comunica que a fila é global).

---

### 42. Todo estado persistido do `FileManager` grava com temp+rename sob `m.mu`, e `fetchAniListEntries` devolve `nil` de propósito

**Location:** `files/filemanager.go` (`writeAtomic`, `loadSavedEpisodesLocked`, `saveEpisodesLocked`, `loadBlockedEpisodesLocked`, `saveBlockedEpisodesLocked`), `files/parser.go` (`ParseEpisodes`), `api/endpoint_animes.go` (`handleAnimes`, `fetchAniListEntries`, `refreshOrphanAnimes`).

**What it looks like:** dois pares de convenções que parecem cerimônia e são cicatriz de um incidente real (07/08/2026): soltar 5 episódios de uma vez pela UI matou a API por 3 minutos e depois prendeu o daemon numa tempestade de 429 na AniList.

**(a) Os métodos públicos de episódios/bloqueados pegam `m.mu` e delegam para um `...Locked`.** Todos fazem read-modify-write do arquivo inteiro, e a UI dispara vários em paralelo — um `POST /release` por episódio, ao mesmo tempo que o daemon chama `UpsertEpisodes` ao organizar torrents. Sem o lock as atualizações se perdem (todas as goroutines leem o mesmo estado inicial e só a última escrita sobrevive) e, pior, dois `WriteFile` truncando o mesmo caminho ao mesmo tempo deixam o arquivo intercalado. Foi assim que `episodes` corrompeu: `LoadSavedEpisodes` passou a falhar, e como `/api/v1/animes` e `/api/v1/animes/{id}/episodes` tratam esse erro como fatal, **os dois passaram a devolver 500 até o restart**. `Config` e `AnimeSettings` já faziam isso certo; episódios e bloqueados eram o buraco. O mutex não é reentrante — daí os `...Locked`.

**(b) `writeAtomic` (temp+rename) para os quatro arquivos, não só o config.** Todo estado aqui é reescrito por inteiro a cada alteração, então `WriteFile` direto deixa uma janela em que o arquivo está pela metade — um leitor concorrente enxerga truncado, e uma queda de energia no meio (Raspberry Pi, cartão SD) deixa truncado para sempre. O `rename` é atômico no mesmo filesystem, então leitor nenhum vê estado intermediário.

**(c) `ParseEpisodes` reporta os DOIS erros.** Para um arquivo já migrado o erro do formato antigo é ruído (ele só reclama que a linha é JSON), e reportar só ele escondia a linha JSONL realmente quebrada. A mensagem que produzia — `invalid episode ID '{"episode_id"'` — apontava para a linha 1 de um arquivo cuja linha 1 estava íntegra, e foi o que mais atrasou o diagnóstico do incidente.

**(d) `fetchAniListEntries` devolve `nil` na falha e slice vazio (não-`nil`) no sucesso — e `handleAnimes` depende disso.** `refreshOrphanAnimes` busca um `GetAnimeInfo` individual por anime que ficou fora do conjunto `covered`. Quando a busca da lista falhava, `covered` ficava vazio e **todo anime com episódio baixado virava órfão**: 16 requests por poll de `/api/v1/animes`, ~10 polls por minuto, com o frontend aberto em duas abas. Isso multiplicava um 500 passageiro da AniList em ~200 requests/min contra um limite de 90 (hoje degradado para 30) — e o 429 resultante fazia a busca da lista falhar de novo, fechando o ciclo: o daemon não saía sozinho. Por isso, com qualquer conta falhando, o refresh de órfãos é **pulado inteiro**: sem a lista não dá para saber o que está coberto, e tratar "não coberto" como "precisa refresh" é justamente o amplificador. Os animes continuam visíveis com os dados locais — degradar campo desatualizado é barato, derrubar a AniList não.

**Don't "fix" by:** chamar o método público de dentro de um `...Locked` (deadlock — `sync.Mutex` não é reentrante); "simplificar" `writeAtomic` de volta para `WriteFile` porque "o lock já resolve" (o lock só protege este processo, não protege queda de energia no meio da escrita); fazer `fetchAniListEntries` devolver slice vazio na falha "porque nil é slice vazio em Go" (apaga a distinção entre "nenhum coberto" e "cobertura desconhecida" e ressuscita a tempestade); tratar o skip do refresh de órfãos como perda de funcionalidade e "só limitar a concorrência" (o `maxConcurrentOrphanRefresh` já existe e não ajuda — ele limita o paralelismo, não o total de requests por poll).

---

### 43. A identidade de um anime é `Media.Id`; status é uma pergunta POR CONTA (download = OR, deleção = AND)

**Location:** `anilist/anilist.go` (`DedupeByMedia`, `GetAnimeInfo`, `GetMediaListStatus`, `GetMediaIDForEntry`), `daemon/verification.go` (`searchAnilist`, `deletableMediaIDs`, `allAccountsAgreeOnDelete`), `daemon/migration.go` (`MigrateAnimeIDsToMedia`), `api/endpoint_animes.go`.

**What it looks like:** três coisas que parecem exageradas — `GetAnimeInfo` recebe a lista de usernames e faz uma requisição por conta para buscar UM anime; `DedupeByMedia` avisa no comentário que ninguém pode ler o `Status` do resultado; e `allAccountsAgreeOnDelete` dispara uma consulta extra por conta que não reportou o anime.

**Why it's right:** `MediaList.Id` é o id da **entrada**, que é por conta — o mesmo anime em duas contas chega como duas entradas com ids diferentes e o mesmo `Media.Id`. Enquanto o `AnimeID` gravado era o id da entrada, ele apontava para **uma** das contas, e o `GET /animes` listava o anime duas vezes: uma sob a chave `id:<AnimeID>` vinda do disco e outra pelo nome, porque a entrada da outra conta não batia com nada conhecido (bug real: "From Old Country Bumpkin to Master Swordsman II", entradas 488911345 e 583631757, media 194829). Pior que a duplicata: qual entrada "vence" dependia do progresso, então o id gravado em disco **mudava** conforme o usuário assistia, e a deleção por status, as settings por anime e a tela de detalhe passavam a procurar por um id que metade dos registros não tinha.

Com `Media.Id` como identidade isso some por construção. O que sobra é o que a identidade única *não* resolve: cada conta tem seu próprio `status` e `progress` para a mesma mídia, e colapsar os dois numa entrada só significa escolher. As escolhas são deliberadamente assimétricas, porque os erros não custam o mesmo:

- **`progress`: o MENOR entre as contas** (`DedupeByMedia`). Um episódio só é "assistido" quando todas as contas o viram; errar para o lado alto apaga episódio que alguém ainda não assistiu.
- **Download: OR** — basta UMA conta ter o anime em status de download. Já sai de graça da união das listas em `searchAnilist`, já que cada busca é filtrada por `status_in` no servidor.
- **Deleção: AND** — TODAS as contas que têm o anime precisam tê-lo em algum status de deleção, e não precisa ser o mesmo (`DROPPED` numa e `COMPLETED` noutra apaga). Uma conta que **não acompanha** o anime não vota; uma que o tem em status neutro (`PLANNING`) **veta**. Distinguir esses dois casos é exatamente o que a consulta extra de `allAccountsAgreeOnDelete` compra: a busca por lista só traz os deletáveis, então "não veio na lista" é ambíguo. Ela só roda para anime **com episódio em disco** que alguma conta quer apagar — um conjunto que se esvazia sozinho, porque o anime some do disco no mesmo passe.

Consequência direta: **ninguém pode ler o `Status` da entrada vencedora do dedup** — ele é de uma conta arbitrária. Quem responde sobre status é `deletableMediaIDs`.

**Migração:** `MigrateAnimeIDsToMedia` resolve cada `anime_id` antigo via `GetMediaIDForEntry` e reescreve `downloaded_episodes` e `anime_settings`, marcando `anime_ids_are_media_ids` no config. Roda no boot **e** no topo do passe de verificação, e o passe **aborta** enquanto ela não tiver rodado: com os ids no formato antigo nada em disco casa com a AniList, e um passe nesse estado rebaixaria a biblioteca inteira. Só escreve depois de resolver tudo, então uma falha de rede no meio não deixa metade convertida. Entradas que já não existem na AniList (404) ficam com o id antigo e são apenas logadas — os episódios continuam em disco.

**Don't "fix" by:** deduplicar por nome (dois animes distintos podem compartilhar título em inglês, e o nome de um registro vindo só do `episodes.json` não vem da AniList); tirar o `media { id }` de `GetFrontendAnimeList`/`GetAllCurrentAnime` por "complexidade" (é um escalar, e sem ele a chave do dedup é `0` — todas as entradas caem no ramo "sem media id" e o bug volta inteiro, silenciosamente); ramificar em `MediaList.Status` depois do dedup (é de uma conta sorteada); trocar o veto por "qualquer conta em status de deleção apaga" (basta uma conta completar um anime para os episódios sumirem debaixo de quem ainda assiste); tratar conta com busca falhada como concordância (transformaria uma falha passageira da AniList em deleção de arquivo, que é irreversível); rodar a migração sem a marca de conclusão (na segunda vez ela leria ids de mídia como ids de entrada).

---

### 44. O `tvshow.nfo` é escrito DEPOIS dos hardlinks, nunca sobrescreve, e falhar nele não falha o organize

**Location:** `internal/files/librarian.go` (`writeShowNFO`, chamado no fim de `Organize`, e `BackfillShowNFOs`); `internal/daemon/jobs.go` (`organizeTorrent` preenche `OrganizeRequest.AnimeID`); `cmd/daemon/main.go` (chamada do backfill no boot).

**What it looks like:** o nfo parece que deveria ser escrito logo depois do `MkdirAll` (é metadado da pasta, não dos arquivos), e um erro ao escrevê-lo parece um erro do `Organize`.

**Why it's right:** três coisas, todas por causa de invariantes que já existiam:

- **Depois dos links:** o rollback de `Organize` é `cleanupIfEmpty`, que usa `os.Remove` e só funciona em pasta **vazia**. Um nfo escrito antes de um hardlink que falha (cross-device, permissão) deixaria a pasta não-vazia e um diretório órfão na biblioteca a cada retry.
- **Não sobrescreve:** se o usuário corrigiu o match à mão no Jellyfin (que reescreve o nfo), o próximo episódio do mesmo anime não pode desfazer isso. `Organize` roda uma vez por torrent, então isso acontece o tempo todo.
- **Só loga em falha:** os hardlinks são o produto; devolver erro aqui faria o job repetir (backoff, e depois dropar) um trabalho que já deu certo, e o `LibraryPaths` nunca seria gravado.

O `AnimeID` é o id de **mídia** da AniList (decisions.md #43) — vem de `EpisodeStruct.AnimeID` já migrado. `AnimeID == 0` (registro antigo não resolvível) simplesmente pula o nfo: o Jellyfin volta a adivinhar pelo nome, que é o comportamento anterior.

**O backfill existe porque `Organize` nao re-roda:** `organizeTorrent` sai cedo quando os episodios do hash ja tem `LibraryPaths`, e `reconcileLibrary` so enfileira quem tem `LibraryPaths` vazio. Sem `BackfillShowNFOs` um anime ja terminado nunca ganharia o arquivo (um em andamento ganharia no proximo episodio). Ele roda **todo boot** e nao tem flag de "ja rodou": o "nao sobrescreve" ja o torna idempotente, e uma flag so criaria um estado a mais para dessincronizar. A pasta vem de `LibraryPaths`, nao de `sanitizeName(AnimeName)` — e o unico caminho que com certeza casa com o que existe em disco depois de renomeacoes manuais ou mudancas na sanitizacao.

**Ele e gated pela migracao de ids** (`main.go`, ramo `else` de `MigrateAnimeIDsToMedia`): com os ids no formato antigo o nfo sairia com um id de ENTRADA, e como nunca e reescrito isso seria permanente. Roda no `else` de propósito — falha de migracao apenas adia o backfill para o proximo boot.

**Requer no Jellyfin:** o plugin `jellyfin-plugin-anilist` como provedor de metadados; sem ele o `uniqueid type="anilist"` é ignorado.

**Don't "fix" by:** mover a escrita para junto do `MkdirAll`; sobrescrever sempre "para manter sincronizado"; propagar o erro de escrita para o retorno de `Organize`; dar uma flag de config ao backfill; derivar a pasta do backfill do nome do anime; rodar o backfill antes/independente da migracao de ids.

---

### 45. Uma pasta de biblioteca por ENTRADA da AniList — o marcador de season fica no nome

**Location:** `internal/files/librarian.go` — `sanitizeName` (era `sanitizeFolderName` + `sanitizeFileName`, agora uma so) e o `destDir` em `Organize`; teste `TestSanitizeNameKeepsSeasonMarker`.

**What it looks like:** o layout "certo" pro Jellyfin/TVDB e uma pasta por serie com `Season 03/` dentro, e ate `cd496c3` o codigo ia meio caminho nessa direcao: um `seasonPattern` apagava "Season 2"/"S2"/"2nd Season"/"Cour 2" do nome da pasta, juntando todas as seasons numa pasta so.

**Why it's right:** na AniList **nao existe** serie com varias seasons. Cada season e uma midia propria, com id, capa, sinopse e numeracao de episodio comecando em 1 — e os nossos dados sao exatamente assim (um `AnimeID` por season, `EpisodeNumber` reiniciando). Juntar as seasons numa pasta so obrigava o Jellyfin a escolher UMA entrada pra pasta inteira, e ele escolhia a season 1: toda season nova aparecia com a capa e os metadados da primeira (relato real: Mushoku Tensei — as entradas 108465, 127720, 146065 e 178789 caiam todas em `Mushoku Tensei Jobless Reincarnation`). O agrupamento nem era coerente: o regex comia "Season 2" mas nao "Part", entao "Season 2 Part 2" ia parar numa quinta pasta.

Com o `tvshow.nfo` (decisions.md #44) isso deixaria de ser um palpite e viraria erro fixo: a primeira season a chegar na pasta gravaria o `uniqueid` dela e, como o nfo nunca e sobrescrito, prenderia a pasta naquela season para sempre.

O layout de franquia com `Season NN/` tambem nao tem como funcionar aqui: o `tvshow.nfo` da franquia precisaria de um id que a AniList nao tem, e o `jellyfin-plugin-anilist` nao fornece metadado por season. Uma pasta por entrada e ao mesmo tempo o diff menor e o unico modelo que casa com o provider.

Como as duas funcoes de sanitizacao ficaram identicas depois de tirar o regex, viraram uma: `sanitizeName`.

**Don't "fix" by:** reintroduzir o strip de season "pra agrupar a franquia"; criar subpastas `Season NN/` (a numeracao de episodio da AniList e por entrada, entao E01 da season 3 e mesmo o episodio 1 da midia dela, nao o 25 da franquia); deduplicar pastas por titulo-base.

---

### 46. `GetFrontendAnimeList` é cacheado por 60s — o poll de `/api/v1/animes` é o que estoura a AniList

**Location:** `internal/anilist/anilist.go` — `ttlCache`, `frontendListCache`, `GetFrontendAnimeList`, `frontendListResponse`, `clearCaches` (chamado por `MockAniListDo`); testes em `anilist_test.go`.

**What it looks like:** um cache com TTL num getter que "só lê uma lista", devolvendo uma cópia da fatia em vez da fatia guardada. Parece otimização prematura em cima de um request que leva ~1s.

**Why it's right:** a AniList hoje limita a **30 req/min** por IP. `Status.svelte` faz poll de `/api/v1/animes` a cada 30s **por aba aberta**, e o handler faz uma busca por conta configurada (`fetchAniListEntries` = `GetCustomListsMap` + `GetFrontendAnimeList`). Com 2 contas e 2 abas isso sozinho são ~16 req/min só de UI parada — some com um F5, com o ciclo de verificação de 10 em 10 minutos e com `GetAnimeInfo` de órfãos e o 429 é garantido. Diagnóstico real (`daemon.log`, 10/08/2026 11:09): 15 requests a `/api/v1/animes` em 2 minutos, seguidos de 429 em cascata que derrubaram junto o `searchAnilist` do daemon (`verification.go:486`) — ou seja, a tela aberta impedia o download automático de rodar. Com o cache, N abas custam no máximo 1 request por minuto por conta.

O TTL de 60s é seguro porque essa lista só muda quando o usuário mexe na AniList; contagem de episódios baixados vem do `episodes.json` local e continua instantânea. A chave inclui username + statuses, então mudança de config gera chave nova (não precisa invalidação).

**Cópia, não a fatia guardada:** `fetchAniListEntries` sobrescreve `ml.CustomLists` nas entradas que recebe. Devolver a fatia do cache faria dois requests concorrentes de `/api/v1/animes` escreverem na mesma memória (corrida real, pega no `-race`) e vazaria o overlay de um chamador pro próximo.

**Don't "fix" by:**
- Cachear `GetAllCurrentAnime` junto — esse é o ciclo do daemon, que roda de 10 em 10 min e **precisa** de dado fresco pra decidir download/deleção.
- Aumentar o intervalo de poll do frontend em vez do cache — não resolve F5, múltiplas abas, nem múltiplos dispositivos na LAN.
- Devolver a fatia do cache direto "pra economizar a cópia".
- Cachear também `GetAnimeInfo` do refresh de órfãos: aquele caminho já está desligado quando a lista falha (decisions.md #42d), que é quando ele amplifica.

---

### 47. Notificações são agrupadas por janela, e o body é escapado só quando o preset é JSON

**Location:** `internal/notifications/notifications.go` — `Notify`, `Flush`, `flushEvent`, `fireBatch`, `buildBatchVars`, `jsonEscape`/`escapeVarsForJSON`/`presetIsJSON`; `NotificationsConfig.BatchWindowSeconds` em `files/filemanager.go`; `notifications.Flush()` no shutdown de `cmd/daemon/main.go`.

**What it looks like:** três coisas que parecem opcionais num disparador de webhook: uma fila com timer, um `Flush()` síncrono no shutdown, e escape de JSON condicionado a um header.

**Why it's right:**

**(a) A janela.** Um backfill de biblioteca dispara um `JobOrganize` por torrent, e cada um fecha com um `download_completed`. Caso real (`daemon.log`, 09/08/2026 17:49→18:22): 67 organizes, os 24 primeiros passaram e o ntfy.sh respondeu **429 nos 53 restantes** — a cota estourou e as notificações foram perdidas sem retry. Como o intervalo mediano entre organizes era de 25s, janela curta não resolve: medido no próprio log, 10s levaria 67 → 53, e 60s leva 67 → 24. Por isso o default é 60s e o campo é configurável.

A fila é **por evento**, não global: um preset inscrito só em `download_failed` não pode receber texto de sucesso, e o filtro por evento que já existia (`slices.Contains(preset.Events, ...)`) continua valendo.

**(b) `len(items) == 1` sai idêntico ao não-agrupado.** `buildVars` virou um wrapper de `buildBatchVars` com um item. É isso que faz ligar o agrupamento não mudar a aparência das notificações de quem recebe uma de cada vez — e é o que os testes `TestNotify_SingleItemBatchMatchesUnbatched` travam.

**(c) `Flush()` é síncrono.** `fireBatch` normalmente dispara em goroutine, mas no shutdown isso é o mesmo que não disparar: o processo sai antes das goroutines rodarem. Daí o parâmetro `wait` e o `wg.Wait()`.

**(d) O escape de JSON depende do `Content-Type`.** Dos 7 presets embutidos, 6 embutem `{{message}}` dentro de uma string JSON (Discord, Slack, Telegram, Gotify, Pushover, Apprise) e 1 manda o body cru (ntfy: `body: '{{message}}'`). O `\n` do agrupamento é JSON inválido dentro de string — e escapar o body do ntfy mandaria a barra-n literal em vez da quebra de linha. Então o escape é aplicado **só no body, e só quando o preset se declara `Content-Type: application/json`**. De brinde isso conserta um bug latente que existia antes do agrupamento: anime com aspas ou barra invertida no nome já quebrava o body desses 6 presets, e o serviço respondia 400 com a notificação sumindo (o log só diz "Webhook returned error status").

A URL fica fora porque escape de URL tem regras próprias, e os headers passam por um strip de `\n`/`\r`: um header com quebra de linha faz o `net/http` **recusar a request inteira**, e o preset do ntfy usa `Title: {{title}}`.

**Don't "fix" by:**
- Trocar a fila por evento por uma fila global "pra mandar menos mensagem ainda" — quebra o filtro `Events` do preset.
- Mandar `{{anime_name}}`/`{{episode}}` do primeiro item quando N > 1: o template passa a mentir sobre os outros N-1.
- Escapar sempre, ou escapar a URL/headers junto: o body de texto puro do ntfy recebe barra-n literal.
- Tirar o `wait` do `Flush` "porque goroutine é mais rápido".
- Adicionar retry com backoff no 429 esperando resolver o caso do log: era cota de serviço, não falha transitória. O que resolve é mandar menos mensagem.

---

### 48. `max_episodes_per_anime` nunca limitou batch — agora a regra é explícita, com quatro tetos em volta

**Location:** `daemon/episodes.go` — `willBatchAnime`, `processAnimeEpisodes` (limite efetivo + tampão), `resolveSearchStrategy`; `daemon/search.go` — `filterBySize`; `daemon/helpers.go` — `checkDiskSpace`/`ErrInsufficientDiskSpace`; `daemon/jobs.go` — `partiallyOrganized`; `api/endpoint_status.go` — `disk_low`.

**What it looks like:** um limite por anime que às vezes não se aplica, um filtro de tamanho que deixa passar tamanho zero, uma guarda de disco que só barra o `Add`, e um booleano de "disco baixo" calculado no servidor para o frontend.

**Why it's right:**

**(a) O limite se aplica só episódio a episódio.** Um batch é **um torrent**: um anime finalizado de 24 episódios com o limite em 12 baixava os 24 arquivos, o `organize` hardlinkava os 24, e `episodes.json` registrava 12 — a tela mostrava 12/24 para sempre com os 24 no disco. Limitar registros não limita bytes nem arquivos; só produz contagem mentirosa. Então `willBatchAnime` decide **antes** do loop, e com ele o limite efetivo é ilimitado.

O que substitui o teto em batch é `max_batch_episodes` (default 30): Naruto/Bleach/Gintama/Detective Conan deixam de virar um torrent único gigante e voltam ao caminho um-a-um, onde o limite por anime vale. Contagem desconhecida (`Media.Episodes == nil`) **não** é elegível a batch — sem saber o tamanho não há como afirmar que o pack cabe no teto. `max_batch_episodes <= 0` é teto **desligado**, e por isso não entra na comparação como número.

O teto também entra no gatilho da Estratégia 2, não só no limite: sem isso um anime de 720 episódios continuaria baixando o pack inteiro para 12 registros — exatamente o defeito.

**(b) O tampão.** Com o limite lançado, se a busca de batch volta vazia (ou o filtro de tamanho a esvazia) a Estratégia 3 assume e baixaria N episódios sem limite. Depois de `resolveSearchStrategy`, se o resultado não é batch, `episodesToDownload` é cortado de volta ao limite; a ordem do `airingSchedule` garante que sobram os mais antigos.

**(c) `Size == 0` passa o filtro de tamanho.** É o valor de `nyaa.parseSize` quando o Nyaa muda o formato da coluna. Descartar tamanho desconhecido trocaria "às vezes baixa um torrent grande" por "não baixa nada": um bug de parsing viraria paralisação silenciosa. O filtro roda **depois** da ordenação por prioridade e preserva a ordem, então o escolhido é o melhor **entre os que cabem**.

**(d) A guarda de disco barra o `Add`, não o passe.** A poda de assistidos, o `deleteEpisodesByStatus` e o `organize` são justamente o que **libera** espaço; um `if disco cheio { return }` no início do passe deixaria o app travado no estado em que não consegue se desentupir. Erro de `statfs` não bloqueia — um volume que não responde (rede, permissão) não é prova de disco cheio. Quando barra, não tenta nenhum magnet e não faz retry (o magnet não é o problema) e notifica `ReasonNoDiskSpace`, que o batch de notificações agrupa.

**(e) O limiar de disco vive no servidor.** `GET /status` devolve `disk_low`, calculado com `min_free_disk_percent`. Um único lugar decide o que é "baixo", e o aviso da tela não pode discordar do que o daemon está fazendo.

**(f) `organizeTorrent` não renotifica um grupo parcialmente organizado.** Se parte do grupo já tem `LibraryPaths`, o torrent já pousou e o webhook já saiu: organiza (é idempotente), grava o marcador nos registros novos, não notifica. Sem isso, o primeiro passe pós-upgrade de uma biblioteca existente — onde a regra (a) cria os registros que faltavam para packs já no disco — viraria enxurrada de `download_completed` duplicado.

**Don't "fix" by:**
- Aplicar `max_episodes_per_anime` em batch "para o limite valer sempre": volta a contagem mentirosa.
- Colocar `totalEpisodes <= configs.MaxBatchEpisodes` sem o guarda de `<= 0`: desligar o teto passaria a ligá-lo para todo mundo.
- Descartar torrent com `Size == 0` "porque tamanho desconhecido é suspeito".
- Consolidar a guarda de disco num `return` no topo do passe de verificação (ver (d)).
- Mover a guarda para `torrents.Session.Add` "porque é um site só": o pacote `torrents` não conhece `files.Config`, e passar a config para lá inverte a dependência.
- Reintroduzir um ratio hardcoded em `lib/utils/status.ts` para economizar um campo na resposta (ver (e)).
