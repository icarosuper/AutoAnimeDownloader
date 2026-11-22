# `decision.sh check` stops scanning links at the first file without one and still says ok

Found on 2026-11-22 while running /code-review on the agentic-tooling changes. Not started. Delete
this file when it is fixed or dropped.

## Symptom

`decision.sh check` prints `ok: 134 entries, index current, links resolve` with a broken link in
the tree. Repro: put `[x](docs/agents/decisions/043-nope.md)` in an untracked `.md` at the repo
root and run `bash .claude/skills/record-decision/decision.sh check`. It passes.

`go test ./...` still catches the link (`TestMarkdownLinksResolve` in
`src/tests/unit/docs_test.go`). Only `check` run on its own is wrong, and the record-decision skill
tells you to run it as the last step.

## Cause

`.claude/skills/record-decision/decision.sh`, `check`: the link scan runs in a `< <(...)` process
substitution that inherits `set -euo pipefail`. For each `.md`, the pipeline is
`sed | grep -oE | sed`. In the first file with no decision link, `grep` exits 1, the pipeline fails,
and the subshell exits. The outer `while read` gets no more lines, so it never sees a broken link.
`about` had the same bug and was fixed in `ac985ef` (`{ grep … || true; }`).

## What to check

- Add `|| true` to the per-file pipeline, or drop the link loop and rely on the Go test, which
  already covers every relative `.md` link. If you drop it, change the "every link resolves" claim
  in `SKILL.md` and the usage comment.
- Smaller things in the same script, found in the same review:
  - `check` no longer fails on two entries with the same number. Two worktrees can each run `new`
    and get `135`. `file_of` then prints both filenames.
  - `refs N` greps only `decisions/NNN` outside the folder, so it misses the `decisions #133` form
    that a backlog file still uses (`baki-dou-part-2-…`).
  - `entries` uses `find -printf`, which only GNU find has, and `next` crashes on an empty directory.
