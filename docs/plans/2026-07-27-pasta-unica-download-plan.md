# Pasta única de download — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remover `save_path` da configuração do usuário, derivando o diretório de download de `<completed_anime_path>/.autoAnimeDownloader`, com migração que move os dados existentes sem quebrar seed nem hardlinks.

**Architecture:** `Config.SavePath` vira campo legado lido só pela migração; um novo `Config.DownloadPath()` deriva o caminho a cada uso. Uma função idempotente `MigrateSavePath` roda no boot e no topo do passe de verificação, lista os diretórios de torrent abrindo uma sessão temporária no caminho antigo e os move com `rename` (mesmo volume garantido, então inode e hardlinks sobrevivem). Três vedações impedem que `save_path` seja reintroduzido: o `PUT /config` zera o campo, o entrypoint do Docker para de emitir, e os testes de integração param de mandar.

**Tech Stack:** Go 1.x (stdlib + `github.com/cenkalti/rain/v2`), Svelte + TypeScript (frontend), Playwright (smoke), Vitest (unit), `swag` (Swagger).

**Spec:** [`2026-07-27-pasta-unica-download-design.md`](2026-07-27-pasta-unica-download-design.md)

## Global Constraints

- **NUNCA commitar ou dar push sem permissão explícita do usuário** (`CLAUDE.md`). Os passos de commit deste plano dizem "peça permissão e então commite" — o comando está pronto, mas a execução depende do usuário.
- Rodar `go test ./...` a partir da raiz do repo depois de cada mudança em Go.
- Nome exato do diretório derivado: `.autoAnimeDownloader` (com o ponto, camelCase igual ao diretório de config do usuário).
- Mensagens de UI existem em **dois** arquivos, sempre em par: `src/internal/frontend/messages/en.json` e `src/internal/frontend/messages/pt-BR.json`.
- Se a API mudar, regerar o Swagger: `swag init -g src/cmd/daemon/main.go -o docs/swagger`.
- Padrão de teste do repo para hardlink/inode: `NewOSFileSystem()` sobre `t.TempDir()` — um `FileSystem` falso não preserva inode. Ver `TestRemoveEpisodesAndLinks_RealHardlinks` em `src/internal/daemon/orchestration_test.go:330`.

---

## Estrutura de arquivos

| Arquivo | Responsabilidade | Ação |
|---|---|---|
| `src/internal/files/filemanager.go` | `Config.DownloadPath()`, `SavePath` legado, `DeleteEmptyFolders` | Modificar |
| `src/internal/files/config_test.go` | Testes de `DownloadPath()` | Criar |
| `src/internal/files/filemanager_test.go` | Testes de `DeleteEmptyFolders` | Criar |
| `src/internal/files/librarian.go` | `ProbePath` (era `ProbePaths`), criação do `.ignore` | Modificar |
| `src/internal/files/librarian_test.go` | Testes de `ProbePath` | Modificar |
| `src/internal/torrents/fakebackend.go` | Registrar chamadas de `Ensure` | Modificar |
| `src/internal/daemon/migration.go` | `MigrateSavePath` | Criar |
| `src/internal/daemon/migration_test.go` | Testes da migração | Criar |
| `src/internal/daemon/helpers.go` | `isConfigComplete`, `FileManagerInterface` | Modificar |
| `src/internal/daemon/verification.go` | Chamada da migração, `ProbePath`, `Ensure`, `DeleteEmptyFolders` | Modificar |
| `src/internal/daemon/manual_download.go` | 3 chamadas de `Ensure` | Modificar |
| `src/internal/daemon/episodes_test.go` | Assinatura do mock `DeleteEmptyFolders` | Modificar |
| `src/cmd/daemon/main.go` | Chamada da migração, `ensureStartupSession` | Modificar |
| `src/internal/api/server.go` | `FileManagerInterface` | Modificar |
| `src/internal/api/endpoint_config.go` | Validação removida, zeragem de `SavePath`, `ProbePath` | Modificar |
| `src/internal/api/endpoint_config_test.go` | Testes das novas regras | Modificar |
| `src/internal/api/endpoint_status.go` | Disco medido na biblioteca | Modificar |
| `docker/entrypoint.sh` | Para de emitir `save_path` | Modificar |
| `docker/docker-compose.test.yml` | `TEST_SAVE_PATH` removida | Modificar |
| `src/tests/integration/integration_test.go` | Para de mandar `save_path` | Modificar |
| `src/internal/frontend/src/routes/Config.svelte` | Campo removido, hint derivado | Modificar |
| `src/internal/frontend/src/lib/api/client.ts` | Tipo `Config` sem `save_path` | Modificar |
| `src/internal/frontend/messages/{en,pt-BR}.json` | Chaves removidas, hint novo | Modificar |
| `src/internal/frontend/tests/smoke/helpers.ts` | `baseConfig` sem `save_path` | Modificar |
| `docs/agents/{decisions,architecture,config,environment}.md` | Documentação | Modificar |
| `docs/swagger/*` | Regerado | Modificar |

---

## Task 1: `Config.DownloadPath()` e `SavePath` legado

**Files:**
- Modify: `src/internal/files/filemanager.go:55` (campo `SavePath`) e após a struct `Config` (novo método)
- Test: `src/internal/files/config_test.go` (criar)

**Interfaces:**
- Consumes: nada.
- Produces: `func (c *Config) DownloadPath() string` — retorna `""` quando `c.CompletedAnimePath == ""`, senão `filepath.Join(c.CompletedAnimePath, ".autoAnimeDownloader")`. Todas as tasks seguintes dependem dela.

- [ ] **Step 1: Escreva o teste que falha**

Criar `src/internal/files/config_test.go`:

```go
package files

import (
	"path/filepath"
	"testing"
)

func TestConfigDownloadPath(t *testing.T) {
	t.Run("deriva o diretorio de download da biblioteca", func(t *testing.T) {
		c := &Config{CompletedAnimePath: filepath.Join("/mnt", "hd", "animes")}
		want := filepath.Join("/mnt", "hd", "animes", ".autoAnimeDownloader")
		if got := c.DownloadPath(); got != want {
			t.Errorf("DownloadPath() = %q, quero %q", got, want)
		}
	})

	// Guarda critica: sem ela filepath.Join devolveria o caminho relativo
	// ".autoAnimeDownloader" e a sessao da rain seria criada no diretorio de
	// trabalho do processo. Com "", SessionManager.Ensure devolve
	// ErrSessionNotReady, que e o comportamento atual para config incompleta.
	t.Run("biblioteca vazia devolve string vazia", func(t *testing.T) {
		c := &Config{CompletedAnimePath: ""}
		if got := c.DownloadPath(); got != "" {
			t.Errorf("DownloadPath() = %q, quero string vazia", got)
		}
	})
}
```

- [ ] **Step 2: Rode o teste e confirme que falha**

Run: `go test ./src/internal/files/ -run TestConfigDownloadPath -v`
Expected: FAIL com `c.DownloadPath undefined (type *Config has no field or method DownloadPath)`

- [ ] **Step 3: Implemente**

Em `src/internal/files/filemanager.go`, trocar a linha do campo `SavePath` (hoje `SavePath string \`json:"save_path"\``) por:

```go
	// SavePath e um campo LEGADO, lido apenas por daemon.MigrateSavePath. O diretorio de
	// download deixou de ser configuravel e passou a ser derivado (ver DownloadPath). O
	// omitempty faz o campo sumir do config.json assim que a migracao o zera.
	SavePath string `json:"save_path,omitempty"`
```

E adicionar, logo depois do fechamento da struct `Config`:

```go
// downloadDirName e o nome do diretorio de download dentro da biblioteca. O ponto o
// esconde do scanner do Jellyfin no Linux; o arquivo .ignore criado por
// Librarian.ProbePath cobre as demais plataformas.
const downloadDirName = ".autoAnimeDownloader"

// DownloadPath e o diretorio onde os torrents baixam e continuam semeando. Ele e derivado
// de CompletedAnimePath, nunca armazenado: assim a restricao de hardlink (origem e destino
// no mesmo filesystem) fica impossivel de violar por configuracao.
//
// Devolve "" quando a biblioteca nao esta configurada. Essa guarda e obrigatoria: sem ela
// filepath.Join produziria o caminho relativo ".autoAnimeDownloader" e a sessao da rain
// seria criada no diretorio de trabalho do processo. Com "", SessionManager.Ensure devolve
// ErrSessionNotReady, que e o comportamento atual para config incompleta.
func (c *Config) DownloadPath() string {
	if c.CompletedAnimePath == "" {
		return ""
	}
	return filepath.Join(c.CompletedAnimePath, downloadDirName)
}
```

Confirme que `path/filepath` já está nos imports do arquivo (está — `filemanager.go` já usa `filepath.Join`).

- [ ] **Step 4: Rode o teste e confirme que passa**

Run: `go test ./src/internal/files/ -run TestConfigDownloadPath -v`
Expected: PASS (dois subtestes)

- [ ] **Step 5: Rode a suíte inteira**

Run: `go test ./...`
Expected: PASS. Nada quebra — o método é aditivo e o `omitempty` não muda leitura.

- [ ] **Step 6: Peça permissão ao usuário e então commite**

```bash
git add src/internal/files/filemanager.go src/internal/files/config_test.go
git commit -m "feat(config): derivar diretorio de download da biblioteca"
```

---

## Task 2: `Librarian.ProbePath` substitui `ProbePaths`

**Files:**
- Modify: `src/internal/files/librarian.go:28-31` (interface), `:197-229` (implementação)
- Modify: `src/internal/daemon/verification.go:53-62` (chamador)
- Modify: `src/internal/api/endpoint_config.go:98-104` (chamador)
- Test: `src/internal/files/librarian_test.go`

**Interfaces:**
- Consumes: `Config.DownloadPath()` da Task 1 — usada indiretamente pelos chamadores, que passam `configs.CompletedAnimePath`.
- Produces: `ProbePath(completedPath string) error` na interface `files.Librarian`. Substitui `ProbePaths(savePath, completedPath string) error`, que deixa de existir.

- [ ] **Step 1: Escreva o teste que falha**

Adicionar ao fim de `src/internal/files/librarian_test.go`:

```go
func TestProbePath(t *testing.T) {
	t.Run("cria biblioteca, diretorio de download e .ignore", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("ProbePath: %v", err)
		}

		downloadDir := filepath.Join(completed, ".autoAnimeDownloader")
		if _, err := os.Stat(downloadDir); err != nil {
			t.Errorf("diretorio de download nao foi criado: %v", err)
		}
		if _, err := os.Stat(filepath.Join(downloadDir, ".ignore")); err != nil {
			t.Errorf(".ignore nao foi criado: %v", err)
		}
	})

	t.Run("nao deixa arquivos de sonda para tras", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("ProbePath: %v", err)
		}

		for _, p := range []string{
			filepath.Join(completed, ".aad_link_probe"),
			filepath.Join(completed, ".autoAnimeDownloader", ".aad_link_probe"),
		} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("sobrou arquivo de sonda em %s", p)
			}
		}
	})

	t.Run("rejeita biblioteca vazia", func(t *testing.T) {
		lib := NewLibrarian(NewOSFileSystem())
		if err := lib.ProbePath(""); err == nil {
			t.Error("quero erro para caminho vazio, veio nil")
		}
	})

	t.Run("e idempotente", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("primeira chamada: %v", err)
		}
		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("segunda chamada: %v", err)
		}
	})
}
```

- [ ] **Step 2: Rode o teste e confirme que falha**

Run: `go test ./src/internal/files/ -run TestProbePath -v`
Expected: FAIL com `lib.ProbePath undefined`

- [ ] **Step 3: Implemente `ProbePath`**

Em `src/internal/files/librarian.go`, trocar a declaração na interface `Librarian` (linhas 28-31):

```go
	// ProbePath valida, no save da config e a cada passe de verificacao, que a biblioteca
	// suporta hardlinks. O cheque de volume cruzado deixou de ser necessario (o diretorio
	// de download e derivado da biblioteca, entao estao sempre no mesmo filesystem), mas
	// existem filesystems sem hardlink nenhum: exFAT, FAT32, alguns mounts SMB/NFS. Usa a
	// mesma funcao de link que Organize usa, entao nunca discorda dele. Tambem cria o
	// diretorio de download e o marcador .ignore.
	ProbePath(completedPath string) error
```

E trocar todo o corpo de `ProbePaths` (linhas 197-229) por:

```go
func (o *organizer) ProbePath(completedPath string) error {
	if completedPath == "" {
		return fmt.Errorf("completed anime path must be set")
	}
	if err := o.fs.MkdirAll(completedPath, 0755); err != nil {
		return fmt.Errorf("cannot access completed path %s: %w", completedPath, err)
	}

	downloadPath := filepath.Join(completedPath, downloadDirName)
	if err := o.fs.MkdirAll(downloadPath, 0755); err != nil {
		return fmt.Errorf("cannot create download folder %s: %w", downloadPath, err)
	}

	// O prefixo com ponto esconde a pasta do scanner do Jellyfin no Linux; o .ignore cobre
	// as plataformas onde o ponto nao marca oculto. As duas defesas juntas, porque a
	// pasta de download agora vive dentro da pasta que o Jellyfin varre.
	ignorePath := filepath.Join(downloadPath, ".ignore")
	if _, err := o.fs.Stat(ignorePath); err != nil {
		if err := o.fs.WriteFile(ignorePath, nil, 0644); err != nil {
			return fmt.Errorf("cannot write ignore marker %s: %w", ignorePath, err)
		}
	}

	probeSrc := filepath.Join(downloadPath, ".aad_link_probe")
	probeDst := filepath.Join(completedPath, ".aad_link_probe")

	// Limpa sobras de uma sonda anterior.
	_ = o.fs.Remove(probeSrc)
	_ = o.fs.Remove(probeDst)

	if err := o.fs.WriteFile(probeSrc, []byte("probe"), 0644); err != nil {
		return fmt.Errorf("cannot write to download path %s: %w", downloadPath, err)
	}
	defer func() { _ = o.fs.Remove(probeSrc) }()

	if err := o.link(probeSrc, probeDst); err != nil {
		return fmt.Errorf("this filesystem does not support hardlinks, which the library requires: %w", err)
	}
	_ = o.fs.Remove(probeDst)

	return nil
}
```

