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

Replace `logger.Logger` with a `zerolog` writing to `bytes.Buffer` to assert log output:

```go
var buf bytes.Buffer
logger.Logger = zerolog.New(&buf)
// ... run code ...
assert(strings.Contains(buf.String(), "expected message"))
```

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
