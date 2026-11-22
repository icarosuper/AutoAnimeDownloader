#!/usr/bin/env bash
# Stop: run `go vet ./...` and `go test ./...` before the agent hands the turn back, when Go files
# changed since the last green run. A failure blocks the stop once and goes back to the agent;
# if it is still red on the second try, the user gets a warning instead of a loop.
# CLAUDE.md "After any change" step 1.
set -uo pipefail

input=$(cat)
root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$root"

# Fingerprint of every uncommitted Go change (tracked diff + untracked files' contents).
fingerprint=$({
  git diff HEAD -- '*.go' 'go.mod' 'go.sum'
  git ls-files --others --exclude-standard -- '*.go' | xargs -r cat
} | sha256sum | cut -d' ' -f1)

state=.claude/hooks/.last-green
[ "$(cat "$state" 2>/dev/null)" = "$fingerprint" ] && exit 0

# Same guard as `make test-backend-unit`: the embed needs a dist/ to compile.
[ -d src/internal/frontend/dist ] || { mkdir -p src/internal/frontend/dist && printf '<html></html>' >src/internal/frontend/dist/index.html; }

if out=$({ go vet ./... && go test ./...; } 2>&1); then
  printf '%s' "$fingerprint" >"$state"
  exit 0
fi

failures=$(grep -vE '^(ok|\?)[[:space:]]' <<<"$out" | tail -n 60)

if [ "$(jq -r '.stop_hook_active // false' <<<"$input")" = "true" ]; then
  jq -n '{systemMessage: "go vet / go test still failing — see the last run in the transcript."}'
  exit 0
fi

jq -n --arg f "$failures" '{
  decision: "block",
  reason: ("go vet / go test failed (CLAUDE.md: run tests after any change). Fix it, or tell the user why it is red:\n" + $f)
}'
