#!/usr/bin/env bash
# Helpers for docs/agents/decisions.md, whose entries are not in numeric order.
#
#   decision.sh next            next free number (highest "### N." + 1)
#   decision.sh anchor "<N>. <title>"   GitHub anchor for an entry heading (without the "### ")
#   decision.sh anchor <N>      anchor of an existing entry
#   decision.sh check           every entry has exactly one Index line, and every Index link resolves
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
FILE="$ROOT/docs/agents/decisions.md"

# GitHub's slug: lowercase, drop everything but [a-z0-9 _-], spaces become '-'. Checked against
# every Index link in the file — "—" leaves two spaces, hence the "--" in many anchors.
slug() { printf '%s' "$1" | tr 'A-Z' 'a-z' | sed -E 's/[^a-z0-9 _-]//g; s/ /-/g'; }

headings() { grep -E '^### [0-9]+\. ' "$FILE" | sed -E 's/^### //'; }

case "${1:-}" in
next)
	headings | sed -E 's/^([0-9]+)\..*/\1/' | sort -n | tail -1 | awk '{print $1 + 1}'
	;;
anchor)
	arg="${2:?usage: decision.sh anchor <N> | \"<N>. <title>\"}"
	if [[ "$arg" =~ ^[0-9]+$ ]]; then
		arg="$(headings | grep -E "^$arg\. " || true)"
		[ -n "$arg" ] || { echo "no entry #$2" >&2; exit 1; }
	fi
	echo "#$(slug "$arg")"
	;;
check)
	bad=0
	while IFS= read -r h; do
		n="${h%%.*}"
		a="#$(slug "$h")"
		c="$(grep -cF "](${a})" <(sed -n '/^## Index/,/^---$/p' "$FILE") || true)"
		[ "$c" = 1 ] || { echo "#$n: $c Index lines link to $a" >&2; bad=1; }
	done < <(headings)
	dups="$(headings | sed -E 's/^([0-9]+)\..*/\1/' | sort -n | uniq -d)"
	[ -z "$dups" ] || { echo "duplicate numbers: $dups" >&2; bad=1; }
	[ "$bad" = 0 ] && echo "ok: $(headings | wc -l) entries, Index consistent"
	exit "$bad"
	;;
*)
	sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
