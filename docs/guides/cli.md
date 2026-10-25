# CLI Guide

The CLI (`autoanimedownloader`) drives the daemon over the REST API. A bare
`autoanimedownloader` prints the command tree. `--help` on every command states
`Usage`, `ArgsUsage`, flags and defaults.

## Global flags

| Flag | Env | Behavior |
|---|---|---|
| `--endpoint` / `--url` | `AAD_API_ENDPOINT` | API URL (default `http://localhost:8091`) |
| `--json` | | stdout is the API `data` value (no `{success,data,error}` envelope). stderr is empty on success |
| `--verbose` / `-v` | | verbose output. Version is `--version` |

Put global flags before the command: `autoanimedownloader --json status`.

Default output (no `--json`) is a human table or a one-line message.

| Outcome | Exit | Output |
|---|---|---|
| success | 0 | table / message; with `--json`, the API data |
| API / daemon error | 1 | stderr text; with `--json`, stdout `{"error":{"code":"…","message":"…"}}` using the API's `code` |
| destructive without `--yes` | 2 | code `CONFIRMATION_REQUIRED`, message lists the flags. No stdin, no prompt |
| daemon unreachable | 1 | code `DAEMON_UNREACHABLE`. `report` and `report crash-ack` never take this path |

## Process

```bash
autoanimedownloader start
autoanimedownloader stop
autoanimedownloader open
```

`start` / `stop` manage the daemon process (systemd user unit or PID file). `open` opens the web UI.

## Loop

```bash
autoanimedownloader loop start
autoanimedownloader loop stop
```

Starts or stops the verification loop on the running daemon.

## Status

```bash
autoanimedownloader status
autoanimedownloader --json status
autoanimedownloader last-check
autoanimedownloader check
```

`status` prints the full payload: daemon state, last check, disk, AniList health, pending crash.
`last-check` is why the last automatic pass did not download. `check` triggers a pass now.

## Configuration

```bash
autoanimedownloader config get
autoanimedownloader config set anilist_usernames me,other
autoanimedownloader config set check_interval 15
autoanimedownloader config set completed_anime_path "/mnt/Anime, Series"
autoanimedownloader config set notifications '{"webhooks":[],"batch_window_seconds":30}'
autoanimedownloader priorities defaults
```

`config get` prints **every** JSON key `config.json` serializes. `config set` accepts any of those
keys by JSON name (case and underscores ignored). A key that already holds a list accepts
comma-separated values; anything else is JSON when it parses, otherwise a string.

`priorities defaults` is the default torrent priority lists. To change yours: `config set priorities '…'`.

## Animes

```bash
autoanimedownloader animes
autoanimedownloader animes search "frieren"
autoanimedownloader animes search "foo" --unreleased
autoanimedownloader animes candidates 154587
autoanimedownloader animes candidates 154587 --query "frieren s01"
autoanimedownloader animes add 154587
autoanimedownloader animes add 154587 --magnet "magnet:?xt=…"
autoanimedownloader animes remove 154587 --yes
autoanimedownloader animes remove 154587 --yes --delete-episodes
autoanimedownloader animes settings 154587
autoanimedownloader animes settings 154587 progress 12
autoanimedownloader animes episodes 154587
autoanimedownloader animes download 154587 3
autoanimedownloader animes redownload 154587 3 --yes
autoanimedownloader animes delete 154587 3 --yes
autoanimedownloader animes release 154587 3
autoanimedownloader animes replace 154587 --magnet "magnet:?xt=…" --yes
autoanimedownloader animes replace-episode 154587 3 --magnet "magnet:?xt=…" --yes
```

`animes` lists tracked titles (columns follow `AnimeInfo`). `search` is AniList; a term shorter than
3 characters returns an empty list, not an error. `candidates` is the ranked whole-title list the
picker shows (empty `items` is success, not an error). `--query` is a Nyaa `q` and skips title
variants and season/part. There is no command that prints a Nyaa description — `nyaa_id` in the
JSON is the terminal equivalent. `add --magnet` skips auto-pick. Flags may follow the id (`animes add 154587 --magnet "…"` is
the same as flags-first). `add` / `remove` are standalone
animes. `--delete-episodes` defaults to false (files stay, marked manually managed).

`release` is not destructive and does not take `--yes`.

Download a new anime: `animes search` then `animes add <media_id>`.

## Torrents

```bash
autoanimedownloader torrents
autoanimedownloader torrents files HASH
autoanimedownloader torrents pause HASH
autoanimedownloader torrents resume HASH
autoanimedownloader torrents announce HASH
autoanimedownloader torrents prioritize HASH
autoanimedownloader torrents prioritize HASH1 HASH2
autoanimedownloader torrents delete HASH --yes
autoanimedownloader torrents delete HASH --yes --keep-data --block
```

One hash on `prioritize` hits `POST /torrents/{hash}/prioritize`. Two or more hits
`POST /torrents/prioritize` with `{"hashes":[…]}` in the order given.

`--keep-data` and `--block` default to false.

## Logs

```bash
autoanimedownloader logs
autoanimedownloader logs --lines 100
autoanimedownloader logs --level error
autoanimedownloader logs --search "nyaa"
NO_COLOR=1 autoanimedownloader logs
```

Reads the daemon log file (last 1000 lines by default).

## Report

```bash
autoanimedownloader report
autoanimedownloader report --output /tmp/aad-report.txt
autoanimedownloader --json report --no-titles --error "…" --context "…" --anime 154587
autoanimedownloader report crash-ack
```

Reads the data dir on disk. The daemon may be dead. `--json` prints `{bundle, environment}`.
`crash-ack` archives `crash-pending.log`; `/status` then clears `pending_crash_at` on the next poll.

## Notifications

```bash
autoanimedownloader notifications test discord
```

Fires a test payload at the webhook named in config.

## Destructive flags

These require `--yes`. Optional flags map 1:1 onto API query params; absent means `false`:

| Command | Extra flags |
|---|---|
| `animes remove` | `--delete-episodes` |
| `animes redownload` | |
| `animes delete` | |
| `animes replace` | `--magnet` (required) |
| `animes replace-episode` | `--magnet` (required) |
| `torrents delete` | `--keep-data` `--block` |

## Examples

```bash
autoanimedownloader start
autoanimedownloader config set anilist_usernames myusername
autoanimedownloader config set completed_anime_path /path/to/library
autoanimedownloader loop start
autoanimedownloader --json animes
autoanimedownloader animes search "sousou no frieren"
autoanimedownloader animes add 154587
autoanimedownloader check
autoanimedownloader logs --level warn -q retry
```

## Troubleshooting

**Command not found.** Install it on `PATH` (`~/.local/bin/` on Linux).

**`DAEMON_UNREACHABLE`.** `autoanimedownloader start`, then retry. `report` still works.

**`CONFIRMATION_REQUIRED`.** Pass `--yes` (and the optional flags listed in the message).

## See also

- [WebUI Guide](webui.md)
- [Development Guide](development.md)
