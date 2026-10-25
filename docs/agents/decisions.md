# Architectural Decisions

Patterns that look wrong but are intentional. Read before "fixing" anything.

## Index

The entries are **not in numeric order** in the file — use this index, or `grep -n "^### 59\."`.
Each entry is self-contained: read only the one the reference points to, not the whole file.

- [**#1** — Var-swap mocking for anilist and nyaa HTTP clients](#1-var-swap-mocking-for-anilist-and-nyaa-http-clients)
- [**#2** — FileManagerInterface declared twice](#2-filemanagerinterface-declared-twice)
- [**#3** — JSONL format for episodes.json with full-file rewrite on save](#3-jsonl-format-for-episodesjson-with-full-file-rewrite-on-save)
- [**#4** — cancelPtr / donePtr pointer mutation for runtime interval updates](#4-cancelptr--doneptr-pointer-mutation-for-runtime-interval-updates)
- [**#5** — State notifier called outside the mutex lock](#5-state-notifier-called-outside-the-mutex-lock)
- [**#6** — Non-blocking send on WebSocket broadcast channel](#6-non-blocking-send-on-websocket-broadcast-channel)
- [**#7** — Never-closing channel as headless tray fallback](#7-never-closing-channel-as-headless-tray-fallback)
- [**#8** — Hard part filter — nil-part torrents rejected when requestedPart is set](#8-hard-part-filter--nil-part-torrents-rejected-when-requestedpart-is-set)
- [**#9** — PREQUEL offset gate requires part ≥ 2](#9-prequel-offset-gate-requires-part--2)
- [**#10** — "Cour N" treated as Part N, not as a distinct concept](#10-cour-n-treated-as-part-n-not-as-a-distinct-concept)
- [**#11** — GetCustomListsMap — separate lightweight query + cache for customLists](#11-getcustomlistsmap--separate-lightweight-query--cache-for-customlists)
- [**#12** — Build logic lives in scripts/build.sh, not in Makefile targets](#12-build-logic-lives-in-scriptsbuildsh-not-in-makefile-targets)
- [**#13** — GetFrontendAnimeList — separate lighter Anilist query for the API endpoint](#13-getfrontendanimelist--separate-lighter-anilist-query-for-the-api-endpoint)
- [**#14** — nyaaSearcher — dependency injection for Nyaa search in processAnimeEpisodes](#14-nyaasearcher--dependency-injection-for-nyaa-search-in-processanimeepisodes)
- [**#15** — atomic.Pointer[Priorities] package-level global in nyaa](#15-atomicpointerpriorities-package-level-global-in-nyaa)
- [**#16** — anime_id is the AniList **media** ID (it was the MediaList entry ID until the migration)](#16-anime_id-is-the-anilist-media-id-it-was-the-medialist-entry-id-until-the-migration)
- [**#17** — --debug-anime early-exit branch in cmd/daemon/main.go](#17---debug-anime-early-exit-branch-in-cmddaemonmaingo)
- [**#18** — extractTitleTokens truncates the torrent name at the first episode/season marker before tokenizing](#18-extracttitletokens-truncates-the-torrent-name-at-the-first-episodeseason-marker-before-tokenizing)
- [**#19** — Disk space is read via OS stat on CompletedAnimePath](#19-disk-space-is-read-via-os-stat-on-completedanimepath)
- [**#20** — ExtractSeason has a roman-numeral fallback kept out of reSeasonPatterns](#20-extractseason-has-a-roman-numeral-fallback-kept-out-of-reseasonpatterns)
- [**#21** — Embedded torrent client + hardlink-into-library model (replaces qBittorrent)](#21-embedded-torrent-client--hardlink-into-library-model-replaces-qbittorrent)
- [**#22** — Organize everything to completed_anime_path, and the batch-hygiene deletion limitation](#22-organize-everything-to-completed_anime_path-and-the-batch-hygiene-deletion-limitation)
- [**#23** — Integration tests skip unless DAEMON_URL is set explicitly](#23-integration-tests-skip-unless-daemon_url-is-set-explicitly)
- [**#24** — A failed torrent is dropped from the session and re-added by the next pass — no blacklist](#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist)
- [**#25** — Seeding is created at startup and is independent of the daemon loop](#25-seeding-is-created-at-startup-and-is-independent-of-the-daemon-loop)
- [**#26** — The hardlink probe runs on every verification pass, not just on config save](#26-the-hardlink-probe-runs-on-every-verification-pass-not-just-on-config-save)
- [**#27** — saveEpisodesToFile merges by hand — it uses neither SaveEpisodesToFile nor a bare UpsertEpisodes](#27-saveepisodestofile-merges-by-hand--it-uses-neither-saveepisodestofile-nor-a-bare-upsertepisodes)
- [**#28** — Organize replaces a library file that has the same name but different bytes](#28-organize-replaces-a-library-file-that-has-the-same-name-but-different-bytes)
- [**#29** — Startup reconciliation keys off empty LibraryPaths, not off the hardlink missing from disk](#29-startup-reconciliation-keys-off-empty-librarypaths-not-off-the-hardlink-missing-from-disk)
- [**#30** — Progress data comes from one Stats() per torrent, pulled only while a screen is open](#30-progress-data-comes-from-one-stats-per-torrent-pulled-only-while-a-screen-is-open)
- [**#31** — Download directory is derived from the library](#31-download-directory-is-derived-from-the-library)
- [**#32** — DELETE /torrents/{hash}: default is delete + block, keep_data is binary, there is no batch endpoint](#32-delete-torrentshash-default-is-delete--block-keep_data-is-binary-there-is-no-batch-endpoint)
- [**#33** — The torrent progress inline must not look at is_downloaded](#33-the-torrent-progress-inline-must-not-look-at-is_downloaded)
- [**#34** — A download-folder swap is detected by a dual marker, and it tears down the rain session](#34-a-download-folder-swap-is-detected-by-a-dual-marker-and-it-tears-down-the-rain-session)
- [**#35** — os.SameFile on Windows resolves the file late, so FileInfo is not a snapshot in tests](#35-ossamefile-on-windows-resolves-the-file-late-so-fileinfo-is-not-a-snapshot-in-tests)
- [**#36** — AppShell picks rail vs tab bar in JS (matchMedia), not with hidden md:flex/md:hidden](#36-appshell-picks-rail-vs-tab-bar-in-js-matchmedia-not-with-hidden-mdflexmdhidden)
- [**#37** — Fixed-width tables only from lg up, and min-w-0 is required on a grid item that contains a scroll strip](#37-fixed-width-tables-only-from-lg-up-and-min-w-0-is-required-on-a-grid-item-that-contains-a-scroll-strip)
- [**#38** — In a flex-wrap header, the flex-1 item gets min-w-[240px] — not min-w-0](#38-in-a-flex-wrap-header-the-flex-1-item-gets-min-w-240px--not-min-w-0)
- [**#39** — A FINITE choice strip wraps (flex-wrap); an UNBOUNDED content strip scrolls](#39-a-finite-choice-strip-wraps-flex-wrap-an-unbounded-content-strip-scrolls)
- [**#40** — NavRail's z-30 is not redundant with the MoreMenu panel's z-50](#40-navrails-z-30-is-not-redundant-with-the-moremenu-panels-z-50)
- [**#41** — The download queue holds EVERY incomplete torrent, is persisted, and queued is the only slug rain never produces](#41-the-download-queue-holds-every-incomplete-torrent-is-persisted-and-queued-is-the-only-slug-rain-never-produces)
- [**#42** — Every persisted FileManager state writes with temp+rename under m.mu, and fetchAniListEntries returns nil on purpose](#42-every-persisted-filemanager-state-writes-with-temprename-under-mmu-and-fetchanilistentries-returns-nil-on-purpose)
- [**#43** — An anime's identity is Media.Id; status is a PER-ACCOUNT question (download = OR, deletion = AND)](#43-an-animes-identity-is-mediaid-status-is-a-per-account-question-download--or-deletion--and)
- [**#44** — tvshow.nfo is written AFTER the hardlinks, never overwrites, and failing it does not fail organize](#44-tvshownfo-is-written-after-the-hardlinks-never-overwrites-and-failing-it-does-not-fail-organize)
- [**#45** — One library folder per AniList ENTRY — the season marker stays in the name](#45-one-library-folder-per-anilist-entry--the-season-marker-stays-in-the-name)
- [**#46** — GetFrontendAnimeList is cached for 60s — the /api/v1/animes poll is what blows AniList's quota](#46-getfrontendanimelist-is-cached-for-60s--the-apiv1animes-poll-is-what-blows-anilists-quota)
- [**#47** — Notifications are grouped by window, and the body is escaped only when the preset is JSON](#47-notifications-are-grouped-by-window-and-the-body-is-escaped-only-when-the-preset-is-json)
- [**#48** — max_episodes_per_anime never limited batches — the rule now lives on the pack window, not a count cap](#48-max_episodes_per_anime-never-limited-batches--the-rule-now-lives-on-the-pack-window-not-a-count-cap)
- [**#49** — Standalone anime: "tracked by the list" is the snapshot the daemon PROCESSES, and DownloadStandaloneAnime never calls handleSavedEpisodes](#49-standalone-anime-tracked-by-the-list-is-the-snapshot-the-daemon-processes-and-downloadstandaloneanime-never-calls-handlesavedepisodes)
- [**#50** — Standalone search: the unreleased filter is server-side and by concatenation, and a blocked card becomes a link instead of a dead end](#50-standalone-search-the-unreleased-filter-is-server-side-and-by-concatenation-and-a-blocked-card-becomes-a-link-instead-of-a-dead-end)
- [**#51** — An episode number accepts 4 digits, except inside brackets](#51-an-episode-number-accepts-4-digits-except-inside-brackets)
- [**#52** — The episode list is synthesized, and an episode's key is (anime, number)](#52-the-episode-list-is-synthesized-and-an-episodes-key-is-anime-number)
- [**#53** — Episode search drops batches and movies, and uses hasMovieMarker — not isMovie](#53-episode-search-drops-batches-and-movies-and-uses-hasmoviemarker--not-ismovie)
- [**#54** — Seeders floor: unreadable seeders are discarded, unreadable size passes](#54-seeders-floor-unreadable-seeders-are-discarded-unreadable-size-passes)
- [**#55** — health compares a seeders BAND, and it comes before fansub](#55-health-compares-a-seeders-band-and-it-comes-before-fansub)
- [**#56** — A long series also searches the episode with zero-padding, and the gate is LastAiredEpisode](#56-a-long-series-also-searches-the-episode-with-zero-padding-and-the-gate-is-lastairedepisode)
- [**#57** — Adaptive pagination: sequential, with a candidate floor, no per-anime budget](#57-adaptive-pagination-sequential-with-a-candidate-floor-no-per-anime-budget)
- [**#58** — RunAnimeDebug mirrors processAnimeEpisodes: enumeration, pack resolution and single fallback](#58-runanimedebug-mirrors-processanimeepisodes-enumeration-pack-resolution-and-single-fallback)
- [**#59** — Batch eligibility stopped being metadata and became a result filter](#59-batch-eligibility-stopped-being-metadata-and-became-a-result-filter)
- [**#60** — The pack window covers from the first pending episode, and the cut is always a prefix](#60-the-pack-window-covers-from-the-first-pending-episode-and-the-cut-is-always-a-prefix)
- [**#61** — A pack episode is immune to the limit prune AND to the watched keep-set](#61-a-pack-episode-is-immune-to-the-limit-prune-and-to-the-watched-keep-set)
- [**#62** — Standalone progress lives in AnimeSettings and is injected into the synthetic MediaList, it does not become a parallel path](#62-standalone-progress-lives-in-animesettings-and-is-injected-into-the-synthetic-medialist-it-does-not-become-a-parallel-path)
- [**#63** — Dismissing the first-steps card lives in localStorage, not in config.json](#63-dismissing-the-first-steps-card-lives-in-localstorage-not-in-configjson)
- [**#64** — The first-steps card steps are ticked by hand, not derived from daemon state](#64-the-first-steps-card-steps-are-ticked-by-hand-not-derived-from-daemon-state)
- [**#65** — AniList budget is spent by the orphan refresh, not by the daemon pass — and a 200's errors field is not read](#65-anilist-budget-is-spent-by-the-orphan-refresh-not-by-the-daemon-pass--and-a-200s-errors-field-is-not-read)
- [**#66** — A banner is degraded state; a toast is an action failure — and the banner has three sources with precedence, not three banners](#66-a-banner-is-degraded-state-a-toast-is-an-action-failure--and-the-banner-has-three-sources-with-precedence-not-three-banners)
- [**#67** — The verification pass is one at a time, and a concurrent one is dropped — not queued](#67-the-verification-pass-is-one-at-a-time-and-a-concurrent-one-is-dropped--not-queued)
- [**#68** — Pack numbering: the offset is guessed from the files themselves, and a basename collision uses the relative path](#68-pack-numbering-the-offset-is-guessed-from-the-files-themselves-and-a-basename-collision-uses-the-relative-path)
- [**#69** — codec now applies to episodes, and a name with no codec tag ties instead of losing](#69-codec-now-applies-to-episodes-and-a-name-with-no-codec-tag-ties-instead-of-losing)
- [**#70** — Resolution is canonicalized in the extractor, and codec presets are a frontend reordering](#70-resolution-is-canonicalized-in-the-extractor-and-codec-presets-are-a-frontend-reordering)
- [**#71** — AniList has no franchise id: the PREQUEL chain is the only source, and searching by name is a trap](#71-anilist-has-no-franchise-id-the-prequel-chain-is-the-only-source-and-searching-by-name-is-a-trap)
- [**#72** — AniList budget is measured from the response headers, not from a counter of ours](#72-anilist-budget-is-measured-from-the-response-headers-not-from-a-counter-of-ours)
- [**#73** — The frontend doesn't query AniList directly, even though it could](#73-the-frontend-doesnt-query-anilist-directly-even-though-it-could)
- [**#74** — The pack-exclusion guard's unit is the torrent's contents, not the list of records](#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records)
- [**#75** — Truncation at the marker is the first attempt of the title match, not the only one](#75-truncation-at-the-marker-is-the-first-attempt-of-the-title-match-not-the-only-one)
- [**#76** — searchNyaaWithVariants stops at the first variant that returns any result — measured, and it stays that way](#76-searchnyaawithvariants-stops-at-the-first-variant-that-returns-any-result--measured-and-it-stays-that-way)
- [**#77** — The per-series absolute axis is a BFS two generations at a time, and the cut level goes back to the queue](#77-the-per-series-absolute-axis-is-a-bfs-two-generations-at-a-time-and-the-cut-level-goes-back-to-the-queue)
- [**#78** — A torrent's unit of ownership is coverage on the absolute axis, not the (anime_id, episode) key](#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)
- [**#79** — Pack choice asks "does it cover the window?", not "is it part N?" — and pack numbering is a guess among three hypotheses](#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses)
- [**#80** — The pack size ceiling is applied TWICE: in the Nyaa search and in the daemon filter](#80-the-pack-size-ceiling-is-applied-twice-in-the-nyaa-search-and-in-the-daemon-filter)
- [**#81** — Report-code precedence is a cascade, not a set](#81-report-code-precedence-is-a-cascade-not-a-set)
- [**#82** — SetLastCheckError clears the last-check report](#82-setlastcheckerror-clears-the-last-check-report)
- [**#83** — The pack name's range is the first plausible match, not the first match](#83-the-pack-names-range-is-the-first-plausible-match-not-the-first-match)
- [**#84** — A pack without a range in its name gets its coverage from the file list, not from assuming it covers everything](#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)
- [**#85** — The frontend's AniList cost is per ACCOUNT and per ORPHAN, not per tab](#85-the-frontends-anilist-cost-is-per-account-and-per-orphan-not-per-tab)
- [**#86** — The Test button answers the service's status, not "the preset exists"](#86-the-test-button-answers-the-services-status-not-the-preset-exists)
- [**#87** — Webhook headers become a list of pairs while the form is open](#87-webhook-headers-become-a-list-of-pairs-while-the-form-is-open)
- [**#88** — The progress bar derives "downloaded" from `episodes_pending`, not from `episodes_downloaded`](#88-the-progress-bar-derives-downloaded-from-episodes_pending-not-from-episodes_downloaded)
- [**#89** — There is no OpenAPI spec: the architecture.md table is the only endpoint doc](#89-there-is-no-openapi-spec-the-architecturemd-table-is-the-only-endpoint-doc)
- [**#90** — There is no `config.json` versioning: the migration checks SHAPE, in one place](#90-there-is-no-configjson-versioning-the-migration-checks-shape-in-one-place)
- [**#91** — There is no component library: daisyUI left because it was a second color vocabulary](#91-there-is-no-component-library-daisyui-left-because-it-was-a-second-color-vocabulary)
- [**#92** — The on-disk AniList cache restores with TTL zero, and a value served from it comes with `ErrFromCache`](#92-the-on-disk-anilist-cache-restores-with-ttl-zero-and-a-value-served-from-it-comes-with-errfromcache)
- [**#93** — The disk guard has two triggers, and the size one still applies with `min_free_disk_percent = 0`](#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_percent--0)
- [**#94** — A stalled torrent is dropped through the failure path, and only one that is *trying* counts as stalled](#94-a-stalled-torrent-is-dropped-through-the-failure-path-and-only-one-that-is-trying-counts-as-stalled)
- [**#95** — MyAnimeList is a list source, not a metadata source](#95-myanimelist-is-a-list-source-not-a-metadata-source)
- [**#96** — A first-season entry rejects later-season packs, but a sequel without a marker doesn't](#96-a-first-season-entry-rejects-later-season-packs-but-a-sequel-without-a-marker-doesnt)
- [**#97** — A pack that doesn't fit on disk is dropped before the choice, not at download time](#97-a-pack-that-doesnt-fit-on-disk-is-dropped-before-the-choice-not-at-download-time)
- [**#98** — A large release without an episode number is an unlabeled pack, and only its file list makes it a pack](#98-a-large-release-without-an-episode-number-is-an-unlabeled-pack-and-only-its-file-list-makes-it-a-pack)
- [**#99** — A roman-numeral sequel searches by its bare base title first](#99-a-roman-numeral-sequel-searches-by-its-bare-base-title-first)
- [**#100** — An empty per-episode search waits before repeating, and stops the later episodes](#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes)
- [**#101** — The disk guard counts what the queue still has to write, not only free space](#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space)
- [**#102** — The bug report has no backend: consent is the act of attaching the file](#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file)
- [**#103** — Log redaction is by key, and a test forces every new key to be classified](#103-log-redaction-is-by-key-and-a-test-forces-every-new-key-to-be-classified)
- [**#104** — `--yes` is a CLI gate; the API keeps its query defaults](#104---yes-is-a-cli-gate-the-api-keeps-its-query-defaults)
- [**#105** — `FetchTorrentDetail` builds the view URL itself; it never takes one from the caller](#105-fetchtorrentdetail-builds-the-view-url-itself-it-never-takes-one-from-the-caller)
- [**#106** — Empty first search skips the picker; custom `q` drops variants; description is on demand; add-with-magnet writes the id after the torrent](#106-empty-first-search-skips-the-picker-custom-q-drops-variants-description-is-on-demand-add-with-magnet-writes-the-id-after-the-torrent)
- [**#107** — Flags after positionals are reordered before urfave parses](#107-flags-after-positionals-are-reordered-before-urfave-parses)

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

**What it looks like:** The file is JSONL (one JSON object per line) but `saveEpisodesToFile` rewrites the entire file, not just appends new lines.

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

**Location:** `internal/nyaa/nyaa.go` — `ScrapNyaa`, `ScrapNyaaForAnime`

**What it looks like:** When `requestedPart != nil`, torrents whose name has no part marker are rejected, even though they might be the right episode. Looks overly strict.

**Why it's right:** Split-season animes (e.g. Mushoku Tensei II Part 1 / Part 2) have the same episode numbers in both halves. Without a hard part filter, Part 1 torrents would be downloaded for Part 2 entries. A torrent with no part marker in its name is ambiguous and must be treated as the wrong release when the caller knows it wants a specific part. The false-negative cost (missing a valid torrent) is lower than the false-positive cost (wrong episode downloaded silently).

**Don't "fix" by:** accepting nil-part torrents as "probably correct." The offset fallback (see decision 9) handles the rare case where no part-marked torrent exists.

---

### 9. PREQUEL offset gate requires part ≥ 2

**Location:** `internal/daemon/helpers.go` — `ComputeEpisodeOffset`

**What it looks like:** `ComputeEpisodeOffset` always returns 0 unless `part >= 2`, even when a PREQUEL relation exists. Looks like a missing case.

**Why it's right:** The offset (PREQUEL episode count) is only needed for split-season Part 2 releases where the torrent site numbers episodes globally (e.g. episode 14 on Nyaa when Anilist calls it episode 1 of Part 2). Kaguya-sama Season 2, for example, also has a PREQUEL relation with 12 episodes — applying a +12 offset there would search for episode 13 when the correct episode is 1. The gate `part >= 2` restricts the offset to the specific case it was designed for, preventing all single-season animes with prior seasons from receiving a wrong offset.

The same reasoning applies to the PREQUEL **format**: only `TV`/`TV_SHORT` enter the count. A recap
movie or an OVA also arrives as `PREQUEL` with `Episodes` filled, but they are not part of the
numbering fansubs follow — adding them would send the search to an episode that does not exist.
When there is no TV PREQUEL, the offset is 0 and the search falls back to relative numbering, which
is what a large share of groups uses. Erring toward 0 is cheap; erring toward a number is a search
that finds nothing.

**Don't "fix" by:** removing the gate or applying the offset whenever a PREQUEL exists. That breaks every multi-season anime that does not use split-part numbering. Nor by accepting a PREQUEL of any format.

---

### 11. `GetCustomListsMap` — separate lightweight query + cache for `customLists`

**Location:** `internal/anilist/anilist.go` — `GetCustomListsMap`; called at the top of `searchAnilist` (`verification.go`) and `fetchAniListEntries` (`api/endpoint_animes.go`) before `GetAllCurrentAnime`.

**What it looks like:** We call Anilist twice per verification cycle: once with a minimal `id + customLists` query, and once with the full `GetAllCurrentAnime` query. Then we overwrite `ml.CustomLists` from the first result when the second comes back with `null`. Looks like redundant work and a band-aid.

**Why it's right:** Anilist's GraphQL engine enforces a query-complexity budget. `GetAllCurrentAnime` includes `airingSchedule.nodes` (which is large for currently-releasing anime — it contains all future episode schedules) plus `relations`, `synonyms`, and `coverImage`. For users watching many currently-airing animes, this query exceeds Anilist's complexity threshold, causing the API to return `null` for the `customLists` scalar on every entry in the response — silently, with HTTP 200 and no error field. COMPLETED/DROPPED animes are unaffected because their `airingSchedule` is empty.

**Root-cause confirmed by:** back-to-back curl tests: Run 1 returned 0/20 null, Runs 2–5 returned 20/20 null (rate-limit exhausted). The user also ran a manual minimal GraphQL query (only `id` + `customLists`) which returned correct data including `{"AutoDownloader": true}` — confirming the API works; only the complex query suppresses the field.

**Effect when broken:** `customLists` is always `null` → `animeIsInExcludedList` never matches → blacklisted animes (e.g. in the "AutoDownloader" custom list used to block titles) are downloaded anyway and the frontend block icon is never shown.

**Budget gate:** when the caller passes `PriorityDisposable` and the gate refuses, the function
serves the **stale** cache read instead of returning `nil` — see #72, "The poll pair degrades
together or not at all".

**Cache:** `GetCustomListsMap` caches results keyed by `username + statuses` (`ttlCache`, see
decisions.md #46). A response with at least one non-null `CustomLists` stays 5 minutes
(`customListsTTL`); an empty response stays only 30 seconds (`customListsEmptyTTL`), because
"empty" can be either an account with no custom lists or a field AniList degraded. Empty used to
not be cached at all, and then an account with no custom lists generated one request per frontend
poll — exactly the amplifier that blew the limit. The cache read happens before the fetch, so no
degraded response gets to evict an still-valid entry.

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

**Location:** `internal/daemon/search.go` (`nyaaSearcher` struct + `defaultNyaaSearcher()`); `internal/daemon/episodes.go` (`processAnimeEpisodes`).

**What it looks like:** `processAnimeEpisodes` receives a `nyaaSearcher` struct with function-valued fields (`searchAnime`, `searchSingleEpisode`, `searchMovie`) instead of calling `searchNyaaForAnime`, `searchNyaaForMovie`, etc. directly. Looks like unnecessary indirection — these are pure functions with no state.

**Why it's right:** The `episodeInTorrents` hash-check fix prevents batch torrents from being re-downloaded in a loop. Without injection, it's impossible to write a test that proves the loop can't recur: a test that calls the real Nyaa makes a live HTTP request, is flaky, and can't observe whether `searchAnime` was invoked. Injection lets the regression test (`TestProcessAnimeEpisodes_BatchNoRedownload`) confirm both that `searchAnime` is never called and that `POST /add` is never sent when all episode hashes already match.

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

### 16. `anime_id` is the AniList **media** ID (it was the MediaList entry ID until the migration)

**Location:** `internal/anilist/anilist.go` (`Media.Id`, `GetAnimeInfo(mediaId, usernames)`); `internal/api/endpoint_animes.go` and `endpoint_anime_episodes.go` (`AnimeID` fields); `daemon/migration.go` (`MigrateAnimeIDsToMedia`).

**What it looks like:** `anime_id` is *the* anime identifier everywhere (settings file, episode records, route parameters) and also the component of the URL `anilist.co/anime/{id}` — there is no longer a separate `anilist_id` in the API.

**Why it's right:** until the migration, the value came from the `id` field of AniList's `MediaList` type, which is the **list-entry** id and therefore **per account**. With more than one account configured, the same anime had two `anime_id`s, and the app treated them as two animes. See decision #43 for the concrete bug and the current design. Old installations are converted once by `MigrateAnimeIDsToMedia`, marked by the config field `anime_ids_are_media_ids`.

**Don't "fix" by:** keying anything off `MediaList.Id` again (it is per-account — see #43); running the migration again after the mark is set (it would read media ids as entry ids).

---

### 17. `--debug-anime` early-exit branch in `cmd/daemon/main.go`

**Location:** `cmd/daemon/main.go` — `runDebugAnime`, the `flag.Int("debug-anime", ...)` check at the top of `main()`.

**What it looks like:** `main()` parses a flag and, if set, runs a completely different code path (`runDebugAnime`) and returns — skipping the PID file, API server, tray, and daemon loop entirely. Looks like a debug hack that snuck into production entry point.

**Why it's right:** It's a deliberate one-shot diagnostic mode (`make debug-anime ID=<anilistId>` / `go run ./src/cmd/daemon --debug-anime <id>`) for the recurring "why didn't this anime download" problem. It reuses real production functions (`daemon.RunAnimeDebug` → `checkEpisode`, `partitionSearchResults`/`pickBatches`) so the debug output can't drift from actual verification-loop behavior, and it deliberately avoids touching the torrent client so it can run without the daemon up (the torrent client is embedded, so there is nothing external to start either way). See `docs/agents/troubleshooting-downloads.md` Step 0 and `daemon/debug.go`.

**Don't "fix" by:** moving this behind the HTTP API (it exists specifically to work without a running daemon) or deleting it as dead code (it's the primary entry point for the fast-path troubleshooting flow).

---

### 18. `extractTitleTokens` truncates the torrent name at the first episode/season marker before tokenizing

**Location:** `internal/nyaa/nyaa_match.go` — `truncateAtFirstMarker`, `reLeadingBracket`, called at the top of `extractTitleTokens`.

**What it looks like:** Tokenizing only the substring before the first `S01E05`/`- 05`/`Episode 3`-style marker, and separately stripping a leading `[Group]`/`(Group)` tag, instead of just tokenizing the whole torrent name. Looks like it would drop legitimate title words.

**Why it's right:** Real torrent names often carry the episode's plot title, streaming-service tag, or a duplicate alt-title in parentheses *after* the episode marker (e.g. `KAIJU GIRL CARAMELISE S01E01 The Kaiju Girl Appears in Tokyo 1080p CR WEB-DL ... (Otome Kaijuu Caramelise, Multi-Subs)`). None of that belongs to the anime's core title, but it isn't covered by the fixed `titleTechnicalTokens` allowlist (unknown fansub tags like `varyg`/`ironclad`, stray split tokens like `h` from `H 264`, episode-title prose). Tokenizing it anyway inflates the Jaccard union enough that `jaccardThreshold` (0.8 for ≤3-token queries) rejects genuine matches — this caused two real currently-airing anime to never download (see debug session that produced this decision). Truncating at the marker (reusing `reEpisodePatterns`/`reSeasonPatterns`, already used by `ExtractEpisodeNumber`/`ExtractSeason`) removes exactly the noise while preserving genuine extra title words that appear *before* the marker (e.g. "Alternative Gun Gale Online" in a Sword Art Online spinoff), so the existing spin-off-rejection tests still pass.

**Don't "fix" by:** lowering `jaccardThreshold` instead. The two failure modes overlap: the real Kaiju Girl Caramelise match scores ~0.33 Jaccard, but the SAO-spinoff torrent that must stay rejected scores ~0.5 — no single threshold accepts one without accepting the other. The marker-truncation approach fixes the union inflation at its source instead.

**Amendment (see #75):** truncation is still the FIRST attempt and holds exactly as described above, but it is no longer the only one: when it fails, `titleMatchesQuery` tries the alternative titles that appear after the marker. The sentence "none of that belongs to the anime's core title" was false for groups that repeat the romaji in the footer.

---

### 19. Disk space is read via OS stat on `CompletedAnimePath`

**Location:** `internal/files/diskspace_unix.go`, `internal/files/diskspace_windows.go`; `internal/api/endpoint_status.go` (`handleStatus`).

**What it looks like:** Reading disk space with a raw platform-specific filesystem syscall on a local path, when a portable Go library might seem cleaner.

**Why it's right:** The dashboard needs both total capacity **and** free space — the disk card shows both; the syscall pair (`Statfs` / `GetDiskFreeSpaceEx`) is the direct way to get both. `handleStatus` swallows stat errors (empty/unreadable `CompletedAnimePath`) rather than surfacing them, so a bad path just hides the disk card instead of breaking `/api/v1/status`.

> **Revision (embedded-client refactor):** The original rationale compared this against qBittorrent's `free_space_on_disk` API (which only reported free space, not total). That comparison is now moot — the torrent client is embedded and there is no qBittorrent API to read from. The old cross-host mount-mismatch caveat no longer applies either. The OS-stat approach stands as the only and correct source.

> **Amendment (see #31):** disk space is now measured on `CompletedAnimePath`, not the legacy `save_path` field. Since the download directory is derived from `CompletedAnimePath` (`Config.DownloadPath`), the two are always on the same filesystem by construction, so measuring the library path also correctly reports free space for downloads-in-progress.

**Don't "fix" by:** trying to route disk stats through the torrent library — it has no such API, and `CompletedAnimePath` is on the same filesystem as the download directory the daemon writes to.

---

### 20. `ExtractSeason` has a roman-numeral fallback kept out of `reSeasonPatterns`

**Location:** `internal/nyaa/nyaa_regex.go` — `reRomanSeason`, `romanSeasonValues`; `internal/nyaa/nyaa.go` — `ExtractSeason`.

**What it looks like:** A second, separate regex (`\b(II|III|IV|V|VI|VII|VIII|IX|X)\b`) tried only after the whole `reSeasonPatterns` loop has failed, instead of just adding it as one more entry in that slice.

**Why it's right:** Some sequels are titled with only a roman numeral (e.g. Anilist id 194829, "Katainaka no Ossan, Kensei ni Naru II") — no "Season 2"/"S2" appears anywhere in the AniList title, and fansub groups (Erai-raws, Ironclad) release episodes using that exact title verbatim, with no separate season marker either. Before this fix, `ExtractSeason("...Naru II - 01 [1080p]...")` returned `nil` while `ExtractAnimeSeasonPart` (via the `"...2nd Season"` synonym) correctly resolved `requestedSeason=2`, so every real torrent got rejected by the hard season filter in `ScrapNyaaForMultipleEpisodes` (`season == nil` vs `requestedSeason=2`) even though `titleMatchesQuery` already matched the full title including "II" — the anime failed to download every cycle. `reSeasonPatterns` is also used by `truncateAtFirstMarker` (decision 18) to decide where to cut a torrent name before tokenizing; if the roman-numeral pattern were merged into that slice, it would truncate `"...Naru II"` right before "II", dropping it from the Jaccard title tokens and silently changing match behavior for every title ending in a numeral. Keeping it as a separate, lower-priority fallback used only by `ExtractSeason` avoids that cross-effect.

**Don't "fix" by:** merging `reRomanSeason` into `reSeasonPatterns`, or matching lowercase roman numerals — lowercase risks false positives from unrelated fansub/codec tokens, and uppercase-only matches how anime titles actually format them.

---

### 21. Embedded torrent client + hardlink-into-library model (replaces qBittorrent)

**Location:** `internal/torrents/` (`TorrentBackend`, `Session`/`SessionManager`, `FakeBackend`); `internal/files/librarian.go`; `internal/daemon/jobs.go` (`JobOrganize`), `episodes.go`, `verification.go`.

**What it looks like:** The app embeds a full BitTorrent client (`github.com/cenkalti/rain/v2`) and hardlinks completed files into a second directory, instead of the more familiar "talk to a running qBittorrent over its WebUI and let it manage files."

**Why it's right:** Embedding makes the daemon a single self-contained binary — no external qBittorrent to install, run, secure, or keep reachable, and no `qbittorrent_url`/`QBITTORRENT_URL` to configure. Torrents download to `save_path` and keep **seeding** there; on completion the video files are **hardlinked** into `completed_anime_path` (the Jellyfin library). Hardlinking (not copy/move) means:
- No wasted space — the library name and the seeded file share the same bytes.
- Seeding is never interrupted — the seeded file is never moved or renamed (renaming would break the torrent). The Jellyfin name (`"Anime Name - E05.mkv"`, for single episodes and for batch files whose number is readable) lives only on the library hardlink.
- The hard constraint: `save_path` and `completed_anime_path` **must be on the same filesystem/volume** (hardlinks can't cross devices). `completed_anime_path` is therefore now **required**, and the config-save endpoint validated the pair with a real hardlink probe (`Librarian.ProbePaths`), rejecting cross-device paths with HTTP 400. A two-way probe **no longer exists** — today it is `Librarian.ProbePath`, one argument; see the amendment at the end of this entry.

Torrent logic sits behind the `TorrentBackend` interface (`Add`/`List`/`Get`/`Remove`/`Ensure`/`SetCallbacks`/`Close`) so the daemon injects one uniform backend — rain-backed `SessionManager` in production, in-memory `FakeBackend` in tests (the qBittorrent mock server is gone). Resume data lives in a bbolt DB at `~/.autoAnimeDownloader/session.db`, deliberately **outside** `save_path` so it survives a `save_path` change. rain listens on a default port range (20000–30000) with no UPnP/NAT-PMP; inbound peers may need manual forwarding, but DHT+PEX work without it.

**Don't "fix" by:** re-introducing an external torrent client or `qbittorrent_url`; renaming the seeded file to the Jellyfin name (breaks seeding); copying/moving into the library instead of hardlinking (wastes space, stops seeding); or moving `session.db` into `save_path` (loses resume data on a path change). Note the webhook event key string is still `download_completed` — the Go constant was renamed `QBittorrentDownloadCompleted → DownloadCompleted`, but that is not user-visible; don't "fix" a webhook that isn't broken.

**Amendment (see #31):** `save_path` is no longer a user-configured field — the download directory is now derived from `completed_anime_path` (`Config.DownloadPath`). What was previously a constraint on the user's configuration ("these two fields must be on the same filesystem, and the endpoint validates it") is now an architectural invariant that holds by construction: there is only one path, so there is nothing to be on a different filesystem from. `Librarian.ProbePaths` (two arguments) is gone; the surviving probe is `Librarian.ProbePath` (one argument, #26).

---

### 22. Organize everything to `completed_anime_path`, and the batch-hygiene deletion limitation

**Location:** `internal/daemon/jobs.go` (`organizeTorrent`), `internal/daemon/episodes.go` (`removeEpisodesAndLinks`, `canRemoveTorrent`). Pinned with real files on disk by `TestRemoveEpisodesAndLinks_RealHardlinks` (`internal/daemon/orchestration_test.go`), whose subtests separate the batch asymmetry (torrent *and* library files kept while a sibling survives) from the non-batch shared-hash case (library link removed, torrent kept).

**What it looks like:** (a) *Every* completed torrent — ongoing or finished — is hardlinked into `completed_anime_path`, not just finished animes. (b) When deleting a watched episode that belongs to a **batch** torrent, its library hardlink is **kept** until the entire batch is deleted, which looks like a leak.

**Why it's right (behavior change):** Previously only FINISHED animes were moved to `completed_anime_path`, while ongoing episodes were renamed in place inside `save_path`. Now `save_path` is purely the download/seeding working directory and `completed_anime_path` is the Jellyfin library — so *every* completed torrent gets organized into the library, uniformly, via the single idempotent `organize` job. This removes the ongoing-vs-finished special case and gives Jellyfin one consistent library path to watch.

**Why it's right (batch guard):** Deletion frees space by removing both the library hardlink and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). For a **single-episode** torrent that maps cleanly to one library file, both links are removed. For a **batch** torrent shared by many episodes, the raw batch filenames can't be safely mapped back to one specific episode — removing "the file for episode 5" risks deleting the wrong file. So per-episode library removal is **skipped for batches**; the batch's library files (and the seeding torrent) are only removed once **all** of that torrent's episodes are in the delete set (`canRemoveTorrent` — which since #74 also requires that the pack does not cover episodes with no record). While any sibling episode survives, the batch torrent and its library links stay. The small space cost of keeping a shared batch around a bit longer is preferred over the correctness risk of deleting the wrong episode's file.

**Don't "fix" by:** trying to delete individual episode files out of a batch torrent's library folder (raw filenames aren't reliably episode-addressable), or removing a batch torrent while siblings still reference it (breaks the survivors' library links and stops seeding for episodes still wanted).

**Amendment (see #31):** `save_path` no longer exists as a config field; "everything organizes to `completed_anime_path`" now also covers the download/seeding directory itself, which lives at `<completed_anime_path>/.torrents` (`Config.DownloadPath`).

---

### 23. Integration tests skip unless `DAEMON_URL` is set explicitly

**Location:** `src/tests/integration/integration_test.go` (`requireDaemon`, `defaultTestPath`, `testSavePath`/`testCompletedPath`); `docker/docker-compose.test.yml` (`test` service env).

**What it looks like:** `go test ./...` reports the entire integration suite as SKIP on a developer machine, even when a daemon *is* running on `localhost:8091`. The obvious-looking "fix" is to drop the `DAEMON_URL` check and let `probeDaemon()` decide, since a reachable daemon seems like exactly the condition the tests want.

**Why it's right:** These tests are not read-only. `TestAPIEndpoints` and `TestFullDownloadFlow` both `PUT /api/v1/config`, overwriting `save_path`, `completed_anime_path`, `anilist_username`, `check_interval` and `excluded_lists` on whatever daemon answers. Gating on reachability alone meant that following `CLAUDE.md`'s "run `go test ./...` after any change" silently reconfigured the developer's own daemon: the save path became `/tmp/test`, and on distros where `/tmp` is a tmpfs the daemon then downloaded real torrents into RAM until the filesystem filled. A live daemon is not consent to reconfigure it, so the opt-in is an explicit env var. `docker-compose.test.yml` sets `DAEMON_URL=http://daemon:8091`, so Docker and CI runs are unaffected.

The paths written into the config are likewise no longer hardcoded: they come from `TEST_SAVE_PATH`/`TEST_COMPLETED_PATH`, defaulting to `~/aad-test/downloads` and `~/aad-test/library`. Two constraints shaped that default — it must not be under `/tmp` (tmpfs), and both paths had to share one filesystem, because the config endpoint runs a real hardlink probe and rejected cross-volume pairs with HTTP 400. The second constraint died with `save_path` (see the amendment at the end of this entry): the probe is `Librarian.ProbePath`, one path, and what it refuses today is a filesystem without hardlinks. Docker overrides both to `/app/data/aad-test/*`, inside the daemon container's mounted volume; note the strings are interpreted by the **daemon** container, not the test container.

**Don't "fix" by:** removing the `DAEMON_URL` gate so the tests "work" during `go test ./...`; hardcoding the config paths again; putting the default paths back under `/tmp`; or pointing `TEST_COMPLETED_PATH` at a filesystem without hardlinks (the probe rejects it).

**Amendment (see #31):** `testSavePath`/`TEST_SAVE_PATH` were removed — the integration test no longer sends `save_path` at all, since the field doesn't exist anymore. `Librarian.ProbePaths` (the two-argument save/completed variant) was replaced by `Librarian.ProbePath(completedPath string)`.

---

### 24. A failed torrent is dropped from the session and re-added by the next pass — no blacklist

**Location:** `internal/daemon/helpers.go` — `HandleTorrentFailure`; wired as the `onFailed` callback in `cmd/daemon/main.go`.

**What it looks like:** When the embedded client stops a torrent with an error, we fire the `download_failed` webhook and then **remove the torrent from the session** (`Remove`, with the bytes kept when there is progress — see the amendment at the end). Since nothing records that the hash failed, the very next verification pass finds the episode missing from the session and re-adds the same magnet. For a torrent that is genuinely dead (no seeds at all) this is an unbounded re-add loop, one attempt per `check_interval`. That looks like a bug we forgot to guard.

**Why it's right:** The alternative is worse. rain leaves a failed torrent in state Stopped **inside** the session and never restarts it, and the per-torrent listener goroutine exits after `NotifyStop` and is not re-armed (`torrents/session.go`). If we only logged, `episodeInTorrents` (`daemon/helpers.go`) would keep seeing the hash and the daemon would believe the episode was downloaded — forever. No retry, no organize, no notification, and the episode silently never reaches the library. Dropping the torrent converts a permanent silent failure into an automatic retry that reuses the machinery already in place (the loop re-searches Nyaa and may pick a *different*, healthier release the second time). `EpisodeRetryLimit` does not bound this, because it resets each pass.

**Cost of the accepted risk:** a dead torrent costs one Nyaa search plus one magnet add per `check_interval` — cheap, visible in the logs, and each retry fires a `download_failed` webhook so the user is not left guessing.

**Don't "fix" by:** adding a speculative per-hash blacklist with a TTL. That is the correct fix *if* the re-add churn is ever observed to be a real problem, but it introduces new state to persist, expire, and expose in the UI (otherwise a permanently blacklisted episode becomes the new silent failure). Build it against a real report, not preemptively. Also don't go back to log-only in `onFailed`.

**Amendment (2026-09-07): the re-add RESUMES, it does not start over.** The original version always
passed `keepData=false`, on the grounds that partial data from a torrent that errored was not
reusable. That is false, and on a 90 GiB pack at 90% it cost the entire download for a transient
error. `Session.Add` adds with `AddTorrentOptions{ID: hash}` and the session runs with
`DataDirIncludesTorrentID`, so the re-add lands in the **same** `<DataDir>/<id>`; rain sees the
existing files (`allocator.HasExisting`) and, with no resume-db entry — `RemoveTorrent` deleted it
— runs the verifier and rebuilds the bitfield on its own. Nothing in the app calls `Verify()`: that
method does not exist on rain's public API, and it does not need to.

`keepData` is **conditional**, not a fixed `true`: only with `PiecesHave > 0`. Zero verified pieces
have nothing to recover, and the kept folder would become permanent junk when the retry lands on
another infohash — the directory is per torrent id and nothing sweeps the download path.
`PiecesHave` is the progress source that survives `Stop` (`BytesCompleted` zeroes when rain frees
the pieces), and the failure arrives here already stopped.

---

### 25. Seeding is created at startup and is independent of the daemon loop

**Location:** `cmd/daemon/main.go` — `ensureStartupSession`, called right after `jobQueue.Start()`; `internal/daemon/verification.go` — the `backend.Ensure(configs.DownloadPath())` call inside the verification pass.

**What it looks like:** `Ensure` is called from **two** places — once at startup and once on every verification pass — which reads like a redundant call that could be dropped from one side.

**Why it's right:** The two calls serve different purposes and both are needed.

- **Startup call:** torrents keep **seeding** from the derived download directory (`Config.DownloadPath()`, nested under `completed_anime_path`) after they complete. If the session were created only by the verification pass, stopping the daemon loop from the WebUI (or booting with the loop stopped) would mean no session at all, i.e. **seeding stops** — a behavior regression against the external-qBittorrent setup, which kept seeding regardless of what the daemon was doing. That matters for private trackers with ratio requirements. Creating the session at startup makes seeding a property of the *process*, not of the loop.
- **Verification-pass call:** the daemon must be able to boot with an incomplete config (no `completed_anime_path`, and therefore no derivable download path). In that case the startup call deliberately does nothing and the session stays lazy; the pass's `Ensure` is what creates it once the user saves a config. It also handles `completed_anime_path` changing at runtime (recreate, since the derived download path moves with it) and is where the `created == true` return feeds startup reconciliation.

Ordering detail: `ensureStartupSession` runs **after** `jobQueue.Start()`. Creating the session arms the resume listeners, which can fire a completion immediately, and `Start()` loads the persisted job list *over* whatever is in memory — enqueueing before it would silently discard the job. The shutdown defers keep their LIFO order (`jobQueue.Stop()` drains organize jobs that still need the session, then `torrentManager.Close()` flushes bbolt); do not reorder them.

**Don't "fix" by:** removing the `Ensure` from the verification pass (breaks the incomplete-config boot and reacting to `completed_anime_path` changes), removing the startup call (stops seeding whenever the loop is stopped), moving startup reconciliation out of the verification pass, or calling `ensureStartupSession` before `jobQueue.Start()`.

**Amendment (see #31):** `daemon.MigrateSavePath` used to run in `main.go` between `jobQueue.Start()` and `ensureStartupSession`, and `ensureStartupSession` was skipped for the whole boot when it failed. Both are **gone** (2026-08-24, see the amendment on #31): `ensureStartupSession` now runs unconditionally right after `jobQueue.Start()`.

---

### 26. The hardlink probe runs on every verification pass, not just on config save

**Location:** `internal/daemon/verification.go` — the `librarian.ProbePath` gate right after `isConfigComplete`; also `internal/api/endpoint_config.go` (`PUT /config`).

**What it looks like:** Every pass writes a probe file to `save_path`, hardlinks it into `completed_anime_path` and deletes both — real disk I/O on a hot loop, duplicating a validation the config endpoint already performs.

**Why it's right:** `isConfigComplete` only checks that the fields are non-empty, and the endpoint probe only covers configs saved **through the API after** the embedded-client upgrade. Two populations bypass it entirely: users who configured a `completed_anime_path` on a different volume back when the app *moved* files (perfectly legal then — `rename` crosses devices, `link` does not), and any deployment where `docker/entrypoint.sh` writes `config.json` straight from env vars. For them the daemon would download happily while every `JobOrganize` failed with `EXDEV`, retried 20 times over ~2.5h, and was dropped — with `LastCheckError` never set, so the WebUI showed a healthy daemon and an empty library.

The probe aborts the pass instead of merely warning: downloading episodes that provably cannot be organized only fills the disk. It reuses the same probe the config endpoint calls (`Librarian.ProbePath`, see the amendment at the end of this entry), so the message the user sees in the UI is identical to the one `PUT /config` returns. Cost is one small file write, one link and two unlinks per `check_interval` (default 10 min) — negligible next to the Anilist and Nyaa requests in the same pass.

**Don't "fix" by:** removing the gate because "the endpoint already validates it" (it does not, for pre-upgrade and entrypoint-written configs), or downgrading it to a warning that lets the pass continue. Caching the result per path pair is a legitimate optimization if the I/O ever shows up in a profile — but it must be invalidated on config change, and the current cost does not justify the extra state.

**Amendment (see #31):** the probe is now single-path. `Librarian.ProbePaths(save, completed)` was replaced by `Librarian.ProbePath(completedPath)`, which writes a probe file directly under `<completedPath>/.torrents` and hardlinks it in place — there is no second user-supplied path to compare against, since the download directory is derived, not configured. The check still exists for the same reason: exFAT/FAT32 and some SMB shares don't support hardlinks at all, so the invariant "the download dir and the library share a filesystem" is guaranteed by construction, but "the filesystem supports hardlinks" is not.

---

### 27. `saveEpisodesToFile` merges by hand — it uses neither `SaveEpisodesToFile` nor a bare `UpsertEpisodes`

**Location:** `internal/daemon/episodes.go` — `saveEpisodesToFile` and `mergeSavedEpisode`; the two `FileManager` primitives in `internal/files/filemanager.go`.

**What it looks like:** The daemon loads every saved episode, merges field by field, and only then calls `UpsertEpisodes` — when the `FileManager` already exposes two perfectly good one-liners. Either `SaveEpisodesToFile(eps)` or `UpsertEpisodes(eps)` would compile and look cleaner.

**Why it's right:** The three functions have genuinely different semantics and each is wrong on its own here.

- (History: the key used to be `EpisodeID`, the AniList schedule-node id; today it is `EpisodeKey{AnimeID, Episode}` — see #52.) `SaveEpisodesToFile` is **append-only with dedupe by key**: for an ID that already exists it drops the incoming record *entirely*. That was the original bug. On the upgrade path to the embedded client, the rain session boots empty, so `episodeInTorrents` is false for every saved episode and the loop re-downloads all of them — and every one of those updates was silently discarded. The stale record kept `EpisodeNumber: 0` (a field that did not exist before the upgrade, so it deserialises as zero for all pre-existing records) and the stale `EpisodeHash`. The first caused every episode of an anime to be organized as `Anime - E00.mkv`; the second broke `JobOrganize`'s saved-episode ↔ torrent join by hash, so the job retried 20 times over ~2.5h and gave up, forever.
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

**Location:** `internal/torrents/session.go` (`toInfo`); `internal/api/endpoint_torrents.go`; `frontend/src/routes/Downloads.svelte`; `frontend/src/routes/Status.svelte`; `frontend/src/routes/AnimeDetail.svelte`.

**What it looks like:** `TorrentInfo` carries a dozen progress fields filled from a single
`t.Stats()` call, and the WebUI polls `GET /api/v1/torrents` on a plain client-side timer
while a screen that needs it is mounted — instead of the more familiar "push updates over
the existing WebSocket". Three screens poll it independently, at different rates:
`Downloads.svelte` every 2s (`POLL_MS`) while the downloads screen is open, `Status.svelte`
every 5s (`TORRENTS_POLL_MS`) for the global speed card, since Status is the default `#/`
landing route, and `AnimeDetail.svelte` adaptively — 2s while that anime has an active torrent,
15s otherwise, re-scheduled with `setTimeout` instead of `setInterval` so the delay can change
between ticks without ever running two timers at once. All of them are cleared on unmount, so
the endpoint is only hit while at least one of those three screens is on screen.

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

### 31. Download directory is derived from the library

**Location:** `internal/files/filemanager.go` (`Config.DownloadPath`), `internal/files/librarian.go` (`ProbePath`), `internal/daemon/migration.go`.

**What it looks like:** `save_path` disappeared from the config; the download directory is `<completed_anime_path>/.torrents`, computed on every use.

**Why it's right:** the same-filesystem constraint from `#21` was a trap that only showed up as an error on save; deriving the path turns it into an invariant. Two required fields with no clear difference confused the user, for whom only the library matters. `ProbePath` still exists because exFAT/FAT32/some SMB have no hardlinks at all. The `.ignore` + the leading-dot prefix keep Jellyfin out of the download folder.

**Don't "fix" by:** reintroducing `save_path` as a config field or environment variable; removing the guard that makes `DeleteEmptyFolders` skip `.torrents`.

**Amendment (2026-08-24): the migration was REMOVED.** `daemon.MigrateSavePath`, the
`Config.SavePath` field, zeroing it on `PUT /config`, and the six migration tests no longer exist.
What remains of the decision is only the derived path (`Config.DownloadPath`).

A consequence that needs to stay explicit: an installation that **has not migrated yet** (a
`config.json` with `save_path` filled in, from an earlier version) loses the link to the data.
The field is now unknown, JSON ignores it, rain opens the derived path, finds it empty and
**re-downloads the entire library**. Moving `<save_path>/*` to `<completed_anime_path>/.torrents`
before upgrading became a manual step, documented in the README.

The three things the migration guaranteed and now nobody does: the rename preserves the inode (seed
and hardlinks survive), the root marker (`torrents.RootMarkerName`) travels with the data —
without that the next `Ensure` sees a folder swap and zeros `LibraryPaths` (see #34) — and the
operation was idempotent with a retry every pass. Anyone reintroducing the migration needs all
three, not just the rename.

---

### 32. `DELETE /torrents/{hash}`: default is delete + block, `keep_data` is binary, there is no batch endpoint

**Location:** `internal/api/endpoint_torrents.go` (`handleTorrentDelete`, `parseBoolQueryParam`), `internal/daemon/episodes.go` (`RemoveTorrentWithEpisodes`, `RemoveTorrentOptions`, `removeEpisodesAndLinks`), `frontend/src/components/TorrentDeleteDialog.svelte`.

**What it looks like:** the delete UI on the Downloads screen (single row or mass selection) opens a dialog with two checkboxes — "delete files" and "don't download again" — **both checked by default**. Unchecking the first becomes `keep_data=true` (keeps the seeded copy **and** the library hardlink — never only one of the two). Unchecking the second becomes `block=false` and the UI shows a yellow text warning instead of blocking the combination. There is no `DELETE /torrents/bulk`; the client fires N requests at the existing per-hash endpoint.

**Why it's right:**

- **Delete + block by default:** that is the action that matches the most common intent of someone clicking "delete" — get rid of the torrent for good. Leaving both unchecked by default would make the common case cost two extra clicks; leaving only "delete" checked by default would silently recreate the torrent on the next loop step (see the next point).
- **"Delete without blocking" is a re-download from scratch, not a cancel:** `handleAlreadySavedEpisode` (`daemon/daemon.go`) re-downloads any episode whose record in `episodes.json` exists but whose hash matches no live torrent in the session — that is how the daemon recovers from a torrent removed from the outside (crash, manual rain cleanup). `DELETE /torrents/{hash}` with `block=false` produces exactly that state: the record survives (unless `keep_data` also takes the hardlinks), the torrent does not. There is no way for the backend to tell "the user wants to stop downloading this" from "the torrent died and needs to start over" by looking at the record alone — that is why the UI warns in text ("without this, the daemon may download it again") instead of the backend forbidding or forcing the combination. Blocking is the only way to say "no, really" to that flow.
- **`keep_data` does not split library from seeded copy:** the library hardlink and the file seeded by the torrent are the same inode (decision 21). Deleting one and keeping the other frees no space — it only breaks the Jellyfin display or stops seeding without gaining disk. `removeEpisodesAndLinks` treats this as a single binary parameter (`keepData`): both stay or both go.
- **No batch endpoint:** the realistic worst case is tens of selected torrents, not thousands — N HTTP requests in parallel (`Promise.all` in the frontend) handles that without requiring partial-error semantics on the backend (what failed? what to retry?) or a new payload format. The cost of implementing and maintaining a `{hashes: []string}` body with a per-item response does not pay off at that scale.
- **DELETE's 404 is decided by the session alone:** same as `pause`/`resume`/`announce` (`torrentAction`), torrent existence is checked only via `server.Torrents.Get(hash)`. If the hash is not in the session but episode records with that hash still exist (orphans — torrent already removed from the outside), the route answers 404 and **does not touch** those records; cleaning an orphan record is `DELETE /animes/{id}/episodes/{episodeId}`'s job, not this route's. Mixing the two responsibilities would make the error contract ambiguous (does 404 mean "torrent does not exist" or "nothing to do"?).

**Out of scope (investigated, not forgotten):**

- **Concurrent-download limit:** would require a new config field, a real queue in the daemon, and persisting the *reason* a torrent is paused — rain persists the paused state (decision 25), but not the reason, so without that extra field the scheduler would resume by mistake a torrent the user paused on purpose. That becomes its own spec.
- **Per-torrent priority:** `github.com/cenkalti/rain/v2` has no concept of priority, neither per torrent nor per file — only global `SpeedLimitDownload`/`SpeedLimitUpload`. Implementing our own throttling on top of that would touch the lib's hot path for a gain that does not pay off. If the concurrent queue above ever exists, the queue order is already the priority, and a "download first" button that moves the item to the front of the queue delivers the value without requiring a scheduler.

**Don't "fix" by:** making the backend refuse `block=false` on its own (the combination is valid and intentional — it just is not the default); trying to split library from seeded copy by adding a third parameter (same inode, see decision 21); adding a batch endpoint "because it's obvious" without first confirming that the realistic scale (tens, not thousands) justifies the extra partial-error complexity.

---

### 33. The torrent progress inline must not look at `is_downloaded`

**Location:** `src/internal/frontend/src/routes/AnimeDetail.svelte` (torrent progress inline), `src/internal/api/endpoint_anime_episodes.go` (`handleAnimeEpisodes`).

**What it looks like:** an inline progress bar on the anime screen appears based only on `!torrent.completed` — never on `!ep.is_downloaded`.

**Why it's right:** **`episode_hash != "" ⟺ is_downloaded`.** `handleAnimeEpisodes` fills `IsDownloaded` and `EpisodeHash` in the same `if` (a saved record exists for that episode), and the daemon writes the saved record at the moment the torrent is **added**, not when it finishes (`daemon/episodes.go`, `daemon/manual_download.go`). So an episode that is still downloading **already** arrives with `is_downloaded: true`. The old inline condition (`torrent && !torrent.completed && !ep.is_downloaded`) required a combination the API never emits, so the bar never appeared in production — and the smoke test missed it because the fixture assembled exactly that impossible state.

**Don't "fix" by:** going back to filtering the progress inline on `!ep.is_downloaded`; writing a test fixture with `is_downloaded: false` + a filled `episode_hash`.

**History:** this entry was born with a second half — daisyUI pinned on v4 while Tailwind was v3,
the cascade-layer conflict with preflight, and `data-tip` vs `.tooltip-content`. That half is all
DEAD: daisyUI was removed on 2026-08-31 (see #91), the `Layout.svelte` that hosted that tooltip no
longer exists, and the cleanup the old amendment left pending (the raw `bg-blue-600`/`bg-red-600` in
`Status.svelte`/`AnimeDetail.svelte` and `StatusBadge.svelte`) is already done — none of those
classes nor that component exist in the frontend today.

---

### 34. A download-folder swap is detected by a dual marker, and it tears down the rain session

**Location:** `internal/torrents/rootmarker.go`, `internal/torrents/sessionmanager.go` (`Ensure`, `checkRoot`, `writeRoot`), `internal/daemon/verification.go` (`clearLibraryPathsAfterRootSwap`), `internal/daemon/migration.go` (the marker travels with the rename). Pinned by `TestSessionManagerDetectsRootSwap`, `TestSessionManagerRootSwapReportedOnce`, `TestSessionManagerMovedAndRepointedIsNotASwap`, `TestSessionManagerFirstRunAdoptsExistingFolder` and `TestClearLibraryPathsAfterRootSwap`.

**What it looks like:** there are two files with the same random id — `.aad_root` **inside** the download folder and `download_root.id` in the config folder. `Ensure` compares the two on every pass and, when they diverge, closes and recreates the rain session.

**Why it's right:** rain holds open *file descriptors*. When the user moves the download folder (or sends it to the trash) with the daemon running, bytes keep landing on the inode that left while the configured path sits empty — the UI shows everything seeding and **every** `JobOrganize` fails with `no such file or directory`, burning through the 20 retries in silence. Nothing in the process notices, because the path string did not change: only the inode behind it did.

The pair of markers is what makes this detectable without depending on inode (which does not exist on Windows):

- the **inside** marker travels with the folder, so it disappears from the configured path exactly when the folder is moved;
- the **outside** id stays where the user does not touch it, so it survives as a reference — including when the folder is moved with the daemon **stopped**.

Recreating the session is the complete fix because rain already does the hard part: in `torrent_allocation.go`, if the files do not exist (`al.HasMissing`), it **discards the resume bitfield** and re-downloads. There is no need to touch the resume db or remove any torrent.

**Why the signal is a latch and not `Ensure`'s return:** the manual-download endpoints also call `Ensure`. If the flag came back in the return, one of those calls would swallow the swap — the session would be recreated, but the orphan library records would never be cleared, and recovery would be left half-done with no visible error. `Ensure` sets `pendingSwap`; only `ConsumeRootSwap`, called on the verification pass, clears it. Pinned by `TestSessionManagerRootSwapSurvivesAnotherEnsure`.

Three properties that fell out for free and are intentional:

- **Moving the folder AND re-pointing the config** (`completed_anime_path` to the new place) is **not** a swap: the marker travelled with it, the ids match, nothing is re-downloaded. That is the "right" way to move the library.
- **First boot / upgrade from a version without a marker** is not a swap: with no reference id there is nothing to compare, and adopting the folder as-is is the only safe reading — the alternative would wipe the records of a healthy library.
- **It is edge-triggered:** `Ensure` reports the swap once and already rewrites the marker, so the next pass is back to normal.

**Amendment to decision 29:** `clearLibraryPathsAfterRootSwap` zeros `LibraryPaths` — exactly what #29 forbids. The prohibition still holds for a missing file: what it protects is the deliberate deletion of **one** episode from the library, which a `Stat` check every pass would resurrect forever. A root swap is a different event: the entire folder the records point at is gone, the daemon is already re-downloading its contents, and detection fires **once per swap** — it never becomes #29's resurrection loop. Without this cleanup recovery is half-done: the torrents re-download, but episodes organized before the swap keep an orphan `LibraryPaths` and never return to the library.

**Don't "fix" by:** returning the flag from `Ensure` instead of the latch (see above); replacing the markers with inode comparison (`syscall.Stat_t.Ino` does not exist on Windows); treating a marker-read error as "it vanished" (a permission failure would start wiping library records — that is why `readRootID` only swallows `IsNotExist`); cleaning `LibraryPaths` on every pass instead of only on a swap (that really would be the #29 violation); ~~dropping the marker rename in `MigrateSavePath`~~ (the migration was removed on 2026-08-24 — see #31; if it comes back, the marker rename becomes mandatory again for the same reason).

---

### 35. `os.SameFile` on Windows resolves the file late, so FileInfo is not a snapshot in tests

**Location:** `internal/files/librarian_test.go` (`TestOrganizeReplacesDifferentFileAtDestination`, the `staleAlias`). Production use is the `os.SameFile` branch of `Organize` (`internal/files/librarian.go`).

**What it looks like:** the test creates an extra hardlink (`staleAlias`) to the old file **before** calling `Organize`, and then compares `os.Stat(staleAlias)` against the destination — instead of keeping an `os.Stat(dest)` from before the swap, which would be the obvious way.

**Why it's right:** on Linux `os.Stat` records dev+inode immediately, so a `FileInfo` is a snapshot of the file at that instant. On Windows it is not: `saveInfoFromPath` (`os/types_windows.go`) stores **only the path**, and `SameFile` calls `loadFileId`, which opens that path again at comparison time. A `FileInfo` taken from `dest` before the replacement starts pointing at the **new** file after it, and `os.SameFile(staleInfo, destInfo)` is always `true` — the test failed only in the `test-backend-windows` job, with the production code correct. Keeping a second name for the old file pins its identity on every platform, because `Organize` replaces with `Remove(dest)` + `link(src, dest)` and the alternate hardlink survives the `Remove`.

In the `Organize` branch the same `os.SameFile` is correct: both `Stat`s are done at comparison time, with both paths existing.

**Don't "fix" by:** replacing the alias with an `os.Stat(dest)` saved before the swap ("it's the same thing and reads better" — it is not, and it only breaks on Windows); marking the test `t.Skip` on Windows (cross-device/hardlink is exactly what that job exists to cover); deleting the old-file assertion because it looks redundant with the content one (a mutation that skips the replacement fails both, but the identity one is what distinguishes "relinked" from "overwrote in place").

---

### 36. `AppShell` picks rail vs tab bar in JS (`matchMedia`), not with `hidden md:flex`/`md:hidden`

**Location:** `frontend/src/components/shell/AppShell.svelte`, `NavRail.svelte`, `NavTabBar.svelte`, `MoreMenu.svelte` (UI redesign Phase 1, spec §5).

**What it looks like:** `AppShell` keeps a boolean `isDesktop` (initialized synchronously from `window.matchMedia('(min-width: 768px)')`, updated by a `change` listener) and mounts `{#if isDesktop}<NavRail>{:else}<NavTabBar>{/if}` — only one of the two exists in the DOM at any moment. The more common Tailwind pattern for "rail on desktop, tab bar on mobile" would be to mount both always and hide them by class (`hidden md:flex` / `md:hidden`), as the old `Layout.svelte` did for its desktop/mobile block.

**Why it's right:** `NavRail` and `NavTabBar` each mount their own `MoreMenu`, which on mobile hosts a `<select id="theme-select-mobile">` (desktop has its own `<select id="theme-select">`, always visible in the rail footer). With both blocks always in the DOM and only hidden by CSS, both would exist at once — two elements with the same control purpose, potentially two duplicated ids if they ever converged, and two interactive "More" menus at the same time for tools that do not respect `display:none` as mere visibility (e.g. Testing Library's `getByLabelText`/`getByRole` find CSS-hidden elements unless the query filters on `hidden: true`, and can fail with "found multiple elements"). Mounting only one of the two in JS eliminates that entire class of problem, at the cost of a `matchMedia` listener — initialized synchronously in the component's `<script>` (before the first render), so there is no flash of the wrong layout.

**Don't "fix" by:** going back to `hidden md:flex`/`md:hidden` "because it's simpler and it's the Tailwind pattern" — it works visually, but reintroduces the dual-DOM that motivated the decision; initializing `isDesktop` only inside `onMount` (would show the wrong layout for a frame before first paint, since the synchronous `matchMedia` read in `<script>` is what prevents that).

---

### 37. Fixed-width tables only from `lg` up, and `min-w-0` is required on a grid item that contains a scroll strip

**Location:** `frontend/src/routes/Status.svelte` (`LIST_GRID` and the hero card), `Downloads.svelte` (`ROW_GRID`/`ROW_INDENT`), `AnimeDetail.svelte` (`EP_GRID`). Covered by `frontend/tests/smoke/layout.spec.ts`.

**What it looks like:** two things that look redundant and are not.

1. The hero card's "Active downloads" strip has `overflow-x-auto` **and** the wrapping `<section>` has `min-w-0`. Looks like belt and suspenders — `overflow-x-auto` alone should be enough for the strip to scroll.
2. The three tables switch to stacked cards at `lg` (1024px), not at `md` (768px), even though the whole app uses `md` as the desktop/mobile breakpoint (including decision 36's `matchMedia`, which stays at 768px).

**Why it's right:**

1. A grid item has `min-width: auto`, i.e. its floor is the content's intrinsic size, not zero. With many downloads the `<section>` was stretched to max-content and `overflow-x-auto` **never activated**: measured on a 1280px grid, the section went to 2494px, the strip had `clientWidth == scrollWidth` and the right-hand column ran off the screen. With `min-w-0` the same section is back to 677px and the strip actually scrolls (639px visible for 2456px of content). `overflow-x-auto` describes the intent; `min-w-0` is what lets it happen.

2. It is arithmetic, not taste: the fixed tracks add up to ~754px (Status), ~740px (Downloads) and ~710px (AnimeDetail), while the usable width at `md` is `768 − 92 (rail) − 48 (main padding) ≈ 628px`. The cut at `md` was always wrong — the 768–880px band fell into horizontal scroll — and only did not bother anyone because nobody opens the app at that width. `lg` is the first breakpoint where the columns fit. The rail-vs-tab-bar breakpoint stays at 768px on purpose: it is about the **shell**, not about the width a table needs.

**Don't "fix" by:** removing `min-w-0` "because it already has `overflow-x-auto`" (that is the inverted order: without `min-w-0` the overflow is decorative); removing `overflow-x-auto` "because `min-w-0` already solves it" (then the strip clips the content instead of scrolling); dropping the tables back to `md` for consistency with the rest of the app (redo the arithmetic above first — usable width at `md` covers none of the three); adding a fixed-width column without revising the arithmetic comment that lives next to each grid constant.

---

### 38. In a `flex-wrap` header, the `flex-1` item gets `min-w-[240px]` — not `min-w-0`

**Location:** `frontend/src/routes/Downloads.svelte` (title block, at the top of the template). Covered by `frontend/tests/smoke/layout.spec.ts` ("subtitle is not squeezed to one word per line").

**What it looks like:** the title/subtitle block is `min-w-[240px] flex-1`, contradicting the `min-w-0` that decision 37 requires on grid/flex items in the rest of the app.

**Why it's right:** they are opposite problems. `min-w-0` is for an item that **contains a scroll strip** and therefore needs to be able to shrink below its content. Here the item is a neighbour of a wide box (the ↓/↑ bandwidth bar, 433px of max-content) inside a `flex-wrap` container, and a flex container's wrap decides by each item's **hypothetical main size** — which for `flex-1` (flex-basis 0%) is ~0, and `min-width` is exactly what enters that calculation. With `min-w-0` the title contributed ~0, the bar never went to the next line and the title was left with the leftovers: measured, 7 lines of subtitle at 500px and 4 at 768px, one word per line. The 240px floor pushes the bar to the next line at any width where it does not fit.

**Don't "fix" by:** swapping to `min-w-0` "to match the rest of the app" (that is the bug back — decision 37's rule is for an item that hosts scroll, not for an item neighbouring wide content); solving it with a breakpoint (`flex-col sm:flex-row`) — that fixes 375px and leaves the 500–768px band squeezed, which is exactly where the symptom was worst; assuming `flex-wrap` alone already wraps the line (it wraps by hypothetical size, not by the final size after flexing).

---

### 39. A FINITE choice strip wraps (`flex-wrap`); an UNBOUNDED content strip scrolls

**Location:** `frontend/src/components/DownloadsToolbar.svelte` (filter pills), the index `<nav>` in `frontend/src/routes/Config.svelte`, and the daemon-action row in the header of `frontend/src/routes/Status.svelte`. Covered by `frontend/tests/smoke/layout.spec.ts` ("every downloads filter pill…", "every config group and exit link…", "status header daemon actions stay on screen").

**What it looks like:** the three strips are `flex flex-wrap`, with no `overflow-x-auto` and no `min-w-0` — while the hero card's "Active downloads" strip (decision 37) still scrolls.

The Status row shows the variant where the symptom is not scroll but **clipping**: it was `flex shrink-0 gap-2`, with no scroll strip at all. As an item of a `flex-wrap` header, `shrink-0` locked its width at the max-content of a single line (358px in `en`, 438px in pt-BR), so it never received a narrow line it could wrap on and "Stop Daemon" ran off the right at ≤414px in Portuguese. Removing `shrink-0` makes the floor the min-content (the widest button) and wrapping happens. Measured after the fix: the buttons keep 148/150/124px at every width — none shrink, so no label wraps in the middle, and `shrink-0` on the CHILDREN (which the decision asked for on the pills) was not needed here.

**Why it's right:** the criterion is whether the strip offers a CLOSED set of choices or a list of arbitrary size. A choice hidden behind a swipe is a choice that does not exist: the user does not know there is more. The pills exist to answer "is there something there?" by count, and "Seeding"/"Problems" sat off-screen on a narrow display. The Config index has four groups plus two exit links — at 390px, 358px of space for 644px of content, i.e. half hidden, including "Torrent search" and the two links, which are the only path to Priorities and Notifications from that screen. The hero strip is different because the active-downloads list has no ceiling: no layout puts N torrents on screen, and then scrolling is the right degrade.

Neither of the two gets `min-w-0`: the item's automatic minimum is min-content (the widest pill/label), which is exactly the desired floor — with `min-w-0` the item would overflow again instead of wrapping.

In the Config index the dividers are `w-full`, so wrapping happens on them: the three resulting rows (day-to-day groups / advanced group / exit links) are the screen's architecture, which on desktop the column already says implicitly.

**Revision:** until 2026-08-11 this decision listed the Config index on the "scrollable" side, classifying it as a "long homogeneous list". That was factually wrong — there are six items, not a long list, and measurement showed half off-screen. The criterion above ("finite vs. unbounded") is what replaces that.

**Don't "fix" by:** putting `shrink-0` back on the Status row "so the buttons don't shrink" (that is the bug back — `shrink-0` on the CONTAINER locks wrapping; what protects the label is `shrink-0` on the child, and here it is not even needed); putting `overflow-x-auto` back on any of the three "for consistency with the hero strip" (the hero solves a different problem — a list with no ceiling); removing `shrink-0` from the items (they would shrink and the label would wrap in the middle, instead of the whole item dropping to the next line); replacing the Config index with a `<select>` on mobile (`<option>` does not style, so the "needs filling in" cue would become text, and the two exit links would stop being `<a>` — losing open-in-new-tab, which is why they are `<a>`).

---

### 40. NavRail's `z-30` is not redundant with the MoreMenu panel's `z-50`

**Location:** `frontend/src/components/shell/NavRail.svelte` (class on the `<nav>`), `MoreMenu.svelte` (panel `absolute z-50`, backdrop `fixed inset-0 z-40`). Covered by `frontend/tests/smoke/layout.spec.ts` ("the More menu paints above the page content", "a modal still paints above the nav rail").

**What it looks like:** the menu panel is already `z-50`, and yet the wrapping `<nav>` still needs `z-30` — looks like a leftover number, and the temptation is to remove one of the two.

**Why it's right:** `position: sticky` creates a stacking context **always**, regardless of z-index. With no z-index on the `<nav>`, that `z-50` only orders things **inside** the rail, and the whole rail paints on the root's z-auto layer, in tree order — losing to any positioned element that comes later in the DOM. That is what happened on the Priorities screen, whose cards used daisyUI's `.card` class (`position: relative`): the cards appeared in front of the open menu. daisyUI left (#91) and those cards are no longer positioned, so that specific cause is gone — `z-30` stays anyway: it orders the whole rail against ANY positioned element that comes later in the DOM, not only against that one. With `z-30` (the same value as `NavTabBar`, which is `fixed`), the whole rail rises together. The backdrop's `z-40` and the panel's `z-50` are still needed to order the menu **against the rail itself**; Modal and Toasts sit at `z-50` on the root and therefore stay above the rail's 30.

**Don't "fix" by:** raising the rail to `z-50` "to match the panel" (it would tie with Modal/Toasts and the dialog would start depending on tree order); removing `z-30` because "sticky already sits in front" (it sits in front of **unpositioned** content — any later `position: relative` in the DOM beats it); removing the panel's `z-50` thinking the nav's `z-30` is enough (inside the rail the panel needs to beat the `z-40` backdrop).

---

### 41. The download queue holds EVERY incomplete torrent, is persisted, and `queued` is the only slug rain never produces

**Location:** `torrents/queue.go` (`queue`, `enforce`, `markQueued`, `load`/`save`), `torrents/sessionmanager.go`, `torrents/status.go` (`StatusQueued`), `files.Config.MaxConcurrentDownloads`. Frontend: `lib/utils/torrentStatus.ts`, `torrentFilters.ts` (`KNOWN_STATUSES`, `isProblemTorrent`, `prioritizeOrder`), `routes/Downloads.svelte` (`canPause`/`canResume`/`canPrioritize`).

**What it looks like:** a pile of things that look like sloppiness, and all of it is deliberate.

**(a) The queue is a list of EVERYTHING, not only of what it paused.** `queue.order` holds every incomplete torrent the session knows; the first `limit` that are not in `queue.paused` download. The previous version stored only the hashes the queue had paused, and the three defects that produced were structural: prioritizing with the limit full deprived nobody (there was no excess to demote and no slack to promote, so the prioritized one only started when a slot freed on its own); batch prioritize inverted the order (each `pushFront` jumped ahead of the previous one); and there was no stable position to show on screen. With the queue holding everything, nothing is inserted in the middle — position only changes by user action — and the question "where does this hash go" disappears. That deleted `insertByAddedAt`, `leastProgressed` and the progress tie-break. `AddedAt` survives for one reason only: to give initial order to torrents the queue does not know yet (`enforce` step 2).

**(b) `enforce` is reconciliation, not event reaction — and step 0 is what stops the queue from dying on every boot.** `SessionManager.list()` returns `nil` while there is no session and a **non-nil** empty slice for a session with no torrents. `nil` means "I know nothing", not "there is nothing": without the guard, step 1 would prune `order`/`paused` wholesale and step 5 would write the empty file over the top — on **every** boot, because `verification.go` calls `SetMaxActiveDownloads` (which runs `enforce`) **before** `Ensure`. It is also what guarantees a failure fixes itself: every verification pass runs `SetMaxActiveDownloads`, so a `pause` rain refused or a `stopping` left mid-way resolves on the next cycle, with no new ticker.

**(c) Step 4 iterates `order`, NEVER the session.** A complete torrent does not enter `order` (seeding does not occupy a slot), so walking the session would mark every seeder as "should not be active" and **pause the entire library**. `order` is the list of incompletes and is exactly the set the queue has authority over; what is outside it the queue does not touch, neither to pause nor to resume. That is why `Pause`/`Resume` of a complete torrent **bypass the queue in both directions**, delegating straight to rain — without the bypass, and with step 4 restricted to `order` (as it must be), pausing a seeder would stop nothing. Consequence: `paused` holds only incomplete hashes.

**(d) `stopping` is a target of nothing.** rain's pause is asynchronous (`Torrent.Stop()` returns immediately, the status only reaches `stopped` up to ~5s later). Treating it as stopped would make `enforce` send `resume` over a stop in progress; treating it as running would make step 4 pause it again. It stays out of both sides and the next pass decides on a stable state.

**(e) Preemption picks by POSITION, not by progress.** The last of the actives in queue order leaves — a download at 95% can be paused if it sits later in the queue, which the old `leastProgressed` avoided. Accepted: position is the axis the user controls, and the alternative is resurrecting the piece (a) deleted. Nothing is lost either way: rain zeroes `Bytes.Completed` on stop, but the piece bitfield survives.

**(f) `Resume` puts it at the END of the queue, it does not start immediately.** With a concurrent-download limit, a resume that started unconditionally would silently punch through the limit — and a limit the UI shows being violated is a limit nobody trusts. Whoever wants "start now" calls `Prioritize`, which goes to the front and preempts. That is why a queued row has **three** buttons, each in one direction: Prioritize goes up, Play goes down to the end, Pause takes it out of rotation. Manual downloads use `daemon.addAndPrioritize` — they punch through the queue, not the limit.

**(g) `queued` is a status slug, not a boolean field — and the position comes from the same map.** It is the only `TorrentInfo.Status` value that does not come from `statusSlug()`: for rain a queued torrent is just another stopped torrent. The source is `q.queued`, the map step 3 just computed, **not** `order` — marking by belonging to `order` would paint the whole screen `queued`, since it now holds the actives too. Whoever is in `paused` shows `stopped` (what rain reports), and `queued` means only "the queue will get to it". `QueuePosition` 0 = not queued, which includes every paused one: it has a position in `order`, but that position predicts nothing, and publishing a number that will not happen is worse than publishing none. When a `pause` fails, the hash **leaves `q.queued`** keeping its position in `order` — the screen shows what is happening, at the cost of a hole in the numbering that lasts until the next pass.

**(h) The queue is persisted in `queue.json`, next to the resume db.** Same place and same reason as `download_root.id`: it is torrent-client state, not user configuration, and it needs to follow the resume db. That kills the ceiling the previous version marked with `ponytail:` — a manual pause now survives restart. The "boot adoption", which ran on every session created and resumed by mistake what the user had paused, shrank to a **one-shot latch**: when `queue.json` does not exist, the read arms `seedPaused` and the first `enforce` **with a session** marks as `paused` the incompletes rain reports stopped. It cannot seed on the read — in `NewSessionManager` there is still no session to query. The cost, real and unavoidable: on that upgrade boot the torrents the old queue (memory only) paused are indistinguishable from the ones the user paused, so both groups freeze and need a click on Play or Prioritize. That is the right trade — freezing is visible on screen and costs a click; resuming on its own is invisible and undoes a user decision while spending bandwidth. It happens once in the life of the install, and boot logs how many hashes were seeded.

**(i) The batch ignores what the individual rejects.** `POST /torrents/prioritize` applies **in the order received** (the frontend decides, it knows what the user clicked) and ignores an unknown or already-complete hash: a list of 12 episodes cannot fail entirely because one of them finished downloading between the render and the click. `POST /torrents/{hash}/prioritize` keeps the usual errors — validation lives in `SessionManager.Prioritize(hash)`, which checks **before** delegating to `PrioritizeAll`. That is not the same as N calls to the individual endpoint: those would race each other and invert the batch. The frontend result message also leaves the `{success} of {total}` of the other batch actions — it is **one** request, so there is no partial success to count, and the denominator is not even verifiable on the client.

**Don't "fix" by:** iterating the session instead of `order` at step 4 (pauses **every seeder** — see (c)); calling `m.Get`/`m.List` from inside `PrioritizeAll` (they go through `markQueued`, which takes `queue.mu`: **deadlock**, the same the line below describes for `enforce`); making `enforce` read via `SessionManager.List()` instead of `list()` (same — that is how the first version hung `TestSessionManagerDetectsRootSwap`); making the queue call the public `Pause`/`Resume` instead of `pause`/`resume` (that really is infinite recursion); updating `lastSaved` without the `Rename` having succeeded (a transient disk failure would freeze the file on the old version forever, because "only write when it changed" would never fire again); inverting the `paused`-before-`pause` order in `SessionManager.Pause` (an `enforce` fired by a completion would promote exactly the torrent the user is stopping); treating `all == nil` as an empty session at step 0 (wipes the persisted queue on every boot, silently); taking `queued` out of `isProblemTorrent` (0 peers is the definition of being queued, and the problems pill would go red every time the limit worked); letting a complete torrent count against the limit (seeding costs no download bandwidth and stopping seeding is the opposite of what you want); putting the queue position inside `statusLabel()` (the same function builds the toolbar status filter, where "In queue #7" means nothing); reordering or renumbering the rows by group so the numbers become contiguous (the non-contiguity is exactly what communicates that the queue is global).

---

### 42. Every persisted `FileManager` state writes with temp+rename under `m.mu`, and `fetchAniListEntries` returns `nil` on purpose

**Location:** `files/filemanager.go` (`writeAtomic`, `loadSavedEpisodesLocked`, `saveEpisodesLocked`, `loadBlockedEpisodesLocked`, `saveBlockedEpisodesLocked`), `files/parser.go` (`ParseEpisodes`), `api/endpoint_animes.go` (`handleAnimes`, `fetchAniListEntries`, `refreshOrphanAnimes`).

**What it looks like:** two pairs of conventions that look like ceremony and are the scar of a real incident (2026-08-07): releasing 5 episodes at once from the UI killed the API for 3 minutes and then trapped the daemon in a 429 storm against AniList.

**(a) The public episode/blocked methods take `m.mu` and delegate to a `...Locked`.** All of them do read-modify-write of the whole file, and the UI fires several in parallel — one `POST /release` per episode, at the same time the daemon calls `UpsertEpisodes` while organizing torrents. Without the lock the updates are lost (every goroutine reads the same initial state and only the last write survives) and, worse, two `WriteFile`s truncating the same path at once leave the file interleaved. That is how `episodes` corrupted: `LoadSavedEpisodes` started failing, and because `/api/v1/animes` and `/api/v1/animes/{id}/episodes` treat that error as fatal, **both started returning 500 until restart**. `Config` and `AnimeSettings` already did this right; episodes and blocked were the hole. The mutex is not reentrant — hence the `...Locked`.

**(b) `writeAtomic` (temp+rename) for all four files, not only config.** Every state here is rewritten whole on every change, so a direct `WriteFile` leaves a window where the file is half-written — a concurrent reader sees it truncated, and a power drop in the middle (Raspberry Pi, SD card) leaves it truncated forever. `rename` is atomic on the same filesystem, so no reader sees an intermediate state.

**(c) `ParseEpisodes` used to report BOTH errors** (the legacy text format was removed in #52, so today only the JSONL error exists). For an already-migrated file the old-format error is noise (it only complains that the line is JSON), and reporting only that hid the JSONL line that was actually broken. The message it produced — `invalid episode ID '{"episode_id"'` — pointed at line 1 of a file whose line 1 was intact, and that is what delayed the incident diagnosis the most.

**(d) `fetchAniListEntries` returns `nil` on failure and an empty (non-`nil`) slice on success — and `handleAnimes` depends on that.** `refreshOrphanAnimes` fetches from AniList the animes that fell outside the `covered` set (today in a batch, `GetAnimeInfoByIDs`; until 29 Aug 2026, one individual `GetAnimeInfo` per anime). When the list fetch failed, `covered` sat empty and **every anime with a downloaded episode became an orphan**: 16 requests per `/api/v1/animes` poll, ~10 polls per minute, with the frontend open in two tabs. That multiplied a passing AniList 500 into ~200 requests/min against a limit of 90 (today degraded to 30) — and the resulting 429 made the list fetch fail again, closing the cycle: the daemon did not get out on its own. So, with any account failing, the orphan refresh is **skipped whole**: without the list there is no way to know what is covered, and treating "not covered" as "needs refresh" is exactly the amplifier. **The batch does not revoke this rule**: it drops the burst cost, but "unknown coverage" is still the state in which the refresh has nothing to decide — and a whole library's `mediaId_in` is still `ceil(N/50)` requests per account per poll. The animes stay visible with local data — degrading a stale field is cheap, knocking AniList over is not.

**Don't "fix" by:** calling the public method from inside a `...Locked` (deadlock — `sync.Mutex` is not reentrant); "simplifying" `writeAtomic` back to `WriteFile` because "the lock already solves it" (the lock only protects this process, it does not protect a power drop mid-write); making `fetchAniListEntries` return an empty slice on failure "because nil is an empty slice in Go" (erases the distinction between "none covered" and "coverage unknown" and resurrects the storm); treating the orphan-refresh skip as a loss of functionality and "just limiting concurrency" (that is what the extinct `maxConcurrentOrphanRefresh` did, and it did not help — limiting parallelism does not reduce the total requests per poll; what reduced it was the batch).

---

### 43. An anime's identity is `Media.Id`; status is a PER-ACCOUNT question (download = OR, deletion = AND)

**Location:** `anilist/anilist.go` (`DedupeByMedia`, `GetAnimeInfo`, `GetMediaListStatus`, `GetMediaIDForEntry`), `daemon/verification.go` (`searchAnilist`, `deletableMediaIDs`, `allAccountsAgreeOnDelete`), `daemon/migration.go` (`MigrateAnimeIDsToMedia`), `api/endpoint_animes.go`.

**What it looks like:** three things that look overdone — `GetAnimeInfo` takes the username list and makes one request per account to fetch ONE anime; `DedupeByMedia` warns in the comment that nobody may read the result's `Status`; and `allAccountsAgreeOnDelete` fires an extra query per account that did not report the anime.

**Why it's right:** `MediaList.Id` is the **entry** id, which is per account — the same anime on two accounts arrives as two entries with different ids and the same `Media.Id`. While the stored `AnimeID` was the entry id, it pointed at **one** of the accounts, and `GET /animes` listed the anime twice: once under the `id:<AnimeID>` key from disk and once by name, because the other account's entry matched nothing known (real bug: "From Old Country Bumpkin to Master Swordsman II", entries 488911345 and 583631757, media 194829). Worse than the duplicate: which entry "won" depended on progress, so the id stored on disk **changed** as the user watched, and status deletion, per-anime settings and the detail screen started looking for an id that half the records did not have.

With `Media.Id` as identity that disappears by construction. What remains is what a unique identity does *not* solve: each account has its own `status` and `progress` for the same media, and collapsing the two into one entry means choosing. The choices are deliberately asymmetric, because the errors do not cost the same:

- **`progress`: the LOWEST among the accounts** (`DedupeByMedia`). An episode is only "watched" when every account has seen it; erring high deletes an episode someone has not watched yet.
- **Download: OR** — ONE account having the anime in a download status is enough. That already falls out of the union of lists in `searchAnilist`, since each fetch is filtered by `status_in` on the server.
- **Deletion: AND** — EVERY account that has the anime must have it in some deletion status, and they need not be the same (`DROPPED` on one and `COMPLETED` on another deletes). An account that **does not track** the anime does not vote; one that has it in a neutral status (`PLANNING`) **vetoes**. Distinguishing those two cases is exactly what `allAccountsAgreeOnDelete`'s extra query buys: the list fetch only brings the deletable ones, so "it did not come back in the list" is ambiguous. It only runs for an anime **with an episode on disk** that some account wants deleted — a set that empties itself, because the anime leaves disk on the same pass.

Direct consequence: **nobody may read the `Status` of the dedup winner** — it belongs to an arbitrary account. Who answers about status is `deletableMediaIDs`.

**Migration:** `MigrateAnimeIDsToMedia` resolves each old `anime_id` via `GetMediaIDForEntry` and rewrites `downloaded_episodes` and `anime_settings`, marking `anime_ids_are_media_ids` on the config. It runs at boot **and** at the top of the verification pass, and the pass **aborts** until it has run: with ids in the old format nothing on disk matches AniList, and a pass in that state would re-download the entire library. It only writes after resolving everything, so a network failure in the middle does not leave half converted. Entries that no longer exist on AniList (404) keep the old id and are only logged — the episodes stay on disk.

**Don't "fix" by:** deduplicating by name (two distinct animes can share an English title, and the name of a record that only came from `episodes.json` does not come from AniList); taking `media { id }` out of `GetFrontendAnimeList`/`GetAllCurrentAnime` for "complexity" (it is a scalar, and without it the dedup key is `0` — every entry falls into the "no media id" branch and the bug comes back whole, silently); branching on `MediaList.Status` after the dedup (it belongs to a lottery account); swapping the veto for "any account in a deletion status deletes" (one account completing an anime is enough for the episodes to vanish from under whoever is still watching); treating an account whose fetch failed as agreement (would turn a passing AniList failure into a file deletion, which is irreversible); running the migration without the completion mark (the second time it would read media ids as entry ids).

---

### 44. `tvshow.nfo` is written AFTER the hardlinks, never overwrites, and failing it does not fail organize

**Location:** `internal/files/librarian.go` (`writeShowNFO`, called at the end of `Organize`, and `BackfillShowNFOs`); `internal/daemon/jobs.go` (`organizeTorrent` fills `OrganizeRequest.AnimeID`); `cmd/daemon/main.go` (backfill call at boot).

**What it looks like:** the nfo looks like it should be written right after `MkdirAll` (it is folder metadata, not file metadata), and an error writing it looks like an `Organize` error.

**Why it's right:** three things, all because of invariants that already existed:

- **After the links:** `Organize`'s rollback is `cleanupIfEmpty`, which uses `os.Remove` and only works on an **empty** folder. An nfo written before a hardlink that fails (cross-device, permission) would leave the folder non-empty and an orphan directory in the library on every retry.
- **Does not overwrite:** if the user corrected the match by hand in Jellyfin (which rewrites the nfo), the next episode of the same anime cannot undo that. `Organize` runs once per torrent, so this happens all the time.
- **Only logs on failure:** the hardlinks are the product; returning an error here would make the job retry (backoff, then drop) work that already succeeded, and `LibraryPaths` would never be written.

`AnimeID` is AniList's **media** id (decisions.md #43) — it comes from an already-migrated `EpisodeStruct.AnimeID`. `AnimeID == 0` (old unresolvable record) simply skips the nfo: Jellyfin goes back to guessing by name, which is the previous behaviour.

**The backfill exists because `Organize` does not re-run:** `organizeTorrent` exits early when the hash's episodes already have `LibraryPaths`, and `reconcileLibrary` only enqueues whoever has empty `LibraryPaths`. Without `BackfillShowNFOs` an already-finished anime would never get the file (an in-progress one would get it on the next episode). It runs **every boot** and has no "already ran" flag: "does not overwrite" already makes it idempotent, and a flag would only create one more piece of state to desync. The folder comes from `LibraryPaths`, not from `sanitizeName(AnimeName)` — the only path that is guaranteed to match what exists on disk after manual renames or sanitization changes.

**It is gated by the id migration** (`main.go`, `else` branch of `MigrateAnimeIDsToMedia`): with ids in the old format the nfo would go out with an ENTRY id, and because it is never rewritten that would be permanent. It runs in the `else` on purpose — a migration failure only postpones the backfill to the next boot.

**Requires in Jellyfin:** the `jellyfin-plugin-anilist` plugin as a metadata provider; without it `uniqueid type="anilist"` is ignored.

**Don't "fix" by:** moving the write next to `MkdirAll`; always overwriting "to keep it in sync"; propagating the write error to `Organize`'s return; giving the backfill a config flag; deriving the backfill folder from the anime name; running the backfill before/independent of the id migration.

---

### 45. One library folder per AniList ENTRY — the season marker stays in the name

**Location:** `internal/files/librarian.go` — `sanitizeName` (used to be `sanitizeFolderName` + `sanitizeFileName`, now one) and `destDir` in `Organize`; test `TestSanitizeNameKeepsSeasonMarker`.

**What it looks like:** the "right" layout for Jellyfin/TVDB is one folder per series with `Season 03/` inside, and until `cd496c3` the code went halfway in that direction: a `seasonPattern` stripped "Season 2"/"S2"/"2nd Season"/"Cour 2" from the folder name, joining every season into one folder.

**Why it's right:** on AniList a series with several seasons **does not exist**. Each season is its own media, with id, cover, synopsis and episode numbering starting at 1 — and our data is exactly like that (one `AnimeID` per season, `EpisodeNumber` restarting). Joining the seasons into one folder forced Jellyfin to pick ONE entry for the whole folder, and it picked season 1: every new season showed up with the first's cover and metadata (real report: Mushoku Tensei — entries 108465, 127720, 146065 and 178789 all landed in `Mushoku Tensei Jobless Reincarnation`). The grouping was not even coherent: the regex ate "Season 2" but not "Part", so "Season 2 Part 2" ended up in a fifth folder.

With `tvshow.nfo` (decisions.md #44) that would stop being a guess and become a fixed error: the first season to arrive in the folder would write its `uniqueid` and, because the nfo is never overwritten, lock the folder to that season forever.

A franchise layout with `Season NN/` cannot work here either: the franchise `tvshow.nfo` would need an id AniList does not have, and `jellyfin-plugin-anilist` does not provide per-season metadata. One folder per entry is at once the smaller diff and the only model that matches the provider.

Because the two sanitization functions became identical after removing the regex, they became one: `sanitizeName`.

**Don't "fix" by:** reintroducing the season strip "to group the franchise"; creating `Season NN/` subfolders (AniList episode numbering is per entry, so E01 of season 3 is that media's episode 1, not episode 25 of the franchise); deduplicating folders by base title.

---

### 46. `GetFrontendAnimeList` is cached for 60s — the `/api/v1/animes` poll is what blows AniList's quota

**Location:** `internal/anilist/anilist.go` — `ttlCache`, `frontendListCache`, `GetFrontendAnimeList`, `frontendListResponse`, `clearCaches` (called by `MockAniListDo`); tests in `anilist_test.go`.

**What it looks like:** a TTL cache on a getter that "just reads a list", returning a copy of the slice instead of the stored slice. Looks like premature optimization on a request that takes ~1s.

**Why it's right:** AniList today limits to **30 req/min** per IP. `Status.svelte` polls `/api/v1/animes` every 30s **per open tab**, and the handler does one fetch per configured account (`fetchAniListEntries` = `GetCustomListsMap` + `GetFrontendAnimeList`). With 2 accounts and 2 tabs that alone is ~16 req/min of idle UI — add an F5, the 10-minute verification cycle and orphan `GetAnimeInfo` and the 429 is guaranteed. Real diagnosis (`daemon.log`, 2026-08-10 11:09): 15 requests to `/api/v1/animes` in 2 minutes, followed by cascading 429s that also took down the daemon's `searchAnilist` (`verification.go`) — i.e. the open screen stopped automatic download from running. With the cache, N tabs cost at most 1 request per minute per account.

The 60s TTL is safe because this list only changes when the user edits AniList; downloaded-episode counts come from the local `episodes.json` and stay instant. The key includes username + statuses, so a config change produces a new key (no invalidation needed).

**A copy, not the stored slice:** `fetchAniListEntries` overwrites `ml.CustomLists` on the entries it receives. Returning the cache slice would make two concurrent `/api/v1/animes` requests write the same memory (real race, caught by `-race`) and leak one caller's overlay into the next.

**Don't "fix" by:**
- Caching `GetAllCurrentAnime` too — that is the daemon cycle, which runs every 10 min and **needs** fresh data to decide download/deletion.
- Increasing the frontend poll interval instead of the cache — does not solve F5, multiple tabs, or multiple devices on the LAN.
- Returning the cache slice directly "to save the copy".
- Caching the orphan refresh (`GetAnimeInfoByIDs`) too: that path is already off when the list fails (decisions.md #42d), which is when it amplifies — and since it became a batch (#65) it costs 1 request per account per poll, the same order as the list fetch next to it, which is cached.

---

### 47. Notifications are grouped by window, and the body is escaped only when the preset is JSON

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

---

### 48. `max_episodes_per_anime` never limited batches — the rule now lives on the pack window, not a count cap

**See also:** [#59](#59-batch-eligibility-stopped-being-metadata-and-became-a-result-filter) — pack eligibility is the filtered search result, not a count cap (`max_batch_episodes`, removed). Entries 59-62 document the current design.

**Location (historical):** when this decision was written, `daemon/episodes.go` had `willBatchAnime` and `resolveSearchStrategy`; both were removed by the batch-by-filter spec. What exists today: `daemon/episodes.go` — `selectEpisodes`, `effectiveMax`, `windowEnd`, `partitionSearchResults`, `pickBatches`, `packSet.covering`, `assignBatches`; `daemon/search.go` — `filterBySize`; `daemon/helpers.go` — `checkDiskSpace`/`ErrInsufficientDiskSpace`; `daemon/jobs.go` — `partiallyOrganized`; `api/endpoint_status.go` — `disk_low`.

**What it looks like:** a per-anime limit that sometimes does not apply, a size filter that lets size zero through, a disk guard that only bars `Add`, and a "disk low" boolean computed on the server for the frontend.

**Why it's right:**

**(a) The limit applies only episode by episode.** A batch is **one torrent**: a finished 24-episode anime with the limit at 12 downloaded the 24 files, `organize` hardlinked the 24, and `episodes.json` recorded 12 — the screen showed 12/24 forever with the 24 on disk. Limiting records does not limit bytes or files; it only produces a lying count. So, when a pack covers the pending window, the second `selectEpisodes` runs with the limit lifted (`len(episodes)+1`), and the effective limit becomes unlimited for that pack's episodes.

What decides pack eligibility today is **no longer** a count cap on the anime's episodes (`max_batch_episodes`, removed) — it is the filtered search result itself: `partitionSearchResults` splits packs from loose episodes and applies to each its OWN size ceiling (`max_batch_torrent_size_gb`/`max_episode_torrent_size_gb`) and the seeders floor; `pickBatches` picks the fewest packs that cover `[firstPending, firstPending + max_episodes_per_anime - 1]` (or the whole window, with the cap off). See entry 59 for why that swap.

**(b) The fallback.** If the pack search does not cover the pending window (no results, or the size filter empties it), the original selection — with the per-anime limit in force — remains: `episodesToDownload` was never replaced, so there is nothing to cut back. `airingSchedule` order guarantees the oldest ones are what stay.

**(c) `Size == 0` passes the size filter.** That is `nyaa.parseSize`'s value when Nyaa changes the column format. Discarding unknown size would swap "sometimes downloads a large torrent" for "downloads nothing": a parsing bug would become a silent stall. The filter runs **after** priority sort and preserves order, so the pick is the best **among those that fit**.

**(d) The disk guard bars `Add`, not the pass.** Watched pruning, `deleteEpisodesByStatus` and `organize` are exactly what **frees** space; an `if disk full { return }` at the start of the pass would leave the app stuck in the state where it cannot unclog itself. A `statfs` error does not block — a volume that does not answer (network, permission) is not proof of a full disk. When it bars, it tries no magnet and does not retry (the magnet is not the problem) and notifies `ReasonNoDiskSpace`, which the notification batch groups.

**(e) The disk threshold lives on the server.** `GET /status` returns `disk_low`, computed with `min_free_disk_percent`. One place decides what "low" is, and the screen warning cannot disagree with what the daemon is doing.

**(f) `organizeTorrent` does not re-notify a partially organized group.** If part of the group already has `LibraryPaths`, the torrent already landed and the webhook already went out: it organizes (it is idempotent), writes the marker on the new records, does not notify. Without that, the first post-upgrade pass of an existing library — where rule (a) creates the missing records for packs already on disk — would become a flood of duplicate `download_completed`.

**Don't "fix" by:**
- Applying `max_episodes_per_anime` to batches "so the limit always holds": that brings the lying count back.
- Reintroducing an episode-count cap on the anime (like `max_batch_episodes`) to decide pack eligibility: episode count is a bad proxy for size — see decisions.md #59.
- Discarding a torrent with `Size == 0` "because unknown size is suspicious".
- Consolidating the disk guard into a `return` at the top of the verification pass (see (d)).
- Moving the guard into `torrents.Session.Add` "because it's one site": the `torrents` package does not know `files.Config`, and passing config there inverts the dependency.
- Reintroducing a hardcoded ratio in `lib/utils/status.ts` to save a field on the response (see (e)).

### 59. Batch eligibility stopped being metadata and became a result filter

**Location:** `daemon/episodes.go` — `partitionSearchResults`, `pickBatches`, `packSet.covering`; `nyaa/nyaa.go` — `ExtractBatchInfo`, `ScrapNyaaForAnime`.

**What it looks like:** no `FINISHED` check, no check of the `Media` episode count before deciding whether an anime "becomes a pack". The whole decision lives inside the already-filtered search result.

**Why it's right:** what matters for pack eligibility is the torrent that is there — size, seeders, covered range — and none of that is known **before** the search. An episode-count cap (like the old `max_batch_episodes`) is a bad proxy for size: measured, 26 remux episodes go past 300 GB (would punch through any reasonable count cap) and 500 episodes in 480p sit at ~80 GB (a low count cap would block a pack that would fit comfortably). And the `FINISHED` gate excluded exactly who depends on packs the most — a long `RELEASING` series (One Piece, Naruto) never had another way not to accumulate 1000+ episode-by-episode records.

Today who decides is `partitionSearchResults` (splits packs from loose episodes and applies to EACH list its own size ceiling — `max_batch_torrent_size_gb`/`max_episode_torrent_size_gb` — and the seeders floor) followed by `pickBatches` (picks the fewest packs, among those that survived the filter, that cover the pending window).

**Known ceiling (ponytail, phantom-range guard):** `ExtractBatchInfo` only guarded the dominant phantom-range case — a resolution marker (`[720-1080p]`) being read as episodes 720-1080 — betting that other phantom ranges (date, bitrate) would fall into `EndEpisode == 0`, the safe fallback. **RESOLVED** (see #83): the bet was wrong for the year. `(2021-2022)` does not fall to zero, it becomes the range 2021..2022, and a complete top-seeded pack was discarded in silence for covering no episode.

**Known ceiling (ponytail, pagination floor):** `ScrapNyaaForAnime` fused pack and loose episode into one search, and the pagination floor (`enoughCandidates`, decisions.md #57) started counting BOTH lists summed. A page 1 with 3 packs that the size filter later discards ends the descent without having gathered a single loose episode — before, with two separate searches, the second would descend on its own. **RESOLVED** (see #80): it showed up measured, on One Piece. The pack ceiling started being pushed into the `nyaa` package by `applyNyaaSettings` — the same push mechanism as `SetMaxSearchPages`, not a direct read of `files.Config` — and a pack above the ceiling leaves before counting toward the floor. The seeders filter stays only in the daemon: Nyaa returns ordered by seeders desc, so it does not truncate the descent the same way.

**Don't "fix" by:**
- Reintroducing an episode-count cap on the anime: count is a bad proxy for size (see above), and the `FINISHED` gate excluded who needs packs the most.
- Accepting `EndEpisode == 0` as "range 0-0" instead of "complete pack": a real pack whose range failed to parse would disappear from selection.

### 60. The pack window covers from the first pending episode, and the cut is always a prefix

**Location:** `daemon/episodes.go` — `windowEnd`, `pickBatches`, `assignBatches`.

**What it looks like:** `pickBatches(packs, firstPending, windowEnd)` walks a cursor from the first pending episode, and `assignBatches` stops assigning a magnet at the first episode in the list that no chosen pack covers — instead of giving everyone the magnet of the first pack found, or trying a loose episode for the rest.

**Why it's right:** without the window anchored on the first pending, `batchResult[0]` would go to every pending episode and episode 800 of a long series would receive the magnet of pack 1-100 — wrong data, not "no magnet". Without the prefix truncation in `assignBatches`, episodes outside coverage would fall into the loose-episode fallback (`magnetsByEpisode`), which on One Piece would fire `DownloadFailed`/`ReasonNotFound` for ~1000 episodes **per pass** — none of them will show up loose on Nyaa, so it is pure error notification, with no chance of success. The cut is always a **prefix** (never "a hole in the middle") because the schedule (`anilist.EpisodeList`) comes in increasing order and the packs `pickBatches` chooses are contiguous from the cursor — there is no way a later pack covers an earlier episode without `packSet.covering` already having picked it first.

**Don't "fix" by:**
- Applying the magnet of the first pack found to every pending episode "because it's an episode either way" — an episode outside the pack's range simply is not in the `.torrent`.
- Falling back to a loose episode for what remains outside the window: on a long series that is a guaranteed error notification, not one more attempt.

### 61. A pack episode is immune to the limit prune AND to the watched keep-set

**Location:** `daemon/episodes.go` — `handleAlreadySavedEpisode` (deletion guard), `buildWatchedKeepSet` (keep-set guard).

**What it looks like:** `handleAlreadySavedEpisode` refuses to delete a saved episode with `IsBatch: true` even when it is above the per-anime limit; `buildWatchedKeepSet` never includes a pack episode in the candidates for "keep even if watched".

**Why it's right:** a pack is one torrent — deleting the record of ONE of its episodes frees no byte, the whole torrent keeps seeding on disk. Without the guard in `handleAlreadySavedEpisode`, the cycle after the one that brought the pack would delete the records above the limit, the space gate would reopen (the record vanished, the byte did not), and the next cycle would try to download again — a download-and-delete loop over a torrent that never left disk. Without the guard in `buildWatchedKeepSet`: `watched_episodes_to_keep` has no granularity **inside** a pack — keeping 3 of a pack's 100 episodes does not keep 3 episodes, it keeps the whole pack (`canRemoveTorrent` returns `false` for the entire group while ANY of the 100 records is outside the delete-set), so including pack episodes in the keep-set does not change the result, it only confuses the count.

The counter (`downloadedEpisodes`) still increments for pack episodes **below** the limit — only deletion above the limit is ignored. It is that normal increment, plus immunity to the prune, that gives the **rotation of successive packs with no new config**: the current pack accumulates records past the limit (immune), the next cycle searches again, `pickBatches` picks the next still-uncovered pack, and so on — only braked by `max_batch_torrent_size_gb` (per torrent) and `checkDiskSpace`.

**Don't "fix" by:**
- Deleting pack records above the limit "so the limit always holds": the torrent stays on disk, and the next cycle downloads again (download-and-delete loop).
- Giving the keep-set granularity inside a pack "to honour `watched_episodes_to_keep` to the letter": the real unit is the torrent, not the record — there is no "keep 3 episodes" of a single file.

### 62. Standalone progress lives in `AnimeSettings` and is injected into the synthetic `MediaList`, it does not become a parallel path

**Location:** `files/filemanager.go` — `AnimeSettings.Progress`; `api/endpoint_anime_settings.go` — `handleAnimeSettings` (partial `PUT`); `daemon/standalone.go` / `api/standalone.go` — `withStandaloneProgress` (two copies, one per package, same contract); `daemon/manual_download.go` — `resolveAnimeDetails`.

**What it looks like:** `AnimeSettings` (per anime, a file separate from `Config`) gains a `Progress int` field; every time a synthetic standalone `MediaList` is built (`appendStandaloneAnimes`, `DownloadStandaloneAnime`, `resolveAnimeDetails` when `GetAnimeInfo` returns `nil`), `withStandaloneProgress` overwrites `ml.Progress` with the saved value before the `MediaList` is used by anything.

**Why it's right:** with the data living in the same field a real list entry uses, `shouldSkipEpisode` (skips episode `<= Progress`), `firstEpisodeToConsider` (starts at `Progress + 1`), `buildWatchedKeepSet` (uses `Progress` to find which saved ones count as watched) and the UI's `EpisodesWatched` count work on standalone **with no `if isStandalone`** — it is the same read as always, only the source of truth for standalone is `AnimeSettings` instead of AniList. A parallel path (an explicit branch at each of those points) would duplicate the "what is watched" logic in four places and one of them would diverge sooner or later.

`resolveAnimeDetails` calls `withStandaloneProgress` on the SAME fallback branch `appendStandaloneAnimes` uses (`GetAnimeInfo` returns `nil` → `GetMediaByID`) — without that, `RunAnimeDebug` (which goes through `resolveAnimeDetails`) reported a standalone with saved progress as if it started at episode 1, diverging from the real pipeline it exists to mirror (decisions.md #58). On `ManualDownload*` the effect is harmless — they select the episode by explicit number, not by `Progress` — but the call is the same so it does not depend on which path a future caller picks.

`PUT /animes/{id}/settings` uses a pointer (`*int`) and does a partial merge: a `PUT` whose body does not mention `progress` cannot zero it — a fresh `AnimeSettings{}` on every `PUT` would silently wipe the saved progress. The pointer is still needed with a single field, because `0` is valid progress and indistinguishable from absent without it. `progress < 0` is rejected with 400 in the handler validation, before the merge.

**Don't "fix" by:**
- An `if anime.IsStandalone` branch in `shouldSkipEpisode`/`firstEpisodeToConsider`/`buildWatchedKeepSet` "to make it explicit": that is exactly the duplication injecting into the `MediaList` avoids.
- A `PUT` replacing the whole `AnimeSettings`, or swapping the `*int` for `int` "because only one field is left": both zero progress on a body that did not mention it.
- Skipping `withStandaloneProgress` in `resolveAnimeDetails` "because `ManualDownload*` does not use `Progress`": `RunAnimeDebug` does, and both go through the same path.

### 49. Standalone anime: "tracked by the list" is the snapshot the daemon PROCESSES, and `DownloadStandaloneAnime` never calls `handleSavedEpisodes`

**Location:** `files/standalone.go`; `anilist/standalone.go` — `SearchMedia`/`GetMediaByID`; `daemon/standalone.go` — `appendStandaloneAnimes`/`DownloadStandaloneAnime`; `daemon/verification.go` — `searchAnilist`; `daemon/helpers.go` — `isConfigComplete`; `api/standalone_guard.go` — `blockReason`; `api/standalone.go` — `resolveMediaList`; `api/endpoint_config.go` — `PUT` validation.

**What it looks like:** a `standalone_animes` file with loose ids; a block function that serves both the `POST` and the search result; a "download now" that does no cleanup; and a standalone anime that is never deleted by automatic pruning.

**Why it's right:**

**(a) The append happens AFTER `DedupeByMedia`.** `GetMediaByID` returns a **synthetic** `MediaList`: `Progress: 0`, `Status: ""`, `Id: 0`. `DedupeByMedia` keeps the **lowest** progress, so appending before the dedupe would make the synthetic zero beat the real entry and the daemon would re-download already-watched episodes. It is order, not style.

**(b) "Tracked by the list" = it appears in the set the daemon processes,** i.e. the entries that survive the `download_statuses` + `DownloadMediaStatuses` filter — **not** "an AniList entry exists". An anime in `PLANNING` with `download_statuses = [CURRENT]` is on a list and the daemon ignores it; adding it as standalone is the feature's most obvious use case, not an error to block. The same definition feeds the three consumers (automatic removal, the `POST`'s 409, the search's `block_reason`) because two different definitions would produce a grey card the backend accepts, or the inverse.

**(c) One block function, two consumers.** `blockReason` decides the 409 and each search result's `block_reason`. "The front does not let you click" and "the back returns an error" need to agree **by construction**, not by discipline. Precedence `blacklist > standalone > tracked > downloaded`: blacklist comes first because it is the only reason where adding would change behaviour **for the worse** — a blacklisted one outside `download_statuses` escapes `searchAnilist`, the standalone record survives the merge and the user's filter is bypassed (the synthetic `MediaList` has null `CustomLists`, so `animeIsInExcludedList` never fires on it). `downloaded` only blocks with a **known** total: 12 records of 24 episodes is the per-anime limit, not "already downloaded".

**(d) `DownloadStandaloneAnime` must not call `handleSavedEpisodes`.** It calls `identifyEpisodesNotInWatching`, which compares **all** saved episodes against the received `checkedEpisodes` and returns the uncovered ones for deletion. With only one anime's episodes in hand and `delete_watched_episodes` on, that would delete the entire library. For the same reason it does not call `deleteEpisodesByStatus` or `DeleteEmptyFolders`: adding an anime is not an occasion to delete anything.

**(e) A standalone is never deleted by status.** `deletableMediaIDs` only evaluates candidates that appeared on some account's list, and a standalone appears on none. It accumulates on disk until the user stops tracking it, **or** until they mark by hand how far they have watched — see decisions.md #62 on `AnimeSettings.Progress`, which is the deliberate exception: without manual progress `delete_watched_episodes`/`watched_episodes_to_keep` never had anything to prune on a standalone.

**(f) Automatic removal consumes the record, and that can go as far as deleting a file.** A standalone that later enters an AniList list leaves the file on the next pass (the real entry already won the dedupe, so what the pass downloads does not change). If **after that** the user takes the anime off the list, the standalone record has already been consumed and its episodes fall into `identifyEpisodesNotInWatching` like those of any anime taken off the list. That is the accepted trade: keeping both records would require distinguishing "active standalone" from "dormant standalone" in the file, in `AnimeInfo` and on the screen, to cover a path standalone users do not take. The explicit `DELETE` closes the **other** window by marking the episodes `ManuallyManaged`.

**(g) An AniList account stopped being required.** `isConfigComplete`, `searchAnilist` and `PUT /config` validation required at least one `anilist_usernames`; with standalones that became a requirement with no function — an install that only uses standalones never configures any account, and the daemon would leave it stuck on the configuration screen forever. The **library** stays required in all three places: without it there is nowhere to download to. `searchAnilist` without an account is not an error — the pass still has standalones to process, and aborting there would make the feature never run. The rule also left the frontend's `requiredChecks`: a requirement only on the screen is a rule the server does not know.

**(h) The concept is called `standalone` in the code, "avulso" in the UI, and never `manual`.** `manual` already means something else here — `ManuallyManaged`, `ManualDownloadEpisode` mean "the user touched this episode by hand, the loop must not touch it". Reusing the word would make two different concepts read the same.

**Don't "fix" by:**
- Appending standalones before `DedupeByMedia` "because the merge is one" (see (a)).
- Swapping `tracked` for "an AniList entry exists" to avoid depending on `fetchAniListEntries` — that breaks the `PLANNING` case, which is the feature's use case.
- Duplicating the block rule in the frontend to not need the `block_reason` field on the search response (see (c)).
- Returning four booleans instead of one `block_reason`: the reasons are mutually exclusive by precedence and the card needs one label.
- Consolidating `DownloadStandaloneAnime` with `handleSavedEpisodes` "to reuse the save" (see (d)).
- Filtering the standalone by `DownloadMediaStatuses` on append: whoever asked for the anime by hand wants to track it also while it is `NOT_YET_RELEASED`.
- Reintroducing `anilist_usernames` as a required field (on any of the four layers) "because the app is about the AniList list": it stopped being (see (g)).

### 50. Standalone search: the unreleased filter is server-side and by concatenation, and a blocked card becomes a link instead of a dead end

**Location:** `anilist/standalone.go` — `SearchMedia`; `api/endpoint_anilist_search.go`; `frontend/src/routes/AddAnime.svelte`; `frontend/src/components/ui/Button.svelte`.

**What it looks like:** a `status_not: NOT_YET_RELEASED` **stuck onto the query string** instead of passed as a GraphQL variable; a toggle that does not know how many results it hid; and a `Button` that sometimes renders `<a>`.

**Why it's right:**

**(a) The filter lives in the query, not in Go.** `perPage` is 20. Filtering after receiving would return searches with 4 useful results in announcement season — the user would read that as "search is broken", not as "I hid 16". The cost is that **nothing knows how many were hidden**, so the toggle is blind: there is no "3 hidden · show". That was the chosen trade; inverting the two is a product decision, not a bug.

**(b) Concatenation, not a null GraphQL variable.** AniList ignores arguments with a null value today, and `status_not: $statusNot` with `$statusNot = null` would work. But if that semantics changes, the filter starts hiding everything or nothing **in silence** — the worst failure mode for a filter. With concatenation, `includeUnreleased = true` makes the argument simply not exist in the query, which is true on any API version. It costs three lines and the test inspects the body sent, not the result, precisely because both implementations would pass a result test.

**(c) Only `NOT_YET_RELEASED` leaves.** `CANCELLED` and `HIATUS` stay: an anime interrupted mid-airing has episodes out and downloadable. "Not released yet" is one status.

**(d) `tracked`/`downloaded`/`standalone` become a link to `#/status/{id}`.** `anime_id` **is** AniList's media id, so the href comes from the search result itself, with no lookup — and `AddStandaloneAnime` writes before answering, so the link works the instant after the Add click, without waiting for a daemon cycle. The block reason left the `tooltip` and became a line on the card: tooltips do not exist on mobile, and the screen was born for the no-account install, which is the one that runs on a phone the most. `blacklist` is the only exception — it is not in the set the daemon processes, it has no page, it stays greyed and disabled.

**(e) A `Button` with `href` renders `<a>` instead of an `on:click` that writes `location.hash`.** A button that navigates loses middle-click, "open in new tab" and the URL in the status bar. The alternative was copying the variant classes into the card, which would duplicate the style on every screen that needs it. `disabled` is ignored with `href` on purpose: there is no disabled anchor, and a link that must not be followed must not be rendered as a link.

**(f) Search fires on submit (button or Enter), never on every key.** The original version had a 300ms debounce. The real cost is not the fast typist — they already spent 1 request per search — it is whoever types with more than 300ms between keys (phone, hunt-and-peck): from the 3rd character they spent **one request per key**. Because `SearchMedia` is `PriorityDisposable`, whoever blew the bucket was refused by `budgetAllows` on the *next* search — the user saw "search failed" for having typed slowly. With submit, one search = one request, and the budget floor is never hit by the screen. The `<form>` exists so Enter is native behaviour instead of an `on:keydown`; the `AbortController` stays because two submits in a row still race each other. The unreleased toggle only redoes the search when a result is already on screen — with no search done, it only picks the parameter of the next submit.

**(g) The search cache lives in `SearchMedia`, not on the endpoint response.** `block_reason` is recomputed by `standaloneGuard` on every response; cached with it, an anime added just now would keep the "Add" button on the card until the TTL expired. Caching only the part that comes from AniList keeps the guard fresh for free. The key is `strings.ToLower(term)+includeUnreleased` — AniList search is case-insensitive, so "One Piece" and "one piece" are not worth two requests. It started paying off together with (f): with submit the keys are whole terms, which repeat (the same user redoing the search, the toggle going back and forth, another tab); with debounce they were prefixes nobody repeated. It is also **the only cache with a free-text key** — the others are anime id and account name, a finite set — and because `ttlCache` never removes an expired entry (`getStale` depends on that), it is the only one that needs a ceiling: past `searchCacheMaxEntries` the map is discarded whole. Whole discard and not LRU on purpose; the `ponytail:` in the code notes the upgrade.

**Don't "fix" by:**
- Moving the filter into Go "so we can show how many were hidden": that reintroduces the search with 4 of 20 (see (a)).
- Going back to searching on `on:input`, with or without debounce "because live results are better UX": AniList's budget is 30 req/min per IP and the search is disposable (see (f)).
- Lifting the search cache onto the endpoint response "because that's one fewer layer": `block_reason` freezes with it (see (g)).
- Caching the search in the frontend instead of the backend: AniList's limit is per IP, so a per-tab cache does not protect the other tabs or the daemon pass — and the budget gate, which is who the cache exists to spare, lives in the backend (see (g)).
- Swapping concatenation for `status_not: $var` with a null value "because a variable is cleaner" (see (b)).
- Extending the filter to `CANCELLED`/`HIATUS` — those have downloadable episodes (see (c)).
- Putting `block_reason` back inside a `tooltip` "to keep the card cleaner": the reason vanishes on all of mobile.
- Making the whole card an `<a>`: the Add button inside the anchor would need `stopPropagation` and would break keyboard navigation.

### 51. An episode number accepts 4 digits, except inside brackets

**Location:** `nyaa/nyaa_regex.go` — `reEpisodePatterns`, `reBatchRange`, `reHasEpisode`, `reBatchPatterns`; `nyaa/nyaa.go` — `ExtractEpisodeNumber` (ceiling `< 10000`).

**What it looks like:** every episode pattern uses `\d{1,4}`, except `\[(\d{1,3})\]`.

**Why it's right:** with `\d{1,3}` One Piece (ep. 1123+) matched no pattern, `ExtractEpisodeNumber` returned `nil` and **every** Nyaa result was discarded — the log showed 30 good lines and `results: 0`. `[05]` stays at 3 digits because `[2025]` (a year) is far more common in a torrent name than a 4-digit episode inside brackets, and a year read as an episode discards the right torrent.

**Don't "fix" by:** extending `\[(\d{1,3})\]` to 4 digits "for consistency".

### 52. The episode list is synthesized, and an episode's key is (anime, number)

**Location:** `anilist/episodes.go` — `EpisodeList`, `lastAiredEpisode`; `anilist/anilist.go` — `Media.NextAiringEpisode`; `files/filemanager.go` — `EpisodeKey`, `EpisodeStruct.Key()`, `loadBlockedEpisodesLocked`; `daemon/episodes.go` — `firstEpisodeToConsider`; `api/endpoint_episode_actions.go` and the `/animes/{id}/episodes/{episodeNumber}/*` routes.

**What it looks like:** **fabricated** `AiringNode`s (id zero) for episodes AniList does not return; `EpisodeStruct` without `EpisodeID`; `blocked_episodes` storing objects; and the routes' `{episodeId}` becoming `{episodeNumber}`.

**Why it's right:**

**(a) `airingSchedule` is not an anime's episode list.** AniList keeps a **window** of ~500 schedule entries per media and discards the old ones. Measured on the API: `Media(21)` (One Piece) has `pageInfo.total = 500` and page 1 **starts at episode 1123** — 1 to 1122 do not exist on the API; `Media(20)` (Naruto, 220 eps) and `Media(16498)` (Shingeki no Kyojin, 25 eps) come back with `total = 0`, an **empty** schedule. While the list came from there, the daemon literally had no episode to process on those animes (the log showed `Processing anime episodes` and nothing after), and on One Piece it started at 1123 as if the earlier ones did not exist. On top of that the code never paginated the schedule, so even inside the window only the first 25 nodes arrived — and paginating would solve nothing, because later pages only bring **newer** episodes.

**(b) That is why the list is synthesized, and the last aired comes from three sources.** `lastAiredEpisode` combines the highest already-aired episode from the schedule, `nextAiringEpisode.episode - 1` and `media.episodes` — the last **only when `FINISHED`**: on a `RELEASING` it is the season's EXPECTED count, and using it would send the daemon hunting an episode that has not aired yet. None of the three works alone (clipped/empty schedule, null `nextAiringEpisode` on a finished anime).

**(c) The key became `(AnimeID, EpisodeNumber)`, not a synthetic id.** An episode outside the window has no node id — and inventing one (`-(mediaID*10000+ep)`) would make an episode's identity depend on a formula nothing validates, with silent collision as the failure mode. Episode number inside an anime is unique by definition and is what the user sees. `EpisodeStruct.EpisodeID` was **removed** (not left as a dead field) on purpose: it is the compiler that points at every place that still identified an episode by the old id.

**(d) Legacy `blocked_episodes` is discarded, not migrated.** It was `[416348, ...]` — node ids that nothing resolves anymore. A parse that fails would stall `LoadBlockedEpisodes` and with it the verification pass, so the old file is read, logged as a warning and treated as an empty list. The user remakes the blocks with one click per episode; guessing would be worse.

**(e) `firstEpisodeToConsider` walks back to the smallest saved episode.** The rule is `Progress + 1` (and a standalone anime has `Progress = 0`, so it starts at 1 with no flag needed). But `watched_episodes_to_keep` and watched pruning only operate on episodes that appear in the list: a saved episode below progress would sit outside it, never be "checked" and fall straight into `identifyEpisodesNotInWatching` — deleted ignoring "how many watched to keep".

**(f) `{episodeId}` became `{episodeNumber}` on the routes, and `episode_id` left the JSON.** The routes already had the right shape; keeping the old name with new semantics would be the worst possible combination. `AnimeInfo.latest_episode_id` became `latest_episode_number` for the same reason.

**Don't "fix" by:**
- Paginating `airingSchedule` "to get the missing episodes": later pages only have newer episodes (see (a)).
- Using `media.episodes` as last aired on a `RELEASING` anime (see (b)).
- Reintroducing a synthetic `EpisodeID` "to not touch on-disk state" (see (c)).
- Trying to convert the old `blocked_episodes` (see (d)).
- Making the list start at `Progress + 1` without the walk-back (see (e)).

### 53. Episode search drops batches and movies, and uses `hasMovieMarker` — not `isMovie`

**Location:** `nyaa/nyaa.go` — `parseRow` of `ScrapNyaa` and of `ScrapNyaaForAnime` (at the time of this decision, `ScrapNyaaForMultipleEpisodes`, later fused into `ScrapNyaaForAnime`), `isMovie`, `hasMovieMarker`; `nyaa/nyaa_regex.go` — `reOvaPattern`.

**What it looks like:** two guards at the start of both episode `parseRow`s (`IsBatch` and `hasMovieMarker`), and a `hasMovieMarker` that is `isMovie` **minus** the last branch.

**Why it's right:**

**(a) Without the batch guard, a 220-episode pack enters as "episode 1".** `[Erai-raws] Naruto - 001 ~ 220 [480p]` matches `- 001` in `ExtractEpisodeNumber`, and the multi-episode search (unlike the single) did not filter batches. Result measured, at the time: 220 episodes in one record, with `IsBatch: false`, punching through the then batch ceiling (`max_batch_episodes`, removed by the batch-by-filter spec) **and** `max_episodes_per_anime` — and organized as a single episode (renamed "Naruto - E01" with `rename_files_for_jellyfin`). Discard rather than accept as a batch because the pack search ran **earlier** in strategy resolution: a batch worth taking would already have been caught there, and what leaks here is exactly what the ceiling rejected. The guard itself (mark `IsBatch`/discard leaked packs from inside the episode parser) still holds in `ScrapNyaaForAnime` today.

**(a2) The guard only holds if `IsBatch` recognizes the range.** The range pattern required a space AFTER the second number (`\s\d{2,4}\s*[-~]\s*\d{2,4}\s`), so `One Piece EP 001-501` and `One Piece 001-501.mkv` passed as "episode 1" — the same bug as (a) through another hole, measured live on the One Piece episode 1 search (2 of the 7 candidates were 500-episode packs). The end of the range now accepts space, end of name, `.` or `[`.

**(b) `isMovie`'s `!reHasEpisode && !IsBatch` branch cannot become an episode guard.** `reHasEpisode` only covers `- 05`, `episode 05` and `S01E05`; `ExtractEpisodeNumber` also covers `EP05`, `E05`, `[05]`, `" 05 ("`, `" 05.mkv"` and `" 5"`. Calling all of `isMovie` would reject every release in those forms (the "Lucky Star EP015 / E015 / 15" tests catch that). Hence the split: `hasMovieMarker` (keywords + OVA/ONA + special) serves both sides, and `isMovie` remains `hasMovieMarker` plus the "has no episode marker" fallback that only makes sense on a movie search.

**(c) `reOvaPattern` needs `\b`.** It was `\(?(ova|ona)\)?`, with no boundary: "ona" matches inside **Persona**, "ova" inside **Nova**. While the pattern was only used in the permissive direction (`ScrapNyaaForMovie`) that passed; as an episode guard it would become rejection of the whole anime.

**Don't "fix" by:**
- Swapping `hasMovieMarker` for `isMovie` in the episode guard "to reuse the function" (see (b)).
- Accepting the leaked batch by marking `IsBatch: true` instead of discarding (see (a)).
- Taking `\b` off `reOvaPattern` to "also catch `(OVA)`" — `\b` already matches before `(` and after `)` (see (c)).

### 54. Seeders floor: unreadable seeders are discarded, unreadable size passes

**Location:** `daemon/search.go` — `filterBySeeders`, `filterSearchResults`, `filterBySize`; `files/filemanager.go` — `Config.MinSeeders` (default `1`); `nyaa/nyaa.go` — `ParseSeeders`.

**What it looks like:** two filters with an almost identical contract and an asymmetry of purpose — `Size == 0` (parse failed) **passes** the size ceiling, `seeders == 0` (including `"-"`) **does not pass** the floor.

**Why it's right:**

**(a) With no floor, a dead torrent is a valid candidate — and it wins when it is the only one.** Measured: `[DB] Naruto Shippuuden Movie 3` with **0 peers** was episode 3's only candidate and was downloaded. The default `1` bars only that; `0` turns it off, following the `max_batch_torrent_size_gb` convention.

**(b) The asymmetry with `filterBySize` is the point, not an oversight.** Unknown size means "I don't know if it fits", and discarding would swap "sometimes downloads a large torrent" for "downloads nothing" — a parsing bug would become a silent stall. Unknown seeders is different: the column is the **only** signal that someone is seeding, and a torrent with no seeder never completes. Letting it through is not the conservative side — it is pinning the episode on a torrent that does not download.

**(c) The filter stayed in the daemon, not in nyaa's `parseRow`.** The plan asked for it in parse, "before any sort". Because the sort is stable and the filters preserve order, the pick is the same; and in the daemon it reuses `filterBySize`'s pattern, holds for every search call site at one point (`filterSearchResults`) and keeps nyaa's `"Found N results"` log showing the dead torrent with its seeders — which is what diagnoses. `nyaa.ParseSeeders` exists only for that (same pattern as `IsBatch`/`ExtractSeason`).

**Don't "fix" by:**
- Making `Size == 0` discarded "for symmetry with seeders" (see (b)).
- Raising the default to "only a healthy torrent": ranking is `health`'s job, not the floor's — the floor exists for what does not download at all.

### 55. `health` compares a seeders BAND, and it comes before `fansub`

**Location:** `nyaa/priorities.go` — `criterionCompare["health"]`, `healthTier`, `healthTierFloors`, `DefaultPriorities().CriteriaOrder`; `nyaa/nyaa.go` — `torrentHealthScore` (log only).

**What it looks like:** the `health` comparator does not use `torrentHealthScore`; it uses a band index (`0 / 1-4 / 5-19 / 20-99 / 100-399 / 400+`). And `health` is in 4th position of the default order, not 7th.

**Why it's right:**

**(a) The sort is lexicographic, so a raw score = health alone decides or never decides.** With a continuous score, a 1-seeder difference already settles the comparison and no criterion after `health` is ever consulted. Comparing bands, 400 seeders beats 12 (bands 5 and 3), but 150 vs 300 ties and `fansub` decides — which is the intended behaviour.

**(b) `health` in 7th position was never reached.** Measured on With You: the 3 candidates were 1080p, `fansub` broke the tie (subsplease=0, judas=2, ember=5) and `health` was not consulted — a torrent with 12 seeders would beat one with 400 by fansub order. The new order is `uncensored, source, resolution, health, codec, fansub, audio, size`.

**(c) Fixed band, not a ratio.** "Tie unless one has 2x the other" looks better (no edge effect), but it is not transitive — 100 ~ 150 ~ 220 and 100 < 220 — and `sort.SliceStable` with an intransitive comparator returns arbitrary order. The cost of a fixed band is the edge: 399 and 400 fall into different bands. Accepted on purpose; any bucketing has an edge.

**(d) `torrentHealthScore` stayed log-only.** It combines seeders with the seeders/leechers ratio, information the band (seeders only) discards. It is worth as a diagnosis in `matched_torrents` (`t=` band, `h=` score), not as a criterion.

**Rollout note:** an existing `config.json` **already has** `criteria_order` written and `LoadConfigs` unmarshals over the default, so the new order does NOT arrive on its own in an old install. The user edits by hand or clicks "restore defaults" on the Priorities page.

**Don't "fix" by:**
- Putting the comparator back on `torrentHealthScore` "because it uses more information" (see (a) and (d)).
- Swapping the band for a ratio/percent tolerance (see (c)).
- Migrating an existing config's `criteria_order` automatically: silently reordering the preference the user saved is worse than the rollout note.

### 56. A long series also searches the episode with zero-padding, and the gate is `LastAiredEpisode`

**Location:** `nyaa/nyaa.go` — `episodeQueries`, `longSeriesEpisodes`, `ScrapNyaa`; `anilist/episodes.go` — `LastAiredEpisode`; `daemon/episodes.go` and `daemon/manual_download.go` — `seriesLength` / `totalEpisodes`.

**What it looks like:** `ScrapNyaa` does two searches (`one piece 1` and `one piece 001`) on a long anime, and "is it long?" comes from `anilist.LastAiredEpisode`, not from `Media.Episodes`.

**Why it's right:**

**(a) `q=one piece 1` never finds episode 1.** The query matches everything that contains "1" and, ordered by seeders, the first hundreds of rows are episodes 1160-1173. Measured: 375 rows without episode 1. Groups number a long series with padding (`[Judas] One Piece 001-574`, `[Erai-raws] Naruto Shippuuden - 001 ~ 079`).

**(b) ADDITIONAL variant, not a replacement.** Padding is convention, not a rule: whoever numbers loose (`Show 5`) is still caught by the simple query. Both lists enter the same pool, deduped by magnet.

**(c) The gate is `LastAiredEpisode`, and that matters.** The plan asked for `Media.Episodes > 100`, but on a RELEASING with no announced end that field is **nil** — i.e. it would be nil exactly on One Piece, the case the change exists to fix. `LastAiredEpisode` already combines the three sources (schedule, `nextAiringEpisode - 1`, and `media.episodes` only when FINISHED) and returns ~1173 there. Covered by `TestSingleEpisodeSearch_ReceivesSeriesLength`, which uses an anime in the One Piece shape (RELEASING, `Media.Episodes == nil`).

**(d) The 100 ceiling and `episode < 100`.** Each variant costs fetches, so padding only on a long series; and episode >= 100 already has 3 digits, where the variant would be identical to the simple one.

**(e) An error on one variant does not cancel the other.** `ScrapNyaa` only returns an error if NO variant answered; if the padded query's page fails, the search continues with what the other brought.

**Don't "fix" by:**
- Swapping the gate for `Media.Episodes` "which is the real total" (see (c)).
- Replacing the simple query with the padded one (see (b)).
- Always searching both forms: doubles Nyaa traffic on every short anime, where padding changes nothing (see (d)).

### 57. Adaptive pagination: sequential, with a candidate floor, no per-anime budget

**Location:** `nyaa/nyaa.go` — `fetchSearchPages`, `parsePagesWith`, `enoughCandidates`, `SetMaxSearchPages`/`ActiveMaxSearchPages`; `files/filemanager.go` — `Config.MaxSearchPages`, `applyNyaaSettings`.

**What it looks like:** one pagination helper used by the three searches (`ScrapNyaa`, `ScrapNyaaForAnime`, `ScrapNyaaForMovie` — at the time of this decision there were four, `ScrapNyaaForBatch` and `ScrapNyaaForMultipleEpisodes` later fused into `ScrapNyaaForAnime`), which descends page by page **sequentially** while it has fewer than 3 accepted candidates, with a configurable ceiling.

**Why it's right:**

**(a) Page 2 was fetched ALWAYS.** On searches that resolve on page 1 — most of them — that was a wasted fetch. That saving is what pays for the 5-page ceiling without raising average traffic: a fixed ceiling of 5 would cost a burst of fetches per pass against nyaa.si, where 429/Cloudflare is expected.

**(b) Floor of accepted candidates, not of rows.** Nyaa returns ordered by seeders desc, so past a handful of candidates the next page only brings a worse option; 3 already gives the ranking a choice. An **empty** page also ends it: it means the query is over, and insisting up to the ceiling would be a guaranteed useless fetch.

**(c) Sequential, no parallel burst.** The plan called for 2-3 concurrent "when it actually needs to descend". Left out on purpose: each page is only asked for because the previous was not enough (parallelizing would require asking for pages that might not be needed, undoing (a)), the gain is only latency on a rare path, and a parallel burst is exactly what causes 429/Cloudflare — the problem the rest of the decision avoids. With no client throttle, this is the worst place to add concurrency.

**(d) Batch and movie entered the same path.** Both used to read only page 1 with a direct `httpGet`, without going through `fetchNyaaPage` — they did not paginate and **did not appear in the log**. Now they use the same helper and get the page log with it.

**(e) There is NO per-anime per-pass fetch budget.** The plan asked for one, so an unsolvable anime would not burn the ceiling every pass. Measured: the worst case is a search that never meets the floor and goes to 5 pages, ~2.3x today's fetches, and **only** on an anime that finds nothing. The budget would require per-anime state crossing `nyaaSearcher` and nyaa's exported functions — plumbing out of proportion to the risk. If the traffic bothers, the shortest path is lowering `max_search_pages`.

**Don't "fix" by:**
- Fetching pages 1..N in parallel "to make it faster" (see (a) and (c)).
- Swapping the floor for "always fetch the ceiling" (see (a)).
- Keeping descending after an empty page (see (b)).

---

### 58. `RunAnimeDebug` mirrors `processAnimeEpisodes`: enumeration, pack resolution and single fallback

**Location:** `daemon/debug.go` — `RunAnimeDebug`; mirrors `daemon/episodes.go` — `processAnimeEpisodes`.

**What it looks like:** debug of an anime does not iterate `anime.Media.AiringSchedule.Nodes`; it builds the list with `anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`, runs the same per-anime search + `partitionSearchResults`/`pickBatches` to decide whether a pack covers the window (and, if so, re-runs `selectEpisodes` with the ceiling lifted), and falls into `searchSingleEpisode` when no pending episode received a pack/movie magnet — three duplicated stretches of the production loop. `willBatchAnime`/`resolveSearchStrategy` left: pack eligibility is the filtered search result, not AniList metadata ([#59](#59-batch-eligibility-stopped-being-metadata-and-became-a-result-filter)).

**Why it's right:**

**(a) The raw schedule describes another pipeline.** On One Piece schedule page 1 goes from 1123 to 1147; production enumerates 1..1147 and takes the first 12 by the ceiling. Iterating the raw nodes, debug reported having searched "1123 to 1147" — no clipped-schedule anime was measured correctly, and that is exactly the class the synthetic `EpisodeList` (#52) exists to cover.

**(b) The pack↔limit rule was never evaluated.** `pickBatches`/`assignBatches` lift the per-anime ceiling in `processAnimeEpisodes` when a pack covers the window; without running the same path, debug was blind to the very rule the standalone-anime feature declares as a prerequisite ("an 1100-episode anime added by hand falls on the limited one-by-one path, unless a pack covers the window").

**(c) Without the fallback, a long series was a guaranteed false positive.** The multi search does not put an episode number in the query (`q=one piece`), and the first pages ordered by seeders are only recent episodes — measured: 750 raw rows, none of episode 1-12. Who finds the old episode is `searchSingleEpisode`, which carries the zero-padding (#56). Without it the `debug-batch` report marked One Piece and Naruto Shippuden as "0 magnets" for **not having searched**, not for Nyaa not having them.

**(d) `savedEpisodes = nil` is intentional.** `firstEpisodeToConsider` only uses that argument to walk the start back below `Progress + 1`, and debug already declares — in the `ponytail:` and the `Warn` it emits — that it treats every episode as not-downloaded. With `nil` it resolves to `Progress + 1`, which on a standalone is 1.

**Accepted trade-off:** `summary.Episodes` now has one entry per enumerated episode (~1147 on One Piece, most with `would_search: false`), and a short FINISHED enters batch, lengthening the run. That is the cost of fidelity; `debug-batch`'s `report.md` aggregates.

**Don't "fix" by:**
- Going back to iterating `AiringSchedule.Nodes` "because it's simpler" — the simplicity produces a diagnosis of a pipeline the daemon does not run.
- Stopping at pack resolution "because that's the step that decides the strategy" — it is only half the search; the other half is the per-episode fallback.
- Truncating `summary.Episodes` to those with `would_search: true` — the report's "nothing searched" count depends on seeing both sides.

---

### 80. The pack size ceiling is applied TWICE: in the Nyaa search and in the daemon filter

**Location:** `nyaa/nyaa.go` — `maxBatchSizeBytes`, `SetMaxBatchTorrentSizeGB`, `batchTooBig`, applied in `ScrapNyaaForAnime`'s `parseRow`; and `daemon/search.go` — `filterBySize`, via `partitionSearchResults`.

**What it looks like:** `max_batch_torrent_size_gb` is already applied by the daemon in `filterBySize`, and yet the `nyaa` package receives the same ceiling by push (`applyNyaaSettings`) and drops the pack row before adding it to the result. Looks like a duplicated filter, and `ScrapNyaaForAnime`'s old comment said explicitly that pushing a filter into `nyaa` was what `applyNyaaSettings` avoided.

**Why it's right:** the two filters do not do the same thing, because they run at different moments of the algorithm. The daemon's decides **what to download**. nyaa's decides **how far to descend the pages** — `fetchSearchPages` stops when `accepted()` reaches `enoughCandidates` (3), and `accepted()` counts what the search accepted. A pack the daemon will discard for size, but that the search accepted, spends one of the three floor slots and ends the descent too early.

Measured on Nyaa (August 2026, `?q=one+piece&s=seeders&o=desc`, which is exactly the query `ScrapNyaaForAnime` builds):

| page | pack | size |
|---|---|---|
| 1 | (complete pack) | 587.5 GiB |
| 1 | `[Judas] One Piece 001-574` | 171.8 GiB |
| 2 | `[Erai-raws] One Piece 0892~1089` | 190.3 GiB |
| 2 | (complete pack) | 1.3 TiB |
| 2 | `[Judas] One Piece 783-1023` | 86.2 GiB |
| 3 | `[Judas] One Piece 575-782` | 61.0 GiB |
| 3 | `One Piece S01E1109-E1122` | 20.0 GiB |
| 3 | `[ToonsHub] One Piece EP1144-EP1155` | 16.6 GiB |

The first three accepted are all giants, the floor closes, and the 86 / 61 / 20 / 16 GiB packs — which **fit** the default ceiling of 100 — are never seen. The reported symptom was "One Piece never downloads because every pack found is over the ceiling"; the cause is not the ceiling being low, it is the search stopping before finding the ones that fit. Raising the ceiling to 200 would hide the bug and still queue 171 GiB in place of 61 GiB.

Pushing config into `nyaa` is not a new pattern: it is the same as `SetPriorities` and `SetMaxSearchPages` (`nyaa` cannot import `files` — cycle — so config arrives by push). The default is `0` (off), so the package used without a push behaves as before.

**Accepted trade-off:** the pack ceiling appears in two places and needs to stay coherent between them (same `Size == 0` passes free rule). The daemon's remains the authority — if the push does not happen, nothing leaks into a download.

**Don't "fix" by:**
- Removing nyaa's filter "because `filterBySize` already does that" — that is exactly what breaks pagination.
- Removing the daemon's filter "because `nyaa` already filters now" — `filterBySize` also covers `max_episode_torrent_size_gb` and is what guarantees the result when config was not pushed (tests, direct use of the package).
- Pushing `min_seeders` into `nyaa` too "for symmetry" — Nyaa already returns ordered by seeders desc, so the seeders filter does not truncate the descent the same way: the best come first by construction.
- Raising `max_batch_torrent_size_gb` as a "solution" for a long series — see above.

---

### 81. Report-code precedence is a cascade, not a set

**Location:** `daemon/report.go` — `searchIssue`.

**What it looks like:** an episode can satisfy three conditions at once — every candidate above the ceiling, every one below the seeders floor, and `len(magnets) == 0`. `searchIssue` picks **one** code with a `switch` whose order looks arbitrary, and ignores that the other conditions are also true.

**Why it's right:** the order is the business rule. When a filter empties the list, "no torrent found" is **also** true — and it is the least actionable of the three, besides being a lie: there were eight. "Every candidate was 8 GB and your ceiling is 3 GB" tells the user what to do (raise the ceiling, or adjust the custom query). "No torrent found" says nothing, and sends them looking on Nyaa for a torrent that had already been found.

Same discipline as `deriveAnimeChip`'s cascade (`lib/domain/animeState.ts`): the first condition that matches wins, and the order is the rule.

**Accepted trade-off:** an episode cut by both filters shows up only as `all_above_size_limit`. Deliberate — the user solves one at a time, and the size one is what they control directly.

**Don't "fix" by:**
- Returning a list of codes "because all three conditions hold" — the UI would start showing three lines per episode, two of them noise.
- Reordering to put `no_torrent_found` first "because it's the most common case" — common is not the criterion; actionable is.
- Feeding the cascade with `partitionSearchResults`'s `packStats` — the pack is decided for the anime, not for the episode, and the per-episode fallback is who has the last word on that episode.

---

### 82. `SetLastCheckError` clears the last-check report

**Location:** `daemon/state.go` — `SetLastCheckError`; `daemon/verification.go` — the order `SetLastCheckError(nil)` → `SetLastCheckReport(...)`.

**What it looks like:** an error setter that also wipes an unrelated field, and a `SetLastCheckReport` that **must** come after a `SetLastCheckError(nil)` that already ran three lines earlier. Looks like accidental coupling, and the "obvious" fix is to split the two.

**Why it's right:** `AnimeVerification`'s early exits (incomplete config, hardlink probe, backend not initialized, `Ensure`, id migration, AniList — there were seven until the `save_path` migration left, see #31) already call `SetLastCheckError`. With the clear inside it, **none of them needs a new line** — and none can forget. And the semantics are the right ones: a pass that aborted before looking at any anime has no per-anime report, it has `pass_error`. Without the clear, the screen would show the *previous* pass's problems side by side with a new pass error, suggesting both came from the same verification.

The second consequence is equally wanted: cancellation (`verification.go`, in the `ctx.Done()` `select`) calls `SetLastCheckError(nil)` and returns, so **an interrupted pass leaves no report** — it was incomplete, and a partial report would say "only this anime had a problem" when the others were not even looked at.

**Accepted trade-off:** the order of the two calls at the end of `AnimeVerification` is meaningful and is not obvious reading only that stretch. That is why there is a dedicated test (`TestSetLastCheckReport_AfterClearingError`) pinning exactly that order.

**Don't "fix" by:**
- Taking the clear out of `SetLastCheckError` and putting an explicit call on every early exit — seven places to forget one.
- Moving `SetLastCheckReport` before `SetLastCheckError(nil)` "because it reads better" — the report is wiped the next instant and the whole feature becomes silence.
- Persisting the report to disk to "survive restart" — `createStartFunc` calls `AnimeVerification` immediately on start (`loop.go`, before the first `time.After`), so it rebuilds in seconds; a file would cost persistence, migration and the chance of showing a report from days ago as if it were the last pass.

---

### 63. Dismissing the first-steps card lives in `localStorage`, not in `config.json`

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts`; consumed by `routes/Status.svelte` (render condition) and `routes/Config.svelte` (the "Show again" button).

**What it looks like:** a user preference the whole app honours, stored in the browser instead of in the file the daemon already reads and writes. Looks like an inconsistency — the rest of configuration is in `config.json`, and the "obvious" fix is to create a `tutorial_dismissed` field there.

**Why it's right:** it is not daemon configuration; it is a UI preference, and the daemon has nothing to do with it. A field in `config.json` would enter the schema the server validates, the `PUT /config` body and — if someone were consistent — the Settings screen's `requiredChecks`, three places for a boolean only the browser consumes. And it would apply to **every** device at once: whoever dismissed it on desktop would never see the card on the phone, where they still do not know the app. `localStorage` is per browser, which is exactly the right scope, and it is the same choice `theme` and `locale` already make in this frontend.

**Accepted trade-off:** switching browsers brings the card back, and clearing the site's storage does too. For a single-user self-hosted app, seeing a three-line card again is cheap; the inverse — not being able to see it on a new device — is the expensive one.

Hiding the card with an empty library leaves the app inoperable, and even so the dismiss is permanent: the verification pass still opens `#/config?missingConfig=true` and the banner there still appears. That backstop is what makes a permanent dismiss safe to offer.

**Don't "fix" by:**
- Creating `tutorial_dismissed` in `config.json` "to follow the user" — puts a UI preference in a daemon schema and unifies devices that should be independent.
- Swapping the "Don't show again" button for an `×` "because that's the card pattern" — on a three-item card the `×` reads as "close until reload", and the behaviour is permanent. The label is the documentation of the behaviour.
- Making the card vanish on its own after N days/visits — it goes away when the user ticks the three, when they dismiss, or when the install is already configured and running; time means nothing.

---

### 64. The first-steps card steps are ticked by hand, not derived from daemon state

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts` (`onboardingDone`); painted by `routes/Status.svelte`.

**What it looks like:** the app **knows** whether the folder is configured and whether the pass has already run — `lib/domain/onboarding.ts` computes exactly that — and even so the card's checkmarks come from a list stored in `localStorage` that the user fills by clicking. Looks like duplicated state, and the "obvious" fix is to derive the three from data the screen already has.

**Why it's right:** that is what the first version did, and on a new install steps ① and ③ were born green on their own — the library folder has a default and the verification pass runs every 10 minutes with nobody asking. The tutorial appeared two thirds done before the user had read the first line, which at once lies about what they did and takes away the reason to read the rest. Ticking is confirmed reading, not disk state.

The derivation still exists, but with another role: `allDone(onboardingSteps(...))` is the gate of "this install is already configured and running, don't teach the obvious", so whoever already uses the app never sees the card. Two uses, one source of truth each.

**Don't "fix" by:**
- Deriving the checkmarks again "to not duplicate state" — that is exactly the bug this fixes.
- Ticking the step automatically when its action is done in the app (configured the folder → tick ①) — back to the same place for whoever already had the folder configured, and makes the card move on its own while it is being read.
- Swapping the numbers for status icons — the numbering is what says there is an order; the number doubles as a tick box precisely so there are not two markers for one state.

---

### 65. AniList budget is spent by the orphan refresh, not by the daemon pass — and a 200's `errors` field is not read

**Location:** `src/internal/anilist/anilist.go` (`sendAnilistRequest`, `ttlCache`); `src/internal/api/endpoint_animes.go` (`refreshOrphanAnimes`); `src/internal/daemon/verification.go` (`searchAnilist`).

**Official docs:** [errors](https://github.com/AniList/docs/blob/master/docs/guide/graphql/errors.md), [rate-limiting](https://github.com/AniList/docs/blob/master/docs/guide/rate-limiting.md), [considerations](https://github.com/AniList/docs/blob/master/docs/guide/considerations.md). **The numbers from there are not copied here on purpose** — the documented limit is 90/min but today it is degraded to 30/min, and the docs themselves call that temporary. A copied number rots in silence and someone sizes a cache by it six months later. The docs are the source; here stays only what is ours.

**What it looks like:** the daemon is what talks to AniList, so the verification pass looks like the consumer of the limit, and `ttlCache`'s TTLs look like generic caution.

**Why it's right:** measuring per minute, per configured account:

| Source | Requests | Note |
|---|---|---|
| Daemon pass | ~0.3/min | 3 per pass (`GetCustomListsMap` + download `GetAllCurrentAnime` + delete `GetAllCurrentAnime`), every `check_interval` (default 10 min) |
| `/animes` poll | ≤1.2/min | `GetFrontendAnimeList` (TTL 60s) + `GetCustomListsMap` (TTL 5min) |
| `refreshOrphanAnimes` | **1 per account every 50 orphans** | `GetAnimeInfoByIDs` (`mediaList(mediaId_in:)`), no cache. Used to be **1 per orphan anime per account**, with a semaphore of 5 concurrent that limited parallelism and not the total. Fixed 29 Aug 2026; the standalone fallback in the same function became a `GetMediaByIDs` |
| `appendStandaloneEntries` | **1 per batch of 50 standalones** | `GetMediaByIDs` (`Page(media(id_in:))`), on both sides (`api/standalone.go` and `daemon/standalone.go`). Used to be `GetMediaByID` in a loop: measured 28 Aug 2026, with 20 standalones they were 20 of the bucket's 30 requests, spent just to build a screen. Fixed 29 Aug 2026; the cache stays **per id**, so the batch and the standalone lookup reuse each other |

The daemon pass is noise. Who blew the limit was the orphan refresh: one request per downloaded anime the filtered fetch did not cover, fired **on every frontend poll**. In a library with tens of animes outside the configured statuses, that was a burst of tens of requests — and AniList has a *burst limiter* separate from the per-minute limit, which punishes exactly that shape. **Fixed on 29 Aug 2026 by batching**, together with the standalones: the two bold rows above started costing `ceil(N/50)` instead of `N`.

**`mediaId_in` exists and is what makes the batch possible** (introspection + measurement, 29 Aug 2026). `Page.mediaList` accepts `mediaId_in: [Int]` next to `userName`, returns **at most one entry per id** (so 50 ids never go past one page — there is no pagination to follow), silently **omits** the id the account does not track, and costs 1 unit like any query. Two measured traps: a nonexistent `userName` answers **500**, and a private account answers **404 "Private User"** — neither is 400, so a malformed query is not distinguishable from a bad account by status.

That is why `mergeFailed` skips the refresh (`handleAnimes`, `endpoint_animes.go`) instead of treating "not covered" as "needs refresh": with the list failed, every downloaded anime becomes an apparent orphan, and the resulting burst feeds the failure. The TTLs of `frontendListCache` and `customListsCache` are also not generic caution — they are what makes the poll cost **independent of the number of open tabs**. Without them, two tabs and two accounts were already ~16 req/min of idle frontend.

**A 200's `errors` field is never read.** The docs are explicit: a 200 can carry an error in the envelope. `sendAnilistRequest` deserializes straight into the data struct and discards `errors`, so that case today becomes an empty response with no diagnosis. **This is not the cause of decision #11's null `customLists`** — that was verified by curl and comes back 200 *without* an `errors` field, it is a complexity budget. They are two distinct problems with the same apparent symptom, and confusing them makes you look for the fix in the wrong place.

**Documented taxonomy** (`errors[].status` when present, otherwise HTTP):

| Code | Meaning | Recoverable on its own? |
|---|---|---|
| 429 | Rate limit. 1 min timeout, with `Retry-After` and `X-RateLimit-Reset` | Yes |
| 403 | API off for instability | Only when AniList comes back |
| 403 (custom message) | IP blocked by hand | No |
| 400 | Invalid query / missing field → the schema changed, it is our bug | No |
| 404 | ID does not exist | Not a failure — becomes `ErrNotFound` |
| 5xx | AniList internal error | Yes, transient |

`errors[].validation` only exists on mutation; the app is read-only and ignores it.

**Don't "fix" by:**
- Optimizing the daemon pass "because it talks to AniList" — it costs 0.3 req/min; the gain is zero and the risk is not.
- Making `refreshOrphanAnimes` run even with the list failed, "so the screen doesn't go stale" — that is the feedback loop that produced the original 429.
- Removing the TTLs because "the daemon only fetches every 10 minutes" — who fetches every 30 seconds is each frontend tab.
- Copying AniList's current limit here as a constant — it is temporary and changes without notice.
- Estimating consumption with a counter of ours instead of reading `X-RateLimit-Remaining` from the response — see #72, which measures the bucket and shows why an error also costs quota.
- Going back to one request per id where a list fits — `Page(media(id_in: [...]))` for loose media, `mediaList(userName:, mediaId_in: [...])` for a list entry. One query costs 1 unit independent of the number of ids (#72). Both paths are already batched: `GetMediaByIDs` and `GetAnimeInfoByIDs`.

---

### 66. A banner is degraded state; a toast is an action failure — and the banner has three sources with precedence, not three banners

**Location:** policy for `components/shell/AppShell.svelte` (single banner slot) and `lib/api/client.ts` (`ApiError`, `silent` option).

**What it looks like:** each error origin wants its own notice, and the obvious way out is to stack — an AniList banner, a backend one, the WebSocket one that already exists in `routes/Downloads.svelte`.

**Why it's right:** the distinction is not severity, it is **duration**:

- **Toast** — an action the user asked for failed (save config, add anime). Already exists, already works, goes away on its own.
- **Banner** — state is degraded between polls and explains why the whole screen looks wrong. Persists while the state persists and **goes away on the first success**, never by timer.

The banner's value is not reporting the error: it is explaining why the list is incomplete or stale. Today `/animes` answers 200 with an account missing and the user concludes an anime vanished.

The sources are independent, but **stacking is wrong**: if the backend does not answer, AniList health is stale information from the last successful poll, and showing it next to "daemon does not respond" is contradictory noise. Precedence, from furthest upstream to furthest downstream (`lib/domain/systemBanner.ts`, `pickBanner`):

1. **Backend unreachable** (`fetch` got no response) — nothing below that is observable.
2. **Backend answering 5xx** — our bug.
3. **AniList unavailable** (403/5xx) — mute banner, "try later".
4. **AniList rate limited** (429) — countdown derived from `Retry-After`, the only case where the exact time is known.

**One** is shown, the highest precedence.

**The WebSocket banner of `routes/Downloads.svelte` stays where it is.** It talks about that screen — progress there is pushed over WS and goes stale without it — while `SystemBanner` talks about the whole app. Consolidating them would require moving ownership of `WebSocketClient` to `AppShell` (today instantiated in `Status` and `Downloads`), which is a refactor of another subject. Absence of the daemon is already covered by the `unreachable` state, which comes from HTTP.

**The report button only appears on our bug** — backend 5xx, and AniList 400 (schema changed, no wait resolves it). A 429 or an outage is not a bug: a report button there only generates an issue that closes with "it's AniList", and trains the user to ignore the button when it matters.

**Blind spot this closes:** `getLastCheck` and `getTorrents` pass `silent: true` so they do not toast every tick. Correct for toast — but today it means a repeated 5xx on those endpoints is **invisible forever**. Backend health is derived from *consecutive* failures per endpoint (≥2), not from an isolated failure, and that is why it coexists with `silent` without reintroducing the spam.

**`pass_error` follows the same rule.** The aborted-pass banner on the Status screen showed raw `err.Error()` — which, with AniList down, was the entire response JSON body (`{"errors":[{"message":…,"locations":[{"line":1,"column":1}]}]}`) in the user's face. `CheckReport` gained `pass_error_code`, classified in `daemon/passerror.go`, and the phrase is built in `lib/domain/passError.ts`. The raw text stays in `pass_error` and is shown **collapsed** in a `<details>`: it is the only thing pasteable into an issue, but it cannot be the first thing you read.

The cause travels **inside** the error (sentinels wrapped with `%w`), not in a new `SetLastCheckError` parameter: there are ~15 call sites, most passing `nil` to clear, and none of them needed to change. A new abort site that forgets to wrap falls into `unknown`, which has its own phrase — degrades, does not break.

**Don't "fix" by:**
- Stacking the banners "because they are different problems" — with the backend down, AniList state on screen is a dated lie.
- Making the banner vanish by timer — the state either ended (a 200 arrived) or it did not.
- Putting a report button on every banner "for symmetry" — reporting a rate limit is reporting normal behaviour.
- Swapping the outage banner for a toast "because it's an error" — a toast goes away, the outage does not; the screen stays wrong after it vanished.
- Sending a ready-made phrase from Go instead of a code, "because it's just one string" — the daemon does not know the browser's locale, and that is how the raw JSON reached the screen.
- Removing the `<details>` of the raw text "because the user doesn't understand it" — whoever opens an issue needs it, and collapsed it does not get in the way of whoever does not.

### 67. The verification pass is one at a time, and a concurrent one is dropped — not queued

`AnimeVerification` takes `verificationMu.TryLock()` and **returns immediately** if another pass is running.

The pass reads `episodes.json` **once** at the start (phase 1) and only writes at the end (phase 3, sequential). Two simultaneous passes — the loop's and `POST /check`'s, two `/check`s, or a `/check` right after an `UpdateInterval` — see the same stale list: the second sees nothing of what the first just downloaded and downloads everything **again**. That is how an anime already covered by a pack (`01 ~ 11`, episodes 6..11 recorded) gained loose torrents of episodes 10 and 11, and the second pass's write overwrote the pack records with the loose hashes — two torrents of the same episodes on disk and the screen showing the pack and the loose ones side by side.

Dropping, not queueing, is deliberate: the dropped pass would read exactly the state the in-flight pass is correcting, and the next loop tick covers what is left. Queueing would only postpone the same stale read.

**Don't "fix" by:**
- Reloading `episodes.json` in the middle of the pass "to see what the other saved" — the window stays open between the read and the write, and the two writes still fight.
- Swapping for `Lock()` (queue) — becomes a queue of redundant passes that run with stale state, each holding the next.
- Taking `TryLock` out because "`POST /check` already runs in a goroutine" — the goroutine is what creates the concurrency, not what solves it.

### 68. Pack numbering: the offset is guessed from the files themselves, and a basename collision uses the relative path

**Location:** `internal/files/librarian.go` — `packEpisodeOffset` and the `if used[destName]` guard in `Organize`; `internal/daemon/jobs.go` — `OrganizeRequest.TotalEpisodes`. Tests in `librarian_test.go` (`TestOrganizeBatch*`, `TestPackEpisodeOffset`) and `orchestration_test.go` (`TestOrganizeTorrent_BatchContinuousNumbering`).

**(a) The offset comes from the files, not from AniList.** A season >= 2 pack passes the season filter (`nyaa.go`, `IsBatch` branch) carrying the marker in the torrent name, but the files inside may be numbered continuously (13-24 for a 12-episode entry). Because each season is its own entry with numbering starting at 1 (decision #45), those files entered the library as `E13`..`E24` in a folder Jellyfin knows with 12 episodes.

The *correct* source of the offset is the PREQUEL's episode count — what `daemon.ComputeEpisodeOffset` already computes at search time — but it does not exist at organize: the job only has the hash and the `episodes.json` records. Bringing it would ask for a new `EpisodeStruct` field (with a migration) or an AniList request inside the job. `packEpisodeOffset` avoids both: it uses `AnimeTotalEpisodes`, which is **already** stored on the record, and only shifts with unambiguous evidence — **every** numbered file above the entry's total **and** at least one file per episode. That is a complete pack that starts at the season's episode 1, so `min - 1` is the offset.

Both guards matter: without "every file above the total", an extra numbered 13 in a 01-12 pack would shift the whole season; without "at least one file per episode", a partial pack (23-24 of a 12-episode season) would be treated as complete and become `E01`/`E02` instead of `E11`/`E12`. A partial pack with continuous numbering stays **as-is** — wrong, but not *more* wrong, and it is the rare case.

**(b) A basename collision started tying by relative path.** `collectVideoFiles` flattens subfolders and the destination came from `filepath.Base(rel)`. In a multi-season pack with one subfolder per season, `Season 1/Anime - 01.mkv` and `Season 2/Anime - 01.mkv` pointed at the same destination, and the second fell into the "different bytes with the same name" branch (made for redownload/replace) **removing the first's hardlink**: an episode vanished from the library and `created` wrote the same path twice. `used` already existed, but it only protected the Jellyfin name. Now it is checked for *any* destination and the tie-break is the relative path, which is unique inside the torrent.

The replacement branch stays intact for what it was made for: a collision between different `Organize` runs. What changed is only the collision **inside the same run**, which is never a legitimate replacement.

**Don't "fix" by:** applying the offset without the two guards "because a continuous pack is common"; using `max - total` in place of `min - 1` (agrees on a complete pack and is wrong on a partial pack from the start of the season); calling AniList from inside `JobOrganize` to get the PREQUEL; creating a `Season NN/` subfolder in the library to "solve numbering" (decision #45); deduplicating a basename collision by overwriting "because the file is the same anime".

### 69. `codec` now applies to episodes, and a name with no codec tag ties instead of losing

`episodeCriteria` (`nyaa/priorities.go`) filters `CriteriaOrder` to the criteria that make sense
on an episode, and `codec` was **out**: the `priorities.codecs` list showed up on the Priorities
screen and in `config.json` without affecting any episode download — only `SortMovieResults` read
it. Because the default is `["hevc","av1",...]`, the episode codec was decided in passing by
`fansub` (Judas/Ember are x265 10-bit; SubsPlease/Erai-raws are H264), and the user had no lever.
Whoever watches through a client with no HEVC decode (a browser — Chrome and Firefox do not ship
HEVC for licensing) pays that as a transcode on the server, which besides burning CPU **burns the
ASS subtitle into the video** and desyncs it. `codec` is now in `episodeCriteria`.

**Only `source` stays out.** A weekly episode is WEB-DL on practically every release; the criterion
would never break a tie and would only spend a comparison.

**The guard that keeps the change from backfiring:** `priorityIndex` returns `len(list)` for an
unknown token, i.e. **the worst**. And `ExtractCodec` returns `""` for a name with no codec tag —
which is exactly SubsPlease's pattern (`[SubsPlease] Anime - 07 (1080p) [HASH].mkv` does not say
H264 anywhere). With `codec` before `fansub` in the default order, turning the raw criterion on
would make every untagged release lose to any tagged x265: the **opposite** of what the codec list
configures. That is why `codecCompare` returns a tie when either of the two names has no tag,
leaving the next criterion to decide. It is the same rule `sizeCompare`/`resCompare` already apply
for missing data, and the same as `filterBySize` (`nyaa.go`): data that could not be read is not a
reason to punish the release.

**Canonical tokens.** `ExtractCodec` only returns `HEVC`, `AV1`, `H.264` and `XviD`
(`reCodecPatterns` maps `x265`→`HEVC` and `x264`→`H.264`). The `"x265"` and `"x264"` entries of
`priorities.codecs`'s default therefore **never match anything** — they are inert. Configuring a
preference for H264 requires writing `h.264`, not `x264`.

**Don't "fix" by:** taking the `codecCompare` guard off "because no tag is no tag"; putting
`source` in `episodeCriteria` too; moving `codec` after `fansub` in the default to work around the
unknown-token problem (hides the same bug instead of solving it).

### 70. Resolution is canonicalized in the extractor, and codec presets are a frontend reordering

**Location:** `nyaa/nyaa.go` — `ExtractResolution`, `canonicalResolutions`; `nyaa/priorities.go` —
defaults of `Resolutions`/`Codecs`/`Sources`; `frontend/src/lib/domain/priorityPresets.ts`;
`frontend/src/routes/Priorities.svelte`.

**(a) Canonicalization lives in `ExtractResolution`, not in the list order.** `ExtractCodec` already
returned a canonical token (`x265` → `HEVC`) and `ExtractResolution` returned the raw capture. That
asymmetry **was** the bug: `4k` and `2160p` are the same resolution and occupied two different
default indexes, so which one won depended only on how the group wrote the torrent name;
`1920x1080` is capturable by the regex, did not exist in the list, and ranked as the worst. Solving
that by reordering the list (putting `4k` next to `2160p`) solves nothing: they remain two items the
user can split again with the screen arrows, and the duplicate stays representable. Canonicalizing
at the extractor's output it stops existing, and the default list shortens to five readable items.

**Does not break existing config.** The old list already has `1080p`, `720p`, `480p` and `2160p` —
the canonicals. `4k`, `uhd`, `fhd`, `hd` and `8k` become unreachable but harmless, and a 4K release
from an old user starts matching the `2160p` they already have.

**`8k` stays off the list on purpose**, not by forgetfulness: the `\b(8K)\b` pattern still exists,
the token is still returned raw, and by not being in the list it ranks as the worst — the right
treatment for a resolution that does not exist in anime.

**(b) A codec preset is a frontend reordering, not data served by the backend.** Only the frontend
consumes it (the CLI does not mention priorities); an endpoint would exist to serve itself — ~60
lines in six files, a response-shape change and `swag init`, against ~30 lines in one file.

And a preset **promotes** tokens already in the user's list instead of stamping a literal array: a
typo becomes a no-op instead of inert config, a token added by hand drops instead of vanishing, and
the canonical tokens keep existing in one place (`reCodecPatterns`). A preset is a one-shot stamp,
not a stored mode: nothing new in `config.json`, no "custom" state, no precedence rule — after
apply it becomes an ordinary editable list, saved by the button that already exists.

**Only `codecs` gets presets.** `resolutions` has the right default and two arrows in the UI;
`fansubs` is taste, not a technical axis; `sources` and `audio` do not affect episodes (marked
"movies only" on the screen); `criteria_order` is a loaded gun (#55); `ignore_list` is additive, not
ranking.

**Discarded: a "prioritize quality" preset.** For airing anime — which is what
`DownloadStatuses: [CURRENT, REPEATING]` downloads — SubsPlease and Erai-raws remux the
Crunchyroll/HIDIVE stream, with no reencode: that is already the source ceiling, and any larger
file of the same episode is a reencode (generation loss) or an upscale. The first case is already
barred by `re-encode` in the default `ignore_list`. For backlog the axis exists, but it is
**source** (BD vs WEB), not compression.

**The test that closes the bug class:** `TestDefaultPriorities_TokensAreReachable`
(`tests/unit/priorities_test.go`) requires every token of the default lists to be something
`ExtractCodec`/`ExtractResolution` can return. That is how `x265`/`x264` sat for years as inert
invisible config (#69); now it breaks CI.

**Known limitation, deliberate:** no default change reaches an existing install — `LoadConfigs`
unmarshals over `getDefaultConfig()`. The lists update through the "reset" buttons that already
exist on the screen; `MaxEpisodeTorrentSizeGB`, `WatchedEpisodesToKeep` and `MinSeeders` ask for a
manual edit, once. There is no automatic migration, for the same reason as #47
(`BatchWindowSeconds`): changing behaviour in an update with nobody asking.

**(c) On the screen, taking a token off the list became a checkbox, not an `✕`.** A token absent
from the list is already treated as the worst (`priorityIndex` returns `len(list)`), so "remove"
was never remove — it was send to the end, with the side effect of the item vanishing from the
screen and no way to bring it back except typing the exact token again (and typing it wrong gives
inert config). Now each row has a checkbox: unchecked it leaves the saved array and stays visible,
greyed, at the end of the list.

The `✕` is left only for what is **not** in the default served by `GET /config/priorities/defaults`:
what the user typed and the inert legacy of an old `config.json` (`x265`, `4k`, `8k`) — which is
exactly what needs a way out. And `criteria_order` lost the add field: it is a closed set —
`sortByCriteria` silently skips a criterion that is not in `criterionCompare` and there is no
validation on `PUT /config` — so free text there only produced inert config, and with the checkbox
the eight criteria are always on the screen, ticked or not. There is nothing to add.

**Don't "fix" by:** returning the raw capture from `ExtractResolution` and "fixing" the list order;
putting `4k`/`uhd`/`fhd`/`hd`/`8k` back in the default; creating a presets endpoint; storing the
chosen preset in `config.json`; writing the new defaults over an existing `config.json`; putting
the `✕` back on canonical tokens "because the checkbox is one more click"; putting the add field
back on `criteria_order` (neither as free text, nor as a select of an enum duplicated in the
frontend); validating `criteria_order` on the backend so we can "afford" to accept free text again.

---

### 71. AniList has no franchise id: the `PREQUEL` chain is the only source, and searching by name is a trap

**Location:** `src/internal/anilist` (queries that ask for `relations`); `daemon.ComputeEpisodeOffset`
(`daemon/helpers.go`). Complements #9, which covers the offset *gate*; here is its **source**.

**What it looks like:** you can identify "the series" — the whole franchise, across cour and part —
by some AniList id, or, failing that, by a name search (`search: "Monogatari"`).

**Why it's wrong — measured by introspection on 28 Aug 2026.** The `Media` type has 55 fields and
**none** of franchise:

```
id, idMal, title, type, format, status, description, startDate, endDate, season, seasonYear,
seasonInt, episodes, duration, chapters, volumes, countryOfOrigin, isLicensed, source, hashtag,
trailer, updatedAt, coverImage, bannerImage, genres, synonyms, averageScore, meanScore,
popularity, isLocked, trending, favourites, tags, relations, characters, staff, studios,
isFavourite, isFavouriteBlocked, isAdult, nextAiringEpisode, airingSchedule, trends,
externalLinks, streamingEpisodes, rankings, mediaListEntry, reviews, recommendations, stats,
siteUrl, autoCreateForumThread, isRecommendationBlocked, isReviewBlocked, modNotes
```

There is no "series" entity in the schema, and `idMal` has **the same granularity** (one id per
cour). The only edge between cours is `relations`. Walking `relationType == "PREQUEL"` filtering
`format ∈ {TV, TV_SHORT}` is not one option among several — it is the only source available.

**A name search is the shortcut that corrupts everything.** It would be one request, and it was
measured:

- `search: "Monogatari"` → 28 TV results, almost all other works (`Ore Monogatari!!`,
  `Shouwa Monogatari`, `Perrine Monogatari`, `Gokinjo Monogatari`).
- `search: "Shingeki no Kyojin"` → 7 results, including `Shingeki! Kyojin Chuugakkou` — the
  12-episode chibi parody, which would enter any episode sum and shift the whole series.
- And it does not return **order**: the sequence would have to be inferred from the titles, which
  is exactly the string-parsing problem that already takes packs down in `nyaa_match.go`.

The `relations` graph is authoritative; the name is not.

**Three `relations` facts that define any implementation:**

1. **`Media.relations` accepts no arguments.** No server-side filter by `relationType`, no
   pagination: every query brings *all* the edges, and filtering is always the client's job.
2. **`relations` can be nested, but AniList cuts at 2 levels.** Tested with depths 1 to 8: the body
   saturates at 5865 bytes from depth 3, and at the third level `relations` comes present with
   `edges: []`. Confirmed on Monogatari, Mushoku Tensei and Shingeki: **2 PREQUEL hops per query,
   always.**
3. **`edges: []` at the cut level does NOT mean root.** This is the serious trap: at the level
   where AniList stops expanding, the response is indistinguishable from an anime that genuinely
   has no prequel. Treating the two as equal stops the walk early and produces a
   **silently wrong** offset. Whoever implements this needs to mark which nodes arrived in an
   authoritative position (query levels 0 and 1) and re-fetch the cut-level ones before concluding
   anything about them. The ceiling of 2 is observed behaviour, not documented — the loop must
   consume what comes and enqueue what is missing, without assuming any depth.

**The chain was validated against the real API** (28 Aug 2026), and the strong validation is
Shingeki: the accumulated sum gives `76..87` for Final Season Part 2, which is **exactly** the
numbering SubsPlease and Erai-raws use on the files. The chain reproduces the real naming
convention with no adjustment. Monogatari (7 levels, crooked chronological order) also closes
coherently.

**Erring to 0 is cheap; erring to a number is not.** A missing offset falls back to relative
numbering, which a good part of the groups use. A wrong offset sends the search to an episode that
does not exist. Every doubt in the walk resolves to 0.

**Don't "fix" by:** looking for a franchise id in the schema "that must exist somewhere"; using
`idMal` as series identity; replacing the walk with `search:` and the franchise name; treating
`edges: []` as root; assuming depth 2 as a contract; including `MOVIE`/`OVA`/`ONA` in the walk
(see #9).

---

### 72. AniList budget is measured from the response headers, not from a counter of ours

**Location:** `src/internal/anilist/anilist.go` — `sendAnilistRequest`; `health.go` — `Priority`,
`ErrBudgetLow`, `recordBudget`, `budgetAllows`, `retryAfter`, `HealthRateLimited` classification.
Pinned by `budget_test.go`. Complements #65, which measures **who** spends; here is **how to know
how much is left**.

**What it looks like:** to not take a 429, the natural path is a client-side limiter
(`golang.org/x/time/rate`) or a queue that holds a request when estimated consumption passes the
ceiling.

**Why it's wrong.** AniList returns the bucket state on **every** response, and the header is
strictly more precise than any internal counter. Measured on 28 Aug 2026:

```
minimal media           HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 29
normal 200               HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 28
404 (media does not exist) HTTP 404  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 27
invalid query            HTTP 400  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 26
heavy nested query       HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 25
```

Four consequences, all with design effect:

1. **An error consumes quota.** A 404 and a malformed query cost 1 each. Blind retry on top of an
   error burns budget, and a counter that only counted successes would be optimistic exactly when
   things are already going badly.
2. **Complexity costs nothing.** The 33 KB nested query decremented 1, same as the minimal query.
   There is no charge for complexity or for number of ids: **maximizing work per request is
   strictly correct, there is no trade-off.** That is why `Page(media(id_in: [...]))` (up to 50
   ids, `perPage` max 50) is worth it whenever there is a fetch-by-id loop. Applied in
   `GetMediaByIDs` (standalones, `media(id_in:)`) and `GetAnimeInfoByIDs` (orphan refresh,
   `mediaList(mediaId_in:)`), both on 29 Aug 2026.
3. **The header is authoritative because it sees the other consumers.** It already sums the daemon
   pass, the frontend poll and the standalone searches — the three compete for the same bucket per
   IP, with no coordination. No internal counter comes close, because none of them sees the
   others' errors.
4. **The bucket resets at once, it does not slide.** Measured: `Remaining` went 24 → 20 in a
   burst, sat at 19 at 21s and came back to **29** at 41s. It is a fixed bucket with a discrete
   reset, not a sliding window with gradual refill — in practice, **the budget recovers whole in
   ≤ 60s**.

**`X-RateLimit-Reset` cannot be used.** It shows up advertised in
`Access-Control-Expose-Headers`, but it is **not present** on any observed 200 response. #65's
table cites it because the official docs cite it; measurement did not find it. A 429 was not
provoked on purpose (it blocks the IP for ~1 min and the user's daemon shares that IP), so
**which headers a 429 carries remains unverified** — what is known is that `health.go` already
reads `Retry-After`.

**The design that follows: reserve by priority, not a queue.** Record `X-RateLimit-Remaining` and
the instant of the read in an atomic next to `health`, on every response **including error**, and
compare against a floor before emitting. Blocking and waiting is the wrong answer — the frontend
poll runs inside an HTTP handler and would hang the screen. The right answer is to degrade what is
cheap to degrade:

| Caller | Criticality | Action with budget low |
|---|---|---|
| Daemon pass | high — losing the pass is an episode not downloaded | always passes |
| Frontend poll (`GetFrontendAnimeList` **and** `GetCustomListsMap`) | low — they already have cache (60s and 5min) | refuse and serve stale cache — **both**, or the degrade is asymmetric |
| Standalone search from the UI | low | refuse, with an error visible to whoever asked |
| Warm-up / prefetch work | low — has a fallback | postpone to the next pass |

**The mandatory escape hatch:** if everyone is refused, nobody emits a request and the process
never discovers that the bucket reset. The read needs a validity — 60s after the last observation,
treat the budget as full again. That is what the reset measurement is for, and it is what stops
the gate from deadlocking itself.

**How it ended up implemented** (29 Aug 2026). `recordBudget` stores `X-RateLimit-Remaining` and
the instant in an `atomic.Pointer`, right after `httpDo` — before any status branch, so the 404
and the 400 count. `budgetAllows` is the gate, at the top of `sendAnilistRequest`: critical always
passes; disposable passes if the read is expired (60s) or if `remaining >= budgetFloor` (10 of
30). The refusal is `ErrBudgetLow` immediately — it never waits.

`Priority` is a **required parameter** of `sendAnilistRequest` and rises to the four mixed-caller
public functions (`GetCustomListsMap`, `GetAnimeInfo`, `GetMediaByID`, `GetMediaByIDs`), which the
daemon and the frontend call with opposite criticalities. The others pin theirs in the body. There
is no implicit default, and that is the feature: a new query does not compile without someone
deciding. The discarded alternative was a wrapper with `sendAnilistRequest` becoming "normal
priority" — smaller diff, but it creates exactly #69's inert-config class.

**The dividing line is not "daemon vs. frontend", it is "repeats on its own vs. someone is
waiting".** Disposable is recurring traffic with a fallback: the `/animes` poll
(`GetFrontendAnimeList`, `GetCustomListsMap`, `GetAnimeInfoByIDs`, and the `GetMediaByIDs` of
`appendStandaloneEntries` and `refreshStandaloneOrphans`) and the per-keystroke search
(`SearchMedia`). Critical is the daemon pass **and** every click action — opening the detail
screen, adding a standalone, downloading an episode by hand: they are rare requests and expensive
to refuse, and sacrificing them returns no budget. A refused `GetFrontendAnimeList` serves the
**expired** cache (`ttlCache.getStale`) instead of dropping the poll; the other disposables
already treat a partial failure as missing data.

**The poll pair degrades together or not at all.** `fetchAniListEntries` is `GetCustomListsMap` +
`GetFrontendAnimeList`, and refusing only one of the two is worse than refusing both: the list
comes from the expired cache and answers **successfully**, `customLists` comes back `nil`, and the
merge concludes that no anime is on an excluded list. `GET /animes` shows a blacklisted one as
normal and `standalone_guard` lets you add as standalone what it should refuse — all in a
response that looks healthy. That is why `GetCustomListsMap` also serves `getStale` when the gate
refuses (`TestCustomListsServesStaleCacheWhenRefused`). What remains is the window where there is
no expired read at all (process just up with the bucket already empty); then `nil` comes back and
#11's overlay guard keeps what the complex query brought. Nulling the whole list in that case
would be worse: `GetCustomListsMap`'s `nil` also happens on a network failure, and it would take
the screen down because of the cheap query even with the face answering.

No warm-up caller exists yet. `GetSeriesIndex` (#77) already receives the `Priority`, but nobody
calls it in production: the warm-up comes in together with ownership by coverage, which is who
builds the seed.

**Don't "fix" by:** adding `x/time/rate` or any client-side token bucket (guesses instead of
measuring, and does not account for the other consumers' errors); counting only 200 responses;
depending on `X-RateLimit-Reset`; blocking the goroutine waiting for the bucket to fill inside an
HTTP handler; refusing *every* caller when the budget is low, without the 60s validity; giving
`Priority` a default (wrapper, a "safe" zero value, or deriving it from the query —
`GetAllCurrentAnime` and `GetFrontendAnimeList` can be the same query with opposite
criticalities); using `getStale` outside the gate's fallback.

---

### 73. The frontend doesn't query AniList directly, even though it could

**Location:** policy for `src/internal/frontend` and `src/internal/api/endpoint_animes.go` — today
every AniList access goes through the backend.

**What it looks like:** the limit is per public IP, and the frontend is the largest consumer (see
#65). If it talked to AniList directly, running on a PC different from the daemon the consumption
would fall on another bucket.

**The technical prerequisites exist** — measured on 28 Aug 2026, that is not what blocks it:

- AniList answers `Access-Control-Allow-Origin: *` and exposes the rate-limit headers in
  `Access-Control-Expose-Headers`. A browser can query directly, including reading the budget.
- The app **uses no token**: every query is public, by username. There is no credential to leak
  to the browser.

**The hole is NAT.** The limit is per **public** IP, not per machine. Daemon on the desktop and
browser on the laptop, in the same house, leave through the same IP → same bucket, zero gain. The
relief only exists when the browser is on a genuinely different network (phone on mobile data,
remote access, VPN) — the least common scenario — and the cost is paid in every scenario.

**The cost is silent duplication.** The frontend would have to replicate in TypeScript
`DedupeByMedia`, the `customLists` merge, `withStandaloneProgress` and `health.go`'s health
classification. Go logic duplicated in TS diverges with nobody noticing, and the daemon would
still need the same data for the pass: two paths for the same data, not one.

| | gain | cost | works when? |
|---|---|---|---|
| Frontend fetches directly | moves the poll to another IP | logic duplicated in TS | only if the browser is on another network |
| Batched `id_in` on standalones | N req/min → 1 | ~10 lines | always |

**Verdict: don't.** The idea points at a real problem, but it attacks the wrong slice with the
expensive instrument. **Re-evaluate only if**, after batched `id_in` and the priority gate (#72),
the rate limit tightens again.

**Don't "fix" by:** moving only "one small query" to the frontend as a middle ground — the cost is
duplicating the merge logic, and it starts on the first query.

---

### 74. The pack-exclusion guard's unit is the torrent's contents, not the list of records

**Location:** `internal/daemon/episodes.go` (`canRemoveTorrent`, `hasUnclaimedContent`,
`declaredSpan`, `forgettableKeys`, `handleSavedEpisodes`). Pinned by
`TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent`,
`TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack`,
`TestRemoveEpisodesAndLinks_RemovesPackDownloadedMidWatch`,
`TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`,
`TestRemoveEpisodesAndLinks_RemovesPackOnceEveryCourClaimsIt`,
`TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard` and
`TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent` (`internal/daemon/episodes_test.go`).

**What it looks like:** the guard that decides whether a shared torrent can leave no longer asks
only "is every record of this hash in the delete set?". It also compares the range declared by the
records (`BatchStart`/`BatchEnd`) with the episodes **claimed** by the group's entries, and
refuses the removal when unclaimed content remains (`hasUnclaimedContent`). Three pieces make the
count close:

- **Who claims is the entry, by its `AnimeTotalEpisodes`** — not the number of records.
  A record is only born for an episode the daemon downloaded: a pack `01-11` taken with progress 5
  has 6 records, and a blocked episode shortens the group the same way. The episodes the entry did
  not record are still hers. An old record does not carry the total (`0`); then the record count
  of that `anime_id` is what is left, which is the behaviour before this fix.
- **`declaredSpan` measures per `anime_id`, not on the union** — each entry stores the range in
  *its* local numbering (#79), so `1..23` under cour 1 and `-10..12` under cour 2 are the same
  23-episode pack; unioning the two would give 34, a pack that does not exist.
- **The pack record that stayed on disk is not deleted** (`forgettableKeys`). It is the only place
  the declared range exists — without it no future cour can adopt the pack (`findCoveringPack`
  requires the range, #78) and the claimed-content count never closes again.

`removeEpisodesAndLinks` gained a `force` parameter for **manual** torrent deletion
(`RemoveTorrentWithEpisodes`, #32): there the unit of deletion is the torrent and who decided was
the user — without that the click would return success without deleting anything. The automatic
pass never uses `force`. On top of that, `handleSavedEpisodes` passes `savedEpisodes ∪ newEpisodes`
to `removeEpisodesAndLinks`, not only the pre-pass snapshot.

**Why it's right:** "every record" is not "all the content". A **season** pack downloaded under
cour 1 (which only has 11 episodes on AniList) writes 11 records but carries 23 episode files —
cour 2's 12 have no record because they have not been downloaded yet. Watching cour 1 put the 11
records in the delete set, the guard concluded "nobody survives" and wiped from disk content the
user never saw. The aggravation of the same pass is the snapshot half: when cour 2 becomes
`CURRENT` and the pass picks the same pack for the new media id, `Session.Add` reuses the infohash
and the new records exist only in `newEpisodes` — invisible to the guard, which deleted the
just-adopted file.

**Why not the record count** (it was the first version of this decision, and it leaked disk): it
confuses "episode of another cour, that nobody downloaded" with "episode of this entry that the
daemon did not need to record". A pack `01-11` of an 11-episode entry, taken with progress 5, has 6
records; `6 < 11` marked unclaimed content **forever**, the torrent never left disk and the records
were deleted anyway — a permanent orphan pack, which is the opposite of the protection the guard
exists to give. Comparing episode **numbers** also does not work: the same hash can have records of
different media ids, each on its own ruler. The entry's total is the only datum that says how much
of the pack belongs to it without needing a conversion.

**Known ceiling (`ponytail:` in the code):** a pack whose range neither the name nor Nyaa's file
list resolved stores `BatchEnd == 0` (unknown, see `hasDeclaredRange` in #79) and stays
undetectable — the guard falls back to the old behaviour. After #84 that is the remainder of a rare
case (detail page down, unreadable file name); before it was **every** pack with no range in the
name.

**Don't "fix" by:** deriving the range from the min/max of the saved episodes (that is the
information that is missing, not one you can reconstruct — it was the bug that motivated
`BatchStart`/`BatchEnd` existing), nor reloading `episodes.json` inside `removeEpisodesAndLinks`
(Phase 3 is sequential on purpose and already has both pieces in memory).

---

### 75. Truncation at the marker is the first attempt of the title match, not the only one

**Location:** `internal/nyaa/nyaa_match.go` (`titleMatchesQuery`, `altTitleCandidates`,
`tokensMatchQuery`, `firstMarkerIndex`). Pinned by `TestTitleMatchesQuery_AltTitleAfterMarker` and
`TestTitleMatchesQuery_AltTitleDoesNotAcceptWrongAnime` (`src/tests/unit/nyaa_test.go`), with real
names collected from Nyaa.

**What it looks like:** `titleMatchesQuery` runs the two checks (every query token present +
Jaccard) first on the name truncated at the marker — exactly like #18 — and, only if that fails,
repeats over **each** alternative title that appears AFTER the marker, one at a time. Each stretch
between parentheses and each slash-separated segment is its own candidate.

**Why it's right:** #18's premise ("everything after the marker is noise") breaks on a whole family
of names. EMBER writes
`[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [...] (Mushoku Tensei II:
Isekai Ittara Honki Dasu Part 2) (Batch)`: the `Season 2` marker is INSIDE a group, and the full
romaji comes three groups later. Truncating left `[mushoku tensei jobless reincarnation]`, and the
romaji query was rejected for a missing token — the anime's ONLY Part 2 packs died before reaching
the batch path. Diddy, NTRX, Fuchs and VARYG do the same with a slash instead of parentheses.

**Why one candidate per stretch, not a single bag of tokens.** That was the first attempt, and
measuring made it worse: joining the English title from the prefix with the romaji from the footer
dilutes both (Jaccard 0.78 on the EMBER case, against **1.00** of the romaji compared alone), and
in `debug-batch` that let a stretch lend a token to its neighbour — Yameii's loose releases of
*SAO Alternative: Gun Gale Online* started matching the query of *Sword Art Online II*. Compared
stretch by stretch, each title is evaluated as a title.

**Why the second attempt only runs after the first fails.** That is what preserves all of #18.
Truncation is what protects a short query from sinking in Jaccard: the pack
`[EMBER] Kimetsu no Yaiba (2024) (Season 4) [...] (Kimetsu no Yaiba: Hashira Geiko-hen | Demon
Slayer: Kimetsu no Yaiba Hashira Training Arc) (Batch)` matches the 3-token query on the truncation
(1.00) and never reaches the candidates — where it would score 0.5 and be rejected. Expanding the
tokens for everyone would swap one false negative for another.

**Measured in `make debug-batch`** (corpus of `scripts/robustness-animes.txt`, before and after): no
match lost; gains on `108465`, where EMBER's season 1 BDRip pack started matching and became the
best option (492 seeders, against 65 of the Judas that was leading). On `166873`, which motivated
the investigation, EMBER's three Part 2 packs started surviving the filter (they were zero).

**Known limitation, PRE-EXISTING and not introduced here:** the query `Sword Art Online II` matches
the loose releases of *SAO Alternative: Gun Gale Online II* (Jaccard 0.57, above the 0.4 of a long
query). Erai-raws releases of that same spinoff already matched before this change, through the
truncation path — what changed was only consistency across releases. Fixing that is touching
`jaccardThreshold`, which is another feature.

**Don't "fix" by:** tokenizing the whole name (brings episode title and release tag, and sinks
Jaccard), nor joining the candidates into one set (measured above), nor creating a threshold of its
own for the second attempt (magic number fitted to five samples).

---

### 76. `searchNyaaWithVariants` stops at the first variant that returns any result — measured, and it stays that way

**Location:** `internal/daemon/search.go` (`searchNyaaWithVariants`, `buildTitleVariants`).

**What it looks like:** the title-variant loop only continues to the next when the current one
returned **nothing** (`result != nil` already ends it). A variant that returns 24 loose episodes and
zero packs ends the search — the English variant, which might bring the pack, does not run.

**Why it's right:** the failure described above is real on paper, and it even got scheduled as a
feature ("F3 · Title variants on the pack path"). It was **measured** after #75 and does not show
up in practice: running `ScrapNyaaForAnime` + `partitionSearchResults` + `pickBatches` for **each**
variant of the 8 animes in `scripts/robustness-animes.txt` plus `146065` and `166873` (29 Aug 2026),
there is not a single anime where variant 1 returns a result and a later variant finds a pack it
did not. Where a pack exists on Nyaa, romaji already finds it:

| id | v1 (romaji) packs/batches | best later variant |
|---|---|---|
| 1735 Naruto Shippuuden | 1 / 1 | same (different pack, same coverage) |
| 1 Cowboy Bebop | 9 / 1 | unique variant |
| 108465 Mushoku Tensei | 3 / 1 | same |
| 20594 Sword Art Online II | 4 / 1 | unique variant |
| 101922 Kimetsu no Yaiba | 3 / 2 | 14 / 1 (more packs, worse coverage) |
| 166873 Mushoku Tensei II Part 2 | 3 / 1 | same |
| 235 Meitantei Conan | 0 / 0 | **0 / 0** on all four |

`235` is the only anime in the corpus that falls on the loose-episode path, and no variant finds a
pack for it — Nyaa has no pack of Detective Conan's first episodes. It is absence of content, not
of a variant.

After #75, the cost of running the next variant stopped having a counterpart: it would be one more
Nyaa search round **per anime, on every pass** where there is no pack — which is most airing
animes, because a current-season pack only appears at the end. Combined with adaptive pagination
(#57), it multiplies.

**Reopen when:** `make debug-batch` shows an anime whose variant 1 returns a result, zero packs
survive the filter, and a manual search with the English variant shows a pack that covers the
window. Then the change is two lines: continue the loop when `partitionSearchResults` returns no
pack.

**Don't "fix" by:** always running every variant and unioning the results (pays N searches per
anime for the measured gain above, which is zero), nor inverting the variant order
(`GenerateSearchTitleVariants` puts romaji first because that is what matches most releases).

---

### 77. The per-series absolute axis is a BFS two generations at a time, and the cut level goes back to the queue

**Location:** `src/internal/anilist/series.go` — `Series`, `GetSeriesIndex`, `walkSeries`,
`recordLink`, `prequelOf`, `resolveSeries`, `seriesCache`. Pinned by `series_test.go`.
Implements what #71 measured; #9 is the offset *gate* of a single part.

**What it looks like:** to know a cour's absolute episode it is enough to read the anime's
`relations` and sum the `PREQUEL`'s episodes — which is what `daemon.ComputeEpisodeOffset` does.

**Why that is not enough.** `ComputeEpisodeOffset` walks **one** hop and only when the title
declares `Part >= 2`. That covers `Shingeki no Kyojin: The Final Season Part 2` → `+16`, but not
the series: that cour's real absolute is `76..87`, which requires summing the **five** previous
entries. Because an AniList query expands at most 2 levels of `relations` (#71), resolving a
7-level chain is not a read — it is a walk.

**The design, in three rules:**

1. **Each request walks two generations, not one.** A media's response brings its `relations`
   (level 0) and its prequel's (level 1), both authoritative. From that come **two** recorded
   links and **three** episode counts per media fetched. `Page(media(id_in: [...]))` with up to 50
   ids costs the same budget unit as a single id (#72), so width is free and the only dimension
   that costs is height: `ceil(height/2)` rounds.
2. **The grandfather goes back to the queue.** The level-2 node comes with a reliable id, format
   and count, but its `relations` arrives as `edges: []` — the cut, indistinguishable from "has no
   prequel" (#71). Recording it as root produces a **silently wrong** offset: on the Shingeki
   fixture, Final Season Part 2 would give `38..49` instead of `76..87`. That is why `recordLink`
   is only called on levels 0 and 1, and level 2 is re-enqueued. Only a node recorded by
   `recordLink` enters the cache — a level-2 node knows how many episodes it has, but not who
   comes before it, and a half-fact is not a fact.
3. **The loop is depth-agnostic.** The ceiling of 2 is observed behaviour, not a contract: it
   consumes what comes, enqueues what is missing. An AniList that started expanding 3 levels would
   only make the walk finish in fewer rounds, without changing a line.

**The cache is 24h because the data is immutable and the walk is monotonic.** Only ancestors
enter, and an ancestor of a finished anime also finished: a `FINISHED` with an episode count does
not gain an episode or a new prequel. A `RELEASING` node or one with no count (`episodes: null`,
unreleased) counts as 0 and is **not** persisted — freezing that zero for 24h would shift the
whole series down on the day the count appeared. In memory, no file: the whole warm-up costs a
few requests, so persisting to disk would be invalidation and migration for nothing.

**Absent ≠ root, in both directions.** An id AniList does not return stays **out** of the map
(same contract as `GetMediaByIDs`), and an unknown ancestor in the middle of the chain stops the
sum there. Both choices err toward the **smaller** offset on purpose: a missing offset falls back
to relative numbering, which a good part of the groups use; a wrong offset sends the search to an
episode that does not exist (#71).

**The measured chains became a test fixture, not markdown** — Mushoku Tensei (4 levels),
Shingeki no Kyojin (6) and Monogatari (7). The strong validation is Shingeki: `76..87` is exactly
the numbering SubsPlease and Erai-raws use on the files. The test seeds **only the ends** of each
chain: seeding every id would make each ancestor arrive as level 0 and the cut would never be
exercised — verified by mutation, the previous version of the test passed with rule 2's bug.

**Don't "fix" by:** treating `edges: []` as root; assuming depth 2 as a contract; fetching one
ancestor per request (height becomes the cost, and AniList's budget is 30/min); caching a level-2,
`RELEASING` or no-count node; persisting the offset to disk or on the episode record (see the
discarded alternative in #78 — a persisted value rots when AniList corrects a count, and the
derived one self-corrects on the next TTL); replacing the walk with `search:` and the franchise
name (#71).

### 78. A torrent's unit of ownership is coverage on the absolute axis, not the `(anime_id, episode)` key

**Location:** `src/internal/daemon/coverage.go` — `resolveSeriesIndex`, `adoptCoveredEpisodes`,
`findCoveringPack`, `dropAdopted`; `packDisplayName` (`episodes.go`), shared with
`assignBatches`; called in `processAnimeEpisodes` (`episodes.go`) and in
`DownloadStandaloneAnime` (`standalone.go`). Pinned by `coverage_test.go`. Consumes #77; it is
the other side of #74.

**The problem.** AniList splits cour and part into **separate media ids**, each numbering from 1;
Nyaa packs are almost always of the **whole season**, crossing that boundary. With ownership
expressed as `files.EpisodeKey{AnimeID, Episode}`, the `1..23` pack downloaded under cour 1 (11
episodes) is invisible to cour 2 — which has no record, does not know the file is already on disk,
and re-downloads the whole season from Nyaa. Real case: Mushoku Tensei S1.

**The rule:** before any search, a pending episode is adopted when there is a pack record, **on
any `anime_id`**, whose declared range contains the episode **on the series' absolute axis**.
`abs = Series.Offset(anime_id) + local number` (#77) — comparing `1..23` with `1..12` without
converting is comparing different rulers.

**Adopting is writing the record pointing at the hash that already exists**, exactly what would
happen if the search found the same torrent again (`Session.Add` reuses the infohash). That is
what makes the rest of the machine work with no new exception: #74 starts seeing the siblings of
the other cour, and `organizeTorrent` hardlinks the files that never had an owner — it already
treats a partially organized group without re-notifying. The range is **copied as the owner
declares it** (it is the torrent name's) and **converted to the adopter's local numbering** —
`findCoveringPack` returns the owner's offset with the record exactly for that. Every record
stores the range on its own entry's ruler (#79) and that is how it is read back; copying `1..23`
from cour 1 under cour 2 (offset 11) would make that record be read as the absolute `12..34`, and
a cour 3 would find coverage for an episode the pack does not have — marked downloaded, never
searched, file nonexistent. Pinned by `TestCoverageOwnership_AdoptedRangeDoesNotShiftForTheNextCour`.

**The index seed includes the `anime_id`s from `episodes.json`, not only the pass's.** The
previous cour is `COMPLETED` and therefore left the pass's universe — and it is **its** offset
that converts the pack's range. Without that seed the feature would not see the pack that
motivated it. It costs almost nothing: width is free (`perPage` 50, one query = one unit — #72)
and a good part of the extra seed would already enter the walk for being an ancestor of the
pass's animes.

**Four gates, all mandatory** — each covers a different way of adopting wrong:

| Gate | Without it |
|---|---|
| Declared range (`BatchStart > 0`) | a pack with no range in the name stores `0`, and unknown would become "covers everything" — same ceiling as #74 |
| Torrent still in the session | a record with a dead hash: episode with an owner and no file, invisible to the re-descent |
| Same `Series.Key` | "absolute 12" would match any other anime's pack |
| Not a movie | `prequelOf` only follows `TV`/`TV_SHORT`, so a post-season movie inherits its offset and its "episode 1" falls inside the pack's range. Coverage by range does not distinguish the two |

**The filter runs at BOTH selection points.** `selectEpisodes` is pure and does not know about
adoption, so the second pass (limit lifted, pack path) would bring the adopted ones back and the
daemon would download what it just adopted.

**Discarded alternative: persist `AbsStart`/`AbsEnd` on `EpisodeStruct` at download time.** The
lookup would be local and offline, but every record from before the feature would be born with `0`
— indistinguishable from "unknown" — i.e. it **would not solve the case that motivated it**, it
would only hold for what was downloaded after; and the backfill would need the derivation anyway.
Worse: a persisted value rots. If AniList corrects an episode count or adds a missing relation,
the stored number stays wrong forever; the derived one self-corrects on the next TTL. Storing as
source of truth a derivable number creates a second truth nobody invalidates. **Reopen only if**
the lookup shows up as a **measured** cost — and then it becomes a derived cache, never a source.

**Don't "fix" by:** comparing `BatchStart`/`BatchEnd` of different media ids without converting;
converting the stored range to the adopter's local numbering (breaks `declaredSpan`); adopting
with an unknown range, with the torrent out of the session or without checking `Series.Key`;
moving adoption inside `selectEpisodes` (it runs twice — would duplicate the records); notifying
`NewEpisode`/`DownloadCompleted` on an adoption (nothing was downloaded, and the completion
webhook already went out when the pack landed).

### 79. Pack choice asks "does it cover the window?", not "is it part N?" — and pack numbering is a guess among three hypotheses

**Location:** `src/internal/daemon/episodes.go` — `packAxis`, `newPackAxis`, `packAxis.localRange`,
`coveringBatch`, `pickBatches`, `assignBatches`, `hasDeclaredRange`, `declaredSpan`;
`src/internal/nyaa/nyaa.go` — `ExtractPart`, `declaredParts` and the `IsBatch` branch of
`ScrapNyaaForAnime`. Pinned by `packaxis_test.go` and the tables of `nyaa_test.go`. Consumes #77
(absolute axis) and is the other side of #78 (ownership by coverage).

**The problem.** AniList splits the season into cours/parts, each entry numbering from 1; groups
release the pack of the **whole season** and each one picks whichever ruler it wants (`sources.md`,
"Pack granularity and numbering": restarts per season, restarts per part, continuous across the
season, absolute across the series). Two consequences, both measured on Mushoku Tensei:

1. **The hard part filter zeroed the packs of every "Part N" entry.** With `requestedPart != nil`,
   a pack **without** a part marker was discarded — and a whole-season pack normally has no such
   marker, even though it contains the requested episodes.
2. **The name's range was read on the wrong ruler.** `01 ~ 23` under a 12-episode entry was
   compared with `1..12` as if it were the same numbering.

**The rule:** on the pack path, `part` only rejects a **declared conflict** (the pack says "Part 1"
and we want 2). With no marker, the pack passes and who decides is **range coverage**. The episode
path keeps the hard filter: there the marker is the only way to tell one part's "- 05" from the
other's "- 05".

**Known ceiling — a pack with no part marker AND no range in the name is decided by nobody.**
`coveringBatch` returns that pack without consulting the axis (there is no number to convert) and
it counts as complete, so "who decides is coverage" holds for a pack **with** a range, not for
this one. A `[Group] Anime S4 [Batch]` that in practice is only Part 1 passes the same as the
whole-season pack — before this decision it was rejected in the scraper, and rejecting it was
worse: it zeroed the packs of every "Part N" entry, which is problem 1 above. Accepting is the
measured choice; the missing tie-break was **Nyaa's detail-page file list** (at the time, an item
in `docs/TODO.md`), which says how many files the pack has and with what numbering. **RESOLVED in
almost every case (see #84):** that read exists (`nyaa.PackFileRange`, consumed on demand by
`packSet.rangeOf`), and the range it returns enters through the same `packAxis` as the name's
range. What remains is the residue where neither the name nor the files resolve (detail page down,
unreadable file name): then the range stores zero and the pack is still used as complete — this
entry's ambiguity, reduced to the rare case.

**`packAxis` is three hypotheses and a tie-break.** Because the name's convention cannot be read,
the daemon tests how much to add to the local number to reach the group's ruler: **0** (relative
to the entry), **`ComputeEpisodeOffset`** (continuous across the season — the immediate prequel's
total) and **`anilist.Series.Offset`** (absolute across the series, #77). Only a hypothesis that
actually **covers the requested episode** competes; among those that cover, the winner is the one
that makes the pack **end on the entry's last episode**. That tie-break is `sources.md`'s "file
count" expressed by the name's span: a 25-episode pack under a 12-episode entry cannot be local, a
12 under the same entry cannot be anything else. In doubt (none hits the end), delta 0 holds: a
missing offset falls back to relative numbering, which a good part of the groups use, while a
wrong offset picks a pack that does not have the episode.

**The STORED range is the converted one, and it can start at zero or below.** A season pack
downloaded under cour 2 starts before that entry's episode 1: `-12..12` is the correct
representation, and it is what makes `owner.Offset + BatchStart` (#78) return the right absolute
range and `declaredSpan` (#74) count the pack's 25 content episodes. That is why the "unknown
range" sentinel became the **end** (`hasDeclaredRange`: `BatchEnd > 0`) and no longer
`BatchStart <= 0` — which already lied about `[Erai-raws] … - 00 ~ 12`. Only the **displayed name**
cuts the start at 1: "-10-12" says nothing on screen, and what matters there is this entry's
slice.

**`ExtractPart` returns `nil` for a name with two parts.** "(Part 1 + Part 2)", "(Part 1+2)" and
"(Season 4 Part 03+04)" cover both halves; returning the first number made the pack be rejected
exactly for the second. With no unique number, who decides is coverage.

**The two "first TV/TV_SHORT PREQUEL" selections stay duplicated on purpose.**
`anilist.prequelOf` walks the **whole** chain (hypothesis "absolute across the series");
`daemon.ComputeEpisodeOffset` takes **one** hop and is gated by `part >= 2` (hypothesis
"continuous across the season"). They are different hypotheses of the same guess — unifying them
would erase the distinction the heuristic uses. The `TODO(F8)`s that pointed one at the other
became this note.

**Measurement (29 Aug 2026).** `make debug-batch` over `scripts/robustness-animes.txt`: episodes
searched and found identical to the previous run on the 8 animes (only seeders varied, live data).
On the motivating cases, all with 12/12 magnets: `166873` and `127720` (Mushoku Tensei) and
`131681` (AoT Final Season Part 2), where the pack `[EMBER] … (Season 4 Part 03+04)` started
surviving the filter.

**Don't "fix" by:** going back to requiring a part marker on the pack path; relaxing the part
filter on the **episode** path; treating a range start ≤ 0 as unknown; converting the stored range
"to make it positive" (breaks #74 and #78); adding a numbering hypothesis without a tie-break that
distinguishes it from the others; unifying `prequelOf` with `ComputeEpisodeOffset`.

---

### 83. The pack name's range is the first plausible match, not the first match

**Location:** `nyaa/nyaa.go` — `batchRange`, called by `ExtractBatchInfo`; `nyaa/nyaa_regex.go` — `reBatchRange`.

**What it looks like:** a `FindAll` with `continue` on three guards where a `FindString` with an `if` would fit, and a band of magic numbers (1900..2100) in the middle. Looks like speculative defence against cases that do not happen.

**Why it's right:** `reBatchRange` is `(\d{1,4})\s*[-~]\s*(\d{1,4})` — it matches **any** pair of numbers joined by `-` or `~`, and in a release name most of those pairs are not an episode. The three measured:

| Name | Match | What it is |
|---|---|---|
| `[Group] Anime Complete Batch [720-1080p]` | `720-1080` | resolution |
| `[Erai-raws] Hibike! Euphonium 2 - 01 ~ 13 …` | `2 - 01` | season + first episode |
| `[EMBER] Mushoku Tensei … (2021-2022) (Season 1 …)` | `2021-2022` | release year |

The year is what forced the change: it does **not** fall into the safe fallback. `EndEpisode == 0` means "unknown range" and every caller treats it as a complete pack; `2021..2022` is a *declared* range that no `packAxis` hypothesis covers, so `coveringBatch` rejects the pack and it vanishes from the choice without a log line that explains. It was a complete BDRip pack, first placed by seeders, discarded by a parenthesis.

The band `start >= 1900 && last <= 2100` only catches a year: no series reaches episode 1900, and `One Piece 1001-1100` — the real case that forced `\d{1,4}` in [#51](#51-an-episode-number-accepts-4-digits-except-inside-brackets) — passes intact.

**`FindAll` and not `FindString`** because a rejected match must not end the search: in `[Group] Anime (2021-2022) 01-24 [BD]` the real range comes **after** the year.

**Accepted trade-off:** `FindAll` is **non-overlapping**, so Hibike still has no range. The match `2 - 01` consumes the `01`, and the following `01 ~ 13` is never seen. It stays as-is on purpose: scanning with overlap (restarting one character after each rejection) produces junk sub-matches — a rejected `2021-2022` would become `021-2022`, which passes the band and stores the range 21..2022. A pack with no range falls on the safe path; a pack with a wrong range lies to ownership by coverage ([#78](#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)).

**Don't "fix" by:**
- Swapping for a token sanitizer that strips `[...]`/`(...)` before matching: the year comes in parentheses, but so does `(001-100)` — that is where half the groups put the real range.
- Narrowing the year band to "only 19xx/20xx with a difference <= 5": the difference between the years distinguishes nothing the band does not already distinguish, and it becomes one more magic number.
- Scanning with overlap to save Hibike (see the trade-off).

---

### 84. A pack without a range in its name gets its coverage from the file list, not from assuming it covers everything

**Location:** `nyaa/nyaa.go` — `PackFileRange`, `TorrentResult.DetailURL`, `detailURL`, `fileListEntryName`; `daemon/episodes.go` — `packCandidate`, `packSet`, `maxPackDetailFetches`, `pickBatches`, `assignBatches`; `daemon/search.go` — `nyaaSearcher.packRange`.

**What it looks like:** pack choice makes one extra HTTP request, in the middle of a function that used to be pure, and a pack's range started being resolved lazily by a stateful type (`packSet`) instead of coming from a call to `ExtractBatchInfo`.

**Why it's right:** the pack name almost never carries the range. Measured on 29 Aug 2026 on the first 8 rows of `?q=<anime>+batch&s=seeders` for three animes: **19 of 24 packs declare no range at all**. That was not the rare case — it was the common case, and the path it followed stored wrong data in `episodes.json`:

`EndEpisode == 0` counted as "complete pack", `pickBatches` stopped the cursor there, `assignBatches` wrote `1..total` and **every** episode of the window became an `EpisodeStruct` pointing at the pack's hash. The ones the pack does not have stay recorded as downloaded forever: the Librarian links only the files that exist and nothing reconciles record with disk afterwards, so they never return to the search.

The case that forced the change, measured at `nyaa.si/view/1891610`: `[EMBER] … (Season 2 | Part 2) … (Batch)` declares no range and brings the files **13..24**. Under the Part 1 entry the daemon recorded 1..12 — intersection **zero** with what the torrent contains.

The detail-page file list answers before downloading, and it answers well: on the 7 packs measured, 574/574, 24/24, 23/23, 12/12 and 103/111 files gave the number (the 8 that missed are OP/ED, which should stay out). It comes from the `.torrent` itself, so it works on old content — unlike AnimeTosho's `files[]` (see `sources.md`).

**The files' range enters through the same `packAxis` as the name's range**, because groups use the same four numbering conventions in both places (`sources.md`, "Pack granularity and numbering"). On the EMBER case: 13..24, delta 12 of the "continuous across the season" hypothesis, becomes 1..12 local.

**Why on demand and with a budget.** The detail page costs ~30 KB (measured on 7 packs) against ~125 KB of a listing — it is not bandwidth, it is number of requests against nyaa.si. `packSet.rangeOf` only fetches when the range is asked for a pack with no range in the name, and memos including failure:

- resolving everything at once would spend 3 requests per anime per pass even when the first pack already declares the range and covers the window — the most common case;
- without a memory of failure, `assignBatches` (which walks the packs once **per episode**) would become one request per episode.

`maxPackDetailFetches = 3` because packs come ordered by quality and `pickBatches` rarely goes past the first ones to close the window.

**An unresolved range stores ZERO, not `1..total`.** Reaching `assignBatches` with no range means both sources failed (detail page down, single-file torrent, no readable name). The pack is still used — refusing it would make a down Nyaa zero everyone's packs — but `1..total` was a guess that `hasDeclaredRange` read as a **declared** range, and from there ownership by coverage ([#78](#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)) adopted, under another cour of the series, an episode the pack might not have. "I don't know" is the only honest record.

**Accepted trade-off:** with a zero range [#78](#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)'s `hasUnclaimedContent` has no span to compare and the deletion guard falls back to the old behaviour on that group. That is the right trade: losing a protection against premature deletion in a rare case is worth less than spreading an invented coverage to the series' other cours.

**Known ceiling, and MEASURED on 30 Aug 2026: it stays as-is.** The range is the `min..max` of the numbers read, so a pack whose files **restart** numbering per season (S01-S04, one folder each) gets the largest season's range instead of the total. The proposed tie-break was the per-folder file count, which [#79](#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses) already wanted. The measurement (`nyaa/live_pack_measure_test.go`, 61 real packs in 12 searches) says **not** to do it:

- **Frequency:** 3 of 48 packs (6%) in the neutral sample `?q=<anime>+batch&s=seeders`, 2 of 13 in the sample that aims at multi-season on purpose. The 5 cases: Vinland Saga (24/24), Jujutsu Kaisen (24/23), Steins;Gate Trix (25/24/4), EMBER SnK (25/12/12/10/16), Haikyuu Complete (25/25/10).
- **No download is lost.** Running the 5 through `packAxis.localRange` under each season's entry, the pack is chosen in all 8 combinations — delta 0 already hits, because a multi-season pack does contain the episodes of every season it brings, in each one's local numbering.
- **The tie-break would change none of the 8 results**, and would count an extras folder as an episode: `Creditless Opening 1` → 1, `OAD - 01` → 1, `Movie 01` → 1 (measured; on 61 packs those folders never stretched `min..max` past the main content, but they would enter a per-folder count directly).
- **The residue is another, and smaller:** the STORED range exceeds the entry's total in 4 of the 8 combinations — Haikyuu S3 stores 1..25 for a 10-episode entry, SnK S2 stores 1..25 for 12, SnK S3 +3, JJK S2 +1. That is over-claim for [#78](#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)'s ownership by coverage, and the fix for it is clamping the end at `totalEpisodes`, not reading folders.

**Gap measured together, this one with no fix:** `ExtractEpisodeNumber` returns `nil` for a file name with a loose number followed by a title — `My Hero Academia 39 Game Start [1080p].mkv`. They were the only 3 packs in the sample where neither the name nor the files resolved the range, all three of the same MHA release.

**Don't "fix" by:**
- Fetching the detail inside the searches' `parseRow`: it becomes one request **per listing row**. That is why `DetailURL` is only captured there and the read happens after the filter and the sort.
- Resolving every pack's range at once "to simplify `packSet`": spends the whole budget on packs that would never be chosen.
- Refusing a pack whose range was not resolved: a down Nyaa would download no pack at all, swapping a wrong record for no download.
- Going back to storing `1..total` "because the pack was chosen as complete": that was exactly the guess that fed ownership-by-coverage adoption with an invented range.
- Filtering files by extension in a list of the `files` package's own: `nyaa.IsVideoFile` is the same list the Librarian uses after download, and `files` imports `nyaa` (the other way would cycle).

---

### 85. The frontend's AniList cost is per ACCOUNT and per ORPHAN, not per tab

**Location:** `api/endpoint_animes.go` — `refreshOrphanAnimes`, `refreshStandaloneOrphans`;
`anilist/anilist.go` — `GetAnimeInfoByIDs`, `GetCustomListsMap`, `frontendListCache`,
`customListsEmptyTTL`. Complements [#72](#72-anilist-budget-is-measured-from-the-response-headers-not-from-a-counter-of-ours)
(how to know how much is left) and [#73](#73-the-frontend-doesnt-query-anilist-directly-even-though-it-could)
(why not move the fetch to the browser).

**What it looks like:** each open tab polls `GET /animes` every 30s, so N tabs cost N times more
budget — that was the reading that motivated the idea of the frontend talking to AniList directly
(#73), and it is what the `ttlCache` comment described before the cache existed.

**Measured on 30 Aug 2026** (isolated daemon, real account, `NYAA_URL` dead so nothing downloads,
three 5-minute phases with the `/animes` poll at 30s reproducing `Status.svelte`):

| phase | tabs | orphans | AniList req per `/animes` call | req/min |
|---|---|---|---|---|
| A | 1 | 0 | 1.50 | 2.4 |
| B | 2 | 0 | **0.94** | 3.0 |
| D | 2 | 100 | **3.00** | 7.8 |

Three consequences:

1. **A tab does not multiply — the TTL absorbs.** Doubling the tabs CHEAPENED the cost per call
   (1.50 → 0.94): `frontendListTTL` (60s) and `customListsTTL` hold, and the second tab almost
   always hits a cache the first just filled. The ceiling of the list+customLists pair is the TTL,
   not the number of tabs.
2. **An orphan multiplies, and per tab.** `refreshOrphanAnimes` → `GetAnimeInfoByIDs` **has no
   cache at all**: it fires `ceil(orphans/50) × accounts` requests on EVERY `/animes` call. With
   100 orphans the cost per call tripled (0.94 → 3.00) and started scaling with tabs. It is the
   only `/animes` path outside the TTL, and it is what decides consumption of an install with a
   large library.
3. **Empty `customLists` is the largest consumer in normal regime.** An account with no custom
   list at all falls into the 30s `customListsEmptyTTL` instead of the 5 min `customListsTTL` — 2
   of the ~3 req/min of phases A and B, two thirds of the frontend cost when there is no orphan.

**The gate never needed to enter:** 0 refusals by `ErrBudgetLow` and 0 429 responses in 22
minutes, with a peak of 11 req/min in a bucket of 30. The only two failures were AniList's own
30s timeout — latency, not budget.

**The count that matters:** `(1 + ceil(orphans/50) × accounts) × tabs × 2/min`, plus ~2/min per
account of empty `customLists`. 2 tabs + 100 orphans + 1 account = 7.8/min (measured). 4 tabs +
200 orphans + 2 accounts ≈ 50/min, and then the gate starts refusing for real.

**Don't "fix" by:** raising `frontendListTTL` thinking a tab is the problem — the measurement says
it is not. If the budget tightens again, the target is putting a TTL on `GetAnimeInfoByIDs`, the
only path with no cache, and then looking at `customListsEmptyTTL`.

### 86. The Test button answers the service's status, not "the preset exists"

**Location:** `notifications/notifications.go` — `fireWebhook`, `FireTestWebhook`,
`ErrWebhookNotFound`; `api/endpoint_notifications.go` — `handleNotificationWebhookTest`.

**What it looks like:** `fireWebhook` returning `error` is useless, because the only path that
really fires — `fireBatch`, in a goroutine — ignores the return and already logs everything that
goes wrong. A `return` would be enough.

**Why it exists:** `POST /notifications/webhooks/{name}/test` answered `200 "Webhook fired
successfully"` for any preset that existed, and the front showed a success toast — with Jellyfin
returning `401` for a wrong token, or with the URL down. The only record of the failure was a
`warn` line in `daemon.log`, which nobody will read to know whether a Test button worked. Testing
delivery is the only reason that endpoint exists; without the return it only confirmed that the
name was spelled right.

Hence the split: `ErrWebhookNotFound` (sentinel, `errors.Is`) → `404`; any other error →
`502 WEBHOOK_FAILED`, with the service's status in the message. Automatic firing keeps ignoring
the return on purpose — there is nobody to report to, and the log already covers it.

**Don't "fix" by:** putting `fireWebhook` back to a `func(...)` with no return because "only the
test uses it"; or mapping a delivery failure to `404`, which would make the front say "webhook
not found" for a refused token.

### 87. Webhook headers become a list of pairs while the form is open

**Location:** `frontend/src/routes/Notifications.svelte` — `headerRows`, `toRows`, `fromRows`,
`confirmWebhook`. The on-disk format did not change: `WebhookPreset.headers` stays
`Record<string,string>` in the API and in `config.json`.

**What it looks like:** a useless conversion. The `Record` is already in hand, you can iterate
with `Object.entries` and write straight into `newWebhook.headers` — that is what the code did.

**Why it exists:** a `Record` is not editable keystroke by keystroke. Renaming a key is deleting
the entry and creating another, so each letter typed in the *Header* field rebuilds the object,
the row changes position in the iteration and the cursor jumps. That is why the old version only
had a pair of "pending" fields and a `+` button: without a list of pairs, already-saved headers
could not be inputs.

The `+` was the real trap — whoever typed in both fields and clicked **Confirm** lost the header
in silence, and the webhook went to disk with the `CHANGE_API_KEY` placeholder (that is how a
Jellyfin kept taking `401` with nobody understanding). With pairs, every header is an input, `+`
only adds an empty row and Confirm writes what is on the screen.

`fromRows` drops a row with an empty key: `+` creates a blank row, and giving up on filling it
must not become a `""` header. Covered by `tests/component/Notifications.headers.test.ts`.

**Don't "fix" by:** swapping `headerRows` for a direct bind on the `Record` "to drop the
conversion"; nor changing the saved format to a list of pairs, which would break everyone's
`config.json` and Go's `files.WebhookPreset` for no gain.

### 88. The progress bar derives "downloaded" from `episodes_pending`, not from `episodes_downloaded`

**Location:** `frontend/src/lib/utils/status.ts` — `breakdown()`, consumed by the three
`TripleProgressBar`s of `routes/Status.svelte`. The backend that produces the number is in
`api/endpoint_animes.go` — `countPendingEpisodes`.

**What it looks like:** a detour. `AnimeInfo` has `episodes_downloaded` right there, the bar wants
the "downloaded" segment, and `released - downloaded` is the obvious subtraction. That is what the
code did.

**Why it exists:** `episodes_downloaded` is not "how many I have already downloaded", it is **how
many files are on disk right now** — it comes from counting `LoadSavedEpisodes()`. The daemon
prunes a watched episode (`daemon/episodes.go`, `buildWatchedKeepSet` / `watched_episodes_to_keep`,
default 2), so that counter **drops** as the user watches. The subtraction then throws an episode
already downloaded AND seen into the yellow segment, which is supposed to mean "still to
download".

Measured symptom: an anime with `5 watched · 5 downloaded · 10 released of 13` painted 5/13 yellow
with the **Up to date** chip next to it, on the same row. Eps 6-10 were on disk and 1-5 had been
pruned; nothing was missing.

`countPendingEpisodes` walks **by episode number** (`for n := watched+1; n <= released`), not
`released - downloaded`, exactly to survive the prune — its comment says so. `episodes_pending` is
the only immune counter, and the bar's `downloaded` is `released - pending` (floored at
`watched`).

The "behind" chip had already taken this same fix earlier, in `lib/domain/animeState.ts` — the
comment there records that the naive subtraction "marked half the list as behind". The bar lagged
and repeated the bug; they are the two consumers of the same number.

**The legend left with it.** It was a `legend: string` prop built in `Status.svelte` with the
CUMULATIVES ("5 watched · 5 downloaded · 10 released of 13") — impossible to match with the
colours, because the purple there had zero width while the text said "5 downloaded". Today it
lives inside `TripleProgressBar`, in four terms that are the DELTAS, each coloured with its
segment's colour: `X watched` (--ok) · `Y to watch` (--accent) · `Z to download` (--warn) ·
`W unreleased` (track). The same four numbers feed width and text, so they cannot diverge again.
A ready-made string has nowhere to hang the `<span>` — that is why the prop died instead of
becoming `legend: string[]`.

**Don't "fix" by:** passing `anime.episodes_downloaded` straight to `TripleProgressBar`'s
`downloaded` prop "because the name matches" — the names match, the semantics do not. Nor putting
the legend back on a string prop "so the component doesn't import i18n". Covered by
`tests/unit/status.utils.test.ts` (`describe('breakdown')`) and
`tests/component/TripleProgressBar.test.ts` (word ↔ colour ↔ width).

**A zeroed term vanishes per anime, stays on Library.** That is what the `keepZeros` prop decides,
and the default (`false`) is the common case. Per anime, zero is noise — most rows have at least
one, and `LIST_GRID`'s 290px was measured for a THREE-term legend, so four in full wrap onto two
lines and the row height oscillates. On the *Library* card it is the opposite: there the four
numbers are the card's content, and a term vanishing would make the sum stop closing with the
total.

When all four terms are zero (anime with no released episode) `0 watched` remains, on purpose: an
empty `<p>` would shrink the row and bring back exactly the height oscillation the omission was
supposed to avoid.

**Watch the separator:** it is `{' · '}`, an expression and not literal text. Svelte trims
whitespace at the start/end of an element's children, and `<span> · </span>` renders `·` glued to
the words. Covered — the tests compare the whole `textContent` of the `<p>`, with spaces.

### 89. There is no OpenAPI spec: the `architecture.md` table is the only endpoint doc

**Location:** `api/server.go` — `SetupRoutes` (there is no `/swagger/` route); `docs/agents/architecture.md`
— "API" section. Negative fact: **there is no** `docs/swagger/`, nor a `// @` annotation on a
handler, nor `swag` in `go.mod`.

**What it looks like:** forgetfulness. Every daemon with REST has a `/swagger/`, the handlers are
ready to take `@Summary`/`@Param`, and putting it back is a `swag init`.

**Why it left:** measured — `swaggo/http-swagger` + `swaggo/swag` cost **11.2 MB in a 34 MB binary
(33%)**, which goes in every AUR and GitHub release on three platforms. In exchange: 343 lines of
annotation in 17 files, 6,482 generated lines, ~15 indirect `go-openapi` deps and a mandatory
regeneration step on every commit that touched the API.

What decided was not the size, it was **who read it**: nothing consumed the spec. There is no
client generator in `package.json`, in the `Makefile` or in the scripts — `lib/api/client.ts` is
written by hand. The only reader was the `/swagger/` console in the browser, which
`curl localhost:8091/api/v1/...` replaces.

`@Param`/`@Success` only repeated type and HTTP code and died with no substitute. But **11
`@Description`s carried an invariant that was nowhere else** — `bytes_completed` null on a paused
torrent because rain frees the piece data; `resume` puts it at the END of the queue and is not
"start now"; `prioritize` demotes the least-progressed and rain keeps the bitfield; batch
prioritize ignores an unknown hash on purpose; the deletion boundary is the torrent, not the
episode. Those became a **normal doc comment** on top of the handler, which is better than the
annotation: it shows up in `go doc` and in the editor hover, and does not depend on regeneration
to be right.

**Don't "fix" by:**
- Re-adding `swag` "because the new endpoint deserves docs": the new endpoint's docs go in the
  `architecture.md` table (step 7 of the checklist in `conventions.md`) and, when there is a
  behaviour invariant, in a doc comment on top of the handler.
- Generating the spec without serving the route "because it's free": it is not — it brings back
  the regeneration step on every API commit and a second copy of the contract to go stale. If one
  day it needs importing into a Bruno/Insomnia, generating on demand and not versioning is cheaper
  than keeping it.

### 90. There is no `config.json` versioning: the migration checks SHAPE, in one place

**Location:** `files/filemanager.go` — `LoadConfigs` (the two legacy-field migrations, ~22
lines). Negative fact: **there is no** `config_version` field, migration chain, or lock that
blocks the app with an old config. And the migration lives in **one** place —
`api/endpoint_config.go` deliberately does not repeat it on `PUT`.

**What it looks like:** a lack of robustness. The project is distributed (AUR + releases) and an
old-version `config.json` loading wrong is a silent failure. The natural way out looks like
numbering the format, writing a v1→v2→v3 chain and locking the app with a big warning when the
migration does not go through.

**Why it does not exist:** because the format already solves two of the three cases on its own.
`LoadConfigs` unmarshals the JSON **over `getDefaultConfig()`** (`filemanager.go`), so:

| Change on `Config` | Needs a migration? |
|---|---|
| **New** field | No — the default holds, an old config loads right |
| **Removed** field | No — JSON ignores an unknown key |
| **Renamed** field | **Yes** — the only case |

A version chain would exist to serve only the third row, which happened **twice** in the life of
the project (`anilist_username` → `anilist_usernames` and `excluded_list` → `excluded_lists`, both
in April 2026).

And the version number would be **less** reliable than the check that is there. `if
config.ExcludedList != "" && len(config.ExcludedLists) == 0` is **derived from the data**; a
`config_version` is **metadata asserting something about the data**, and `config.json` is a file
the user opens in an editor. They edit a field and the version sits still; they restore a backup
and the version comes from another moment than the shape. Then the chain runs the wrong
migration — with confidence, which is what the shape check does not do.

The lock has the opposite problem: **no `config.json` that exists today has a version field**.
Either "no version" is treated as v1, and then the lock never protects the real case, or it fires
on everyone at debut. And for a rename, migrating is never *impossible* — if you can instruct the
user on screen, you can do it.

**The precedent that closes the subject:** `save_path` (#31, amendment of 24 Aug 2026) was exactly
this question with a **worse** consequence — without migrating, rain finds the derived path empty
and re-downloads the entire library. The decision was to remove the migration and document a
manual step. And what locked there was not a missing version field: the operation preserves the
inode, carries `torrents.RootMarkerName` with it and there is no way to know whether the user
already moved the folder by hand. No chain makes that safe.

Which is the general ruler: a config migration is either **pure shape** (rename — 3 lines, no
version needed) or **a change with an effect outside the config** (moving data on disk — a version
does not make it safe). The framework helps in neither.

**When to revisit:** if config migration becomes frequent (3+ in a year) or if one appears whose
old→new mapping is not mechanical. Today it is 2 in ~2 years, both mechanical.

**Don't "fix" by:**
- Adding `config_version` + a chain: see above; shape already answers, and the number can lie.
- Locking the app when the config looks old: every config that exists today fires that lock.
- Repeating the migration on `PUT /config` "for the old client": that is what `endpoint_config.go`
  used to do, and the comment was wrong — no CLI version sent the singular key (the old switch's
  `case "anilist_username"` set `AnilistUsernames`, the plural). A body that still sends the
  singular key is persisted and cured on the next `LoadConfigs`; the copy in the handler only
  anticipated the cure by one cycle, at the cost of two migrations to diverge.
- Taking the ~22 lines out of `LoadConfigs` "because it's been months": an empty account list is
  **not** an error state (#49 — a standalones-only install never configures an account), so a
  pre-April config would load with no accounts, without complaining, and the daemon would simply
  download nothing from a list.

### 91. There is no component library: daisyUI left because it was a second color vocabulary

**Location:** `frontend/tailwind.config.js` (`plugins: []`), `frontend/src/app.css` (base of `html`
and `.tooltip`), `frontend/src/components/ui/` (the primitives), `frontend/src/lib/design/tokens.css`
(the single source of truth for colour). Negative fact: **there is no** `daisyui` in
`package.json`, nor a `daisyui:` block in the config, nor a
`.btn`/`.card`/`.modal-*`/`.alert`/`.checkbox`/`.select` class on any screen.

**What it looks like:** impoverishment. A ready-made component library left and in its place came
hand-written Tailwind classes, which is normally the wrong way up the ladder.

**Why it left:** it was not being used as a component library. The real use was **45 colour
utilities** (`text-base-content` ×27, `bg-base-100/200`, `border-base-300`,
`bg-success/warning/error`) against **12 component classes**. And the 45 duplicated what the
project already had: `tokens.css` defines `--bg-card`, `--text-primary`, `--ok`, `--warn`,
`--danger` and does the theme swap through **its own** `[data-theme]` — daisyUI's theme block was
a manual hex copy, not a source.

Three pieces of evidence that the duplication was real and charged a toll:

1. **There were two tests just to watch it.** In `tests/unit/designTokens.consistency.test.ts`,
   they compared each colour of `tokens.css` with the hex copy in `tailwind.config.js`, because
   daisyUI v4 compiles theme keys to OKLCH at build time and cannot consume `var(--x)`. The file
   **still exists** with the invariant that remains (the two theme blocks need to exist and be
   filled); only the two comparison tests were deleted.
2. **The project's tokens were renamed to not collide.** `heading`/`body` are only called that
   because `primary`/`secondary` were reserved by daisyUI's brand palette.
3. **The comment that justified `base-300`'s value described a use that no longer existed** —
   20 lines propped on a `grep bg-base-300` that yielded 5 results; on the day of the removal it
   yielded zero (the 5 uses were `border-base-300`).

What replaced each thing, inventing nothing new: `.modal-*` → the `ui/Modal.svelte` that already
existed (and whose own comment said it was born for that, with migrating `ConfirmDialog` left for
later); `.checkbox` → `ui/Checkbox.svelte`; `.btn` → `ui/Button.svelte`; `.alert`/`.toast` → the
same `bg-menu` + `shadow-elevation` surface as `ActionMenu`, with the status colour only on the
border and the icon; `.card`/`.card-body` → `flex flex-col rounded-card`. Only `.tooltip` had no
equivalent and became ~40 lines of CSS in `app.css`, **keeping the class and `data-tip` contract**
so as not to touch the 14 use sites.

**Gains that were not the goal:** `ConfirmDialog` gained trapped focus, Esc and focus return (the
`<dialog>` + `.modal-open` did not have it), and two `svelte-check` a11y warnings left with it.

**Verification:** before/after screenshot of the 4 affected screens in the 2 themes. The difference
sat between **0.0002% and 0.002% of the pixels**, all of it explained: card border (`base-300` →
`border-default`, which is what the other 41 borders of the app already used), the ad-hoc
`text-base-content/40|50|60` ramp collapsed into `text-subtle` (the app has 3 named levels, not 4
opacities), and ~6px of shift on the checkbox rows.

**Don't "fix" by:**
- Re-adding daisyUI (or swapping for another component lib) to "not write CSS": the hand-written
  CSS is ~40 lines of tooltip; the rest is reuse of `components/ui/`.
- Reintroducing `bg-base-*`/`text-base-content`: those classes no longer exist, and the vocabulary
  is `bg-card`/`bg-sunken`/`text-heading`/`text-body`/`text-subtle`.
- Taking `background-color`/`color` off `html` in `app.css`: `AppShell` paints its wrapper, but
  nothing covers the overscroll area or the instant before the shell mounts. That came from
  daisyUI's `base: true`.
- Swapping `app.css`'s `.tooltip` for `title=""`: the native one has a ~1s delay and does not
  style, and they are 14 sites on a hot screen.


### 92. The on-disk AniList cache restores with TTL zero, and a value served from it comes with `ErrFromCache`

**Location:** `anilist/persist.go` (the whole snapshot), `anilist/anilist.go`
(`ttlCache.snapshot`/`restore`, the failure paths of `GetAllCurrentAnime`,
`GetFrontendAnimeList`, `getMediaListEntry`, `GetCustomListsMap`), `anilist/standalone.go`
(`GetMediaByID`/`GetMediaByIDs`), `anilist/health.go` (`ErrFromCache`, `Health.CacheSavedAt`),
`files/filemanager.go` (`LoadAnilistCache`/`SaveAnilistCache`, `anilist_cache` file).

Negative fact: **there is no** cache mode, "I am offline" flag, repository layer or second read
front. `ttlCache` remains the only one, and disk only makes it survive restart.

**What looks wrong, part one:** a persisted cache that `get` never sees. Entries restored from the
snapshot get an expiry in the past (`restore(..., 0)`), so every happy-path read misses the cache
and goes to the network as before — the on-disk file looks useless.

It is deliberate, and it is the invariant that separates "survives a crash" from "started showing
stale data". Restoring with a live TTL would make every screen serve yesterday's list without even
trying AniList, and would save requests nobody asked to save. The only path that reads the snapshot
is the **failure** one (`getStale`), so no screen got older than it was. The exception is
`seriesCache`, restored with the full TTL: only a `FINISHED` anime link with an episode count
enters it, immutable by construction (see `recordLink`) — there is nothing to expire.

**What looks wrong, part two:** functions that return a value **and** an error at the same time,
and call sites that do `if err != nil && !errors.Is(err, anilist.ErrFromCache)`.

`ErrFromCache` is not "it went wrong", it is "it went right with data from another time". It goes
as an error because that makes the old behaviour the **safe default**: every caller that does not
know the sentinel sees `err != nil` and behaves exactly as before the cache existed. Only whoever
checks `errors.Is` opts into using the value, and the choice stays visible at the call site instead
of hidden in a flag someone forgets to read. A boolean on the return would invert that — whoever
ignored the boolean would start treating days-old data as fresh without writing a line.

Two call sites deserve attention:

- `api/endpoint_animes.go` **refuses** the orphan refresh when the list came from cache. Coverage
  is known (the list came back whole), but each uncovered anime would become a `GetAnimeInfoByIDs`
  doomed to fail, on every 30s poll per tab — #65's burst, now without even a chance of succeeding.
- `daemon/verification.go` **accepts** the cache list for status deletion. It is an explicit
  decision of the project owner, and the risk has a name: an anime that left a deletion status
  *after* the snapshot (`COMPLETED` that became `REPEATING`, and the pass downloaded the season)
  shows up as deletable again, and the pass deletes what it just downloaded. Before the cache that
  was impossible — an account with no answer did not enter `inDeleteStatus` and
  `allAccountsAgreeOnDelete` refused on its own. If it ever bites, the fix is one line: treat
  `ErrFromCache` as an account with no answer there.

**Why there are three `MediaList` caches, not one** (`frontendListCache`, `passListCache`,
`mediaEntryCache`): the three queries ask for **different** field sets. The frontend one brings
`coverImage` and does not bring `relations` or the entry's `status`; the pass one brings
`relations` and `status` and does not bring `coverImage`; the per-media-id one (`mediaByIDFields`)
brings everything. Serving the detail screen from the frontend cache would leave a manual download
without `relations` — and widening the 30s-poll query to cover that is exactly what #57 and #65
tightened.

**`stampAiringAt`/`rebaseAiring` exist because `timeUntilAiring` is relative to the fetch
instant.** Every "already aired" gate reads that field (`daemon/episodes.go`,
`anilist/episodes.go`, `daemon/manual_download.go`), so served raw from cache it freezes and the
pass concludes forever that nothing new premiered. The write converts to an absolute `airingAt`
(only where the API did not send it) and the read recomputes the countdown. That is what makes the
episode that was ten minutes away when AniList went down enter the search on its own, **with no
new rule** — the pass already searches when the countdown goes negative.

**Relation to #72:** the budget gate's refusal still serves the expired value **without** an
error. The data there is seconds old and the next poll comes back fresh; the screen gains nothing
from knowing. Only a real failure carries `ErrFromCache`.


### 93. The disk guard has two triggers, and the size one still applies with `min_free_disk_percent = 0`

**Location:** `daemon/helpers.go` (`checkDiskSpace`), called in `daemon/episodes.go`
(`attemptDownloadWithRetries`, once before the loop and once per candidate) and in
`daemon/manual_download.go` (`addAndPrioritize`).

`checkDiskSpace` receives `torrentSizeBytes` and refuses for two different reasons:

1. **the torrent does not fit** in the free space — runs whenever the size is known, including with
   `min_free_disk_percent = 0`;
2. **the volume would fall below the required percentage** after the download — only runs with
   `min_free_disk_percent > 0`.

**What looks wrong:** a config documented as "`0` = off" that does not turn the whole guard off.
Deliberate. The percentage is the user's policy ("I want 10% of slack"), and turning policy off is
their right. "This torrent is bigger than the disk" is not policy, it is arithmetic: the download
will fill the volume and stall in the middle either way, and letting it in fulfills no wish.

**Amended by [#101](#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space):** the size is compared with the free space MINUS what the torrents already added still have to write, not with the free space alone.

**Why the size did not come earlier:** it always existed on the Nyaa result
(`nyaa.TorrentResult.Size`, the same datum as the `max_batch_torrent_size_gb`/
`max_episode_torrent_size_gb` ceilings), but it was lost on the way — `resolvedMagnets` stored
`magnets []string`. Today it stores `candidates []nyaa.TorrentResult` exactly so `Size` reaches
`Add`. If someone one day "simplifies" the field back to a list of strings, the guard goes blind
again.

**Size 0 = unknown, and it passes.** That is the pasted-magnet case: the size would only appear
after the metadata, and refusing for not knowing would bar a legitimate download. Then the guard
is the percentage it always was — see `ManualDownloadEpisodeWithMagnet`/`ManualDownloadAnimeWithMagnet`,
which pass `0` on purpose.

**The check is per candidate, not one before the loop.** Each candidate has a size, and the second
in the list may fit where the first did not — skipping one does not end the attempt. The
size-less check **before** the loop still exists, and it is something else: a disk already full
today means no candidate will do, and trying N times would only fill the log.


### 94. A stalled torrent is dropped through the failure path, and only one that is *trying* counts as stalled

**Location:** `daemon/stalled.go` (`dropStalledTorrents`, `stalling`, `stalledTorrentTimeout`),
called in `daemon/verification.go` right after `backend.List()`.

Negative fact: **there is no** retry policy of its own, blacklist, attempt counter or config for
the threshold. A torrent at zero peers for more than an hour enters `HandleTorrentFailure`, the
same function that handles a torrent rain stopped with an error — `download_failed` webhook,
leaves the session keeping the bytes, and the next pass searches again
([#24](#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist)).

**What looks wrong, part one:** dropping instead of pausing. Pausing looks gentler and frees the
same `max_concurrent_downloads` slot. It does not work: for the queue, paused is "paused by the
user", it occupies no slot and **never rises on its own** (`torrents/queue.go`). The stalled
download would become stalled, silent and permanent — which is exactly the state this function
exists to end.

**What looks wrong, part two:** the status filter. Only `downloading` and `downloading_metadata`
are candidates. It looks like an oversight not to cover `verifying`/`allocating`, but zero peers
on those two is local work in progress, not a dead swarm. And the three excluded are mandatory,
not optional: a **queued** torrent is at zero peers because it has not even started
(`queue.markQueued`), **paused** because the user told it to stop, and **complete** because nobody
is pulling from it. Without that filter the function would drop the entire queue an hour after
the app came up.

**Why the clock is package-level and in memory:** only the pass writes it and nothing outside the
daemon needs to read it. A restart re-observes stalls from zero, and that costs at most one extra
`check_interval` cycle. The prune at the end of each call is what stops a stamp from weeks ago
from immediately dropping a torrent that stalled, came back and stalled again.

**Do not confuse with `frontend/src/lib/stores/stallTracker.ts`.** Both measure the same thing and
do not talk: the frontend's fires the "no seeds" chip at 10 minutes and dies on a page reload; the
daemon's drops the torrent in one hour. The thresholds are different on purpose — painting a
warning and dropping a download do not deserve the same patience.

**Don't "fix" by:** promoting the threshold to config with no request (the cost of being wrong is
one extra pass, not a lost download — the bytes stay), nor turning the drop into a blacklist. #24
still holds: if the re-add becomes observed churn, then persisted state is discussed.

### 95. MyAnimeList is a list source, not a metadata source

**Location:** `anilist/mal.go` (`GetMALEntries`, `GetMALList`, `malEntriesFor`, `ErrMALEmpty`);
the MAL halves of `GetAnimeInfo`/`GetAnimeInfoByIDs` (`anilist/anilist.go`); the call sites in
`daemon/verification.go` (`searchAnilist`, the delete fetch, `allAccountsAgreeOnDelete`,
`malAccountKey`) and `api/endpoint_animes.go` (`fetchMALEntries`) / `api/standalone_guard.go`.

A MAL account only answers "which anime, in which status, with what progress". Each MAL id is
mapped to an AniList `Media.Id` through `idMal_in`, and from there the entry is the same synthetic
`MediaList` a standalone anime gets (`Id == 0`, empty `CustomLists`, Media from `GetMediaByIDs`)
with the MAL status and progress on top. Identity stays AniList's
([#43](#43-an-animes-identity-is-mediaid-status-is-a-per-account-question-download--or-deletion--and)),
so `episodes.json`, `anime_settings`, the routes and the library don't know MAL exists.

**What looks wrong, part one: the list comes from `myanimelist.net/animelist/{user}/load.json`**,
an unofficial endpoint, and not from the official API v2. The v2 answers `403` without a client ID,
and a client ID either ships in the binary (public, shared rate limit, one revocation breaks every
install) or makes every user register an app. `load.json` needs nothing, but only reads **public**
lists. If it breaks, the fallback is v2 with a user-supplied client ID: only `fetchMALList` changes.
`TestLive_MALList` (`AAD_LIVE_MAL=1`) is the alarm — it already caught `is_rewatching` arriving as
`""` on some entries.

**What looks wrong, part two: an empty MAL list is an error (`ErrMALEmpty`), not an answer.**
A private list, a format change on MAL's side (every field decodes to zero) and a total mapping
failure all look like an empty list. As an empty answer the account would silently lose its veto:
AniList account with X in `COMPLETED`, MAL account watching X, MAL comes back empty → the AniList
account alone is unanimous and X is deleted. As a failure, `allAccountsAgreeOnDelete` refuses,
the same as for an AniList account that didn't answer.

**What looks wrong, part three: the status filter runs before the AniList hydration.** The MAL
list is fetched whole (`status=7`) and cached raw (MAL id, status, progress); only the entries in
the requested statuses go to `GetMediaByIDs`. `mediaByIDCache` lives 60s and `/api/v1/animes` is
polled every 30s per tab, so hydrating a ~300-entry list (mostly `COMPLETED`) would cost ~6 AniList
queries per minute per MAL account — the burn
[#46](#46-getfrontendanimelist-is-cached-for-60s--the-apiv1animes-poll-is-what-blows-anilists-quota)
and [#85](#85-the-frontends-anilist-cost-is-per-account-and-per-orphan-not-per-tab) closed. The deletion
vote never hydrates at all: it only needs AniList id + status.

**Caches:** `malListCache` (raw list, 60s) and `malIDMapCache` (MAL→AniList id, 24h, `0` = no
counterpart, cached so an unmappable entry doesn't cost a request per pass). Both go into the disk
snapshot: the list comes back with TTL zero like the other lists
([#92](#92-the-on-disk-anilist-cache-restores-with-ttl-zero-and-a-value-served-from-it-comes-with-errfromcache)),
the mapping with its full TTL like `Series`. MAL down → stale list + `ErrFromCache`.

**Don't confuse:** the `inDeleteStatus` key of a MAL account is `"mal:" + user` (`malAccountKey`),
so an AniList and a MAL account with the same name don't overwrite each other. And
`MediaList.Id == 0` means "standalone **or** MAL" — never use it to tell them apart.

**Don't "fix" by:** making MAL a metadata source (a second identity and most of the app
duplicated); putting the MAL code in its own package (it needs `ttlCache`, the snapshot,
`sendAnilistRequest` and `GetMediaByIDs`, all internal to `anilist`); treating `ErrMALEmpty` as an
empty list; or hydrating before filtering.

### 96. A first-season entry rejects later-season packs, but a sequel without a marker doesn't

**Location:** `daemon/episodes.go` (`isFirstSeasonEntry`, the first case of
`partitionSearchResults`); the permissive rule it narrows is the pack branch of
`nyaa.ScrapNyaaForAnime`.

With no season requested, the nyaa package accepts a pack of **any** season and lets the range
coverage decide ([#79](#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses)). That broke on Medalist: the entry is season 1, progress 2, and
`[Sylvar] Medalist Season 2 (BD Remux)` declares the files 1..9 — delta 0 matched, and episodes
3..9 were assigned to the season 2 pack (it only didn't download because it didn't fit on disk).

**What looks inconsistent: the drop happens only when the entry is CERTAINLY season 1** — no season
marker in its titles (`ExtractAnimeSeasonPart`) and no `PREQUEL` edge of format TV/TV_SHORT. A
sequel whose title carries no marker still accepts packs of any season, because there the pack's
"Season 2" can be the right one under a different numbering, and coverage is still the better
judge. It lives in the daemon and not in the nyaa filter because only the daemon has the relations.

**Don't "fix" by:** moving it into `ScrapNyaaForAnime` (no relations there); or making the nyaa
pack filter strict for every unmarked entry (it zeroes the packs of every sequel without a marker).

### 97. A pack that doesn't fit on disk is dropped before the choice, not at download time

**Location:** `daemon/episodes.go` (`packsThatFit`, called right before `pickBatches` in
`processAnimeEpisodes`, and in `debug.go`).

The disk guard ([#93](#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_percent--0)) already ran per candidate in
`attemptDownloadWithRetries`, but by then `pickBatches` had given the pack the whole window. On
Flower and Asura the only covering pack was a 78.8 GB BD remux; it failed the guard and every
episode was left with no candidate, while the SubsPlease episodes had 26 seeders. Now the packs the
guard would refuse leave first: the next pack covers the window, or the per-episode search does.

**What looks redundant: the guard still runs again at download time.** The early call is a
snapshot; the space can change between the choice and the `Add`, and loose episodes never went
through this one.

**Cost accepted:** a pack dropped here and one dropped at the `Add` look the same in the report
(`BatchSkippedNoCoverage` when no pack is left to cover). Telling them apart needs its own code.

### 98. A large release without an episode number is an unlabeled pack, and only its file list makes it a pack

**Location:** `nyaa/nyaa.go` (`minUnlabeledPackBytes`, `TorrentResult.UnlabeledPack`, the
`unlabeled` case of `ScrapNyaaForAnime`); `daemon/episodes.go` (`packSet.covering`);
`nyaa/nyaa_match.go` (`truncateAtFirstMarker` dropping bracket groups from a markerless name).

Old anime is released as one torrent named only by the title and the source:
`[man] Azumanga Daioh [BD 1080p HEVC FLAC]` (82 seeders) carries no "batch", no range, no
"complete". It wasn't `IsBatch`, had no episode number, and was dropped — Azumanga was left with
dead loose episodes while ~15 live packs sat on the same page. Two things blocked it: the
classification, and the title match (with no marker to cut at, `[BD 1080p HEVC FLAC]` was
tokenized and sank the Jaccard below 0.8).

**What looks inconsistent with [#84](#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything): an unknown range doesn't count as "covers
everything" for this pack.** A labeled pack with an unresolved range still does (a Nyaa outage
must not zero everyone's packs). An unlabeled one is a pack only by guess — size ≥ 1 GiB, no
number, no movie marker — so it must prove it with the file list (`PackFileRange`); without it,
`covering` skips it. It shares the `maxPackDetailFetches` budget.

**Don't "fix" by:** lowering `minUnlabeledPackBytes` (a short or web special, like the 19 MiB
`Azumanga Web Daioh`, would compete); or letting an unresolved unlabeled pack count as complete.

### 99. A roman-numeral sequel searches by its bare base title first

**Location:** `nyaa/nyaa.go` (`romanSequelBase`, the first variant in
`GenerateSearchTitleVariants`); `reRomanSeasonTitle` in `nyaa/nyaa_regex.go`.

DanMachi V's titles are "…Machigatteiru Darou ka **V: Houjou no Megami-hen**" and "…in a Dungeon?
V". The releases say "…Machigatteiru Darou ka **S5** - 08". Every variant carried the numeral and
the subtitle, so the title match required tokens the releases don't have, and only dead DubOnly
rows (named after the full title) were found. `ExtractSeason` already read the "V" as season 5 —
the filter was right, the query was wrong. The base title ("…Darou ka") now goes first; the
season filter keeps the other seasons out.

**What looks too strict: the numeral counts only when the English title carries the same one as a
word.** "Lupin III" (English "Lupin the 3rd") is a name, not a season 3, and gets no base variant.

### 100. An empty per-episode search waits before repeating, and stops the later episodes

**Location:** `daemon/search.go` (`searchMisses`, `episodeMisses`, `nyaaSearcher.misses`);
`daemon/episodes.go` (`stuck` and the miss checks in `processAnimeEpisodes`'s download loop);
`daemon/standalone.go` (the manual download clears `misses`).

Two dead-swarm entries (Sound! Euphonium Shorts and its sequel) cost ~60 Nyaa requests per pass:
every pending episode fell into the per-episode search, and each one tried all four title variants
— every `check_interval` (10 min), forever. After the pack fixes (#96–#99) that was most of the
pass's traffic.

Two rules, both only for the per-episode fallback:

1. **Backoff per episode.** An episode whose search comes back with no candidate is not searched
   again for 1h, then 2h, 4h, 8h, 16h, capped at 24h. Finding a candidate clears it.
2. **Stop at the first miss.** Once an episode ends the pass without a torrent, the episodes after
   it skip the per-episode search in that pass. Episodes are released in order; if 3 isn't there,
   4 almost never is.

**Why this doesn't lose episodes:** the anime search (`ScrapNyaaForAnime`, one query, packs AND
loose episodes) still runs every pass. A pack or a live episode that shows up is caught there;
the skipped episodes still get whatever it found. A newly aired episode has no miss recorded and
is searched right away. A skipped episode keeps its report row with the stats of its last real
search; one that was never searched itself (skipped because an earlier one is missing) reports
`awaiting_earlier_episode` — `no_torrent_found` would claim a search that didn't happen.

**Cost accepted:** an episode that's never released (a fansub that skipped one) blocks the
per-episode search of the ones after it — they're left to the anime search. And the state is in
memory: a restart forgets it, at the price of one full round of searches.

**Don't "fix" by:** applying it to the manual download (the button would do nothing for up to a
day — `misses` is nil there) or to `RunAnimeDebug` (a diagnostic must show every episode's search —
the one deliberate gap in [#58](#58-runanimedebug-mirrors-processanimeepisodes-enumeration-pack-resolution-and-single-fallback)'s mirror); or persisting it to disk.

### 101. The disk guard counts what the queue still has to write, not only free space

**Location:** `daemon/diskbudget.go` (`diskBudget`, `newDiskBudget`); `checkDiskSpace`'s
`reservedBytes` in `daemon/helpers.go`; built once per pass in `daemon/verification.go` and in
`DownloadStandaloneAnime`; used by `packsThatFit` and `attemptDownloadWithRetries`;
`files.EpisodeStruct.TorrentSize`; `torrents.InfoHash`.

A clean run on a disk with 99 GB free and `min_free_disk_percent: 10` (~6 GB of headroom) queued
58 torrents, ~130 GiB. The guard was right about every big pack (29 refused) and wrong about the
sum: it compared each torrent with the free space NOW, and a torrent only takes its space as it
downloads (rain creates sparse files). Each ~1.4 GiB loose episode fit on its own.

**The rule:** a torrent fits when `size + reserved` fits, where `reserved` is what every incomplete
torrent of the session still has to write plus everything this pass already added.

- **Per pass, shared by the parallel animes.** `reserve` checks and counts under one lock — two
  animes must not both take the last free bytes.
- **Keyed by infohash** (`torrents.InfoHash` on the magnet, before `Add`). The N episodes of a pack
  add the same magnet N times and pay once; a torrent already in the session pays nothing.
- **A queued torrent's size comes from the record.** `BytesTotal` is 0 until the metadata arrives,
  and a queued torrent never started. So the record now keeps the size Nyaa announced
  (`torrent_size`). Remaining = `BytesTotal` (or the recorded size) minus what's written, with the
  bitfield ratio as the written amount when it's larger (a paused torrent zeroes `BytesCompleted`).
- **`packsThatFit` asks the same budget.** Asking free space alone there would let a pack win the
  window and lose it at the add — #97's bug again.

**Cost accepted:** a torrent with neither metadata nor a recorded size (records older than
`torrent_size`, hand-pasted magnets) reserves nothing until its metadata arrives. And a pack can
still pass `packsThatFit` and lose at the add when another anime of the same pass reserved the
bytes in between — rare, and the episodes are retried next pass.

**Not here:** the queue (`torrents/queue.go`) doesn't gate promotion by disk. The guard stops the
over-commit when torrents are added; a queue already over-committed (from before this) still
downloads everything it holds.

**Manual downloads pass `reservedBytes = 0`** (`manual_download.go`): a magnet the user pastes is
their call, and its size is usually unknown anyway.

**Don't "fix" by:** summing reservations per record instead of per hash (a pack would count once
per episode); checking free space alone in `packsThatFit`; or splitting `reserve` into a check and a
separate count (the parallel animes would race between the two).

---

### 102. The bug report has no backend: consent is the act of attaching the file

**The rule:** nothing in the app sends a report anywhere. The daemon builds a redacted bundle
locally (`GET /api/v1/diagnostics`), the report screen shows it in full, downloads it as a `.txt`
and opens the GitHub issue form pre-filled through the URL. The user attaches the file by hand.

- **Why no backend.** An upload endpoint is a server to run, a place where logs of other people's
  machines pile up, and an abuse target — for a self-hosted app with no account system. GitHub
  already has identity, moderation and a conversation with the reporter.
- **Why the file isn't in the URL.** GitHub cuts long URLs off, and a URL ends up in browser
  history and server logs. Only the short `environment` summary and the user's own text go there
  (the text truncated to a 4000-char encoded budget, `report.ts`).
- **Consent = attaching.** The preview is exactly the file (the titles checkbox regenerates both),
  and the screen says the issue is public. There is no separate "I agree" step to keep in sync.
- **Reading from disk, not from the daemon** (`diagnostics` package): the same bundle can be built
  by a CLI command with the daemon dead (planned for `v2.6.0`).

**Cost accepted:** a user who doesn't attach the file sends an issue with only the environment
summary. The issue form's required "I attached the report file" checkbox is the only nudge.

**Don't "fix" by:** adding an upload/"send report" button, putting the bundle in the issue URL, or
having the endpoint read state from the running daemon instead of the files.

---

### 103. Log redaction is by key, and a test forces every new key to be classified

**The rule:** the bundle's log lines are JSON, and each field is redacted by its **key** through
`keyClasses` in `diagnostics/redact.go`: `keep`, `drop` (`remote_addr`), `pseudonymize` (account
names → `user1`, `user2`…, stable within one report) or `title` (anime names, torrent names, paths,
ids → `<title-N>` when the user unticks "include titles"). Free text — `message`, `error` and kept
strings — then goes through the text scrubber: webhook URLs and header values anywhere →
`<webhook-N>`, the home dir → `~`, URLs to hosts outside the allowlist → `<url>`, known usernames →
`userN`. `"HTTP request"` lines are dropped whole (they carry the LAN IPs of whoever opened the UI).

- **Why by key and not by regex over the text.** A title has no shape a regex can find; the key is
  the only reliable signal of what a value is. The text scrubber only catches what does have a shape
  (URLs, the configured webhooks, the configured usernames, the home path).
- **A key missing from the table is treated as a `title`** — the strictest class that still keeps
  the line readable. That's the runtime safety net; the real guard is the test.
- **`keys_test.go` (`TestEveryLogKeyIsClassified`)** greps every non-test `.go` file under `src/`
  (minus `src/tests`) for zerolog field calls with a literal key (`.Str("anime", …)`, `.Int(…)`,
  `.Interface(…)`…) and fails listing each key `keyClasses` doesn't have. A new log field can't
  reach a report without someone deciding what it is.

**Cost accepted:** a key built at runtime (not a string literal) escapes the test and falls back to
`title`. None exists today.

**Don't "fix" by:** defaulting unknown keys to `keep`, replacing the table with a regex over the
whole line, or silencing the test by adding a key as `keep` without checking what it carries.

### 104. --yes is a CLI gate; the API keeps its query defaults

**The rule:** destructive CLI commands (`animes remove` / `redownload` / `delete` / `replace` /
`replace-episode`, `torrents delete`) refuse without `--yes`: exit 2, code `CONFIRMATION_REQUIRED`,
message lists the flags. No stdin, no prompt, no TTY check. After `--yes` they call the same
endpoints with the same query defaults (`parseBoolQueryParam`: missing → `false`). `--keep-data`,
`--block` and `--delete-episodes` map 1:1 onto those params; absent means `false`.

The web UI keeps its dialog. The API does not grow a confirmation parameter. An agent cannot hang
on stdin, and changing API defaults to match the web dialog would change the UI's "unchecked means
false" contract too.

**Don't "fix" by:** prompting when stdin is not a TTY, defaulting `--yes` on in scripts, or adding
`confirm=true` to the API.

### 105. FetchTorrentDetail builds the view URL itself; it never takes one from the caller

**Location:** `nyaa/detail.go` (`FetchTorrentDetail`, `ParseNyaaID`); consumed later by the
standalone torrent picker's description pane.

**The rule:** the only input is a numeric Nyaa id. The view URL is always
`getNyaaBaseURL()+"/view/"+strconv.Itoa(nyaaID)`. A caller that already has a `DetailURL` parses
the id with `ParseNyaaID` and hands that over — it does not pass the string through.

**Why:** the API that will expose this will take an id from the client. Accepting a URL would let
the client point the daemon at any host (`NYAA_URL` is the operator's choice; a request body is
not). Building the URL here keeps SSRF out of every future caller by construction.

**Don't "fix" by:** adding a `FetchTorrentDetailURL(url string)` convenience that "just" reuses
the same parser, or by letting the API accept a URL "because the UI already has it".

### 106. Empty first search skips the picker; custom q drops variants; description is on demand; add-with-magnet writes the id after the torrent

**Location:** Add's first `getTorrentCandidates` (`AddAnime.svelte`); `PreviewWholeTitleTorrents`
(`daemon/preview.go`) for a custom `q`; `GET /api/v1/animes/{id}/torrent-candidates/{nyaaId}`;
`DownloadStandaloneAnimeWithMagnet` (`daemon/standalone.go`) and `POST /standalone-animes`.

**The rule:**

1. **Empty first search skips the picker.** Add's first candidates GET (no `q`) with `items: []`
   POSTs `/standalone-animes` without a magnet — singles, or track-with-zero for an unreleased
   title — and stays on the card. A later empty custom query does **not**: Enter on an empty
   field is a no-op, and a custom `q` that returns no packs keeps the picker open. The
   automatic-singles fall-through exists only for that first, daemon-query search.
2. **Custom `q` drops variants.** A non-empty `q` is the Nyaa query as typed: no AniList
   title-variant loop, no season/part row filter. What still runs: `ShouldIgnore`, `min_seeders`,
   size ceiling, packs-only (or movie, when AniList format is `MOVIE`), Priorities ranking, cap 8,
   `titleMatchesQuery` against the query tokens.
3. **Description is on demand.** `/view/<id>` is fetched only for the selected row
   (`GET …/torrent-candidates/{nyaaId}`). The candidates list does not scrape it; range on that
   list comes from the name (`ExtractBatchInfo`).
4. **Add-with-magnet writes the id after the torrent.** The magnet enters the session first,
   then the standalone id, then coverage records. If `backend.Add` (or the disk guard) refuses,
   the id is never written. If the id is written and a later step fails, `RemoveStandaloneAnime`
   runs before the error returns. Coverage is only the aired episodes the chosen torrent covers
   (name, then one `PackFileRange` if the name has none); unknown range writes **no** episode
   records, not `1..total` ([#84](#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)).
5. **`name` / `nyaa_id` / `size_bytes` are optional on POST** because a magnet URI cannot rebuild
   coverage or the Nyaa size. The picker sends them from the selected row; a pasted magnet may
   omit them (coverage then unknown, disk guard without a size).

**Why:** the ranking is still the right default; the picker is a confirm step, not a Nyaa
browser. Fetching eight `/view/<id>` pages up front is the cost [sources.md](sources.md) called
out. Writing the id before the torrent leaves a tracked anime with nothing in the session.

**Don't "fix" by:** falling through to automatic singles on a custom empty query; fetching
`/view/<id>` inside the candidates handler "so the pane is instant"; writing the standalone id
before `backend.Add`; persisting the picker cache or the custom `q`; inventing `1..total`
coverage from a magnet that has no `name`/`nyaa_id`.

### 107. Flags after positionals are reordered before urfave parses

**Location:** `cmd/cli/args.go` (`reorderInterspersedFlags`); `run` in `main.go` is the only
entry that tests and `main` use.

**The rule:** Go's `flag.FlagSet.Parse` stops at the first non-flag argument. urfave/cli v2
uses that parser, so `animes add 21 --magnet URI` never sets `--magnet` — leftover tokens fail
`requireArgs`, or a bool flag after the id is silently ignored. `run` rewrites argv before
`App.Run`: command names stay in order, global flags move to the front, each command's flags
sit immediately after that command. `--` still ends flag parsing. A new CLI flag is picked up
from the command tree; do not maintain a second list of names.

**Why:** the documented order is `animes add <id> --magnet` / `candidates <id> --query` /
`replace <id> --magnet --yes`. Putting flags first works without this; Unix tools accept both.

**Don't "fix" by:** telling users to put flags first; calling `newApp().Run` from tests (that
skips the reorder); `SkipFlagParsing` (the leaf would stop seeing `--magnet` at all); bumping
to urfave v3 only for interspersed flags.
