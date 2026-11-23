---
name: finish
description: Close out the current uncommitted change in AutoAnimeDownloader — /simplify, /code-review (simple fixes applied, decisions asked), the decision-checker, tests and docs, then commit to the current branch. Only when the user invokes it (/finish, or "roda o simplify e a review, arruma tudo e commita"). Optional argument is the code-review level (low|medium|high|xhigh); without one, code-review reuses the last level.
disable-model-invocation: true
---

# Finish a change

This skill ends in a commit of the change that exists now. It is not permission to push
(CLAUDE.md rule 5).

Run the steps in order, without stopping between them unless a step says to ask. Each step works on
the result of the one before: simplify rewrites code, so the review must see the rewritten version.

## 0. Scope

`git status --short` and `git diff HEAD --stat`. Nothing uncommitted → say so and stop.

Classify the change: **code** (any `.go` or anything under `src/internal/frontend/` except docs) or
**docs-only**. A docs-only change skips steps 1–3.

## 1. `/simplify`

Invoke the `simplify` skill. It applies its fixes itself.

## 2. `/code-review`

Invoke the `code-review` skill with `args` = `"<level>"`, where `<level>` is this skill's argument
if the user gave one; otherwise no args. No `--fix`: sort the findings first, then act. Never pass
`max` or `ultra` unless the user named it.

- **Simple fix** — one obvious correct change that does not alter intended behavior (a missed nil
  check, a wrong variable, an off-by-one, a leaked resource, a stale comment) → fix it yourself.
- **Needs a decision** — more than one reasonable fix, the fix changes behavior the user may have
  wanted, it touches a recorded decision, or you are not sure the finding is real → **stop and ask**
  before going on, one or two lines per finding with `file:line` and the options. Apply the simple
  fixes first so the question is about the rest only; continue with step 3 after the answer.

A finding marked **pre-existing** (a bug the change did not add) falls under CLAUDE.md "Bad code in
the way": do not fix it silently — collect it and ask the user fix / backlog / ignore in the final
report, and leave it out of this commit.

## 3. decision-checker

Run the `decision-checker` subagent on the uncommitted diff, with one line on what the change is
meant to do (not why it is fine). For each finding:

- **stale entry / missing entry** → fix it (amend the entry, or add one with `record-decision`);
- **contradiction / silent removal** → stop and ask the user before committing: either the diff or
  the decision has to change, and that is their call.

## 4. Tests and checks

- `go test ./...` (and `go vet ./...`) when Go changed.
- `cd src/internal/frontend && bun run check && bun run test:unit` when the frontend changed.

A failure → fix it and rerun. If the fix is not obvious or would change behavior, stop and report.

## 5. Docs

CLAUDE.md "After any change", step 2: endpoints, config fields, env vars, patterns, added/removed
files → the matching doc in `docs/agents/`. A new API endpoint or web-app action → CLI parity (step
3). Translate any Portuguese you came across (CLAUDE.md "Everything in English").

## 6. Commit

- Stage the files of this change by name (`git add <paths>`, never `git add -A`); leave out anything
  that was already modified before the change started and is unrelated — ask if unsure.
- One commit on the **current branch**, whatever it is (do not switch or create one), message per CLAUDE.md "Commit style": `type: lowercase imperative`, subject
  only, no body, no trailer, the behavior rather than the file.
- Do not push.

## 7. Report (in Portuguese, short)

- the commit hash and subject;
- what simplify and code-review changed, one line each;
- decision-checker result (which entries it read, anything amended);
- test result;
- pre-existing findings awaiting fix / backlog / ignore, with `file:line`.
