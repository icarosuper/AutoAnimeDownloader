# Rules

Mandatory rules for working in this repo. These supplement [Conventions](conventions.md).

## Documentation

### Always update `architecture.md` when adding or removing Go files

Every new file under `src/internal/` or `src/cmd/` must get a section in `architecture.md` before the PR is merged. The section must list every exported symbol (and key unexported ones) with a one-line purpose.

Every deleted file must have its section removed.

This keeps `architecture.md` the single source of truth for "what exists and where" — the primary navigation doc for AI-assisted development.
