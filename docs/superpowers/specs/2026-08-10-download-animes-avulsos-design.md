# Download de animes avulsos

Data: 2026-08-10

## Problema

Todo o app é ancorado na lista do AniList. `searchAnilist` (`daemon/verification.go`) monta o
universo de animes do loop a partir das listas das contas configuradas, e **toda** leitura de
um anime individual passa por `mediaList(userName:)` — `anilist.GetAnimeInfo` devolve
`(nil, nil)` quando nenhuma conta acompanha aquele media id. Consequência: um anime que não
está em lista nenhuma é invisível para o daemon, `GET /animes/{id}/episodes` responde 404 e
`daemon.ManualDownload*` falha antes de chegar ao Nyaa.

Não existe caminho para "quero este anime, baixa" sem passar pela lista do AniList.

**Pré-requisito:** a regra batch↔limite de
[`2026-08-10-limite-episodios-batch-design.md`](2026-08-10-limite-episodios-batch-design.md)
sai antes desta feature, em release próprio. Esta spec assume `max_batch_episodes` e o
`willBatch` de `processAnimeEpisodes` já no lugar — é o que faz um anime de 1100 episódios
adicionado a dedo cair no caminho um-a-um limitado em vez de virar um torrent único.

## Objetivo

1. Tela nova com barra de busca no AniList; clicar num resultado passa a acompanhar o anime.
2. O anime avulso é acompanhado **persistentemente**: participa de todo passe de verificação
   junto com os da lista do AniList, e episódios novos entram sozinhos conforme vão ao ar.
3. O download começa no ato da adição, não no próximo ciclo.
4. Avulsos aparecem na lista do Status com um chip que os distingue, e podem deixar de ser
   acompanhados de lá.
5. Adicionar um anime que o app já cobre é **bloqueado nos dois lados**: 409 no backend, card
   cinza com tooltip no frontend, pela mesma função.
6. Avulso que depois entra numa lista do AniList sai do arquivo de avulsos: passa a ser
   acompanhado pela lista.

Não-objetivo: importar lista de outra fonte; adicionar por magnet (já existe em
`AnimeDetail`).

## Vocabulário

O conceito se chama **standalone** no código, não `manual`. `manual` já está tomado e significa
outra coisa: `ManuallyManaged`, `ManualDownloadEpisode` querem dizer "o usuário mexeu neste
episódio à mão, o loop não deve tocar". Reaproveitar a palavra faria dois conceitos diferentes
lerem igual. Na UI, em português, é "avulso".

## Arquitetura

### Estado

Arquivo novo `~/.autoAnimeDownloader/standalone_animes` (Windows:
`%APPDATA%\.autoAnimeDownloader\standalone_animes`) — array JSON de media ids, sem extensão,
exatamente a convenção de `blocked_episodes`:

```json
[21, 154587]
```

Sem `added_at`, sem objeto: a única pergunta que se faz a este arquivo é "este id está aqui?".

`FileManager` ganha três métodos, na forma de `BlockEpisode`/`UnblockEpisode`:

| Método | Comportamento |
|---|---|
| `LoadStandaloneAnimes() ([]int, error)` | Arquivo ausente = lista vazia, não erro |
| `AddStandaloneAnime(id int) error` | Idempotente |
| `RemoveStandaloneAnime(id int) error` | Id ausente não é erro |

Os três entram em `FileManagerInterface` — que é **declarada duas vezes**, em `daemon` e em
`api`. As duas declarações e os mocks de teste mudam.

### AniList: o primitivo que faltava

Duas funções novas em `anilist/anilist.go`:

| Função | Query | Papel |
|---|---|---|
| `SearchMedia(term string) ([]MediaSearchResult, error)` | `Page(perPage:20){media(search:$q, type:ANIME, sort:SEARCH_MATCH){ id, title{english romaji}, format, status, seasonYear, episodes, coverImage{large medium} }}` | Alimenta a barra de busca |
| `GetMediaByID(id int) (*MediaList, error)` | `Media(id:$id)` com os **mesmos campos** de `getMediaListEntry` — inclusive `synonyms`, `relations` e `airingSchedule` com `id` | Leitura de anime independente de lista |

`GetMediaByID` devolve um `MediaList` sintético: só `Media` preenchido, `Progress: 0`,
`Status: ""`, `Id: 0`. Os campos precisam ser os mesmos de `getMediaListEntry` porque
`resolveSearchStrategy` e `searchNyaaForSingleEpisode` dependem de `Synonyms` e `Relations`
(offset de temporada dividida via PREQUEL).

