# Development Guide

This guide is for developers who want to contribute to or modify AutoAnimeDownloader.

## Prerequisites

- **Go** 1.24 or later
- **Node.js** 20 or later
- **npm** (comes with Node.js)
- **Git** for version control
- **qBittorrent** with WebUI enabled (for testing)
- **Anilist account** (for testing)

## Project Structure

```
AutoAnimeDownloader/
├── src/
│   ├── cmd/
│   │   ├── daemon/          # Daemon entry point
│   │   └── cli/             # CLI entry point
│   ├── internal/
│   │   ├── api/             # REST API and WebSocket
│   │   ├── daemon/          # Core daemon logic
│   │   ├── files/            # File management
│   │   ├── logger/          # Logging system
│   │   ├── anilist/         # Anilist API client
│   │   ├── nyaa/            # Nyaa scraper
│   │   ├── torrents/         # qBittorrent client
│   │   └── frontend/        # Svelte frontend
│   └── tests/
│       ├── integration/     # Integration tests
│       ├── unit/            # Unit tests
│       └── mocks/           # Mock servers
├── docs/                    # Documentation
├── scripts/                 # Build and utility scripts
├── infra/                   # Service files and installers
└── build/                   # Build outputs
```

## Setting Up Development Environment

### 1. Clone the Repository

```bash
git clone https://github.com/icarosuper/AutoAnimeDownloader.git
cd AutoAnimeDownloader
```

### 2. Install Go Dependencies

```bash
go mod download
```

### 3. Install Frontend Dependencies

```bash
cd src/internal/frontend
npm install
cd ../../..
```

### 4. Build Frontend

