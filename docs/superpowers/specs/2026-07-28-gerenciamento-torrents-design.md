# Gerenciamento de torrents — delete, filtros, seleção em massa e status inline

Data: 2026-07-28
Branch base: `feature/torrent-integrado` (commit `2e0f18e`)

## Objetivo

Fechar quatro lacunas do backlog do torrent integrado:

1. Excluir um torrent pela UI (o backend já tem a capacidade, falta expor).
2. Filtro, busca e ordenação na tabela de Downloads.
3. Seleção em massa com ações em lote na tabela de Downloads.
4. Status do torrent inline na tela de detalhe do anime (`/status/:id`).

## Fora de escopo

**Limite de downloads simultâneos.** Precisa de campo de config novo, componente de fila no daemon e persistência do motivo da pausa (a rain persiste o estado pausado, não o motivo — sem isso o scheduler retomaria torrents pausados de propósito pelo usuário). Vira spec própria.

**Prioridade por torrent.** Decisão tomada e registrada aqui para não ser re-investigada: `github.com/cenkalti/rain/v2` não tem conceito nenhum de prioridade — nem por torrent, nem por arquivo. Só existe `SpeedLimitDownload`/`SpeedLimitUpload` global. Construir throttling próprio em cima disso mexeria no caminho quente da lib e não compensa. **Se** a fila de simultâneos for implementada depois, a ordem da fila é a prioridade, e um botão "baixar primeiro" que move para o topo entrega o valor sem scheduler nenhum.

## Contexto do código atual

Fatos apurados que a implementação depende:

- `torrents.Session.Remove(hash, keepData)` existe (`session.go:88`) e já é usado em `HandleTorrentFailure` (`daemon/helpers.go:75`) e em `removeEpisodesAndLinks` (`daemon/episodes.go:463`). Falta só a rota.
- `endpoint_torrents.go` hoje tem `GET /torrents` e três ações POST (pause/resume/announce), todas construídas sobre o helper `torrentAction`.
- **Armadilha do delete:** `handleAlreadySavedEpisode` (`daemon/episodes.go:302`) rebaixa qualquer episódio cujo registro exista mas cujo hash não esteja no cliente ("Redownloading episode: was missing from torrents"). Remover só o torrent, deixando o registro, é um restart de download — nunca um delete.
- `removeEpisodesAndLinks` (`daemon/episodes.go:422`) já resolve corretamente o caso batch: `allEpisodesInDeleteSet` só libera a remoção do torrent e dos hardlinks quando nenhum episódio irmão sobrevive. Ele fixa `keepData=false`.
- `AnimeDetail.svelte` já implementa seleção em massa completa (`Set<number>`, select-all, barra de ações). É o padrão a copiar, não a reinventar.
- `TorrentResponse` já carrega `anime_id`, `episode_number` e `is_batch`; `AnimeEpisodeInfo` **não** carrega o hash do episódio.
- `GET /torrents` já é consumido por `Downloads.svelte` (2s) e `Status.svelte` (5s). Polling desse endpoint é padrão estabelecido.
- `Config.svelte:149` já tem o padrão de leitura de query param em rota hash.
- `FakeBackend.Remove` (`fakebackend.go:101`) descarta o argumento `keepData`.

## Arquitetura

### Componente 1 — `DELETE /api/v1/torrents/{hash}`

Handler novo em `src/internal/api/endpoint_torrents.go`. Não usa o helper `torrentAction` (aquele é POST-only e sem query params), mas segue a mesma forma: valida método, valida hash, 404 quando o torrent não está na sessão, delega, responde com `JSONSuccess`.

```
DELETE /api/v1/torrents/{hash}?keep_data=<bool>&block=<bool>
```

| Param | Default | Efeito |
|---|---|---|
| `keep_data` | `false` | `true` mantém no disco a cópia semeada **e** os hardlinks da biblioteca |
| `block` | `false` | `true` bloqueia os episódios do torrent contra re-download automático |

Valores aceitos: `true`/`false`/`1`/`0`/ausente (via `strconv.ParseBool`, com ausente ⇒ `false`). Valor não-parseável ⇒ `400 INVALID_QUERY_PARAM`.

Respostas: `200` sucesso, `400` hash vazio ou param inválido, `404` hash não está na sessão, `405` método errado, `500` falha na remoção.

O handler delega tudo para o daemon; nenhuma lógica de biblioteca ou de episódios vive na camada de API.

**Caso de borda deliberado:** o 404 é decidido só pela presença do hash na sessão do cliente, igual ao helper `torrentAction` já faz. Se o torrent não está na sessão mas registros de episódio com aquele hash existem, a resposta é 404 e os registros ficam — limpar registro órfão é assunto do fluxo de episódio (`DELETE /animes/{id}/episodes/{episodeId}`), não desta rota.

