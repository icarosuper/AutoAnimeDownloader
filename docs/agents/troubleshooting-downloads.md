# Troubleshooting Failed Downloads

Use this guide when investigating why animes are failing to download, to identify bugs or gaps in the search/filter logic.

---

## Step 0 — Fast path: `make debug-anime`

Before doing anything manual, try:

```bash
make debug-anime ID=<anilistId>
```

`<anilistId>` is the AniList **media** ID (visible in the web UI URL for that anime, `#/status/<id>`, or via `GET /api/v1/animes`). This runs a one-shot pass — no running daemon required (the torrent client is embedded, nothing external to start) — that:

1. Fetches the anime from AniList and logs the full response (title, progress, status, synonyms, relations, airing schedule).
2. Picks the episodes that would be searched (same `checkEpisode` logic the real loop uses).
3. Runs the real search/match pipeline (same movie→pack→single priority as production: `resolveMovie`, then `partitionSearchResults`/`pickBatches` over the anime search, then `searchSingleEpisode` for whatever isn't covered) against live Nyaa.
4. Logs **every raw torrent row** Nyaa returned for the query (`"Raw Nyaa row"`), then the **matched subset** (`"Found ... results"` with `matched_torrents` — name, seeders/leechers, size and health score of each candidate, in ranking order), then a per-episode summary (`magnets_found`).

Output goes to `.debug_<anilistId>_<N>/` in the current directory — nothing is written to `~/.autoAnimeDownloader`:
- `debug.jsonl` has the full trace from steps 1-4 above (one JSON object per line — grep/`jq` it, or just read it directly)
- `summary.json` has the quick per-episode view (`would_search`, `magnets_found`)

Comparing `debug.jsonl`'s raw rows against its matched set tells you immediately whether the problem is upstream (Nyaa has nothing for this title/episode) or in filtering (Nyaa has it, but it got rejected — see Step 6 for why).

For a whole population at once, `make debug-batch` runs this same pass over `scripts/robustness-animes.txt` and writes a triaged `.debug_batch/report.md` — start there, and only open the `debug.jsonl` of the animes it flags (see [Commands](commands.md)).

**Known limitations** (logged as a warning at the start of every run): episodes are always treated as not-yet-downloaded (no `episodes.json` / torrent-backend check), and excluded-list membership is never evaluated (the single-anime AniList query doesn't fetch custom-list data). If the anime is being skipped for one of those two reasons, this tool won't show it — fall back to Steps 1-8 below.

---

## Step 1 — Read the logs

```bash
build/autoanimedownloader logs
# or
grep -E '"level":"warn"|"level":"error"' ~/.autoAnimeDownloader/daemon.log
```

For archived logs:

```bash
zcat ~/.autoAnimeDownloader/daemon-*.log.gz | grep -E '"level":"warn"|"level":"error"'
```

---

## Step 2 — Identify the failure type

Look for:

```json
{"level":"warn","episode":"Anime Name - Episode N","attempts":N,"message":"Failed to download episode after all attempts"}
```

**`attempts: 0`** → search returned zero magnets. The download loop never ran. Root cause is upstream in the Nyaa search or result filtering.

**`attempts > 0`** → magnets were found but the embedded torrent client rejected all of them. With the embedded client the usual causes are:
- **`ErrSessionNotReady`** — `completed_anime_path` is empty/unset, so the download path (`Config.DownloadPath()`, derived from it) is empty and no torrent session exists yet. Complete the config.
- **Magnet parse failure** — a malformed magnet (`parseInfoHash` couldn't extract the info hash).

**Nenhum log de busca para o anime inteiro** → antes de acusar a busca, procure:

```json
{"level":"info","anime":"...","episode":N,"absolute_episode":M,"owner_anime_id":X,"message":"Adopting episode already covered by a downloaded pack"}
```

Isso é a **posse por cobertura** (decisions.md #78): o episódio já está dentro de um pack em
disco, baixado sob outra entrada da AniList (o cour anterior, tipicamente), então ele nunca chega
ao Nyaa — e isso é o comportamento correto, não uma falha. O registro novo aponta para o hash que
já existe. Se o arquivo de fato não estiver lá, o problema é da faixa declarada (`BatchStart`/
`BatchEnd`, lida do nome do torrent), não da busca.

A separate, new failure class is **"the torrent downloaded but nothing shows up in the library"** — the download succeeded but the `organize` job (hardlink into `completed_anime_path` + webhook) is failing or retrying. See the embedded-client section below.

---

## Embedded-client reality (no qBittorrent)

The BitTorrent client is embedded (`github.com/cenkalti/rain/v2`) — there is no external qBittorrent to connect to, and no `qbittorrent_url` config. A few things to know when downloads misbehave:

- **Resume database:** rain stores resume data (piece bitfields) in a bbolt DB at `~/.autoAnimeDownloader/session.db` (Windows: `%APPDATA%\.autoAnimeDownloader\session.db`), kept outside the download path so it survives a `completed_anime_path` change (which moves the derived download path along with it). If torrents don't resume after a restart, check this file exists and is writable.
- **Ports / connectivity:** rain listens on a default port range (20000–30000). There is **no UPnP/NAT-PMP** — inbound peers may require manual port forwarding. This is optional: DHT + PEX still find peers without it. If a well-seeded torrent gets zero peers, suspect a firewall blocking that range (or no outbound connectivity at all). There is no `torrent_port` config in this version.
- **"Downloaded but not in the library":** completion triggers an `organize` job that **hardlinks** the video files into `completed_anime_path`. Diagnose with:

  ```bash
  grep '"Organize:' ~/.autoAnimeDownloader/daemon.log
  cat ~/.autoAnimeDownloader/pending_jobs.json   # a stuck organize job sits here, retrying
  ```

  The download path is derived from `completed_anime_path` (a hidden subfolder inside it), so the two are always on the same volume by construction — the old "different volumes" cross-device hardlink failure is no longer possible. The most common cause today is `"no video files found in <dir>"` — the torrent has no recognized video file to link.

---

## Step 3 — Check if Nyaa returned HTTP errors

Enable debug-level logs and re-run, or check if there are already debug entries:

```bash
grep -i "Error searching Nyaa\|status 50" ~/.autoAnimeDownloader/daemon.log
```

A `502`/`503` means Nyaa was temporarily down — not a logic bug. The daemon retries on the next cycle automatically.

If the same anime fails across **multiple daemon runs on different days**, it is a logic problem, not a transient Nyaa outage.

---

## Step 4 — Find the Nyaa search URLs that were used

Enable debug logging, then look for entries from `nyaa.go`:

```bash
grep '"Searching Nyaa' ~/.autoAnimeDownloader/daemon.log | grep '"anime_name":"Failing Anime"'
```

This gives you the exact URL sent to Nyaa. Fetch it and extract the torrent names:

```bash
curl -sL "<url>" | grep -E 'title="[^"]{20,}' | grep 'href="/view/'
```

If the URL returns zero results, the search query is wrong. Proceed to Step 5.

If the URL returns results but the anime still fails, the results are being filtered out. Proceed to Step 6.

---

## Step 5 — Diagnose the search query

The query is built from the anime title variants. Entry point: `search.go:buildTitleVariants` → `nyaa.GenerateSearchTitleVariants`.

Check what variants are generated for the failing anime:

1. Look at the Anilist romaji and english title (visible in the debug log field `title`).
2. Trace `GenerateSearchTitleVariants` (`nyaa/nyaa.go`) — it tries: clean romaji → original romaji → clean english → original english.
3. Check `ExtractAnimeSeasonPart` (`daemon/helpers.go`) — season/part affect how the query is constructed and filtered.

Search [nyaa.si](https://nyaa.si) manually with each variant to find which one (if any) returns results. If none work:

- The anime may not be available on Nyaa under any of those names.
- The fansub community may use a different title — compare against what's actually posted on Nyaa.
- Consider improving `GenerateSearchTitleVariants` to generate better variants (e.g., stripping subtitles after `:`, trying shorter prefixes).

---

## Step 6 — Diagnose result filtering

If the Nyaa URL returns results but none reach `attemptDownloadWithRetries`, results are being dropped somewhere in the scraping/filtering pipeline. Trace through `nyaa/nyaa.go`:

**6a. Ignore patterns** (`reIgnorePatterns` in `nyaa_regex.go`):
Dub, raw, hardcoded, HC, re-encode are dropped. Check if the available torrents on Nyaa for this anime are all in one of these categories.

**6b. Episode number extraction** (`reEpisodePatterns`):
For single-episode searches, a torrent is only kept if its extracted episode number matches the target. Check if the torrent names on Nyaa use an unusual pattern not covered by `reEpisodePatterns`.

**6c. Season/Part filtering**:
If `ExtractAnimeSeasonPart` returns a season or part, results are filtered to match. Check if the season/part extracted from the title is correct. A wrong extraction causes all results to be dropped. Watch for sequels titled with only a roman numeral (e.g. "... II") — `extractSeason` (`nyaa.go`) has a roman-numeral fallback (`reRomanSeason` in `nyaa_regex.go`, decision 20) precisely because fansub releases for these often carry no other season marker; if a new roman-numeral case still fails, check whether the numeral is spelled differently in the torrent name than in the AniList title.

**6d. Batch detection** (`reBatchPatterns`):
Single-episode searches drop batch torrents. If Nyaa only has a pack available, the single-episode search will return nothing (correct behavior — the pack search/selection path, run earlier over the same anime search, should have caught it first; see [decisions.md #59-61](decisions.md)).

**6e. Title match / Jaccard filtering** (`titleMatchesQuery` in `nyaa_match.go`):
This check runs before episode/season/batch filters and rejects torrents in two ways:
1. All query title tokens must appear in the torrent name.
2. Jaccard similarity between query tokens and torrent tokens must meet a length-dependent threshold: ≥ 0.8 for queries ≤ 3 tokens (strict — prevents "SAO" matching "SAO Alternative Gun Gale Online"), or ≥ 0.4 for queries ≥ 4 tokens (looser — allows long queries to match fansubs that embed both title translations in the torrent name).

Metadata tokens not in `titleTechnicalTokens` inflate the torrent's token count and lower Jaccard — this is the most common cause of `results: 0` when Nyaa clearly has the anime.

To diagnose: trace `extractTitleTokens` on both the query and a torrent name, then compute Jaccard manually:

```
query tokens   = extractTitleTokens("<search variant>")
torrent tokens = extractTitleTokens("<torrent name from Nyaa>")
jaccard        = |intersection| / |union|
```

Se der abaixo do threshold, confira também a segunda tentativa antes de mexer no allowlist: o match
repete as duas checagens sobre cada `altTitleCandidates(<tail depois do marcador>)` (decisions.md
#75), então o nome pode passar por um título alternativo mesmo com o corte reprovando.

If Jaccard < 0.8, add the offending torrent-side tokens to `titleTechnicalTokens` in `nyaa_match.go` (streaming service tags like `nf`/`amzn`/`cr`, codec fragments like `eac3`/`ddp2`, subtitle markers like `multisub`/`multi`/`subs`, etc.).

**6f. Nenhum pack sobrevive — o anime baixa episódio a episódio mesmo com pack no Nyaa**

Sintoma-âncora no log: existem linhas `Attempting to download episode` para todos os episódios, e
**não existe** a linha `Using batch torrents to cover the pending window` (`daemon/episodes.go`).
Isso quer dizer que a busca voltou com resultados, mas nenhum pack chegou ao `pickBatches` — o
problema é filtro, não ausência de pack. Confirme na mesma busca: `Found anime torrents on Nyaa`
com `results: N` e todos os nomes sendo episódio avulso.

Receita para achar **qual** filtro matou cada pack (foi o que fechou o caso do Mushoku Tensei II
Part 2, `166873`):

1. Extraia os nomes crus da busca: as linhas `Raw Nyaa row` do passe em
   `~/.autoAnimeDownloader/daemon.log` (ou do `debug.jsonl` do Step 0).
2. Rode, para **cada** nome de pack, as funções do pacote `nyaa` que decidem o descarte —
   `isBatch`, `extractSeason`, `extractPart` e `titleMatchesQuery` — com a query que o daemon usou
   de verdade (a variante, não o título da AniList).
3. Monte a tabela `torrent × batch × part × match`. A coluna que der `false` é o filtro culpado, e
   cada linha pode morrer por um motivo diferente.

Os três culpados já vistos, todos na mesma investigação:

- ~~**`titleMatchesQuery` falso porque `truncateAtFirstMarker` comeu o título alternativo.**~~
  **CORRIGIDO** (decisions.md #75): quando o corte no marcador falha, `titleMatchesQuery` tenta cada
  título alternativo que vem depois dele (grupos entre parênteses, segmentos separados por barra).
  Se você ainda vir um pack morrer neste filtro, verifique se o título alternativo está num formato
  que `altTitleCandidates` não enxerga — colchetes e texto solto ficam de fora de propósito.
- **Filtro duro de part matando pack de season inteira.** Com `requestedPart != nil`, pack sem
  marcador de part explícito é descartado — e pack de season inteira normalmente não tem esse
  marcador, embora contenha os episódios pedidos. Ver `sources.md`, "Granularidade e numeração dos
  packs".
- **`extractPart` lendo `Part 1 + Part 2` como `1`.** Devolve o primeiro número que casa, então um
  pack que cobre as duas metades é descartado para a busca da part 2.

**Comparação que fecha o diagnóstico rápido:** rode o mesmo passo na entrada **irmã** sem "Part" no
título (a Part 1 da mesma season). Se ela acha pack e a Part 2 não, `requestedPart != nil` está no
caminho — o filtro de part nem roda na irmã.

Verifique também se a variante de título certa chegou a ser tentada: `searchNyaaWithVariants`
(`daemon/search.go`) **para na primeira variante que devolve qualquer resultado**, e
`GenerateSearchTitleVariants` põe o romaji primeiro. Se o romaji trouxe 24 episódios avulsos, a
variante em inglês — que poderia casar com os packs — nunca rodou.

> Este suspeito foi **medido e descartado** no corpus (decisions.md #76): depois de #75 não há anime
> em que uma variante posterior ache pack que o romaji não achou. Confirme antes de mexer no laço —
> se você tem um caso real, ele é a evidência que reabre a decisão.

---

## Step 7 — Write a failing test

Once the root cause is identified, write a test before fixing:

- For search query issues: add a case to `src/tests/unit/nyaa_test.go` with the failing title and assert the correct variant is tried.
- For regex/filter issues: add a torrent name to the relevant test table in `nyaa_test.go` and assert the correct episode/season/part is extracted (or that the torrent is not incorrectly filtered).
- For `ExtractAnimeSeasonPart` issues: add a case to `src/tests/unit/season_part_test.go`.

Run tests:

```bash
go test ./...
```

---

## Step 8 — Fix and verify

Fix the logic, run tests, then trigger a manual download cycle to confirm:

```bash
build/autoanimedownloader check
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

**Rebuild before verifying** — changes only apply after a new binary is built:

```bash
go build -o build/autoanimedownloader ./src/cmd/daemon
```

Then trigger and check:

```bash
build/autoanimedownloader check
grep '"Failed to download' ~/.autoAnimeDownloader/daemon.log | tail -20
```

If `attempts` is now `> 0` (even if some fail), the search is working and the fix moved the problem downstream.
