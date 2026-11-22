#!/usr/bin/env bash
# PostToolUse(Edit|Write): flag Portuguese in the text that was just written. Advisory, never blocks.
# CLAUDE.md "Everything in English" — the pt-BR UI translation is the only exception.
set -euo pipefail

input=$(cat)
file=$(jq -r '.tool_input.file_path // ""' <<<"$input")

case "$file" in
  */messages/pt-BR.json | */i18n/*) exit 0 ;; # the translation itself
esac

# Only the new text: Edit → new_string, Write → content. Pre-existing lines are not re-flagged.
text=$(jq -r '.tool_input.new_string // .tool_input.content // ""' <<<"$input")
[ -n "$text" ] || exit 0

# High-signal words only: accented function words and the -ção/-ções suffix. Plain words like
# "para" or "com" collide with English and URLs.
pattern='(*UCP)\b(não|são|também|então|você|está|já|até|porém|além|código|arquivo|usuário|\w+(ção|ções))\b'
hits=$(LC_ALL=C.UTF-8 grep -inP "$pattern" <<<"$text" | head -n 8 || true)
[ -n "$hits" ] || exit 0

jq -n --arg file "$file" --arg hits "$hits" '{
  hookSpecificOutput: {
    hookEventName: "PostToolUse",
    additionalContext: ("Possible Portuguese in what you just wrote to \($file) (CLAUDE.md: everything in English except the pt-BR UI translation):\n\($hits)\nTranslate it unless it is a pt-BR UI string a test asserts on, or a user phrase a skill must match.")
  }
}'
