#!/usr/bin/env bash
# Runs --debug-anime over the curated list in scripts/robustness-animes.txt and writes a
# triaged report.md under .debug_batch/. Uses live network (AniList + Nyaa) and takes minutes.
#
# Not a suite: no pass/fail. The result is a report for a human or an agent to read.
#
# --report-only rebuilds report.md from the .debug_batch/ already on disk, with no network: it is
# for adjusting the triage without paying another round against Nyaa.
set -uo pipefail

REPORT_ONLY=0
[ "${1:-}" = "--report-only" ] && REPORT_ONLY=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST="$ROOT/scripts/robustness-animes.txt"
OUT="$ROOT/.debug_batch"
BIN="$ROOT/build/aad-debug"
CONFIG="$HOME/.autoAnimeDownloader/config.json"

# Below this, the best torrent found on Nyaa is treated as weak, and the anime goes on the list
# of candidates for an alternative source. The bar is low on purpose: it is a list to investigate
# by hand, not a gate.
WEAK_SEEDERS=${WEAK_SEEDERS:-10}

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
[ -f "$LIST" ] || { echo "missing $LIST" >&2; exit 1; }

if [ "$REPORT_ONLY" -eq 0 ]; then
	echo "Building debug binary..."
	go build -o "$BIN" ./src/cmd/daemon || exit 1

	rm -rf "$OUT"
	mkdir -p "$OUT"
fi

ids=()
while read -r id _rest; do
	[ -n "$id" ] && ids+=("$id")
done < <(sed 's/#.*//' "$LIST" | tr -d '[:blank:]' | grep -E '^[0-9]+$')

declare -A exit_code
i=0
for id in "${ids[@]}"; do
	exit_code[$id]=0
	[ "$REPORT_ONLY" -eq 1 ] && continue
	i=$((i + 1))
	echo "[$i/${#ids[@]}] debugging anime $id..."
	# CWD inside $OUT (just wiped): NextDebugDir finds nothing and N is always 1, so the
	# path is predictable without parsing any log.
	(cd "$OUT" && "$BIN" --debug-anime "$id")
	exit_code[$id]=$?
	sleep 2
done

report="$OUT/report.md"
{
	echo "# debug-batch — ${#ids[@]} animes — $(date -Iseconds)"
	echo
	if [ -f "$CONFIG" ]; then
		jq -r '"config: max_episodes_per_anime=\(.max_episodes_per_anime), max_batch_episodes=\(.max_batch_episodes), min_seeders=\(.min_seeders), max_episode_torrent_size_gb=\(.max_episode_torrent_size_gb)"' "$CONFIG"
	else
		echo "config: $CONFIG not found"
	fi
	echo
	echo "| id | anime | searched | with magnet | without magnet | best seeders |"
	echo "|---|---|---|---|---|---|"
} > "$report"

suspects=""
nothing=""
errors=""
weak=""

for id in "${ids[@]}"; do
	dir="$OUT/.debug_${id}_1"
	summary="$dir/summary.json"
	rel=".debug_batch/.debug_${id}_1/debug.jsonl"

	if [ "${exit_code[$id]}" -ne 0 ]; then
		errors+="- $id → $rel"$'\n'
	fi

	if [ ! -f "$summary" ]; then
		echo "| $id | (no summary.json) | - | - | - | - |" >> "$report"
		continue
	fi

	# Best seeders among torrents that passed the filter (the matched_torrents log already
	# carries "S:<seeders>/L:<leechers>"). Signal of "Nyaa has it, but is it any good?" without
	# touching Go.
	rows=$(jq -r 'select(.matched_torrents) | .matched_torrents[]' "$dir/debug.jsonl" 2>/dev/null)
	found=$(printf '%s' "$rows" | grep -c 'S:' || true)
	best=$(printf '%s' "$rows" | grep -oE 'S:[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)
	best=${best:-0}

	read -r name searched with without eps <<< "$(jq -r '
		[.episodes[] | select(.would_search)] as $s |
		[$s[] | select(.magnets_found > 0)] as $ok |
		[$s[] | select(.magnets_found == 0) | .episode] as $bad |
		[(.anime_name | gsub("[ \t|]+"; "_")), ($s | length), ($ok | length), ($bad | length),
		 (if ($bad | length) == 0 then "-" else ($bad[0:10] | join(",")) end)] | @tsv' "$summary")"

	echo "| $id | ${name//_/ } | $searched | $with | $without | $best |" >> "$report"

	if [ "$searched" -eq 0 ]; then
		nothing+="- $id ${name//_/ } → $rel"$'\n'
	elif [ "$with" -eq 0 ]; then
		suspects+="- $id ${name//_/ } — eps $eps → $rel"$'\n'
		# "Found it and died on the seeders floor" and "Nyaa does not have it" ask for different
		# sources: the first is the same release with nobody seeding, the second is missing content.
		if [ "$found" -gt 0 ]; then
			weak+="- $id ${name//_/ } — $found torrents found, best with $best seeders (dead, barred by min_seeders)"$'\n'
		else
			weak+="- $id ${name//_/ } — no torrent found on Nyaa"$'\n'
		fi
	elif [ "$best" -lt "$WEAK_SEEDERS" ]; then
		weak+="- $id ${name//_/ } — best torrent with $best seeders"$'\n'
	fi
done

{
	echo
	echo "## SUSPECTS (would_search, 0 magnets)"
	echo "${suspects:-- none}"
	echo "## NOTHING SEARCHED (no episode selected)"
	echo "${nothing:-- none}"
	echo "## ERRORS (exit ≠ 0)"
	echo "${errors:-- none}"
	echo "## NO GOOD SOURCE ON NYAA (candidates for another source, < $WEAK_SEEDERS seeders or nothing)"
	echo "${weak:-- none}"
} >> "$report"

echo
echo "$report"
