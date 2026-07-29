# Testing Patterns

## Running Tests

```bash
go test ./...                                    # all unit tests
go test -v ./src/tests/unit/                     # verbose unit tests
go test -v -run TestName ./src/tests/unit/       # single test

# integration (requires Docker)
make test-backend-integration
# or directly:
docker compose -f docker/docker-compose.test.yml up --build --abort-on-container-exit
```

**Always run `go test ./...` after any code change.**

`go test ./...` **skips** the integration suite: it only runs when `DAEMON_URL` is set explicitly. That gate is deliberate — see [Decisions #23](decisions.md). Do not "fix" the skip by removing the gate; run the tests through Docker instead.

## Test Structure

- `src/tests/unit/` — unit tests (`package unit`)
- `src/tests/integration/` — Docker-based end-to-end HTTP tests. Gated on `DAEMON_URL`; the
  library path it writes into the daemon's config comes from
  `TEST_COMPLETED_PATH` (default `~/aad-test/library`)
- `src/tests/mocks/` — standalone mock HTTP servers (Docker images, **not** Go test imports)
- `src/internal/api/*_test.go` — handler-level tests

## Mock Patterns

### 1. Package-Level Var Swap (Anilist, Nyaa)

Production code has a swappable package var. Tests replace it and defer restore:

```go
// In production (anilist/anilist.go):
var httpDo = func(req *http.Request) (*http.Response, error) { ... }

// Mock function returns a restore closure:
func MockAniListDo(fn func(*http.Request) (*http.Response, error)) (restore func()) { ... }

// In test:
restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
    return &http.Response{
        StatusCode: 200,
        Body:       io.NopCloser(strings.NewReader(jsonFixture)),
    }, nil
})
defer restore()
```

Same pattern for Nyaa: `nyaa.MockNyaaHttpGet(fn) (restore func())`.

### 2. Interface Injection (Torrent backend)

Torrent logic sits behind the `torrents.TorrentBackend` interface. Production uses the rain-backed `SessionManager`/`Session`; tests use the in-memory `torrents.FakeBackend` instead of a mock qBittorrent HTTP server:

```go
type TorrentBackend interface {
    Ensure(savePath string) (bool, error)
    Add(magnet string) (string, error)
    List() []TorrentInfo
    Get(hash string) (TorrentInfo, bool)
    Remove(hash string, keepData bool) error
    SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error))
    Close() error
}

// Test:
backend := torrents.NewFakeBackend()
backend.AddCompleted("abcd...hash", "/save/abcd")   // seed a completed torrent
// or drive callbacks:
backend.CompleteTorrent("abcd...hash", "/save/abcd") // fires onComplete
backend.FailTorrent("abcd...hash", err)              // fires onFailed
```

`FakeBackend.Remove` on an unknown hash is a **no-op returning `nil`**, deliberately mirroring rain's `RemoveTorrent` (which returns `(nil, nil)` for an id absent from its map). Don't assert an error there — the fake used to return one, and since `removeEpisodesAndLinks` and `HandleTorrentFailure` both log a `Warn` when `Remove` fails, that made tests observe warnings production never emits.

Library hardlinking is likewise tested through the `files.Librarian` interface (`NewLibrarian`) backed by a `MockFileSystem`, so `Organize`/`ProbePaths` can be exercised without touching a real disk. Two cases need a **real** disk instead (`t.TempDir()` + `OSFileSystem`), because they turn on file identity and link counts that an in-memory fake cannot model: `Organize`'s same-inode vs. different-inode branch (decision 28), and "deleting an episode removes both links" (`TestRemoveEpisodesAndLinks_RealHardlinks`).

### 3. In-Memory FileSystem

`files.FileSystem` interface abstracts disk I/O. Tests use `MockFileSystem` with in-memory maps:

```go
mockFS := NewMockFileSystem()
mockFS.SetFile("/config.json", []byte(`{"anilist_username":"test"}`))
manager := files.NewManager(mockFS, "/config.json", "/episodes", "/blocked", "/settings")
```

For tests needing real disk: `withTempManager` creates a temp dir with real `OSFileSystem`:

```go
func withTempManager(t *testing.T, fn func(*files.FileManager)) {
    tmp, _ := os.MkdirTemp("", "aad_test_home_")
    defer os.RemoveAll(tmp)
    // ... setup paths ...
    manager := files.NewManager(files.NewOSFileSystem(), configPath, episodesPath, blockedPath, settingsPath)
    fn(manager)
}
```

### 4. API Handler Tests

Use `httptest.NewRecorder` + `http.NewRequest` with a `mockFileManager`:

```go
mockFM := &mockFileManager{
    episodes: []files.EpisodeStruct{...},
}
server := &Server{State: daemon.NewState(), FileManager: mockFM}
handler := handleAnimes(server)

req := httptest.NewRequest("GET", "/api/v1/animes", nil)
rec := httptest.NewRecorder()
handler(rec, req)

// assert rec.Code, decode rec.Body
```

### 5. Logger Capture

Use the `captureLogs` helper (`src/tests/unit/logcapture_test.go`) rather than swapping `logger.Logger` by hand:

```go
logBuf := captureLogs(t, zerolog.ErrorLevel)
// ... run code ...
assert(strings.Contains(logBuf.String(), "expected message"))
```

It writes into a **mutex-guarded** buffer and restores the previous logger from `t.Cleanup`. Both matter, and a plain `zerolog.New(&bytes.Buffer{})` gets both wrong:

- `AnimeVerification` fans out into goroutines that all log through the package-level logger, so an unsynchronized `bytes.Buffer` is a real data race. `-race` reports it from inside zerolog's writer, several frames away from the test that caused it.
- Restoring the logger on the last line of the test body is skipped by any `t.Fatal` above it, leaving the global logger writing into a dead test's buffer — which silently swallows the output every later test in the package asserts on.

### 6. Real rain Sessions (`torrents` package)

`SessionManager`'s own tests open **real** rain sessions, with DHT turned off via the unexported `sessionOptions{disableDHT: true}` seam (`newTestManager` sets it; production's `NewSession` always leaves DHT on).

