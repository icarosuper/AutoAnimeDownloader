# Progresso de Download + Pausar/Retomar/Re-announce — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expor na WebUI o progresso dos torrents em andamento (barra, velocidade, ETA, peers, status) e três controles por torrent — pausar, retomar e forçar re-announce.

**Architecture:** Amplia o `TorrentInfo` que já existe em `internal/torrents/backend.go` com os campos do `Stats()` da rain (um único `Stats()` por torrent, sem método paralelo), acrescenta `Pause`/`Resume`/`Announce` à interface `TorrentBackend` (implementados em `Session`, delegados por `SessionManager`, simulados no `FakeBackend`), publica `GET /api/v1/torrents` + três `POST` de ação, e adiciona a rota `#/downloads` no frontend com polling de 2s enquanto a tela está montada. Um card de velocidade global entra no `Status.svelte` somando as velocidades da mesma lista — sem precisar de `Session.Stats()`.

**Tech Stack:** Go 1.22+ (`net/http` com `r.PathValue`), `github.com/cenkalti/rain/v2 v2.3.1`, Svelte 5 + `svelte-spa-router` + TailwindCSS/DaisyUI, Paraglide (i18n), Vitest, Playwright, swag (Swagger).

## Global Constraints

- **Escopo:** itens 1, 2 e 3 do design [`2026-07-26-rain-features-webui-design.md`](2026-07-26-rain-features-webui-design.md). Itens 4 (recheck), 5 (progresso por arquivo), 6 (ratio/auto-stop), 7 (stats globais da sessão) e 8 (peers/trackers) estão **fora** deste plano — não implementar.
- **Uma chamada de `Stats()` por torrent por snapshot.** `Stats()` não é getter: é round-trip bloqueante por canal para dentro do loop do torrent (`torrent/torrent_commands.go:141`). `toInfo` deve guardar o resultado numa variável e ler todos os campos dela.
- **`Peers()`, `Trackers()`, `Webseeds()`, `FileStats()` nunca entram na rota de listagem.** Não usar neste plano.
- **`Status` da rain é enum int** com `String()` que devolve display text com espaço (`"Downloading Metadata"`). A API expõe **slug estável** (`downloading_metadata`), mapeado à mão — nunca `String()`.
- **`ETA` é `*time.Duration`; `nil` significa infinito/desconhecido.** Serializar como `null`, nunca como duração crua ou zero.
- **`Bytes.Total` é 0 até a metadata chegar.** Toda divisão precisa de guarda `> 0`.
- **`SessionManager.List()` devolve `nil` quando não há sessão** (`sessionmanager.go:97`, sem `save_path`). O endpoint responde `[]` com HTTP 200, nunca 500 e nunca `null` no JSON.
- **Convenções obrigatórias** (`docs/agents/conventions.md`): erros com `fmt.Errorf("failed to <ação>: %w", err)`; handlers como closure sobre `*Server` com dispatch por `switch r.Method`; respostas só via `JSONSuccess`/`JSONError`/`JSONInternalError`; tags JSON em snake_case; imports na ordem internos → stdlib → terceiros; anotações swag em todo handler.
- **Sem sentinel errors** — o código é message-driven, não `errors.Is`-driven.
- **Toda i18n vai nos dois arquivos**: `src/internal/frontend/messages/en.json` e `pt-BR.json`. Uma chave só em um dos dois quebra o build do Paraglide.
- **Ao final de cada tarefa:** `go test ./...` verde (e, nas tarefas de frontend, `bun run test:unit` + `bun run check` com 0 errors).

---

## Estrutura de arquivos

| Arquivo | Responsabilidade | Tarefa |
|---|---|---|
| `src/internal/torrents/backend.go` (modificar) | `TorrentInfo` ampliado; `Pause`/`Resume`/`Announce` na interface `TorrentBackend` | 1, 2 |
| `src/internal/torrents/status.go` (criar) | `statusSlug(torrent.Status) string` — tradução do enum da rain para slug de API | 1 |
| `src/internal/torrents/status_test.go` (criar) | Cobertura exaustiva do mapeamento de status | 1 |
| `src/internal/torrents/session.go` (modificar) | `toInfo` com um `Stats()`; `Pause`/`Resume`/`Announce` sobre a rain | 1, 2 |
| `src/internal/torrents/sessionmanager.go` (modificar) | Delegação de `Pause`/`Resume`/`Announce` sob `RLock` | 2 |
| `src/internal/torrents/fakebackend.go` (modificar) | Campos de progresso + `Pause`/`Resume`/`Announce` + drivers de teste | 1, 2 |
| `src/internal/torrents/fakebackend_test.go` (criar) | Comportamento de pause/resume/announce do fake | 2 |
| `src/internal/api/endpoint_torrents.go` (criar) | `GET /api/v1/torrents` + 3 `POST` de ação, com join anime/episódio | 3, 4 |
| `src/internal/api/endpoint_torrents_test.go` (criar) | Testes dos 4 handlers | 3, 4 |
| `src/internal/api/server.go` (modificar) | Registro das 4 rotas | 3, 4 |
| `src/internal/frontend/src/lib/utils/torrents.ts` (criar) | `formatSpeed`, `formatEta`, `formatPercent`, `statusLabelKey` | 5 |
| `src/internal/frontend/tests/unit/torrents.utils.test.ts` (criar) | Testes puros dos formatadores | 5 |
| `src/internal/frontend/src/lib/api/client.ts` (modificar) | `TorrentInfo` + `getTorrents`/`pauseTorrent`/`resumeTorrent`/`announceTorrent` | 5 |
| `src/internal/frontend/src/routes/Downloads.svelte` (criar) | Tela de progresso com os três botões e polling de 2s | 6 |
| `src/internal/frontend/src/App.svelte` (modificar) | Rota `/downloads` | 6 |
| `src/internal/frontend/src/components/Layout.svelte` (modificar) | Link de nav (desktop + mobile) | 6 |
| `src/internal/frontend/messages/{en,pt-BR}.json` (modificar) | Chaves novas de nav, tela e card | 6, 7 |
| `src/internal/frontend/src/routes/Status.svelte` (modificar) | Card de velocidade global | 7 |
| `docs/agents/architecture.md`, `decisions.md`, `docs/swagger/*` (modificar) | Documentação + Swagger regenerado | 8 |

---

## Task 1: `TorrentInfo` com dados de progresso

**Files:**
- Modify: `src/internal/torrents/backend.go:6-16` (struct `TorrentInfo`)
- Create: `src/internal/torrents/status.go`
- Create: `src/internal/torrents/status_test.go`
- Modify: `src/internal/torrents/session.go:150-157` (`toInfo`)
- Modify: `src/internal/torrents/fakebackend.go:33-52,97-117` (`Add`, `AddCompleted`, `CompleteTorrent`)

**Interfaces:**
- Consumes: nada (primeira tarefa).
- Produces: `torrents.TorrentInfo` com os campos `Status string`, `BytesCompleted int64`, `BytesTotal int64`, `BytesUploaded int64`, `DownloadSpeed int`, `UploadSpeed int`, `PeersTotal int`, `PiecesHave uint32`, `PiecesTotal uint32`, `ETASeconds *int64`, `SeededForSeconds int64` — além dos já existentes `Hash`, `Name`, `DataDir`, `Completed`. E `torrents.statusSlug(torrent.Status) string` (não exportado, uso interno do pacote).

- [ ] **Step 1: Escrever o teste do mapeamento de status (falhando)**

Criar `src/internal/torrents/status_test.go`:

```go
package torrents

import (
	"testing"

	"github.com/cenkalti/rain/v2/torrent"
)

func TestStatusSlug(t *testing.T) {
	cases := []struct {
		status torrent.Status
		want   string
	}{
		{torrent.Stopped, "stopped"},
		{torrent.DownloadingMetadata, "downloading_metadata"},
		{torrent.Allocating, "allocating"},
		{torrent.Verifying, "verifying"},
		{torrent.Downloading, "downloading"},
		{torrent.Seeding, "seeding"},
		{torrent.Stopping, "stopping"},
	}

	for _, c := range cases {
		if got := statusSlug(c.status); got != c.want {
			t.Errorf("statusSlug(%v) = %q, want %q", c.status, got, c.want)
		}
	}
}

// A rain pode ganhar estados novos numa atualização. Um status desconhecido tem que virar
// "unknown", nunca string vazia — a UI usa o slug como chave de tradução.
func TestStatusSlugUnknown(t *testing.T) {
	if got := statusSlug(torrent.Status(99)); got != "unknown" {
		t.Errorf("statusSlug(99) = %q, want %q", got, "unknown")
	}
}

// Os slugs são contrato de API: precisam ser únicos, minúsculos e sem espaço, ao contrário
// do Status.String() da rain, que devolve display text ("Downloading Metadata").
func TestStatusSlugsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	all := []torrent.Status{
		torrent.Stopped, torrent.DownloadingMetadata, torrent.Allocating,
		torrent.Verifying, torrent.Downloading, torrent.Seeding, torrent.Stopping,
	}
	for _, s := range all {
		slug := statusSlug(s)
		if seen[slug] {
			t.Errorf("duplicate slug %q for status %v", slug, s)
		}
		seen[slug] = true
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./src/internal/torrents/ -run TestStatusSlug -v`
Expected: FAIL com `undefined: statusSlug`

