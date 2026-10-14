# Limites de download: episódios, tamanho de torrent e espaço em disco

Data: 2026-08-10

Três tetos que faltam, agrupados porque são a mesma pergunta ("o app baixa mais do que devia") e
porque todos moram no mesmo trecho de `daemon/episodes.go` + `Config` + tela de Config.

## Problema

**`max_episodes_per_anime` não limita nada em download batch, só mente na contagem.** Um anime
finalizado de 24 episódios com o limite em 12 baixa *um torrent com os 24 arquivos* (Estratégia 2
de `resolveSearchStrategy`, `daemon/episodes.go:133`), o `organize` hardlinka os 24 para a
biblioteca, e `episodes.json` registra 12. A tela mostra 12/24 para sempre enquanto os 24 arquivos
estão no disco.

O outro lado do mesmo defeito: não há teto de tamanho para batch. Naruto (720 eps), Bleach (366),
Gintama e Detective Conan são `FINISHED`, casam com o gatilho de batch e viram um torrent único
gigante.

E não há teto **em bytes** em lugar nenhum. Um pack de 26 episódios em Blu-ray remux tem 300 GB e o
app o aceita igual a um de 12 GB; um "episódio" individual pode vir num release de 8 GB. O único
sinal de tamanho que existe hoje é ordenação: `nyaa.Priorities` desempata por `Size`
(`nyaa/priorities.go:140`), nunca descarta.

Por fim, nada olha o disco antes de baixar. `GET /status` já devolve `disk_free`/`disk_total` e a
tela já pinta o número de vermelho abaixo de um ratio **hardcoded** no frontend
(`lib/utils/status.ts:64`), mas isso é decoração: o daemon continua enfileirando torrents num
volume cheio, e o que o usuário vê depois é falha de hardlink no `organize`.

## Objetivo

1. O limite por anime passa a se aplicar **só quando o download é episódio a episódio**, nunca em
   batch.
2. Um limite novo (`max_batch_episodes`) decide a partir de que tamanho um anime finalizado deixa
   de usar batch e volta ao caminho um-a-um.
3. A contagem em `episodes.json` deixa de mentir sobre o que está no disco.
4. Teto de tamanho em bytes, separado para batch e para episódio individual: torrent acima do teto
   é **descartado do resultado da busca**, não baixado e depois apagado.
5. Abaixo de uma porcentagem configurável de espaço livre, o daemon **para de adicionar torrent** e
   a tela de Status diz isso em cima, não num número vermelho num card.

Não-objetivo: mudar o gatilho de batch em si, nem as estratégias de busca no Nyaa. Não é cota por
anime nem por biblioteca ("no máximo 500 GB de anime"): o sinal aqui é o filesystem.

## A regra: batch versus um-a-um

**Uma regra, válida para todo anime:** `max_episodes_per_anime` se aplica quando o download é
episódio a episódio, e não se aplica em batch.

Um batch é um torrent só: limitar os registros não limita os bytes, não limita os arquivos que vão
para a biblioteca, e só produz a contagem mentirosa descrita no Problema.

A decisão é tomada **antes** do loop de episódios, e é isso que a mantém pequena. O gatilho de
batch em `resolveSearchStrategy` é `animeIsFinished && !animeIsMovie && len(episodesToDownload) > 1`;
os dois primeiros termos já se conhecem de antemão, e o terceiro é irrelevante para a decisão
(0 ou 1 episódio cabe em qualquer limite). Em `processAnimeEpisodes` (`daemon/episodes.go:21`):

```go
// totalEpisodes desconhecido (Media.Episodes == nil) NÃO lança o limite: sem saber o
// tamanho não há como afirmar que o pack cabe no teto. Um anime RELEASING costuma vir
// com nil, e um FINISHED com nil é dado incompleto da AniList — nos dois casos o
// caminho conservador é continuar limitado.
withinBatchCeiling := totalEpisodes > 0 &&
    (configs.MaxBatchEpisodes <= 0 || totalEpisodes <= configs.MaxBatchEpisodes)

willBatch := animeIsFinished && !animeIsMovie && withinBatchCeiling
```

Com `willBatch`, o limite efetivo passado a `checkEpisode`/`handleAlreadySavedEpisode`
(`episodes.go:258,300`) é ilimitado; sem ele, é `configs.MaxEpisodesPerAnime`.

Note que `MaxBatchEpisodes <= 0` significa **teto desligado**, e por isso não pode entrar na
comparação como número: `totalEpisodes <= 0` seria sempre falso e desligar o teto teria o efeito
oposto ao pretendido.

