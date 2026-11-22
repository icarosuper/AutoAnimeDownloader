# Agentic tooling: enforce the CLAUDE.md rules instead of asking for them

Found on 2026-11-22 while reviewing which agentic-development patterns the repo does not use yet.
Not started. Delete this file when it is fixed or dropped.

The context layer (CLAUDE.md, `docs/agents/`, 135 decisions, four skills, `make sandbox`) is
mature. What is missing is enforcement: almost every rule in CLAUDE.md depends on the agent
remembering it. Points below, roughly ordered by payoff per effort. Remove each section when it
ships (on 2026-11-22: point 1, the hooks in `.claude/settings.json`; point 2, the docs checks in
`src/tests/unit/docs_test.go`; point 3, the search/match golden set in
`src/internal/daemon/golden_test.go`; point 4, the `decision-checker` subagent in
`.claude/agents/`; point 5, the import rules in `src/tests/unit/architecture_test.go`).

## 6. Smaller items

- **Worktrees for parallel slices**: `docs/v3.0.0.md` already coordinates slices across chats;
  `EnterWorktree` / agent `isolation: worktree` keeps each slice on its own checkout.
