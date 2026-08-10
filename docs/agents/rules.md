# Rules

Mandatory rules for working in this repo. These supplement [Conventions](conventions.md).

## Documentation

### Always update `architecture.md` when adding or removing Go files

Every new file under `src/internal/` or `src/cmd/` must get a section in `architecture.md` before the PR is merged. The section must list every exported symbol (and key unexported ones) with a one-line purpose.

Every deleted file must have its section removed.

This keeps `architecture.md` the single source of truth for "what exists and where" — the primary navigation doc for AI-assisted development.

## Código ruim encontrado pelo caminho

Ao se deparar com código ruim / bloat / deprecado / que poderia melhorar:

- **Dentro do escopo da task** → ajuste na hora.
- **Fora do escopo da task** → pergunte ao dev o que fazer. Normalmente as opções são: ajustar agora, marcar com `TODO` pra depois, ou ignorar.
