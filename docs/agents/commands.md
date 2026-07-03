# Commands

## Backend (Go)

For test commands and mock patterns, see [Testing](testing.md).

```bash
# Generate Swagger docs
swag init -g src/cmd/daemon/main.go -o docs/swagger

# Build manually (frontend must be built first)
cd src/internal/frontend && bun install --frozen-lockfile && bun run build && cd ../../..
go build -o build/autoanimedownloader-daemon ./src/cmd/daemon
go build -o build/autoanimedownloader ./src/cmd/cli
```

## Frontend (Svelte + Vite + Bun)

```bash
cd src/internal/frontend
bun install
bun run dev    # dev server (proxies API to localhost:8091)
bun run build  # production build (output embedded in Go binary)
bun run check  # TypeScript type checking
bun run i18n:compile  # recompile i18n messages (paraglide-js)
```

## Full Platform Builds (Docker required)

```bash
make build                          # build all platforms
make build PLATFORM=linuxamd64      # linux x86_64
make build PLATFORM=linuxarm64      # linux ARM64
make build PLATFORM=windows         # windows amd64

make package PLATFORM=linuxamd64    # package already-built binaries
make release PLATFORM=linuxamd64    # build + package in one step

# The actual Docker logic lives in scripts/build.sh <platform> <version>
# Makefile delegates to it — don't add build logic directly to Makefile targets

# Linux installation
make install    # install daemon, CLI, and systemd service
make uninstall
```

## Running the Daemon

```bash
ENVIRONMENT=dev ./autoanimedownloader-daemon   # formatted logs
PORT=:8092 ./autoanimedownloader-daemon        # custom port (default 8091)
```

## Debugging a specific anime

`--debug-anime <anilistId>` on the daemon binary runs a one-shot pass that fetches the anime from AniList, runs the real search/match pipeline against Nyaa, and logs everything (raw Nyaa rows, matched torrents, per-episode results). No download happens, no daemon or qBittorrent needs to be running. `<anilistId>` is the AniList MediaList ID (same ID used in `/api/v1/animes/{id}/episodes`).

```bash
make debug-anime ID=123
# equivalent to:
go run ./src/cmd/daemon --debug-anime 123
```

Output lands in `.debug_<anilistId>_<N>/` in the current directory (`N` increments on each run against the same anime, so before/after comparisons are possible):
- `debug.jsonl` — full structured trace (raw Nyaa rows, matched torrent names, skip reasons), one JSON object per line
- `summary.json` — per-episode quick view (`would_search`, `magnets_found`)

Nothing is written to `~/.autoAnimeDownloader` during a debug run. See [Troubleshooting Downloads](troubleshooting-downloads.md) Step 0 for how to read the output.

## CLI Notable Flags

```bash
# logs command — flags mirror the web UI Logs page
autoanimedownloader logs                        # last 1000 lines, colorized
autoanimedownloader logs -n 500                 # last 500 lines
autoanimedownloader logs --level error          # filter by level (all/debug/info/warn/error)
autoanimedownloader logs --search "nyaa"        # text search (-q alias)
autoanimedownloader logs --level warn -q retry  # combine filters

# global flags
autoanimedownloader --json logs                 # JSON output (returns lines/total/shown)
NO_COLOR=1 autoanimedownloader logs             # disable ANSI colors
```
