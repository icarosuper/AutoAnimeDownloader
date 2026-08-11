# Seleção de Torrents em Animes Antigos — Plano de Correção

Diagnóstico dos downloads avulsos de 2026-08-11 (`~/.autoAnimeDownloader/daemon.log`, 11:53–12:03) e o plano de correção
dividido em partes independentes. Cada parte é implementável e testável sozinha; a ordem sugerida é a listada.

Veredito do usuário: **opção C** para o ranking (tiers de saúde agora, score ponderado só se não bastar) **+ correções a–d**,
mais as Partes F e G (paginação e query) aprovadas na sequência.

Ordem: **A → B → C → D** consertam os bugs observados; **E** conserta a escolha entre candidatos; **F → G** aumentam e
melhoram o pool de candidatos, e só valem depois que A/B/E estiverem de pé.

## Evidência da rodada

| Anime | Estratégia | Torrent escolhido | Resultado |
|---|---|---|---|
| Takopi | batch | `[EMBER] Takopii no Genzai (Batch)` | ok |
| My Little Monster | batch | `[HorribleSubs] Tonari no Kaibutsu-kun (1-13) [720p]` | único candidato a passar o filtro |
| Naruto | multi-ep | `[Erai-raws] Naruto - 001 ~ 220 [480p]` salvo como "Episode 1" | batch vazou pela busca de episódio |
| One Piece | multi-ep + single | — | 0 resultados em todas as buscas |
| Berserk | batch | `[Erai-raws] Kenpuu Denki Berserk - 01 ~ 25 [1080p]` | ok |
| K-ON! | batch | `[MTBB] K-ON! S1 (BD 1080p)` | ok |
| With You | batch | `[SubsPlease] Kimi to Koete (01-12) [Batch]` | ganhou por fansub; `health` nunca foi avaliado |
| Hentatsu | multi-ep | `[Artsuki] Hentatsu - 4..7` | ok (só existem esses) |
| Naruto Shippuden | multi-ep + single | `... - 001 ~ 079 [BATCH]` + `[DB] Naruto Shippuuden Movie 3 (Eng Sub).avi` | batch vazado + filme aceito como ep 3 |

---

## Parte A — `ScrapNyaaForMultipleEpisodes` não filtra batch

**Arquivo:** `src/internal/nyaa/nyaa.go:434-522` (`parseRow` de `ScrapNyaaForMultipleEpisodes`)

A busca de episódio único tem o guard `if isBatch(name) { return }` (`nyaa.go:316`); a multi-episódio **não tem**.
`[Erai-raws] Naruto - 001 ~ 220 [480p][Multiple Subtitle]` → `extractEpisodeNumber` casa `- 001` → vira "episódio 1"
comum, com `skipSubfolder: false`.

Consequências confirmadas no log:
- pack de 220 episódios baixado como 1 registro, furando `max_episodes_per_anime` **e** `max_batch_episodes`
  (que barraram corretamente o caminho de batch: 220 > 30);
- salvo com `IsBatch: false` → `jobs.go:358` organiza como episódio único. Com `rename_files_for_jellyfin` ligado,
  os 220 arquivos virariam "Naruto - E01";
- mesmo caso no Shippuden (`001 ~ 079 [BATCH]` → "episódio 1").

**Mudança:** adicionar o mesmo guard `isBatch` no `parseRow` da multi-episódio.

Decisão em aberto (resolver na implementação): descartar o batch de vez (simétrico com a busca single, diff de 3 linhas)
ou aceitá-lo marcando `IsBatch: true` e submetendo ao teto de `max_batch_episodes`. O caminho de batch já existe em
`resolveSearchStrategy` e roda **antes** da multi-episódio, então um batch que valha a pena já teria sido pego lá —
o vazamento aqui é justamente o que o teto rejeitou. Preferência: **descartar**.

**Teste:** `nyaa_test.go` — resultado com `- 001 ~ 220` na busca multi-episódio não pode aparecer.

