# A raiz comum dos dois problemas, e o eixo absoluto por série

Escrito em 2026-08-28, depois de investigar [cour-split-apaga-e-rebaixa.md](cour-split-apaga-e-rebaixa.md)
e [part-2-nunca-acha-batch.md](part-2-nunca-acha-batch.md) juntos. Os dois são o mesmo problema
saindo por dois lugares diferentes.

Nada disso está implementado — este arquivo é o registro da análise e do plano. A única mudança
que já entrou no código está na última seção.

## A raiz

O daemon trata a unidade de posse como `files.EpisodeKey{AnimeID, Episode}`, onde `AnimeID` é uma
entrada da AniList. Mas:

- a AniList quebra cour/part em **media ids separados**, cada um com numeração própria a partir de 1;
- os packs do Nyaa são quase sempre da **season inteira**, atravessando essa fronteira.

Toda a sintomatologia dos dois documentos decorre disso:

- **Posse** (doc 1): um hash que cobre a season toda só é "conhecido" pelos registros do cour 1.
  Podar o cour 1 fecha o grupo em `allEpisodesInDeleteSet` e o torrent é apagado com os dados; o
  cour 2 não tem como saber que o arquivo já está em disco.
- **Busca** (doc 2): a query é montada a partir de um media id que é "Part 2", `requestedPart != nil`
  liga o filtro duro, e ele descarta justamente os packs que existem — que são de season inteira,
  porque é assim que packs são lançados.

Os dois documentos chegam à mesma frase por caminhos diferentes: *"pack sem marcador de part cobre
a janela pendente"* (doc 2) e *"procurar um torrent já baixado cujo `BatchStart..BatchEnd` cubra o
episódio, em qualquer `anime_id`"* (doc 1). É o mesmo conceito — **cobertura por range** em vez de
identidade por chave.

### O acoplamento perverso

Não são só parentes, se alimentam:

1. Corrigir **só o doc 2** (achar os packs de Part 2) faz cair **mais vezes** no agravante do doc 1:
   o mesmo pack de season escolhido para o media id novo, `Session.Add` reusando o infohash, e a
   Fase 3 apagando com o snapshot pré-passe. Consertar a busca sem consertar a poda torna o bug 1
   mais frequente.
2. Na outra direção, o fix "não rebaixar" do doc 1 resolveria o caso do doc 2 **sem tocar em match
   nem em filtro de part** — o pack já estaria em disco. Só que não está, porque o doc 1 apagou.

## Ideia descartada: id de série na AniList

Se a AniList desse um id da franquia, o problema morria na origem. Não dá. Introspecção do tipo
`Media` em 2026-08-28 — 55 campos, nenhum de franquia:

```
id, idMal, title, type, format, status, description, startDate, endDate, season,
seasonYear, seasonInt, episodes, duration, chapters, volumes, countryOfOrigin,
isLicensed, source, hashtag, trailer, updatedAt, coverImage, bannerImage, genres,
synonyms, averageScore, meanScore, popularity, isLocked, trending, favourites, tags,
relations, characters, staff, studios, isFavourite, isFavouriteBlocked, isAdult,
nextAiringEpisode, airingSchedule, trends, externalLinks, streamingEpisodes,
rankings, mediaListEntry, reviews, recommendations, stats, siteUrl,
autoCreateForumThread, isRecommendationBlocked, isReviewBlocked, modNotes
```

Não existe entidade "série" no schema. `idMal` tem a mesma granularidade (por cour). A única aresta
entre cours é `relations`. Portanto a caminhada de relações não é uma opção entre várias — é a única
fonte disponível.

## A caminhada de PREQUEL funciona

Caminhar `relations` seguindo `relationType == "PREQUEL"` e filtrando `format` para TV/TV_SHORT,
até a raiz. Verificado contra a API real:

```
 108465 eps=  11 abs   1..11  Mushoku Tensei: Isekai Ittara Honki Dasu
 127720 eps=  12 abs  12..23  Mushoku Tensei: Isekai Ittara Honki Dasu Part 2
 146065 eps=  13 abs  24..36  Mushoku Tensei II: Isekai Ittara Honki Dasu
 166873 eps=  12 abs  37..48  Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2

  16498 eps=  25 abs   1..25  Shingeki no Kyojin
  20958 eps=  12 abs  26..37  Shingeki no Kyojin Season 2
  99147 eps=  12 abs  38..49  Shingeki no Kyojin Season 3
 104578 eps=  10 abs  50..59  Shingeki no Kyojin Season 3 Part 2
 110277 eps=  16 abs  60..75  Shingeki no Kyojin: The Final Season
 131681 eps=  12 abs  76..87  Shingeki no Kyojin: The Final Season Part 2

  15689 eps=   4 abs   1..4   Nekomonogatari (Kuro)
   5081 eps=  15 abs   5..19  Bakemonogatari
  11597 eps=  11 abs  20..30  Nisemonogatari
  17074 eps=  26 abs  31..56  Monogatari Series: Second Season
  21262 eps=  12 abs  57..68  Owarimonogatari
  20918 eps=   4 abs  69..72  Tsukimonogatari
  21745 eps=   7 abs  73..79  Owarimonogatari (Ge)
```