### Componente 2 — `daemon.RemoveTorrentWithEpisodes`

Função exportada nova em `src/internal/daemon/episodes.go`:

```go
type RemoveTorrentOptions struct {
    KeepData bool
    Block    bool
}

func RemoveTorrentWithEpisodes(
    fm FileManagerInterface,
    backend torrents.TorrentBackend,
    librarian files.Librarian,
    hash string,
    opts RemoveTorrentOptions,
) error
```

Fluxo:

1. `fm.LoadSavedEpisodes()` — erro aqui aborta com `fmt.Errorf("failed to load saved episodes: %w", err)` e nada é removido. Junta os episódios com `EpisodeHash == hash`. **A unidade de exclusão aqui é o torrent**, não o episódio — num batch, todos os episódios daquele hash saem juntos.
2. **Torrent órfão** (nenhum episódio casou — adicionado à mão, ou registro já removido): chama `backend.Remove(hash, opts.KeepData)` e retorna. `Block` é ignorado, não há id para bloquear.
3. Caso normal: se `opts.Block`, chama `fm.BlockEpisode(id)` para cada `EpisodeID` do grupo **antes** de remover os registros. Falha de bloqueio é logada em `Warn` e não aborta — o bloqueio é proteção, não o trabalho principal.
4. Chama `removeEpisodesAndLinks(fm, backend, librarian, ids, saved, opts.KeepData)` com o grupo inteiro como delete set. Como o delete set é o torrent completo, `allEpisodesInDeleteSet` devolve `true` e o helper já faz o certo: remove hardlinks, remove o torrent, apaga os registros.
5. Devolve o erro de `removeEpisodesAndLinks` sem envolver — o handler responde 500.

### Componente 3 — `keepData` em `removeEpisodesAndLinks`

Único ajuste na função existente: ganha um parâmetro `keepData bool`, que

- é repassado a `backend.Remove(ep.EpisodeHash, keepData)` (hoje literal `false`, linha 463);
- quando `true`, pula o laço de `librarian.RemoveFromLibrary`.

Os três chamadores atuais (`RemoveEpisodesWithLinks`, `deleteEpisodesByStatus`, `handleSavedEpisodes`) passam `false`. Comportamento atual inalterado.

**Por que os dois lados andam juntos:** os arquivos da biblioteca são hardlinks para a cópia semeada — mesmo inode. Apagar um e manter o outro não libera espaço nenhum. `keep_data` é honestamente binário.

### Componente 4 — `episode_hash` em `AnimeEpisodeInfo`

Campo novo em `src/internal/api/endpoint_anime_episodes.go`:

```go
EpisodeHash string `json:"episode_hash,omitempty"`
```

Preenchido a partir do episódio salvo que o handler já tem em mãos ao montar `IsDownloaded`/`DownloadDate`.

É o que torna o join do status inline exato: um torrent batch vem com `episode_number: null`, então juntar por número de episódio não cobriria batch. Com o hash, batch e single usam o mesmo caminho.

### Componente 5 — Tela de Downloads

`Downloads.svelte` (218 linhas) ganharia filtro, ordenação, seleção e diálogo, indo para ~500. Quebra em unidades com uma responsabilidade cada:

| Arquivo | Responsabilidade | Depende de |
|---|---|---|
| `src/lib/utils/torrentFilters.ts` | Funções puras: filtrar, ordenar, serializar/desserializar o estado da view | Só o tipo `TorrentInfo` |
| `src/components/DownloadsToolbar.svelte` | Busca, filtro de status, contador de seleção, barra de ações em massa | Emite eventos; não busca dado |
| `src/components/TorrentDeleteDialog.svelte` | Os dois checkboxes e o texto de consequência | `ConfirmDialog` |
| `src/routes/Downloads.svelte` | Fetch, polling, orquestração das ações, tabela | Os três acima + `api/client` |

`torrentFilters.ts` expõe:

```ts
type SortKey = 'default' | 'name' | 'progress' | 'download_speed' | 'eta' | 'size' | 'peers'
type SortDir = 'asc' | 'desc'
type ViewState = { query: string; statuses: string[]; sort: SortKey; dir: SortDir }

filterTorrents(list: TorrentInfo[], v: Pick<ViewState,'query'|'statuses'>): TorrentInfo[]
sortTorrents(list: TorrentInfo[], sort: SortKey, dir: SortDir): TorrentInfo[]
encodeViewState(v: ViewState): string   // querystring
decodeViewState(qs: string): ViewState  // tolerante a lixo, cai no default
```

Comportamento:

- **Busca** normaliza acento e caixa (`normalize("NFD")` + strip de diacríticos) e casa contra `anime_name` e `name`.
- **Filtro de status** é multi-seleção sobre os slugs que o backend já emite (`stopped`, `downloading_metadata`, `allocating`, `verifying`, `downloading`, `seeding`, `stopping`). Nenhum slug novo é inventado. Lista vazia = sem filtro.
- **Ordenação** por clique no cabeçalho. `sort: 'default'` preserva a ordem que o backend manda (não-concluídos primeiro, depois alfabética) — o backend continua ordenando e não muda. A ordenação do cliente só entra quando o usuário clica. Com `sort: 'default'`, `dir` é ignorado e some da URL. `eta_seconds: null` vai sempre para o fim, nas duas direções.
- **Estado na URL**, não em `localStorage`: `#/downloads?q=frieren&status=downloading,seeding&sort=progress&dir=desc`. Via o store `querystring` do `svelte-spa-router` e `replace()` (não empilha histórico). Campos no default são omitidos da URL, para a URL limpa continuar limpa.

Seleção em massa, seguindo o padrão de `AnimeDetail.svelte`:

- `Set<string>` de hashes, checkbox por linha, select-all no cabeçalho, barra de ações visível com seleção > 0.
- **Select-all marca apenas as linhas visíveis** (pós-filtro), nunca a lista inteira.
- **Ao mudar filtro ou busca, a seleção é podada** para o que continua visível. Evita que uma ação em lote atinja algo que saiu de vista.
- Ações em lote: pausar, retomar, reanunciar, excluir. Cada uma filtra o que faz sentido (não manda pausar quem já está `stopped`/`stopping`) e dispara N requisições aos endpoints por hash já existentes. Toast agregado: "3 de 4 pausados".
- **Sem endpoint de lote.** São dezenas de torrents no pior caso; um endpoint em lote traria semântica de erro parcial que não paga o custo.

`ConfirmDialog.svelte` ganha um `<slot />` opcional entre a mensagem e os botões, para `TorrentDeleteDialog` compor em cima dele em vez de duplicar o modal.

O diálogo de exclusão tem dois checkboxes, ambos **marcados por default**:

- "Apagar os arquivos baixados" → manda `keep_data=false`
- "Não baixar este episódio de novo" → manda `block=true`

E um texto de consequência que muda conforme os checkboxes. Com "não baixar de novo" desmarcado, o texto avisa que o daemon vai rebaixar no próximo passe — porque vai (`handleAlreadySavedEpisode`). É a combinação incoerente do desenho, e ela é tratada na UI, com texto, não bloqueada no backend.

### Componente 6 — Status inline em `/status/:id`

`AnimeDetail.svelte` já tem 856 linhas; quase nada novo entra nele.

`src/lib/utils/torrentsByEpisode.ts` — puro:

```ts
indexTorrentsByEpisode(torrents: TorrentInfo[], episodes: AnimeEpisodeInfo[]): Map<number, TorrentInfo>
```

Junta por `episode.episode_hash === torrent.hash`. Um torrent batch aparece no mapa para **todos** os episódios que compartilham aquele hash. Torrent de outro anime não entra (nenhum episódio casa).

`AnimeDetail.svelte`:

- Busca `GET /torrents` (o mesmo endpoint que Downloads e Status já consomem — sem endpoint novo).
- Na linha do episódio, quando existe torrent **não-concluído** (`completed === false`), renderiza barra de progresso, badge de status e velocidade/ETA. Episódio já na biblioteca fica exatamente como está hoje.
- **Sem botões.** Pausar/retomar continua sendo assunto da tela de Downloads — a linha do episódio já tem baixar/excluir/rebaixar/substituir/liberar.
- **Polling adaptativo:** 2s enquanto houver torrent ativo deste anime, 15s quando não houver. Sem torrent ativo, a tela se comporta como hoje.
- Falha no `GET /torrents` degrada em silêncio (sem barras), espelhando o join best-effort que `handleTorrents` já faz com os episódios. A página do anime não pode quebrar por dado acessório.

### i18n

Toda string nova entra em `src/internal/frontend/messages/en.json` e `messages/pt-BR.json`, com recompilação via `bun run i18n:compile`. O `messages.js` compilado não é editado à mão.

## Tratamento de erro

| Situação | Resposta |
|---|---|
| Hash não está na sessão | `404 TORRENT_NOT_FOUND`, nada é tocado |
| `keep_data`/`block` não-parseáveis | `400 INVALID_QUERY_PARAM` |
| `LoadSavedEpisodes` falha | `500`, nada é removido — remover o torrent sem saber quais registros apagar deixaria estado inconsistente |
| `BlockEpisode` falha | `Warn` no log, remoção segue |
| `RemoveFromLibrary` falha num path | `Warn` no log, segue nos demais (comportamento atual, preservado) |
| `backend.Remove` falha, caso normal | `Warn` no log; o retorno é decidido por `DeleteEpisodesFromFile` (comportamento atual de `removeEpisodesAndLinks`, preservado) |
| `backend.Remove` falha, torrent órfão | `500` — é a única operação daquele caminho, não há o que preservar |
| `DeleteEpisodesFromFile` falha | `500` — o registro sobreviveu, então o estado é o de antes |
| Ação em lote falha em alguns | Toast agregado com o parcial; a lista recarrega e mostra a verdade |
| `GET /torrents` falha em `AnimeDetail` | Silencioso, sem barras |

