#!/usr/bin/env bash
# PreToolUse(Bash): `git push` always asks the user first, even in bypass mode.
# Commits go through without a prompt; pushing is what the user confirms.
set -euo pipefail

input=$(cat)
[[ $input == *git* ]] || exit 0 # most Bash calls: skip jq and grep
cmd=$(jq -r '.tool_input.command // ""' <<<"$input")

# Heredoc bodies are data (file contents, a Python script's strings), not commands: drop them before
# matching, unless the heredoc feeds a shell (`bash <<EOF`), whose body does run. Without this, a
# doc edit that merely mentions the command asked for a push confirmation.
cmd=$(printf '%s\n' "$cmd" | awk '
  skip { if ($0 ~ "^[[:space:]]*" term "[[:space:]]*$") skip = 0; next }
  { print }
  match($0, /<<-?[[:space:]]*["\047]?[A-Za-z_][A-Za-z0-9_]*/) {
    head = substr($0, 1, RSTART - 1)
    if (head ~ /(^|[^[:alnum:]_\/.-])(ba|z|fi)?sh[[:space:]]*$/) next
    term = substr($0, RSTART, RLENGTH); sub(/^<<-?[[:space:]]*["\047]?/, "", term)
    skip = 1
  }')

# git, then push as any later word of the same command segment. Broad on purpose: listing git's
# global options (-C "dir with space", --git-dir <path>…) always misses a shape, and a false ask
# (`git log --grep push`) only costs one confirmation.
if printf '%s' "$cmd" | grep -qE '(^|[;&|(`[:space:]])git[[:space:]]([^;&|`]*[[:space:]])?push\b'; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "ask",
      permissionDecisionReason: "Never push without permission — confirm this one explicitly."
    }
  }'
fi
