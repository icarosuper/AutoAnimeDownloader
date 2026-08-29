# Plano de correção — checklist ordenado

Arquivo de trabalho, **temporário**: quando todos os itens estiverem marcados, esta pasta inteira
some. O que precisa sobreviver a ela já foi migrado (ver "O que já está documentado fora daqui",
no fim).

Os quatro documentos de diagnóstico desta pasta descrevem **três** problemas com uma raiz comum e
**um** problema independente:

| Documento | Assunto |
|---|---|
| [cour-split-apaga-e-rebaixa.md](cour-split-apaga-e-rebaixa.md) | pack de season é apagado e rebaixado quando o cour muda |
| [part-2-nunca-acha-batch.md](part-2-nunca-acha-batch.md) | anime com "Part 2" no título nunca acha pack |
| [raiz-comum-eixo-absoluto.md](raiz-comum-eixo-absoluto.md) | a raiz dos dois e o plano do eixo absoluto por série |
| [rate-limit-anilist.md](rate-limit-anilist.md) | orçamento da AniList — problema pré-existente e independente |

**Duas regras de ordenação que não são preferência:**

1. **F1 vem antes de F2/F3.** Consertar a busca de packs sem consertar a poda faz o bug de apagar
   rodar *mais* vezes: o mesmo pack de season passa a ser escolhido para o media id novo,
   `Session.Add` reusa o infohash e a Fase 3 apaga com o snapshot pré-passe.
2. **F6 vem antes de F7 e F8.** Os dois consomem o mesmo dado; F6 sozinho não muda comportamento
   nenhum e é verificável por teste.

**Decisões de projeto já fechadas** (29/ago/2026) — o porquê de cada uma está no bloco da feature:

| Decisão | Escolha | Onde |
|---|---|---|
| De onde sai o offset de anime fora do passe | **derivar na hora**, semeando o BFS com os `anime_id` de `episodes.json`; `EpisodeStruct` não ganha campo | F7 (e o conjunto-semente no F6) |
| Como a prioridade chega ao gate de orçamento | **parâmetro em todos os call sites**, enum de dois valores — sem default implícito | F5 |
| F3 sai junto do F2? | **não**: condicional ao `debug-batch` depois do F2 | F3 |

---

## Protocolo de cada item — vale para TODOS os itens deste arquivo

Cada feature (F1, F2, …) é **uma unidade de trabalho fechada**. Uma feature por vez, do começo ao
fim, sem emendar a próxima.

Ao terminar uma feature, antes de qualquer outra coisa:

1. **`go test ./...`** — tem que passar inteiro, não só o pacote tocado.
2. **Rodar a verificação específica do item**, quando ela existir (ex.: `make debug-batch` no F2).
3. **Atualizar as docs afetadas**, na mesma leva: `architecture.md` se mudou símbolo, endpoint ou
   arquivo; `decisions.md` se o padrão é não-óbvio ou se a entrada existente descreve o
   comportamento antigo; `config.md`/`environment.md` se mudou campo ou env var. Os itens de doc
   já estão listados dentro de cada feature.
4. **`swag init -g src/cmd/daemon/main.go -o docs/swagger`** se a API mudou.
5. **Marcar o item aqui** (`- [ ]` → `- [x]`), com uma linha do que ficou diferente do planejado,
   se ficou.
6. **Sugerir o título do commit**, seguindo a convenção do repositório (conferida no `git log`):
   `tipo: descrição em PT-BR, no infinitivo, minúscula, sem ponto final`. Tipos em uso: `feat`,
   `fix`, `refactor`, `test`, `docs`, `chore` (e `chore!` para breaking change). Sugerir **um**
   título, não uma lista de opções — se o diff não couber em um título, é sinal de que virou mais
   de uma feature e o problema é o diff, não o título.
   Exemplos no formato certo, para as features deste plano: `fix: preservar pack cujo conteúdo
   excede os episódios registrados` (F1), `fix: não descartar título alternativo no match do Nyaa`
   (F2), `feat: buscar avulsos da AniList em lote` (F4).

Depois disso: **PARAR e devolver para revisão humana.** Reportar o que foi feito, o resultado dos
testes (a saída, não "passou"), o que ficou de fora, e o título de commit sugerido — e esperar.
**Não commitar, não fazer push, não começar a próxima feature.** O commit é manual, feito pelo
usuário depois de revisar o diff.