---

## Parte B — `ScrapNyaa` não filtra filme/OVA/special

**Arquivo:** `src/internal/nyaa/nyaa.go:289-376` (`parseRow` de `ScrapNyaa`)

`isMovie` (`nyaa.go:184`) só é usada na direção inversa, em `ScrapNyaaForMovie`. Na busca de episódio nada rejeita filme.

Rastro de `[DB] Naruto Shippuuden Movie 3 (Eng Sub).avi` na busca do episódio 3:
1. `extractEpisodeNumber` — padrão `\s(\d{1,4})\s*\(` casa `" 3 ("` → episódio 3;
2. `titleMatchesQuery` — `truncateAtFirstMarker` corta em `" 3 ("`, sobra `"[DB] Naruto Shippuuden Movie"`;
   `movie` está em `titleTechnicalTokens`, então os tokens viram `{naruto, shippuuden}` → Jaccard 1.0 → aceito.

**Mudança:** guard `if isMovie(name, animeName, false) { return }` no `parseRow` de `ScrapNyaa`.

Atenção: `isMovie` retorna `true` quando o nome **não tem** marcador de episódio nem é batch (`nyaa.go:203`). Como o
`parseRow` já exige `*animeEpisode == episode` logo depois, esse ramo é redundante mas inofensivo — o guard tem que
vir depois da extração do episódio, ou vai rejeitar nomes que a extração aceitaria.

**Teste:** `nyaa_test.go` — `[DB] Naruto Shippuuden Movie 3 (Eng Sub).avi` não pode casar com a busca do episódio 3.

---

## Parte C — Piso de saúde: descartar torrent morto

**Arquivos:** `src/internal/nyaa/nyaa.go` (os quatro `parseRow`), `src/internal/files/filemanager.go` (config)

Hoje não existe piso: torrent com 0 seeders é candidato válido. O `Movie 3` da rodada está com **0 peers** — foi o único
candidato do episódio 3 e foi baixado mesmo assim.

**Mudança:** descartar `parseSeeders(seeders) < MinSeeders` no parse, antes de qualquer ordenação.

Config nova: `min_seeders`, default `1` (só barra o torrent literalmente morto). `<= 0` desliga o filtro, seguindo a
convenção de `max_batch_torrent_size_gb` e afins.

Checklist de campo de config novo: `docs/agents/conventions.md`.

**Teste:** resultado com `-` / `0` na coluna de seeders não entra na lista.

---

## Parte D — Logar seeders/tamanho dos candidatos

**Arquivo:** `src/internal/nyaa/nyaa.go` — os quatro `logger...Strs("matched_names", torrentNames(results))`

Hoje o log só registra o nome. Auditar "por que esse ganhou" exige reproduzir a busca à mão — foi o que travou o
diagnóstico do My Little Monster e do With You.

**Mudança:** trocar `torrentNames` por um formatador que inclua seeders, tamanho e o health score calculado, na ordem
já ordenada. Algo como `"[SubsPlease] ... | S:412 | 1.4GiB | h=4.21"`.

Sem teste dedicado (é log).

---

## Parte E — Ranking: `health` em faixas log (opção C)

**Arquivo:** `src/internal/nyaa/priorities.go:97` (`criterionCompare["health"]`), `filemanager.go` (default de config)

Dois problemas somados hoje:

1. **`health` é o 7º de 8 critérios num sort lexicográfico** (`priorities.go:150`) — fansub/resolução decidem antes e
   ele nunca é alcançado. No With You os 3 candidatos são 1080p, aí `fansub` desempata
   (subsplease=0, judas=2, ember=5) e acabou. `health` não chegou a ser consultado.
2. **`SortTorrentResults` filtra a `criteria_order` para `{uncensored, resolution, fansub, health, size}`**
   (`episodeCriteria`, `priorities.go:174`) e é usada **também para batch** (`nyaa.go:665`) — ou seja, `source`,
   `codec` e `audio` são descartados justamente onde mais importam (BD vs WEBRip num pack de anime antigo).

