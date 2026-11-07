#!/usr/bin/env bash
# Runs a throwaway daemon for poking at the real web UI (by hand or with a browser agent) without
# touching the real installation:
#   - HOME points at .sandbox/home, so ~/.autoAnimeDownloader is a scratch copy, wiped on
#     every start (config, state, torrents, daemon.log all live there)
#   - its own port (SANDBOX_PORT, default 8092), so it runs next to a real daemon on 8091
#   - AniList and Nyaa are the mocks in src/tests/mocks — no real network for the watch list or
#     the search, and the mock magnet is not a real torrent, so nothing is ever downloaded
#   - DISPLAY/WAYLAND_DISPLAY unset: no tray icon, and no browser tab opened on the desktop
#   - a config.json is written before boot, so the first-boot onboarding is skipped
# Runs in the foreground; Ctrl+C — or `kill "$(cat .sandbox/sandbox.pid)"` from another shell —
# stops the daemon and both mocks. From another shell it has to be TERM, not INT: a script started
# in the background with `&` inherits SIGINT as ignored, and a non-interactive bash can't trap it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SANDBOX="$ROOT/.sandbox"
PORT="${SANDBOX_PORT:-8092}"
ANILIST_PORT="${SANDBOX_ANILIST_PORT:-18080}"
NYAA_PORT="${SANDBOX_NYAA_PORT:-18081}"

for p in "$PORT" "$ANILIST_PORT" "$NYAA_PORT"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
    echo "port $p is already in use — stop whatever holds it or set SANDBOX_PORT/SANDBOX_ANILIST_PORT/SANDBOX_NYAA_PORT" >&2
    exit 1
  fi
done

rm -rf "$SANDBOX"
mkdir -p "$SANDBOX/bin" "$SANDBOX/home/.autoAnimeDownloader" "$SANDBOX/library"

if [[ "${SANDBOX_SKIP_FRONTEND:-}" != "1" ]]; then
  echo "==> Building frontend..."
  (cd "$ROOT/src/internal/frontend" && bun install --frozen-lockfile >/dev/null && bun run build >/dev/null)
fi

echo "==> Building daemon and mocks..."
(cd "$ROOT" && go build -o "$SANDBOX/bin/daemon" ./src/cmd/daemon)
(cd "$ROOT/src/tests/mocks/anilist" && go build -o "$SANDBOX/bin/mock-anilist" .)
(cd "$ROOT/src/tests/mocks/nyaa" && go build -o "$SANDBOX/bin/mock-nyaa" .)

cat > "$SANDBOX/home/.autoAnimeDownloader/config.json" <<EOF
{
  "anilist_usernames": ["sandbox"],
  "completed_anime_path": "$SANDBOX/library",
  "min_free_disk_gb": 0,
  "min_seeders": 0
}
EOF

pids=()
cleanup() {
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -f "$SANDBOX/sandbox.pid"
}
trap cleanup EXIT INT TERM
echo $$ > "$SANDBOX/sandbox.pid"

PORT="$ANILIST_PORT" "$SANDBOX/bin/mock-anilist" >"$SANDBOX/mock-anilist.log" 2>&1 &
pids+=($!)
PORT="$NYAA_PORT" "$SANDBOX/bin/mock-nyaa" >"$SANDBOX/mock-nyaa.log" 2>&1 &
pids+=($!)

env -u DISPLAY -u WAYLAND_DISPLAY \
  HOME="$SANDBOX/home" \
  PORT=":$PORT" \
  ENVIRONMENT=dev \
  ANILIST_API_URL="http://127.0.0.1:$ANILIST_PORT" \
  NYAA_URL="http://127.0.0.1:$NYAA_PORT" \
  "$SANDBOX/bin/daemon" >"$SANDBOX/daemon.out" 2>&1 &
pids+=($!)

for _ in $(seq 1 50); do
  if (exec 3<>"/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then break; fi
  if ! kill -0 "${pids[2]}" 2>/dev/null; then
    echo "daemon exited during startup — see $SANDBOX/daemon.out" >&2
    exit 1
  fi
  sleep 0.2
done

cat <<EOF

Sandbox up: http://localhost:$PORT
  data dir   $SANDBOX/home/.autoAnimeDownloader
  logs       $SANDBOX/daemon.out, mock-anilist.log, mock-nyaa.log
  CLI        AAD_API_ENDPOINT=http://localhost:$PORT go run ./src/cmd/cli <command>
Ctrl+C to stop.
EOF

wait "${pids[2]}"
