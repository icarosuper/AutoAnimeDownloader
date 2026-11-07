# Contributing

Contributions are welcome. Setup, project layout and how to run the daemon locally are in the
[Development Guide](docs/guides/development.md).

## Reporting bugs

The fastest way is from the app itself: **More → Report a problem** in the web UI, or **Report a
problem** in the tray menu. It opens the [bug report form](https://github.com/icarosuper/AutoAnimeDownloader/issues/new?template=bug_report.yml)
with your environment already filled in and gives you a report file to attach.

If the app won't open, file the form by hand and include the steps to reproduce, what you expected
versus what happened, the relevant log lines (`autoanimedownloader logs` or
`~/.autoAnimeDownloader/daemon.log`) and your OS and app version.

## Suggesting features

Use the [feature request form](https://github.com/icarosuper/AutoAnimeDownloader/issues/new?template=feature_request.yml). Start from the
problem you hit, not the solution — it's easier to find the right fix when the situation is clear.

## Pull requests

1. Fork the repo and create a branch off `master`: `git checkout -b feat/your-feature`
2. Make your changes and run the tests before opening a PR:
   - `go test ./...`
   - `make test-frontend` if you touched the web UI
3. Push and open a pull request against `master`

A new API endpoint, or a web UI action that calls the API, ships with its CLI command in the same
PR — see the checklists in [Conventions](docs/agents/conventions.md).

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/), in English, subject line only,
lowercase after the type:

```
feat: add new endpoint for episode filtering
fix: resolve memory leak in state manager
docs: update build guide
test: add tests for config validation
refactor: simplify daemon loop logic
```

### Code style

- **Go**: `gofmt`, handle all errors, follow the patterns in [Conventions](docs/agents/conventions.md)
- **Frontend**: Svelte + Tailwind, formatted with Prettier
- Everything in English — code, comments, logs, docs. The only exception is the Portuguese
  translation of the web UI.