- [ ] **Step 3: Implementar `statusSlug`**

Criar `src/internal/torrents/status.go`:

```go
package torrents

import "github.com/cenkalti/rain/v2/torrent"

// statusSlug converte o enum de status da rain num slug estável de API.
//
// A rain expõe Status.String(), mas ele devolve display text com espaço
// ("Downloading Metadata") e pode ser reescrito em qualquer upgrade da lib. O slug daqui é
// contrato com a WebUI (chave de tradução), então é mapeado à mão de propósito.
func statusSlug(s torrent.Status) string {
	switch s {
	case torrent.Stopped:
		return "stopped"
	case torrent.DownloadingMetadata:
		return "downloading_metadata"
	case torrent.Allocating:
		return "allocating"
	case torrent.Verifying:
		return "verifying"
	case torrent.Downloading:
		return "downloading"
	case torrent.Seeding:
		return "seeding"
	case torrent.Stopping:
		return "stopping"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./src/internal/torrents/ -run TestStatusSlug -v`
Expected: PASS (3 testes)

- [ ] **Step 5: Ampliar `TorrentInfo`**

Em `src/internal/torrents/backend.go`, substituir a struct `TorrentInfo` por:

```go
// TorrentInfo is a backend-agnostic snapshot of a torrent, exposing what the daemon needs
// (identity, on-disk location, completion) plus the progress data the WebUI renders. It
// replaces the qBittorrent WebUI Torrent struct for the embedded client.
//
// Every field comes from a SINGLE rain Stats() call per torrent — Stats() is a blocking
// round-trip into that torrent's goroutine, not a getter, so a second call per snapshot
// doubles the cost for nothing.
type TorrentInfo struct {
	// Hash is the BitTorrent info hash (lowercase hex, 40 chars) — the join key with
	// downloaded_episodes' EpisodeHash.
	Hash string
	// Name is the torrent's display name (from the magnet / metadata).
	Name string
	// DataDir is the on-disk root of this torrent's content (<DataDir>/<id>).
	DataDir string
	// Completed reports whether all pieces are downloaded (seeding).
	Completed bool
	// Status is the API slug for rain's status enum: stopped, downloading_metadata,
	// allocating, verifying, downloading, seeding, stopping (or unknown).
	Status string
	// BytesCompleted is the number of bytes downloaded and hash-checked.
	BytesCompleted int64
	// BytesTotal is the torrent's total size. It is 0 until the metadata arrives (a magnet
	// spends real time in downloading_metadata) — guard every division on it.
	BytesTotal int64
	// BytesUploaded is the number of bytes uploaded to the swarm.
	BytesUploaded int64
	// DownloadSpeed and UploadSpeed are bytes per second, as a 1-minute moving average.
	DownloadSpeed int
	UploadSpeed   int
	// PeersTotal is the number of connected, handshaked peers.
	PeersTotal int
	// PiecesHave and PiecesTotal back the detail/tooltip view.
	PiecesHave  uint32
	PiecesTotal uint32
	// ETASeconds is the estimated time to completion, in seconds. nil means unknown or
	// infinite — rain only fills ETA while Downloading with a non-zero speed.
	ETASeconds *int64
	// SeededForSeconds is how long the torrent has been in seeding status.
	SeededForSeconds int64
}
```

- [ ] **Step 6: Reescrever `toInfo` com um único `Stats()`**

Em `src/internal/torrents/session.go`, substituir a função `toInfo`:

```go
func toInfo(t *torrent.Torrent) TorrentInfo {
	// Um Stats() só: é round-trip bloqueante para dentro da goroutine do torrent, não getter.
	st := t.Stats()

	var eta *int64
	if st.ETA != nil {
		secs := int64(st.ETA.Seconds())
		eta = &secs
	}

	return TorrentInfo{
		Hash:             t.InfoHash().String(),
		Name:             t.Name(),
		DataDir:          t.Dir(),
		Completed:        st.Status == torrent.Seeding,
		Status:           statusSlug(st.Status),
		BytesCompleted:   st.Bytes.Completed,
		BytesTotal:       st.Bytes.Total,
		BytesUploaded:    st.Bytes.Uploaded,
		DownloadSpeed:    st.Speed.Download,
		UploadSpeed:      st.Speed.Upload,
		PeersTotal:       st.Peers.Total,
		PiecesHave:       st.Pieces.Have,
		PiecesTotal:      st.Pieces.Total,
		ETASeconds:       eta,
		SeededForSeconds: int64(st.SeededFor.Seconds()),
	}
}
```

- [ ] **Step 7: Preencher `Status` no `FakeBackend`**

Em `src/internal/torrents/fakebackend.go`, três edições pontuais para que o fake nunca devolva status vazio (o endpoint e a UI tratam o slug como obrigatório):

Em `Add`, trocar a linha de criação do torrent:

```go
	if _, ok := f.torrents[hash]; !ok {
		f.torrents[hash] = &TorrentInfo{
			Hash:    hash,
			Name:    magnet,
			DataDir: "/fake/" + hash,
			Status:  "downloading",
		}
	}
```

Em `AddCompleted`:

```go
func (f *FakeBackend) AddCompleted(hash, dataDir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrents[hash] = &TorrentInfo{Hash: hash, DataDir: dataDir, Completed: true, Status: "seeding"}
}
```

Em `CompleteTorrent`, dentro do `if t, ok := ...`:

```go
	if t, ok := f.torrents[hash]; ok {
		t.Completed = true
		t.Status = "seeding"
		if dataDir != "" {
			t.DataDir = dataDir
		}
	}
```

- [ ] **Step 8: Rodar a suíte inteira**

Run: `go test ./...`
Expected: PASS em todos os pacotes. `toInfo` e `TorrentInfo` são consumidos por `daemon` e `api`; adicionar campos é aditivo, então nada deve quebrar. Se algo falhar, é sinal de que um teste dependia do zero-value de `Status` — corrigir o teste, não a struct.

- [ ] **Step 9: Commit**

```bash
git add src/internal/torrents/backend.go src/internal/torrents/status.go src/internal/torrents/status_test.go src/internal/torrents/session.go src/internal/torrents/fakebackend.go
git commit -m "feat(torrents): expor dados de progresso no TorrentInfo"
```

---

## Task 2: `Pause` / `Resume` / `Announce` no `TorrentBackend`

**Files:**
- Modify: `src/internal/torrents/backend.go` (interface `TorrentBackend`)
- Modify: `src/internal/torrents/session.go` (após `Remove`, linha ~93)
- Modify: `src/internal/torrents/sessionmanager.go` (após `Remove`, linha ~123)
- Modify: `src/internal/torrents/fakebackend.go`
- Create: `src/internal/torrents/fakebackend_test.go`

**Interfaces:**
- Consumes: `TorrentInfo` da Task 1.
- Produces: três métodos novos em `torrents.TorrentBackend` — `Pause(hash string) error`, `Resume(hash string) error`, `Announce(hash string) error`. Todos devolvem erro quando o hash não está na sessão. No `FakeBackend`, `Resume` põe `Status = "downloading"`, `Pause` põe `Status = "stopped"`, e `AnnounceCalls() []string` expõe o histórico de chamadas para asserção nos testes.

> **A armadilha central desta tarefa:** o listener por torrent (`armListener`, `session.go:125`) é uma goroutine one-shot que faz `select` em `NotifyComplete` / `NotifyStop` / `NotifyClose`. Um `Stop()` limpo faz `NotifyStop` emitir `nil` — o `select` casa, o `if err != nil` não dispara callback nenhum, e **a goroutine morre**. Sem re-armar no `Resume`, o torrent retomado completa em silêncio: `JobOrganize` nunca é enfileirado e o episódio nunca chega à biblioteca. `Resume` **tem** que chamar `armListener`.

