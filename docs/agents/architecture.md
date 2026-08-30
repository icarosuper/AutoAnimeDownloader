# Architecture

> **Como usar este arquivo.** Ele responde *"qual arquivo eu abro e o que me morde lá"* — fluxo
> entre arquivos, invariante que dói quebrar, fato negativo, nome ambíguo e o ponteiro para a
> decisão. Ele **não** lista símbolo com uma linha de propósito: para "que funções existem neste
> pacote", `grep '^func ' <arquivo>.go` responde de graça e sempre atualizado. Leia o mapa de
> componentes e o fluxo, depois pule direto para a seção do pacote que você vai tocar.

## Components

```
src/cmd/daemon/      → Daemon binary entry point (embeds frontend build, starts API)
src/cmd/cli/         → CLI binary for managing the daemon via REST API
src/internal/
  api/               → HTTP server, REST handlers, WebSocket, middleware, Swagger docs
  daemon/            → Verification loop: Anilist → Nyaa → embedded torrent client → track episodes
  files/             → Config, episode tracking (JSON files), and library hardlinking (Librarian)
  anilist/           → GraphQL client for Anilist API
  nyaa/              → HTML scraper for Nyaa torrent site
  torrents/          → Embedded BitTorrent client (github.com/cenkalti/rain/v2) behind a TorrentBackend interface
  frontend/          → Svelte 5 + Vite + Tailwind 3 + daisyUI 4 web UI (compiled to Go embed)
                       (o par de versões é obrigatório — ver decisão 33)
  notifications/     → Webhook template interpolation and HTTP firing. Called by daemon on NewEpisode/DownloadFailed; by job queue on DownloadCompleted.
  logger/            → zerolog-based structured logger (console + rotating file)
  tray/              → System tray icon (fyne/systray)
  version/           → Build-time version injection via ldflags
src/tests/
  unit/              → Unit tests with mocks
  integration/       → Docker-based end-to-end tests
  mocks/             → Mock servers for Anilist and Nyaa
```

The daemon ships as a single self-contained binary — the BitTorrent client is embedded (`github.com/cenkalti/rain/v2`), so there is no external qBittorrent to install, configure, or connect to.

## Key Data Flow

1. **Verification loop** (periodic, configurable interval):
   - Fetch watch list from Anilist GraphQL API
   - Per anime: scrape Nyaa for matching torrents (filter by resolution/fansub)
   - Add new episodes to the embedded torrent client (`TorrentBackend.Add`)
   - Record downloaded episodes in `episodes.json` — skip re-downloads
   - Torrents download to the derived download path (`Config.DownloadPath()`, `<completed_anime_path>/.torrents`) and keep **seeding** there; on completion an `organize` job hardlinks the video files into `completed_anime_path` (the Jellyfin library)

2. **Frontend embedding**: `bun run build` → `src/internal/frontend/dist/`, Go embeds via `//go:embed dist` in API server. Daemon serves SPA at `/`, proxies `/api/` to REST handlers.

3. **Real-time updates**: WebSocket at `/api/v1/ws` broadcasts daemon state changes. Svelte frontend subscribes for live status.

## Configuration & State Files

| File | Location (Linux) | Purpose |
|------|-----------------|---------|
| `config.json` | `~/.autoAnimeDownloader/` | User settings (Anilist usernames, paths, intervals) |
| `downloaded_episodes` | `~/.autoAnimeDownloader/` | Tracks downloaded episodes (JSONL, no extension) |
| `blocked_episodes` | `~/.autoAnimeDownloader/` | Episodes to skip (JSON array of `{anime_id, episode}`, no extension). O formato antigo (array de ids de nó da AniList) é descartado com aviso ao ser lido — ver decisions.md #52 |
| `anime_settings` | `~/.autoAnimeDownloader/` | Per-anime settings keyed by AniList **media** ID (JSON map, no extension) |
| `standalone_animes` | `~/.autoAnimeDownloader/` | Media IDs tracked **without** being in any AniList list (JSON array of IDs, no extension) |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |
| `pending_jobs.json` | `~/.autoAnimeDownloader/` | Persisted job queue (`organize` jobs) |
| `session.db` | `~/.autoAnimeDownloader/` | rain resume database (bbolt) — piece bitfields, kept **outside** the download path so it survives a library path change |
| `queue.json` | `~/.autoAnimeDownloader/` | Download queue state: `{"order": [...], "paused": [...]}`. Lives next to the resume database because it is torrent-client state, not user config. Missing/corrupted = rebuilt from `AddedAt`, never fatal — see decisions.md #41 |
| `download_root.id` | `~/.autoAnimeDownloader/` | Id of the download folder the session is bound to. Its twin, `.aad_root`, lives **inside** the download folder; the pair is how a moved/trashed/replaced folder is detected — see decisions.md #34 |

