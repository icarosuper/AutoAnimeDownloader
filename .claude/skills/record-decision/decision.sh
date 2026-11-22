#!/usr/bin/env bash
# Helpers for docs/agents/decisions/, one decision per NNN-<slug>.md file plus a generated index.md.
#
#   decision.sh next              next free number (highest NNN + 1)
#   decision.sh new "<title>"     create NNN-<slug>.md for the next number, print its path
#   decision.sh path <N>          path of an existing entry
#   decision.sh refs <N>          every reference to entry N in the repo
#   decision.sh about <file>...   entries that name these files or that they cite (review shortlist)
#   decision.sh index             regenerate index.md from the files' headings
#   decision.sh check             headings match filenames, index is current, every link resolves
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
DIR="$ROOT/docs/agents/decisions"
INDEX="$DIR/index.md"

# Lowercase, drop everything but [a-z0-9 -], hyphens collapsed, cut near 50 chars at a word boundary.
# Only used when a file is created: the filename is frozen after that, whatever the title becomes.
slug() {
	local s
	s="$(printf '%s' "$1" | tr 'A-Z_' 'a-z-' | sed -E 's/[^a-z0-9 -]//g; s/ /-/g; s/-+/-/g; s/^-//; s/-$//')"
	if [ "${#s}" -gt 50 ]; then
		s="${s:0:51}"
		s="${s%-*}"
	fi
	printf '%s' "${s%-}"
}

entries() { find "$DIR" -maxdepth 1 -name '[0-9][0-9][0-9]-*.md' -printf '%f\n' | sort; }
num_of() { echo "$((10#${1:0:3}))"; }
title_of() { head -1 "$DIR/$1" | sed -E 's/^# [0-9]+\. //'; }
file_of() {
	local f
	f="$(entries | grep -E "^$(printf '%03d' "$1")-" || true)"
	[ -n "$f" ] || { echo "no entry #$1" >&2; exit 1; }
	printf '%s' "$f"
}

render_index() {
	cat <<'EOF'
# Decisions

Patterns that look wrong but are intentional. Read before "fixing" anything.

One decision per file, `NNN-<slug>.md`, each self-contained. To check whether a change goes
against an earlier decision: read this index, then open only the entries that matter; for a
keyword the titles don't show, `grep -ril "<word>" docs/agents/decisions/`. Why it is laid out
this way: [#134](134-each-decision-is-its-own-numbered-file-and-the.md).

**Generated** by `.claude/skills/record-decision/decision.sh index` from each file's heading —
do not edit by hand.

## Index

EOF
	local f
	while IFS= read -r f; do
		printf -- '- [**#%s** — %s](%s)\n' "$(num_of "$f")" "$(title_of "$f")" "$f"
	done < <(entries)
}

case "${1:-}" in
next)
	last="$(entries | tail -1)"
	echo "$(($(num_of "$last") + 1))"
	;;
new)
	title="${2:?usage: decision.sh new \"<title>\"}"
	n="$("$0" next)"
	f="$DIR/$(printf '%03d' "$n")-$(slug "$title").md"
	printf '# %s. %s\n' "$n" "$title" >"$f"
	echo "$f"
	;;
path)
	echo "docs/agents/decisions/$(file_of "${2:?usage: decision.sh path <N>}")"
	;;
refs)
	n="${2:?usage: decision.sh refs <N>}"
	p="$(printf '%03d' "$n")"
	cd "$ROOT"
	# From code and other docs: decisions/NNN; between entries: the filename, or a bare #N.
	git grep -nE "decisions/$p\b" -- ':!docs/agents/decisions/index.md' || true
	git grep -nE "\]\($p-|#$n\b" -- 'docs/agents/decisions/' ':!docs/agents/decisions/index.md' \
		":!docs/agents/decisions/$p-*" || true
	;;
about)
	shift
	[ "$#" -gt 0 ] || { echo "usage: decision.sh about <file>..." >&2; exit 2; }
	cd "$ROOT"
	# The entries a change to these files may contradict: the ones that name the file (as
	# <parent>/<base>, the way Location lines spell it, e.g. nyaa/nyaa.go) and the ones the file
	# itself cites. A shortlist, not a verdict: an entry that only names the function is missed.
	{
		for f in "$@"; do
			short="$(basename "$(dirname "$f")")/$(basename "$f")"
			grep -lF -- "$short" "$DIR"/[0-9][0-9][0-9]-*.md 2>/dev/null | sed "s|^|$f	names it	|" || true
			[ -f "$f" ] && grep -oE 'decisions/[0-9]{3}' "$f" | sort -u | while IFS= read -r c; do
				e="$(entries | grep -E "^${c#decisions/}-" || true)"
				[ -n "$e" ] && printf '%s\tcited in it\t%s/%s\n' "$f" "$DIR" "$e"
			done
		done
	} | sed "s|$DIR/||" | sort -u -t'	' -k3,3 -k1,1 | awk -F'\t' '{ printf "%-40s %-12s %s\n", $3, $2, $1 }'
	;;
index)
	render_index >"$INDEX"
	echo "wrote $INDEX ($(entries | wc -l) entries)"
	;;
check)
	bad=0
	while IFS= read -r f; do
		n="$(num_of "$f")"
		head -1 "$DIR/$f" | grep -qE "^# $n\. " || { echo "$f: first line is not '# $n. <title>'" >&2; bad=1; }
	done < <(entries)
	diff -q <(render_index) "$INDEX" >/dev/null || { echo "index.md is stale: run decision.sh index" >&2; bad=1; }
	# Every link to a decision file, anywhere in the repo, must resolve.
	cd "$ROOT"
	while IFS=: read -r src target; do
		dir="$(dirname "$src")"
		[ -e "$dir/$target" ] || { echo "$src: broken link $target" >&2; bad=1; }
	done < <(git ls-files -co --exclude-standard -- '*.md' | grep -v node_modules | while IFS= read -r f; do
		# Inline code is an example, not a link: drop it before looking.
		sed 's/`[^`]*`//g' "$f" | grep -oE '\]\([^)#[:space:]]*[0-9]{3}-[a-z0-9-]+\.md' | sed -E "s|^\]\(|$f:|"
	done)
	[ "$bad" = 0 ] && echo "ok: $(entries | wc -l) entries, index current, links resolve"
	exit "$bad"
	;;
*)
	sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
