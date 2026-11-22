# Agentic tooling: enforce the CLAUDE.md rules instead of asking for them

Found on 2026-11-22 while reviewing which agentic-development patterns the repo does not use yet.
Not started. Delete this file when it is fixed or dropped.

The context layer (CLAUDE.md, `docs/agents/`, 135 decisions, four skills, `make sandbox`) is
mature. What is missing is enforcement: almost every rule in CLAUDE.md depends on the agent
remembering it. Points below, roughly ordered by payoff per effort. Remove each section when it
ships (on 2026-11-22: point 1, the hooks in `.claude/settings.json`; point 2, the docs checks in
`src/tests/unit/docs_test.go`).

## 3. Golden set for the search/match pipeline

The most fragile part of the project is search + filtering (it is why the `debug-anime` skill
exists). Every diagnosed case becomes a fixture — the real Nyaa titles plus the expected
outcome — in a table-driven test under `testdata/`. Over time it is the regression suite that
lets an agent change the matcher without silently breaking an older case. `debug-anime` can end
with "add the case to the golden set".

## 4. Project-specific review subagents (`.claude/agents/`)

E.g. a `decision-checker` that takes the diff and checks it against the decisions index, so
contradicting a recorded decision does not depend on the main agent remembering to read it. A
separate reviewer is not anchored on the author's reasoning.

## 5. Architecture fitness tests

Invariants such as "package X never imports Y" or "only package Z writes to the data dir" become
a test built on `go/packages` instead of a sentence in `architecture.md`.

## 6. Smaller items

- **Worktrees for parallel slices**: `docs/v3.0.0.md` already coordinates slices across chats;
  `EnterWorktree` / agent `isolation: worktree` keeps each slice on its own checkout.
