# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repo.

## Project Overview

AutoAnimeDownloader = Go daemon. Auto-downloads anime episodes. Integrates Anilist (watch list), Nyaa (torrent scraper), and an embedded BitTorrent client (github.com/cenkalti/rain/v2 — no external qBittorrent). Has embedded Svelte web UI + REST API.

## Important — After Any Change

1. **Run tests**: `go test ./...`
2. **Update docs**: if you changed endpoints, config fields, env vars, patterns, or added/removed files → update the relevant doc in `docs/agents/`; if you introduced a non-obvious or unusual pattern → add an entry to `decisions.md`
   - `architecture.md` documents what **the code can't answer by itself**: flow between files, invariants that hurt to break, negative facts ("there is no `daemon.go`"), disambiguation of similar names, and the pointer to the decision. **Don't** list symbols with a one-line purpose — `grep '^func '` answers that for free and always up to date; a hand-written symbol table is a copy that goes stale. A new file only gets text if it brings one of those five; deleted file → deleted text. The question it answers is "which file do I open and what bites me there", not "which functions exist"
   - New entry in `decisions.md` → next number (the highest today is **#104**, and entries are not in numeric order in the file) + one line in the Index at the top. When referencing a decision, always use the anchor: `[decisions.md #43](decisions.md#43-a-identidade-de-um-anime-é-mediaid-status-é-uma-pergunta-por-conta-download--or-deleção--and)`
3. **CLI parity**: a new API endpoint, or a web-app action that hits the API, ships with its CLI command in the **same change** (client method, nested command, `--help`, `docs/guides/cli.md`). Checklist in [conventions.md](docs/agents/conventions.md). Frontend-only work (layout, i18n, presets that live in the frontend, WebSocket) does not. A new `config.json` field is already settable through `config set`.
4. **NEVER COMMIT OR PUSH WITHOUT PERMISSION**: the user will do that themselves

## Important — Everything in English

Since 2026-10-12, everything in the repo is in **English**: code (identifiers, comments, log and
error messages), file and directory names, documentation, specs, TODO and commit messages. The only
exception is the Portuguese **i18n translation** of the web UI, which stays.

Found something in Portuguese while working — in a file you're touching or one next to it? **Translate
it to English right away, without asking**, and tell the user what you changed (`file:line` or file
name) at the end. This overrides the "Bad code in the way" rule below for this one case. Renaming a
file or translating a heading changes links and anchors (e.g. the `decisions.md` anchors) — update
every reference in the same change.

## Commit style

Conventional Commits, in **English** (since 2026-10-12 — earlier history is in Portuguese and stays
as is), subject line only — no body, no `Co-Authored-By`, no trailing period.

`type: imperative verb + what changed` — all lowercase after `type:`.

Types in use: `feat`, `fix`, `docs`, `refactor`, `test`, `chore` (`!` after the type for a breaking
change, e.g. `chore!: drop save_path migration`).

The subject states the behavior, not the file: describe the rule that now holds, and when an
"instead of" explains the change, it belongs in the subject.

```
feat: pick pack by window coverage instead of part marker
fix: serve stale customLists cache when the gate refuses
docs: record that packs without a range skip the coverage check
```

## Branch

The default is to commit straight to `master` — don't create a branch on your own. Only for a larger
feature that will take several commits, **ask first** whether the user wants a branch.

## Important — Bad code in the way

Found bad code **near** what you're working on — poorly written, deprecated, unused, outdated,
unoptimized, duplicated, a comment lying about the code — **ask before acting**. Always the three
options, and the user picks:

1. **fix it now**
2. **just leave a TODO** (in the code, next to it)
3. **ignore it**

Never decide on any of the three by yourself — neither "I'll fix it in passing because it's quick"
nor "I'll leave it alone because it's out of scope". Say what it is, where it is (`file:line`) and why
it's bad, in one or two lines, and ask. This applies to what you find in passing; what the user's
request already covers isn't "in passing", it's the job. (Portuguese text is the exception — see
"Everything in English".)

## Docs

- [Architecture](docs/agents/architecture.md) — read when navigating code or adding features; update when adding packages, symbols, or API endpoints; maps all packages, symbols, data flow, API endpoints
- [Conventions](docs/agents/conventions.md) — read when writing Go code; error handling, handler pattern, naming, checklists for new endpoints/config fields/CLI commands
- [Config Reference](docs/agents/config.md) — read when touching `config.json` fields
- [Environment Variables](docs/agents/environment.md) — read when touching env vars or dev setup
- [Testing](docs/agents/testing.md) — read when writing or running tests; mock patterns
- [Commands](docs/agents/commands.md) — read when building or running the daemon/frontend
- [Decisions](docs/agents/decisions.md) — read before "fixing" unusual patterns; 104 self-contained entries. **Start from the Index at the top and read only the entry it points to** — the whole file is 2k lines and never needs to be read end to end
- [Troubleshooting Downloads](docs/agents/troubleshooting-downloads.md) — follow when animes fail to download; diagnose search/filter logic bugs step by step
- [Sources](docs/agents/sources.md) — read before adding a torrent source; measured limits of Nyaa, the AnimeTosho JSON API, and why the alternatives were rejected