**Mudança:**
- `criterionCompare["health"]` passa a comparar **faixas** (tiers) em vez do score cru: bucket log2 de seeders
  (ex.: `0`, `1-4`, `5-19`, `20-99`, `100-399`, `400+`). Dentro da mesma faixa, empate → o critério seguinte decide.
  Efeito pretendido: 400 seeds ganha de 12; 300 vs 400 empata e o fansub decide.
- `health` sobe na `criteria_order` default: depois de `resolution`, antes de `fansub`.
  Nova ordem: `uncensored, source, resolution, health, codec, fansub, audio, size`.
- Config existente já tem `criteria_order`, então quem quiser a ordem antiga só reordena. **Não migra config existente
  automaticamente** — `config.json` do usuário já tem `criteria_order` gravado e o unmarshal preserva. Anotar isso na
  entrega: para valer no ambiente atual, o usuário edita o `criteria_order` dele à mão.
- Revisar `episodeCriteria`: manter `source`/`codec` para batch é uma mudança separada (ver "Fora de escopo").

**Score ponderado (opção B) fica para depois** — só se as faixas não resolverem. Seria trocar o sort lexicográfico por
soma ponderada com `criteria_weights` novo no config.

**Teste:** `priorities_test.go` — dois 1080p, um SubsPlease com 12 seeders e um EMBER com 400: EMBER ganha.
Dois 1080p com 300 e 400 seeders (mesma faixa): o fansub decide.

---

---

## Parte F — Paginação: adaptativa, mais funda, em paralelo

**Arquivos:** `src/internal/nyaa/nyaa.go:211` (`fetchNyaaPage`), os quatro pontos de busca, `filemanager.go` (config)

Contexto medido na rodada: **não existe throttle** nas requisições ao Nyaa — os ~3s por episódio são latência pura
(2 páginas sequenciais dentro do loop de variantes). 458 fetches em 11 min ≈ 0,7 req/s sustentado.

Estado atual, por tipo de busca:

| Busca | Função | Páginas | Passa por `fetchNyaaPage`? |
|---|---|---|---|
| episódio único | `ScrapNyaa` | 2, sempre | sim (logado) |
| multi-episódio | `ScrapNyaaForMultipleEpisodes` | 2, sempre | sim (logado) |
| batch | `ScrapNyaaForBatch` | **1** | não — `httpGet` direto (`nyaa.go:570`) |
| filme | `ScrapNyaaForMovie` | **1** | não — `httpGet` direto (`nyaa.go:700`) |

Duas coisas erradas aí: a página 2 é buscada **incondicionalmente** (`nyaa.go:285` e `:430`) mesmo quando a página 1
já deu candidatos de sobra; e batch/filme leem só a página 1 — justamente onde um pool maior paga (caso My Little
Monster), além de não aparecerem no log por não passarem pelo `fetchNyaaPage`.

Nota de escopo: **paginação não conserta nenhum bug diagnosticado** — os 12 season packs do My Little Monster, os 3
candidatos do With You e as escolhas ruins de Naruto/Shippuden estavam todos na página 1. O ganho é pool maior para o
ranking, e só passa a valer depois de A/B/E. Teto fixo de 5 páginas sempre custaria ~1200 fetches por passada
(240 buscas × 5), a cada `check_interval` de 10 min — ~2 req/s em rajadas contra o nyaa.si, onde se espera
429/Cloudflare. Daí a versão adaptativa.

**Mudança:**
- **Paginação adaptativa**: buscar página 1; continuar para N+1 só enquanto os candidatos aceitos estiverem abaixo de
  um piso. Buscas que já resolvem na página 1 (Takopi, Berserk, K-ON) passam a gastar 1 fetch em vez de 2 — a economia
  paga o teto maior sem subir o tráfego médio.
