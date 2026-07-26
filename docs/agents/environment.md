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
| `NYAA_URL` | `"https://nyaa.si"` | `nyaa/nyaa.go` | Override Nyaa base URL |
| `ANILIST_API_URL` | `"https://graphql.anilist.co"` | `anilist/anilist.go` | Override Anilist GraphQL endpoint |

> The former `QBITTORRENT_URL` override was removed along with the external qBittorrent dependency — the BitTorrent client is now embedded (`github.com/cenkalti/rain/v2`).

## OS / Path

| Variable | Where | Description |
|----------|-------|-------------|
| `HOME` | `files/filemanager.go`, `logger/logger.go`, `cmd/daemon/main.go` | Linux/macOS: base for `~/.autoAnimeDownloader/` |
| `APPDATA` | same files | Windows: base for `%APPDATA%\AutoAnimeDownloader\` |

## Test-Only

| Variable | Default | Where | Description |
|----------|---------|-------|-------------|
| `DAEMON_URL` | — (unset = skip) | `tests/integration/integration_test.go` | Daemon base URL for integration tests. **Must be set explicitly or every integration test skips** — the suite overwrites the config of whatever daemon answers, so it must never find a developer's daemon by accident. Set to `http://daemon:8091` by `docker-compose.test.yml`. When set but empty/unreachable, tests also skip. The `http://localhost:8091` fallback is only used to build the URL once the gate has passed |
| `TEST_SAVE_PATH` | `~/aad-test/downloads` | `tests/integration/integration_test.go` | `save_path` the integration tests write into the daemon's config. Interpreted by the **daemon**, not the test process. Docker overrides it to `/app/data/aad-test/downloads` |
| `TEST_COMPLETED_PATH` | `~/aad-test/library` | `tests/integration/integration_test.go` | `completed_anime_path` the integration tests write. Must share a filesystem with `TEST_SAVE_PATH` (hardlinks). Docker overrides it to `/app/data/aad-test/library` |
| `SCENARIO` | — | `tests/mocks/anilist/`, `tests/mocks/nyaa/` | `"empty"` = mock returns empty data |
| `PORT` | `8080`/`8081` | `tests/mocks/*/mock_server.go` | Listen port for each mock server (Anilist, Nyaa) |
| `TEST_LOGGER_INIT` | — | `logger/logger_test.go` | When empty, auto-inits logger for tests |
