#!/usr/bin/env bash
# Roda o --debug-anime sobre a lista curada de scripts/robustness-animes.txt e escreve um
# report.md triado em .debug_batch/. Usa rede ao vivo (AniList + Nyaa) e leva minutos.
#
# Nao e suite: nao tem pass/fail. O resultado e um relatorio para leitura humana ou de agente.
#
# --report-only regera o report.md a partir do .debug_batch/ que ja esta em disco, sem rede: e para
# ajustar a triagem sem pagar outra rodada contra o Nyaa.
set -uo pipefail

REPORT_ONLY=0
[ "${1:-}" = "--report-only" ] && REPORT_ONLY=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST="$ROOT/scripts/robustness-animes.txt"
OUT="$ROOT/.debug_batch"
BIN="$ROOT/build/aad-debug"
CONFIG="$HOME/.autoAnimeDownloader/config.json"

# Abaixo disso o melhor torrent encontrado no Nyaa e considerado fraco, e o anime entra na lista
# de candidatos a uma fonte alternativa. Bar baixa de proposito: e uma lista para investigar a mao,
# nao um gate.
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
	# CWD dentro de $OUT (recem-apagado): NextDebugDir nao acha nada e o N e sempre 1, entao o
	# caminho e previsivel sem parsear log nenhum.
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
		echo "config: $CONFIG nao encontrado"
	fi
	echo
	echo "| id | anime | buscados | com magnet | sem magnet | melhor seeders |"
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
		echo "| $id | (sem summary.json) | - | - | - | - |" >> "$report"
		continue
	fi

	# Melhor seeders entre os torrents que passaram o filtro (o log matched_torrents ja traz
	# "S:<seeders>/L:<leechers>"). Sinal de "o Nyaa tem, mas presta?" sem tocar no Go.
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
		# "Achou e morreu no piso de seeders" e "o Nyaa nao tem" pedem fontes diferentes: a
		# primeira e o mesmo release sem quem semeie, a segunda e conteudo ausente.
		if [ "$found" -gt 0 ]; then
			weak+="- $id ${name//_/ } — $found torrents encontrados, melhor com $best seeders (mortos, barrados por min_seeders)"$'\n'
		else
			weak+="- $id ${name//_/ } — nenhum torrent encontrado no Nyaa"$'\n'
		fi
	elif [ "$best" -lt "$WEAK_SEEDERS" ]; then
		weak+="- $id ${name//_/ } — melhor torrent com $best seeders"$'\n'
	fi
done

{
	echo
	echo "## SUSPEITOS (would_search, 0 magnets)"
	echo "${suspects:-- nenhum}"
	echo "## NADA BUSCADO (nenhum episódio selecionado)"
	echo "${nothing:-- nenhum}"
	echo "## ERROS (saída ≠ 0)"
	echo "${errors:-- nenhum}"
	echo "## SEM FONTE BOA NO NYAA (candidatos a outra fonte, < $WEAK_SEEDERS seeders ou nada)"
	echo "${weak:-- nenhum}"
} >> "$report"

echo
echo "$report"
