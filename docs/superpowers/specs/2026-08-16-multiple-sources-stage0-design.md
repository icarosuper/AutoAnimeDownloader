# Etapa 0 — Gate: existe swarm alternativo para anime velho nicho?

**Data:** 2026-08-16
**Status:** desenhado, não executado
**Escopo:** medição. Nenhuma linha de código de produção.

---

## Por que esta etapa existe

O objetivo do projeto maior é: **anime velho e nicho não baixa**. Anime da temporada
corrente baixa sem problema.

O plano original era pesquisar fontes, testar downloads, agregar resultados e implementar
adapters. As etapas de pesquisa e teste, do jeito que estavam desenhadas, mediriam a variável
errada — e é por isso que esta etapa existe antes de qualquer adapter.

O sintoma reportado não é "não acha nada". É **"acha, mas está morto"**: existem candidatos no
Nyaa, com 0-2 seeders, barrados por `min_seeders` ou travando em 0%. Isso está registrado em
`.debug_batch/report.md` (12/ago), na seção `SEM FONTE BOA NO NYAA`.

Isso tem uma consequência que descarta a maior parte do plano original:

> **Trocar de indexador não ressuscita swarm.** Se o infohash é o mesmo, o swarm é o mesmo. Uma
> fonte nova só ajuda se hospedar um infohash **diferente** com peers **vivos**.

Logo, a única pergunta que decide o projeto é a hipótese abaixo. Enquanto ela não for
respondida, qualquer discussão de arquitetura de adapter é especulação.

## Hipótese sob teste

> Para conteúdo que o Nyaa tem morto (0 seeders), existe fonte que oferece um infohash
> **diferente** com peers **vivos**?

**Se confirmada** → segue para o desenho do adapter (spec própria).
**Se refutada** → não existe fonte nova que resolva. O projeto vira Debrid ou Usenet — outro
*protocolo*, não outro indexador — e nada da arquitetura de adapter se aplica.

---

## Achados medidos em 2026-08-16

Medidos com `curl` contra os endpoints reais nesta data. Corrigem o `docs/agents/sources.md`,
cujos vereditos foram formados sobre anime moderno — a população onde todas as fontes são
equivalentes, e por isso não conseguiam ver a diferença que importa aqui.

### AnimeTosho — espelho do Nyaa, confirmado

`GET https://feed.animetosho.xyz/json?q=cowboy+bebop` → HTTP 200, 75 resultados.

| medida | valor |
|---|---|
| itens com `nyaa_id` não-nulo | **75 / 75** |
| itens com `anidex_id` não-nulo | **0 / 75** |
| `article_title` | `"Nyaa"` em 100% |

Campos disponíveis: `anidb_aid`, `anidex_id`, `article_url`, `info_hash`, `leechers`,
`magnet_uri`, `nekobt_id`, `num_files`, `nyaa_id`, `nzb_url`, `seeders`, `timestamp`,
`torrent_name`, `tosho_id`, `total_size`, `tracker_updated`.

**Veredito:** ganho **zero** para swarm morto — mesmo infohash, mesmo swarm. O `sources.md` o
elege "melhor fonte nova candidata"; isso continua válido como upgrade de *parsing* (JSON no
lugar de goquery, some a fragilidade de contar `<td>`), mas é irrelevante para o problema desta
spec. Nesta etapa ele serve apenas como **controle de método**.

### AniDex — inviável

`GET https://anidex.info/rss/?q=cowboy+bebop` → **HTTP 403**, `DDoS-Guard` js-challenge
(`/.well-known/ddos-guard/js-challenge/`).

**Veredito:** fora do escopo. Exigiria browser headless para resolver o challenge, a cada
requisição do daemon — custo desproporcional antes de qualquer evidência de que valha a pena.

### TokyoTosho — a única candidata com sinal real

`GET https://www.tokyotosho.info/rss.php?terms=cowboy+bebop&type=1` → HTTP 200, 147 itens.

Ele **agrega múltiplos indexadores**, não só o Nyaa. Trackers observados nos magnets:

```
t.acg.rip:6699          tr.bangumi.moe:6969     t.nyaatracker.com
open.acgtracker.com     tracker.kamigami.org
```

Hosts de torrent observados: `acg.rip`, `files.catbox.moe`.

Isso é infohash diferente, tracker diferente, **swarm potencialmente diferente** — exatamente a
condição que a hipótese exige. É o oposto do veredito atual do `sources.md`
("mesmos uploaders, mesmo conteúdo do Nyaa, ganho marginal quase nulo").

**A pegadinha:** o RSS **não expõe seeders**. As tags do `<item>` são, e apenas:

```
title  category  link  description  guid  pubDate
```

`title` é o nome do release (utilizável pelo pipeline de classificação). `description` é HTML e
carrega magnet, tamanho, submitter e comentário. **Nenhum campo de seeders ou leechers.**

Isso é o que força o passo 4 do procedimento: seeders é ao mesmo tempo a variável da hipótese e
o filtro de que o daemon depende (`min_seeders`, `filterBySeeders`). Nenhum feed responde
"está vivo?" — a medição tem que ir ao swarm.

---

## População

**Pré-requisito de execução:** a lista de animes nicho reais que falharam na prática. Sem ela a
medição roda na população errada — que foi como o `sources.md` chegou ao veredito que esta spec
contesta.

Critérios da lista:
- 4-6 animes que **você viu falhar**, não escolhidos por serem "velhos" no abstrato.
- Nicho ou antigo. **Sem série longa** (One Piece, Naruto, Detective Conan): a falha delas tem
  outra causa e está fora do escopo — ver "Fora do escopo".
- IDs de anime que você **não acompanha** na AniList, mesma restrição de
  `scripts/robustness-animes.txt` (`resolveAnimeDetails` usa a entrada real quando ela existe).