Creating a session is the only thing that binds a **fixed** port — rain's `DefaultConfig.DHTPort`, 7246/udp — so with DHT on, the whole package failed with `address already in use` whenever the user's daemon was running, and every test run started chatting with the public DHT bootstrap routers. Turning it off makes the package hermetic and cut its runtime from ~0.5s to ~0.02s. Peer ports (20000–30000) are only bound when a torrent is actually added, which these tests never do.

Run this package with `-race`: `TestSessionManagerConcurrentEnsureAndDelegation` exists specifically to exercise the window where `Ensure`/`Close` could swap the session out from under a delegated call.

## Never Let Background Work Outlive Its Test

Two entry points start work in a goroutine and return immediately: `handleCheck` (POST `/check`) and `daemon.StartLoop`. **A test that triggers either must wait for it to finish before returning.**

| Entry point | How to wait |
|---|---|
| `handleCheck` | `defer server.waitForChecks()` (backed by `Server.checks`, a `sync.WaitGroup`) |
| `daemon.StartLoop` | `stopLoop(t, loopControl, state, done)` — cancels and waits |

A leaked goroutine keeps running `AnimeVerification` for the rest of the package's execution: real AniList HTTP calls, real disk writes under the configured paths, and reads of package-level globals (`logger.Logger`, anilist's `httpDo`, `nyaa`'s active priorities) that later tests are busy replacing. The symptoms land far from the cause:

- `-race` blames whichever unrelated test happened to swap a global at that moment.
- The `nyaa` sorting and `priorities` tests fail under `-count=5` because a phantom verification loop called `nyaa.SetPriorities` in the middle of them — `ActivePriorities().Fansubs[0]` even panicked on an empty slice.

Pass `daemon.LoopControl.Done` read **before** any `UpdateInterval` call: the field is a snapshot taken at construction, and `UpdateInterval` starts a fresh goroutine with a fresh channel while the field still points at the original.

Any test that lets `AnimeVerification` run to completion must also stub AniList — `t.Cleanup(anilist.MockAniListDo(...))`, registered **before** the goroutine's own cleanup so it is undone last — and point `SavePath`/`CompletedAnimePath` at `t.TempDir()`. Otherwise the pass hits the live API and creates the configured folders on the developer's real disk.

## Docker Mock Servers

`src/tests/mocks/` contains standalone HTTP servers for integration tests:

| Mock | Default Port | Env Control |
|------|-------------|-------------|
| `anilist/mock_server.go` | 8080 | `SCENARIO=empty` → empty media list |
| `nyaa/mock_server.go` | 8081 | `SCENARIO=empty` → no results |