Nota: `isCrossDevice` deixa de ser usado aqui. Se ele não tiver outro chamador, o compilador de testes não reclama (é função de pacote), mas `Organize` (`librarian.go:172`) continua usando — deixe como está.

- [ ] **Step 4: Atualize os dois chamadores**

Em `src/internal/daemon/verification.go`, trocar o bloco das linhas 46-62 (comentário + `if librarian != nil`) por:

```go
	// A biblioteca e montada com hardlinks. O endpoint de save da config sonda isso, mas
	// configs escritos antes deste upgrade (ou direto no config.json pelo
	// docker/entrypoint.sh) nunca passaram por ele. Sem esta porta um filesystem sem
	// suporte a hardlink baixa alegremente enquanto todo JobOrganize morre, e a UI mostra
	// um daemon saudavel. Sondar aqui devolve a mesma mensagem acionavel do endpoint, e
	// aborta o passe: baixar o que nao da para organizar so enche o disco.
	if librarian != nil {
		if err := librarian.ProbePath(configs.CompletedAnimePath); err != nil {
			logger.Logger.Error().Err(err).
				Str("completed_anime_path", configs.CompletedAnimePath).
				Msg("Completed anime path failed the hardlink probe; skipping verification")
			state.SetLastCheckError(err)
			return
		}
	}
```

Em `src/internal/api/endpoint_config.go`, trocar o bloco das linhas 96-104 por:

```go
		// A biblioteca e montada com hardlinks; nem todo filesystem suporta. Verifica no
		// momento do save, com a mesma funcao que o runtime usa.
		if server.Librarian != nil {
			if err := server.Librarian.ProbePath(config.CompletedAnimePath); err != nil {
				JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
		}
```

- [ ] **Step 5: Atualize o stub de teste da interface**

`src/internal/api/endpoint_config_test.go:21` implementa a interface `files.Librarian`. Trocar:

```go
func (s *stubLibrarian) ProbePaths(savePath, completedPath string) error  { return s.probeErr }
```

por:

```go
func (s *stubLibrarian) ProbePath(completedPath string) error { return s.probeErr }
```

E o comentário acima do tipo (`linha 14`), que menciona `ProbePaths`, passa a dizer `ProbePath`.

- [ ] **Step 6: Rode os testes e confirme que passam**

Run: `go test ./src/internal/files/ -run TestProbePath -v && go test ./...`
Expected: PASS. Se o compilador apontar outro implementador de `files.Librarian` com a assinatura antiga, ajuste do mesmo jeito.

- [ ] **Step 7: Peça permissão ao usuário e então commite**

```bash
git add src/internal/files/librarian.go src/internal/files/librarian_test.go src/internal/daemon/verification.go src/internal/api/endpoint_config.go src/internal/api/endpoint_config_test.go
git commit -m "refactor(files): ProbePaths vira ProbePath de caminho unico"
```

---

## Task 3: `DeleteEmptyFolders` de argumento único, pulando `.autoAnimeDownloader`

**Files:**
- Modify: `src/internal/files/filemanager.go:440-456` (`DeleteEmptyFolders`), `:618` (`deleteEmptyFolders`)
- Modify: `src/internal/daemon/helpers.go:24` e `src/internal/api/server.go:29` (interfaces)
- Modify: `src/internal/daemon/verification.go:263` (chamador)
- Modify: `src/internal/daemon/episodes_test.go:44` (mock)
- Test: `src/internal/files/filemanager_test.go` (criar)

**Interfaces:**
- Consumes: `downloadDirName` da Task 1.
- Produces: `DeleteEmptyFolders(completedAnimeSaveFolder string) error` — a assinatura de dois argumentos deixa de existir em `FileManager` e nas duas interfaces.

- [ ] **Step 1: Escreva o teste que falha**

Criar `src/internal/files/filemanager_test.go`:

```go
package files

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestManager(t *testing.T) *FileManager {
	t.Helper()
	dir := t.TempDir()
	return NewManager(
		NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "episodes"),
		filepath.Join(dir, "blocked"),
		filepath.Join(dir, "settings"),
	)
}

func TestDeleteEmptyFolders(t *testing.T) {
	t.Run("apaga pasta vazia da biblioteca", func(t *testing.T) {
		completed := t.TempDir()
		vazia := filepath.Join(completed, "Anime Vazio")
		if err := os.MkdirAll(vazia, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		if _, err := os.Stat(vazia); err == nil {
			t.Error("pasta vazia deveria ter sido apagada")
		}
	})

	// A rain aloca <download>/<id> e so depois escreve nele. Sem esta guarda a varredura
	// da biblioteca apagaria diretorios de torrent recem-criados.
	t.Run("nunca entra no diretorio de download", func(t *testing.T) {
		completed := t.TempDir()
		torrentDir := filepath.Join(completed, ".autoAnimeDownloader", "abc123")
		if err := os.MkdirAll(torrentDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		if _, err := os.Stat(torrentDir); err != nil {
			t.Errorf("diretorio de torrent vazio nao deveria ter sido apagado: %v", err)
		}
	})

	t.Run("rejeita caminho vazio", func(t *testing.T) {
		if err := newTestManager(t).DeleteEmptyFolders(""); err == nil {
			t.Error("quero erro para caminho vazio, veio nil")
		}
	})
}
```

- [ ] **Step 2: Rode o teste e confirme que falha**

Run: `go test ./src/internal/files/ -run TestDeleteEmptyFolders -v`
Expected: FAIL com `not enough arguments in call to newTestManager(t).DeleteEmptyFolders`

- [ ] **Step 3: Implemente**

Em `src/internal/files/filemanager.go`, trocar `DeleteEmptyFolders` (linhas 440-456) por:

```go
// DeleteEmptyFolders remove os diretorios de anime que ficaram vazios na biblioteca depois
// de uma exclusao. O diretorio de download vive dentro dela (ver Config.DownloadPath), e a
// rain aloca <download>/<id> antes de escrever qualquer byte — por isso a varredura o pula
// explicitamente, senao apagaria torrents recem-adicionados.
func (m *FileManager) DeleteEmptyFolders(completedAnimeSaveFolder string) error {
	if completedAnimeSaveFolder == "" {
		return fmt.Errorf("completed anime path cannot be empty")
	}

	if err := m.deleteEmptyFolders(completedAnimeSaveFolder); err != nil {
		return fmt.Errorf("failed to delete empty folders in completed anime save folder: %w", err)
	}

	return nil
}
```

E em `deleteEmptyFolders` (linha 618), logo depois de `if !entry.IsDir() { continue }`, inserir:

```go
		if entry.Name() == downloadDirName {
			continue
		}
```

- [ ] **Step 4: Atualize interfaces, chamador e mock**

Em `src/internal/daemon/helpers.go:24` e `src/internal/api/server.go:29`, trocar a linha por:

```go
	DeleteEmptyFolders(completedAnimeSaveFolder string) error
```

Em `src/internal/daemon/verification.go:263`:

```go
	if err := fileManager.DeleteEmptyFolders(configs.CompletedAnimePath); err != nil {
```

Em `src/internal/daemon/episodes_test.go:44`:

```go
func (m *mockFileManagerForEpisodes) DeleteEmptyFolders(string) error { return nil }
```

- [ ] **Step 5: Rode os testes e confirme que passam**

Run: `go test ./...`
Expected: PASS. Se o compilador apontar outro mock com a assinatura antiga, ajuste do mesmo jeito.

- [ ] **Step 6: Peça permissão ao usuário e então commite**

```bash
git add src/internal/files/filemanager.go src/internal/files/filemanager_test.go src/internal/daemon/helpers.go src/internal/daemon/verification.go src/internal/daemon/episodes_test.go src/internal/api/server.go
git commit -m "refactor(files): DeleteEmptyFolders varre so a biblioteca e pula o dir de download"
```

---

## Task 4: `FakeBackend` registra as chamadas de `Ensure`

**Files:**
- Modify: `src/internal/torrents/fakebackend.go:32` (`Ensure`) e a struct
- Test: `src/internal/torrents/fakebackend_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `func (f *FakeBackend) EnsureCalls() []string` — os `savePath` passados a `Ensure`, em ordem. A Task 5 usa isso para provar que a migração abriu a sessão no caminho **antigo**.

Nota: `Ensure` continua devolvendo `(false, nil)` e o `DataDir` sintético de `Add` não muda — os testes de migração montam os diretórios via `AddCompleted(hash, dataDir)`, que já aceita um `DataDir` explícito.

- [ ] **Step 1: Escreva o teste que falha**

Adicionar ao fim de `src/internal/torrents/fakebackend_test.go`:

```go
func TestFakeBackendRecordsEnsureCalls(t *testing.T) {
	f := NewFakeBackend()

	if _, err := f.Ensure("/velho"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := f.Ensure("/novo"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got := f.EnsureCalls()
	if len(got) != 2 || got[0] != "/velho" || got[1] != "/novo" {
		t.Errorf("EnsureCalls() = %v, quero [/velho /novo]", got)
	}
}
```

- [ ] **Step 2: Rode o teste e confirme que falha**

Run: `go test ./src/internal/torrents/ -run TestFakeBackendRecordsEnsureCalls -v`
Expected: FAIL com `f.EnsureCalls undefined`

- [ ] **Step 3: Implemente**

Em `src/internal/torrents/fakebackend.go`, adicionar o campo à struct, junto de `announceCalls`:

```go
	// ensureCalls records every Ensure(savePath) for assertions.
	ensureCalls []string
```

Trocar `Ensure` (linha 32) por:

```go
// Ensure is a no-op for the fake (no real session to create), but records the path so tests
// can assert which save path a caller opened the session at — MigrateSavePath opens the OLD
// path before moving the data, and that ordering is the whole point of the migration.
func (f *FakeBackend) Ensure(savePath string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCalls = append(f.ensureCalls, savePath)
	return false, nil
}

// EnsureCalls returns the save paths passed to Ensure, in order.
func (f *FakeBackend) EnsureCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ensureCalls...)
}
```

- [ ] **Step 4: Rode os testes e confirme que passam**

Run: `go test ./src/internal/torrents/ -v && go test ./...`
Expected: PASS

- [ ] **Step 5: Peça permissão ao usuário e então commite**

```bash
git add src/internal/torrents/fakebackend.go src/internal/torrents/fakebackend_test.go
git commit -m "test(torrents): FakeBackend registra chamadas de Ensure"
```

---

## Task 5: `MigrateSavePath`

**Files:**
- Create: `src/internal/daemon/migration.go`
- Test: `src/internal/daemon/migration_test.go` (criar)

**Interfaces:**
- Consumes: `Config.DownloadPath()` (Task 1), `FakeBackend.EnsureCalls()` (Task 4), `torrents.TorrentBackend` (`Ensure`/`List`/`Close`), `files.FileSystem` (`MkdirAll`/`Stat`/`Rename`/`Remove`).
- Produces: `func MigrateSavePath(fs files.FileSystem, fm FileManagerInterface, backend torrents.TorrentBackend) error`. A Task 6 chama isso de `cmd/daemon/main.go` e de `verification.go`.

- [ ] **Step 1: Escreva os testes que falham**

Criar `src/internal/daemon/migration_test.go`:

```go
package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// migrationFM e um FileManagerInterface que guarda o config em memoria, para que o teste
// possa ver o que MigrateSavePath persistiu.
type migrationFM struct {
	mockFileManagerForEpisodes
	configs *files.Config
	saves   int
}

func (m *migrationFM) LoadConfigs() (*files.Config, error) { return m.configs, nil }
func (m *migrationFM) SaveConfigs(c *files.Config) error {
	m.configs = c
	m.saves++
	return nil
}

// makeTorrentDataDirAt cria <root>/<id> com um arquivo dentro e devolve o caminho.
func makeTorrentDataDirAt(t *testing.T, root, id string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "episode.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	return dir
}

func TestMigrateSavePath_MovesTorrentDirs(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dirA := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")
	dirB := makeTorrentDataDirAt(t, oldSave, "bbbbbbbbbbbbbbbbbbbbbb")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dirA)
	backend.AddCompleted("89abcdef0123456789abcdef0123456789abcdef", dirB)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	dest := filepath.Join(completed, ".autoAnimeDownloader")
	for _, id := range []string{"aaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbb"} {
		moved := filepath.Join(dest, id, "episode.mkv")
		if _, err := os.Stat(moved); err != nil {
			t.Errorf("torrent %s nao foi movido: %v", id, err)
		}
	}
	if fm.configs.SavePath != "" {
		t.Errorf("save_path deveria ter sido zerado, veio %q", fm.configs.SavePath)
	}
	// A sessao temporaria tem que ser aberta no caminho ANTIGO: e de la que os
	// DataDirs sao lidos.
	if calls := backend.EnsureCalls(); len(calls) != 1 || calls[0] != oldSave {
		t.Errorf("EnsureCalls() = %v, quero [%s]", calls, oldSave)
	}
}