## Testes

### Go

`src/internal/api/endpoint_torrents_test.go`:

- Delete feliz: torrent sai da sessão e os registros de episódio somem.
- `keep_data=true` chega no backend como `keepData=true` e os hardlinks da biblioteca sobrevivem.
- `block=true` bloqueia **todos** os ids do grupo (importa no batch).
- Torrent órfão (sem registro casando) é removido sem erro e sem bloquear nada.
- Hash desconhecido ⇒ 404. Método errado ⇒ 405. Hash vazio ⇒ 400. `keep_data=talvez` ⇒ 400.

`src/internal/daemon/episodes_test.go`:

- `RemoveTorrentWithEpisodes` num batch com N episódios remove os N registros de uma vez e chama `backend.Remove` uma única vez.
- `keepData=true` não chama `librarian.RemoveFromLibrary`.

`src/internal/api/endpoint_anime_episodes_test.go`:

- `episode_hash` presente para episódio baixado, ausente (`omitempty`) para não baixado.

**Pré-requisito de teste:** `FakeBackend` ganha `RemovedKeepData map[string]bool`, populado em `Remove`. Hoje o argumento é descartado (`fakebackend.go:101`), e sem isso o teste de `keep_data` não prova nada.

### Frontend

`tests/unit/torrentFilters.test.ts`:

- Busca casa com e sem acento, com e sem caixa, tanto em `anime_name` quanto em `name`.
- Filtro de status com múltiplos slugs; lista vazia não filtra.
- Cada `SortKey` nas duas direções; `eta_seconds: null` sempre por último.
- `sort: 'default'` preserva a ordem recebida.
- `encodeViewState`/`decodeViewState` round-trip; querystring com lixo cai no default.

`tests/unit/torrentsByEpisode.test.ts`:

- Episódio single casa com seu torrent.
- Batch cobrindo N episódios aparece nas N entradas.
- Torrent órfão não entra.
- Torrent de outro anime não vaza.

`tests/component/TorrentDeleteDialog.test.ts`:

- Ambos os checkboxes começam marcados.
- O texto de consequência muda ao desmarcar "não baixar de novo".
- O evento de confirmação carrega `{ keepData, block }` coerentes com os checkboxes.

`tests/smoke/downloads.spec.ts`:

- Digitar na busca reduz as linhas da tabela.
- Select-all marca só as linhas visíveis e exibe a barra de ações.
- Trocar o filtro poda a seleção.
- O diálogo de exclusão abre e dispara `DELETE /api/v1/torrents/<hash>?keep_data=false&block=true`.

`tests/smoke/anime-detail.spec.ts` (estende o existente):

- Com torrent ativo mockado para o anime, a linha do episódio mostra a barra de progresso.

## Documentação

- `docs/agents/architecture.md`: rota `DELETE /torrents/{hash}` na tabela de endpoints; arquivos novos (`torrentFilters.ts`, `torrentsByEpisode.ts`, `DownloadsToolbar.svelte`, `TorrentDeleteDialog.svelte`); campo `episode_hash`.
- `docs/agents/decisions.md`: entrada nova sobre a semântica do delete — por que o default é apagar+bloquear, por que "excluir sem bloquear" é um rebaixar-do-zero e não um cancelar, e por que `keep_data` não separa biblioteca de cópia semeada (mesmo inode).
- Swagger: `swag init -g src/cmd/daemon/main.go -o docs/swagger`.
- `docs/agents/conventions.md`: sem mudança.

## Ordem de implementação sugerida

1. `FakeBackend.RemovedKeepData` (destrava os testes do resto).
2. `keepData` em `removeEpisodesAndLinks` + `RemoveTorrentWithEpisodes` + testes de daemon.
3. `DELETE /torrents/{hash}` + testes de API + Swagger.
4. `episode_hash` em `AnimeEpisodeInfo` + teste.
5. `torrentFilters.ts` + `torrentsByEpisode.ts` + unit tests (puros, independentes da UI).
6. `ConfirmDialog` com slot, `TorrentDeleteDialog`, `DownloadsToolbar`.
7. `Downloads.svelte` reescrito sobre os itens 5 e 6.
8. Status inline em `AnimeDetail.svelte`.
9. i18n, smoke tests, docs.

Os passos 2–4 são backend puro e os 5–8 frontend puro; a única dependência cruzada é o item 4 alimentando o 8.