One Piece é `RELEASING`, nunca casa com o gatilho de batch, cai no caminho um-a-um e continua
limitado a 12 episódios por passe.

**O buraco que a regra abre, e o tampão.** Se `willBatch` é verdadeiro o limite é lançado, mas a
busca de batch no Nyaa pode voltar vazia e a Estratégia 3 (multi-episódio) assume — baixando os 24
episódios um a um, sem limite. Depois de `resolveSearchStrategy`, se o resultado **não** é batch
(`skipSubfolder == false`) e o limite havia sido lançado, `episodesToDownload` é cortado de volta
ao limite. A ordem do `airingSchedule` garante que o corte preserva os episódios mais antigos, que
é o certo.

## Config nova: `max_batch_episodes`

Acima deste número de episódios, um anime finalizado **não** usa batch e volta ao caminho um-a-um
(onde `max_episodes_per_anime` volta a valer). Default **30**: uma temporada de 12, 24 ou 26
episódios continua batchando; Naruto, Bleach, Gintama e Detective Conan deixam de virar um torrent
único gigante.

`0` (ou negativo) desliga o teto: todo anime finalizado com contagem conhecida batcha, que é o
comportamento anterior a esta spec.

`Media.Episodes` é `*int` e vem `nil` com frequência — sempre em anime `RELEASING`, e às vezes em
`FINISHED` com dado incompleto. Contagem desconhecida **não** lança o limite (ver o trecho acima):
sem saber o tamanho, não há como afirmar que o pack cabe no teto.

**Mudança de comportamento em biblioteca existente, aceita conscientemente.** Dois caminhos
verificados:

- Anime finalizado de 24 eps com 12 registros e o pack já no disco: `willBatch` verdadeiro, o
  limite é lançado, os eps 13–24 entram como "a baixar", `Session.Add` recebe o mesmo magnet e
  devolve o hash existente (`session.go:76` já é idempotente), e os 12 registros que faltavam são
  criados apontando para o torrent completo. Os arquivos já estavam na biblioteca; agora a
  contagem confere.
- Naruto de 720 eps com 12 registros: `720 > 30`, `willBatch` falso, limite continua 12, os 12
  registros existentes já o consomem em `handleAlreadySavedEpisode`, e nada novo é baixado.

## Correção necessária em `organizeTorrent`

O caminho acima cria registros novos para um torrent **já organizado**. Em `daemon/jobs.go:345`,
`needsOrganize` dispara quando *qualquer* episódio do grupo tem `LibraryPaths` vazio, e o webhook
`DownloadCompleted` sai junto — cada anime finalizado da biblioteca dispararia uma notificação
duplicada no primeiro passe após o upgrade.

Correção, correta em geral e não só nesta migração: se **parte** do grupo já tem `LibraryPaths`, o
torrent já pousou e o webhook já saiu. Organiza (é idempotente), grava o marcador nos registros
novos, **não** notifica.

Vai junto com a regra acima, no mesmo release: hoje o caminho quase não dispara, mas a regra o
transforma em enxurrada de notificações no primeiro passe pós-upgrade de toda biblioteca
existente.

## Teto de tamanho de torrent

Duas configs novas, ambas em **GiB** com decimal (`float64`), `0` = desligado:

| Config | Aplica a | Default |
|---|---|---|
| `max_batch_torrent_size_gb` | resultado de `searchBatch` (Estratégia 2) | `0` |
| `max_episode_torrent_size_gb` | `searchSingleEpisode`, `searchMultiple`, `searchMovie` | `0` |

**Default `0` nos dois, de propósito.** Qualquer default positivo mudaria em silêncio o que uma
biblioteca existente baixa no primeiro passe após o upgrade — e não há número defensável sem saber
se o usuário quer 1080p web ou remux. O hint na tela sugere valores (`~1.5` por episódio 1080p,
`~40` para um pack de 26); o padrão é não filtrar.

