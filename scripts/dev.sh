#!/usr/bin/env bash
set -e

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT/src/internal/frontend"
BUILD_DIR="$ROOT/build"

echo "==> Building frontend..."
cd "$FRONTEND_DIR"
bun install --frozen-lockfile
bun run build

echo "==> Building binaries..."
cd "$ROOT"
go build -o "$BUILD_DIR/autoanimedownloader-daemon" ./src/cmd/daemon
go build -o "$BUILD_DIR/autoanimedownloader" ./src/cmd/cli

echo "==> Starting daemon..."
"$BUILD_DIR/autoanimedownloader" start
