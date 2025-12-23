# Build Guide

This document describes how to build AutoAnimeDownloader from source.

## Prerequisites

- **Go** 1.24 or later
- **Node.js** 20 or later
- **npm** (comes with Node.js)
- **make** (for Linux builds)
- **zip** (for packaging on Linux)

## Build Process

The build process consists of two critical steps:

1. **Build the frontend** - This must be done FIRST
2. **Build the Go binaries** - This embeds the frontend into the daemon binary

### Why This Order Matters

The frontend must be built before the Go build because the Go code uses `//go:embed` to embed the frontend files directly into the binary. The embed directive requires the files to exist at compile time.

## Building for Linux

### Using the Build Script

The easiest way to build for Linux is using the provided script:

```bash
./scripts/build.sh
```

This will:
1. Build the frontend
2. Build the daemon and CLI for Linux amd64
3. Build the daemon and CLI for Linux arm64
4. Generate SHA256 checksums for all binaries

Output will be in:
- `build/linux-amd64/` - Binaries for x86_64
- `build/linux-arm64/` - Binaries for ARM64

### Manual Build

If you prefer to build manually:

```bash
# Step 1: Build frontend
cd src/internal/frontend
npm ci
npm run build
cd ../../..

# Step 2: Build daemon for Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-amd64/autoanimedownloader-daemon ./src/cmd/daemon

# Step 3: Build CLI for Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-amd64/autoanimedownloader ./src/cmd/cli
```

## Building for Windows

### Using the Build Script

On Windows, use PowerShell:

```powershell
.\scripts\build.ps1
```

This will:
1. Build the frontend
2. Build the daemon and CLI for Windows amd64
3. Generate SHA256 checksums

Output will be in `build/windows-amd64/`

### Manual Build

```powershell
# Step 1: Build frontend
cd src\internal\frontend
npm ci
npm run build
cd ..\..\..

# Step 2: Build daemon
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -a -installsuffix cgo -ldflags="-w -s" -o build\windows-amd64\autoanimedownloader-daemon.exe .\src\cmd\daemon

# Step 3: Build CLI
go build -a -installsuffix cgo -ldflags="-w -s" -o build\windows-amd64\autoanimedownloader.exe .\src\cmd\cli
```

## Cross-Compilation

You can build for any platform from any platform. Just set the appropriate `GOOS` and `GOARCH` environment variables:

- Linux amd64: `GOOS=linux GOARCH=amd64`
- Linux arm64: `GOOS=linux GOARCH=arm64`
- Windows amd64: `GOOS=windows GOARCH=amd64`

## Build Flags Explained

- `CGO_ENABLED=0` - Disables CGO for static binaries
- `-a` - Force rebuilding of packages
- `-installsuffix cgo` - Use a different installation directory
- `-ldflags="-w -s"` - Strip debug information and symbol table (smaller binaries)

## Testing Builds Locally

After building, you can test the daemon:

```bash
# Linux
./build/linux-amd64/autoanimedownloader-daemon

# Windows
.\build\windows-amd64\autoanimedownloader-daemon.exe
```

The daemon will start on port 8091 by default. Access the web UI at http://localhost:8091

## Frontend Embedding

The frontend is embedded directly into the daemon binary using Go's `embed` package. This means:

- **No external files needed** - The frontend is part of the binary
- **Single executable** - Everything is self-contained
- **Works on all platforms** - Same approach for Windows and Linux

The embed is configured in `src/internal/frontend/embed.go`:

```go
//go:embed dist/*
var DistFS embed.FS
```

This embeds all files from the `dist/` directory (created by `npm run build`) into the binary.

## Troubleshooting

### "Frontend dist directory not found"

This means the frontend wasn't built before the Go build. Run `npm run build` in `src/internal/frontend/` first.

### "Cannot find package frontend"

Make sure you're building from the project root directory, not from within `src/cmd/daemon/`.

### Build fails with "no such file or directory"

Ensure the frontend build completed successfully and created the `dist/` directory with files inside.

