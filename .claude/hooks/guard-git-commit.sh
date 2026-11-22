#!/usr/bin/env bash
# PreToolUse(Bash): `git commit` and `git push` always ask the user first, even in bypass mode.
# CLAUDE.md rule 5 — the user commits and pushes themselves unless they say otherwise.
set -euo pipefail

input=$(cat)
[[ $input == *git* ]] || exit 0 # most Bash calls: skip jq and grep
cmd=$(jq -r '.tool_input.command // ""' <<<"$input")

# git, then commit/push as any later word of the same command segment. Broad on purpose: listing
# git's global options (-C "dir with space", --git-dir <path>…) always misses a shape, and a false
# ask (`git log --grep commit`) only costs one confirmation.
if printf '%s' "$cmd" | grep -qE '(^|[;&|(`[:space:]])git[[:space:]]([^;&|`]*[[:space:]])?(commit|push)\b'; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "ask",
      permissionDecisionReason: "CLAUDE.md: never commit or push without permission — confirm this one explicitly."
    }
  }'
fi