// PreservesHardlinks e o teste que sustenta a tese "nada rebaixa": rename no mesmo volume
// preserva o inode, entao o hardlink ja criado na biblioteca continua apontando para os
// mesmos bytes depois da migracao. Usa OSFileSystem sobre t.TempDir() de proposito — um
// FileSystem falso nao tem inode.
func TestMigrateSavePath_PreservesHardlinks(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	src := filepath.Join(dir, "episode.mkv")
	libDir := filepath.Join(completed, "My Anime")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	libLink := filepath.Join(libDir, "My Anime - E05.mkv")
	if err := os.Link(src, libLink); err != nil {
		t.Fatalf("link: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	moved := filepath.Join(completed, ".autoAnimeDownloader", "aaaaaaaaaaaaaaaaaaaaaa", "episode.mkv")
	movedInfo, err := os.Stat(moved)
	if err != nil {
		t.Fatalf("stat arquivo movido: %v", err)
	}
	linkInfo, err := os.Stat(libLink)
	if err != nil {
		t.Fatalf("stat hardlink da biblioteca: %v", err)
	}
	if !os.SameFile(movedInfo, linkInfo) {
		t.Error("o hardlink da biblioteca deixou de apontar para o arquivo do torrent")
	}
}

func TestMigrateSavePath_Idempotent(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	fsys := files.NewOSFileSystem()
	if err := MigrateSavePath(fsys, fm, backend); err != nil {
		t.Fatalf("primeira migracao: %v", err)
	}
	savesDepoisDaPrimeira := fm.saves

	if err := MigrateSavePath(fsys, fm, backend); err != nil {
		t.Fatalf("segunda migracao: %v", err)
	}
	if fm.saves != savesDepoisDaPrimeira {
		t.Errorf("a segunda migracao deveria ser no-op, mas salvou o config de novo")
	}
}

// SkipsAncestorOfDest cobre o layout default do Docker, onde a biblioteca fica DENTRO do
// save path: SAVE_PATH=/app/downloads, COMPLETED_ANIME_PATH=/app/downloads/completed.
func TestMigrateSavePath_SkipsAncestorOfDest(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(oldSave, "completed")
	if err := os.MkdirAll(completed, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	// Um DataDir patologico que e ancestral do destino: nunca pode ser movido.
	backend.AddCompleted("89abcdef0123456789abcdef0123456789abcdef", oldSave)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	moved := filepath.Join(completed, ".autoAnimeDownloader", "aaaaaaaaaaaaaaaaaaaaaa", "episode.mkv")
	if _, err := os.Stat(moved); err != nil {
		t.Errorf("o torrent normal deveria ter sido movido: %v", err)
	}
	if _, err := os.Stat(oldSave); err != nil {
		t.Errorf("o save path nao deveria ter sido movido para dentro de si mesmo: %v", err)
	}
}

func TestMigrateSavePath_AbortsOnFailure(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	// Destino ocupado por um ARQUIVO: MkdirAll do diretorio de download falha.
	if err := os.MkdirAll(completed, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(completed, ".autoAnimeDownloader"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err == nil {
		t.Fatal("quero erro, veio nil")
	}
	// A vedacao critica: com o save_path ainda no config, a proxima tentativa migra de
	// novo. Zera-lo aqui faria o daemon apontar para o caminho novo com os dados no
	// antigo, e rebaixar tudo em silencio.
	if fm.configs.SavePath != oldSave {
		t.Errorf("save_path deveria continuar %q, veio %q", oldSave, fm.configs.SavePath)
	}
}

func TestMigrateSavePath_NoOpWhenNothingToDo(t *testing.T) {
	fsys := files.NewOSFileSystem()

	t.Run("save_path vazio", func(t *testing.T) {
		fm := &migrationFM{configs: &files.Config{CompletedAnimePath: t.TempDir()}}
		if err := MigrateSavePath(fsys, fm, torrents.NewFakeBackend()); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.saves != 0 {
			t.Error("nao deveria ter salvo o config")
		}
	})

	t.Run("biblioteca vazia (config incompleta)", func(t *testing.T) {
		fm := &migrationFM{configs: &files.Config{SavePath: t.TempDir()}}
		if err := MigrateSavePath(fsys, fm, torrents.NewFakeBackend()); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.saves != 0 {
			t.Error("nao deveria ter salvo o config")
		}
	})

	t.Run("save_path ja igual ao derivado: so limpa o campo", func(t *testing.T) {
		completed := t.TempDir()
		fm := &migrationFM{configs: &files.Config{
			SavePath:           filepath.Join(completed, ".autoAnimeDownloader"),
			CompletedAnimePath: completed,
		}}
		backend := torrents.NewFakeBackend()
		if err := MigrateSavePath(fsys, fm, backend); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.configs.SavePath != "" {
			t.Errorf("save_path deveria ter sido zerado, veio %q", fm.configs.SavePath)
		}
		if len(backend.EnsureCalls()) != 0 {
			t.Error("nao deveria ter aberto sessao nenhuma")
		}
	})
}
```

- [ ] **Step 2: Rode os testes e confirme que falham**

Run: `go test ./src/internal/daemon/ -run TestMigrateSavePath -v`
Expected: FAIL com `undefined: MigrateSavePath`

- [ ] **Step 3: Implemente**

Criar `src/internal/daemon/migration.go`:

```go
package daemon

import (
	"fmt"
	"path/filepath"
	"strings"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
)

// MigrateSavePath converte uma instalacao que ainda tem save_path configurado para o modelo
// de pasta unica: os dados dos torrents sao movidos para <completed_anime_path>/.autoAnimeDownloader
// e o campo legado e zerado.
//
// Por que MOVER e nao so repontar: a rain resolve o diretorio de cada torrent como
// filepath.Join(DataDir, torrentID) em tempo de execucao (session_storage.go:29) — o caminho
// nao vai para o resume data. Trocar o save path sem mover faria todo torrent existente
// apontar para um diretorio vazio, reverificar, achar nada e rebaixar tudo.
//
// Por que o rename e seguro: o probe de hardlink sempre exigiu que save path e biblioteca
// estivessem no mesmo volume, entao qualquer config que funcionava tem origem e destino no
// mesmo filesystem. Rename preserva o inode: o torrent segue semeando dos mesmos bytes e os
// hardlinks ja criados na biblioteca continuam validos.
//
// E idempotente: chamada no boot e no topo do passe de verificacao.
func MigrateSavePath(fs files.FileSystem, fm FileManagerInterface, backend torrents.TorrentBackend) error {
	configs, err := fm.LoadConfigs()
	if err != nil {
		return fmt.Errorf("migration: failed to load configs: %w", err)
	}
	if configs == nil || configs.SavePath == "" {
		return nil // nada a migrar
	}
	if configs.CompletedAnimePath == "" {
		// Config incompleta: sem biblioteca nao ha destino. O passe de verificacao tenta
		// de novo depois que o usuario salvar a configuracao.
		return nil
	}

	dest := configs.DownloadPath()
	oldSavePath := configs.SavePath

	if oldSavePath == dest {
		configs.SavePath = ""
		if err := fm.SaveConfigs(configs); err != nil {
			return fmt.Errorf("migration: failed to clear the legacy save path: %w", err)
		}
		logger.Logger.Info().Msg("Migration: legacy save path already matched the derived download path; cleared the field")
		return nil
	}

	if backend == nil {
		return fmt.Errorf("migration: torrent backend not initialized")
	}

	// Abrir a sessao no caminho ANTIGO e listar e o que torna a migracao precisa: move
	// exatamente os diretorios que sao torrents, e nada que o usuario tenha deixado no
	// save path.
	if _, err := backend.Ensure(oldSavePath); err != nil {
		return fmt.Errorf("migration: failed to open the torrent session at the old save path %s: %w", oldSavePath, err)
	}
	var dataDirs []string
	for _, t := range backend.List() {
		if t.DataDir != "" {
			dataDirs = append(dataDirs, t.DataDir)
		}
	}
	if err := backend.Close(); err != nil {
		logger.Logger.Warn().Err(err).Msg("Migration: error closing the temporary torrent session")
	}

	if err := fs.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("migration: cannot create the download folder %s: %w", dest, err)
	}

	moved := 0
	for _, dir := range dataDirs {
		if isAncestorOrEqual(dir, dest) {
			// Caso patologico: mover um diretorio para dentro dele mesmo. Nao acontece
			// com DataDirs normais (<save>/<uuid>), mas o layout default do Docker
			// aninha a biblioteca dentro do save path, entao a guarda e explicita.
			logger.Logger.Warn().Str("data_dir", dir).Str("dest", dest).
				Msg("Migration: skipping a torrent directory that contains the destination")
			continue
		}
		target := filepath.Join(dest, filepath.Base(dir))
		if _, err := fs.Stat(target); err == nil {
			continue // ja migrado numa execucao anterior
		}
		if err := fs.Rename(dir, target); err != nil {
			return fmt.Errorf("migration: failed to move %s to %s: %w", dir, target, err)
		}
		moved++
	}

	configs.SavePath = ""
	if err := fm.SaveConfigs(configs); err != nil {
		return fmt.Errorf("migration: moved %d torrent folders but failed to clear the legacy save path: %w", moved, err)
	}

	// So tem efeito se a pasta antiga ficou vazia. Sobras do usuario ficam onde estao.
	if err := fs.Remove(oldSavePath); err != nil {
		logger.Logger.Info().Str("old_save_path", oldSavePath).
			Msg("Migration: the old save path still has files in it and was left in place")
	}

	logger.Logger.Info().
		Str("old_save_path", oldSavePath).
		Str("new_download_path", dest).
		Int("moved", moved).
		Msg("Migration: moved the download folder into the library; seeding and library hardlinks were preserved")
	return nil
}

// isAncestorOrEqual reporta se dir e o proprio child ou um diretorio acima dele.
func isAncestorOrEqual(dir, child string) bool {
	dirClean := filepath.Clean(dir)
	childClean := filepath.Clean(child)
	if dirClean == childClean {
		return true
	}
	return strings.HasPrefix(childClean, dirClean+string(filepath.Separator))
}
```

- [ ] **Step 4: Rode os testes e confirme que passam**

Run: `go test ./src/internal/daemon/ -run TestMigrateSavePath -v`
Expected: PASS (todos os subtestes)

- [ ] **Step 5: Rode a suíte inteira**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Peça permissão ao usuário e então commite**

```bash
git add src/internal/daemon/migration.go src/internal/daemon/migration_test.go
git commit -m "feat(daemon): migrar save_path legado para dentro da biblioteca"
```

---

## Task 6: Ligar a migração e trocar os consumidores para `DownloadPath()`

**Files:**
- Modify: `src/cmd/daemon/main.go:222-237` (`ensureStartupSession`), `:297` (após `jobQueue.Start()`)
- Modify: `src/internal/daemon/verification.go` (chamada da migração antes do `Ensure`, e o `Ensure`)
- Modify: `src/internal/daemon/manual_download.go:51,86,130`
- Modify: `src/internal/api/endpoint_status.go:37-38`

**Interfaces:**
- Consumes: `MigrateSavePath` (Task 5), `Config.DownloadPath()` (Task 1).
- Produces: nada de novo.

Esta task e a Task 5 **têm que andar juntas na mesma branch**: trocar os consumidores sem a migração deixaria os dados existentes órfãos.

- [ ] **Step 1: Chame a migração no boot**

Em `src/cmd/daemon/main.go`, logo **depois** de `jobQueue.Start()` (linha 297) e **antes** do bloco de `defer`, inserir:

```go
	// Converte uma instalacao antiga (save_path configurado) para o modelo de pasta unica,
	// movendo os dados. Roda DEPOIS de jobQueue.Start() e ANTES de ensureStartupSession:
	// a sessao temporaria que a migracao abre no caminho antigo pode disparar uma
	// conclusao, e Start() carrega a lista de jobs persistida por cima do que esta em
	// memoria — a reconciliacao de startup re-enfileira o que for preciso. Uma falha aqui
	// nao e fatal: o passe de verificacao tenta de novo e aborta enquanto nao conseguir.
	if err := daemon.MigrateSavePath(files.NewOSFileSystem(), fileManager, torrentManager); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to migrate the legacy save path; the verification pass will retry")
	}
```

- [ ] **Step 2: Troque `ensureStartupSession` para o caminho derivado**

Em `src/cmd/daemon/main.go`, trocar o corpo de `ensureStartupSession` (linhas 222-237) por:

```go
func ensureStartupSession(manager *torrents.SessionManager, fileManager *files.FileManager) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load configs at startup; torrent session will be created on the first verification pass")
		return
	}
	if configs == nil || configs.DownloadPath() == "" {
		logger.Logger.Info().Msg("Completed anime path not configured; torrent session will be created once the configuration is saved")
		return
	}
	downloadPath := configs.DownloadPath()
	if _, err := manager.Ensure(downloadPath); err != nil {
		logger.Logger.Error().Err(err).Str("download_path", downloadPath).Msg("Failed to create the embedded torrent session at startup; the verification pass will retry")
		return
	}
	logger.Logger.Info().Str("download_path", downloadPath).Msg("Embedded torrent session started; seeding is active independently of the daemon loop")
}
```

- [ ] **Step 3: Chame a migração no passe e troque o `Ensure`**

Em `src/internal/daemon/verification.go`, logo **antes** do bloco `if librarian != nil` (o probe), inserir:

```go
	// Converte instalacoes antigas antes de qualquer coisa tocar o caminho de download.
	// Abortar aqui e deliberado: seguir para o caminho novo com os dados no antigo faria
	// a rain reverificar, achar nada e rebaixar tudo.
	if err := MigrateSavePath(files.NewOSFileSystem(), fileManager, backend); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to migrate the legacy save path; skipping verification")
		state.SetLastCheckError(err)
		return
	}
	configs, err = fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to reload configs after migration; skipping verification")
		state.SetLastCheckError(err)
		return
	}