Unidade em GiB nos dois porque duas unidades diferentes na mesma tela ("MB por episódio, GB por
pack") é o tipo de detalhe que faz o usuário errar por um fator de 1000. `float64` porque
"1.5 GiB por episódio" é o caso comum e um `int` forçaria a virar MB.

**Onde filtra.** `nyaa.TorrentResult.Size` já é `int64` em bytes (`nyaa/nyaa.go:34`, preenchido por
`parseSize`), então é um filtro de slice, não uma requisição nova:

```go
// daemon/search.go
func filterBySize(results []nyaa.TorrentResult, maxGB float64) []nyaa.TorrentResult
```

Chamado em quatro lugares, cada um com o teto do seu tipo: as três estratégias de
`resolveSearchStrategy` e o fallback de episódio individual dentro do loop de
`processAnimeEpisodes` (`episodes.go:77`). `resolveSearchStrategy` ganha um parâmetro `configs
*files.Config` — os dois call sites (`episodes.go:63`, `debug.go:90`) já o têm em mão.

Filtra **depois** da ordenação por prioridade, não antes: `Priorities` já ordenou por qualidade e
grupo, e o filtro preserva a ordem, então o escolhido continua sendo o melhor **entre os que
cabem**. Filtrar antes daria no mesmo e custaria mexer no `nyaa`.

**`Size == 0` passa o filtro.** É o valor de `parseSize` quando o Nyaa muda o formato da coluna ou
a linha vem sem tamanho. Descartar tamanho desconhecido trocaria "às vezes baixa um torrent grande"
por "não baixa nada", que é pior, e um bug de parsing viraria uma paralisação silenciosa. O
descarte por tamanho loga em Debug com o tamanho e o teto; tamanho zero loga em Debug que passou
sem verificação.

**Sinergia com a regra de batch.** Se o filtro esvazia o resultado do batch, `resolveSearchStrategy`
cai naturalmente na Estratégia 3 e o tampão da seção anterior devolve o limite por anime. Ou seja:
um pack de 26 episódios acima do teto de bytes vira 12 episódios individuais neste passe, sem
código novo para o caso. É o mesmo caminho de "busca de batch voltou vazia".

## Guarda de espaço em disco

Config nova `min_free_disk_percent` (`int`, default **10**, `0` desliga). Abaixo dessa porcentagem
de espaço livre no volume da biblioteca, **nenhum torrent novo é adicionado**.

`files.DiskSpace(path)` já existe (`files/diskspace_unix.go`, `diskspace_windows.go`) e o volume da
biblioteca é o mesmo do diretório de download por construção (`Config.DownloadPath`), então
`CompletedAnimePath` é o único caminho a medir.

```go
// daemon/helpers.go
var ErrInsufficientDiskSpace = errors.New("insufficient free disk space")

// Devolve ErrInsufficientDiskSpace quando o livre está abaixo do teto. Erro de statfs NÃO
// bloqueia: um volume que não responde a statfs (rede, permissão) não é prova de disco cheio,
// e transformar isso em "para de baixar tudo" é pior que o risco que a guarda cobre.
func checkDiskSpace(configs *files.Config) error
```

**Dois call sites, ambos em `daemon`:** `attemptDownloadWithRetries` (`episodes.go:324` — cobre o
loop, o download avulso e qualquer coisa futura que passe pelo `processAnimeEpisodes`) e
`manual_download.go:22` (cobre magnet manual e download manual de episódio). São os dois únicos
chamadores de `backend.Add` fora de teste, verificado por grep.

Não vai em `torrents.Session.Add` — que seria um site só — porque o pacote `torrents` não conhece
`files.Config` e passar a config para lá só para ler uma porcentagem inverte a dependência.

**O passe de verificação continua rodando inteiro.** A guarda barra o `Add`, não o passe: a poda de
episódios assistidos, o `deleteEpisodesByStatus` e o `organize` são justamente o que **libera**
espaço, e um `if disco cheio { return }` no início do passe deixaria o app travado no estado em que
não consegue se desentupir. Escrever isso no código, com comentário: é a "consolidação" óbvia que
alguém tenta depois.

Quando barra, `attemptDownloadWithRetries` devolve `""` **sem tentar nenhum magnet e sem retry** —
o `magnets[i]` não é o problema, tentar 3 vezes só enche o log. Notifica
`notifications.DownloadFailed` com uma razão nova `ReasonNoDiskSpace`; o batch de notificações
(`BatchWindowSeconds`) junta os N episódios do passe numa mensagem só, então não há enxurrada.

**API.** Os handlers que chamam `manual_download` mapeiam `errors.Is(err, ErrInsufficientDiskSpace)`
para **409 `INSUFFICIENT_DISK_SPACE`** — não 500: a causa é conhecida e acionável pelo usuário.

**`GET /status` ganha `disk_low bool`**, calculado no servidor com `min_free_disk_percent`. É o que
mata o `LOW_DISK_SPACE_RATIO` hardcoded do frontend: um único lugar decide o que é "baixo", e o
aviso da tela não pode discordar do que o daemon está fazendo. `isDiskSpaceLow` e seu teste em
`lib/utils/status.ts` são **deletados**, não parametrizados — o cálculo deixa de existir no
frontend.

## Frontend

Na tela de Config, grupo "Downloads":

- `max_batch_episodes` — hint explicando a regra ("acima disso, baixa episódio por episódio
  respeitando o limite por anime"), validação `>= 0` na lista de `requiredChecks`.
- `max_batch_torrent_size_gb` e `max_episode_torrent_size_gb` — hint com os valores de referência,
  `step="0.1"`, validação `>= 0`.
- `min_free_disk_percent` — hint "abaixo disso o download é pausado", validação `0..99`. `100`
  bloquearia todo download para sempre.

No Status, com `status.disk_low`, um alerta `role="alert"` no topo da tela — acima do
`status_error_alert` (`Status.svelte:455`), no mesmo padrão de markup, na variante `danger` em vez
de `warn`: "Espaço em disco baixo — downloads pausados". É o alerta de cima porque um card vermelho
no meio da grade não explica que o app **parou** de baixar; o número do card continua vermelho como
hoje, só deixa de ser o único sinal.

## Testes

- `willBatch` e o limite efetivo: finalizado 26 eps → todos os 26; finalizado 39 eps com limite 30
  → um-a-um cortado em 12; `RELEASING` de 1100 eps → um-a-um em 12; filme → inalterado; finalizado
  com `Media.Episodes == nil` → **continua limitado** em 12; `max_batch_episodes = 0` com 720 eps
  conhecidos → batcha.
- O tampão: `willBatch` verdadeiro mas busca de batch vazia → `episodesToDownload` volta ao limite.
- `organizeTorrent` não redispara `DownloadCompleted` quando parte do grupo já tem `LibraryPaths`.
- `filterBySize`: teto `0` não filtra nada; resultado acima do teto sai; `Size == 0` **fica**; a
  ordem relativa dos que sobram é preservada.
- Teto de batch que esvazia o resultado → cai na Estratégia 3 **e** o limite por anime volta a
  valer (o mesmo caminho do tampão, com o filtro como gatilho em vez da busca vazia).
- Teto de episódio não afeta a escolha de batch, e vice-versa: um pack de 40 GiB com
  `max_episode_torrent_size_gb = 1.5` continua sendo baixado.
- `checkDiskSpace`: livre acima do teto → `nil`; abaixo → `ErrInsufficientDiskSpace`;
  `min_free_disk_percent = 0` → `nil` sempre; erro de `DiskSpace` → `nil` (não bloqueia).
- `attemptDownloadWithRetries` com disco cheio devolve `""` **sem chamar `backend.Add` nenhuma
  vez** (`FakeBackend` com contador) e notifica `ReasonNoDiskSpace`.
- Disco cheio **não** impede a poda: biblioteca com episódios assistidos e
  `delete_watched_episodes` ligado, disco abaixo do teto → os episódios continuam sendo apagados.
- Download manual com disco cheio → 409 `INSUFFICIENT_DISK_SPACE`.
- `GET /status` devolve `disk_low` verdadeiro/falso conforme `min_free_disk_percent`, e `false`
  quando `disk_total == 0` (biblioteca não configurada).

Vitest: o teste de `isDiskSpaceLow` é removido junto com a função.

## Docs a atualizar

- `config.md`: `max_batch_episodes`, `max_batch_torrent_size_gb`, `max_episode_torrent_size_gb`,
  `min_free_disk_percent`.
- `architecture.md`: `disk_low` em `StatusResponse`, `filterBySize` e `checkDiskSpace`.
- `decisions.md`: (a) a regra batch↔limite e por que `max_episodes_per_anime` nunca limitou batch;
  (b) `Size == 0` passa o filtro de tamanho — por que um parsing quebrado não pode virar
  paralisação; (c) a guarda de disco barra o `Add` e **não** o passe, porque a poda é o que libera
  espaço; (d) o limiar de disco vive no servidor (`disk_low`), não no frontend — "don't fix by":
  reintroduzir o ratio em `lib/utils/status.ts` para economizar um campo na resposta.
- `swag init -g src/cmd/daemon/main.go -o docs/swagger` (o `Config` do `GET/PUT /config` e o
  `StatusResponse` mudam).

## Tamanho

~6 arquivos Go tocados (`daemon/episodes.go`, `daemon/search.go`, `daemon/helpers.go`,
`daemon/jobs.go`, `daemon/manual_download.go`, `files/filemanager.go`, `api/endpoint_config.go`,
`api/endpoint_status.go`), ~3 telas Svelte tocadas, ~8 mensagens de i18n. Nenhum arquivo novo,
nenhuma dependência nova, nenhuma requisição de rede nova.