Cacheado 60s no `ttlCache` que já existe, pela mesma razão de `frontendListCache`: `GET /animes`
sofre poll de 30s por aba aberta e passaria a chamar `GetMediaByID` por anime avulso.

### Merge no loop de verificação

`AnimeVerification` carrega os ids avulsos e passa para `searchAnilist(configs, standaloneIDs)`,
que **depois** do `DedupeByMedia` anexa cada avulso ainda ausente da lista mesclada.

A ordem é o requisito, não um detalhe: se o anime também estiver numa lista do AniList, a
entrada real precisa vencer. Anexar antes do dedupe deixaria o `Progress: 0` sintético ganhar —
`DedupeByMedia` mantém o **menor** progresso — e o daemon voltaria a baixar episódios já
assistidos.

Efeito colateral desejável: um avulso que depois entra numa lista do AniList passa a ser
tratado pela entrada real, com progresso — e o registro avulso deixa de ter função.

**Uma definição de "acompanhado pela lista", usada em três lugares.** Um media id está
acompanhado quando aparece no conjunto que o daemon **processa**: as entradas de
`download_statuses` que sobrevivem ao filtro de `DownloadMediaStatuses`. Não é "existe entrada
no AniList": um anime em `PLANNING` com `download_statuses = [CURRENT]` está numa lista e o
daemon o ignora — e adicioná-lo como avulso é o caso de uso mais óbvio da feature, não um erro
a bloquear.

Essa mesma definição alimenta os três consumidores: a remoção abaixo, o bloqueio do `POST` e o
flag do resultado de busca (§ Guardas). Duas definições diferentes produziriam um card cinza na
tela que o backend aceita, ou o inverso.

**Remoção automática.** No passe de verificação, depois do merge e **antes** de anexar os
avulsos, todo id avulso presente na lista real vai para `RemoveStandaloneAnime`, com log. É a
única escrita: a entrada real já vencia o dedupe, então o que aquele passe baixa não muda — o que
muda é que o arquivo para de crescer, o chip "Avulso" desaparece da tela e o `is_standalone` deixa
de mentir sobre a origem daquele anime.

**A consequência tem de estar escrita, e ela chega a apagar arquivo:** se depois disso o usuário
tirar o anime da lista do AniList, o registro avulso já foi consumido e o passe deixa de cobri-lo.
Com `delete_watched_episodes` **ligado**, os episódios dele caem em `identifyEpisodesNotInWatching`
e `handleSavedEpisodes` os remove do disco (`episodes.go:465`) — o mesmo caminho que qualquer
anime tirado da lista já percorre hoje, mas que o registro avulso vinha impedindo. Com a config
desligada, o efeito para em "deixa de baixar episódio novo".

É a troca aceita, porque a alternativa é pior de manter: preservar os dois registros exigiria
distinguir "avulso ativo" de "avulso dormente" no arquivo, no `AnimeInfo` e na tela, para cobrir
um caminho (adicionar à lista e tirar depois) que quem usa avulso não percorre. Quem quiser o
anime de volta o adiciona como avulso outra vez — e nesse ponto o `blockReason` já não bloqueia,
porque ele saiu da lista processada.

### Guardas de adição: `standaloneGuard`

Arquivo novo `api/standalone_guard.go`. Uma função de bloqueio, dois consumidores — o `POST` e o
handler de busca. É a mesma função nos dois porque "o front não deixa clicar" e "o back devolve
erro" precisam concordar por construção, não por disciplina.

```go
type standaloneGuard struct {
    standalone  map[int]bool // standalone_animes
    downloaded  map[int]int  // mediaID → nº de registros em episodes.json
    tracked     map[int]bool // snapshot de fetchAniListEntries (§ Merge)
    blacklisted map[int]bool // customLists ∩ excluded_lists
}

// "" = pode adicionar. Precedência: blacklist > avulso > lista > baixado.
func (g standaloneGuard) blockReason(mediaID, totalEpisodes int) string
```

**Nenhuma query nova.** Os quatro sinais já existem e três deles já vêm de cache: o arquivo de
avulsos e o `episodes.json` são locais; `tracked` e `blacklisted` saem de `fetchAniListEntries`
(`frontendListCache`, 60s) e `GetCustomListsMap` (5min) — exatamente o par que `GET /animes`
monta hoje. Uma busca de 20 resultados custa o mesmo que zero.