- **Teto configurável** `max_search_pages`, default 5-6, valendo só nesse regime.
- **Paralelismo modesto** (2-3 concorrentes) quando de fato precisar descer; sequencial não faz sentido quando já se
  sabe que vai buscar várias.
- **Orçamento de fetches por anime por passada.** Sem isso, um anime insolúvel (One Piece) queima o teto inteiro toda
  passada, para sempre.
- **Uniformizar batch/filme** para `fetchNyaaPage` + a mesma paginação. Ganha o log junto.

**Teste:** com um mock que serve N páginas, uma busca que satisfaz o piso na página 1 não pode pedir a página 2;
uma que não satisfaz tem que pedir até o teto e parar lá.

---

## Parte G — Zero-padding da query em séries longas

**Arquivo:** `src/internal/nyaa/nyaa.go:268` (`params.Set("q", fmt.Sprintf("%s %d", query, episode))`)

O fix real do One Piece, e não é profundidade de página. `q=one piece 1` casa com tudo que contém "1"; ordenado por
seeders, as 150 primeiras linhas são os episódios 1160-1173 e o episódio 1 nunca aparece — nem em 375 linhas.

Grupos numeram série longa com zero-padding: `[Judas] One Piece 001-574`, `[Erai-raws] One Piece - 001 ~ 220`,
`[Erai-raws] Naruto Shippuuden - 001 ~ 079`.

**Mudança:** quando o total de episódios do anime passa de ~100, buscar o episódio com padding de 3 dígitos
(`one piece 001`). Provavelmente como variante adicional de query, não substituindo a atual — o padding é convenção,
não regra, e o não-padded ainda pega quem numera solto.

**Teste:** anime com `Media.Episodes > 100` gera a query com `001`; anime curto mantém `1`.

---

## Fora de escopo desta rodada

Anotado para não se perder; nada aqui foi aprovado.

- **`isBatch` exige marcador explícito → animes antigos perdem os melhores packs.** Causa real do My Little Monster
  (não o ranking): a página tinha 12 season packs BD 1080p — `[neoHEVC] My Little Monster [BD 1080p x265 HEVC AAC]`,
  `[Cleo] Tonari no Kaibutsu-kun | My Little Monster [10bit BD1080p]`, `[Chihiro]_..._[1920x1080_Blu-ray_FLAC]` etc.
  Nenhum tem `(1-13)`, `[Batch]` ou `S01`, então `isBatch` (`nyaa.go:109`) rejeita todos; sobrou o HorribleSubs 720p.
  Eles também não aparecem na busca por episódio (não têm número de episódio). Release de anime antigo é exatamente
  esse formato. Saída provável: relaxar `isBatch` **quando a busca é de batch** (título casa + sem número de episódio
  + tamanho compatível com temporada), ao custo de mais risco de puxar OVA/filme.
- **Batch parcial que cobre a faixa pedida.** Mesmo com a Parte G, One Piece só fecha assim: `[Judas] One Piece
  001-574` e `[Anime Time] One Piece (0001-1071)` existem, mas `max_batch_episodes: 30` barra — corretamente, e não é
  pra afrouxar. Exigiria entender que um pack cobre os episódios pedidos e baixar só a faixa.
- **Variantes de título param na primeira que retorna algo** (`search.go:91`). "tonari no kaibutsukun" deu 0,
  "Tonari no Kaibutsu-kun" deu 1 → parou. A variante "My Little Monster" nunca foi tentada. O pool de candidatos é o
  do primeiro título que casou, não a união.
- **`episodeCriteria` descarta `source`/`codec` também em batch** (ver Parte E, item 2). Corrigir isso muda a escolha
  em todo batch já baixado, então merece rodada própria.

## Depois de cada parte

Por `CLAUDE.md`: `go test ./...`; atualizar `docs/agents/config.md` (Parte C), `docs/agents/architecture.md` se surgir
símbolo novo, e `docs/agents/decisions.md` se a solução for não-óbvia (as faixas de saúde da Parte E são candidata).
Sem commit sem permissão.