- [ ] **Step 1: Escrever os testes do fake (falhando)**

Criar `src/internal/torrents/fakebackend_test.go`:

```go
package torrents

import "testing"

const testMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
const testHash = "0123456789abcdef0123456789abcdef01234567"

func TestFakeBackendPauseSetsStoppedStatus(t *testing.T) {
	f := NewFakeBackend()
	hash, err := f.Add(testMagnet)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if err := f.Pause(hash); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}

	info, ok := f.Get(hash)
	if !ok {
		t.Fatal("torrent disappeared after Pause")
	}
	if info.Status != "stopped" {
		t.Errorf("Status = %q, want %q", info.Status, "stopped")
	}
}

func TestFakeBackendResumeSetsDownloadingStatus(t *testing.T) {
	f := NewFakeBackend()
	hash, _ := f.Add(testMagnet)
	_ = f.Pause(hash)

	if err := f.Resume(hash); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	info, _ := f.Get(hash)
	if info.Status != "downloading" {
		t.Errorf("Status = %q, want %q", info.Status, "downloading")
	}
}

func TestFakeBackendAnnounceRecordsCall(t *testing.T) {
	f := NewFakeBackend()
	hash, _ := f.Add(testMagnet)

	if err := f.Announce(hash); err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	calls := f.AnnounceCalls()
	if len(calls) != 1 || calls[0] != hash {
		t.Errorf("AnnounceCalls() = %v, want [%s]", calls, hash)
	}
}

func TestFakeBackendActionsOnUnknownHashError(t *testing.T) {
	f := NewFakeBackend()

	if err := f.Pause("deadbeef"); err == nil {
		t.Error("Pause on unknown hash should error")
	}
	if err := f.Resume("deadbeef"); err == nil {
		t.Error("Resume on unknown hash should error")
	}
	if err := f.Announce("deadbeef"); err == nil {
		t.Error("Announce on unknown hash should error")
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `go test ./src/internal/torrents/ -run TestFakeBackend -v`
Expected: FAIL com `f.Pause undefined (type *FakeBackend has no field or method Pause)`

- [ ] **Step 3: Declarar os métodos na interface**

Em `src/internal/torrents/backend.go`, dentro de `type TorrentBackend interface`, depois de `Remove`:

```go
	// Pause stops a torrent (rain's Torrent.Stop). It does not block: the torrent enters
	// "stopping" and only reaches "stopped" up to ~5s later, after the stop event reaches
	// the trackers. rain persists the paused state, so it survives a restart.
	Pause(hash string) error
	// Resume starts a paused torrent (rain's Torrent.Start) and re-arms its completion
	// listener — pausing consumes the one-shot NotifyStop, which would otherwise leave the
	// resumed torrent completing silently.
	Resume(hash string) error
	// Announce forces a re-announce to all trackers and DHT. It does not override the
	// trackers' minimum interval, so calling it in a loop achieves nothing.
	Announce(hash string) error
```

- [ ] **Step 4: Implementar no `Session`**

Em `src/internal/torrents/session.go`, logo após `Remove`:

```go
func (s *Session) Pause(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	if err := t.Stop(); err != nil {
		return fmt.Errorf("failed to pause torrent %s: %w", hash, err)
	}
	logger.Logger.Info().Str("hash", hash).Msg("Paused torrent")
	return nil
}

func (s *Session) Resume(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to resume torrent %s: %w", hash, err)
	}
	// Pausing made NotifyStop fire (with a nil error), which ended the one-shot listener
	// goroutine armed by Add/SetCallbacks. Without re-arming, this torrent would finish
	// downloading without ever enqueuing JobOrganize — the episode would never be
	// hardlinked into the library. Re-arming may leave two listeners on a
	// pause/resume/pause/resume torrent; that is safe, because JobOrganize is idempotent.
	s.armListener(t)
	logger.Logger.Info().Str("hash", hash).Msg("Resumed torrent")
	return nil
}

func (s *Session) Announce(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	// rain's Announce returns nothing — it queues the announce and returns immediately.
	t.Announce()
	logger.Logger.Info().Str("hash", hash).Msg("Forced torrent re-announce")
	return nil
}
```

- [ ] **Step 5: Implementar a delegação no `SessionManager`**

Em `src/internal/torrents/sessionmanager.go`, logo após `Remove` (mesmo padrão de `RLock` + `ErrSessionNotReady` usado por `Add`):

```go
func (m *SessionManager) Pause(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Pause(hash)
}

func (m *SessionManager) Resume(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Resume(hash)
}

func (m *SessionManager) Announce(hash string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ErrSessionNotReady
	}
	return m.session.Announce(hash)
}
```

- [ ] **Step 6: Implementar no `FakeBackend`**

Em `src/internal/torrents/fakebackend.go`, adicionar o campo de histórico na struct (junto de `NextHash`):

```go
	// announceCalls records every Announce(hash) for assertions.
	announceCalls []string
```

E, após `Remove`:

```go
func (f *FakeBackend) Pause(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	t.Status = "stopped"
	return nil
}

func (f *FakeBackend) Resume(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	t.Status = "downloading"
	return nil
}

func (f *FakeBackend) Announce(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.torrents[hash]; !ok {
		return fmt.Errorf("fake: torrent %s not found", hash)
	}
	f.announceCalls = append(f.announceCalls, hash)
	return nil
}

// AnnounceCalls returns the hashes passed to Announce, in order.
func (f *FakeBackend) AnnounceCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.announceCalls...)
}
```

- [ ] **Step 7: Rodar os testes do pacote**

Run: `go test ./src/internal/torrents/ -v`
Expected: PASS. O `var _ TorrentBackend = (*FakeBackend)(nil)` em `fakebackend.go:24` é a rede de segurança — se algum método faltar, o pacote nem compila.

- [ ] **Step 8: Rodar a suíte inteira**

Run: `go test ./...`
Expected: PASS. Qualquer outro fake/stub de `TorrentBackend` fora do pacote apareceria aqui como erro de compilação; se aparecer, adicionar os três métodos lá também.

- [ ] **Step 9: Commit**

```bash
git add src/internal/torrents/
git commit -m "feat(torrents): adicionar Pause/Resume/Announce ao TorrentBackend"
```

---

## Task 3: `GET /api/v1/torrents`

**Files:**
- Create: `src/internal/api/endpoint_torrents.go`
- Create: `src/internal/api/endpoint_torrents_test.go`
- Modify: `src/internal/api/server.go:97` (registro de rota, após a linha de `notifications`)

**Interfaces:**
- Consumes: `torrents.TorrentInfo` (Task 1); `server.Torrents` (`torrents.TorrentBackend`) e `server.FileManager.LoadSavedEpisodes()`, ambos já existentes no `Server`.
- Produces: `api.TorrentResponse` com as tags JSON `hash`, `name`, `status`, `anime_name`, `anime_id`, `episode_number`, `is_batch`, `bytes_completed`, `bytes_total`, `bytes_uploaded`, `progress`, `download_speed`, `upload_speed`, `peers_total`, `eta_seconds`, `seeded_for_seconds`. A rota `GET /api/v1/torrents` devolve `SuccessResponse{data=[]TorrentResponse}`.

- [ ] **Step 1: Escrever os testes (falhando)**

Criar `src/internal/api/endpoint_torrents_test.go`:

```go
package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const magnetA = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
const hashA = "0123456789abcdef0123456789abcdef01234567"

func decodeTorrentList(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("Expected success=true")
	}
	raw, ok := response.Data.([]interface{})
	if !ok {
		t.Fatalf("Expected data to be an array, got %T", response.Data)
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]interface{}))
	}
	return out
}

func TestHandleTorrentsRejectsNonGet(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()

	handleTorrents(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// Sem save_path o SessionManager devolve nil em List(). A resposta tem que ser uma lista
// vazia com HTTP 200 — nunca 500 e nunca "data": null.
func TestHandleTorrentsEmptyBackendReturnsEmptyArray(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()

	handleTorrents(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if items := decodeTorrentList(t, w); len(items) != 0 {
		t.Errorf("Expected 0 torrents, got %d", len(items))
	}
}

func TestHandleTorrentsJoinsEpisodeMetadata(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{EpisodeID: 1, AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 7},
	}}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent, got %d", len(items))
	}
	if items[0]["anime_name"] != "Frieren" {
		t.Errorf("anime_name = %v, want Frieren", items[0]["anime_name"])
	}
	if items[0]["episode_number"] != float64(7) {
		t.Errorf("episode_number = %v, want 7", items[0]["episode_number"])
	}
	if items[0]["is_batch"] != false {
		t.Errorf("is_batch = %v, want false", items[0]["is_batch"])
	}
}

