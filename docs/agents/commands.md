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

# Linux installation
make install    # install daemon, CLI, and systemd service
make uninstall
```

## Running the Daemon

```bash
ENVIRONMENT=dev ./autoanimedownloader-daemon   # formatted logs
PORT=:8092 ./autoanimedownloader-daemon        # custom port (default 8091)
```