`tracked` sai de `fetchAniListEntries`, não de `searchAnilist`: são consultas diferentes com **os
mesmos dois filtros** (`download_statuses` server-side + `MediaStatusAllowed`), e a do pacote `api`
é a que já tem cache no caminho do frontend. O `api` não passa a importar `daemon` por causa
disto — o que tem de ser igual é a regra, e ela está escrita no § Merge.

**`downloaded` só bloqueia com contagem conhecida:** `totalEpisodes > 0 && downloaded >= totalEpisodes`.
Total desconhecido nunca bloqueia, e um anime de 24 episódios com 12 registros (o limite por
anime) não é "já baixado" — é justamente o caso que a regra batch↔limite (spec separada) passa a
completar.

**Blacklist vem primeiro na precedência** porque é o único motivo em que adicionar mudaria o
comportamento **para pior**: os outros três são inócuos (os registros existentes já fazem o loop
pular tudo) e a mensagem é clareza. Já um blacklisted em status fora de `download_statuses`
escapa do `searchAnilist`, o registro avulso sobrevive ao merge e o filtro do usuário é
contornado — o `MediaList` sintético do `GetMediaByID` tem `CustomLists` nulo, então
`animeIsInExcludedList` nunca dispara nele. Dentro de `download_statuses` o vazamento não
existe: a entrada real vence o dedupe e o loop a exclui normalmente.

| Motivo | Código HTTP 409 | Tooltip |
|---|---|---|
| `blacklist` | `ALREADY_BLACKLISTED` | "Na blacklist" |
| `standalone` | `ALREADY_STANDALONE` | — (botão "Adicionado", é sucesso) |
| `tracked` | `ALREADY_TRACKED` | "Já está na sua lista do AniList" |
| `downloaded` | `ALREADY_DOWNLOADED` | "Já baixado" |

### Endpoints

| Método | Rota | Handler / arquivo |
|---|---|---|
| `GET` | `/api/v1/anilist/search?q=<termo>` | `handleAniListSearch` — `endpoint_anilist_search.go` |
| `POST` | `/api/v1/standalone-animes` | `handleStandaloneAnimeAdd` — `endpoint_standalone_animes.go` |
| `DELETE` | `/api/v1/standalone-animes/{id}?delete_episodes=<bool>` | `handleStandaloneAnimeRemove` — mesmo arquivo |

**`GET /anilist/search`** devolve, por resultado: `id`, `title`, `format`, `status`, `year`,
`episodes`, `cover`, e **`block_reason`** — `""` quando pode adicionar, senão um dos quatro
motivos da § Guardas. Termo vazio ou com menos de 3 caracteres devolve lista vazia, não 400.

`block_reason` é **um campo, não quatro booleanos**, e substitui o `already_standalone` que uma
versão anterior desta spec previa: os motivos são mutuamente exclusivos por precedência, o front
precisa de um rótulo só por card, e a string vem do mesmo `blockReason` que o `POST` usa para
decidir o 409.

**`POST /standalone-animes`**, body `{"media_id": 21}`:

1. Biblioteca não configurada (`configs.DownloadPath() == ""`) ⇒ 409 `LIBRARY_NOT_CONFIGURED`.
   Primeiro porque não depende de rede e porque, sem ela, o `Ensure` do passo 4 falha e o
   usuário recebe um 500 opaco no lugar da causa.
2. `GetMediaByID(id)` — `nil` ⇒ 404. É também o que valida que o id existe e é ANIME, **e** de
   onde sai o `totalEpisodes` que o passo 3 precisa.
3. `blockReason(id, totalEpisodes)` ⇒ 409 com o código da tabela da § Guardas.
4. `AddStandaloneAnime(id)`.
5. `daemon.DownloadStandaloneAnime(...)`.
6. `200` com `{"added": N}`.

`N == 0` é resposta normal, não erro: um anime `NOT_YET_RELEASED`, ou um cujo próximo episódio
ainda não foi ao ar, é adicionado e fica acompanhado — é exatamente o objetivo 2. Quem traduz
isso é o toast (§ Frontend), não um código de erro.

**Síncrono**, seguindo `POST /animes/{id}/episodes/{episodeId}/download`, que já espera o Nyaa
dentro do handler. É uma busca só (`searchMultiple` cobre os N episódios de uma vez) e devolver
o número real de episódios enfileirados vale mais que um 202 vazio.