// Um torrent batch cobre N episódios mas é UM torrent: tem que aparecer uma vez só, sem
// número de episódio.
func TestHandleTorrentsBatchAppearsOnceWithoutEpisodeNumber(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{EpisodeID: 1, AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 1, IsBatch: true},
		{EpisodeID: 2, AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 2, IsBatch: true},
		{EpisodeID: 3, AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 3, IsBatch: true},
	}}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent for a batch, got %d", len(items))
	}
	if items[0]["is_batch"] != true {
		t.Errorf("is_batch = %v, want true", items[0]["is_batch"])
	}
	if items[0]["episode_number"] != nil {
		t.Errorf("episode_number = %v, want null for a batch", items[0]["episode_number"])
	}
}

// bytes_total é 0 enquanto a metadata não chega: progress tem que ser 0, não NaN nem
// divisão por zero.
func TestHandleTorrentsProgressIsZeroWithoutMetadata(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if items[0]["progress"] != float64(0) {
		t.Errorf("progress = %v, want 0", items[0]["progress"])
	}
	if items[0]["eta_seconds"] != nil {
		t.Errorf("eta_seconds = %v, want null", items[0]["eta_seconds"])
	}
}

// Uma falha ao ler downloaded_episodes não pode derrubar a tela de progresso: o join é
// opcional, os dados do torrent não.
func TestHandleTorrentsSurvivesEpisodeLoadFailure(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	fm := &mockFileManager{loadEpisodesErr: errTestLoadEpisodes}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent, got %d", len(items))
	}
	if items[0]["anime_name"] != nil && items[0]["anime_name"] != "" {
		t.Errorf("anime_name = %v, want empty when the join failed", items[0]["anime_name"])
	}
}
```

No topo do arquivo, declarar o erro usado no último teste:

```go
var errTestLoadEpisodes = errors.New("boom")
```

(e acrescentar `"errors"` ao bloco de imports).

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `go test ./src/internal/api/ -run TestHandleTorrents -v`
Expected: FAIL com `undefined: handleTorrents`

- [ ] **Step 3: Implementar o handler**

Criar `src/internal/api/endpoint_torrents.go`:

```go
package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"

	"net/http"
	"sort"
)

// TorrentResponse is one row of the downloads screen: a torrent's live progress joined with
// the anime/episode it belongs to. A batch torrent covers several episodes but is a single
// torrent, so it appears once, with EpisodeNumber nil and IsBatch true.
type TorrentResponse struct {
	Hash   string `json:"hash" example:"0123456789abcdef0123456789abcdef01234567"`
	Name   string `json:"name" example:"[SubsPlease] Frieren - 07 (1080p).mkv"`
	Status string `json:"status" example:"downloading"`

	AnimeName string `json:"anime_name,omitempty" example:"Sousou no Frieren"`
	AnimeID   int    `json:"anime_id,omitempty" example:"154587"`
	// EpisodeNumber is null for batch torrents (they map to several episodes).
	EpisodeNumber *int `json:"episode_number"`
	IsBatch       bool `json:"is_batch" example:"false"`

	BytesCompleted int64 `json:"bytes_completed" example:"524288000"`
	// BytesTotal is 0 until the torrent's metadata arrives.
	BytesTotal    int64 `json:"bytes_total" example:"1073741824"`
	BytesUploaded int64 `json:"bytes_uploaded" example:"104857600"`
	// Progress is 0..1, and 0 while BytesTotal is unknown.
	Progress      float64 `json:"progress" example:"0.48"`
	DownloadSpeed int     `json:"download_speed" example:"2097152"`
	UploadSpeed   int     `json:"upload_speed" example:"524288"`
	PeersTotal    int     `json:"peers_total" example:"14"`
	// EtaSeconds is null when unknown or infinite.
	EtaSeconds       *int64 `json:"eta_seconds" example:"240"`
	SeededForSeconds int64  `json:"seeded_for_seconds" example:"3600"`
}

// @Summary      List torrents
// @Description  Returns a live snapshot of every torrent in the embedded client — progress, speed, ETA, peers and status — joined with the anime/episode it belongs to. Batch torrents appear once, with a null episode_number. Responds with an empty list (not an error) when no session exists yet, i.e. before save_path is configured.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]TorrentResponse}
// @Failure      405  {object}  SuccessResponse
// @Router       /torrents [get]
func handleTorrents(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		// List returns nil when there is no session yet (no save_path configured). That is a
		// normal, empty state — not an error.
		list := server.Torrents.List()

		// The anime/episode join is best-effort: without it the screen still shows progress,
		// just with the raw torrent name. Failing the whole request over it would blank the
		// downloads screen on an unrelated JSONL problem.
		byHash := map[string][]files.EpisodeStruct{}
		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to load saved episodes for torrent list; returning torrents without anime metadata")
		} else {
			for _, ep := range episodes {
				byHash[ep.EpisodeHash] = append(byHash[ep.EpisodeHash], ep)
			}
		}

		out := make([]TorrentResponse, 0, len(list))
		for _, t := range list {
			out = append(out, buildTorrentResponse(t, byHash[t.Hash]))
		}

		// Deterministic order: unfinished torrents first (that is what the user opened the
		// screen for), then alphabetical.
		sort.SliceStable(out, func(i, j int) bool {
			iDone := out[i].Status == "seeding"
			jDone := out[j].Status == "seeding"
			if iDone != jDone {
				return !iDone
			}
			return out[i].Name < out[j].Name
		})

		JSONSuccess(w, http.StatusOK, out)
	}
}

// buildTorrentResponse merges a torrent snapshot with the episodes that share its info hash.
func buildTorrentResponse(t torrents.TorrentInfo, eps []files.EpisodeStruct) TorrentResponse {
	resp := TorrentResponse{
		Hash:             t.Hash,
		Name:             t.Name,
		Status:           t.Status,
		BytesCompleted:   t.BytesCompleted,
		BytesTotal:       t.BytesTotal,
		BytesUploaded:    t.BytesUploaded,
		DownloadSpeed:    t.DownloadSpeed,
		UploadSpeed:      t.UploadSpeed,
		PeersTotal:       t.PeersTotal,
		EtaSeconds:       t.ETASeconds,
		SeededForSeconds: t.SeededForSeconds,
	}

	if t.BytesTotal > 0 {
		resp.Progress = float64(t.BytesCompleted) / float64(t.BytesTotal)
	}

	if len(eps) == 0 {
		return resp
	}

	resp.AnimeName = eps[0].AnimeName
	resp.AnimeID = eps[0].AnimeID
	// More than one episode on the same info hash means a batch, whatever the flag says.
	resp.IsBatch = eps[0].IsBatch || len(eps) > 1
	if !resp.IsBatch {
		n := eps[0].EpisodeNumber
		resp.EpisodeNumber = &n
	}
	return resp
}
```

- [ ] **Step 4: Registrar a rota**

Em `src/internal/api/server.go`, dentro de `SetupRoutes`, após a linha `apiMux.HandleFunc("/api/v1/logs", handleLogs(s))`:

```go
	apiMux.HandleFunc("/api/v1/torrents", handleTorrents(s))
```

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `go test ./src/internal/api/ -run TestHandleTorrents -v`
Expected: PASS (6 testes)

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add src/internal/api/endpoint_torrents.go src/internal/api/endpoint_torrents_test.go src/internal/api/server.go
git commit -m "feat(api): adicionar GET /api/v1/torrents com progresso e join de episodio"
```

---

## Task 4: Endpoints de ação (pause / resume / announce)

**Files:**
- Modify: `src/internal/api/endpoint_torrents.go` (acrescentar handlers)
- Modify: `src/internal/api/endpoint_torrents_test.go` (acrescentar testes)
- Modify: `src/internal/api/server.go` (3 rotas novas)

**Interfaces:**
- Consumes: `TorrentBackend.Pause/Resume/Announce` (Task 2); `handleTorrents` (Task 3).
- Produces: `POST /api/v1/torrents/{hash}/pause`, `/resume` e `/announce`, todas devolvendo `SuccessResponse` com `data: null`. Hash ausente da sessão → 404 com código `TORRENT_NOT_FOUND`.

