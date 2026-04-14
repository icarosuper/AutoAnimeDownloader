# Testing Patterns

## Running Tests

```bash
go test ./...                                    # all unit tests
go test -v ./src/tests/unit/                     # verbose unit tests
go test -v -run TestName ./src/tests/unit/       # single test

# integration (requires Docker)
docker compose -f docker/docker-compose.test.yml up --build --abort-on-container-exit
```

**Always run `go test ./...` after any code change.**

## Test Structure

- `src/tests/unit/` — unit tests (`package unit`)
- `src/tests/integration/` — Docker-based end-to-end HTTP tests
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

### 2. Interface Injection (qBittorrent / Torrents)

`torrents.HTTPClient` interface — tests pass a `MockHTTPClient` with URL-keyed responses:

```go
type HTTPClient interface {
    Get(url string) (*http.Response, error)
    PostForm(url string, data url.Values) (*http.Response, error)
}

// Test:
mock := NewMockHTTPClient()
mock.AddResponse("/api/v2/torrents/info", 200, torrentsJSON)
service := torrents.NewTorrentService(mock, "http://localhost:8080", "/save", "/completed")
```

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
| `qbittorrent/mock_server.go` | 8082 | In-memory torrent store |

`docker-compose.test.yml` wires daemon env vars (`ANILIST_API_URL`, `NYAA_URL`, `QBITTORRENT_URL`) to point at these mocks.

## Frontend Tests

No automated tests exist for the Svelte frontend. Only static type checking via `bun run check` (svelte-check).
