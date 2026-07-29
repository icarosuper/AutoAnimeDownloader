# Features da rain expostas na WebUI — Design

Levantamento do que o cliente BitTorrent embutido (`github.com/cenkalti/rain/v2`) já oferece e que hoje **não** aparece na WebUI, com custo, ordem de prioridade e as armadilhas de cada item.

Validado contra o fonte da rain na tag **v2.3.1**, pacote `torrent` (caminhos abaixo são relativos ao módulo em `GOMODCACHE`). Continuação de [`2026-07-24-embedded-torrent-client-design.md`](2026-07-24-embedded-torrent-client-design.md).

**Status:** nada implementado. Item 1 (progresso) é o pedido original; o resto é o menu a escolher.

---

## 0. A restrição que molda tudo

`Torrent.Stats()` **não é um getter** — é um round-trip bloqueante por canal para dentro do loop daquele torrent (`torrent/torrent_commands.go:141`). O mesmo vale para `Peers()` (`:263`), `Trackers()` (`:209`) e `Webseeds()` (`:289`).

Consequências que valem para todos os itens:

1. **Uma chamada de `Stats()` por torrent por snapshot.** `toInfo()` em `internal/torrents/session.go:148` já chama `Stats()` e descarta quase tudo — dá para ampliar sem custo algum.
2. **`Peers()`/`Trackers()` nunca entram na rota de listagem.** Só em endpoint de detalhe, sob demanda.
3. **Puxar quando alguém está olhando, não empurrar sempre.** Polling do frontend enquanto a tela está montada custa zero quando ninguém acessa. Se um dia virar push por WebSocket, o ticker precisa ser condicionado a `len(wsm.clients) > 0`, senão martela as goroutines dos torrents para sempre.

E uma restrição de configuração: `SpeedLimitDownload`, `SpeedLimitUpload`, `PortBegin`/`PortEnd`, `DHTEnabled` e `PEXEnabled` são campos de `Config` lidos no `NewSession` (`torrent/config.go:75-102`). Expor qualquer um deles **exige recriar a sessão** — o que `SessionManager.Ensure` já sabe fazer e o `session.db` (fora do `save_path`) torna seguro. Não são setters.

---

## 1. Progresso de download (âncora — pedido original)

Tudo vem de um único `Stats()`, campos verificados em `torrent/torrent_stats.go:12-117`:

| Campo da rain | Uso na UI |
|---|---|
| `Bytes.Completed` / `Bytes.Total` | barra de progresso, `%` |
| `Speed.Download` / `Speed.Upload` | velocidade (média móvel de 1 min) |
| `ETA *time.Duration` | tempo restante |
| `Peers.Total` | conectividade |
| `Status` | estado (enum, ver abaixo) |
| `Pieces.Have` / `Pieces.Total` | detalhe/tooltip |
| `Bytes.Uploaded`, `SeededFor` | ratio de seed (item 6) |

**Plano:** ampliar `TorrentInfo` (`internal/torrents/backend.go`) — não adicionar método paralelo — refletir em `FakeBackend`, expor `GET /api/v1/torrents`, criar rota `#/downloads`, e um card de velocidade global no `Status.svelte` ao lado do de disco.

**Join com anime/episódio:** `TorrentInfo.Hash` ↔ `EpisodeStruct.EpisodeHash`. Um torrent **batch** mapeia para N episódios (`IsBatch`) — deve aparecer **uma vez**, com o nome do anime, não N vezes.

### Armadilhas
- **`Bytes.Total` é 0 até a metadata chegar.** Magnet passa tempo real em `DownloadingMetadata`; a UI precisa de estado próprio e a divisão precisa de guarda.
- **`ETA` é ponteiro; nil = infinito** (`torrent_stats.go:116`, preenchido só quando `Status == Downloading` e a velocidade é > 0). Serializar como `null`, nunca como duração crua.
- **`Status` é enum int.** Tem `String()` (`torrent/torrent_status.go:25`) mas devolve display text com espaço (`"Downloading Metadata"`). Para a API, mapear à mão para slug estável (`downloading_metadata`).
- **`SessionManager.List()` devolve `nil` sem `save_path`** (`sessionmanager.go:94`). O endpoint responde lista vazia, não 500.

---

## 2. Pausar / retomar — **recomendado**

`Torrent.Start()` / `Stop()` (`torrent/session_torrent.go:177,189`). Ambos persistem o estado via `resumer.WriteStarted`, então sobrevive a restart.

Melhor custo-benefício da lista: dois métodos no `TorrentBackend` (+ `FakeBackend`), dois endpoints, dois botões. A tela de progresso praticamente pede isso.

Nota: `Stop()` não bloqueia — o torrent entra em `Stopping` e vira `Stopped` até ~5s depois (announce de stop com timeout). A UI precisa tolerar o estado intermediário.

## 3. Forçar re-announce — **recomendado**

