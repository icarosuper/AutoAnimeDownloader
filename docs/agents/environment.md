# Environment Variables

## Application

| Variable | Default | Where | Description |
|----------|---------|-------|-------------|
| `ENVIRONMENT` | `"dev"` | `cmd/daemon/main.go` | Set to `dev` for formatted console logs. Any other value = production JSON logs |
| `PORT` | `":8091"` | `cmd/daemon/main.go`, `daemon/helpers.go` | HTTP listen address. Prefix `:` auto-added if missing |
| `DISPLAY` | — | `cmd/daemon/main.go` | Linux only. System tray icon shown only when non-empty (graphical session) |

## Service Overrides

These override values from `config.json`. Useful for Docker/CI:

| Variable | Default | Where | Description |
|----------|---------|-------|-------------|
| `QBITTORRENT_URL` | config value | `daemon/helpers.go`, `api/endpoint_episode_actions.go` | Override qBittorrent WebUI URL |
| `NYAA_URL` | `"https://nyaa.si"` | `nyaa/nyaa.go` | Override Nyaa base URL |
| `ANILIST_API_URL` | `"https://graphql.anilist.co"` | `anilist/anilist.go` | Override Anilist GraphQL endpoint |

## OS / Path

| Variable | Where | Description |
|----------|-------|-------------|
| `HOME` | `files/filemanager.go`, `logger/logger.go`, `cmd/daemon/main.go` | Linux/macOS: base for `~/.autoAnimeDownloader/` |
| `APPDATA` | same files | Windows: base for `%APPDATA%\AutoAnimeDownloader\` |

## Test-Only

| Variable | Default | Where | Description |
|----------|---------|-------|-------------|
| `DAEMON_URL` | `"http://localhost:8091"` | `tests/integration/integration_test.go` | Daemon base URL for integration tests |
| `SCENARIO` | — | `tests/mocks/anilist/`, `tests/mocks/nyaa/` | `"empty"` = mock returns empty data |
| `PORT` | `8080`/`8081`/`8082` | `tests/mocks/*/mock_server.go` | Listen port for each mock server |
| `TEST_LOGGER_INIT` | — | `logger/logger_test.go` | When empty, auto-inits logger for tests |