**O Shingeki é a validação forte:** `76..87` é exatamente a numeração que SubsPlease e Erai usaram
nos arquivos do Final Season Part 2. A cadeia reproduz a convenção real de nomeação sem nenhum
ajuste. O Monogatari (7 níveis, ordem torta) também fecha coerente.

### As duas derivações

Da cadeia saem os dois valores que resolvem a raiz:

- **`seriesKey`** — id da raiz da cadeia (`108465` para Mushoku Tensei, `16498` para Shingeki).
  Identidade estável que atravessa cour.
- **`absEp`** — offset acumulado + episódio local.

| Sintoma | Como o par `(seriesKey, absEp)` resolve |
|---|---|
| Cour 2 rebaixa o que já está em disco (doc 1, §4) | antes do Nyaa, procurar torrent cuja cobertura contenha `absEp` — em qualquer `anime_id` |
| Filtro duro de part mata packs de season (doc 2, §2) | pergunta vira "esse pack cobre a janela `absEp` pendente?", que é o que `pickBatches` já quer decidir |
| `Part 1 + Part 2` vira `part=1` (doc 2, §2) | irrelevante: cobertura por range não lê marcador de part |

## Os limites — o que isto NÃO resolve

**1. A poda que apaga o torrent (doc 1, §2-3) não precisa disto.** O guard só precisa varrer
`downloaded_episodes` inteira por hash, em vez de olhar os registros de um anime só, mais incluir
`data.newEpisodes` no `byHash`. É diff menor e independente. Não amarrar um no outro.

**2. Mapear a numeração do torrent para `absEp` continua heurística.** É o buraco honesto: os grupos
não concordam sobre numeração.

| Release | Numeração | Convenção |
|---|---|---|
| Erai, MT S2 Part 1 | `00 ~ 12` | reinicia por season |
| EMBER, MT S2 Part 2 | `1..12` | reinicia por part |
| Diddy `S02` | 1..25 | contínua por season |
| SubsPlease/Erai, AoT FS P2 | `76..87` | absoluta por série |

O eixo absoluto é canônico **do lado do daemon**, mas o lado do torrent precisa de um palpite. A
parte boa: é escolha entre poucas hipóteses (offset 0, offset do prequel imediato, offset da série
inteira), e a **contagem de arquivos do pack desempata** — um pack com 25 arquivos numerado a partir
de 1 só pode ser season contínua. `ComputeEpisodeOffset` hoje é a hipótese "offset do prequel
imediato"; vira uma das três em vez da única.

**3. A causa #1 do doc 2 é imune a tudo isto.** `truncateAtFirstMarker` (`nyaa_match.go:63`) comendo
o romaji entre parênteses no nome do EMBER é bug de string puro. Continua lá, e continua sendo o de
melhor retorno isolado.

## Custo na AniList, e por que o cache atual não serve

| | valor |
|---|---|
| `mediaByIDTTL` (`standalone.go:16`) | 60 s |
| `CheckInterval` default (`filemanager.go:191`) | 10 min |
| Limite AniList (comentário em `anilist.go:19`) | 30 req/min por IP |
| Throttle proativo no pacote | não existe — só `health.go` reagindo depois do 429 |

O TTL é 60s e o passe é a cada 10 min: **o cache está sempre frio quando o passe começa**. Reusar
`GetMediaByID` ingenuamente re-caminharia todas as cadeias em toda verificação — 30 a 45 requests em
rajada para uma lista média, acima do limite. E um 429 aí derruba o ciclo inteiro de download, que é
o modo de falha documentado nas decisions.md #11 e #57.

### A solução: BFS com `id_in` + nesting de 2 níveis

Investigado a fundo em 2026-08-28. Três fatos da API definem o desenho:

**1. `Media.relations` não tem argumentos.** Introspecção confirma: sem filtro server-side por
`relationType`, sem paginação. Toda query de relations traz **todos** os edges — filtrar é sempre
trabalho do cliente.

**2. `relations` pode ser aninhado, mas a AniList corta em 2 níveis.** Testado com profundidades de
1 a 8: o corpo da resposta satura em 5865 bytes a partir da profundidade 3, e a inspeção mostra por
quê — no terceiro nível o campo `relations` vem presente mas com `edges: []`. Confirmado em três
franquias (Monogatari, Mushoku Tensei, Shingeki): **2 hops de PREQUEL por query, sempre**.