`Torrent.Announce()` (`session_torrent.go:199`). É a saída para o clássico "travado em 0 peers". Barato: um método, um endpoint, um botão. Não sobrepõe o intervalo mínimo dos trackers.

## 4. Recheck (verificar peças)

`Torrent.Verify()` (`session_torrent.go:206`) — apaga o bitfield do bbolt e reverifica os arquivos em disco.

**Armadilha séria:** pela doc na própria função (`:203-205`), o torrent **fica parado depois de verificar**. Se a UI não chamar `Start()` em seguida, o download morre em silêncio. Ou re-inicia automaticamente, ou o texto do botão diz explicitamente que o torrent ficará pausado.

## 5. Progresso por arquivo

`Torrent.FileStats()` (`session_torrent.go:57`) → `[]FileStats{File, BytesCompleted}`; `File.Path()` é relativo, o absoluto é `filepath.Join(t.Dir(), f.Path())`.

Genuinamente útil para batch de anime (ver quais episódios do pacote já fecharam). Encaixa direto no item *"Adicionar modal com opções de torrents de animes / episódios"* do [`TODO.md`](../TODO.md).

Nota: retorna erro quando o torrent não está rodando (`torrent.go:437`) — tratar, não propagar 500.

## 6. Ratio de seed + auto-stop

`Stats.Bytes.Uploaded` e `Stats.SeededFor` existem, então **exibir** ratio é trivial (cai no item 1).

Mas **a rain não tem limite de ratio.** O "parar de semear ao atingir ratio X" seria nosso: comparar no tick e chamar `Stop()`. Relevante porque hoje o app semeia indefinidamente por design (ver `decisions.md #21`) — é a diferença entre isso ser uma escolha e ser um vazamento de banda.

## 7. Stats globais da sessão

`Session.Stats()` → `SessionStats` (`torrent/session_stats.go:12-76`): `SpeedDownload`, `SpeedUpload`, `Uptime`, `Torrents`, `Peers`, `BytesDownloaded`, `BytesUploaded`, além de métricas de cache de leitura/escrita.

Barato e não sofre da restrição do item 0 (lê métricas, não faz round-trip por torrent). Card natural ao lado do de espaço em disco. Também alimentaria o *"Dashboard de estatísticas"* do `TODO.md`.

Requer expor um método novo no `TorrentBackend` (a `Session` da rain está encapsulada em `internal/torrents/session.go`).

## 8. Peers e trackers por torrent — prioridade mais baixa

`Torrent.Peers()` → `[]Peer` com `Client`, `Addr`, `Source` (tracker/DHT/PEX/incoming/manual), `DownloadSpeed`, `UploadSpeed`, flags de choke/snub/encriptação (`torrent_commands.go:224-241`).
`Torrent.Trackers()` → `[]Tracker` com `Status`, `Seeders`, `Leechers`, `LastAnnounce`, `NextAnnounce`, `Error` (`:194-203`).

Só diagnóstico. **Endpoint de detalhe apenas** — ver item 0.

## 9. Adjacentes, com ressalva

Todos exigem recriação da sessão (ver item 0), portanto **não são setters de configuração**:

- **Limite de velocidade** — `SpeedLimitDownload`/`SpeedLimitUpload` em KB/s (`config.go:75-77`).
- **Porta fixa / range** — `PortBegin`/`PortEnd` (`config.go:36-37`). Ficou como enhancement futuro no design anterior. A rain **não tem UPnP/NAT-PMP**.
- **Toggle DHT/PEX** — `DHTEnabled`, `PEXEnabled` (`config.go:41,98`).

E dois que provavelmente não valem: `Torrent.Move()` é transferência inter-sessão via RPC, não relocação em disco; `AddPeer`/`AddTracker` (`session_torrent.go:140,145`) são nicho.

---

## Ordem sugerida

1. **Progresso** (item 1) — o pedido; desbloqueia a tela onde todo o resto mora.
2. **Pausar/retomar + re-announce** (2, 3) — baratos, e a tela de progresso já os pede.
3. **Stats globais** (7) — um card, custo baixo.
4. **Progresso por arquivo** (5) — junto do modal de torrents do `TODO.md`.
5. **Recheck** (4) — só com o "fica parado depois" resolvido.
6. **Ratio + auto-stop** (6) — precisa de decisão de produto (qual ratio? por anime ou global?).
7. **Peers/trackers** (8) — quando houver necessidade real de diagnóstico.

## Ligação com o TODO

Itens do [`TODO.md`](../TODO.md) que este documento cobre parcial ou totalmente:

- *"Adicionar modal com opções de torrents de animes / episódios"* → itens 5, 8.
- *"Dashboard de estatísticas"* → item 7 (complementa os dados que já estão no `downloaded_episodes`).
- *"Torrent imbutido"* → concluído no design anterior; este documento é o desdobramento na UI.