- [ ] **Step 1: Escrever os testes (falhando)**

Acrescentar ao final de `src/internal/api/endpoint_torrents_test.go`:

```go
func newTorrentActionServer(t *testing.T) (*Server, *torrents.FakeBackend) {
	t.Helper()
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	return &Server{Torrents: backend, FileManager: &mockFileManager{}}, backend
}

func postTorrentAction(handler http.HandlerFunc, hash string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/"+hash+"/action", nil)
	req.SetPathValue("hash", hash)
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleTorrentPause(t *testing.T) {
	server, backend := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentPause(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	info, _ := backend.Get(hashA)
	if info.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", info.Status)
	}
}

func TestHandleTorrentResume(t *testing.T) {
	server, backend := newTorrentActionServer(t)
	_ = backend.Pause(hashA)

	w := postTorrentAction(handleTorrentResume(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	info, _ := backend.Get(hashA)
	if info.Status != "downloading" {
		t.Errorf("Status = %q, want downloading", info.Status)
	}
}

func TestHandleTorrentAnnounce(t *testing.T) {
	server, backend := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentAnnounce(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if calls := backend.AnnounceCalls(); len(calls) != 1 || calls[0] != hashA {
		t.Errorf("AnnounceCalls() = %v, want [%s]", calls, hashA)
	}
}

func TestHandleTorrentActionsUnknownHashReturn404(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	handlers := map[string]http.HandlerFunc{
		"pause":    handleTorrentPause(server),
		"resume":   handleTorrentResume(server),
		"announce": handleTorrentAnnounce(server),
	}
	for name, handler := range handlers {
		w := postTorrentAction(handler, "ffffffffffffffffffffffffffffffffffffffff")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected %d, got %d", name, http.StatusNotFound, w.Code)
		}
	}
}

func TestHandleTorrentActionsRejectNonPost(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/"+hashA+"/pause", nil)
	req.SetPathValue("hash", hashA)
	w := httptest.NewRecorder()
	handleTorrentPause(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleTorrentActionsRejectEmptyHash(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentPause(server), "")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `go test ./src/internal/api/ -run TestHandleTorrent -v`
Expected: FAIL com `undefined: handleTorrentPause`

- [ ] **Step 3: Implementar os handlers**

Acrescentar ao final de `src/internal/api/endpoint_torrents.go`:

```go
// torrentAction is the shared shape of the three per-torrent controls: POST only, hash from
// the path, 404 when the torrent is not in the session, and the backend call last.
//
// The action takes the *Server rather than a bound method value: SetupRoutes runs inside
// Server.Start, and binding server.Torrents.Pause at registration time would freeze whatever
// backend was set then. Resolving it per request keeps the handler honest if the field is
// assigned later (as tests do).
func torrentAction(server *Server, action func(s *Server, hash string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		hash := r.PathValue("hash")
		if hash == "" {
			JSONError(w, http.StatusBadRequest, "INVALID_HASH", "Torrent hash is required")
			return
		}

		// Checked up front so a missing torrent is a clean 404 instead of a 500 built from
		// the backend's error string — the codebase has no sentinel errors to match on.
		if _, ok := server.Torrents.Get(hash); !ok {
			JSONError(w, http.StatusNotFound, "TORRENT_NOT_FOUND", "Torrent not found")
			return
		}

		if err := action(server, hash); err != nil {
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, nil)
	}
}

// @Summary      Pause a torrent
// @Description  Stops a torrent. Does not block: the torrent enters the "stopping" state and only reaches "stopped" up to ~5s later, once the stop event reaches the trackers. The paused state is persisted and survives a daemon restart.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/pause [post]
func handleTorrentPause(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Pause(hash) })
}

// @Summary      Resume a torrent
// @Description  Restarts a paused torrent and re-arms its completion listener.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/resume [post]
func handleTorrentResume(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Resume(hash) })
}

// @Summary      Force a torrent re-announce
// @Description  Re-announces the torrent to all trackers and DHT — the way out of "stuck at 0 peers". It does not override the trackers' minimum interval, so repeated calls have no extra effect.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/announce [post]
func handleTorrentAnnounce(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Announce(hash) })
}
```

- [ ] **Step 4: Registrar as rotas**

Em `src/internal/api/server.go`, logo abaixo da rota de `/api/v1/torrents` criada na Task 3:

```go
	apiMux.HandleFunc("/api/v1/torrents/{hash}/pause", handleTorrentPause(s))
	apiMux.HandleFunc("/api/v1/torrents/{hash}/resume", handleTorrentResume(s))
	apiMux.HandleFunc("/api/v1/torrents/{hash}/announce", handleTorrentAnnounce(s))
```

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `go test ./src/internal/api/ -run TestHandleTorrent -v`
Expected: PASS (todos os testes de Task 3 + Task 4)

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add src/internal/api/
git commit -m "feat(api): adicionar acoes de pause, resume e announce por torrent"
```

---

## Task 5: Cliente e formatadores no frontend

**Files:**
- Create: `src/internal/frontend/src/lib/utils/torrents.ts`
- Create: `src/internal/frontend/tests/unit/torrents.utils.test.ts`
- Modify: `src/internal/frontend/src/lib/api/client.ts` (interface + 4 funções)

**Interfaces:**
- Consumes: o JSON de `TorrentResponse` (Task 3) e as rotas de ação (Task 4).
- Produces: `TorrentInfo` (interface TS), `getTorrents()`, `pauseTorrent(hash)`, `resumeTorrent(hash)`, `announceTorrent(hash)` em `client.ts`; `formatSpeed(bytesPerSecond)`, `formatEta(seconds)`, `formatPercent(progress)`, `totalSpeeds(torrents)` em `lib/utils/torrents.ts`.

> Todos os comandos desta tarefa rodam a partir de `src/internal/frontend/`.

- [ ] **Step 1: Escrever os testes dos formatadores (falhando)**

Criar `src/internal/frontend/tests/unit/torrents.utils.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { formatSpeed, formatEta, formatPercent, totalSpeeds } from '../../src/lib/utils/torrents'
import type { TorrentInfo } from '../../src/lib/api/client'

describe('formatSpeed', () => {
  it('formats zero as an idle dash', () => {
    expect(formatSpeed(0)).toBe('—')
  })

  it('formats bytes per second', () => {
    expect(formatSpeed(512)).toBe('512 B/s')
  })

  it('formats kilobytes and megabytes with one decimal', () => {
    expect(formatSpeed(1536)).toBe('1.5 KB/s')
    expect(formatSpeed(2 * 1024 * 1024)).toBe('2.0 MB/s')
  })
})

describe('formatEta', () => {
  // ETA nulo = infinito/desconhecido. A rain só preenche ETA enquanto baixa com velocidade
  // maior que zero, então null é o caso comum, não a exceção.
  it('renders null as a dash', () => {
    expect(formatEta(null)).toBe('—')
  })

  it('renders seconds under a minute', () => {
    expect(formatEta(45)).toBe('45s')
  })

  it('renders minutes and seconds', () => {
    expect(formatEta(150)).toBe('2m 30s')
  })

  it('renders hours and minutes', () => {
    expect(formatEta(7380)).toBe('2h 3m')
  })

  it('renders days for very long ETAs', () => {
    expect(formatEta(180000)).toBe('2d 2h')
  })
})

describe('formatPercent', () => {
  it('renders a fraction as a whole percentage', () => {
    expect(formatPercent(0.4567)).toBe('46%')
  })

  it('renders zero progress', () => {
    expect(formatPercent(0)).toBe('0%')
  })

  it('renders full progress', () => {
    expect(formatPercent(1)).toBe('100%')
  })
})

describe('totalSpeeds', () => {
  const torrent = (download: number, upload: number): TorrentInfo => ({
    hash: 'h' + download,
    name: 'n',
    status: 'downloading',
    episode_number: null,
    is_batch: false,
    bytes_completed: 0,
    bytes_total: 0,
    bytes_uploaded: 0,
    progress: 0,
    download_speed: download,
    upload_speed: upload,
    peers_total: 0,
    eta_seconds: null,
    seeded_for_seconds: 0,
  })

  it('sums download and upload across torrents', () => {
    expect(totalSpeeds([torrent(100, 10), torrent(250, 40)])).toEqual({ download: 350, upload: 50 })
  })

  it('returns zeros for an empty list', () => {
    expect(totalSpeeds([])).toEqual({ download: 0, upload: 0 })
  })
})
```

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `bun run test:unit`
Expected: FAIL — o módulo `src/lib/utils/torrents` não existe.