**Controles obrigatórios**, além da lista:

| controle | id | serve para |
|---|---|---|
| Cowboy Bebop | 1 | Baixa hoje, 26/26 com 306 seeders. Se a medição disser que ele está morto, **o método está errado** e nada do resultado vale. |
| um anime da lista de falhas | — | Confirma que o método enxerga a falha que motivou a spec. |

## Procedimento, por anime

1. **Nyaa** — `nyaa.ScrapNyaaForAnime` (já existe, reusar). Registra por candidato:
   `(nome, infohash, seeders)`.
2. **TokyoTosho** — `GET rss.php?terms=<query>&type=1`. Registra `(título, infohash, tracker)`.
   O infohash sai do `magnet:?xt=urn:btih:` dentro do `<description>`; note que ele vem em
   **base32** (`3YNQLPELLLV33KZHDZVCADN3OUKEP3GZ`), não hex — normalizar antes de comparar com
   o do Nyaa.
3. **AnimeTosho** — `GET feed.animetosho.xyz/json?q=<query>`. **Controle de método:** tem que
   devolver os mesmos infohashes do passo 1. Se divergir, o comparador está quebrado e os
   passos 2 e 4 não têm significado. Aborta a rodada.
4. **Medição de swarm** — para cada infohash **exclusivo do TokyoTosho** (ausente do conjunto do
   Nyaa): adiciona no `rain`, aguarda **90 segundos**, lê `peers_total` de `TorrentInfo`. É a
   única saída, já que o feed não informa seeders.

   A janela é fixa e igual para todos os candidatos, incluindo os do controle Cowboy Bebop — é o
   que torna os números comparáveis entre si. 90s cobre bootstrap do DHT mais um ciclo de
   announce ao tracker com folga; um swarm que não produz um único peer nesse tempo é, para o
   propósito do daemon, morto. `peers_total == 0` num candidato do controle invalida a rodada
   (ver "Controles obrigatórios").

## Saída

Uma tabela, no molde do `.debug_batch/report.md`:

| id | anime | nyaa_cands | nyaa_melhor_seeders | tt_cands | tt_infohash_exclusivos | tt_melhor_peers_medido |
|---|---|---|---|---|---|---|

Mais uma seção de veredito por anime: `CONFIRMA` (Nyaa morto e TokyoTosho vivo), `EMPATE`
(ambos mortos), `IRRELEVANTE` (Nyaa já estava vivo).

## Critério de decisão

Fixado **antes** de medir, para que o resultado não seja interpretado depois:

> Se em **≥ 1/3** da população existir infohash exclusivo do TokyoTosho com `peers > 0` onde o
> melhor do Nyaa tinha `seeders == 0` → **hipótese confirmada**. Abre-se a spec de arquitetura
> do adapter.
>
> Abaixo de 1/3 → **hipótese refutada**. O projeto de "múltiplas fontes" é encerrado, e a
> continuação é Debrid ou Usenet, em spec própria.

## Forma

Script one-shot em `scripts/`, no molde de `scripts/debug-batch.sh`. Roda a mão, escreve
relatório, não entra no daemon.

**Explicitamente NÃO faz parte desta etapa:** interface `Source`, registry de fontes, campo novo
em `nyaaSearcher`, campo de config, extração de `classify` do `parseRow`, adapter de qualquer
espécie. Se a hipótese cair, tudo isso seria descartado.

---

## Fora do escopo

### Série longa (One Piece, Naruto)

Falha por causa **diferente**, e nenhuma fonte a resolve. Verificado em 2026-08-16:

- `rain` v2.3.1 (`go.mod:24`) expõe só `Files()` e `FileStats()` em `session_torrent.go:52-57`,
  ambos **read-only**. Não há prioridade nem skip por arquivo — o cliente não sabe baixar parte
  de um pack.
- `max_batch_torrent_size_gb` default **100** (`files/filemanager.go:190`).
- Medido no Nyaa para One Piece (`docs/agents/sources.md`): pack completo 587,5 GiB; `[Judas]
  001-574` **171,8 GiB**; `[Judas] 575-782` 61 GiB (só na página 3).

Para a janela pendente 1-12, o **único** pack vivo que cobre custa 171,8 GiB e é barrado pelo
teto de 100. Os episódios soltos 1-12 existem e estão mortos. Não há pack pequeno vivo para o
começo de One Piece — e não haverá em fonte nenhuma, porque 574 episódios pesam o que pesam.

O que falta não é *onde procurar*, é **baixar parte de um torrent**. Saídas possíveis, todas em
spec própria: teto de tamanho por anime via `AnimeSettings` (menor diff, custa disco), seleção
por arquivo (fork/troca do cliente — `anacrolix/torrent` tem prioridade por arquivo), ou Debrid.

### Qualidade da escolha e resiliência do Nyaa

Não são o motivo desta spec. Se caírem de brinde num adapter futuro, ótimo; não guiam decisão
aqui.

---

## Atualizações a fazer em `docs/agents/sources.md`

Ao executar esta etapa, corrigir o doc com o que foi medido:

1. AnimeTosho: acrescentar a medida 75/75 `nyaa_id` e explicitar que ele **não** resolve swarm
   morto — hoje o doc o recomenda como "melhor fonte nova candidata" sem essa ressalva.
2. AniDex: registrar o 403 do DDoS-Guard.
3. TokyoTosho: corrigir o veredito "mesmos uploaders, mesmo conteúdo". Ele agrega acg.rip,
   bangumi.moe e nyaatracker, e **não** expõe seeders no RSS.
4. Registrar que os vereditos anteriores foram formados sobre anime moderno, onde as fontes são
   equivalentes por construção.