**`DELETE /standalone-animes/{id}`**: `RemoveStandaloneAnime(id)`; com `delete_episodes=true`
chama `daemon.RemoveEpisodesWithLinks` com os episódios daquele anime; sem o parâmetro (default
`false`, via o `parseBoolQueryParam` que já existe em `endpoint_torrents.go`) faz
`UpsertEpisodes` marcando-os `ManuallyManaged=true`.

Esse `ManuallyManaged` é o que fecha a janela em que os arquivos correriam risco ao deixar de
acompanhar **por este endpoint** — ver § Deleção automática, que também descreve a outra janela
(a adoção por lista do § Merge), essa deliberadamente aberta.

**Não existe `GET /standalone-animes`.** Ninguém precisaria dele: a tela de busca recebe
`block_reason` no próprio resultado, e o Status recebe `is_standalone` dentro do `AnimeInfo` que
já busca.

### `daemon.DownloadStandaloneAnime` (arquivo novo `daemon/standalone.go`)

Assinatura: `DownloadStandaloneAnime(fm FileManagerInterface, backend torrents.TorrentBackend, configs *files.Config, mediaID int) (int, error)`.

Faz exatamente três coisas: `backend.Ensure(configs.DownloadPath())`, `processAnimeEpisodes`
para aquele anime, `saveEpisodesToFile(fm, result.newEpisodes)`. Devolve
`len(result.newEpisodes)`.

**Não pode chamar `handleSavedEpisodes`, e isso precisa de comentário no código.**
`handleSavedEpisodes` chama `identifyEpisodesNotInWatching`, que compara *todos* os episódios
salvos contra os `checkedEpisodes` recebidos e devolve os não cobertos para deleção. Com os
episódios de um único anime na mão e `delete_watched_episodes` ligado, ela apagaria o resto da
biblioteca inteira. Sem o comentário, alguém "consolida" as duas depois.

Também não chama `deleteEpisodesByStatus` nem `DeleteEmptyFolders`: adicionar um anime não é
ocasião para apagar nada.

### `resolveMediaList`: um helper, três call sites

`GetAnimeInfo` devolvendo `nil` para quem não está em lista de conta nenhuma quebra três
lugares para um avulso:

| Local | Sintoma sem o fallback |
|---|---|
| `handleAnimeEpisodes` (`endpoint_anime_episodes.go`) | 404 — a tela de detalhe não abre |
| merge de `handleAnimes` (`endpoint_animes.go`) | O avulso só aparece na lista depois de ter episódio baixado |
| `refreshOrphanAnimes` (`endpoint_animes.go`) | Fica sem capa, sem total de episódios |

`resolveMediaList(id, usernames, standaloneSet)` tenta `GetAnimeInfo` e cai em `GetMediaByID`
quando o id é avulso. Uma condição em cada call site.

`AnimeInfo` ganha `is_standalone bool` (`is_standalone`, `omitempty`).

### Deleção automática: o que muda e o que cai de graça

| Mecanismo | Avulso |
|---|---|
| `deletableMediaIDs` (deleção por status) | **Nunca** o alcança: só entram candidatos que apareceram na lista de alguma conta, e um avulso não aparece em nenhuma |
| `delete_watched_episodes` / `watched_episodes_to_keep` | **Nunca** o alcança: `Progress` é sempre 0, então nenhum episódio é "assistido". O avulso acumula no disco até o usuário deixar de acompanhá-lo — que é o que "avulso" deveria significar |
| `identifyEpisodesNotInWatching` (poda dos não cobertos) | Protegido **enquanto** está na lista: é processado em todo passe, então seus episódios entram em `checkedEpisodes`. A janela de risco é o instante em que sai da lista, e o `ManuallyManaged` do `DELETE` é o que a fecha. A remoção automática do § Merge abre uma **segunda** janela, sem `ManuallyManaged` para fechá-la: o avulso adotado por uma lista e depois tirado dela fica desprotegido como qualquer anime de lista — descrito lá |

As duas primeiras linhas são consequência da arquitetura existente, não decisões novas — vão
para `decisions.md` como comportamento documentado, não como correção.

## Frontend

### Rota nova `#/add` → `routes/AddAnime.svelte`

Como se chega:

- Botão primário **"+ Adicionar anime"** no header do `Status` — o caminho real. Desabilitado,
  com tooltip apontando para `#/config`, quando a biblioteca não está configurada: é o mesmo
  bloqueio do `LIBRARY_NOT_CONFIGURED`, dito antes de o usuário digitar uma busca inteira para
  descobrir que nada pode ser adicionado.