**3. Uma query custa uma unidade de rate limit, independente do tamanho.** Uma query com 12 ids e
nesting de 2 níveis devolveu 33 KB e os headers vieram `X-RateLimit-Limit: 30`,
`X-RateLimit-Remaining: 29`. Não há cobrança por complexidade. Isso é decisivo: **maximizar trabalho
por request é estritamente correto**, não há trade-off.

Combinando os três, o warm-up da mesma lista de 12 animes (Mushoku Tensei II P2, Shingeki FS P2,
Owarimonogatari Ge, Jujutsu Kaisen, Kimetsu, SPY×FAMILY, Shangri-La, Fruits Basket The Final,
Konosuba 3, SPY×FAMILY P2, Kaijuu 8-gou, Jujutsu Kaisen 2nd):

```
request #1: 12 ids pedidos -> 20 conhecidos, 4 ainda sem relations  (33800 bytes)
request #2:  4 ids pedidos -> 25 conhecidos, 2 ainda sem relations  (13457 bytes)
request #3:  2 ids pedidos -> 28 conhecidos, 1 ainda sem relations  ( 9266 bytes)
request #4:  1 ids pedidos -> 28 conhecidos, 0 ainda sem relations  ( 2110 bytes)

TOTAL: 28 medias em 4 requests, 58633 bytes
```

**4 requests** para os mesmos 28 medias — contra 7 sem nesting. O número de requests é
`ceil(altura / 2) + 1`, e a altura foi 7 (Monogatari). O último request só confirma que a raiz não
tem prequel.

O custo quase não cresce com o tamanho da lista: 50 animes dariam praticamente os mesmos 4 requests,
já que largura é grátis até `perPage: 50` e o rate limit não cobra por volume.

#### A dependência é vertical, não horizontal

É o que torna o BFS possível. Você só descobre o avô depois de buscar o pai — isso é inescapável.
Mas os pais de 12 animes diferentes são descobríveis no mesmo instante. O BFS batcheia a dimensão
horizontal; o nesting come 2 níveis da vertical de uma vez.

Sem nesting, o nível 1 da mesma lista cai de 12 para 4 ids:

```
Mushoku Tensei II Part 2   -> 146065 Mushoku Tensei II          NOVO
Shingeki FS Part 2         -> 110277 Shingeki FS                NOVO
Owarimonogatari (Ge)       ->  20918 Tsukimonogatari            NOVO
Fruits Basket: The Final   -> 111762 Fruits Basket: 2nd Season  NOVO
SPY×FAMILY Part 2          -> 140960 SPY×FAMILY        [JA NA LISTA]
Jujutsu Kaisen 2nd Season  -> 113415 Jujutsu Kaisen    [JA NA LISTA]
Jujutsu Kaisen, Kimetsu, SPY×FAMILY, Shangri-La, Konosuba 3, Kaijuu 8-gou
                           — sem prequel TV
```

Seis não têm prequel TV, dois apontam para algo já na lista (o conjunto `known` evita o request), e
só quatro geram ids novos.

#### Implementar agnóstico à profundidade

O teto de 2 níveis é comportamento observado, não documentado — pode mudar. O loop deve **consumir o
que vier e enfileirar o que faltar**, sem assumir profundidade nenhuma. Assim, se a AniList afrouxar
o teto o código fica mais rápido sozinho, e se apertar para 1 ele continua correto, só com mais
requests.

### Orçamento da AniList

O consumo, os headers de rate limit e o gate que impede o 429 estão em
[rate-limit-anilist.md](rate-limit-anilist.md) — é assunto próprio, e vale independente desta
feature. O que importa aqui: com nesting o warm-up custa ~4 requests, e o gate por prioridade
espalha o warm-up por vários passes sozinho se o orçamento estiver apertado.

### O cache certo

O dado é excepcionalmente cacheável, por duas propriedades:

- **Imutável.** Ancestral com `status: FINISHED` tem `episodes` que nunca mais muda. Regra de
  invalidação inteira: só guarda se `FINISHED && episodes != nil`.
- **Monotônico.** A caminhada é para **trás**. O anime processado vem fresco da lista a cada passe;
  só ancestrais entram no cache. Sequel nova é entrada nova na lista, cujos ancestrais já estão em
  cache. Nada é invalidado, nunca.

Como o warm-up inteiro custa ~4 requests, **um `ttlCache` em memória com TTL longo (24h) basta** —
reinício de daemon custa 4 requests, uma vez. Não precisa de arquivo em disco.

Isso apaga do desenho: arquivo novo em `~/.autoAnimeDownloader/`, serialização, mutex, migração, e
qualquer limitador de "K cadeias por passe". Sobra uma query nova, um loop BFS e um cache que já
existe no pacote.