- [ ] **Step 3: Acrescentar tipos e chamadas ao `client.ts`**

Em `src/internal/frontend/src/lib/api/client.ts`, depois da interface `LogsResponse` (linha ~132):

```ts
export interface TorrentInfo {
  hash: string
  name: string
  status: string
  anime_name?: string
  anime_id?: number
  episode_number: number | null
  is_batch: boolean
  bytes_completed: number
  bytes_total: number
  bytes_uploaded: number
  progress: number
  download_speed: number
  upload_speed: number
  peers_total: number
  eta_seconds: number | null
  seeded_for_seconds: number
}
```

E, junto das demais funções exportadas (por exemplo depois de `getLogs`):

```ts
export async function getTorrents(): Promise<TorrentInfo[]> {
  return apiRequest<TorrentInfo[]>('GET', '/torrents')
}

export async function pauseTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/pause`)
}

export async function resumeTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/resume`)
}

export async function announceTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/announce`)
}
```

- [ ] **Step 4: Implementar os formatadores**

Criar `src/internal/frontend/src/lib/utils/torrents.ts`:

```ts
import type { TorrentInfo } from '../api/client.js'

const IDLE = '—'

/** Formats a byte-per-second rate. Zero renders as a dash: an idle torrent has no speed. */
export function formatSpeed(bytesPerSecond: number): string {
  if (!bytesPerSecond || bytesPerSecond <= 0) return IDLE
  if (bytesPerSecond < 1024) return `${bytesPerSecond} B/s`
  const units = ['KB/s', 'MB/s', 'GB/s']
  let value = bytesPerSecond / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

/**
 * Formats a remaining time in seconds. null means infinite/unknown — rain only fills ETA
 * while the torrent is downloading at a non-zero speed, so null is the common case.
 */
export function formatEta(seconds: number | null): string {
  if (seconds === null || seconds === undefined || seconds < 0) return IDLE
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60)
    return `${minutes}m ${seconds % 60}s`
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600)
    return `${hours}h ${Math.floor((seconds % 3600) / 60)}m`
  }
  const days = Math.floor(seconds / 86400)
  return `${days}d ${Math.floor((seconds % 86400) / 3600)}h`
}

/** Formats a 0..1 progress fraction as a whole percentage. */
export function formatPercent(progress: number): string {
  return `${Math.round((progress || 0) * 100)}%`
}

/** Sums per-torrent speeds into the session-wide rates shown on the status screen. */
export function totalSpeeds(torrents: TorrentInfo[]): { download: number; upload: number } {
  return torrents.reduce(
    (acc, t) => ({ download: acc.download + t.download_speed, upload: acc.upload + t.upload_speed }),
    { download: 0, upload: 0 },
  )
}
```

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `bun run test:unit`
Expected: PASS — os 4 arquivos anteriores mais `torrents.utils.test.ts`.

- [ ] **Step 6: Verificar tipos**

Run: `bun run check`
Expected: `0 ERRORS` (os 19 warnings de a11y são pré-existentes e não contam).

- [ ] **Step 7: Commit**

```bash
git add src/internal/frontend/src/lib/utils/torrents.ts src/internal/frontend/tests/unit/torrents.utils.test.ts src/internal/frontend/src/lib/api/client.ts
git commit -m "feat(frontend): adicionar cliente e formatadores de torrents"
```

---

## Task 6: Tela `#/downloads`

**Files:**
- Create: `src/internal/frontend/src/routes/Downloads.svelte`
- Modify: `src/internal/frontend/src/App.svelte:1-20`
- Modify: `src/internal/frontend/src/components/Layout.svelte` (nav desktop ~linha 66 e mobile ~linha 168)
- Modify: `src/internal/frontend/messages/en.json`, `src/internal/frontend/messages/pt-BR.json`

**Interfaces:**
- Consumes: `getTorrents`, `pauseTorrent`, `resumeTorrent`, `announceTorrent`, `TorrentInfo` (Task 5); `formatSpeed`, `formatEta`, `formatPercent` (Task 5).
- Produces: rota `/downloads`; chaves i18n `nav_downloads`, `downloads_*`.

> Comandos a partir de `src/internal/frontend/`.

- [ ] **Step 1: Adicionar as chaves de i18n em inglês**

Em `src/internal/frontend/messages/en.json`, acrescentar (mantendo o JSON válido — vírgula na entrada anterior):

```json
  "nav_downloads": "Downloads",
  "downloads_title": "Downloads",
  "downloads_subtitle": "Live progress of the embedded torrent client",
  "downloads_empty_title": "No active torrents",
  "downloads_empty_desc": "Torrents appear here while they download and keep seeding after they finish.",
  "downloads_col_name": "Torrent",
  "downloads_col_progress": "Progress",
  "downloads_col_speed": "Speed",
  "downloads_col_eta": "ETA",
  "downloads_col_peers": "Peers",
  "downloads_col_actions": "Actions",
  "downloads_pause": "Pause",
  "downloads_resume": "Resume",
  "downloads_announce": "Re-announce",
  "downloads_batch": "Batch",
  "downloads_episode": "Episode {number}",
  "downloads_uploaded": "{size} uploaded",
  "downloads_status_stopped": "Paused",
  "downloads_status_downloading_metadata": "Fetching metadata",
  "downloads_status_allocating": "Allocating",
  "downloads_status_verifying": "Verifying",
  "downloads_status_downloading": "Downloading",
  "downloads_status_seeding": "Seeding",
  "downloads_status_stopping": "Pausing",
  "downloads_status_unknown": "Unknown",
  "status_card_speed": "Speed",
  "status_speed_up": "{speed} up"
```

- [ ] **Step 2: Adicionar as mesmas chaves em português**

Em `src/internal/frontend/messages/pt-BR.json`:

```json
  "nav_downloads": "Downloads",
  "downloads_title": "Downloads",
  "downloads_subtitle": "Progresso ao vivo do cliente de torrent embutido",
  "downloads_empty_title": "Nenhum torrent ativo",
  "downloads_empty_desc": "Os torrents aparecem aqui enquanto baixam e continuam semeando depois de concluídos.",
  "downloads_col_name": "Torrent",
  "downloads_col_progress": "Progresso",
  "downloads_col_speed": "Velocidade",
  "downloads_col_eta": "Tempo restante",
  "downloads_col_peers": "Peers",
  "downloads_col_actions": "Ações",
  "downloads_pause": "Pausar",
  "downloads_resume": "Retomar",
  "downloads_announce": "Re-announce",
  "downloads_batch": "Batch",
  "downloads_episode": "Episódio {number}",
  "downloads_uploaded": "{size} enviados",
  "downloads_status_stopped": "Pausado",
  "downloads_status_downloading_metadata": "Buscando metadata",
  "downloads_status_allocating": "Alocando",
  "downloads_status_verifying": "Verificando",
  "downloads_status_downloading": "Baixando",
  "downloads_status_seeding": "Semeando",
  "downloads_status_stopping": "Pausando",
  "downloads_status_unknown": "Desconhecido",
  "status_card_speed": "Velocidade",
  "status_speed_up": "{speed} de envio"
```

- [ ] **Step 3: Compilar as mensagens e confirmar que as chaves existem**

Run: `bun run i18n:compile`
Expected: exit 0, sem aviso de chave faltando em algum idioma.

- [ ] **Step 4: Criar a tela**

