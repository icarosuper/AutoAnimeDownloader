# Fontes de torrent — levantamento

Leia antes de acrescentar uma fonte nova ao daemon. Registra o que foi **medido** em agosto/2026,
não o que se costuma dizer sobre cada site. Hoje o daemon tem uma fonte só: Nyaa, via
`internal/nyaa` (scraping de HTML com goquery).

**Ressalva sobre os vereditos originais (1ª rodada, ago/2026):** foram formados medindo anime
**moderno**, população em que todas as fontes são equivalentes por construção — mesmos uploaders,
mesmos releases, todos vivos. Por isso não enxergavam a diferença que importa para anime velho e
nicho. As medições de 16/ago (marcadas abaixo) corrigem o que caiu.

O seam para acrescentar fonte já existe: a interface `nyaaSearcher`, injetada em
`daemon.processAnimeEpisodes` (ver `decisions.md` #14). Uma fonte nova é uma implementação nova,
não uma refatoração.

---

## Nyaa (fonte atual)

`https://nyaa.si/?f=0&c=1_2&q=<titulo>&s=seeders&o=desc`, paginado por `&p=N`.

**Limites que importam para o algoritmo:**

- **Não tem filtro de tamanho na query.** Nem `minsize`/`maxsize`. Quem filtra tamanho é o
  daemon, depois de baixar a página. (O `minage`/`maxage`/`minsize` que aparece em issues de
  scraper é do NyaaPantsu, um fork diferente, não do `nyaa.si`.)
- **`s=seeders&o=desc` é péssimo para pack de série longa.** A ordenação enterra pack de 40-100
  seeders sob centenas de episódios semanais com 200-1000 seeders. Medido em One Piece: o pack
  parcial mais útil (61 GiB) só aparece na **página 3**. Foi a causa do bug de "One Piece nunca
  baixa" — ver `decisions.md` #80.
- **Tem RSS** (`?page=rss`), com tags próprias de seeders/leechers/infohash/size — mais barato de
  parsear que HTML e imune a mudança de layout. **Mas não substitui o HTML** (medido 16/ago):

  | | `s=seeders&o=desc` | `&p=N` | itens/página |
  |---|---|---|---|
  | HTML | respeitado — `332 147 130 107 72 …` | páginas diferentes | 75 |
  | RSS | **ignorado** — sai `5 3 9 0 6 6 46 …` (timestamp desc) | **ignorado** — md5 dos guids de `p=2` idêntico ao de `p=1` | 75 |

  Ou seja: o RSS entrega os 75 uploads mais recentes e acabou. Não dá para varrer as 5 páginas nem
  ordenar por seeders. Trocar HTML→RSS some com o goquery e some com a busca junto.
- `f=1` filtra só "trusted", `f=2` filtra "no remakes". Hoje o daemon usa `f=0`.

**Por que o scraping de HTML continua:** `s=seeders&o=desc` combinado com `&p=N` só existe na
interface HTML. Nenhuma das alternativas medidas (RSS do Nyaa, JSON do AnimeTosho) ordena por
seeders. O goquery não é dívida acidental — é o preço da única interface que ordena.

### Página de detalhe (`/view/<id>`) — o que ela acrescenta

Medido em 28/ago/2026 em `nyaa.si/view/1323474` (`[Judas] One Piece 001-574`, upload de jan/2021,
171.8 GiB, 574 arquivos) — pior caso de propósito: pack gigante e antigo.

| Campo só do detalhe | Serve? |
|---|---|
| **Lista de arquivos** (`div.torrent-file-list`, `<li>` com nome + `span.file-size`) | **Sim.** Os 574 arquivos vieram completos, com nome e tamanho, em árvore de pastas. |
| `Completed` (snatches; 8723 no caso) | Sinal de saúde fraco — não decide nada que `Seeders` já não decida. |
| Submitter, info hash, categoria | Já derivados do nome/magnet pelo daemon. |
| Descrição do uploader (fonte, áudio, subs, notas) | Texto livre. Heurística ruidosa em cima de prosa de fansub — não vale contra `ShouldIgnore`, que decide por nome. |

Flag trusted/remake **não** exige o detalhe: já vem na classe do `<tr>` da listagem
(`success`/`danger`).

**Custo medido:** 175 KB naquela página — mas ela é o pior caso (574 arquivos). Remedido em
29/ago/2026 sobre 7 packs reais (Vinland Saga, Mushoku Tensei, AoT, One Piece): **30 KB de média**,
contra 125 KB de uma listagem inteira. Uma página de detalhe custa ~1/4 de uma busca, não mais que
ela. O que continua caro é buscar detalhe de **todas** as linhas: 75 × 30 KB ≈ 18x uma busca.

**A diferença que importa contra o AnimeTosho:** a lista de arquivos do Nyaa é gerada do próprio
`.torrent` e **funciona em conteúdo antigo**. O `files[]` do AnimeTosho não (vem `[]` para entrada
velha — ver a ressalva na seção dele). Para o problema de série longa, a fonte certa da lista de
arquivos é o próprio Nyaa, não o AnimeTosho.

### Que caso de busca vale a pena

- **Episódio individual — não.** Um arquivo. Nenhuma entrada nova para a decisão.
- **Filme — quase nada.** Só distinguiria um resultado "filme" que na verdade é pasta de BD com
  extras ou coletânea. Marginal.
- **Pack — sim, e é o único que paga.** Duas coisas, de pesos bem diferentes:

**1. Cobertura real do pack — isso é bug, não melhoria.** `extractBatchInfo`
(`internal/nyaa/nyaa.go`) lê a faixa só do **nome**. Faixa desconhecida vira `EndEpisode == 0`, e
daí `coveringBatch` (`internal/daemon/episodes.go`) trata o pack como completo, `pickBatches` para
o cursor ali e `assignBatches` grava `batchStart/batchEnd = 1..total`. Cada episódio da janela vira
um `EpisodeStruct` apontando para o hash do pack. Se um `"(Season 1+OVA) [Batch]"` cobre só 1-12 e
a janela era 1-50, os 38 restantes ficam **registrados como baixados**: o Librarian linka apenas os
arquivos que existem e nada reconcilia os registros com o disco depois, então esses episódios nunca
voltam para a busca. A lista de arquivos responde a cobertura real ANTES de adicionar o torrent.

**2. Teto por episódio em vez de por tamanho total** — mudança de política, não correção. O pack
acima reprova em qualquer `max_batch_torrent_size_gb` sadio, mas dá ~300 MiB/episódio, que é ótimo.
**Não economiza download:** o `rain` não seleciona arquivo (ver "O que NÃO resolve" do AnimeTosho),
então o pack inteiro desce do mesmo jeito — muda só o critério de aceitar. Só vale se o teto atual
estiver reprovando pack que o usuário quer.

**Como manter barato, se for implementado:** só linha de **pack**, e só quando o dado muda a
decisão — nome sem faixa (`EndEpisode == 0`) ou pack acima do teto. Dá ~1-3 requisições por anime.
E **fora do `parseRow`**, depois do sort/filtro: dentro do loop de página vira uma requisição por
linha.

**O que continua sem resposta:** swarm morto, seleção de arquivo dentro do pack (limitação do
`rain`), e ordenação.

---

## Granularidade e numeração dos packs — o que a fonte entrega vs. o que a AniList pede

Medido em 28/ago/2026, a partir das linhas `Raw Nyaa row` de passes reais. Não é detalhe de
parsing: é a razão de o daemon e a fonte discordarem sobre o que é "um episódio".

**1. Pack é quase sempre de season inteira, não de cour.** A AniList quebra cour/part em **media
ids separados**, cada um com numeração própria a partir de 1 (ver `decisions.md` #45 e #71). Os
grupos não seguem esse recorte: o pack lançado cobre a season toda. Consequência prática, medida em
Mushoku Tensei II Part 2 (`166873`): os únicos packs disponíveis eram de season inteira, e a busca
montada a partir da entrada "Part 2" os descartou por não trazerem marcador de part explícito. Um
pack "sem part" não é um pack errado — é o formato normal de lançamento.

**2. Não existe uma convenção de numeração; existem quatro.** Todas observadas no mesmo período:

| Release | Numeração no nome/arquivos | Convenção |
|---|---|---|
| `[Erai-raws] … Honki Dasu - 00 ~ 12 [BATCH]` (MT S2 Part 1) | `00 ~ 12` | reinicia por **season** |
| `[EMBER] … (Season 2 \| Part 2) … (Batch)` (MT S2 Part 2) | `1..12` | reinicia por **part** |
| `[Diddy] Mushoku Tensei - S02 (BD 1080p HEVC Opus)` | `1..25` | contínua pela **season inteira** |
| SubsPlease / Erai-raws, AoT Final Season Part 2 | `76..87` | **absoluta pela série** |

A quarta linha é a que costuma surpreender: `76..87` bate exatamente com a soma acumulada dos
episódios da cadeia de `PREQUEL` na AniList (ver `decisions.md` #71). Ou seja, parte dos grupos usa
o eixo absoluto da franquia como numeração de arquivo — e nenhum deles anuncia qual convenção
adotou.

**3. Por isso mapear a numeração do pack é sempre palpite, e o desempate está na contagem de
arquivos.** Não dá para ler a convenção do nome. Dá para reduzir a poucas hipóteses (offset 0,
offset do prequel imediato, offset da série inteira) e usar a **quantidade de arquivos** do pack
para escolher: um pack com 25 arquivos numerado a partir de 1 só pode ser season contínua, não um
cour de 12. É a mesma evidência que `packEpisodeOffset` já usa no organize (`decisions.md` #68), e
é o que torna a lista de arquivos da página de detalhe do Nyaa (seção "Página de detalhe", acima)
valiosa **antes** de baixar, não só depois.

**4. Um "Part 1 + Part 2" no nome não é "part 1".** `[Fuchs] … - S02 … (Season 2) (Part 1 + Part 2)`
cobre as duas metades, mas um extrator que devolve o primeiro número que casar lê `1`. Ler marcador
de part é sempre frágil; perguntar "esse pack cobre a janela pendente?" não é.

> **Implementado** (29/ago/2026, `decisions.md` #79): no caminho de pack o marcador de part só
> rejeita conflito declarado, `extractPart` devolve `nil` para nome com duas parts, e as três
> hipóteses de numeração desta seção viraram `daemon.packAxis` — com o span do nome no lugar da
> contagem de arquivos como desempate. A contagem de arquivos da página de detalhe seria um
> desempate mais forte, mas só vale a pena junto do scrape de detalhe, que o `TODO.md` acompanha
> pela cobertura do pack.

---

## AnimeTosho — bom complemento, mau substituto

**Atenção ao domínio.** `animetosho.org` congelou em maio/2026 e não indexa mais nada. A
continuação é `animetosho.xyz`, e a API/feeds migraram para `feed.animetosho.xyz` (os endpoints do
domínio antigo saíram em 1º/agosto/2026). Ambos verificados respondendo em agosto/2026.

Ele **indexa o próprio Nyaa** (campos `source: "nyaa"`, `nyaa_id`, `article_url`). Não é um acervo
concorrente — é um front-end melhor para o acervo que o daemon já usa.

**Quanto "melhor front-end" quer dizer, medido em 16/ago** (`?q=cowboy+bebop`, 75 resultados):
`nyaa_id` não-nulo em **75/75**, `anidex_id` não-nulo em **0/75**, `article_title == "Nyaa"` em
100%. Mesmo infohash ⇒ mesmo swarm. Para o problema de "anime velho acha candidato morto", o ganho
é **zero** — trocar de indexador não ressuscita swarm.

### API

`GET https://feed.animetosho.xyz/json?q=<query>` → array JSON. Campos por item:

```
title, torrent_name, magnet_uri, torrent_url, info_hash,
seeders, leechers, total_size, num_files, timestamp,
nyaa_id, article_url, anidb_aid, anidb_eid, nzb_url
```

Mapeia quase 1:1 em `nyaa.TorrentResult` — some o goquery e some a fragilidade de contar `<td>`.

**Dois limites medidos em 16/ago que impedem usá-lo como substituto do Nyaa:**

- **Não ordena por seeders.** `order=seeders-d` é aceito e **silenciosamente ignorado** (volta ao
  default, timestamp desc). Só `order=size-d` funciona. Com 75 itens/página, varrer 5 páginas viria
  a ser "os 375 uploads mais recentes que citam o nome" — o pior caso possível para anime antigo, e
  reintroduziria o bug de `decisions.md` #80 numa forma pior. (`page=N` **funciona**, ao contrário
  do RSS do Nyaa.)
- **O campo `seeders` é cache velho.** Em `?q=cowboy+bebop`, a idade de `tracker_updated` teve
  mediana de **22 dias** e máximo de **95 dias**. Comparação dos topos no mesmo instante:

  ```
  nyaa  (ao vivo): 333 147 130 105 72
  tosho (cache):   191  72  44  42 37
  ```

  `min_seeders`/`filterBySeeders` passariam a decidir sobre número desatualizado, e o erro é
  sistematicamente **para baixo** — falso negativo exatamente na direção do sintoma que motivou a
  investigação de fontes múltiplas.

### Lista de arquivos do pack ANTES de baixar

`GET https://feed.animetosho.xyz/json?show=torrent&id=<id>` acrescenta `is_batch`, `files[]` e
`primary_file_id`. Cada item de `files[]` traz `filename`, `size` e `crc32`.

Isso responde "quais episódios este pack cobre e quanto pesa cada um" **sem** adicionar o torrent
e esperar o metadata chegar. É o dado que falta hoje para medir teto por episódio coberto em vez
de por tamanho total do pack.

**Ressalva medida:** entradas antigas vêm com `files: []`. O pack `[Judas] One Piece 575-782`
(id 369911) devolve lista vazia; um batch recente do SubsPlease devolve os 26 `.mkv` com nome e
tamanho. A lista de arquivos só é confiável para conteúdo recente — trate `files: []` como
"desconhecido", não como "pack vazio".

### O que NÃO resolve

Não permite baixar só alguns arquivos de um pack. Isso é limitação do **cliente**, não da fonte:
o `rain` v2.3.1 não tem seleção de arquivo (`Torrent.Files()`/`FileStats()` são read-only, e não
existe nada de prioridade/skip por arquivo no módulo). Seleção por episódio dentro de pack exigiria
fork do `rain` ou troca do cliente.

Também não resolve swarm morto (mesmo infohash) nem substitui a ordenação por seeders — ver acima.

### Quando ele vale

Como fonte **adicional**, quando aparecer requisito que o HTML do Nyaa não atende: match por
`anidb_aid` em vez de string de título. **`files[]` saiu dessa lista** (medido 28/ago): a página
de detalhe do próprio Nyaa entrega a lista de arquivos e, ao contrário daqui, funciona em conteúdo
antigo — ver "Página de detalhe" na seção do Nyaa. Como **substituto** do Nyaa hoje, não vale: ganho de parsing, perda de ordenação e de
frescor de seeders.

---

## Outras fontes avaliadas

| Fonte | Veredito |
|---|---|
| **AniDex** | **Inviável** (medido 16/ago): `GET anidex.info/rss/?q=…` → **HTTP 403**, js-challenge do DDoS-Guard (`/.well-known/ddos-guard/js-challenge/`). Exigiria browser headless a cada requisição do daemon. Fora de escopo. |
| **TokyoTosho** | **Veredito anterior ("mesmo conteúdo do Nyaa") caiu.** Medido 16/ago: `rss.php?terms=…&type=1` → 147 itens, e ele **agrega outros indexadores** — trackers `t.acg.rip`, `tr.bangumi.moe`, `open.acgtracker.com`, `tracker.kamigami.org`, hosts `acg.rip` e `files.catbox.moe`. Infohash diferente ⇒ swarm potencialmente diferente: é a única candidata que poderia ressuscitar conteúdo morto. **A pegadinha:** o RSS não expõe seeders — as tags do `<item>` são só `title`, `category`, `link`, `description`, `guid`, `pubDate` (magnet vem no HTML da `description`, e o infohash em **base32**, não hex). Sem seeders, `min_seeders` não tem o que filtrar; medir "está vivo?" exige ir ao swarm. Ver a spec da Etapa 0 em `docs/superpowers/specs/`. |
| **RSS de fansub direto** (SubsPlease, Erai-raws) | Sinal limpíssimo e sem ruído para a temporada corrente; **inútil para backlog** (não hospedam catálogo antigo). Serve como "fonte preferida" para anime em exibição, não como fonte geral. |
| **Usenet / NZB** | O AnimeTosho carrega `nzb_url`. Velocidade de HTTP, sem problema de seeder, retenção longa. Exige provider pago + indexer + um caminho de download inteiro em paralelo ao `rain`. Mudança grande. |
| **Debrid** (Real-Debrid, AllDebrid) | Manda o magnet, o serviço baixa do lado dele, e você escolhe **quais arquivos** puxar por HTTP. É a única saída que dá seleção por episódio dentro de pack sem tocar no `rain`. Custa mensalidade e vira dependência externa. |

---

## Ordem recomendada

1. **Corrigir a busca de pack no Nyaa** — feito, ver `decisions.md` #80. Era bug, não falta de
   fonte.
2. **Não trocar o Nyaa por nada agora.** Nem RSS (perde ordenação e paginação) nem AnimeTosho
   (perde ordenação, seeders em cache de ~22 dias). O ganho seria de parsing; o custo é a busca.
3. **Responder a Etapa 0 antes de escrever adapter** — "existe fonte com infohash **diferente** e
   peers **vivos** onde o Nyaa está morto?". Só o TokyoTosho tem sinal para isso. Enquanto não for
   medido, arquitetura de múltiplas fontes é especulação.
4. **Lista de arquivos do pack, se virar requisito** — vem da página de detalhe do **Nyaa**
   (`/view/<id>`), não do AnimeTosho: só ela funciona em pack antigo. Escopo e custo na seção
   "Página de detalhe" acima; o segundo uso, além da cobertura real, é desempatar a convenção de
   numeração do pack (seção "Granularidade e numeração dos packs"). O AnimeTosho como fonte adicional só sobra para match por
   `anidb_aid`. Nenhum dos dois como substituto.
5. **Debrid/Usenet** — só se seleção por arquivo virar requisito de verdade. É troca de
   *protocolo*, e é a saída se a Etapa 0 for refutada.