Isso não é cerimônia: várias destas features mexem em código que **apaga arquivo do disco** (F1) ou
muda o score de **todo** match do projeto (F2). Um diff por vez é o que torna cada um revisável, e
é o que permite reverter um sem desfazer os outros.

---

## Onda 0 — parar a perda de dados

- [x] **F1 · Guard de exclusão de pack**
  *Onde:* `src/internal/daemon/episodes.go` — `allEpisodesInDeleteSet`, `removeEpisodesAndLinks`,
  `handleSavedEpisodes`.

  > **Premissa do diagnóstico que estava errada (verificada no código em 28/ago/2026):** o
  > `savedEpisodes` que chega em `removeEpisodesAndLinks` **já é a `downloaded_episodes` inteira**
  > — vem de `fileManager.LoadSavedEpisodes()` em `verification.go`, e `RemoveEpisodesWithLinks`
  > recarrega o arquivo por conta própria. O `byHash` **não** é escopado por anime. Logo, "fazer o
  > guard varrer toda a `downloaded_episodes`" é no-op: já é assim. Os dois defeitos reais são
  > outros, abaixo.

  - [x] **Defeito A — o snapshot é pré-passe.** `handleSavedEpisodes` chama
        `saveEpisodesToFile(data.newEpisodes)` e logo depois passa `data.savedEpisodes` (lido antes
        do passe) para `removeEpisodesAndLinks`. Os registros recém-criados para outro media id
        apontando para o mesmo hash são invisíveis para o guard. Corrigir incluindo
        `data.newEpisodes` no `byHash`.
        *Isso conserta só o agravante do mesmo passe.*
  - [x] **Defeito B — o guard confunde "todos os registros" com "todo o conteúdo".**
        `allEpisodesInDeleteSet` conclui "nada sobrevive" quando todo registro **existente** do hash
        está no delete set. É falso quando o pack cobre episódios que **não têm registro nenhum** —
        exatamente o cour 2, que ainda não foi baixado. Comparar a cobertura do delete set com a
        faixa declarada no próprio registro (`BatchStart`/`BatchEnd`, que já existem em
        `EpisodeStruct`): pack que declara 1..23 com só 11 registros tem 12 episódios de conteúdo
        sem dono → não remover os dados.
        *Este é o que efetivamente para a perda de dados no cenário base.*
        **Teto conhecido:** pack sem faixa no nome grava `BatchStart == 0` (desconhecida) e continua
        indetectável — fica como está hoje. Marcar com comentário `ponytail:`, e é mais um uso para
        a lista de arquivos da página de detalhe (ver `TODO.md`).
  - [x] Teste (defeito B): pack `BatchStart=1, BatchEnd=23` com 11 registros, todos os 11 no delete
        set → torrent e dados sobrevivem
  - [x] Teste (defeito A): mesmo passe salva `{Y,1..12}` no mesmo hash e poda `{X,1..11}` → torrent
        sobrevive
  - [x] Teste de não-regressão: pack `BatchStart=1, BatchEnd=12` com os 12 registros no delete set
        → torrent **é** removido (o comportamento atual, que está certo)

  *Feito em 29/ago/2026.* Diferenças do planejado: `allEpisodesInDeleteSet` foi renomeada para
  `canRemoveTorrent` (o nome antigo passou a mentir — ela agora devolve `false` com todos os
  registros no delete set); a comparação de cobertura usa a **contagem** de registros contra o span
  declarado (`declaredSpan`), não os números de episódio, porque um mesmo hash pode ter registros de
  media ids em numerações locais diferentes. Registrado em `decisions.md` #74.

  *Por que primeiro:* é o único item que apaga arquivo do disco, e os itens de busca aumentam a
  frequência dele.
  *Contorno enquanto não sai:* marcar os episódios do cour 1 como `ManuallyManaged` (blinda contra
  a poda em `identifyEpisodesNotInWatching` e `deleteEpisodesByStatus`), ou
  `delete_watched_episodes: false`. Nenhum dos dois impede o rebaixamento — isso é o F7.

---

## Onda 1 — independentes entre si, qualquer ordem (exceto F3, que só se decide depois do F2)