There is no qBittorrent mock server anymore — `src/tests/mocks/` contains the Anilist and Nyaa mocks only (the torrent client is embedded). `docker-compose.test.yml` wires daemon env vars `ANILIST_API_URL` and `NYAA_URL` to point at these mocks.

## Frontend Tests

Frontend tests live in `src/internal/frontend/tests/`:

| Layer | Dir | Tool | Command |
|---|---|---|---|
| Unit | `tests/unit/` | Vitest + jsdom | `bun run test:unit` |
| Component | `tests/component/` | Vitest + @testing-library/svelte | `bun run test:component` |
| Smoke | `tests/smoke/` | Playwright (Chromium) | `bun run test:smoke` |

All layers mock the backend — no daemon needed. Config: `vitest.config.ts`, `playwright.config.ts`.

Run `bun install` in `src/internal/frontend/` and `bunx playwright install chromium` before first use.

**Asserting on a captured request body:** wait for the *route handler*, not for `page.waitForRequest`. The latter resolves on the request event, which can land before the handler that records the body — with the suite on parallel workers that read `undefined` intermittently. `config.spec.ts`'s `captureSave()` exposes a `whenSaved` promise resolved inside the handler; `await saved.whenSaved` before touching `saved.body`.

### Layout regressions (`tests/smoke/layout.spec.ts`)

Separate from the per-screen specs, which assert **content**. A broken layout does not fail those: the element is still in the DOM and still "visible" to Playwright — it is just off-screen. `layout.spec.ts` asserts geometry instead:

| Assertion | Guards against |
|---|---|
| `documentElement.scrollWidth <= clientWidth` on every screen at 768/1024/1280/1440px | any fixed-track grid whose columns no longer fit — the tables in `Status`/`Downloads`/`AnimeDetail` all switch to stacked cards below `lg` for exactly this reason |
| the active-downloads strip has `scrollWidth > clientWidth` | the strip degrading from "scrolls" back to "stretches its card", which is what a missing `min-w-0` on the grid item does |
| the nav rail's `boundingBox().y` is `0` before and after scrolling to the bottom | the rail losing `sticky top-0` and scrolling off with the page |
| `elementFromPoint` over a MoreMenu item returns the menu, not page content (on `#/priorities`, whose daisyUI `.card`s are `position: relative`) | the rail losing `z-30`: `position: sticky` creates a stacking context, so the panel's `z-50` only orders things *inside* the rail |
| `elementFromPoint` over the delete dialog returns the dialog | the counterpart — something (Modal/Toasts at root `z-50`) ending up *below* the rail's `z-30` |
| the Downloads header subtitle wraps into ≤2 line boxes at 375–1280px | the title block going back to `min-w-0 flex-1`: with flex-basis 0 and no min-width floor, `flex-wrap` never moves the speed box to its own line and the subtitle collapses to one word per line |
| all four Downloads filter pills pass `toBeInViewport({ ratio: 1 })` and their row has `scrollWidth <= clientWidth` at 375/414/640px | the pill row going back to `overflow-x-auto`, which hid "Seeding"/"Problems" behind a horizontal scroll |

Two things make these tests worth their runtime: they need **volume** (the fixtures build 24 torrents and 30 animes — with the two-row fixtures the other specs use, nothing overflows and every assertion passes vacuously), and the rail test asserts `window.scrollY > 200` first, so it cannot pass on a page that never scrolled.

Two techniques recur here and are worth reusing:

- **Line counting** — `range.selectNodeContents(el)` + `getClientRects().length` gives one rect per line box, i.e. the real wrap count. Asserting `clientHeight` in px would need a hardcoded line-height.
- **Stacking** — `document.elementFromPoint` at an element's center, then `el.contains(top)`. A `locator.click()` timeout would also catch it, but 30s later and without naming the covering element. When the covering element is the one that *should* intercept (the MoreMenu backdrop), drive the click with `page.mouse.click(x, y)` — `locator.click()` refuses a covered target, which is the behaviour under test.
- The Priorities screen needs **two** mocks the shared `baseConfig` doesn't provide: `config.priorities` and `GET /config/priorities/defaults`. Without them `load()` throws, no card renders, and any stacking assertion passes vacuously.

When adding a fixed-px column to any of the three grids, re-check the arithmetic in the comment above that grid's constant — usable width is `viewport − 92px rail − main padding`.
