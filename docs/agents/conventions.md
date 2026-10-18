# Coding Conventions

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
return fmt.Errorf("episode %d not found for anime %d", episodeNumber, animeId)
```

Logging errors (zerolog chain):

```go
logger.Logger.Error().Err(err).Msg("Failed to add torrent")
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
| JSON tags | snake_case | `json:"completed_anime_path"` |
| Go types | PascalCase | `EpisodeStruct`, `SessionManager` |

## Readability

**Names explain.** Functions and variables are named so the reader knows what they are without
opening the body. No `tmp`, `data`, `x`, `doStuff`. If a name needs a comment to be understood,
the name is wrong — rename it, do not add the comment.

**A large inline condition or expression becomes a variable.** When an `if` (or any expression)
joins several checks, the variable name is the explanation:

```go
// bad
if ep.Number > anime.Progress && !ep.Downloaded && ep.Number <= anime.EpisodeCount {

// good
isPendingEpisode := ep.Number > anime.Progress && !ep.Downloaded && ep.Number <= anime.EpisodeCount
if isPendingEpisode {
```

**A block that grew a new responsibility becomes a function.** When a stretch of code starts
doing *something else* inside the function, extract it and name that thing. The trigger is the
change of responsibility, not the line count — do not split a cohesive function just because it
got long.

## Comments

Do not write a comment that explains **what** the code does — only **why** it exists that way,
and only when the names of the variables and functions do not already make it obvious. A comment
found outside that rule: delete it.

**Non-obvious decision → `decisions.md` + a pointer in the code.** If a later reader would ask
"why this, when the obvious way is the other one?", add an entry to [decisions.md](decisions.md)
and point to it from the relevant stretch, by number:

```go
// Converts saved AnimeIDs from list-entry ids to media ids (decisions.md #43).
```

The format used in the repo is `decisions.md #N` — keep it, that is what `grep` finds.

## Dual FileManagerInterface

`FileManagerInterface` is declared **twice** — in `api/server.go` and `daemon/helpers.go`. Both must be kept in sync. When adding a new method to `files.FileManager`, update **both** interfaces.

## CLI stays at parity with the API

A new API endpoint, or a web-app action that hits the API, ships with its CLI command in the
**same change**. The CLI is how an agent (and a human who does not open the web UI) drives the
daemon — leaving a verb only on the screen means the agent cannot do it.
`--yes` is a CLI gate; the API keeps its query defaults
([decisions.md #104](decisions.md#104---yes-is-a-cli-gate-the-api-keeps-its-query-defaults)).

Does not apply to frontend-only work: layout, i18n, presets that live in the frontend, WebSocket.
A new `config.json` field is already settable through `config set` (passthrough of every JSON
key); no new command.

## Adding a New API Endpoint — Checklist

1. Create handler in `endpoint_<feature>.go` (or extend existing file)
   - Closure over `*Server`
   - Method dispatch with `switch r.Method`
   - Validate input, respond with `JSONSuccess`/`JSONError`/`JSONInternalError`
2. Register route in `SetupRoutes()` in `server.go`
   - `apiMux.HandleFunc("/api/v1/...", handleFeature(s))`
3. If handler needs new `FileManager` method:
   - Add method to `files.FileManager`
   - Update `FileManagerInterface` in **both** `api/server.go` and `daemon/helpers.go`
4. CLI, in the same change:
   - method on `api.Client` (`src/internal/api/client.go`)
   - nested command under the matching resource in `src/cmd/cli/` (`animes …`, `torrents …`)
   - `--help` text (`Usage`, `ArgsUsage`, flags, defaults). Destructive actions take `--yes`
   - `docs/guides/cli.md`
5. Write tests in `endpoint_<feature>_test.go` (and the CLI/client tests for the new command)
6. Run tests: `go test ./...`
7. Update `docs/agents/architecture.md` API table — it is the **only** endpoint doc since Swagger
   left the binary

## Adding a New Config Field — Checklist

1. Add field to `Config` struct in `filemanager.go` with `json:"snake_case"` tag
2. Set default in `getDefaultConfig()` if needed
3. Add validation in `handleUpdateConfig()` in `endpoint_config.go` if needed
4. Update `docs/agents/config.md`
5. Update frontend config form (`src/internal/frontend/src/routes/Config.svelte`)
6. CLI: no new command. `config set` already accepts every JSON key; `config get` lists them
7. Run tests: `go test ./...`