Criar `src/internal/frontend/src/routes/Downloads.svelte`:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    getTorrents,
    pauseTorrent,
    resumeTorrent,
    announceTorrent,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { formatSpeed, formatEta, formatPercent } from "../lib/utils/torrents.js";
  import { formatBytes } from "../lib/utils/status.js";
  import Loading from "../components/Loading.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.downloads_title(),
    subtitle: m.downloads_subtitle(),
    emptyTitle: m.downloads_empty_title(),
    emptyDesc: m.downloads_empty_desc(),
    colName: m.downloads_col_name(),
    colProgress: m.downloads_col_progress(),
    colSpeed: m.downloads_col_speed(),
    colEta: m.downloads_col_eta(),
    colPeers: m.downloads_col_peers(),
    colActions: m.downloads_col_actions(),
    pause: m.downloads_pause(),
    resume: m.downloads_resume(),
    announce: m.downloads_announce(),
    batch: m.downloads_batch(),
  };

  let torrents: TorrentInfo[] = [];
  let loading = true;
  // Hashes com ação em voo: desabilitam os botões daquela linha sem congelar a tabela toda.
  let busy = new Set<string>();

  // O slug de status vem do backend; o mapa é exaustivo com statusSlug() do Go.
  function statusLabel(status: string): string {
    switch (status) {
      case "stopped": return m.downloads_status_stopped();
      case "downloading_metadata": return m.downloads_status_downloading_metadata();
      case "allocating": return m.downloads_status_allocating();
      case "verifying": return m.downloads_status_verifying();
      case "downloading": return m.downloads_status_downloading();
      case "seeding": return m.downloads_status_seeding();
      case "stopping": return m.downloads_status_stopping();
      default: return m.downloads_status_unknown();
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case "seeding": return "badge-success";
      case "downloading": return "badge-info";
      case "stopped": return "badge-ghost";
      case "stopping": return "badge-warning";
      default: return "badge-neutral";
    }
  }

  // "stopping" dura até ~5s depois do Stop(); durante ele nem pausar nem retomar faz sentido.
  function canPause(t: TorrentInfo): boolean {
    return t.status !== "stopped" && t.status !== "stopping";
  }
  function canResume(t: TorrentInfo): boolean {
    return t.status === "stopped";
  }

  async function load() {
    try {
      torrents = await getTorrents();
    } catch (err) {
      console.error("Failed to load torrents:", err);
    } finally {
      loading = false;
    }
  }

  async function runAction(hash: string, action: (h: string) => Promise<void>) {
    busy = new Set(busy).add(hash);
    try {
      await action(hash);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      const next = new Set(busy);
      next.delete(hash);
      busy = next;
    }
  }

  let pollInterval: ReturnType<typeof setInterval> | null = null;

  onMount(() => {
    load();
    // Polling só enquanto a tela está montada: cada snapshot custa um Stats() por torrent,
    // que é round-trip bloqueante para dentro da goroutine de cada um.
    pollInterval = setInterval(load, 2000);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-base-content">{T && T.title}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{T && T.subtitle}</p>
  </div>

  {#if loading}
    <Loading message="Loading torrents..." />
  {:else if torrents.length === 0}
    <div class="card bg-base-200 border border-base-300">
      <div class="card-body items-center text-center py-12">
        <h2 class="text-lg font-medium text-base-content">{T && T.emptyTitle}</h2>
        <p class="text-sm text-base-content/50">{T && T.emptyDesc}</p>
      </div>
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{T && T.colName}</th>
            <th>{T && T.colProgress}</th>
            <th>{T && T.colSpeed}</th>
            <th>{T && T.colEta}</th>
            <th>{T && T.colPeers}</th>
            <th class="text-right">{T && T.colActions}</th>
          </tr>
        </thead>
        <tbody>
          {#each torrents as t (t.hash)}
            <tr>
              <td class="max-w-xs">
                <div class="font-medium text-base-content truncate" title={t.name}>
                  {t.anime_name || t.name}
                </div>
                <div class="flex items-center gap-2 mt-0.5">
                  <span class="badge badge-xs {statusClass(t.status)}">{$locale && statusLabel(t.status)}</span>
                  {#if t.is_batch}
                    <span class="text-xs text-base-content/40">{T && T.batch}</span>
                  {:else if t.episode_number !== null}
                    <span class="text-xs text-base-content/40">
                      {$locale && m.downloads_episode({ number: t.episode_number })}
                    </span>
                  {/if}
                </div>
              </td>
              <td class="min-w-[10rem]">
                <!-- bytes_total fica em 0 até a metadata chegar; a barra some nesse intervalo -->
                {#if t.bytes_total > 0}
                  <progress class="progress progress-primary w-full" value={t.progress} max="1"></progress>
                  <div class="text-xs text-base-content/40 mt-0.5">
                    {formatPercent(t.progress)} · {formatBytes(t.bytes_completed)} / {formatBytes(t.bytes_total)}
                  </div>
                {:else}
                  <progress class="progress w-full"></progress>
                  <div class="text-xs text-base-content/40 mt-0.5">—</div>
                {/if}
                <div class="text-xs text-base-content/30">
                  {$locale && m.downloads_uploaded({ size: formatBytes(t.bytes_uploaded) })}
                </div>
              </td>
              <td class="whitespace-nowrap text-sm">
                <div>↓ {formatSpeed(t.download_speed)}</div>
                <div class="text-base-content/40">↑ {formatSpeed(t.upload_speed)}</div>
              </td>
              <td class="whitespace-nowrap text-sm">{formatEta(t.eta_seconds)}</td>
              <td class="text-sm">{t.peers_total}</td>
              <td>
                <div class="flex justify-end gap-1">
                  {#if canResume(t)}
                    <button
                      class="btn btn-xs"
                      disabled={busy.has(t.hash)}
                      on:click={() => runAction(t.hash, resumeTorrent)}
                    >
                      {T && T.resume}
                    </button>
                  {:else}
                    <button
                      class="btn btn-xs"
                      disabled={busy.has(t.hash) || !canPause(t)}
                      on:click={() => runAction(t.hash, pauseTorrent)}
                    >
                      {T && T.pause}
                    </button>
                  {/if}
                  <button
                    class="btn btn-xs btn-ghost"
                    disabled={busy.has(t.hash)}
                    on:click={() => runAction(t.hash, announceTorrent)}
                  >
                    {T && T.announce}
                  </button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

- [ ] **Step 5: Registrar a rota**

Em `src/internal/frontend/src/App.svelte`, acrescentar o import e a entrada de rota:

```svelte
  import Downloads from "./routes/Downloads.svelte";
```

```svelte
    "/downloads": Downloads,
```

(colocar a entrada logo depois de `"/status/:id": AnimeDetail,`)

- [ ] **Step 6: Adicionar o link na navegação**

Em `src/internal/frontend/src/components/Layout.svelte`:

Acrescentar ao objeto `T` (após `navStatus`):

```svelte
    navDownloads: m.nav_downloads(),
```

No nav **desktop**, entre o link de `#/status` e o de `#/config`:

```svelte
          <a
            href="#/downloads"
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/downloads'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            {T && T.navDownloads}
          </a>
```

No menu **mobile**, entre os mesmos dois links:

```svelte
          <a
            href="#/downloads"
            class="block px-3 py-2 rounded-md text-base font-medium transition-colors {currentPath === '/downloads'
              ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
              : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}"
          >
            {T && T.navDownloads}
          </a>
```

- [ ] **Step 7: Verificar tipos e build**

Run: `bun run check`
Expected: `0 ERRORS`

Run: `bun run build`
Expected: exit 0

- [ ] **Step 8: Rodar os testes de frontend**

Run: `bun run test:unit && bun run test:component`
Expected: PASS nos dois

- [ ] **Step 9: Commit**

```bash
git add src/internal/frontend/src/routes/Downloads.svelte src/internal/frontend/src/App.svelte src/internal/frontend/src/components/Layout.svelte src/internal/frontend/messages/
git commit -m "feat(frontend): adicionar tela de downloads com progresso e controles"
```

---

## Task 7: Card de velocidade global no Status

**Files:**
- Modify: `src/internal/frontend/src/routes/Status.svelte` (imports ~linha 20, estado ~linha 54, `loadInitialData` ~linha 98, `onMount`/`onDestroy` ~linha 187, grid de cards ~linha 214)

**Interfaces:**
- Consumes: `getTorrents`, `TorrentInfo` (Task 5); `totalSpeeds`, `formatSpeed` (Task 5); chaves `status_card_speed` e `status_speed_up` (Task 6).
- Produces: nada consumido por tarefas posteriores.

> Comandos a partir de `src/internal/frontend/`.

- [ ] **Step 1: Importar o que falta**

Em `src/internal/frontend/src/routes/Status.svelte`, acrescentar `getTorrents` e `TorrentInfo` ao import de `client.js` (linha 3-12) e um import novo:

```svelte
  import { totalSpeeds, formatSpeed } from "../lib/utils/torrents.js";
```

- [ ] **Step 2: Acrescentar estado e reatividade**

Depois de `let animes: AnimeInfo[] = [];` (linha ~55):

```svelte
  let torrents: TorrentInfo[] = [];
```

Depois de `$: diskSpaceLow = ...` (linha ~75):

```svelte
  $: speeds = totalSpeeds(torrents);
  $: hasTraffic = speeds.download > 0 || speeds.upload > 0;
```

E a função de carga, ao lado de `loadAnimes`:

```svelte
  async function loadTorrents() {
    try {
      torrents = await getTorrents();
    } catch (err) {
      console.error("Failed to load torrents:", err);
    }
  }
```

- [ ] **Step 3: Adicionar a chave de tradução ao objeto `T`**

No objeto `$: T = $locale && { ... }`, junto de `cardDisk`:

```svelte
    cardSpeed: m.status_card_speed(),
```

- [ ] **Step 4: Ligar o polling**

Em `onMount` (linha ~187), depois de `loadInitialData();`:

```svelte
    loadTorrents();
    torrentsPollInterval = setInterval(loadTorrents, 5000);
```

Declarar o handle junto dos demais intervalos (linha ~87):

```svelte
  let torrentsPollInterval: ReturnType<typeof setInterval> | null = null;
```

E limpar em `onDestroy`:

```svelte
    if (torrentsPollInterval) clearInterval(torrentsPollInterval);
```

- [ ] **Step 5: Adicionar o card e abrir espaço no grid**

Trocar a classe do grid (linha ~214) de `lg:grid-cols-5` para `lg:grid-cols-6`:

```svelte
    <div class="grid grid-cols-2 lg:grid-cols-6 gap-3">
```

E, logo depois do bloco `<!-- Disk space -->`, acrescentar:

```svelte
      <!-- Global speed -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardSpeed}</span>
          <span class="text-base font-medium {hasTraffic ? 'text-base-content' : 'text-base-content/40'}">
            ↓ {formatSpeed(speeds.download)}
          </span>
          <span class="text-xs text-base-content/40">
            {$locale && m.status_speed_up({ speed: formatSpeed(speeds.upload) })}
          </span>
        </div>
      </div>
```

- [ ] **Step 6: Verificar tipos, build e testes**

Run: `bun run check`
Expected: `0 ERRORS`

Run: `bun run build`
Expected: exit 0

Run: `bun run test:unit && bun run test:component`
Expected: PASS

- [ ] **Step 7: Rodar os smoke tests**

Run: `bun run test:smoke`
Expected: PASS. O `status.spec.ts` renderiza a tela de status; se ele afirmar um número fixo de cards ou a classe `lg:grid-cols-5`, atualizar a asserção para 6 colunas — o teste está descrevendo o layout antigo, não um bug.

- [ ] **Step 8: Commit**

```bash
git add src/internal/frontend/src/routes/Status.svelte
git commit -m "feat(frontend): adicionar card de velocidade global no status"
```

---

## Task 8: Documentação e Swagger

**Files:**
- Modify: `docs/agents/architecture.md` (tabela de endpoints da API; tabela de símbolos de `torrents`)
- Modify: `docs/agents/decisions.md` (entrada nova #30)
- Modify: `docs/TODO.md`
- Modify: `docs/swagger/docs.go`, `swagger.json`, `swagger.yaml` (regenerados)

**Interfaces:**
- Consumes: tudo das tarefas 1–7.
- Produces: nada.

- [ ] **Step 1: Instalar o `swag` se necessário**

Run: `which swag || go install github.com/swaggo/swag/cmd/swag@latest`
Expected: caminho do binário. Se o `go install` for usado, confirmar que `$(go env GOPATH)/bin` está no `PATH`.

- [ ] **Step 2: Regenerar o Swagger**

Run: `swag init -g src/cmd/daemon/main.go -o docs/swagger`
Expected: exit 0, e `git diff --stat docs/swagger/` mostrando as 4 rotas novas (`/torrents`, `/torrents/{hash}/pause`, `/torrents/{hash}/resume`, `/torrents/{hash}/announce`).

- [ ] **Step 3: Atualizar `architecture.md`**

Na tabela de endpoints da API, acrescentar as 4 linhas:

```markdown
| `GET /api/v1/torrents` | Live snapshot of every torrent (progress, speed, ETA, peers, status) joined with its anime/episode. Batch torrents appear once, `episode_number: null`. Empty list when no session exists yet |
| `POST /api/v1/torrents/{hash}/pause` | Stops a torrent (rain `Torrent.Stop`). Non-blocking: the torrent sits in `stopping` for up to ~5s |
| `POST /api/v1/torrents/{hash}/resume` | Restarts a paused torrent and re-arms its completion listener |
| `POST /api/v1/torrents/{hash}/announce` | Forces a tracker/DHT re-announce; does not override the trackers' minimum interval |
```

Na tabela de símbolos do pacote `torrents`, acrescentar:

```markdown
| `TorrentBackend.Pause/Resume/Announce(hash)` | Per-torrent controls. `Resume` re-arms the completion listener that pausing consumed |
| `statusSlug(torrent.Status)` | Maps rain's status enum to the stable API slug (`downloading_metadata`, …) — never `Status.String()`, which is display text |
```

E, na descrição de `TorrentInfo`, registrar que ele agora carrega os campos de progresso vindos de um único `Stats()`.

- [ ] **Step 4: Acrescentar a decisão #30**

Ao final de `docs/agents/decisions.md`:

```markdown
### 30. Progress data comes from one `Stats()` per torrent, pulled only while a screen is open

**Location:** `internal/torrents/session.go` (`toInfo`); `internal/api/endpoint_torrents.go`; `frontend/src/routes/Downloads.svelte`.

**What it looks like:** `TorrentInfo` carries a dozen progress fields filled from a single
`t.Stats()` call, and the WebUI polls `GET /api/v1/torrents` every 2s while the downloads
screen is mounted — instead of the more familiar "push updates over the existing WebSocket".

**Why it's right:** rain's `Stats()` is not a getter — it is a blocking round-trip into that
torrent's goroutine (`torrent/torrent_commands.go:141`), and so are `Peers()`, `Trackers()`
and `Webseeds()`. Reading fields one at a time, or on a server-side ticker, would hammer
every torrent's goroutine forever, including when nobody is looking. Pulling on demand costs
exactly zero when no screen is open. The same reasoning keeps `Peers()`/`Trackers()` out of
the list route entirely.

**Don't "fix" by:** adding a parallel `Stats()`-per-field accessor; moving the poll to a
server-side ticker or the WebSocket without gating it on `len(wsm.clients) > 0`; or calling
`Peers()`/`Trackers()`/`FileStats()` from the list endpoint.

**Related:** the API exposes a hand-written status slug (`statusSlug`) rather than rain's
`Status.String()`, which returns display text with a space (`"Downloading Metadata"`) and can
be reworded by any library upgrade. And `Session.Resume` re-arms the per-torrent listener,
because pausing makes the one-shot `NotifyStop` fire and kills the goroutine — without the
re-arm a resumed torrent would complete without ever enqueuing `JobOrganize`.
```

- [ ] **Step 5: Atualizar o `TODO.md`**

Em `docs/TODO.md`, remover a linha `- Torrent imbutido` (concluída no design anterior) e acrescentar, sob o item do modal de torrents, uma nota de que progresso e pause/resume/announce já existem — restam progresso por arquivo (item 5) e peers/trackers (item 8).

- [ ] **Step 6: Verificação final completa**

Run: `go build ./... && go test ./...`
Expected: exit 0 e PASS em todos os pacotes

Run: `CGO_ENABLED=0 go build ./...`
Expected: exit 0 (a asserção que o CI faz)

Run (a partir de `src/internal/frontend/`): `bun run check && bun run build && bun run test:unit && bun run test:component`
Expected: `0 ERRORS` e PASS

- [ ] **Step 7: Commit**

```bash
git add docs/
git commit -m "docs: documentar endpoints de torrents e a decisao de polling de progresso"
```

---

## Verificação manual antes de fechar

Coisas que teste automatizado não cobre — rodar o daemon e conferir na UI:

1. **Magnet novo:** a linha aparece em `downloading_metadata`, sem barra de progresso, com `—` no ETA. Quando a metadata chega, a barra aparece e `bytes_total` deixa de ser 0.
2. **Pausar:** o status vira `stopping` e só depois (até ~5s) `stopped`. Durante `stopping` os dois botões ficam desabilitados.
3. **Retomar e deixar concluir:** o episódio tem que chegar à biblioteca (`completed_anime_path`). É a prova de que o re-arm do listener no `Resume` funciona — sem ele o download termina e nada é hardlinkado.
4. **Re-announce em torrent com 0 peers:** o contador de peers deve mexer (ou não, se os trackers estiverem no intervalo mínimo — o log confirma que a chamada saiu).
5. **Sem `save_path` configurado:** `#/downloads` mostra o estado vazio, não um erro.