```

Nota: o recarregamento é necessário porque a migração zera `SavePath` e persiste. Se a variável `configs` do escopo não for reatribuível com `=` (declarada com `:=` num `if`), ajuste para o padrão do arquivo — o objetivo é que o resto da função enxergue o config já migrado.

Depois, trocar o `Ensure` (linha ~72):

```go
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
```

E a mensagem de config incompleta (linha 42) passa a citar só a biblioteca:

```go
		state.SetLastCheckError(fmt.Errorf("missing required configuration for daemon (Anilist username or completed anime path)"))
```

- [ ] **Step 4: Troque as 3 chamadas em `manual_download.go`**

Nas linhas 51, 86 e 130 de `src/internal/daemon/manual_download.go`, trocar:

```go
	if _, err := backend.Ensure(configs.SavePath); err != nil {
```

por:

```go
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
```

- [ ] **Step 5: Meça o disco na biblioteca**

Em `src/internal/api/endpoint_status.go`, trocar as linhas 37-38 por:

```go
		// Mesmo volume que o diretorio de download, por construcao (ver Config.DownloadPath).
		if cfg, err := server.FileManager.LoadConfigs(); err == nil && cfg.CompletedAnimePath != "" {
			diskTotal, diskFree, _ = files.DiskSpace(cfg.CompletedAnimePath)
```

- [ ] **Step 6: Rode a suíte e confirme que passa**

Run: `go test ./...`
Expected: PASS. Ajuste os imports que o compilador apontar (`files` e `logger` em `verification.go`, `files` em `main.go` — provavelmente já presentes).

- [ ] **Step 7: Peça permissão ao usuário e então commite**

```bash
git add src/cmd/daemon/main.go src/internal/daemon/verification.go src/internal/daemon/manual_download.go src/internal/api/endpoint_status.go
git commit -m "feat(daemon): usar o caminho de download derivado e rodar a migracao"
```

---

## Task 7: Superfície de configuração — `save_path` deixa de ser exigido e não pode voltar

**Files:**
- Modify: `src/internal/daemon/helpers.go:90-92` (`isConfigComplete`)
- Modify: `src/internal/api/endpoint_config.go:86-90` (validação) e o bloco de migração de campos legados (~linha 76)
- Test: `src/internal/api/endpoint_config_test.go`

**Interfaces:**
- Consumes: nada de novo.
- Produces: `PUT /config` passa a aceitar payload sem `save_path` e a persistir `SavePath` sempre vazio.

- [ ] **Step 1: Escreva os testes que falham**

Adicionar ao fim de `src/internal/api/endpoint_config_test.go`. `server.Librarian` fica `nil`, então o `ProbePath` é pulado — igual aos testes de PUT que já existem no arquivo:

```go
func TestHandleUpdateConfig_SavePath(t *testing.T) {
	// save_path deixou de ser exigido: o diretorio de download e derivado da biblioteca.
	t.Run("PUT sem save_path e aceito", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
		}
	})

	// A vedacao: um cliente antigo mandando save_path nao pode reintroduzir o campo, senao
	// daemon.MigrateSavePath re-arma a cada boot.
	t.Run("PUT com save_path persiste o campo vazio", func(t *testing.T) {
		mockFM := &mockFileManager{}
		handler := handleUpdateConfig(&Server{State: daemon.NewState(), FileManager: mockFM})

		config := files.Config{
			AnilistUsernames:    []string{"testuser"},
			SavePath:            "/tmp/legado",
			CompletedAnimePath:  "/tmp/completed",
			CheckInterval:       10,
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   5,
		}

		jsonData, _ := json.Marshal(config)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
		}
		if mockFM.configs == nil {
			t.Fatal("o config nao foi persistido")
		}
		if mockFM.configs.SavePath != "" {
			t.Errorf("SavePath deveria ter sido zerado, veio %q", mockFM.configs.SavePath)
		}
	})
}
```

Nota: `mockFileManager.SaveConfigs` (linha 53) guarda o ponteiro em `m.configs`, que é o que a segunda asserção lê. `mockFM.configs` começa `nil` e `LoadConfigs` devolve um default nesse caso — por isso o `t.Fatal` antes de desreferenciar.

- [ ] **Step 2: Rode os testes e confirme que falham**

Run: `go test ./src/internal/api/ -run TestHandleUpdateConfig_SavePath -v`
Expected: FAIL — o primeiro subteste com 400 (`"Save path is required"`) em vez de 200, o segundo com `SavePath == "/tmp/legado"`.

- [ ] **Step 3: Implemente**

Em `src/internal/daemon/helpers.go`, trocar `isConfigComplete`:

```go
func isConfigComplete(config *files.Config) bool {
	return len(config.AnilistUsernames) > 0 && config.CompletedAnimePath != ""
}
```

Em `src/internal/api/endpoint_config.go`, **remover** o bloco inteiro:

```go
		if config.SavePath == "" {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Save path is required")
			return
		}