### Alternativas consideradas e descartadas

| | warm-up de 12 animes | veredito |
|---|---|---|
| um request por ancestral (DFS) | 16 requests | descartada: ignora que a dependência é só vertical |
| K cadeias por passe | horas, incremental | descartada: durante o warm-up os dois bugs continuam acontecendo |
| BFS batched com `id_in` | 7 requests | superada pela linha abaixo |
| **BFS + `id_in` + nesting de 2 níveis** | **4 requests** | **escolhida** |
| busca por nome da franquia (`search:`) | 1 request | **descartada — ver abaixo** |
| rate limiter (`x/time/rate`) | não resolve, só enfileira | descartada — superada pelo gate por header, ver seção dos headers |
| incluir `COMPLETED` na query da lista | parcial | descartada: só pega ancestrais que estão na conta, e infla uma query quente |

A busca por nome seria um request só, e é uma armadilha. Medido:

- `search: "Monogatari"` → 28 resultados TV, quase todos de outras obras (`Ore Monogatari!!`,
  `Shouwa Monogatari`, `Perrine Monogatari`, `Gokinjo Monogatari`…).
- `search: "Shingeki no Kyojin"` → 7 resultados, incluindo `Shingeki! Kyojin Chuugakkou` (a paródia
  chibi, 12 episódios), que entraria na soma e corromperia todos os offsets.
- E não devolve **ordem**: a sequência teria que ser inferida dos títulos, que é exatamente o
  problema de parsing de string que quebrou o doc 2.

O grafo de `relations` é autoritativo; o nome não é.

Fica registrado que o problema de os três consumidores (poll do frontend, passe do daemon, avulsos)
disputarem os mesmos 30 req/min sem coordenação é **pré-existente e independente** desta feature. A
solução está na seção dos headers de rate limit, e vale por si só.

## Armadilhas da implementação

- **Filtrar `format`** para TV/TV_SHORT ao seguir `PREQUEL`. Filme de recap e OVA também chegam como
  `PREQUEL` com `Episodes` preenchido. *(já corrigido — ver última seção)*
- **`episodes` pode ser `nil`** em anime não lançado. Trata como 0 e não persiste no cache.
- **`edges: []` no último nível do nesting NÃO significa raiz.** Esta é a armadilha séria. No nível
  em que a AniList corta a expansão, `relations` vem presente e vazio — indistinguível de um anime
  que genuinamente não tem prequel. Tratar os dois como iguais faz o cálculo parar cedo e produzir
  um offset **silenciosamente errado**. É obrigatório marcar quais nodes vieram em posição
  autoritativa (nível 0 e 1 da query) e quais vieram no nível cortado, e re-buscar os do nível
  cortado antes de concluir qualquer coisa sobre eles.
- **Ciclos.** O conjunto `known` do BFS já protege: id visto não volta para a fronteira.
- **`perPage` máximo é 50.** Lista com mais de 50 ids num mesmo nível vira 2 requests naquele nível.
- **Errar para 0 é barato, errar para um número não.** Offset ausente cai na numeração relativa, que
  boa parte dos grupos usa; offset errado manda a busca para um episódio inexistente.

## Ordem de execução proposta

1. **Guard do doc 1** — varrer por hash em toda a `downloaded_episodes` + incluir `data.newEpisodes`
   no `byHash` de `handleSavedEpisodes`. Menor diff, e é o que para a perda de dados.
2. **`truncateAtFirstMarker`** (doc 2, causa #1) — bug de string, escopo fechado, melhor retorno
   isolado.
3. **`seriesKey`/`absEp` via BFS** — o refactor de verdade, e o único que justifica mexer no filtro
   de part.

## O que já foi feito

`ComputeEpisodeOffset` (`src/internal/daemon/helpers.go`) pegava o primeiro edge `PREQUEL` com
`Episodes != nil` **sem olhar `format`** — e `MediaRelationNode` nem tinha o campo. Um prequel que
fosse filme de recap ou OVA entrava como offset e deslocava a busca inteira. Foi o filtro que
precisou ser aplicado para a caminhada acima funcionar, então entrou agora:

- `MediaRelationNode` ganhou `Format MediaFormat`;
- as três queries que pedem `relations` (`GetAllCurrentAnime`, `getMediaListEntry`, `GetMediaByID`)
  passaram a trazer o campo;
- `ComputeEpisodeOffset` só aceita PREQUEL de `TV`/`TV_SHORT`;
- teste novo cobrindo `MOVIE`/`OVA`/`ONA` como PREQUEL → offset 0;
- `decisions.md` #9 e a linha do `architecture.md` atualizadas, já que descreviam o gate antigo.
