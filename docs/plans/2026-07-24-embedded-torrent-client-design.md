# Cliente BitTorrent Embutido (rain/v2) — Design

Remover a dependência do **qBittorrent externo** e embutir o cliente BitTorrent no próprio binário via `github.com/cenkalti/rain/v2`. O app vira um binário único, sem exigir que o usuário instale/configure um qBittorrent. As decisões de produto estão travadas (ver prompt de origem); este documento é o **plano de implementação faseado** e aponta explicitamente onde a realidade do código/lib diverge do que foi assumido.

Validado contra o fonte da rain na tag **v2.3.1** (`github.com/cenkalti/rain/v2`, pacote `torrent`).

---

## 0. Conflitos entre as decisões travadas e a realidade

Antes do plano, os desvios encontrados ao ler o repositório e o fonte da v2. Nenhum é fatal; o plano os absorve.

1. **rain NÃO tem UPnP/NAT-PMP** (refuta a opção "automático" da decisão #10). A v2.3.1 expõe só um range de porta de escuta (`PortBegin`/`PortEnd`, default 20000–30000). DHT+PEX funcionam sem encaminhamento; só as conexões de entrada ficam limitadas. → **v1 usa o range padrão da rain**, encaminhamento é opcional e documentado; porta fixa configurável fica como enhancement futuro (ver Riscos).
2. **`NotifyStop()` retorna `<-chan error`** (não `struct{}`). `NotifyComplete()`, `NotifyMetadata()`, `NotifyClose()` retornam `<-chan struct{}`. Todos são **one-shot** (disparam uma vez; re-armar após re-add). O listener por torrent trata isso.
3. **`NewSession(cfg Config)` recebe `Config` por valor**; os defaults vêm da **variável** `torrent.DefaultConfig` (não é função). `Torrent.InfoHash()` retorna o tipo `InfoHash`, não `string` (usar `.String()`).
4. **O "episodes.json" na verdade é `downloaded_episodes`** (JSONL, sem extensão, em `~/.autoAnimeDownloader/`), indexado por `EpisodeID` (AniList airing-node ID). O join com o cliente de torrent é o campo **`EpisodeHash` (= infohash)**. Setar `AddTorrentOptions.ID = infohash` mantém esse join sem tradução — decisão #5 confirmada.
5. **`verification.go` NÃO é um loop de polling.** Ele faz **uma** leitura de estado por passe (`GetDownloadedTorrents()`), acionado por estado externo (AniList/Nyaa). Portanto **não** vira event-driven: apenas troca a leitura HTTP por `session.ListTorrents()` (em memória, barato). Só a notificação-de-conclusão (hoje `JobNotifyOnComplete`, que faz poll) vira event-driven. Esta é a leitura correta (e mais estreita) da decisão #2 — não transformar o daemon inteiro em reativo.
6. **Layout em disco** é `<DataDir>/<id>/<conteúdo-do-torrent>` com `DataDirIncludesTorrentID=true` por default — próximo da decisão #5 (o `<name>` vem do conteúdo do torrent, não é inserido pela lib).
7. **Arquivos semeados mantêm o nome original do torrent.** Hoje `RenameEpisodeFile` renomeia o arquivo real via qBit `/renameFile`. No modelo de hardlink **não** renomeamos o arquivo semeado (quebraria o seeding); o nome no formato Jellyfin vive **apenas no hardlink da biblioteca**. `JobRenameFile` deixa de existir.
8. **`Torrent.Move()` é transferência inter-sessão via RPC** (`host:port`), não relocação em disco — decisão #5 confirmada; não usar.
9. **Licença MIT** (compatível com GPLv3) e deps pure-Go (resume via `go.etcd.io/bbolt`, sem cgo) — compatível com `CGO_ENABLED=0`. Marcado alta-confiança; confirmar com uma asserção de build no CI (P0).

### Decisões de produto respondidas nesta rodada
- **Apagar episódio = apagar AMBOS os links** (torrent + hardlink da biblioteca) para liberar espaço de fato. A "sobrevivência do hardlink" da decisão #7 é uma propriedade de segurança do mecanismo, não o comportamento da limpeza automática.
- **Batch/multi-arquivo = hardlink da pasta, sem rename** (espelha o comportamento atual, que pula rename de batch).
- **`qbittorrent_url` removido por completo** (hard-remove). Configs antigas continuam carregando (JSON ignora campos desconhecidos no unmarshal); a chave some no próximo save.
- **Sem `torrent_port` na v1** (YAGNI); rain usa o range padrão.

---

## 1. Inventário de arquivos / call sites que mudam

### Núcleo do torrent (substituição)
| Arquivo | Papel hoje | Mudança |
|---|---|---|
| `src/internal/torrents/torrents.go` | Cliente HTTP do qBittorrent WebUI (`/api/v2/torrents/...`), `TorrentService` reconstruído por operação | **Reescrito** como wrapper da sessão rain (ver P2). Sai todo o HTTP, `CATEGORY`, `getTorrentsHash`, `RenameEpisodeFile`, `SendAnimeToCompletedFolder`, `IsTorrentCompleted(state string)` |

### 11 sites que constroem `TorrentService` hoje → passam a receber a `Session` injetada
| # | Arquivo:linha | Contexto |
|---|---|---|
| 1 | `torrents/torrents.go:38` | `NewTorrentService` (def) |
| 2 | `daemon/manual_download.go:48` | `newTorrentServiceFromConfig` (fábrica) |
| 3–5 | `daemon/manual_download.go:56,87,126` | `ManualDownloadEpisodeWithMagnet`, `ManualDownloadAnimeWithMagnet`, `ManualDownloadEpisode` |
| 6 | `daemon/verification.go:47` | passe de verificação |
| 7 | `daemon/jobs.go:180` | reconstruído a cada tick de 5s em `processDueJobs` |
| 8–11 | `api/endpoint_episode_actions.go:150,272,377,474` | delete / redownload / replace-ep / replace-anime |

Sites de teste (mock HTTP): `daemon/episodes_test.go:152,188,253,307,340,428`, `tests/unit/torrent_test.go:101` — migram para o `TorrentBackend` fake (P2).

### Fila de jobs e orquestração
| Arquivo | Mudança |
|---|---|
| `daemon/jobs.go` | Remove `JobNotifyOnComplete`, `JobRenameFile`, `JobMoveToCompleted` (os 3 tipos que fazem poll). Adiciona `JobOrganize(hash)` idempotente. Preserva `retryBackoff` (30s·2ⁿ, teto 10min) e persistência em `pending_jobs.json` |
| `daemon/verification.go` | `GetDownloadedTorrents()` → `session.ListTorrents()`; `buildTorrentsHashSet`/`episodeInTorrents` (helpers.go) inalterados na semântica (set por infohash) |
| `daemon/episodes.go` | `DownloadTorrentWithOptions` → `session.Add`; enqueue de rename/move some; deleção passa a "apagar ambos os links" com guarda de batch |
| `daemon/manual_download.go` | usa a `Session` injetada; sem fábrica por operação |
| `daemon/helpers.go` | remove `getQBittorrentURL` + override `QBITTORRENT_URL`; `isConfigComplete` deixa de exigir `QBittorrentUrl` e passa a exigir `CompletedAnimePath` |
| `api/endpoint_episode_actions.go` | remove `getQBittorrentURLForAPI` + override env; usa a `Session` injetada no `Server` |
| `cmd/daemon/main.go` | cria a `Session` no startup, injeta no `Server`/loop/JobQueue, chama `session.Close()` no bloco de shutdown (SIGINT/SIGTERM já existente, linhas ~295–309); roda a **reconciliação de startup** |

### Camada de filesystem
| Arquivo | Mudança |
|---|---|
| `files/filesystem.go` | Adiciona `Link(oldname, newname string) error` à interface `FileSystem` e ao `OSFileSystem` |
| `files/librarian.go` (novo) | `Librarian` (FileOrganizer): hardlink + nome Jellyfin + pasta por anime + `ProbePaths` |
| `files/crossdevice_unix.go` / `crossdevice_windows.go` (novos) | `isCrossDevice(err) bool` (EXDEV / `ERROR_NOT_SAME_DEVICE`) com build tags |
| `tests/unit/mock_filesystem.go` | Adiciona `Link` ao `MockFileSystem` (senão o pacote não compila) |

### Config / API / CLI / Front / Docs / CI
| Arquivo | Mudança |
|---|---|
| `files/filemanager.go` | Remove campo `QBittorrentUrl` e default; `CompletedAnimePath` vira obrigatório de fato (validado no endpoint) |
| `api/endpoint_config.go` (+ `_test.go`) | Remove validação de `qbittorrent_url`; adiciona validação de `CompletedAnimePath` não-vazio **+ sonda de hardlink** (`Librarian.ProbePaths`) |
| `cmd/cli/main.go` | Remove `qbittorrent_url` das chaves de `config get/set` e da tabela |
| `frontend/src/routes/Config.svelte` | Remove a seção qBittorrent e a validação client-side; torna `completed_anime_path` obrigatório |
| `frontend/src/lib/api/client.ts` | Remove `qbittorrent_url` da interface `Config` |
| `frontend/messages/en.json` + `pt-BR.json` | Remove `config_section_qbittorrent`, `config_label_qbit_url`, `config_hint_qbit_url`, `config_val_qbit_url`; ajusta validação de completed path |
| `frontend/tests/smoke/helpers.ts` | Remove fixture `qbittorrent_url` |
| `notifications/notifications.go` | Evento `QBittorrentDownloadCompleted` → `DownloadCompleted`; `ReasonQbitRejected` → mensagem genérica |
| `tests/mocks/qbittorrent/` | **Removido** (substituído pelo `TorrentBackend` fake) |
| `docker/docker-compose.test.yml`, `.github/workflows/build.yml` | Remove serviço/imagem `mock-qbittorrent`; adiciona matriz Linux+Windows e asserção `CGO_ENABLED=0` |
| `README.md`, `docs/guides/*`, `docs/agents/architecture.md`, `docs/agents/decisions.md`, `docs/swagger/*` | Remove qBittorrent; documenta cliente embutido; `swag init` |
| `src/internal/tray/*` | **Sem mudança** (não referencia qBittorrent) |

---

## 2. Ciclo de vida da sessão rain

### Criação e injeção
- Nova struct `torrents.Session` envolvendo `*torrent.Session`, criada **uma vez** no startup do daemon (`main.go`, ao lado da criação da `JobQueue`), armazenada no `api.Server` e passada ao payload do loop e à `JobQueue`. Substitui os 11 call sites.
- **`torrent.Config` derivada da config do app:**
  - `DataDir = configs.SavePath` (onde os arquivos ficam e seguem semeando).
  - `Database = ~/.autoAnimeDownloader/session.db` — **fora do DataDir de propósito**, para que o resume sobreviva a uma troca de `SavePath`.
  - `DataDirIncludesTorrentID = true` (default; garante isolamento por torrent).
  - Porta: range padrão da rain (sem config na v1).
- Construtor: `NewSession(configs *files.Config) (*Session, error)`. A `Session` **não** importa `daemon` (evita ciclo, já que `daemon` importa `torrents`): ela apenas gerencia a rain e **publica eventos**. Quem reage é o `daemon`, via callback/canal — ver §3. O `Librarian` (pacote `files`, folha) e a `JobQueue` são orquestrados pelo `daemon`, não pela `Session`.

### Config incompleta no startup
Hoje o daemon pode iniciar sem config completa (gate `isConfigComplete`). A sessão precisa de `DataDir = SavePath`. **Criação preguiçosa:** se `SavePath` estiver vazio no boot, a sessão não é criada; o loop já não roda até a config ficar completa. Ao salvar uma config completa (endpoint), a sessão é criada. **Troca de `SavePath` em runtime** fecha a sessão atual e abre outra com o novo `DataDir` (ver Riscos).

### Fechamento
`session.Close()` no bloco de shutdown gracioso já existente (`main.go` ~295–309), antes de encerrar o processo — faz flush limpo do resume (bbolt). A `JobQueue.Stop()` (defer) e a remoção do PID file permanecem.

### Reconciliação de startup (segurança contra crash)
Os canais `Notify*` são one-shot e só disparam enquanto há listener. Um torrent que conclui com o daemon desligado, ou um crash entre "concluiu" e "hardlink criado", nunca re-dispara `NotifyComplete`. Por isso:

1. No boot, após a sessão recarregar do BoltDB e re-verificar peças, iterar `session.ListTorrents()`.
2. Para cada torrent com `Stats().Status == Seeding` cujo hardlink de biblioteca **ainda não existe**, enfileirar `JobOrganize(hash)`.
3. Re-armar um listener para cada torrent ainda em andamento.

A **fila de jobs continua sendo a lista durável de trabalho**; o evento da rain apenas substitui o poll como gatilho de enfileiramento. Assim preservamos a robustez que o `pending_jobs.json` dava, sem polling.

### Mapeamento de API (verdade da v2.3.1)
| Operação | Chamada |
|---|---|
| adicionar | `ses.AddURI(magnet, &torrent.AddTorrentOptions{ID: infohash})` → `(*Torrent, error)` |
| listar | `ses.ListTorrents()` → `[]*Torrent`; por torrent `t.Stats()` |
| concluído? | `t.Stats().Status == torrent.Seeding` |
| remover | `ses.RemoveTorrent(id, keepData=false)` |
| identidade | `AddTorrentOptions.ID = infohash` (parseado do magnet); `t.InfoHash().String()` exposto à parte |
| fechar | `ses.Close()` |

O infohash é parseado do magnet **antes** do add (elimina o loop atual de "adiciona por nome e faz poll pelo hash", `torrents.go:87–99,239–259`).

---

## 3. Refatoração de polling → eventos (preservando retry/backoff)

### Listener por torrent
Ao adicionar um torrent, `Session.Add` sobe uma goroutine que faz o `select` abaixo e, nos eventos, chama **callbacks injetados** (`OnComplete(hash)`, `OnFailed(hash, err)`) que o `daemon` registra no startup — é o callback (código do `daemon`) que enfileira `JobOrganize`. Assim o pacote `torrents` não importa `daemon`.

```
select {
case <-t.NotifyComplete():   // struct{}, one-shot → enfileira JobOrganize(hash); fim
case err := <-t.NotifyStop():  // error → se err != nil, loga/Notify(DownloadFailed); fim
case <-t.NotifyClose():        // struct{} → torrent removido/sessão encerrada → sai (evita leak)
}
```

`NotifyClose` **precisa** estar no `select` para a goroutine não vazar quando `RemoveTorrent` for chamado.

### `JobOrganize` (novo, idempotente) substitui 3 tipos de job
- Enfileirado por (a) o listener em `NotifyComplete`, (b) a reconciliação de startup.
- Executa: localiza o torrent na sessão por hash; confirma `Seeding`; chama `Librarian.Organize(...)` (hardlink + nome Jellyfin); dispara webhook `DownloadCompleted`.
- **Idempotente:** `Organize` verifica se os links já existem → no-op. Rodar duas vezes é seguro.
- **Retry/backoff preservado:** falha transiente de FS → o job incrementa `Attempts`, aplica `retryBackoff` (inalterado) e persiste. Falha `EXDEV` **não** deve acontecer em runtime porque a sonda no save já rejeitou caminhos cross-device; se acontecer, propaga como erro real (job falha e é reportado), nunca mascarado.

Removidos: `JobNotifyOnComplete` (poll de 30s→10min por conclusão), `JobRenameFile` (poll pela lista de arquivos), `JobMoveToCompleted` (`setLocation` do qBit).

---

## 4. Camada `Librarian` (FileOrganizer)

Nova em `files` (usa o `FileSystem` já injetável). Responsabilidade **do app**, não do rain: pasta por anime, biblioteca de completos e rename Jellyfin.

### Interface e injeção
```go
type Librarian interface {
    // Organize cria hardlinks dos arquivos de vídeo concluídos na biblioteca.
    // Single-episode → nome Jellyfin (se RenameFilesForJellyfin); batch → pasta raw.
    Organize(req OrganizeRequest) error
    // RemoveFromLibrary apaga o hardlink de um episódio na biblioteca.
    RemoveFromLibrary(animeName string, episodeNumber int) error
    // ProbePaths valida em tempo de save que save↔completed estão no mesmo volume.
    ProbePaths(savePath, completedPath string) error
}

type organizer struct {
    fs   FileSystem
    link func(oldname, newname string) error // default os.Link; injetável nos testes
}
```

`OrganizeRequest` carrega: `TorrentDataDir` (raiz `<DataDir>/<id>`), `AnimeName`, `CompletedPath`, `EpisodeNumber *int`, `IsBatch bool`, `RenameJellyfin bool`.

### Regra de hardlink (unificada e obrigatória)
1. `link(origem, destino)` (default `os.Link`).
2. Se `isCrossDevice(err)` → **cancela e retorna o erro** ("esses caminhos precisam estar no mesmo volume"). Sem fallback de copiar/mover.
3. Qualquer outro erro (permissão etc.) → propaga cru, nunca mascarado.

`isCrossDevice`: unix compara com `syscall.EXDEV`; windows com `windows.ERROR_NOT_SAME_DEVICE` (`golang.org/x/sys/windows`) — `errors.Is`.

### Nomeação e caminhos (reuso da lógica atual)
- **Single-episode + `RenameFilesForJellyfin`:** hardlink `<DataDir>/<id>/<arquivo>` → `<CompletedPath>/<AnimeSanitizado>/<AnimeName> - E{NN}{ext}` (reusa `"%s - E%02d"` e `sanitizeFileName`/`sanitizeFolderName` migrados de `torrents.go`).
- **Single-episode sem o flag:** mantém o nome original do arquivo no hardlink.
- **Batch/multi-arquivo:** percorre os arquivos de vídeo do torrent e cria um hardlink **raw** (nome original) por arquivo em `<CompletedPath>/<AnimeSanitizado>/`. **Sem** rename Jellyfin (decisão respondida; mesmo critério do loop automático de hoje).

### Sonda no save (mesma operação do runtime)
`ProbePaths`: cria um arquivo temporário em `SavePath`, tenta `link` dele em `CompletedAnimePath`, apaga ambos. Falhou cross-device → rejeita a config na hora. Usa **a mesma função de link** do runtime, para nunca discordar dela.

### Seeding preservado
O arquivo original **permanece** em `<DataDir>/<id>/...` (seeding intacto). O hardlink aponta para os mesmos bytes — sem duplicar espaço. Consequência: `RemoveTorrent(keepData=false)` apaga a cópia de seeding, mas o hardlink mantém os bytes vivos (por isso a limpeza automática apaga os dois — §5).

---

## 5. Semântica de deleção (apagar ambos os links)

Locais: `deleteEpisodesByStatus` (animes em `delete_statuses`), `handleSavedEpisodes` (assistidos além de `watched_episodes_to_keep`), e o endpoint manual de delete.

Para cada episódio a apagar:
1. `RemoveFromLibrary(animeName, episodeNumber)` — `os.Remove` do hardlink na biblioteca.
2. `session.RemoveTorrent(hash, keepData=false)` — remove a cópia de seeding.

Só quando **ambos** os links somem os bytes são liberados → mantém a intenção original de `watched_episodes_to_keep`.

**Guarda de batch (hash compartilhado):** vários episódios de um batch compartilham o mesmo `EpisodeHash`. Só chamar `RemoveTorrent` quando **nenhum outro** episódio salvo referenciar aquele infohash; caso contrário, apagar apenas o hardlink daquele episódio na biblioteca e deixar o torrent (e os demais arquivos) intactos.

---

## 6. Config, WebUI, CLI, tray

- **`files.Config`:** remove `QBittorrentUrl` e seu default. `SavePath` = DataDir (continua obrigatório). `CompletedAnimePath` passa a obrigatório.
- **`endpoint_config.go` (PUT):** remove a validação de `qbittorrent_url`; adiciona `CompletedAnimePath == ""` → 400; adiciona `Librarian.ProbePaths(SavePath, CompletedAnimePath)` → 400 com mensagem clara em caso de cross-device. Atualiza `endpoint_config_test.go` (remove fixtures de `qbittorrent_url`, adiciona casos de completed-path obrigatório e da sonda).
- **`helpers.go`:** `isConfigComplete` exige `AnilistUsernames` + `SavePath` + `CompletedAnimePath` (troca `QBittorrentUrl`); remove `getQBittorrentURL` e o override `QBITTORRENT_URL` (idem em `endpoint_episode_actions.go`).
- **CLI (`cmd/cli/main.go`):** remove `qbittorrent_url` das chaves de `config set`/`get` e da tabela de exibição.
- **WebUI (`Config.svelte`):** remove a seção "qBittorrent" (campo URL + validação); `completed_anime_path` obrigatório (mensagem de validação). Remove as 4 chaves i18n de qbit em `en.json`/`pt-BR.json`. `client.ts`: remove `qbittorrent_url` da interface `Config`.
- **Notificações:** evento `QBittorrentDownloadCompleted` → `DownloadCompleted`; `ReasonQbitRejected` vira mensagem genérica ("torrent rejeitado").
- **Tray:** sem mudança.
- **Disco (decisão #19):** `handleStatus` continua lendo `files.DiskSpace(cfg.SavePath)` — como `SavePath` = DataDir do rain, os números passam a refletir exatamente o volume dos downloads (some a ressalva de host/container diferente).

---

## 7. Estratégia de testes por camada

**Interface-first:** a rain fica atrás de uma interface `TorrentBackend` (Add/List/Remove/Stats/InfoHash/subscribe), com um **fake** in-memory. Isso substitui o papel do `MockHTTPClient`/mock-server do qBittorrent e mantém todos os testes de lógica de `daemon`/`episodes`/`api` funcionando.

- **Unit (lógica pura):** geração do nome Jellyfin; construção de caminhos; seleção de arquivos no batch; classificação `isCrossDevice` com erros fabricados (EXDEV / ERROR_NOT_SAME_DEVICE vs. outros); parse de infohash a partir do magnet.
- **Integration (`t.TempDir`, `OSFileSystem` real):** caminho feliz do hardlink — o link existe, conteúdo idêntico, e **origem intacta com link count == 2** (prova o seeding preservado).
- **Branch de erro:** injetar `link` que retorna EXDEV → `Organize`/`ProbePaths` cancelam e propagam; nenhum link parcial ou pasta órfã na biblioteca.
- **Orquestração:** `TorrentBackend` fake dispara "complete" → `JobOrganize` enfileirado e executado; reconciliação de startup enfileira para `Seeding`-sem-link; deleção apaga ambos os links; guarda de batch não remove torrent com irmãos vivos.
- **CI em matriz Linux + Windows** (o mapeamento de errno no Windows só se confirma rodando lá) + asserção `CGO_ENABLED=0 go build ./...`.
- **Smoke manual pré-release** para topologias não-simuláveis: cross-volume real, bind mount de Docker, mount de rede.

---

## 8. Plano de migração

- **`qbittorrent_url`:** hard-remove. Configs antigas carregam normalmente (o unmarshal do JSON ignora campos desconhecidos); a chave desaparece no próximo save. Override `QBITTORRENT_URL` removido.
- **Downloads em andamento no qBit: não são importados.** Na primeira execução da versão nova a sessão rain nasce vazia (BoltDB vazio). O check de presença por hash (`episodeInTorrents`) não encontra nada → o loop re-adiciona os **mesmos magnets** → a rain **rebaixa do zero**. Como o infohash é o mesmo, `downloaded_episodes` continua válido assim que a rain tiver os torrents. Migração automática, ao custo de re-download.
- Os arquivos antigos ficavam no layout do qBit (`SavePath/animeName/`), diferente do layout da rain (`<DataDir>/<id>/`), então a rain **não** reaproveita os bytes existentes → re-download completo. Documentar em release note (custo de banda).
- Os torrents na categoria `autoAnimeDownloader` do qBit ficam órfãos; o usuário pode removê-los. Documentar.

---

## 9. Riscos e pontos em aberto (com recomendação)

1. **Sem UPnP/NAT-PMP na rain.** Conectividade de entrada depende de encaminhamento manual. **Rec.:** v1 no range padrão + documentar que forward é opcional; adicionar `torrent_port` (porta fixa) + integrar lib UPnP pure-Go (ex. `huin/goupnp`) como enhancement futuro se houver reclamação de velocidade.
2. **Trocar `SavePath` depois de já haver downloads** orfaniza os dados no diretório antigo e dispara re-download (novo `DataDir`). **Rec.:** aviso na UI; BoltDB fica no dir de config (o resume sobrevive); documentar como operação desencorajada.
3. **Sessão precisa de `SavePath` no startup.** **Rec.:** criação preguiçosa amarrada ao gate `isConfigComplete`; recriar a sessão ao trocar `SavePath`.
4. **Seeding indefinido faz o disco crescer.** A limpeza "apaga ambos os links" cobre os assistidos; os completos ainda não assistidos semeiam até serem assistidos. **Rec.:** aceitável na v1; `StopAfterDownload` (existe na v2) como toggle futuro "não semear".
5. **Re-verificação do resume no boot** re-hasheia peças (pico de CPU/IO) em bibliotecas grandes. **Rec.:** aceitável; documentar.
6. **`CGO_ENABLED=0` com rain** validado por inspeção de deps, não por build real. **Rec.:** asserção no CI (P0).
7. **Feature planejada "standalone download"** (`docs/plans/2026-07-17-standalone-anime-download-design.md`, ainda não implementada) compartilha este seam: `ManualDownloadStandalone` chama `DownloadTorrentWithOptions` e `EnqueueRenameFile`. **Rec.:** quando implementada, deve usar a `Session` injetada e o `Librarian`/`JobOrganize` (não a fila de rename removida). Ponto de integração a alinhar.
8. **Renome do evento de webhook** `QBittorrentDownloadCompleted` → `DownloadCompleted` muda o nome visível a quem configurou webhooks. **Rec.:** aceitar a quebra (o nome antigo cita um componente que deixou de existir); mencionar no release note.

---

## 10. Fases de implementação

- **P0 — Dependência e scaffolding.** `rain/v2` no `go.mod`; CI matriz Linux+Windows + asserção `CGO_ENABLED=0`; `Link` na `FileSystem` + `OSFileSystem` + `MockFileSystem`; helpers `isCrossDevice` (unix/windows).
- **P1 — `Librarian`.** Pacote com nome Jellyfin + caminhos + `Organize` (hardlink) + `ProbePaths`, link injetável. Testes unit/integration/EXDEV completos. Sem wiring.
- **P2 — Cliente embutido.** Reescreve `torrents` como `Session` rain atrás de `TorrentBackend` + fake; Add/List/Remove/Stats-completion/InfoHash; ciclo de vida (create/close, caminho do BoltDB).
- **P3 — Orquestração event-driven.** Listeners por torrent; `JobOrganize` idempotente (remove os 3 tipos de poll); reconciliação de startup; injeta a `Session` única nos 11 sites; semântica de deleção (ambos os links) + guarda de batch.
- **P4 — Config/API/CLI.** Remove `qbittorrent_url`; `CompletedAnimePath` obrigatório; sonda de hardlink no save; remove override env. Atualiza `endpoint_config` + testes.
- **P5 — Frontend + docs.** `Config.svelte` (tira seção qBit, completed obrigatório), i18n, `client.ts`; README/guides/architecture/decisions; `swag init`; remove mock-qbittorrent (server + compose + build.yml).
- **P6 — Migração + release.** Migração de config (ignora `qbittorrent_url`); release notes (re-download, forward manual, remover torrents antigos do qBit); matriz de smoke.

Cada fase encerra com `go test ./...` verde e atualização dos docs afetados (regra do `CLAUDE.md`).