```

E, junto do bloco que já migra `anilist_username` → `anilist_usernames` (~linha 76), adicionar:

```go
		// save_path deixou de ser configuravel: o diretorio de download e derivado da
		// biblioteca. Zerar aqui e a vedacao que impede a API de reintroduzir o campo e
		// re-armar daemon.MigrateSavePath a cada boot. A migracao so pode ser disparada
		// por um config.json escrito por uma versao anterior, que e o caso de uso.
		config.SavePath = ""
```

- [ ] **Step 4: Rode os testes e confirme que passam**

Run: `go test ./src/internal/api/ -run TestHandleUpdateConfig_SavePath -v && go test ./...`
Expected: PASS

- [ ] **Step 5: Peça permissão ao usuário e então commite**

```bash
git add src/internal/daemon/helpers.go src/internal/api/endpoint_config.go src/internal/api/endpoint_config_test.go
git commit -m "feat(api): save_path deixa de ser configuravel e e zerado no PUT /config"
```

---

## Task 8: Docker e testes de integração param de escrever `save_path`

**Files:**
- Modify: `docker/entrypoint.sh:12` e `:31`
- Modify: `docker/docker-compose.test.yml:76`
- Modify: `src/tests/integration/integration_test.go:25` e o body do PUT de config

**Interfaces:**
- Consumes: a zeragem da Task 7.
- Produces: nada de novo.

Sem esta task, um container com `FORCE_CONFIG_UPDATE` reescreve `save_path` e roda a migração a cada boot, e os testes de integração — que reconfiguram um daemon real (`decisions.md #23`) — disparariam a migração nele.

- [ ] **Step 1: Limpe o entrypoint**

Em `docker/entrypoint.sh`, remover a linha 12:

```sh
SAVE_PATH="${SAVE_PATH:-/app/downloads}"
```

E remover a linha `"save_path": "$SAVE_PATH",` do heredoc que gera o `config.json` (linha 31). O JSON resultante começa em `"completed_anime_path"`.

- [ ] **Step 2: Limpe o compose de teste**

Em `docker/docker-compose.test.yml`, remover a linha:

```yaml
      - TEST_SAVE_PATH=/app/data/aad-test/downloads
```

- [ ] **Step 3: Limpe os testes de integração**

Em `src/tests/integration/integration_test.go`, remover a constante `testSavePath` (linha 25) e a chave `save_path` do body enviado no `PUT /api/v1/config`. Se `getEnvOrDefault` ou `defaultTestPath` ficarem sem uso, o compilador aponta — remova junto.

- [ ] **Step 4: Verifique**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. Os testes de integração continuam SKIP sem `DAEMON_URL`, que é o esperado.

Run: `sh -n docker/entrypoint.sh`
Expected: sem saída (sintaxe válida)

- [ ] **Step 5: Peça permissão ao usuário e então commite**

```bash
git add docker/entrypoint.sh docker/docker-compose.test.yml src/tests/integration/integration_test.go
git commit -m "chore: parar de escrever save_path no docker e nos testes de integracao"
```

---

## Task 9: Frontend — campo removido, hint derivado

**Files:**
- Modify: `src/internal/frontend/src/routes/Config.svelte:26-27, 61, 172, 305-313`
- Modify: `src/internal/frontend/src/lib/api/client.ts:101`
- Modify: `src/internal/frontend/messages/en.json` e `messages/pt-BR.json`
- Modify: `src/internal/frontend/tests/smoke/helpers.ts:41`

**Interfaces:**
- Consumes: a API da Task 7 (aceita payload sem `save_path`).
- Produces: nada de novo.

- [ ] **Step 1: Atualize as mensagens**