- [x] **F2 · Match de título não pode comer o título alternativo**
  *Onde:* `src/internal/nyaa/nyaa_match.go` — `truncateAtFirstMarker`.
  Não descartar o texto depois do marcador de season quando ele contém um título alternativo entre
  parênteses/pipes — ou casar contra **todas** as variantes de título, não só a que originou a
  busca.
  - [x] Teste com o nome real que falhou:
        `[EMBER] … (2024) (Season 2 | Part 2) … (Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2) (Batch)`

  > **Não é "um bug de string isolado", apesar de parecer.** `truncateAtFirstMarker` alimenta
  > `extractTitleTokens`, que produz os tokens dos **dois** lados do Jaccard, para **toda** busca —
  > episódio e pack. Mexer nele mexe no score de todo match do projeto. O comentário atual da função
  > ("everything after the marker is … never part of the anime's core title") é exatamente a
  > premissa que o nome do EMBER quebra, então ele também precisa ser corrigido.
  - [x] Rodar `make debug-batch` (corpus de `scripts/robustness-animes.txt`) antes e depois e
        comparar — o teste unitário sozinho não cobre a regressão de Jaccard na população

  *Feito em 29/ago/2026.* Diferenças do planejado: o corte no marcador **não** foi alterado — ele
  virou a *primeira* tentativa de `titleMatchesQuery`, e só quando ela falha o match tenta os
  títulos alternativos do rodapé, **um trecho por vez**. Duas medições decidiram esse desenho: (a)
  juntar prefixo + rodapé num conjunto único de tokens dá 0.78 de Jaccard contra 1.00 do romaji
  sozinho, e no `debug-batch` fazia os avulsos do Yameii de *SAO Alternative: GGO* casarem a query
  de *Sword Art Online II*; (b) manter a primeira tentativa intacta é o que impede o pack do EMBER
  de *Kimetsu no Yaiba* (query de 3 tokens) de regredir. Registrado em `decisions.md` #75.

  *Resultado no corpus:* nenhum match perdido; `108465` passou a casar o pack BDRip do EMBER de
  season 1 e trocou o melhor torrent de 65 para 492 seeders. No `166873` (o caso que motivou tudo),
  os três packs de Part 2 do EMBER passaram a sobreviver ao filtro — eram zero.

  *Melhor retorno isolado do lote, e imune ao refactor da Onda 2.*

