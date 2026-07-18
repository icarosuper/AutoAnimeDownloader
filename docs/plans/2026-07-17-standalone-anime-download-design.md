# Standalone Anime Download — Design

Baixar animes **avulsos**: conteúdo obtido por fora do AniList, escolhido manualmente pelo usuário. Nova tela lista os avulsos baixados; um botão `+` abre um fluxo de busca no Nyaa onde o usuário digita uma pesquisa, vê os torrents crus com detalhes (seeders/tamanho/fansub/resolução) e escolhe qual baixar.

O ponto central: **avulsos não passam pelo AniList nem pelo loop de verificação**. Todo o resto do app é chaveado por AniList MediaList ID (`episodes.json`, `/animes`, loop do daemon). Avulsos são um subsistema paralelo e independente — magnet → qBittorrent → registro próprio.

## Approach

**Busca crua no Nyaa, usuário escolhe o torrent.** Não usar AniList + auto-pick. Toda a máquina frágil de casar título→torrent (season/part/offset, jaccard, retries — ver `troubleshooting-downloads.md`) existe porque o daemon precisa *adivinhar* sozinho. Num avulso o usuário escolhe, então essa máquina não é necessária e nem desejável — pior, o conteúdo pode nem estar no AniList. Rejeitado: retornar resultados do AniList e auto-selecionar torrent — reintroduz a maior fonte de bug do app para ganho zero neste caso de uso.

**Persistência própria (`manual_downloads.json`).** A fonte da verdade da tabela é um arquivo novo, não o qBittorrent. Guarda nome amigável (a pesquisa do usuário), magnet, hash, nome cru do torrent e data. Sobrevive a apagar/renomear o torrent no qBit e dá um nome legível na tabela. Rejeitado: listar direto da categoria do qBittorrent — nome feio (`[SubsPlease]...1080p`), some se o torrent for removido, e mistura com os downloads automáticos.

**Reuso da primitiva de download existente.** `TorrentService.DownloadTorrentWithOptions(magnet, animeName, epName, isCompleted, skipSubfolder)` já é agnóstica ao AniList (é o que o "colar magnet manual" usa hoje). Nada novo na camada de torrent além de expor busca crua.

## Backend

### `nyaa` — busca crua

- Extrair o `parseRow` (hoje duplicado inline em `ScrapNyaa`, `ScrapNyaaForMultipleEpisodes`, `ScrapNyaaForBatch`, `ScrapNyaaForMovie`) para um helper compartilhado `parseTorrentRow(s *goquery.Selection) (TorrentResult, bool)`. Dedup real, não código novo perdido.
- `SearchNyaaRaw(query string) ([]TorrentResult, error)`: `fetchNyaaPage` (2 páginas, como os demais) → `parseTorrentRow` por linha → **sem** `titleMatchesQuery`, **sem** filtro de season/part. Ordena por `SortTorrentResults` (uncensored → resolução → fansub → health → tamanho — o usuário ainda vê a lista completa e decide).

### `files` — persistência

Espelha o padrão de `AnimeSettings` (`loadAll*`/`saveAll*` + `mu` lock, JSON indentado):

- Struct `ManualDownload`: `Name string` (a query), `TorrentName string` (nome cru p/ detalhe), `Magnet string`, `Hash string`, `Date time.Time`.
- Novo campo `manualDownloadsPath` em `FileManager`; wire em `NewDefaultFileManager`/`NewManager` + const `manualDownloadsFileName = "manual_downloads"` (sem extensão, como os demais state files).
- `LoadManualDownloads() ([]ManualDownload, error)` — arquivo ausente → slice vazia, sem erro.
- `AddManualDownload(md ManualDownload) error` — append + save.
- `DeleteManualDownload(hash string) error` — remove por hash + save.
- Adicionar os 3 métodos a `FileManagerInterface` (daemon **e** api — são declarações separadas com a mesma forma).

### `daemon` — orquestração do avulso

`ManualDownloadStandalone(name, magnet string, torrent nyaa.TorrentResult, configs, fm, jobQueue) (files.ManualDownload, error)` em `manual_download.go`:

1. `ts.DownloadTorrentWithOptions(magnet, name, name, false, skipSubfolder)` — `skipSubfolder=true` quando batch/movie (`torrent.IsBatch` ou `torrent.Episode == nil`), como o loop faz.
2. hash vazio → erro.
3. **Jellyfin rename (reuso condicional):** só quando o torrent escolhido é episódio único (`!IsBatch && Episode != nil`) e `RenameFilesForJellyfin` ligado → `jobQueue.EnqueueRenameFile(hash, name, *torrent.Episode)`. Batch/movie **não** renomeia — idêntico ao comportamento do loop automático, que só renomeia downloads de episódio único. (Sem `move_to_completed`, sem webhook nesta v1.)
4. `fm.AddManualDownload(...)` e retorna.

### `api` — endpoints

| Método | Endpoint | Handler | Arquivo |
|--------|----------|---------|---------|
| `GET` | `/api/v1/nyaa/search?q=` | `handleNyaaSearch` | `endpoint_nyaa_search.go` |
| `GET` | `/api/v1/standalone` | `handleStandaloneList` | `endpoint_standalone.go` |
| `POST` | `/api/v1/standalone` | `handleStandaloneAdd` | `endpoint_standalone.go` |
| `DELETE` | `/api/v1/standalone/{hash}` | `handleStandaloneDelete` | `endpoint_standalone.go` |

- `handleNyaaSearch`: `q` vazio → 400. Chama `nyaa.SearchNyaaRaw(q)`, retorna `[]TorrentResult`.
- `handleStandaloneAdd`: body `{name, magnet, torrent}` (o `TorrentResult` escolhido, p/ decidir skipSubfolder/rename sem re-scrape). Chama `daemon.ManualDownloadStandalone`.
- `handleStandaloneDelete`: `fm.DeleteManualDownload(hash)` + `ts.DeleteTorrents([hash])` (remove torrent + arquivos). Segue o padrão dos `endpoint_episode_actions.go`.

## Frontend

Nova rota no hash router (`Layout.svelte` ganha item de nav "Avulsos"):

- `routes/Standalone.svelte` (`#/standalone`): tabela dos avulsos (`GET /standalone`) com nome/data/tamanho e ação 🗑 (`ConfirmDialog` → `DELETE`). Botão `+` → `#/standalone/add`.
- `routes/StandaloneAdd.svelte` (`#/standalone/add`): input de pesquisa (`Input`) → `GET /nyaa/search?q=` → tabela de torrents (nome, seeders/leechers, tamanho, fansub, resolução, badge batch/movie) → botão "Baixar" por linha → `POST /standalone` → toast + volta pra `#/standalone`.
- `client.ts`: `searchNyaa(q)`, `listStandalone()`, `addStandalone(body)`, `deleteStandalone(hash)` + tipos `TorrentResult`/`ManualDownload`. Erros via `toasts.add` (padrão atual).

Reuso: `Input`, `ConfirmDialog`, `Toasts`, `Loading`, `ErrorMessage`.

## Decisões / limitações conhecidas

- **Avulsos são invisíveis ao loop do daemon.** Não entram em `episodes.json`, não são verificados, não têm redownload automático nem delete por episódio assistido. É intencional — são "baixe e esqueça". → registrar em `decisions.md`.
- **Jellyfin rename só para episódio único.** Batch/movie não renomeia (não há número de episódio confiável) — mesmo critério do loop automático. → `decisions.md`.
- **Sem `move_to_completed` / notificações na v1.** YAGNI; adicionar se o usuário pedir.
- **1 torrent escolhido = 1 linha.** Sem agrupamento por anime.

## Testing

- `nyaa`: `SearchNyaaRaw` contra o mock server — retorna resultados sem filtrar por título (inclui nomes que `titleMatchesQuery` reprovaria). Garantir que o refactor de `parseTorrentRow` não quebra os testes `nyaa_test.go` existentes.
- `files`: round-trip de `ManualDownloads` (add → load → delete) com `MockFileSystem`, incluindo arquivo ausente → slice vazia.
- `daemon`: `ManualDownloadStandalone` com mock de qBittorrent — hash vazio → erro; batch → `skipSubfolder=true` e sem enqueue de rename; episódio único com Jellyfin ligado → enqueue de rename.
- `api`: `handleNyaaSearch` com `q` vazio → 400; add/delete round-trip com FileManager mockado.

## Docs (após implementar)

- `architecture.md`: novos endpoints, `manual_downloads.json` na tabela de state files, `SearchNyaaRaw`/`parseTorrentRow` no mapa do `nyaa`, métodos `*ManualDownload*` no `files`, rotas `Standalone*` no front.
- `config.md`/`environment.md`: nada muda (sem novo campo de config nem env var).
- `decisions.md`: as duas primeiras limitações acima.
- Swagger: `swag init` (API mudou).