Em `src/internal/frontend/messages/en.json`, **remover** `config_label_save_path`, `config_hint_save_path` e `config_val_save_path`; e trocar `config_hint_completed_path` por:

```json
  "config_hint_completed_path": "Your Jellyfin library. Completed episodes are hardlinked here, and in-progress downloads live in a hidden .autoAnimeDownloader folder inside it.",
```

Em `src/internal/frontend/messages/pt-BR.json`, remover as mesmas três chaves e trocar:

```json
  "config_hint_completed_path": "Sua biblioteca do Jellyfin. Os episódios completos são vinculados aqui por hardlink, e os downloads em andamento ficam numa pasta oculta .autoAnimeDownloader dentro dela.",
```

- [ ] **Step 2: Atualize o `Config.svelte`**

Remover as linhas 26-27 do bloco `T`:

```js
    labelSavePath: m.config_label_save_path(),
    hintSavePath: m.config_hint_save_path(),
```

Remover `save_path: "",` do objeto `config` inicial (linha 61).

Remover a validação da linha 172:

```js
      if (!config.save_path?.trim()) throw new Error(m.config_val_save_path());
```

Remover o `<Input id="save_path" ...>` inteiro (linhas 305-313).

- [ ] **Step 3: Atualize o tipo da API**

Em `src/internal/frontend/src/lib/api/client.ts`, remover a linha 101 (`save_path: string`) da interface `Config`.

- [ ] **Step 4: Atualize o fixture de smoke**

Em `src/internal/frontend/tests/smoke/helpers.ts`, remover a linha 41 (`save_path: '/downloads',`) de `baseConfig`.

- [ ] **Step 5: Recompile o i18n antes de qualquer verificação**

As funções `m.config_label_save_path()` são **geradas** a partir dos JSON de mensagem pelo Paraglide. Sem recompilar, o `svelte-check` valida contra as funções antigas e não acusa nada.

Run (a partir de `src/internal/frontend/`): `npm run i18n:compile`
Expected: `src/lib/i18n/` regenerado, sem as três chaves removidas.

- [ ] **Step 6: Verifique**

Run (a partir de `src/internal/frontend/`): `npm run check && npm run test:unit && npm run test:component && npm run build`
Expected: PASS, sem erro de tipo. Se `Config.svelte` ainda referenciar alguma das funções removidas, o `check` aponta.

Rode também `npm run test:smoke` se o ambiente tiver os browsers do Playwright instalados; se falhar por ambiente, registre isso explicitamente em vez de mascarar.

Nota: o `package.json` usa `bun run` dentro do script `build`. Se `npm` não estiver disponível, troque o prefixo por `bun run` nos comandos acima; ver `docs/agents/commands.md`.

- [ ] **Step 7: Peça permissão ao usuário e então commite**

```bash
git add src/internal/frontend/src/routes/Config.svelte src/internal/frontend/src/lib/api/client.ts src/internal/frontend/messages/en.json src/internal/frontend/messages/pt-BR.json src/internal/frontend/tests/smoke/helpers.ts
git commit -m "feat(frontend): remover o campo de pasta de download da configuracao"
```

`src/internal/frontend/src/lib/i18n/` é gerado e ignorado por um `.gitignore` próprio dentro dele — não entra no commit. A fonte de verdade são os dois arquivos em `messages/`.

---

## Task 10: Documentação e Swagger

**Files:**
- Modify: `docs/agents/decisions.md`, `architecture.md`, `config.md`, `environment.md`
- Modify: `docs/swagger/docs.go`, `swagger.json`, `swagger.yaml` (gerados)

**Interfaces:**
- Consumes: tudo que veio antes.
- Produces: nada de código.

- [ ] **Step 1: Nova entrada em `decisions.md`**

Adicionar uma entrada `### 31. Diretório de download derivado da biblioteca` seguindo o formato das outras (Location / What it looks like / Why it's right / Don't "fix" by). Conteúdo obrigatório:

- **Location:** `internal/files/filemanager.go` (`Config.DownloadPath`), `internal/files/librarian.go` (`ProbePath`), `internal/daemon/migration.go`.
- **What it looks like:** `save_path` sumiu da configuração; o diretório de download é `<completed_anime_path>/.autoAnimeDownloader`, calculado a cada uso.
- **Why it's right:** a restrição de mesmo-filesystem do `#21` era uma armadilha que só aparecia como erro no save; derivando o caminho ela vira invariante. Dois campos obrigatórios sem diferença clara confundiam o usuário, para quem só a biblioteca importa. `ProbePath` continua existindo porque exFAT/FAT32/alguns SMB não têm hardlink nenhum. O `.ignore` + o prefixo com ponto mantêm o Jellyfin fora da pasta de download.
- **Don't "fix" by:** reintroduzir `save_path` como campo de config ou variável de ambiente (re-arma a migração a cada boot); tirar a zeragem de `SavePath` no `PUT /config`; deixar `MigrateSavePath` seguir em frente quando o rename falha (rebaixa tudo em silêncio); tirar a guarda que faz `DeleteEmptyFolders` pular `.autoAnimeDownloader`.

Emendar também `#21` (o "mesmo filesystem" deixa de ser restrição sobre configuração do usuário e vira invariante), `#22` e `#26` (a sonda passou a ser de caminho único).

- [ ] **Step 2: `architecture.md`**

Documentar: `Config.DownloadPath()`, o arquivo novo `internal/daemon/migration.go` com `MigrateSavePath`, e as assinaturas alteradas de `Librarian.ProbePath` e `FileManager.DeleteEmptyFolders`.

- [ ] **Step 3: `config.md`**

Remover `save_path` dos campos configuráveis. Documentar que o diretório de download é derivado, com o caminho exato, e mencionar a migração automática para quem vinha de versão anterior.

- [ ] **Step 4: `environment.md`**

Remover as linhas de `TEST_SAVE_PATH` (linha 34) e documentar `SAVE_PATH` como removida/ignorada.

- [ ] **Step 5: Regenere o Swagger**

Run: `swag init -g src/cmd/daemon/main.go -o docs/swagger`
Expected: `docs/swagger/{docs.go,swagger.json,swagger.yaml}` atualizados; `save_path` some do schema de `files.Config`.

- [ ] **Step 6: Verificação final**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Peça permissão ao usuário e então commite**

```bash
git add docs/
git commit -m "docs: modelo de pasta unica de download"
```

---

## Desvios do spec

Registrados aqui para que o revisor não os leia como omissão:

1. **`FakeBackend` não passou a derivar `DataDir` do `savePath` do `Ensure`.** O spec (§7) previa isso. Na prática os testes de migração montam os diretórios com `AddCompleted(hash, dataDir)`, que já aceita um `DataDir` explícito — mudar a derivação arriscaria os testes existentes que dependem do `"/fake/" + hash` sintético, sem ganho. A Task 4 entrega no lugar `EnsureCalls()`, que é o que os testes realmente precisam para provar que a sessão foi aberta no caminho antigo.

2. **`verification.go` recarrega o config depois da migração.** O spec não menciona; é necessário porque `MigrateSavePath` zera `SavePath` e persiste, e o resto do passe precisa enxergar o config migrado.