Windows uses `%APPDATA%\.autoAnimeDownloader\` for **all** the config/state files above (note the leading dot — same folder name as on Linux). See `configsFolder` in `files/filemanager.go` and `getJobsFilePath` / `getSessionDBPath` / `getPIDFilePath` in `cmd/daemon/main.go`. There is no dotless `%APPDATA%\AutoAnimeDownloader\` variant.

## On-Disk Layout

- **Download / seeding:** torrents live at `<Config.DownloadPath()>/<torrent-id>/...`, i.e. `<completed_anime_path>/.torrents/<torrent-id>/...` (rain's `DataDir` with `DataDirIncludesTorrentID`). Files are **never renamed here** — renaming would break seeding. Torrents keep seeding after completion. The download directory is **derived**, not user-configured — see decisions.md #31.
- **Library (Jellyfin):** when a torrent completes, its video files are **hardlinked** into `<completed_anime_path>/<AnimeName>/` — one folder **per AniList entry**, season/cour marker kept (`sanitizeName`, decisions.md #45), plus a `tvshow.nfo` carrying the AniList id. With `RenameFilesForJellyfin`, every file gets the Jellyfin name `"Anime Name - E05.mkv"` — a single episode from its record, a batch file from the episode number parsed out of its own filename (`nyaa.ExtractEpisodeNumber`); files with no readable number (NCOP/NCED, extras, movies) and two files of one pack landing on the same number keep the raw filename. The hardlink shares bytes with the seeded copy, so no space is duplicated.
- **Same volume, by construction:** the download directory lives inside `completed_anime_path`, so the old cross-filesystem failure mode is now structurally impossible. `Librarian.ProbePath(completedPath)` still validates that the filesystem supports hardlinks at all (exFAT/FAT32/some SMB shares don't) — it runs on config save and on every verification pass (decisions.md #26).
- **Deletion** frees space by removing **both** links: the library hardlink (`Librarian.RemoveFromLibrary`) and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). A batch torrent shared by multiple episodes is only removed once **all** its episodes are deleted **and** every episode the pack covers is claimed by some entry in the group (`canRemoveTorrent` / `hasUnclaimedContent`, decisions.md #74). While it is held back, its records are kept too (`forgettableKeys`) — they carry the declared range. The manual delete path passes `force` and bypasses that guard.
- **No `save_path` migration.** `daemon.MigrateSavePath` used to move a legacy `save_path`'s torrent folders into the derived download path at boot and at the top of every pass; it was removed along with the `Config.SavePath` field. An old `config.json` still carrying the key loads (JSON ignores unknown fields) but its data is **not** moved — moving it is a manual pre-upgrade step. See decisions.md #31.

## API

Swagger UI at `http://localhost:8091/swagger/` when daemon running.

Key endpoints:

