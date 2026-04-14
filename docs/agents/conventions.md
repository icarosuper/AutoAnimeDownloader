# Coding Conventions

## Critical Rules

1. **Always run tests** after implementing anything: `go test ./...`
2. **Always update docs** (`docs/agents/`) when adding/changing endpoints, config fields, env vars, or patterns
3. **Always update Swagger** annotations when changing API endpoints

## Error Handling

Wrap errors with `fmt.Errorf("failed to <action>: %w", err)`. Use `%w` (not `%v`) so callers can use `errors.Is`/`errors.As`.

```go
config, err := m.fs.ReadFile(m.configPath)
if err != nil {
    return nil, fmt.Errorf("failed to read config file: %w", err)
}
```

Domain/validation errors (no underlying error to wrap):

```go
return fmt.Errorf("episode %d not found for anime %d", episodeId, animeId)
```

Logging errors (zerolog chain):

```go
logger.Logger.Error().Err(err).Msg("Failed to connect to qBittorrent")
logger.Logger.Error().Err(err).Stack().Msg("Critical failure")  // .Stack() for serious errors
```

No sentinel errors — behavior is message-driven, not `errors.Is`-driven.

## HTTP Handler Pattern

Handlers are closures over `*Server`:

```go
func handleFeature(server *Server) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // method dispatch
        switch r.Method {
        case http.MethodGet:
            handleGetFeature(server)(w, r)
        case http.MethodPost:
            handleCreateFeature(server)(w, r)
        default:
            JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and POST methods are allowed")
        }
    }
}
```

Path params via Go 1.22+ `r.PathValue`:

```go
id, err := strconv.Atoi(r.PathValue("id"))
if err != nil || id <= 0 {
    JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
    return
}
```

Responses — always use shared helpers from `responses.go`:

```go
JSONSuccess(w, http.StatusOK, data)
JSONError(w, http.StatusBadRequest, "ERROR_CODE", "Human message")
JSONInternalError(w, err)
```

All API responses follow envelope `{success, data, error}`.

## Swagger Annotations

Every handler needs swag annotations above the function:

```go
// @Summary      Short description
// @Description  Longer description
// @Tags         feature
// @Accept       json
// @Produce      json
// @Param        id   path      int   true  "Anime ID"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Router       /feature/{id} [get]
func handleGetFeature(server *Server) http.HandlerFunc {
```

Regenerate docs after changes: `swag init -g src/cmd/daemon/main.go -o docs/swagger`

## Import Order

Internal packages first, then stdlib, then third-party:

```go
import (
    "AutoAnimeDownloader/src/internal/daemon"
    "AutoAnimeDownloader/src/internal/files"
    "AutoAnimeDownloader/src/internal/logger"

    "encoding/json"
    "net/http"
    "strconv"

    "github.com/PuerkitoBio/goquery"
)
```

## File & Function Naming

| What | Convention | Example |
|------|-----------|---------|
| API endpoint files | `endpoint_<area>.go` | `endpoint_config.go`, `endpoint_episode_actions.go` |
| API endpoint tests | `endpoint_<area>_test.go` | `endpoint_config_test.go` |
| Handler functions | `handle` + resource/action | `handleConfig`, `handleDownloadEpisode` |
| JSON tags | snake_case | `json:"save_path"` |
| Go types | PascalCase | `EpisodeStruct`, `TorrentService` |

## Dual FileManagerInterface

`FileManagerInterface` is declared **twice** — in `api/server.go` and `daemon/helpers.go`. Both must be kept in sync. When adding a new method to `files.FileManager`, update **both** interfaces.

## Adding a New API Endpoint — Checklist

1. Create handler in `endpoint_<feature>.go` (or extend existing file)
   - Closure over `*Server`
   - Method dispatch with `switch r.Method`
   - Validate input, respond with `JSONSuccess`/`JSONError`/`JSONInternalError`
   - Add Swagger annotations
2. Register route in `SetupRoutes()` in `server.go`
   - `apiMux.HandleFunc("/api/v1/...", handleFeature(s))`
3. If handler needs new `FileManager` method:
   - Add method to `files.FileManager`
   - Update `FileManagerInterface` in **both** `api/server.go` and `daemon/helpers.go`
4. If exposing to CLI: add method in CLI's `client.go`
5. Write tests in `endpoint_<feature>_test.go`
6. Regenerate Swagger: `swag init -g src/cmd/daemon/main.go -o docs/swagger`
7. Run tests: `go test ./...`
8. Update `docs/agents/architecture.md` API table

## Adding a New Config Field — Checklist

1. Add field to `Config` struct in `filemanager.go` with `json:"snake_case"` tag
2. Set default in `getDefaultConfig()` if needed
3. Add validation in `handleUpdateConfig()` in `endpoint_config.go` if needed
4. Update `docs/agents/config.md`
5. Update frontend config form (`src/internal/frontend/src/routes/Config.svelte`)
6. Run tests: `go test ./...`
