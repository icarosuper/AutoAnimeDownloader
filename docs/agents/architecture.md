# Architecture

> **How to use this file.** It answers *"which file do I open and what bites me there"* — flow
> between files, invariants that hurt to break, negative facts, ambiguous names and the pointer to
> the decision. It does **not** list symbols with a one-line purpose: for "which functions exist in
> this package", `grep '^func ' <file>.go` answers for free and always up to date. Read the
> component map and the flow, then jump straight to the section of the package you'll touch.

## Components

```
src/cmd/daemon/      → Daemon binary entry point (embeds frontend build, starts API)
src/cmd/cli/         → CLI binary (same package `main`, split by resource) talking to the daemon REST API
src/internal/
  api/               → HTTP server, REST handlers, WebSocket, middleware
  daemon/            → Verification loop: Anilist → Nyaa → embedded torrent client → track episodes
  files/             → Config, episode tracking (JSON files), and library hardlinking (Librarian)
  anilist/           → GraphQL client for Anilist API
  nyaa/              → HTML scraper for Nyaa torrent site
  torrents/          → Embedded BitTorrent client (github.com/cenkalti/rain/v2) behind a TorrentBackend interface
  frontend/          → Svelte 5 + Vite + Tailwind 3 web UI (compiled to Go embed). No
                       component library: the primitives live in components/ui/ and the
                       colors in lib/design/tokens.css (decisions.md #91)
  notifications/     → Webhook template interpolation and HTTP firing. Called by daemon on NewEpisode/DownloadFailed; by job queue on DownloadCompleted.
  logger/            → zerolog-based structured logger (console + rotating file)
  datadir/           → The data dir path (~/.autoAnimeDownloader, %APPDATA%\.autoAnimeDownloader) — leaf package
  diagnostics/       → Bug report bundle (redacted environment, config, log) and the crash file lifecycle
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
| `blocked_episodes` | `~/.autoAnimeDownloader/` | Episodes to skip (JSON array of `{anime_id, episode}`, no extension). The old format (an array of AniList node ids) is discarded with a warning when read — see decisions.md #52 |
| `anime_settings` | `~/.autoAnimeDownloader/` | Per-anime settings keyed by AniList **media** ID (JSON map, no extension) |
| `standalone_animes` | `~/.autoAnimeDownloader/` | Media IDs tracked **without** being in any AniList list (JSON array of IDs, no extension) |
| `anilist_cache` | `~/.autoAnimeDownloader/` | Snapshot of the `anilist` caches (MAL lists and the MAL→AniList id mapping included), so the app survives AniList being down (JSON, no extension). **Opaque** bytes to `files` on purpose: the schema belongs to `anilist` (`persist.go`), and that's what keeps `files` from importing `anilist` just to persist its cache. Disposable — deleting it only costs an outage with no safety net until the next fetch |
| `daemon.log` | `~/.autoAnimeDownloader/` | Rotating log file |
| `pending_jobs.json` | `~/.autoAnimeDownloader/` | Persisted job queue (`organize` jobs) |
| `session.db` | `~/.autoAnimeDownloader/` | rain resume database (bbolt) — piece bitfields, kept **outside** the download path so it survives a library path change |
| `queue.json` | `~/.autoAnimeDownloader/` | Download queue state: `{"order": [...], "paused": [...]}`. Lives next to the resume database because it is torrent-client state, not user config. Missing/corrupted = rebuilt from `AddedAt`, never fatal — see decisions.md #41 |
| `crash.log` | `~/.autoAnimeDownloader/` | Where the Go runtime writes a fatal panic (`debug.SetCrashOutput`, set in `main.go` before `logger.Init`). Empty on a clean run |
| `crash-pending.log` | `~/.autoAnimeDownloader/` | The previous run's crash, promoted from `crash.log` at boot. Its existence is `pending_crash_at` in `/status` |
| `crashes/` | `~/.autoAnimeDownloader/` | Handled crashes (`crash-<yyyymmdd-hhmmss>.log`, named from the crash's mtime), newest 5 kept |
| `download_root.id` | `~/.autoAnimeDownloader/` | Id of the download folder the session is bound to. Its twin, `.aad_root`, lives **inside** the download folder; the pair is how a moved/trashed/replaced folder is detected — see decisions.md #34 |

Windows uses `%APPDATA%\.autoAnimeDownloader\` for **all** the config/state files above (note the leading dot — same folder name as on Linux). The path is computed in **one** place, `datadir.Path()` — every file above (and the logger, the CLI's PID file and the crash files) goes through it. It's a leaf package on purpose: `files` imports `logger`, so the function can't live in `files`. There is no dotless `%APPDATA%\AutoAnimeDownloader\` variant.

## On-Disk Layout

- **Download / seeding:** torrents live at `<Config.DownloadPath()>/<torrent-id>/...`, i.e. `<completed_anime_path>/.torrents/<torrent-id>/...` (rain's `DataDir` with `DataDirIncludesTorrentID`). Files are **never renamed here** — renaming would break seeding. Torrents keep seeding after completion. The download directory is **derived**, not user-configured — see decisions.md #31.
- **Library (Jellyfin):** when a torrent completes, its video files are **hardlinked** into `<completed_anime_path>/<AnimeName>/` — one folder **per AniList entry**, season/cour marker kept (`sanitizeName`, decisions.md #45), plus a `tvshow.nfo` carrying the AniList id. With `RenameFilesForJellyfin`, every file gets the Jellyfin name `"Anime Name - E05.mkv"` — a single episode from its record, a batch file from the episode number parsed out of its own filename (`nyaa.ExtractEpisodeNumber`); files with no readable number (NCOP/NCED, extras, movies) and two files of one pack landing on the same number keep the raw filename. The hardlink shares bytes with the seeded copy, so no space is duplicated.
- **Same volume, by construction:** the download directory lives inside `completed_anime_path`, so the old cross-filesystem failure mode is now structurally impossible. `Librarian.ProbePath(completedPath)` still validates that the filesystem supports hardlinks at all (exFAT/FAT32/some SMB shares don't) — it runs on config save and on every verification pass (decisions.md #26).
- **Deletion** frees space by removing **both** links: the library hardlink (`Librarian.RemoveFromLibrary`) and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). A batch torrent shared by multiple episodes is only removed once **all** its episodes are deleted **and** every episode the pack covers is claimed by some entry in the group (`canRemoveTorrent` / `hasUnclaimedContent`, decisions.md #74). While it is held back, its records are kept too (`forgettableKeys`) — they carry the declared range. The manual delete path passes `force` and bypasses that guard.
- **No `config.json` versioning.** There is no `config_version` field, no migration chain and no
  startup block: `LoadConfigs` unmarshals **over** `getDefaultConfig()`, so an added or removed field
  needs no migration at all, and the two legacy renames are detected by **shape**, in `LoadConfigs`
  only — `PUT /config` does not repeat them. See decisions.md #90.
- **No `save_path` migration.** `daemon.MigrateSavePath` used to move a legacy `save_path`'s torrent folders into the derived download path at boot and at the top of every pass; it was removed along with the `Config.SavePath` field. An old `config.json` still carrying the key loads (JSON ignores unknown fields) but its data is **not** moved — moving it is a manual pre-upgrade step. See decisions.md #31.

## API

The table below is the **only** endpoint doc: there is no OpenAPI spec or `/swagger/` anymore
(removed — they were 33% of the binary and nothing consumed the spec).

Key endpoints:

| Method | Endpoint | Handler func | File |
|--------|----------|-------------|------|
| `GET` | `/api/v1/status` | `handleStatus` | `endpoint_status.go` — the `disk_low` threshold lives **only** on the server. The AniList state is written by **any** call (a pass or a frontend poll), because they all go through `sendAnilistRequest` — decisions.md #65/#66. `anilist.cache_saved_at` is filled in on **read**, not when the state is written: it's not a fact about AniList, it's what the banner uses to say how old the cache on screen is. `pending_crash_at` (mtime of `crash-pending.log`, `null` when there is none) is what makes the web UI offer the report screen after a crash |
| `GET` | `/api/v1/last-check` | `handleLastCheck` | `endpoint_last_check.go` — memory only. `problems` (should have downloaded and did not) and `limits` (the config doing what it was configured to do). When `pass_error` is set both lists come empty, because `SetLastCheckError` clears the report (decisions.md #82). A manual download stays out: that path returns the error on its own HTTP response |
| `GET/PUT` | `/api/v1/config` | `handleConfig` | `endpoint_config.go` |
| `GET` | `/api/v1/config/priorities/defaults` | `handlePriorityDefaults` | `endpoint_priorities.go` |
| `GET` | `/api/v1/animes` | `handleAnimes` | `endpoint_animes.go` — `next_airing_at` is a **timestamp**, not a countdown: the response is cached 60s and a precomputed counter would freeze. Search by `alt_names` is client-side (`lib/utils/status.ts`). `episodes_downloaded` is **files on disk right now**, not a lifetime total: it DROPS when the daemon prunes watched episodes, so `released - downloaded` is not "still to download" — `episodes_pending` is ([decisions.md #88](decisions.md#88-a-barra-de-progresso-deriva-baixados-de-episodes_pending-e-não-de-episodes_downloaded)) |
| `GET` | `/api/v1/anilist/search?q=&include_unreleased=` | `handleAniListSearch` | `endpoint_anilist_search.go` — a term shorter than 3 chars returns an empty list, **not** 400. `block_reason` per result (`""` = addable) |
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
| `GET` | `/api/v1/diagnostics?titles=<bool>&error=&context=&anime=` | `handleDiagnostics` | `endpoint_diagnostics.go` — `{bundle, environment}`. `titles` defaults to on; `error`/`context`/`anime` are the error toast's, echoed into the bundle's Context section after redaction |
| `DELETE` | `/api/v1/diagnostics/crash` | `handleDiagnosticsCrash` | `endpoint_diagnostics.go` — moves `crash-pending.log` to `crashes/`. Idempotent; answers the usual 200 envelope, not 204, because `apiRequest` always parses the body |
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

`RunAnimeDebug` mirrors `processAnimeEpisodes`: episode selection (`anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`), the pack search/selection (`partitionSearchResults` + `pickBatches`) that lifts the per-anime limit when it resolves, and the per-episode `searchSingleEpisode` fallback when no batch/movie/single magnet was found — so the debug pass reports the pipeline the daemon actually runs (see [decisions.md #58](decisions.md#58-runanimedebug-espelha-processanimeepisodes-enumeração-resolução-de-pack-e-fallback-single)). The one deliberate difference: it searches every uncovered episode, without the miss backoff of [decisions.md #100](decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes).

`scripts/robustness-animes.txt` (curated media IDs, one per line) + `scripts/debug-batch.sh` (`make debug-batch`) run that pass over the whole list and write a triaged `.debug_batch/report.md`.

---

## Mapa de arquivos — o que mora onde e o que morde

### `src/internal/daemon/` — o passe

**Não existe `daemon.go`.** O orquestrador está espalhado: `loop.go` (a goroutine), `verification.go`
(o passe), `episodes.go` (seleção e remoção por episódio), `search.go` (busca no Nyaa + filtros),
`coverage.go` (posse por cobertura), `helpers.go` (guardas de config/disco), `stalled.go` (torrent
sem peers), `manual_download.go` (downloads vindos da API) e `standalone.go`.

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
3. **Episódio avulso**, por episódio ainda descoberto. An episode can skip this search without
   any Nyaa line in the log: after an empty search it waits 1h→24h before the next one, and the
   episodes after the first miss of the pass skip it too (`searchMisses`, in memory,
   [decisions.md #100](decisions.md#100-an-empty-per-episode-search-waits-before-repeating-and-stops-the-later-episodes)).
   The manual download (`DownloadStandaloneAnime`) and `RunAnimeDebug` don't apply it.

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
- `checkDiskSpace` barra download, **nunca** o passe de verificação. Erro de `statfs` não bloqueia.
  Recebe o tamanho do torrent e é chamada **por candidato**: o segundo da lista pode caber onde o
  primeiro não coube, e o gatilho de tamanho vale mesmo com a porcentagem desligada
  ([#93](decisions.md#93-a-guarda-de-disco-tem-dois-gatilhos-e-o-de-tamanho-vale-mesmo-com-min_free_disk_percent--0)).
  In the pass it goes through a per-pass `diskBudget` (`diskbudget.go`): the size is checked on top
  of what the session's incomplete torrents and this pass's adds still have to write
  ([#101](decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space))
- O teto de tamanho de pack é aplicado **duas vezes**: na busca do Nyaa e de novo no filtro do daemon
  (decisions.md #80). Mexer só num dos dois não muda o comportamento

**Custo de rede.** `packSet` resolve a faixa de um pack **sob demanda** — do nome quando o nome traz,
senão da página de detalhe — e memoriza **inclusive a falha**; sem isso `assignBatches`, que varre os
packs uma vez por episódio, viraria uma requisição por episódio. Teto de `maxPackDetailFetches` (3)
páginas de detalhe por anime por passe. Ver [decisions.md #84](decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything).

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

**`stalled.go`** — chamado no topo do passe, logo depois do `backend.List()`. Torrent a zero peers
por mais de uma hora **sai da sessão** pelo mesmo `HandleTorrentFailure` de um torrent que a rain
parou com erro: nada de caminho novo, e o passe seguinte procura de novo no Nyaa
([#94](decisions.md#94-a-stalled-torrent-is-dropped-through-the-failure-path-and-only-one-that-is-trying-counts-as-stalled)).
O relógio é de pacote (`stalledTorrents`), em memória, e **não** é o `stallTracker.ts` do frontend —
aquele pinta o chip "sem seeds" aos 10 minutos e não conversa com o daemon. O snapshot passado para
a função não é refeito depois do drop: quem saiu volta a ser procurado só no próximo passe.

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
aquilo não gera tipo TS utilizável. A cascata de precedência dos três problemas de busca
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

**`mkv.go`** — `VideoCodec` lê o codec do **header** do arquivo, não do nome: `nyaa.ExtractCodec` lê do
nome do release e boa parte dos uploads nunca escreve isso (Erai-raws não escreve; muito BD rip também
não). Matroska é EBML (RFC 8794), e o codec fica num caminho curto e fixo **antes** dos Clusters
(`Segment → Tracks → TrackEntry → {TrackType, CodecID}`), então a leitura custa poucos KB e alguns
seeks, nunca o arquivo inteiro. `TrackType` é conferido porque um `.mkv` de anime carrega vídeo, áudio
e várias legendas — pegar o primeiro `TrackEntry` às cegas reportaria o codec do áudio. Os `CodecID`
são mapeados para o **mesmo vocabulário do `nyaa.ExtractCodec`** (`HEVC`, `H.264`, `AV1`), senão o
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

**`filesystem.go`** — the `FileSystem` interface exists for tests (`MockFileSystem`); it includes
`Link`, which is what the `Librarian` uses.

### `src/cmd/cli/` — daemon CLI

Split by resource, same package `main` (`main_test.go` keeps testing unexported helpers): `main.go`
holds the app, global flags, `start` / `stop` / `open` / `loop`; `config.go`, `status.go`,
`animes.go`, `torrents.go`, `logs.go`, `report.go`, `notifications.go` hold the matching trees.
There is no root `episodes` command and no `GetEpisodes` — `GET /api/v1/episodes` is gone; listing
is `animes episodes <id>`.

`report` and `report crash-ack` call `diagnostics.Build` / `ArchivePendingCrash` against
`datadir.Path()` and never the HTTP client, so they work with the daemon dead. `--yes` is a CLI
gate: the API has no confirmation parameter ([decisions.md #104](decisions.md#104---yes-is-a-cli-gate-the-api-keeps-its-query-defaults)).

### `src/internal/api/` — HTTP

Handler pattern, response envelope and the new-endpoint checklist live in
[conventions.md](conventions.md), not here.

**`client.go` is the HTTP client _for_ the daemon API, used by the CLI binary** (`cmd/cli`) — the
daemon does not use it. Confusing the two leads to looking for business logic in it. There is no
`GetEpisodes`.

**`endpoint_animes.go`** — the two filters (`DownloadStatuses` server-side, `DownloadMediaStatuses`
client-side) only bar **new** entries: an anime that already has a downloaded episode never leaves
the list, whatever its current status. For those orphans, `refreshOrphanAnimes` reloads
cover/progress/blacklist in at most two batch queries; failure only blanks the AniList-sourced
fields, instead of dropping the anime or failing the request. `countPendingEpisodes` counts by
**episode number**, not `released - downloaded`, because a watched episode can still be saved
(`watched_episodes_to_keep`).

**`endpoint_anime_episodes.go`** — `AnimeEpisodeInfo` **does not carry an episode name**: AniList
does not expose a per-episode title (`AiringNode` only has number and time) — decisions.md #52.
`animeId` is the **media** id, the same one as the `anilist.co/anime/{id}` link; there is no
separate `anilistId` field.

**Vocabulary — never call a standalone anime "manual".** Standalone = tracked without being on any
AniList list. "Manual" already means something else: the user touched that episode by hand
(`ManuallyManaged`, `ManualDownloadEpisode`). The files are `standalone.go`, `standalone_guard.go`,
`endpoint_standalone_animes.go` and `endpoint_anilist_search.go`. `standaloneGuard` is **one
blocking rule with two consumers** (the `POST` and the search handler), so "the front does not let
you click" and "the back returns an error" agree by construction.

**`endpoint_torrents.go`** — a file's `path` is the **raw** path from the metadata and is never
rewritten to "Ep NN": the point of the panel is reading fansub/resolution/codec off it. `Progress`
falls back to the piece ratio (`PiecesHave/PiecesTotal`) when `BytesCompleted` reads 0 with a
non-zero total. `DELETE` is registered on the **same** mux pattern as pause/resume/announce — a
Go 1.22+ pattern with no method prefix matches every verb — so the method check inside the handler
is what separates the two.

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

<a id="media-status-filter"></a>**Two status filters, similar names, different things**:
`Config.DownloadStatuses` filters by **list** status (`MediaListStatus` — the user's relationship with
the anime, e.g. `CURRENT`) and is applied server-side by AniList (`status_in`). `Config.DownloadMediaStatuses`
filters by **media** status (`MediaStatus` — the anime's airing state, e.g. `RELEASING`); AniList
doesn't accept that one in the same filter, so both consumers apply it client-side via
`anilist.MediaStatusAllowed`: `searchAnilist` (`daemon/verification.go`, the download pipeline, right
after the per-account fetch and before the dedup) and `fetchAniListEntries` (`api/endpoint_animes.go`,
the frontend listing). **Neither filter hides an anime that already has a downloaded episode**,
whatever its current status — see `endpoint_animes.go` above.

**Toda query declara uma `Priority`, e não há valor padrão de propósito**: query nova não compila sem
alguém decidir a criticidade. O gate de orçamento (`health.go`) recusa `PriorityDisposable` com
`ErrBudgetLow` quando a cota da AniList está baixa — nada sai, e a saúde do pacote não muda
(decisions.md #72). O limite é 30 req/min por IP.

**Saúde é exposta como código, nunca frase pronta** — o daemon não sabe o locale do navegador (mesma
razão de `daemon/passerror.go`). Ela é escrita por **toda** resposta que passa por
`sendAnilistRequest`, e a AniList devolve 200 com erro no corpo, então o envelope também é lido
(decisions.md #65/#66).

**Caches** (`ttlCache[T]`, todos em memória; os de baixo também vão para um snapshot em disco, ver
`persist.go`): `GetFrontendAnimeList` 60s por `username+statuses` — o poll de `/api/v1/animes`, 30s
por aba aberta, é o que estourava a cota da AniList (decisions.md #46); `customLists` 5min
(decisions.md #11); `mediaByIDCache` 60s por media id (avulsos); `seriesCache` 24h; `searchCache`
10min por `termo-minúsculo+includeUnreleased` — **o único com chave de texto livre**, por isso é o
único com teto (`searchCacheMaxEntries`, descarte total, não LRU) e o único que `ttlCache.size()`
serve. `passListCache` e `mediaEntryCache` são **só fallback**: gravados com TTL zero, nunca
consultados por `get`.

**Três buscas de lista com nomes parecidos, e os campos NÃO são os mesmos** — é a desambiguação que
mais morde aqui:

| função | quem chama | tem `relations`? | tem `status` da entrada? | tem `coverImage`? |
|---|---|---|---|---|
| `GetFrontendAnimeList` | poll de `/animes` | ✗ | ✗ | ✓ |
| `GetAllCurrentAnime` | passe do daemon | ✓ | ✓ | ✗ |
| `getMediaListEntry` (`mediaByIDFields`) | `/animes/{id}/*`, download manual | ✓ | ✓ | ✓ |

Por isso a tela de detalhe **não** pode ser servida do cache do frontend: ficaria sem `relations`, e
é dele que sai a busca do Nyaa do download manual. É também por isso que existem três caches de
`MediaList` em vez de um (decisions.md #92).

**`persist.go` — o snapshot em disco (`anilist_cache`, na pasta de configs).** Existe para uma coisa
só: telas e passe continuarem de pé com a AniList fora do ar, restart incluído. **A invariante que
dói quebrar:** as coleções de `MediaList` voltam do disco com **TTL zero** (`restore(..., 0)`), então
só `getStale` as vê e só o caminho de falha as serve — restaurar com TTL vivo faria toda tela passar
a mostrar a lista de ontem sem nem tentar a rede. O `seriesCache` é a exceção (imutável por
construção, volta com o TTL cheio). Fato negativo: **não existe** modo cache, flag de offline nem
camada de repositório — o `ttlCache` segue sendo a única frente de leitura.

**`ErrFromCache` acompanha o valor, não o substitui.** Quem não checa `errors.Is` vê `err != nil` e
se comporta como antes do cache existir, que é o padrão seguro; quem opta por usar o valor escreve
isso no call site. `/animes` usa para **recusar** o refresh de órfãos (seria a rajada da #65 contra
uma API morta); `verification.go` usa para **aceitar** a lista, deleção por status incluída — com o
risco registrado na decisions.md #92. `timeUntilAiring` é relativo à busca, então gravação e leitura
passam por `stampAiringAt`/`rebaseAiring`; sem isso o passe servido do cache concluiria para sempre
que nada novo estreou.

**`series.go` — o eixo absoluto da série.** A numeração contínua que atravessa cour e part, que boa
parte dos grupos usa nos nomes de arquivo. A AniList **não tem id de franquia**: a cadeia de `PREQUEL`
é a única fonte (decisions.md #71), e como uma query só expande 2 níveis de `relations`, resolver a
cadeia é um BFS, não uma leitura (decisions.md #77). Cada media buscada ensina **dois** elos (níveis 0
e 1, os únicos em posição autoritativa); o avô, cujo `relations` veio cortado, volta para a fronteira
em vez de virar raiz. A semente vem pronta de quem chama porque **`anilist` não pode importar
`files`** — e o conjunto certo só o daemon conhece.

**`mal.go` — MyAnimeList, as a list source only.** It lives in `anilist` on purpose (it needs
`ttlCache`, the snapshot, `sendAnilistRequest` and `GetMediaByIDs`); there is **no** `internal/mal`
package and no `Account` type. A MAL entry becomes the same synthetic `MediaList` a standalone anime
gets, so `MediaList.Id == 0` means "standalone **or** MAL" and nothing can tell them apart by it.
What bites: `ErrMALEmpty` is a **failure** — every caller must treat it like a failed fetch, or the
account loses its deletion veto; and the status filter must run **before** `GetMediaByIDs`, or the
frontend poll hydrates the whole list every minute. The MAL account loops live next to the AniList
ones in `daemon/verification.go` and `api/endpoint_animes.go`/`api/standalone_guard.go`, keyed
`"mal:" + user` in `inDeleteStatus`
([decisions.md #95](decisions.md#95-myanimelist-is-a-list-source-not-a-metadata-source)).

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

### `src/internal/diagnostics/` — bug report

Builds the report bundle from **files in the data dir only** — config, `anime_settings`,
`daemon.log` (or the newest `daemon-*.log.gz` after a rotation), `crash-pending.log` — and never
asks the running daemon. That's what lets the CLI `report` command build the same bundle
with the daemon dead. The config is read as a raw map, **not** through `LoadConfigs`: a config that
fails to load is exactly what a report must still carry. Nothing is ever sent; the user attaches
the file by hand ([decisions.md #102](decisions.md#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file)).

- **Redaction is by log key** (`redact.go`, `keyClasses`): keep / drop / pseudonymize (usernames →
  `userN`) / title (`<title-N>` when titles are off). A key missing from the table is treated as a
  title at runtime, and `keys_test.go` fails the build for any `.Str("key", …)` in the source that
  isn't classified ([decisions.md #103](decisions.md#103-log-redaction-is-by-key-and-a-test-forces-every-new-key-to-be-classified)).
  On top of the keys, free text is scrubbed: webhook URLs/headers anywhere → `<webhook-N>`, the
  home dir → `~`, URLs to hosts outside the allowlist → `<url>`. `"HTTP request"` lines are dropped
  whole (they carry the LAN IPs)
- **Crash lifecycle** (`crash.go`): the runtime writes a fatal panic to `crash.log`; the next boot
  (`StartCrashCapture`) promotes a non-empty one to `crash-pending.log`; handling it in the UI
  (`DELETE /diagnostics/crash`) moves it to `crashes/`, newest 5 kept. An older pending crash
  nobody handled is archived before a new one takes its place, so none is lost
- `environment.go` reads the OS through a `host` struct (goos/getenv/readFile) so the docker,
  systemd and desktop detection is testable without the real machine

### Small packages

- **`datadir/`** — `Path()`: the data dir, created if missing. Not cached: it's a stat and a
  `MkdirAll`, and callers run once at boot
- **`logger/`** — zerolog, console + rotating `daemon.log`. `InitDebug` sends the trace to another
  file, so that `--debug-anime` never touches `~/.autoAnimeDownloader`
- **`version/`** — `Version`, `"dev"` by default, overridden by ldflags
- **`cli/`** — daemon lifecycle control for the CLI binary (`cmd/cli` is the only importer). Two
  paths, in this order: a systemd **user** unit and, if it isn't installed, a PID file. **There is no
  Windows Service Manager integration** — it existed, calling `sc`/`net`, but nothing in the repo
  installs that service (`development.md` says a Windows service isn't supported) and
  `package-windows` doesn't even ship the CLI binary, so all three paths were unreachable in every
  published artifact. On Windows the PID file applies
- **`tray/`** — build-tagged: the real implementation needs CGO, so `tray_stub.go` keeps the
  CGO-less build compiling (CI enforces it in `build.yml`). On Linux the daemon only starts the
  tray with a non-empty `DISPLAY`. "Report a problem" opens `#/report` in the browser
- **`frontend/embed.go`** — `//go:embed dist/*`. `dist/` has to exist **before** `go build`, which
  is why the frontend always builds first

### `src/internal/frontend/src/` — Svelte 5

There is no component library: daisyUI was removed (decisions.md #91). Button, modal, checkbox,
chip and the like live in `components/ui/`; color comes from `lib/design/tokens.css`, which is the
single source of truth and switches themes through its own `[data-theme]`.

**Routes** (hash router): `Status.svelte` (`#/`), `AddAnime.svelte` (`#/add`), `Downloads.svelte`
(`#/downloads`), `AnimeDetail.svelte` (`#/status/:id`), `Config.svelte` (`#/config`),
`Priorities.svelte` (`#/priorities`), `Logs.svelte` (`#/logs`), `Notifications.svelte`
(`#/notifications`), `Report.svelte` (`#/report`).

**Negative facts that save a search:**

- **Status and the anime list are one screen.** There is no separate "Library" route
- **There is no `Layout.svelte`** — it was deleted; the shell is `AppShell.svelte`. It wrote the six
  navigation links twice, a desktop block and a mobile one
- **There is no health-check endpoint.** `stores/backendHealth.ts` derives backend health inside
  `apiRequest`, from the responses the app already makes
- **There is one toast store**, `stores/toast.ts`. A second one (`toasts.ts`) used to receive the
  client's automatic error toasts and nothing rendered it — every one of them was silently lost
- `AnimeDetail` has no `#` column: it repeated, in another font, the number the episode title
  already carries

**Structure:** `components/shell/` (AppShell, NavRail, NavTabBar, MoreMenu, SystemBanner),
`components/ui/` (primitives), `lib/domain/` (pure logic, no Svelte import, covered by vitest),
`lib/stores/`, `lib/utils/`, `lib/api/client.ts` (typed wrappers per endpoint),
`lib/websocket/client.ts` and `lib/report.ts` (links to the report screen and to GitHub; `REPO_URL`
lives only there).

**UI layer rules:**

- **No `ui/` component contains a hex literal** — they all consume the semantic Tailwind names
  backed by `lib/design/tokens.css`
- `lib/navItems.ts` is the **single source** of the navigation list, read by `NavRail`/`NavTabBar`/`MoreMenu`
- `AppShell` picks `NavRail` vs `NavTabBar` by `matchMedia` in JS, **not** by `hidden md:flex` — the
  CSS version would keep both mounted, duplicating ids
- `torrentStatus.ts` has to be kept in sync by hand with `statusSlug()` in
  `src/internal/torrents/status.go`
- **A banner is degraded state; a toast is a failed action.** `lib/domain/systemBanner.ts` picks ONE
  banner by precedence (backend unreachable → backend 5xx → AniList down → rate limit), never stacks
  them (decisions.md #66). Client errors become a `toast.error` unless the caller passes `silent`.
  A reportable banner's "Report" link opens `#/report` too, pre-filled with the banner's kind and
  detail — not a bare GitHub link.
  An **unexpected** failure (5xx, network, unreadable response — not a 4xx, not an abort) gets a
  10 s toast with a Report link to `#/report` pre-filled with the error. `toast.ts` skips a toast
  identical to one already on screen, because most pages also toast the same `err.message` in
  their catch — the client's one, with the link, wins
- **Crash → report screen.** `SystemBanner` polls `/status` every 30 s; a new `pending_crash_at`
  opens `#/report?crash=1` on a fresh page load and only toasts in a tab that was already open (a
  route change could throw away an unsaved form). The last crash reacted to is remembered in
  `localStorage` (`aad.lastPendingCrash`)
- Progress comes from **one** `Stats()` per torrent, pulled only while a screen is open
  (decisions.md #30) — `speedHistory` and `stallTracker` don't poll on their own: the owning screen pushes them
