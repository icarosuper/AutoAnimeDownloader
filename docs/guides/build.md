# Build Guide

How to build AutoAnimeDownloader from source.

## Prerequisites

- **Go** 1.24+
- **Bun** (frontend package manager/build tool)
- **Docker** — only needed for the Makefile's cross-platform builds
- **make** (Linux/macOS/WSL)

## Why the frontend builds first

The Go daemon embeds the frontend with `//go:embed dist/*` (`src/internal/frontend/embed.go`). The `dist/` directory must exist before `go build` runs, so the frontend always builds before the Go binaries.

## Recommended: Makefile (Linux/macOS/WSL)

```bash
make build                       # build all platforms (linuxamd64, linuxarm64, windows) via Docker
make build PLATFORM=linuxamd64   # build one platform
make package PLATFORM=linuxamd64 # zip + sha256 an already-built platform
make release PLATFORM=linuxamd64 # build + package in one step
make help                        # full target list (test, dev, clear-data, debug-anime, ...)
```

Each platform builds inside Docker (`scripts/build.sh <platform> <version>`, using `docker/Dockerfile.build.{amd64,arm64,windows}`), so cross-compilation works from any host without a local cross toolchain. `docker buildx` is used when available (required for ARM64); otherwise it falls back to a classic `docker build`.

Output lands in `build/linux-amd64/`, `build/linux-arm64/`, `build/windows-amd64/`; packaged zips/exe go to `build/packages/`.

`VERSION` defaults to `git describe --tags --always --dirty`; override with `make build VERSION=1.2.3`.

## Windows native build (no Docker)

```powershell
.\scripts\build.ps1
```

Builds directly on Windows with the tray icon. For cross-platform builds from Windows, use WSL or Docker Desktop with `make build`.

## Manual build (single platform, no Docker)

```bash
# 1. Build the frontend
cd src/internal/frontend && bun install && bun run build && cd ../../..

# 2. Build the binaries
go build -ldflags="-w -s" -o build/autoanimedownloader-daemon ./src/cmd/daemon
go build -ldflags="-w -s" -o build/autoanimedownloader ./src/cmd/cli
```

Binaries built this way have no tray icon (that requires the Docker cross-build images) — fine for local testing.

## Troubleshooting

**"pattern dist/\*: no matching files found"** — the frontend wasn't built first. Run `bun run build` in `src/internal/frontend/` before `go build`.

**Docker build fails** — make sure Docker is installed and running. ARM64 builds require `docker buildx`; install it with `docker buildx install` if missing.