- Item em `navItems.ts` com grupo `more`, aparecendo no `MoreMenu` no desktop e no mobile.
  Deliberadamente **não** no rail nem na `NavTabBar`: a tab bar é um grid de 4 colunas fixas e
  um quinto item quebraria o layout.

A tela: `<input>` com debounce de 300ms e `AbortController` cancelando a requisição anterior.
Os dois são requisito, não polimento — sem debounce o rate limit de 30 req/min do AniList
estoura digitando, e sem o abort um resultado velho pinta por cima do novo. Busca a partir de 3
caracteres.

Cada resultado: `Cover` + título + uma linha de meta (`TV · 2016 · 24 eps · Finalizado`) +
botão, cujo estado vem do `block_reason`:

| `block_reason` | Card | Botão |
|---|---|---|
| `""` | normal | `Adicionar` → `Adicionando…` → `Adicionado` |
| `standalone` | normal | `Adicionado`, desabilitado — é sucesso, não impedimento |
| `blacklist` / `tracked` / `downloaded` | `opacity-50` | desabilitado, dentro de um wrapper `tooltip` com o motivo |

O tooltip envolve o **botão desabilitado**, no mesmo padrão `<div class="tooltip" data-tip=...>`
de `NavRail.svelte:141` — um `<button disabled>` não emite eventos de mouse, então o `data-tip`
tem de morar no wrapper para o tooltip aparecer. O card inteiro não é clicável em nenhum estado
(não há navegação a partir da busca), então cinza + botão desabilitado é a superfície toda.

**O front é best-effort e o backend é a autoridade.** O `block_reason` sai de um snapshot com até
60s de idade, então um card pode estar clicável para um anime que acabou de entrar na lista. O
`POST` devolve 409 e o toast traduz o código — as mesmas quatro mensagens dos tooltips, mais
`LIBRARY_NOT_CONFIGURED`. Não há retry nem revalidação: a próxima busca já vem correta.

Sucesso com `added > 0` mostra toast com a contagem e um link para `#/downloads`; com
`added == 0`, toast neutro "acompanhando — nenhum episódio disponível ainda", sem link. Um "0
episódios adicionados" seco leria como falha no caso mais comum de anime que ainda vai estrear.

Estados vazios: sem termo digitado, sem resultado, erro de busca.

Nenhum componente novo: `Cover`, `Chip`, `Button`, `Loading` e `toasts` cobrem tudo — a grade
de capas é um `<ul>` com `grid`.

### Status

`AnimeInfo.is_standalone` renderiza um `Chip variant="neutral"` "Avulso" **ao lado** do chip
derivado, nunca dentro de `deriveAnimeChip`. Aquela cascata devolve um único estado de
download; "avulso" é origem, não estado — enfiar lá faria um avulso baixando perder o chip
"Baixando".

### AnimeDetail: deixar de acompanhar

Ação **"Deixar de acompanhar"** no header do `AnimeDetail`, visível só quando `is_standalone`.
É onde o usuário já está depois de clicar no anime na lista do Status, e o header já é o lugar
das ações de anime (breadcrumb, capa, magnet, query customizada). Colocá-la na linha do Status
custaria uma coluna de `ActionMenu` que a lista não tem hoje.

Diálogo: `ConfirmDialog` (que já expõe slot para conteúdo extra) + um `Checkbox` "apagar
arquivos baixados", desmarcado por default, inline no `AnimeDetail`. `TorrentDeleteDialog` não
serve — é específico de `keepData`/`block`.

### Cliente de API

`lib/api/client.ts`: `searchAniList(q)`, `addStandaloneAnime(id)`,
`removeStandaloneAnime(id, deleteEpisodes)`. A busca passa `{ silent: true }` — erro de digitação
não deve virar toast a cada tecla.

## Testes

Go, o que tem decisão:

- Merge pós-dedupe: avulso que também está numa lista do AniList não duplica **e** não zera o
  `Progress`.
- `DownloadStandaloneAnime` não apaga nada: biblioteca com 3 animes e
  `delete_watched_episodes` ligado, adiciona um avulso, os 3 continuam salvos.
- `DELETE` com e sem `delete_episodes` — no caso "sem", os episódios ficam com
  `ManuallyManaged=true`.