- [x] **F3 · Variantes de título no caminho de pack** — **DESCARTADO por medição (29/ago/2026)**
  *Onde:* `src/internal/daemon/search.go` — `searchNyaaWithVariants`.
  Hoje só continua para a variante seguinte se a atual não devolveu **nada**. Precisa continuar
  quando devolveu episódios mas nenhum pack — foi o que impediu a variante em inglês de rodar, e
  era ela que casaria com os packs do EMBER.

  > **Decisão fechada (29/ago/2026): condicional ao resultado do F2.** Só escrever se, depois do F2,
  > o `make debug-batch` ainda mostrar anime perdendo pack por falta de variante.
  >
  > **Por quê:** F2 e F3 atacam o mesmo sintoma por lados diferentes, e o custo do F3 não é o diff —
  > é o regime. Ele acrescenta mais uma rodada de busca ao Nyaa por anime em **todo passe** em que
  > não houver pack, que é a maioria dos animes em exibição (pack de temporada corrente só aparece
  > no fim). Combinado com a paginação adaptativa (`decisions.md` #57), isso multiplica. Se o F2
  > sozinho zerar a categoria no corpus, o melhor resultado possível para o F3 é nunca ser escrito.
  >
  > A falha que ele descreve continua real e continua registrada aqui — o que foi decidido é que ela
  > não é agendada até haver evidência de que ainda importa depois do F2.

  - [x] Rodar `make debug-batch` depois do F2 e decidir por evidência
  - [x] ~~(se entrar) Teste: variante 1 devolve só avulsos → variante 2 é tentada~~ — não entrou

  **Veredito: não implementar.** A medição rodou `ScrapNyaaForAnime` + `partitionSearchResults` +
  `pickBatches` para **cada** variante dos 8 animes do corpus mais `146065` e `166873`, com o código
  pós-F2. Não há um único anime em que a variante 1 devolva resultado e uma variante posterior ache
  pack que ela não achou — onde o Nyaa tem pack, o romaji já o encontra. O `235` (Meitantei Conan) é
  o único que cai no caminho de avulso, e as quatro variantes dão zero pack: é ausência de conteúdo,
  não de variante. Nenhuma linha de produção mudou; a tabela da medição e a condição de reabertura
  estão em `decisions.md` #76, e o parágrafo do `troubleshooting-downloads.md` que apontava a
  variante como suspeito foi anotado com o resultado.

  *A rodada de `make debug-batch` reaproveitada é a de 15:01 de 29/ago/2026 (`.debug_batch/report.md`),
  já pós-F2 — é ela que registra os 492 seeders do `108465` citados no F2. A sonda por variante é
  medição mais estrita que o relatório, que não separa pack de avulso.*

- [x] **F4 · Avulsos em lote (`id_in`)**
  *Onde:* `src/internal/api/standalone.go` — `appendStandaloneEntries`; **e**
  `src/internal/daemon/standalone.go` (mesmo loop).
  Trocar o loop de `GetMediaByID` por `Page(media(id_in: [...]))`. Leva N req/min → 1.
  - [x] Os **dois** lados (corrigir só um deixa metade do consumo de pé)
  - [x] Continuar gravando o cache **por id**, não por lote — senão um avulso novo invalida tudo;
        buscar só os ids que erraram o cache
  - [x] `perPage` máximo é 50 → mais de 50 avulsos viram 2 queries
  - [x] Id pedido que não voltou = ausente: `id_in` **omite silenciosamente** media apagado da
        AniList. Precisa do mesmo tratamento que o `ErrNotFound` do `GetMediaByID` dá hoje, senão
        um avulso morto some da tela sem aviso

  *Feito em 29/ago/2026.* Diferenças do planejado: `GetMediaByID` **não** foi removida — ela
  continua servindo os lookups de um id só (`resolveMediaList`, `DownloadStandaloneAnime`,
  `handleStandaloneAnimeAdd`, `resolveAnimeDetails`), e o bloco de campos das duas queries virou a
  const `mediaByIDFields` para os dois caminhos não poderem divergir. O mapa devolvido por
  `GetMediaByIDs` tem **três** desfechos por id, não dois: valor, `nil` explícito (a AniList não
  conhece o id) e **chave ausente** (não deu para buscar — só esse caso vem com erro). Sem essa
  distinção o aviso de "avulso morto" sairia também num 429, que é ruído. O erro não invalida o
  mapa: as páginas que passaram antes dele continuam lá. Registrado em `decisions.md` #65 e #72.

  *Independente de todo o resto; é um 429 esperando acontecer hoje. Medições em `decisions.md` #65
  e #72.*

- [x] **F5 · Gate de orçamento por prioridade**
  *Onde:* `src/internal/anilist/anilist.go` — `sendAnilistRequest`; estado ao lado de `health.go`.
  - [x] Gravar `X-RateLimit-Remaining` + instante da leitura em **toda** resposta, inclusive erro
  - [x] Piso por criticidade: passe do daemon sempre passa; poll do frontend serve cache velho;
        busca de avulso pela UI recusa com erro visível; warm-up adia
  - [x] **Validade de 60s na leitura** — sem isso o gate se auto-trava: se todos recusarem, ninguém
        emite request e o processo nunca descobre que o balde resetou
  - [x] Não bloquear goroutine esperando (o poll roda dentro de handler HTTP)

  > **Decisão fechada (29/ago/2026): parâmetro de prioridade em TODOS os call sites**, com um enum
  > de dois valores (crítico / descartável). São 9 diffs de produção + 7 em `health_test.go`, todos
  > mecânicos.
  >
  > **Por quê:** criticidade não é derivável da query — `GetAllCurrentAnime` e `GetFrontendAnimeList`
  > podem ser a mesma query com variáveis diferentes e criticidades opostas —, então alguém tem que
  > declarar. A alternativa descartada era um wrapper `sendAnilistRequestP(prio, …)` com o
  > `sendAnilistRequest` atual virando "prioridade normal": diff menor, mas cria um default
  > implícito, e quem adicionar uma query nova daqui a seis meses vai herdá-lo sem decidir nada.
  > Esse é exatamente o formato do bug do `codec` fora de `episodeCriteria` (`decisions.md` #69):
  > config inerte que ninguém enxerga por anos. 16 linhas mecânicas uma vez saem mais barato que a
  > classe de bug.
  >
  > **Consequência para quem implementa:** toda query nova é obrigada a declarar sua criticidade
  > para compilar. Isso é a feature, não o custo.

  - [x] Atualizar `decisions.md` #72 com o desenho como implementado (hoje ele registra a medição e
        o desenho proposto, não o código)

  *Feito em 29/ago/2026.* Diferenças do planejado: (a) a `Priority` não parou em
  `sendAnilistRequest` — subiu para as **quatro** funções públicas de chamador misto
  (`GetCustomListsMap`, `GetAnimeInfo`, `GetMediaByID`, `GetMediaByIDs`), que daemon e frontend
  chamam com criticidades opostas; fixar a criticidade dentro delas mataria metade da feature (o
  poll de `/animes` nunca degradaria) ou quebraria o passe. (b) A linha divisória que emergiu não é
  "daemon vs. frontend", é **"se repete sozinho vs. alguém está esperando"**: `SearchMedia` (uma
  query por tecla) é descartável, mas adicionar avulso, abrir a tela de detalhe e baixar episódio à
  mão são críticos — recusá-los não devolve orçamento nenhum e quebra um clique. (c) O "serve cache
  velho" exigiu um `ttlCache.getStale`, porque o `get` existente não distingue vencido de ausente.
  Nenhum chamador de warm-up existe ainda; ele entra com o F6. Registrado em `decisions.md` #72.

  *Vale por si só, e torna o F6 seguro por construção. Desenho e medições em `decisions.md` #72.*

---

## Onda 2 — o eixo absoluto por série

- [x] **F6 · Base: `seriesKey` + `absEp`** (só o dado, nenhum consumidor muda)
  *Onde:* `src/internal/anilist` — query nova + BFS + `ttlCache`.
  - [x] **Conjunto-semente = animes do passe ∪ `anime_id` distintos em `episodes.json`** (decisão do
        F7, abaixo). Quem monta a semente é o **daemon**, que passa a lista de ids pronta: `anilist`
        não pode importar `files`, e é essa restrição que define a assinatura pública do F6
  - [x] BFS com `Page(media(id_in: [...]))` e `relations` aninhado em 2 níveis; loop **agnóstico à
        profundidade** (consome o que vier, enfileira o que faltar)
  - [x] Marcar quais nodes vieram em posição autoritativa e **re-buscar os do nível cortado** —
        `edges: []` no nível em que a AniList para de expandir é indistinguível de "sem prequel", e
        confundir os dois produz offset silenciosamente errado
  - [x] Filtrar `format ∈ {TV, TV_SHORT}` ao seguir `PREQUEL` (já feito em `ComputeEpisodeOffset`,
        ver `decisions.md` #9)
  - [x] `episodes == nil` (anime não lançado) → trata como 0 e **não** persiste no cache
  - [x] Cache em memória, TTL 24h, só grava `FINISHED && episodes != nil` — o dado é imutável e a
        caminhada é monotônica (só ancestrais entram). Nada de arquivo em disco: o warm-up inteiro
        custa ~4 requests
  - [x] Conjunto `known` protege contra ciclo
  - [x] **Teste com as cadeias medidas como golden fixture** (é a validação real; em markdown
        apodrece, em teste quebra o CI):

        ```
         108465 eps= 11 abs  1..11   Mushoku Tensei: Isekai Ittara Honki Dasu
         127720 eps= 12 abs 12..23   Mushoku Tensei: Isekai Ittara Honki Dasu Part 2
         146065 eps= 13 abs 24..36   Mushoku Tensei II: Isekai Ittara Honki Dasu
         166873 eps= 12 abs 37..48   Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2

          16498 eps= 25 abs  1..25   Shingeki no Kyojin
          20958 eps= 12 abs 26..37   Shingeki no Kyojin Season 2
          99147 eps= 12 abs 38..49   Shingeki no Kyojin Season 3
         104578 eps= 10 abs 50..59   Shingeki no Kyojin Season 3 Part 2
         110277 eps= 16 abs 60..75   Shingeki no Kyojin: The Final Season
         131681 eps= 12 abs 76..87   Shingeki no Kyojin: The Final Season Part 2

          15689 eps=  4 abs  1..4    Nekomonogatari (Kuro)
           5081 eps= 15 abs  5..19   Bakemonogatari
          11597 eps= 11 abs 20..30   Nisemonogatari
          17074 eps= 26 abs 31..56   Monogatari Series: Second Season
          21262 eps= 12 abs 57..68   Owarimonogatari
          20918 eps=  4 abs 69..72   Tsukimonogatari
          21745 eps=  7 abs 73..79   Owarimonogatari (Ge)
        ```

        O Shingeki é a validação forte: `76..87` é exatamente a numeração que SubsPlease e Erai
        usaram nos arquivos do Final Season Part 2.
  - [x] Atualizar `architecture.md` com os símbolos novos
  *Feito em 29/ago/2026.* Diferenças do planejado: **nenhum chamador de warm-up entrou** — F6 é
  regra de ordenação ("F6 sozinho não muda comportamento nenhum"), e um warm-up no passe mudaria.
  Quem semeia entra com o F7; a linha do `decisions.md` #72 que dizia "o warm-up entra com o F6"
  foi corrigida. O tipo devolvido é `Series{Key, Offset}` e **não** tem método `Abs(ep)`: sem
  consumidor, `Offset + ep` já diz tudo. A semente do F7 (`anime_id` de `episodes.json`) não entra
  aqui porque nada monta a semente ainda — a assinatura `GetSeriesIndex(ids, priority)` já a
  aceita, que era o ponto do item. Registrado em `decisions.md` #77.

  *Duplicação deixada de propósito:* `anilist.prequelOf` repete a seleção de "primeiro `PREQUEL` de
  `TV`/`TV_SHORT`" que `daemon.ComputeEpisodeOffset` já faz. Não unificada aqui porque o F8
  rebaixa a segunda a uma das hipóteses de numeração de pack, e as duas podem divergir no caminho.
  Marcada com `TODO(F8)` nas duas funções e listada como item do F8.

  *A fixture pegou um bug do próprio teste:* a primeira versão semeava a cadeia inteira, então todo
  ancestral chegava como nível 0 da resposta e o corte de 2 níveis nunca era exercitado — um
  mutante que tratava o nível cortado como raiz passava. O teste agora semeia só as pontas. O mock
  renderiza exatamente 2 níveis e devolve `edges: []` no terceiro, como a AniList real.

  *Custo medido: 28 medias em 4 requests (`ceil(altura/2) + 1`), e quase não cresce com o tamanho
  da lista. Fatos da API em `decisions.md` #71 e #72.*

- [x] **F7 · Posse por cobertura** (consome F6)
  Antes de buscar no Nyaa, procurar torrent já baixado cuja cobertura (`BatchStart..BatchEnd` no
  eixo absoluto) contenha o `absEp` pedido — em **qualquer** `anime_id`.

  > **Decisão fechada (29/ago/2026): derivar o offset na hora — `EpisodeStruct` NÃO ganha campo.**
  >
  > **O problema.** `BatchStart`/`BatchEnd` estão na numeração **local** da entrada sob a qual o pack
  > foi baixado; o episódio pedido está na numeração local de outra entrada. Comparar `1..23` com
  > `1..12` sem converter é comparar réguas diferentes. Converter exige o offset do `AnimeID`
  > **daquele registro** — que é justamente o cour anterior, já `COMPLETED`, que não vem em
  > `GetAllCurrentAnime`. É literalmente o anime que sumiu do universo do passe e causou o bug do
  > doc 1.
  >
  > **A escolha:** semear o BFS do F6 também com os `anime_id` presentes em `episodes.json`, não só
  > com os do passe.
  >
  > - Custo quase zero: largura é grátis (`perPage` 50, e uma query custa 1 unidade independente do
  >   número de ids — `decisions.md` #72), e boa parte dessa semente extra já entraria na caminhada
  >   de qualquer jeito, por serem os ancestrais dos animes do passe. Biblioteca de 200 animes = 4
  >   queries, absorvidas pelo cache de 24h.
  > - Nenhum campo novo, nenhuma migração, e o registro antigo — inclusive o pack que **já está em
  >   disco** e originou toda a investigação — funciona no dia 1.
  >
  > **Alternativa descartada: persistir `AbsStart`/`AbsEnd` no registro na hora do download.** O
  > lookup ficaria local e offline, mas todo registro anterior à feature nasceria com `0`, que é
  > indistinguível de "desconhecido" — ou seja, a feature não resolveria o caso que a motivou, só
  > valeria para o que fosse baixado depois. E o backfill precisaria da derivação de qualquer forma.
  > Pior: valor persistido apodrece — se a AniList corrigir uma contagem de episódios ou adicionar
  > uma relação que faltava, o número gravado fica errado para sempre; o derivado se autocorrige no
  > próximo TTL. Guardar como fonte da verdade um número que dá para derivar cria uma segunda
  > verdade que ninguém invalida.
  >
  > **Reabrir só se** o lookup do F7 aparecer como custo **medido** — e aí ele vira cache derivado,
  > nunca fonte.
  - [x] Teste: pack da season baixado sob o cour 1 → cour 2 não busca no Nyaa
  - [x] Atualizar `decisions.md`: por que a unidade de posse deixou de ser `(anime_id, episódio)`

  *Feito em 29/ago/2026.* Diferenças do planejado: (a) adotar não é só "não buscar" — o episódio
  ganha **registro apontando para o hash que já existe**, que é o que a `Session.Add` produziria
  se a busca reencontrasse o mesmo torrent. Sem isso o F1 continuaria sem enxergar os irmãos do
  outro cour e os arquivos sem dono nunca seriam hardlinkados; com isso, `organizeTorrent` já
  trata o caso (grupo parcialmente organizado, sem re-notificar) e nenhuma exceção nova precisou
  existir. (b) ~~A faixa é **copiada como o dono a declara**, nunca convertida para a numeração
  local do adotante — é o que `declaredSpan` (#74) espera.~~ **Errado, corrigido depois:** a faixa
  é **convertida** para a numeração local de quem adota. Ela é lida de volta somando o offset do
  `anime_id` do próprio registro, então copiá-la deslocava a faixa e fazia um terceiro cour achar
  cobertura para episódio que o pack não tem. O `declaredSpan` do #74 também mudou junto (mede por
  `anime_id`, não na união). Ver `decisions.md` #74 e #78. (c) Duas portas que o plano não citava
  e que são obrigatórias: o torrent tem que estar **vivo na sessão** (adotar hash morto deixa
  episódio com dono e sem arquivo) e o anime pedido **não pode ser filme** (`prequelOf` só segue
  TV, então um filme pós-season herda o offset dela e o "episódio 1" dele cai dentro da faixa do
  pack). (d) O filtro roda nos **dois** pontos de seleção: a segunda passada de `selectEpisodes`
  (limite levantado, caminho de pack) traria os adotados de volta. (e) `DownloadStandaloneAnime`
  também semeia o índice — adicionar avulso é o momento mais provável de a season já estar em
  disco. Registrado em `decisions.md` #78.

  *Mata o "cour 2 rebaixa o que já está em disco".*

- [x] **F8 · Busca por cobertura** (consome F6) — **o mais arriscado, por último**
  Filtro duro de part deixa de perguntar "esse pack é da part N?" e passa a perguntar "esse pack
  cobre a janela pendente?" — que é o que `pickBatches` já quer decidir.
  - [x] Heurística de mapear a numeração do pack para o eixo absoluto: poucas hipóteses (offset 0,
        offset do prequel imediato, offset da série inteira) + **contagem de arquivos como
        desempate** (um pack de 25 arquivos numerado a partir de 1 só pode ser season contínua).
        `ComputeEpisodeOffset` vira uma das hipóteses, não a única
  - [x] **Resolver a duplicação que o F6 deixou marcada.** A seleção de "primeiro `PREQUEL` de
        `TV`/`TV_SHORT`" existe hoje em **dois** lugares: `anilist.prequelOf` (`series.go`, caminha
        a cadeia inteira) e `daemon.ComputeEpisodeOffset` (`helpers.go`, um salto só, gated por
        `part >= 2`). Os dois carregam um `TODO(F8)` apontando um para o outro. Não foram
        unificados no F6 de propósito: é este item que decide o destino delas — se
        `ComputeEpisodeOffset` vira "hipótese offset do prequel imediato", as duas podem divergir
        legitimamente; se a heurística não precisar dela, ela sai. **Se o F8 for descartado como o
        F3 foi, unificar as duas e apagar os dois TODOs** — sem o F8 não sobra motivo para a
        duplicação existir
  - [x] Na dúvida, resolver para **0**: offset ausente cai na numeração relativa, que boa parte dos
        grupos usa; offset errado manda a busca para episódio inexistente
  - [x] Absorve o `Part 1 + Part 2` do `extractPart` — cobertura por range não lê marcador de part
  - [x] Testes com os quatro formatos de numeração de `sources.md`
        ("Granularidade e numeração dos packs")
  *Dependência opcional que fortalece muito:* o item do `TODO.md` de scrappar a página de detalhe
  do Nyaa dá a lista de arquivos, que é exatamente o desempate desta heurística. Se os dois forem
  feitos, este depois daquele.

  *Feito em 29/ago/2026.* Diferenças do planejado: (a) o filtro de part **não** saiu do caminho de
  pack — ele foi rebaixado de "exige marcador" para "rejeita conflito declarado". Cobertura por
  range sozinha não distingue o pack da Part 1 do pack da Part 2 quando os dois reiniciam em 1, que
  é o caso comum; o marcador continua sendo o único sinal ali. O que estava errado era rejeitar
  pack **sem** marcador, que é o formato normal de lançamento. (b) O desempate por contagem de
  arquivos virou o **span do próprio nome** (`fim - início + 1`), que já está disponível e diz a
  mesma coisa para pack com faixa no nome; a lista de arquivos da página de detalhe continua sendo
  o desempate mais forte e continua no `TODO.md`. (c) A regra do desempate é "a hipótese cujo fim
  cai no último episódio da entrada", não uma comparação de contagem solta. (d) A faixa **gravada**
  passou a ser a convertida para a numeração local, e ela pode começar em zero ou abaixo — o que
  obrigou a trocar o sentinela de "faixa desconhecida" de `BatchStart <= 0` para `BatchEnd <= 0`
  (`hasDeclaredRange`), em `declaredSpan` (#74) e `findCoveringPack` (#78). Sem isso o pack de
  season gravado sob um cour posterior seria lido como "sem faixa". (e) `extractPart` também
  absorveu a forma compacta `Part 1+2` / `Part 03+04`, que é a mesma ambiguidade — isso mudou duas
  expectativas que já existiam em `nyaa_test.go`. Registrado em `decisions.md` #79.

---

## Já feito (pendente de commit)

- [x] `ComputeEpisodeOffset` só aceita `PREQUEL` de `TV`/`TV_SHORT` — `MediaRelationNode` ganhou
      `Format`, as três queries que pedem `relations` passaram a trazê-lo, teste novo cobrindo
      `MOVIE`/`OVA`/`ONA` → offset 0, `decisions.md` #9 e `architecture.md` atualizados.
      *Está no working tree; é um commit próprio, independente de tudo acima.*

---

## O que já está documentado fora daqui

Migrado dos quatro documentos, para esta pasta poder ser apagada sem perda:

| Conteúdo | Onde foi parar |
|---|---|
| AniList não tem id de franquia; `relations`/`PREQUEL` é a única fonte; `search:` por nome é armadilha; nesting corta em 2 níveis; `edges: []` ≠ raiz | `docs/agents/decisions.md` #71 |
| Headers de rate limit: 1 query = 1 unidade, erro consome cota, balde reseta inteiro em ≤60s, `X-RateLimit-Reset` não vem em 200, gate por prioridade em vez de limiter | `docs/agents/decisions.md` #72 |
| Frontend buscar direto na AniList: descartado (NAT + duplicação em TS), com condição de reavaliação | `docs/agents/decisions.md` #73 |
| `appendStandaloneEntries` como consumidor dominante do orçamento | `docs/agents/decisions.md` #65 (linha nova na tabela) |
| Pack é de season, não de cour; as quatro convenções de numeração dos grupos; contagem de arquivos como desempate | `docs/agents/sources.md`, "Granularidade e numeração dos packs" |
| Como diagnosticar "nenhum pack sobrevive": linhas `Raw Nyaa row` + tabela `batch × part × match`, e a ausência da linha `Using batch torrents…` como âncora | `docs/agents/troubleshooting-downloads.md`, Step 6f |
| Itens que não viram feature agora (`Part 1 + Part 2`, frontend direto, página de detalhe) | `docs/TODO.md` |
| Cadeias de `PREQUEL` medidas | vira fixture de teste no F6, acima |

**O que morre com a pasta, de propósito:** o narrativo dos dois casos (Mushoku Tensei S1 e S2 Part 2
passo a passo), todos os `arquivo:linha` (apodrecem em semanas), as tabelas de contagem de requests
do warm-up, e as alternativas de implementação descartadas (DFS um-request-por-ancestral, K cadeias
por passe, `x/time/rate`) — dessas só o veredito de uma linha sobreviveu, nas decisions.