The frontend must be built before building the Go code (it's embedded):

```bash
cd src/internal/frontend
npm run build
cd ../../..
```

### 5. Build and Run

```bash
# Build daemon
go build -o AutoAnimeDownloader-daemon ./src/cmd/daemon

# Build CLI
go build -o AutoAnimeDownloader-cli ./src/cmd/cli

# Run daemon
./AutoAnimeDownloader-daemon
```

## Development Workflow

### Running in Development Mode

Set the `ENVIRONMENT` variable to `dev` for development mode:

```bash
ENVIRONMENT=dev ./AutoAnimeDownloader-daemon
```

**Development mode features:**
- Formatted console logs (instead of JSON)
- More verbose logging
- Debug information

### Hot Reloading

For frontend development:

```bash
cd src/internal/frontend
npm run dev
```

This starts Vite dev server. Note: The daemon serves the embedded frontend, so you'll need to rebuild after changes.

### Testing

#### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...

# Run specific package tests
go test ./src/internal/api/...
```

#### Integration Tests

Integration tests require Docker and docker-compose:

```bash
# Start test environment
docker-compose -f docker-compose.test.yml up -d

# Run integration tests
cd src/tests/integration
go test -v ./...

# Or use the script
./scripts/run-integration-tests.sh
```

## Code Style and Conventions

### Go Code

Follow standard Go conventions:

1. **Format code**: Use `gofmt` or `go fmt`
2. **Lint**: Use `golint` or `golangci-lint`
3. **Documentation**: Document exported functions and types
4. **Error handling**: Always handle errors, never ignore them
5. **Naming**: Use descriptive names, follow Go naming conventions

### Frontend Code

1. **Format**: Use Prettier (configured in project)
2. **Lint**: Use ESLint (if configured)
3. **Components**: Keep components small and focused
4. **Styling**: Use Tailwind CSS utility classes

## Architecture Overview

### Daemon

The daemon (`src/cmd/daemon/main.go`) is the main service:

1. Initializes logger
2. Creates file manager
3. Creates state manager
4. Starts API server
5. Starts verification loop

### API Server

The API server (`src/internal/api/`) provides:

- REST endpoints for CLI and WebUI
- WebSocket for real-time updates
- Static file serving for frontend

### Daemon Logic

The daemon logic (`src/internal/daemon/`) handles:

- State management
- Verification loop
- Episode tracking
- Error handling

### File Management

File management (`src/internal/files/`) handles:

- Configuration loading/saving
- Episode tracking
- File system operations

## Adding New Features

### Adding a New API Endpoint

1. Create handler in `src/internal/api/endpoint_*.go`
2. Register route in `src/internal/api/server.go`
3. Add Swagger documentation
4. Write tests
5. Update CLI if needed

### Adding Frontend Features

1. Create component in `src/internal/frontend/src/`
2. Add route if needed
3. Update API client if needed
4. Style with Tailwind CSS
5. Test in browser

### Adding Configuration Options

1. Add field to `src/internal/files/config.go`
2. Update validation
3. Update API endpoints
4. Update frontend form
5. Update CLI commands
6. Update documentation

## Debugging

### Logs

Logs are written to:
- **Linux/Mac**: `~/.autoAnimeDownloader/daemon.log`
- **Windows**: `%APPDATA%\AutoAnimeDownloader\daemon.log`

In development mode, logs also go to console.

### Debugging the Daemon

```bash
# Run with verbose logging
ENVIRONMENT=dev ./AutoAnimeDownloader-daemon

# Check logs
tail -f ~/.autoAnimeDownloader/daemon.log
```

### Debugging the Frontend

1. Open browser DevTools (F12)
2. Check Console for errors
3. Check Network tab for API calls
4. Check WebSocket connection

### Debugging API

Use Swagger UI:
- Access: `http://localhost:8091/swagger/`
- Test endpoints directly
- View API documentation

## Building for Production

### Build Scripts

```bash
# Linux
make build

# Windows
.\scripts\build.ps1
```

### Manual Build

```bash
# 1. Build frontend
cd src/internal/frontend
npm run build
cd ../../..

# 2. Build Go binaries
go build -ldflags="-w -s" -o AutoAnimeDownloader-daemon ./src/cmd/daemon
go build -ldflags="-w -s" -o AutoAnimeDownloader-cli ./src/cmd/cli
```

## Testing

### Writing Tests

#### Unit Tests

Create `*_test.go` files in the same package:

```go
package api

import "testing"

func TestHandler(t *testing.T) {
    // Test implementation
}
```

#### Integration Tests

Create tests in `src/tests/integration/`:

```go
package integration

import "testing"

func TestAPIEndpoint(t *testing.T) {
    // Integration test
}
```

### Test Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Dependencies

### Adding Go Dependencies

```bash
go get <package>
go mod tidy
```

### Adding Frontend Dependencies

```bash
cd src/internal/frontend
npm install <package>
```

## Common Tasks

### Updating Swagger Documentation

1. Update comments in `src/cmd/daemon/main.go`
2. Run: `swag init -g src/cmd/daemon/main.go -o docs`
3. Commit generated files

### Updating Frontend

1. Make changes in `src/internal/frontend/src/`
2. Build: `cd src/internal/frontend && npm run build`
3. Rebuild daemon: `go build ./src/cmd/daemon`

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./src/internal/api/...

# With verbose output
go test -v ./...

# With race detector
go test -race ./...
```

## Troubleshooting

### Build Errors

**Frontend not found:**
- Make sure frontend is built before Go build
- Check `src/internal/frontend/dist/` exists

**Import errors:**
- Run `go mod download`
- Run `go mod tidy`

### Runtime Errors

**Port already in use:**
- Change port: `PORT=:8092 ./AutoAnimeDownloader-daemon`
- Or stop existing daemon

**Permission errors:**
- Check file permissions
- Check directory permissions

## Resources

- [Go Documentation](https://golang.org/doc/)
- [Svelte Documentation](https://svelte.dev/docs)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [Swagger/OpenAPI](https://swagger.io/specification/)

## See Also

- [Contributing Guide](contributing.md) - How to contribute
- [Build Guide](build.md) - Building from source
- [CLI Guide](cli-guide.md) - CLI reference
- [WebUI Guide](webui-guide.md) - WebUI reference