| Method | Endpoint | Handler func | File |
|--------|----------|-------------|------|
| `GET` | `/api/v1/status` | `handleStatus` | `endpoint_status.go` — o limiar de `disk_low` mora **só** no servidor. O estado da AniList é escrito por **qualquer** chamada (passe ou poll do frontend), porque todas passam por `sendAnilistRequest` — decisions.md #65/#66 |
| `GET` | `/api/v1/last-check` | `handleLastCheck` | `endpoint_last_check.go` — só memória. `problems` (devia ter baixado e não baixou) e `limits` (a config funcionando como configurada). Quando há `pass_error` as duas listas vêm vazias, porque `SetLastCheckError` limpa o relatório (decisions.md #82). Download manual fica fora: aquele caminho devolve o erro na própria resposta HTTP |
| `GET/PUT` | `/api/v1/config` | `handleConfig` | `endpoint_config.go` |
| `GET` | `/api/v1/config/priorities/defaults` | `handlePriorityDefaults` | `endpoint_priorities.go` |
| `GET` | `/api/v1/animes` | `handleAnimes` | `endpoint_animes.go` — `next_airing_at` é **timestamp**, não contagem regressiva: a resposta é cacheada 60s e um contador pré-computado congelaria. Busca por `alt_names` é client-side (`lib/utils/status.ts`) |
| `GET` | `/api/v1/anilist/search?q=&include_unreleased=` | `handleAniListSearch` | `endpoint_anilist_search.go` — termo com menos de 3 chars devolve lista vazia, **não** 400. `block_reason` por resultado (`""` = adicionável) |
| `POST` | `/api/v1/standalone-animes` | `handleStandaloneAnimeAdd` | `endpoint_standalone_animes.go` — body `{"media_id": 21}`, answers `{"added": N}`; 409 `LIBRARY_NOT_CONFIGURED` / `ALREADY_BLACKLISTED` / `ALREADY_STANDALONE` / `ALREADY_TRACKED` / `ALREADY_DOWNLOADED` |
| `DELETE` | `/api/v1/standalone-animes/{id}?delete_episodes=<bool>` | `handleStandaloneAnimeRemove` | `endpoint_standalone_animes.go` — without `delete_episodes` the episodes stay and are marked `ManuallyManaged` |
| `GET/PUT` | `/api/v1/animes/{id}/settings` | `handleAnimeSettings` | `endpoint_anime_settings.go` |
| `GET` | `/api/v1/animes/{id}/episodes` | `handleAnimeEpisodes` | `endpoint_anime_episodes.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/download` | `handleDownloadEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/redownload` | `handleRedownloadEpisode` | `endpoint_episode_actions.go` |
| `DELETE` | `/api/v1/animes/{id}/episodes/{episodeNumber}` | `handleDeleteEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/release` | `handleReleaseEpisode` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/episodes/{episodeNumber}/replace` | `handleReplaceEpisodeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/animes/{id}/replace` | `handleReplaceAnimeWithMagnet` | `endpoint_episode_actions.go` |
| `POST` | `/api/v1/check` | `handleCheck` | `endpoint_check.go` |
| `POST` | `/api/v1/daemon/start` | `handleDaemonStart` | `endpoint_daemon_start.go` |
| `POST` | `/api/v1/daemon/stop` | `handleDaemonStop` | `endpoint_daemon_stop.go` |
| `GET` | `/api/v1/logs` | `handleLogs` | `endpoint_logs.go` |
| `POST` | `/api/v1/notifications/webhooks/{name}/test` | `handleNotificationWebhookTest` | `endpoint_notifications.go` |
| `GET` | `/api/v1/torrents` | `handleTorrents` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/pause` | `handleTorrentPause` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/resume` | `handleTorrentResume` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/announce` | `handleTorrentAnnounce` | `endpoint_torrents.go` |
| `POST` | `/api/v1/torrents/{hash}/prioritize` | `handleTorrentPrioritize` | `endpoint_torrents.go` |
| `GET` | `/api/v1/torrents/{hash}/files` | `handleTorrentFiles` | `endpoint_torrents.go` — files inside the torrent, in metadata order |
| `POST` | `/api/v1/torrents/prioritize` | `handleTorrentsPrioritize` | `endpoint_torrents.go` — batch, body `{"hashes":[...]}`, applied in the order received; unknown/completed hashes ignored |
| `DELETE` | `/api/v1/torrents/{hash}?keep_data=<bool>&block=<bool>` | `handleTorrentDelete` | `endpoint_torrents.go` |
| `WS` | `/api/v1/ws` | `handleWebSocket` | `websocket.go` |

## Version Injection

Version injected at build time via ldflags:
```bash
go build -ldflags "-X AutoAnimeDownloader/src/internal/version.Version=v1.2.0" ./src/cmd/daemon
```
Defaults to `"dev"` if not injected.

## Debug Mode

`cmd/daemon/main.go` accepts `--debug-anime <anilistId>`: runs `daemon.RunAnimeDebug` (see `daemon/debug.go`) and exits, skipping the PID file / API server / tray / loop entirely. See [Commands](commands.md) and [Troubleshooting Downloads](troubleshooting-downloads.md).

`RunAnimeDebug` mirrors `processAnimeEpisodes`: episode selection (`anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`), the pack search/selection (`partitionSearchResults` + `pickBatches`) that lifts the per-anime limit when it resolves, and the per-episode `searchSingleEpisode` fallback when no batch/movie/single magnet was found — so the debug pass reports the pipeline the daemon actually runs (see [decisions.md #58](decisions.md#58-runanimedebug-espelha-processanimeepisodes-enumeração-resolução-de-pack-e-fallback-single)).

`scripts/robustness-animes.txt` (curated media IDs, one per line) + `scripts/debug-batch.sh` (`make debug-batch`) run that pass over the whole list and write a triaged `.debug_batch/report.md`.

---

## Mapa de arquivos — o que mora onde e o que morde

### `src/internal/daemon/` — o passe

**Não existe `daemon.go`.** O orquestrador está espalhado: `loop.go` (a goroutine), `verification.go`
(o passe), `episodes.go` (seleção e remoção por episódio), `search.go` (busca no Nyaa + filtros),
`coverage.go` (posse por cobertura), `helpers.go` (guardas de config/disco), `manual_download.go`
(downloads vindos da API) e `standalone.go`.

**Um passe por vez**: `AnimeVerification` pega `verificationMu.TryLock()` e volta na hora se outro já
está rodando (decisions.md #67).

**Ordem de prioridade do download** (em `processAnimeEpisodes`) — é o fluxo central do projeto:

0. **Posse por cobertura** (`adoptCoveredEpisodes`, antes de qualquer busca): episódio já contido num
   pack em disco, em **qualquer** `anime_id` da mesma série, vira registro apontando para aquele hash
   — sem download e sem Nyaa (decisions.md #78)
1. **Filme** → `resolveMovie` → `skipSubfolder=true`
2. **Packs**, resolvidos antes do loop de episódios, cobrindo a janela a partir do primeiro pendente
   → `partitionSearchResults` + `pickBatches`/`assignBatches`. Elegibilidade vem do **resultado** da
   busca (tamanho, seeders, faixa coberta), nunca do metadado do anime
3. **Episódio avulso**, por episódio ainda descoberto

Os três passam pelo piso de `min_seeders` (`filterSearchResults`).

**Invariantes que doem quebrar:**

- `DownloadStandaloneAnime` **nunca** pode chamar `handleSavedEpisodes`. Com os episódios de um único
  anime em mãos e `delete_watched_episodes` ligado, `identifyEpisodesNotInWatching` apagaria o resto
  da biblioteca ([decisions.md #49](decisions.md#49-anime-avulso-acompanhado-pela-lista-é-o-snapshot-que-o-daemon-processa-e-downloadstandaloneanime-nunca-chama-handlesavedepisodes))
- `max_episodes_per_anime` só é levantado **depois** que um pack foi realmente escolhido; sem pack, a
  seleção limitada vale e os episódios mais antigos são os que ficam. `selectEpisodes` roda duas vezes
  no passe, e `dropAdopted` tem de rodar nos **dois** pontos — a função é pura, a segunda passada
  traria os adotados de volta
- `firstEpisodeToConsider` recua até o menor episódio já salvo. Sem o recuo, um salvo abaixo do
  progresso não é "checado" e a poda o apagaria ignorando `watched_episodes_to_keep`
- Faixa de pack que nem o nome nem a lista de arquivos resolveram é gravada como **zero**, não como
  `1..total`: as duas fontes falharam e "não sei" é o único registro honesto (decisions.md #84)
- O sentinela de "faixa desconhecida" é o **fim** (`BatchEnd`). Começo 0 ou negativo é faixa legítima
  ("00 ~ 12", ou pack de season gravado sob um cour posterior)
- `checkDiskSpace` barra download, **nunca** o passe de verificação. Erro de `statfs` não bloqueia
- O teto de tamanho de pack é aplicado **duas vezes**: na busca do Nyaa e de novo no filtro do daemon
  (decisions.md #80). Mexer só num dos dois não muda o comportamento

**Custo de rede.** `packSet` resolve a faixa de um pack **sob demanda** — do nome quando o nome traz,
senão da página de detalhe — e memoriza **inclusive a falha**; sem isso `assignBatches`, que varre os
packs uma vez por episódio, viraria uma requisição por episódio. Teto de `maxPackDetailFetches` (3)
páginas de detalhe por anime por passe. Ver [decisions.md #84](decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo).

**Eixos de numeração.** `packAxis` são as hipóteses de quanto somar ao número **local** da entrada:
relativa (0, sempre primeira), contínua pela season (`ComputeEpisodeOffset` — o gate exige `part >= 2`
e PREQUEL `TV`/`TV_SHORT`, decisions.md #9) e absoluta pela série
(`anilist.Series.Offset`). O zero-value = só a relativa, o comportamento antigo (decisions.md #79).
Entre as hipóteses que cobrem o episódio ganha a que faz o pack **terminar** no último episódio da
entrada — desempate por contagem, ver `sources.md`.

**`coverage.go`** — a unidade de posse deixa de ser `files.EpisodeKey{AnimeID, Episode}` e passa a ser
a faixa que o torrent cobre no **eixo absoluto da série** (decisions.md #77). É o que impede o cour 2
de rebaixar do Nyaa o pack de season que já está em disco sob o cour 1. Quem semeia o índice é o
daemon (`resolveSeriesIndex`): os ids do passe **mais** os `anime_id` de `episodes.json` — o cour
anterior já `COMPLETED` saiu do universo do passe, e é o offset dele que converte a faixa
(decisions.md #78). Erro ao resolver é logado, não aborta.

**Reconciliação** (`reconcileLibrary`, `verification.go`) — no boot e periodicamente, enfileira um job
`organize` para todo torrent concluído cujo episódio ainda não está na biblioteca. O critério é
`LibraryPaths` **vazio**, não o hardlink faltando em disco (decisions.md #29); `clearLibraryPathsAfterRootSwap`
é a única exceção, quando `Ensure` reporta `RootSwapped` (decisions.md #34).

**`webui.go`** — o que o processo chama de "porta" é na verdade o **Addr** do `http.Server` (`":8091"`).
`WebUIURL` tira os dois-pontos: interpolar o valor cru produz `http://localhost::8091`, host inválido.
A conversão já esteve escrita em três lugares e a cópia mais nova esquecia o strip. `WaitForListener`
existe porque `apiServer.Start()` roda em goroutine e loga "API server started" **antes** do socket
estar ligado — quem abre o browser logo depois perde a corrida e leva `ERR_CONNECTION_REFUSED`.

**`jobs.go`** — fila de jobs persistida, para desacoplar o hardlink-na-biblioteca do loop de
verificação. Um único job idempotente, `organize` (substituiu os antigos `rename_file` /
`move_to_completed` / `notify_on_complete`, que eram baseados em poll). Tick de 5s, backoff
`30s * 2^(n-1)` até 10min, máximo 20 tentativas, persistido em `pending_jobs.json`. Idempotência:
episódio com `LibraryPaths` já preenchido conta como pronto — sem re-link e sem re-disparar o webhook —
então evento de conclusão e passe de reconciliação podem enfileirar os dois sem medo. Criada no
`main.go` antes de `NewServer`, ligada ao backend/librarian por `SetOrchestration`.

**`migration.go`** — roda no boot **e** no topo de todo passe. `verification.go` recarrega a config
logo depois de chamá-la, porque a migração persiste uma config mudada que o resto do passe precisa
enxergar. A migração de `save_path` que morava aqui foi removida junto com o campo (decisions.md #31).

**`report.go`** — os campos de detalhe são achatados com `omitempty`, **nunca** um `map[string]any`:
aquilo não gera Swagger nem tipo TS utilizável. A cascata de precedência dos três problemas de busca
está em decisions.md #81.

**`state.go`** — `SetLastCheckError` **limpa** `lastCheckReport`. Consequência: `SetLastCheckReport`
tem de ser chamado depois do `SetLastCheckError(nil)` do fim do passe (decisions.md #82).
`GetLastCheckReport` devolve **valor**, para o handler preencher `pass_error` sem escrever no objeto
compartilhado.

**`passerror.go`** — o backend manda **código**, o frontend monta a frase (mesma fronteira que
`lib/domain/checkIssue.ts`); antes disso o banner despejava `error.Error()` cru, às vezes um JSON da
AniList. O código viaja **dentro** do erro, embrulhado no sítio do aborto — não num parâmetro novo de
`SetLastCheckError`. Sítio novo que esqueça de embrulhar cai em `PassErrUnknown`, que tem frase
própria: degrada, não quebra.

**`debug.go`** — espelha `processAnimeEpisodes` (enumeração, resolução de pack, fallback single) para
que o passe de debug reporte o pipeline que a produção roda de verdade
([decisions.md #58](decisions.md#58-runanimedebug-espelha-processanimeepisodes-enumeração-resolução-de-pack-e-fallback-single)).
Sem backend de torrent e sem `episodes.json`; a saída vai para `.debug_<animeId>_<N>/` no cwd de quem
chamou, **não** em `~/.autoAnimeDownloader`.

### `src/internal/files/` — persistência

**Não existe `save_path`.** O diretório de download é derivado por `Config.DownloadPath()`
(`<completed_anime_path>/.torrents`), calculado a cada chamada (decisions.md #31).

**`EpisodeKey{AnimeID, Episode}` é a identidade de um episódio** em todo o app — arquivo de
episódios, bloqueados, rotas da API.

**Armadilha:** `SaveEpisodesToFile` só **acrescenta** os novos e **descarta em silêncio** update em id
existente (decision 27). Para reescrever um registro — por exemplo gravar `LibraryPaths` depois de
organizar — é `UpsertEpisodes`.

**`FileManagerInterface` é declarada duas vezes**, em `api/server.go` e `daemon/helpers.go`. Método novo
em `files.FileManager` → atualizar **as duas** (ver `conventions.md`).

**`mkv.go`** — `VideoCodec` lê o codec do **header** do arquivo, não do nome: `nyaa.extractCodec` lê do
nome do release e boa parte dos uploads nunca escreve isso (Erai-raws não escreve; muito BD rip também
não). Matroska é EBML (RFC 8794), e o codec fica num caminho curto e fixo **antes** dos Clusters
(`Segment → Tracks → TrackEntry → {TrackType, CodecID}`), então a leitura custa poucos KB e alguns
seeks, nunca o arquivo inteiro. `TrackType` é conferido porque um `.mkv` de anime carrega vídeo, áudio
e várias legendas — pegar o primeiro `TrackEntry` às cegas reportaria o codec do áudio. Os `CodecID`
são mapeados para o **mesmo vocabulário do `nyaa.extractCodec`** (`HEVC`, `H.264`, `AV1`), senão o
painel e a tela de prioridades nomeariam codecs de formas diferentes. Devolve `""` — **nunca** erro —
para todo caso sem resposta. Só Matroska: MP4 teria estrutura análoga mas exigiria um segundo parser, e
anime na prática é `.mkv`.

**`librarian.go`** — hardlink dos vídeos concluídos para `<completed_anime_path>/<AnimeName>/`, uma
pasta **por entrada AniList**, com marcador de season/cour preservado (decisions.md #45) e um
`tvshow.nfo` carregando o id da AniList (para o plugin do Jellyfin casar por id, não por nome de
pasta). No rename Jellyfin, o número de um arquivo de pack sai do nome do próprio arquivo menos
`packEpisodeOffset` — pack de season ≥ 2 numerado continuamente vira a numeração da entrada
(decisions.md #68). Sem número legível (NCOP/NCED, extras, filmes) ou em colisão de nome dentro do
pack, mantém o nome cru; dois arquivos com o mesmo basename (subpasta por season, achatada) usam o
caminho relativo em vez de um sobrescrever o outro. `Organize` é idempotente. `ProbePath` valida que o
filesystem suporta hardlink (exFAT/FAT32/alguns SMB não suportam) e roda no save da config e em todo
passe (decisions.md #26, #31). `BackfillShowNFOs` **não** está na interface `Librarian` — o `main.go`
segura o `*organizer` concreto — e só roda quando `MigrateAnimeIDsToMedia` teve sucesso.

**`parser.go`** — JSONL, um objeto por linha. O formato de texto legado (`id:hash:name`) foi removido
junto com `EpisodeID`: sem anime id nem número de episódio, aquele registro não tem chave
(decisions.md #52).

**`filesystem.go`** — a interface `FileSystem` existe para teste (`MockFileSystem`); inclui `Link`, que
é o que o `Librarian` usa.

### `src/internal/api/` — HTTP

Padrão de handler, envelope de resposta e checklist de endpoint novo estão em
[conventions.md](conventions.md), não aqui.

**`client.go` é o cliente HTTP _para_ a API do daemon, usado pelo binário da CLI** (`cmd/cli`) — o
daemon não o usa. Confundir os dois leva a procurar lógica de negócio nele.

**`endpoint_animes.go`** — os dois filtros (`DownloadStatuses` server-side, `DownloadMediaStatuses`
client-side) só barram entrada **nova**: anime que já tem episódio baixado nunca sai da listagem, não
importa o status atual. Para esses órfãos, `refreshOrphanAnimes` recarrega capa/progresso/blacklist em
no máximo duas queries em lote; falha só deixa os campos vindos da AniList em branco, em vez de sumir
com o anime ou derrubar o request. `countPendingEpisodes` conta por **número de episódio**, não
`released - downloaded`, porque assistido pode continuar salvo (`watched_episodes_to_keep`).

**`endpoint_anime_episodes.go`** — `AnimeEpisodeInfo` **não carrega nome de episódio**: a AniList não
expõe título por episódio (`AiringNode` só tem número e horário) — decisions.md #52. `animeId` é o id
de **media**, o mesmo do link `anilist.co/anime/{id}`; não existe um campo `anilistId` separado.

**Vocabulário — nunca chame anime avulso de "manual".** "Avulso"/standalone = acompanhado sem estar em
lista nenhuma da AniList. "Manual" já significa outra coisa: o usuário mexeu naquele episódio à mão
(`ManuallyManaged`, `ManualDownloadEpisode`). Os arquivos são `standalone.go`, `standalone_guard.go`,
`endpoint_standalone_animes.go` e `endpoint_anilist_search.go`. O `standaloneGuard` é **uma regra de
bloqueio com dois consumidores** (o `POST` e o handler de busca), para que "o front não deixa clicar" e
"o back devolve erro" concordem por construção.

**`endpoint_torrents.go`** — o `path` de um arquivo é o caminho **cru** dos metadados e nunca é
reescrito para "Ep NN": o objetivo do painel é justamente ler fansub/resolução/codec dali. `Progress`
cai para a razão de peças (`PiecesHave/PiecesTotal`) quando `BytesCompleted` lê 0 com total não-zero.
O `DELETE` está registrado no **mesmo** padrão de mux que pause/resume/announce — um padrão Go 1.22+
sem prefixo de método casa todo verbo — então a checagem de método dentro do handler é o que separa os
dois.

### `src/internal/anilist/`

**`EpisodeList` é a única fonte de "quais episódios existem".** Ela sintetiza a lista, usando o nó real
do `airingSchedule` quando existe, porque a AniList guarda só uma janela da agenda. `AiringNode.ID` é
o id do nó de agenda e **não identifica** um episódio.

**Identidade: `Media.Id`, nunca `MediaList.Id`.** O mesmo anime em várias contas aparece uma vez por
conta, cada uma com seu `MediaList.Id`/`Progress`/`Status`, mas o mesmo `Media`. `Media.Id` é o que
`episodes.json`, `anime_settings`, as rotas `/animes/{id}/*` e o link da AniList usam
([decisions.md #43](decisions.md#43-a-identidade-de-um-anime-é-mediaid-status-é-uma-pergunta-por-conta-download--or-deleção--and)).
Instalações anteriores são convertidas por `MigrateAnimeIDsToMedia`, com o passe abortando até
conseguir.

**`DedupeByMedia` colapsa as entradas por `Media.Id` e mantém o **menor** `Progress`** — um episódio só
é "assistido"/deletável quando todas as contas o viram. O `Status` da entrada sobrevivente **não pode
ser lido**: ele é de uma conta arbitrária. Status é por conta:

- **Download — OR**: basta uma conta ter o anime num status de `DownloadStatuses`
- **Deleção — AND** (`allAccountsAgreeOnDelete`): toda conta que *tem* o anime precisa tê-lo em algum
  status de `DeleteStatuses` (não precisam ser o mesmo). Conta que não acompanha não vota; conta em
  status neutro (`PLANNING`) **veta**, e conta cuja lista falhou também

**Dois filtros de status, nomes parecidos, coisas diferentes** {#media-status-filter}:
`Config.DownloadStatuses` filtra por status de **lista** (`MediaListStatus` — a relação do usuário com
o anime, ex. `CURRENT`) e é aplicado server-side pela AniList (`status_in`). `Config.DownloadMediaStatuses`
filtra por status da **mídia** (`MediaStatus` — o estado de exibição do anime, ex. `RELEASING`); a
AniList não aceita esse no mesmo filtro, então os dois consumidores o aplicam client-side via
`anilist.MediaStatusAllowed`: `searchAnilist` (`daemon/verification.go`, pipeline de download, logo
após o fetch por conta e antes do dedup) e `fetchAniListEntries` (`api/endpoint_animes.go`, listagem
do frontend). **Nenhum dos dois filtros esconde anime que já tem episódio baixado**, qualquer que seja
o status atual — ver `endpoint_animes.go` acima.

**Toda query declara uma `Priority`, e não há valor padrão de propósito**: query nova não compila sem
alguém decidir a criticidade. O gate de orçamento (`health.go`) recusa `PriorityDisposable` com
`ErrBudgetLow` quando a cota da AniList está baixa — nada sai, e a saúde do pacote não muda
(decisions.md #72). O limite é 30 req/min por IP.

**Saúde é exposta como código, nunca frase pronta** — o daemon não sabe o locale do navegador (mesma
razão de `daemon/passerror.go`). Ela é escrita por **toda** resposta que passa por
`sendAnilistRequest`, e a AniList devolve 200 com erro no corpo, então o envelope também é lido
(decisions.md #65/#66).

**Caches** (todos em memória, `ttlCache[T]`): `GetFrontendAnimeList` 60s por `username+statuses` — o
poll de `/api/v1/animes`, 30s por aba aberta, é o que estourava a cota da AniList (decisions.md #46);
`customLists` 5min (decisions.md #11); `seriesCache` 24h; `searchCache` 10min por
`termo-minúsculo+includeUnreleased` — **o único com chave de texto livre**, por isso é o único com
teto (`searchCacheMaxEntries`, descarte total, não LRU) e o único que `ttlCache.size()` serve.

**`series.go` — o eixo absoluto da série.** A numeração contínua que atravessa cour e part, que boa
parte dos grupos usa nos nomes de arquivo. A AniList **não tem id de franquia**: a cadeia de `PREQUEL`
é a única fonte (decisions.md #71), e como uma query só expande 2 níveis de `relations`, resolver a
cadeia é um BFS, não uma leitura (decisions.md #77). Cada media buscada ensina **dois** elos (níveis 0
e 1, os únicos em posição autoritativa); o avô, cujo `relations` veio cortado, volta para a fronteira
em vez de virar raiz. A semente vem pronta de quem chama porque **`anilist` não pode importar
`files`** — e o conjunto certo só o daemon conhece.

### `src/internal/nyaa/`

**`ScrapNyaaForAnime` devolve packs e episódios na _mesma_ lista** — quem separa é
`daemon.partitionSearchResults`. É a busca única por trás da resolução de pack + episódio.

**Paginação adaptativa** (`fetchSearchPages`, compartilhada pelas três buscas): página 1, e só desce
mais enquanto os candidatos aceitos estiverem abaixo de `enoughCandidates` (3) e a página tiver linhas,
até `ActiveMaxSearchPages()`. Erro só é erro na página 1.

**Busca de episódio descarta batch e filme, e o guarda certo é `hasMovieMarker`, não `isMovie`**
(decisions.md #53) — são duas funções parecidas e só uma é segura aqui. Série longa (>100 episódios)
ganha **também** a variante com zero-padding de 3 dígitos, nunca no lugar da simples, e quem libera é
`LastAiredEpisode` (decisions.md #56).

**`ExtractPart` devolve `nil` quando o nome declara duas parts** ("(Part 1 + Part 2)", "(Season 4 Part
03+04)"): ele cobre as duas metades, e a primeira que casasse mentiria.

**A faixa lida do nome é o primeiro casamento _plausível_, não o primeiro casamento**
(decisions.md #83): rejeita tag de resolução (`[720-1080p]`) e faixa invertida (`"Euphonium 2 - 01 ~ 13"` → `2..1`).

**`PackFileRange`** lê a faixa que o pack realmente traz da lista de arquivos da página de detalhe,
na mesma régua do nome (decisions.md #84). **`IsVideoFile` mora aqui, e não em `files`**, porque os dois
leem a mesma lista de arquivos de um pack (o Librarian depois de baixar, `PackFileRange` antes) e
`files` já importa `nyaa` — o contrário seria ciclo.

**Ordenação** (`priorities.go`): o critério `health` compara a **faixa** (`healthTier`, pisos
`1/5/20/100/400` seeders), não o score cru (decisions.md #55). `SetPriorities` e
`SetMaxSearchPages`/`SetMaxBatchTorrentSizeGB` usam o mesmo padrão atomic+restore, empurrados por
`files.LoadConfigs` a cada load bem-sucedido.

A resolução é canonicalizada **no extrator**, e os presets de codec são reordenação feita no frontend
(decisions.md #70).

**Casamento de título** (`nyaa_match.go`): tokens técnicos são removidos, o nome é cortado no primeiro
marcador de season/episódio, e o limiar de Jaccard depende do tamanho da query — `0.8` para ≤3 tokens
(impede "SAO" de casar com "SAO Alternative Gun Gale Online"), `0.4` para ≥4 (fansub que embute as duas
traduções do título). Só se o nome truncado falhar é que cada título alternativo escrito **depois** do
marcador é tentado sozinho (decisions.md #75).

**`nyaa_regex.go`** é só regex pré-compilada em nível de pacote, para não recompilar dentro de loop por
torrent. Nada de lógica.

### `src/internal/torrents/` — cliente BitTorrent embutido

`github.com/cenkalti/rain/v2` atrás da interface `TorrentBackend`. Um seam, dois lados: produção usa
`SessionManager`/`Session`; teste usa o `FakeBackend` em memória.

**Ordem de lock: `queue.mu` → `SessionManager.mu`, nunca o contrário.** `queue.enforce` segura
`queue.mu` enquanto chama `list`/`pause`/`resume`, então todo método do `SessionManager` solta o
próprio lock **antes** de mexer na fila. Corolário: `PrioritizeAll` **não pode** chamar `Get`/`List` —
os dois passam por `markQueued`, que pega `queue.mu`.

**`queueOps` são as delegações cruas, todas não exportadas.** Passar por `List`/`Pause`/`Resume`
re-entraria na fila: recursão infinita em pause/resume.

**`enforce` desiste quando `list()` devolve `nil`** — sem sessão. `nil` **não** é sessão vazia; tratar
os dois igual esvaziaria a fila persistida (decision 41).

**`completedFromStats` é deliberadamente independente do `Status`** (`Pieces.Have >= Pieces.Total`):
pausar um torrent terminado o tira de `Seeding`, e ler conclusão do status marcaria um completo como
incompleto.

**`queued` é o único slug que a rain nunca produz** — quem o escreve é `queue.markQueued`
(decision 41). `statusSlug` mapeia o enum da rain para os slugs estáveis da API, e **tem de ser
mantido em sincronia à mão com `src/lib/utils/torrentStatus.ts` no frontend**.

**Identidade da pasta de download** (`rootmarker.go`): `.aad_root` mora **dentro** da pasta de download
e viaja com ela; `download_root.id` mora na pasta de config, onde o usuário não move. O par é como uma
pasta movida/apagada/trocada é detectada (decisions.md #34). Sem id gravado (primeira execução ou
upgrade) **nunca** é swap. O swap é latched por `Ensure` e lido por `ConsumeRootSwap`, porque os
endpoints de download manual também chamam `Ensure`.

`session.db` (bbolt, resume) e `queue.json` ficam **fora** da pasta de download, para sobreviverem a uma
troca de caminho da biblioteca.

### `src/internal/notifications/`

`Notify` dispara em goroutines; `Flush` dispara o pendente **sincronamente** e só retorna quando as
requisições terminaram — é chamado no shutdown, onde goroutine seria o mesmo que não enviar.

`NewEpisode` só é disparado quando há **pelo menos um magnet para tentar**; episódio sem resultado de
busca vai direto para `DownloadFailed`/`ReasonNotFound`.

As notificações são agrupadas por janela, e o escape de JSON acontece **só no corpo e só quando o
preset declara `Content-Type: application/json`** (decisions.md #47).
Valores de header têm `\n`/`\r` removidos: o `net/http` rejeita a requisição inteira com eles, e o ntfy
usa header para título.

Variáveis de template (URL, headers e corpo): `{{title}}`, `{{message}}`, `{{anime_name}}`,
`{{episode}}`, `{{reason}}`, `{{count}}`. Numa janela com N > 1, `{{message}}` vira uma linha por item.

### Pacotes pequenos

- **`stringutil/`** — `RemoveSpecialCharacters`, os chars que quebram a query do Nyaa
- **`logger/`** — zerolog, console + `daemon.log` rotativo. `InitDebug` manda o trace para outro
  arquivo, para que `--debug-anime` nunca toque o `~/.autoAnimeDownloader`
- **`version/`** — `Version`, `"dev"` por padrão, sobrescrito por ldflags
- **`cli/`** — controle de ciclo de vida do daemon para o binário da CLI (`cmd/cli` é o único
  importador). Prefere o gerenciador de serviço da plataforma (systemd **user** unit no Linux, serviço
  no Windows) e cai para um PID file
- **`tray/`** — build-tagged: a implementação real precisa de CGO, então `tray_stub.go` mantém o build
  sem CGO compilando (a CI cobra isso em `build.yml`). No Linux o daemon só sobe a tray com `DISPLAY`
  não-vazio
- **`frontend/embed.go`** — `//go:embed dist/*`. `dist/` precisa existir **antes** do `go build`, e é
  por isso que o frontend sempre compila primeiro

### `src/internal/frontend/src/` — Svelte 5

O par de versões Svelte/Vite/Tailwind/daisyUI é obrigatório — decisions.md #33.

**Rotas** (hash router): `Status.svelte` (`#/`), `AddAnime.svelte` (`#/add`), `Downloads.svelte`
(`#/downloads`), `AnimeDetail.svelte` (`#/status/:id`), `Config.svelte` (`#/config`),
`Priorities.svelte` (`#/priorities`), `Logs.svelte` (`#/logs`), `Notifications.svelte`
(`#/notifications`).

**Fatos negativos que economizam busca:**

- **Status e lista de animes são uma tela só.** Não existe rota "Biblioteca" separada
- **Não existe `Layout.svelte`** — foi apagado; o shell é `AppShell.svelte`. Ele escrevia os seis links
  de navegação duas vezes, um bloco desktop e um mobile
- **Não existe endpoint de health-check.** `stores/backendHealth.ts` deriva a saúde do backend dentro
  do `apiRequest`, das respostas que o app já faz
- `AnimeDetail` não tem coluna `#`: ela repetia, em outra fonte, o número que o título do episódio já
  carrega

**Estrutura:** `components/shell/` (AppShell, NavRail, NavTabBar, MoreMenu, SystemBanner),
`components/ui/` (primitivas), `lib/domain/` (lógica pura, sem import de Svelte, coberta por vitest),
`lib/stores/`, `lib/utils/`, `lib/api/client.ts` (wrappers tipados por endpoint) e
`lib/websocket/client.ts`.

**Regras da camada de UI:**

- **Nenhum componente de `ui/` contém hex literal** — todos consomem os nomes semânticos do Tailwind
  apoiados em `lib/design/tokens.css`
- `lib/navItems.ts` é a **fonte única** da lista de navegação, lida por `NavRail`/`NavTabBar`/`MoreMenu`
- `AppShell` escolhe `NavRail` vs `NavTabBar` por `matchMedia` em JS, **não** por `hidden md:flex` —
  a versão CSS manteria os dois montados, duplicando ids
- `torrentStatus.ts` tem de ser mantido em sincronia à mão com `statusSlug()` em
  `src/internal/torrents/status.go`
- **Banner é estado degradado; toast é falha de ação.** `lib/domain/systemBanner.ts` escolhe UM banner
  por precedência (backend inacessível → backend 5xx → AniList fora → rate limit), nunca empilha
  (decisions.md #66). Erros do client caem em `toasts.add`, salvo quando quem chama pede o contrário
- Progresso vem de **um** `Stats()` por torrent, puxado só enquanto uma tela está aberta
  (decisions.md #30) — `speedHistory` e `stallTracker` não fazem poll próprio: a tela dona empurra
