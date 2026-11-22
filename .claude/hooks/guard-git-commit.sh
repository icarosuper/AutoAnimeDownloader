#!/usr/bin/env bash
# PreToolUse(Bash): `git commit` and `git push` always ask the user first, even in bypass mode.
# CLAUDE.md rule 5 — the user commits and pushes themselves unless they say otherwise.
set -euo pipefail

cmd=$(jq -r '.tool_input.command // ""')

# git, optionally with global options (-C dir, -c k=v, --no-pager…), then commit/push.
if printf '%s' "$cmd" | grep -qE '(^|[;&|(`[:space:]])git([[:space:]]+(-[Cc][[:space:]]+[^[:space:]]+|--?[a-z-]+(=[^[:space:]]+)?))*[[:space:]]+(commit|push)\b'; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "ask",
      permissionDecisionReason: "CLAUDE.md: never commit or push without permission — confirm this one explicitly."
    }
  }'
fi