- `SearchMedia` e `GetMediaByID` via `MockAniListDo`.
- `blockReason`, tabela: já avulso → `standalone`; na lista processada → `tracked`; em lista
  excluída → `blacklist`; 24 de 24 registros → `downloaded`; **12 de 24 registros → `""`** (o
  limite por anime não é "já baixado"); **`totalEpisodes == 0` com registros → `""`**; nada →
  `""`; avulso **e** em lista excluída → `blacklist` (a precedência, não `standalone`).
- Um anime em `PLANNING` com `download_statuses = [CURRENT]` → `""`. É o caso de uso da feature;
  se este teste virar `tracked`, a definição de "acompanhado" regrediu para "existe entrada".
- `POST` devolve 409 com cada um dos cinco códigos, **e não chama `AddStandaloneAnime` nem
  `DownloadStandaloneAnime` em nenhum deles** — um bloqueio que grava no arquivo antes de recusar
  seria pior que nenhum bloqueio.
- `POST` com biblioteca não configurada devolve 409 sem tocar a AniList.
- `POST` de anime sem episódio no ar → 200 com `{"added": 0}`, e o id **está** no arquivo.
- Remoção automática: avulso presente na lista processada sai do arquivo no passe; avulso em
  `PLANNING` (fora de `download_statuses`) **permanece**; a remoção acontece antes do append e
  não altera quais episódios o passe baixa.
- `GET /anilist/search` preenche `block_reason` com os mesmos valores para a mesma entrada — o
  teste que trava front e back na mesma função.

Vitest: nada. A tela é I/O e render; o `block_reason` chega pronto do servidor e o mapa
motivo → rótulo é um lookup no componente, não lógica de domínio.

## Docs a atualizar

- `architecture.md`: arquivos novos, os 3 endpoints, `standalone_animes` na tabela de estado,
  rota `#/add`, `SearchMedia`/`GetMediaByID`, `resolveMediaList`, `is_standalone` em `AnimeInfo`,
  `standaloneGuard`/`blockReason` e `block_reason` no resultado da busca.
- `decisions.md`: (a) por que
  `DownloadStandaloneAnime` não pode chamar `handleSavedEpisodes`; (b) avulso nunca é apagado
  por status nem por assistido; (c) "acompanhado pela lista" é o snapshot que o daemon processa,
  não "existe entrada no AniList" — com o `PLANNING` como o caso que a definição errada quebra, e
  a nota de que o mesmo `blockReason` serve os dois lados de propósito ("don't fix by": duplicar
  a regra no frontend para evitar o campo na resposta da busca).
- `swag init -g src/cmd/daemon/main.go -o docs/swagger`.

## Fora de escopo

**Blacklist em status fora de `download_statuses`.** Um anime numa custom list excluída, mas com
status de lista que o daemon não busca (`PLANNING`, `DROPPED`), não aparece no snapshot de onde
`blacklisted` sai — nem na busca nem no `POST`. Adicioná-lo como avulso contorna o filtro de
exclusão. Fechar isso exigiria uma query por id **com** `customLists`, que é justamente o campo
que a AniList devolve nulo em query complexa (`decisions.md` §11, o motivo de
`GetCustomListsMap` existir): seria uma segunda chamada de rede por resultado de busca para um
sinal que a própria API entrega de forma pouco confiável. Se aparecer na prática, o conserto é
tornar `GetCustomListsMap` chamável sem filtro de status e cachear por conta.

**Trava contra duplo clique ou duas abas adicionando o mesmo anime.** `AddStandaloneAnime` é
idempotente e `Session.Add` devolve o hash existente para o mesmo magnet (`session.go:76`), então
o pior caso é uma busca no Nyaa repetida. Trava exigiria estado por media id em memória para
economizar uma requisição que o usuário provocou.

**Anime cujos episódios estão todos em `blocked_episodes`.** Adicionar não baixa nada. É o mesmo
não-efeito de `added == 0`, o toast já cobre, e a checagem custaria cruzar o arquivo de bloqueios
com o `airingSchedule` de cada resultado da busca.

**Auto-remoção do avulso quando o anime termina e está todo baixado.** A lista cresce para
sempre. Adicionar quando isso virar incômodo real.

**Modal em vez de tela.** Considerado — a tela tem um único trabalho, já que a gestão dos
avulsos vive no Status. Mas rota é linkável, recarregável, e cabe melhor no mobile que um modal
com grade de capas.

## Tamanho

~6 arquivos Go novos, ~7 tocados; 1 rota Svelte nova, ~4 tocadas; ~22 mensagens de i18n (as
quatro dos tooltips/toasts de bloqueio, a do `LIBRARY_NOT_CONFIGURED` e a do `added == 0`).
