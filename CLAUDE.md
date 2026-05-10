# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repo.

## Project Overview

AutoAnimeDownloader = Go daemon. Auto-downloads anime episodes. Integrates Anilist (watch list), Nyaa (torrent scraper), qBittorrent. Has embedded Svelte web UI + REST API.

## Important — After Any Change

1. **Run tests**: `go test ./...`
2. **Update docs**: if you changed endpoints, config fields, env vars, patterns, or added/removed files → update the relevant doc in `docs/agents/`
3. **Regenerate Swagger**: if API changed → `swag init -g src/cmd/daemon/main.go -o docs/swagger`

## Docs

- [Architecture](docs/agents/architecture.md) — read when navigating code or adding features; maps all packages, symbols, data flow, API endpoints
- [Rules](docs/agents/rules.md) — read when adding/removing files or unsure about a convention
- [Conventions](docs/agents/conventions.md) — read when writing Go code; error handling, handler pattern, naming, checklists for new endpoints/config fields
- [Config Reference](docs/agents/config.md) — read when touching `config.json` fields
- [Environment Variables](docs/agents/environment.md) — read when touching env vars or dev setup
- [Testing](docs/agents/testing.md) — read when writing or running tests; mock patterns
- [Commands](docs/agents/commands.md) — read when building or running the daemon/frontend
- [Decisions](docs/agents/decisions.md) — read before "fixing" unusual patterns; documents intentional architectural choices
