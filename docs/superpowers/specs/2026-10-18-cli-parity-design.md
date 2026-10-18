# CLI parity

**Date:** 2026-10-18
**Status:** executed
**Origin:** `v2.6.0` in [TODO.md](../../TODO.md) — "Bring the CLI to parity with the webApp's features"
**Expected scope:** `src/cmd/cli/` (split by resource, new commands), `src/internal/api/client.go`
(one method per endpoint the CLI still lacks), `docs/guides/cli.md` (rewrite), conventions /
`CLAUDE.md` (standing rule), `docs/agents/architecture.md` (CLI notes that the code cannot answer).

---

## The idea in one sentence

An AI — or a human who does not open the web UI — drives the daemon through the CLI: every API
action is a nested command, `--json` is the machine contract, and destructive work takes explicit
flags instead of a prompt.

## Why this shape

Linux CLIs nest **resource then action** (`ip link show`, `systemctl start`, `git remote add`).
This repo already does that (`loop start`, `config get`). `urfave/cli` is the library in use.

The consumer that matters for this release is an agent. Agents run `--help`, pass `--json`, and
hang forever on stdin. The command tree *is* the map of the API; a tutorial in the data dir or in
the web UI would be a second copy that goes stale.

WebSocket and frontend-only work (priority presets live in the frontend, [decisions.md #70](../../agents/decisions.md#70-resolução-canonicaliza-no-extrator-e-os-presets-de-codec-são-reordenação-no-frontend)) stay out.

## Agent contract

Global flags already exist and stay: `--endpoint` / `--url` (env `AAD_API_ENDPOINT`), `--json`,
`--verbose` / `-v`.

| Rule | Behavior |
|---|---|
| `--json` | stdout is the API `data` value (no `{success,data,error}` envelope). stderr is empty on success |
| default (no `--json`) | human table / one-line message, as today |
| success | exit 0 |
| API / daemon error | exit 1, stderr text; with `--json`, stdout is `{"error":{"code":"…","message":"…"}}` using the API's `code` |
| destructive without `--yes` | exit 2, code `CONFIRMATION_REQUIRED`, message lists the flags. No stdin, no prompt, no TTY check |
| daemon unreachable | exit 1, code `DAEMON_UNREACHABLE`. `report` and `report crash-ack` never take this path |

Destructive commands (require `--yes`):

- `animes remove`
- `animes redownload`
- `animes delete`
- `animes replace`
- `animes replace-episode`
- `torrents delete`

Optional flags on those commands map **1:1** onto the API query params. Absent means the API
default (`false`):

- `torrents delete`: `--keep-data`, `--block`
- `animes remove`: `--delete-episodes`

`--help` on every command states `Usage`, `ArgsUsage`, flags and those defaults. A bare
`autoanimedownloader` prints the tree. That is the contract for the agent.

The human tutorial is the existing [cli.md](../../guides/cli.md), rewritten in the same change.
Nothing new is written into `~/.autoAnimeDownloader`, the library folder, or a web UI screen.

## Command tree

Existing commands stay. `episodes` (root) is **removed**: it calls `GET /api/v1/episodes`, which
no longer exists (`endpoint_episodes_test.go` already says so). Listing moves to
`animes episodes <id>`.

`animes` keeps its current Action (list) and gains subcommands. `urfave/cli` allows both.

| Command | API / source | Notes |
|---|---|---|
| `start` / `stop` | process (`internal/cli`) | unchanged |
| `loop start` / `loop stop` | `POST /api/v1/daemon/start\|stop` | unchanged |
| `open` | browser | unchanged |
| `status` | `GET /api/v1/status` | print the full `StatusResponse` (disk, anilist health, `pending_crash_at`), not four fields |
| `last-check` | `GET /api/v1/last-check` | new |
| `check` | `POST /api/v1/check` | unchanged |
| `config get` / `config set` | `GET/PUT /api/v1/config` | `get` prints **every** JSON key (the table today shows 7). `set` already passthroughs |
| `priorities defaults` | `GET /api/v1/config/priorities/defaults` | new |
| `logs` | `GET /api/v1/logs` | unchanged |
| `report` | `diagnostics.Build(datadir.Path(), opts)` | disk only; daemon may be dead. Flags: `--output PATH`, `--no-titles`, `--error`, `--context`, `--anime` |
| `report crash-ack` | `diagnostics.ArchivePendingCrash(datadir.Path())` | disk only. `/status` reads `crash-pending.log` mtime, so the UI clears on the next poll |
| `animes` | `GET /api/v1/animes` | list, already exists; table columns follow `AnimeInfo` |
| `animes search <q>` | `GET /api/v1/anilist/search?q=&include_unreleased=` | `--unreleased`. Term shorter than 3 chars: empty list, not an error (same as the API) |
| `animes add <media_id>` | `POST /api/v1/standalone-animes` | body `{"media_id": N}` |
| `animes remove <id> --yes` | `DELETE /api/v1/standalone-animes/{id}` | `--delete-episodes` |
| `animes settings <id>` | `GET /api/v1/animes/{id}/settings` | |
| `animes settings <id> progress <n>` | `PUT /api/v1/animes/{id}/settings` | partial body `{"progress": n}` |
| `animes episodes <id>` | `GET /api/v1/animes/{id}/episodes` | replaces root `episodes` |
| `animes download <id> <ep>` | `POST .../episodes/{ep}/download` | |
| `animes redownload <id> <ep> --yes` | `POST .../redownload` | |
| `animes delete <id> <ep> --yes` | `DELETE .../episodes/{ep}` | |
| `animes release <id> <ep>` | `POST .../release` | not destructive; no `--yes` |
| `animes replace <id> --magnet URI --yes` | `POST /api/v1/animes/{id}/replace` | whole anime |
| `animes replace-episode <id> <ep> --magnet URI --yes` | `POST .../episodes/{ep}/replace` | |
| `torrents` | `GET /api/v1/torrents` | |
| `torrents files <hash>` | `GET /api/v1/torrents/{hash}/files` | |
| `torrents pause\|resume\|announce <hash>` | `POST /api/v1/torrents/{hash}/…` | |
| `torrents prioritize <hash> [hash…]` | one hash → `POST /{hash}/prioritize`; two or more → `POST /torrents/prioritize` with `{"hashes":[…]}` in the order given | |
| `torrents delete <hash> --yes` | `DELETE /api/v1/torrents/{hash}` | `--keep-data` `--block` |
| `notifications test <name>` | `POST /api/v1/notifications/webhooks/{name}/test` | |

The agent composes primitives: “download a new anime” is `animes search` then `animes add <id>`;
“stop an anime” is `torrents` plus `pause` / `delete` / `animes remove`, depending on intent.

## `report` reads the disk

`diagnostics.Build` already refuses to talk to the daemon ([decisions.md #102](../../agents/decisions.md#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file)).
The CLI calls it with `datadir.Path()` and writes `Report.Bundle` to `--output` or stdout.
`--json` prints `{bundle, environment}` (same shape as `GET /api/v1/diagnostics`).

`report crash-ack` only calls `ArchivePendingCrash`. It does not `DELETE /diagnostics/crash`: that
handler is the same disk move, and `pending_crash_at` is computed from the file on read.

## Client

`src/internal/api/client.go` is the HTTP client the CLI uses. Today it has status, config, animes,
the removed episodes list, check, loop start/stop.

Add one method per remaining endpoint the tree uses. `parseResponse` already unwraps the envelope
and surfaces `Error.Code` + `Message`; keep that, and make the CLI JSON error use those two
fields. Portuguese comments in this file are translated in the same change.

`GetEpisodes` (`GET /api/v1/episodes`) is deleted with the root `episodes` command.

`report` / `report crash-ack` do not go through `Client`.

## Files in `src/cmd/cli/`

`main.go` is already past 700 lines. Split by resource, same package `main` (existing
`main_test.go` keeps testing unexported helpers):

| File | Holds |
|---|---|
| `main.go` | `cli.App`, global flags, `start` / `stop` / `open` / `loop` |
| `config.go` | `config get` / `set` (moved), `priorities defaults` |
| `status.go` | `status`, `last-check`, `check` |
| `animes.go` | list, search, add/remove, settings, episodes, download/redownload/delete/release/replace |
| `torrents.go` | list, files, pause/resume/announce/prioritize/delete |
| `logs.go` | `logs` (moved) |
| `report.go` | `report`, `report crash-ack` |
| `notifications.go` | `notifications test` |

No new package. `internal/cli` stays process lifecycle (systemd user unit / PID file).

## `--yes` is CLI-only

The API does not grow a confirmation parameter. The web UI keeps its dialog. The CLI refuses
without `--yes` and then calls the same endpoints with the same query defaults
(`parseBoolQueryParam`: missing → `false`). That mismatch is intentional and gets
[decisions.md](../../agents/decisions.md) **#104** when implementing.

## Standing rule after this ships

A new API endpoint, or a web-app action that hits the API, ships with its CLI command in the
**same change**: client method, nested command, `--help` text, `docs/guides/cli.md`. Frontend-only
work (layout, i18n, presets that live in the frontend, WebSocket) does not. A new `config.json`
field is already settable through `config set`; no new command.

Recorded in [conventions.md](../../agents/conventions.md) (endpoint checklist) and [CLAUDE.md](../../../CLAUDE.md)
(after any change).

## Testing

Package `main` tests in `src/cmd/cli/main_test.go` (and sibling `*_test.go` next to the new files):

- `--yes` gate: each destructive command without the flag returns `CONFIRMATION_REQUIRED` and does
  not call the client
- `config get` table/JSON contains every key `files.Config` serializes (the existing
  `TestMatchConfigKey_ResolveTodaChaveDaConfig` stays; add the print-all-keys assertion)
- `animes search` with a 2-character term: empty list, exit 0
- `torrents prioritize` with one hash vs two hashes hits the matching client method
- `report` with a temp dir and no daemon: `Build` output is written; `crash-ack` on a missing
  pending file is success (same as `ArchivePendingCrash`)

`Client` methods: `httptest.Server` in `src/internal/api/client_test.go` (new), one happy path per
new method, plus a 409 with `code` propagated.

`go test ./...` after the change. No new integration job: the CLI talks to the same API the
existing Docker suite already hits.

Portuguese identifiers and comments in the CLI tests (`nao consegui serializar`,
`TestMatchConfigKey_ResolveTodaChaveDaConfig`) are translated in the same change.

## Docs to update when implementing

- `docs/guides/cli.md` — rewrite as the human tutorial of this tree; drop `episodes` at the root
  and the 7-key `config get`
- `docs/agents/architecture.md` — CLI reads `diagnostics` from disk; `GetEpisodes` is gone;
  `--yes` is CLI-only (pointer to #104)
- `docs/agents/commands.md` — CLI flags section covers `--yes` / `--json` / `--keep-data`
- `docs/agents/conventions.md` — already updated in the design change (endpoint checklist)
- `docs/agents/testing.md` — one line: CLI tests live next to `cmd/cli`, client tests in `api/`
- `docs/TODO.md` — this spec
- `decisions.md` **#104**: `--yes` is a CLI gate; the API keeps its query defaults

## Not doing

- WebSocket
- frontend priority presets as commands (`config set priorities '…'` is enough; `priorities
  defaults` is the read)
- interactive prompts, `--yes` defaulting to on when stdin is not a TTY
- a tutorial in the data dir, the library, or the web UI
- changing API defaults for `keep_data` / `block` / `delete_episodes` to match the web dialog
- shipping the CLI inside the Windows package (known gap, `architecture.md`; out of this release)
- a `GET /api/v1/diagnostics` client method used by `report` (disk path is the point)
