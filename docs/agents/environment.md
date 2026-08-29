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

> `docker/entrypoint.sh` no longer reads a `SAVE_PATH` env var to template the initial `config.json` — the download directory is derived from `COMPLETED_ANIME_PATH` (see `docs/agents/config.md` — Download Path). If `SAVE_PATH` is still set in a Docker Compose file or `.env`, the entrypoint simply ignores it; it no longer appears in the generated config.

## Docker entrypoint (initial `config.json`)

Read **only** by `docker/entrypoint.sh`, which templates the first `config.json` on container
start. They are not read by the Go code, and they do nothing after the file exists unless
`FORCE_CONFIG_UPDATE` is set (which rewrites the whole file — see the caveat below).

| Variable | Default | Config key written | Notes |
|----------|---------|--------------------|-------|
| `COMPLETED_ANIME_PATH` | `/app/downloads/completed` | `completed_anime_path` | The library folder. Point it at a mounted volume — the image only creates `/app/data` |
| `ANILIST_USERNAMES` | `""` | `anilist_usernames` (array) | Comma-separated; `"a, b"` → `["a","b"]`, empty → `[]` |
| `ANILIST_USERNAME` | `""` | — | **Legacy**, accepted as a fallback for `ANILIST_USERNAMES`. Still writes the plural key |
| `EXCLUDED_LISTS` | `""` | `excluded_lists` (array) | Comma-separated, same conversion |
| `EXCLUDED_LIST` | `""` | — | **Legacy** fallback for `EXCLUDED_LISTS`, same as above |
| `CHECK_INTERVAL` | `10` | `check_interval` | |
| `MAX_EPISODES_PER_ANIME` | `12` | `max_episodes_per_anime` | |
| `EPISODE_RETRY_LIMIT` | `5` | `episode_retry_limit` | |
| `DELETE_WATCHED_EPISODES` | `true` | `delete_watched_episodes` | `true`/`1` → `true`, anything else → `false` |
| `FORCE_CONFIG_UPDATE` | — (unset) | — | When non-empty, overwrites an existing `config.json` |

> The generated file carries **7 of the ~25 fields** in `Config`. That is safe on creation —
> `LoadConfigs` unmarshals over `getDefaultConfig()`, so the absent fields keep their defaults —
> but `FORCE_CONFIG_UPDATE` rewrites the *whole* file, discarding everything the user set
> through the UI or CLI.

## OS / Path

| Variable | Where | Description |
|----------|-------|-------------|
| `HOME` | `files/filemanager.go`, `logger/logger.go`, `cmd/daemon/main.go` | Linux/macOS: base for `~/.autoAnimeDownloader/` |
| `APPDATA` | same files | Windows: base for `%APPDATA%\AutoAnimeDownloader\` |

## Test-Only

| Variable | Default | Where | Description |
|----------|---------|-------|-------------|
| `DAEMON_URL` | — (unset = skip) | `tests/integration/integration_test.go` | Daemon base URL for integration tests. **Must be set explicitly or every integration test skips** — the suite overwrites the config of whatever daemon answers, so it must never find a developer's daemon by accident. Set to `http://daemon:8091` by `docker-compose.test.yml`. When set but empty/unreachable, tests also skip. The `http://localhost:8091` fallback is only used to build the URL once the gate has passed |
| `TEST_COMPLETED_PATH` | `~/aad-test/library` | `tests/integration/integration_test.go` | `completed_anime_path` the integration tests write. Docker overrides it to `/app/data/aad-test/library` |
| `SCENARIO` | — | `tests/mocks/anilist/`, `tests/mocks/nyaa/` | `"empty"` = mock returns empty data |
| `PORT` | `8080`/`8081` | `tests/mocks/*/mock_server.go` | Listen port for each mock server (Anilist, Nyaa) |
| `TEST_LOGGER_INIT` | — | `logger/logger_test.go` | When empty, auto-inits logger for tests |
