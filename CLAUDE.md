# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repo.

## What Is This

AutoAnimeDownloader (AAD) automates anime episode downloads. It syncs your Anilist watching list, searches Nyaa for new episodes matching your preferences (resolution, subtitle group, etc.), and sends torrents to qBittorrent — all unattended. A built-in Svelte web UI lets you monitor activity, configure rules, and manage downloads from the browser.

## Project Overview

AutoAnimeDownloader = Go daemon. Auto-downloads anime episodes. Integrates Anilist (watch list), Nyaa (torrent scraper), qBittorrent. Has embedded Svelte web UI + REST API.

## Important — After Any Change

1. **Run tests**: `go test ./...`
2. **Update docs**: if you changed endpoints, config fields, env vars, or patterns → update the relevant doc in `docs/agents/`
3. **Regenerate Swagger**: if API changed → `swag init -g src/cmd/daemon/main.go -o docs/swagger`

## Docs

- [Architecture](docs/agents/architecture.md) — components, data flow, config files, API endpoints, version injection
- [Conventions](docs/agents/conventions.md) — coding patterns, handler structure, error handling, checklists for adding endpoints/config
- [Config Reference](docs/agents/config.md) — all `config.json` fields, types, defaults, validation
- [Environment Variables](docs/agents/environment.md) — all env vars (app, service overrides, test-only)
- [Testing](docs/agents/testing.md) — test commands, mock patterns (var swap, interface injection, in-memory FS)
- [Commands](docs/agents/commands.md) — build, test, run commands for backend and frontend
- [TODO](docs/agents/TODO.md) — planned features and known issues
