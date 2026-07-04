# Disk Space on Dashboard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show total/available disk space (with a hardcoded red-threshold warning) on the main dashboard, per the first bullet of `docs/TODO.md`.

**Architecture:** A cross-platform `files.DiskSpace(path)` reads OS-level filesystem stats on `Config.SavePath`. `handleStatus` calls it and adds `disk_total`/`disk_free` (bytes) to the existing `/api/v1/status` response — no new endpoint, no new config field. The frontend adds one more stat card to `Status.svelte`, reading fields already fetched on mount.

**Tech Stack:** Go (`syscall` on Linux/Darwin, `golang.org/x/sys/windows` on Windows — already an indirect dependency), Svelte 5 + TypeScript.

**Design doc:** `docs/plans/2026-07-03-disk-space-dashboard-design.md`

---

### Task 1: `files.DiskSpace` — Linux/Darwin implementation

**Files:**
- Create: `src/internal/files/diskspace_unix.go`
- Test: `src/tests/unit/diskspace_test.go`

**Step 1: Write the failing test**

```go
// src/tests/unit/diskspace_test.go
package unit

import (
	"os"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
)

func TestDiskSpace(t *testing.T) {
	t.Run("valid path returns total >= free > 0", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "aad_test_disk_")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmp)

		total, free, err := files.DiskSpace(tmp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total == 0 {
			t.Error("expected total > 0")
		}
		if free > total {
			t.Errorf("expected free (%d) <= total (%d)", free, total)
		}
	})

	t.Run("nonexistent path returns error", func(t *testing.T) {
		_, _, err := files.DiskSpace("/definitely/does/not/exist/aad")
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./src/tests/unit/ -run TestDiskSpace -v`
Expected: FAIL with `undefined: files.DiskSpace`

**Step 3: Write minimal implementation**

```go
// src/internal/files/diskspace_unix.go
//go:build linux || darwin

package files

import (
	"fmt"
	"syscall"
)

// DiskSpace returns total and free bytes for the filesystem containing path.
func DiskSpace(path string) (total uint64, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem for %q: %w", path, err)
	}
	total = uint64(stat.Blocks) * uint64(stat.Bsize)
	free = uint64(stat.Bavail) * uint64(stat.Bsize)
	return total, free, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./src/tests/unit/ -run TestDiskSpace -v`
Expected: PASS (both subtests)

**Step 5: Commit**

```bash
git add src/internal/files/diskspace_unix.go src/tests/unit/diskspace_test.go
git commit -m "feat: adicionar files.DiskSpace para Linux/macOS"
```

---

### Task 2: `files.DiskSpace` — Windows implementation

No Windows machine available to run this — write it correctly by API contract and verify with a cross-compile (`go build`, not `go test`, since we can't execute Windows binaries here).

**Files:**
- Create: `src/internal/files/diskspace_windows.go`

**Step 1: Write the implementation**

```go
// src/internal/files/diskspace_windows.go
//go:build windows

package files

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// DiskSpace returns total and free bytes for the filesystem containing path.
func DiskSpace(path string) (total uint64, free uint64, err error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert path %q: %w", path, err)
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytes, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem for %q: %w", path, err)
	}
	return totalBytes, freeBytes, nil
}
```

**Step 2: Cross-compile check (no test execution possible on this machine)**

Run: `GOOS=windows GOARCH=amd64 go build ./src/internal/files/`
Expected: no output, exit code 0

**Step 3: Commit**

```bash
git add src/internal/files/diskspace_windows.go
git commit -m "feat: adicionar files.DiskSpace para Windows"
```

---

### Task 3: Promote `golang.org/x/sys` to a direct dependency

`diskspace_windows.go` imports `golang.org/x/sys/windows` directly; it's currently listed as `// indirect` in `go.mod` (pulled in transitively). Running `go mod tidy` will move it to the direct requirements block — this covers both GOOS build variants regardless of which platform you run it on.

**Step 1: Run**

```bash
go mod tidy
```

**Step 2: Verify**

Run: `grep golang.org/x/sys go.mod`
Expected: the line no longer has the `// indirect` suffix.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: promover golang.org/x/sys a dependência direta"
```

---

### Task 4: Wire disk space into `GET /api/v1/status`

**Files:**
- Modify: `src/internal/api/endpoint_status.go`
- Modify: `src/internal/api/endpoint_status_test.go`

**Step 1: Update the existing test to use `mockFileManager` and assert the new fields**

`endpoint_status_test.go` currently builds `fileManager := &files.FileManager{}` — a zero-value struct whose internal `fs` field is `nil`. Once `handleStatus` calls `LoadConfigs()`, that zero-value manager would panic (nil interface dereference inside `LoadConfigs`). Switch to the `mockFileManager` already defined in `endpoint_config_test.go` (same package, implements the full `FileManagerInterface`), pointed at a real temp dir so `files.DiskSpace` has something valid to stat.

Replace the top of `TestHandleStatus`:

```go
func TestHandleStatus(t *testing.T) {
	state := daemon.NewState()
	state.SetStatus(daemon.StatusRunning)
	state.SetLastCheck(time.Now())
	state.SetLastCheckError(nil)

	tmpDir := t.TempDir()
	fileManager := &mockFileManager{configs: &files.Config{SavePath: tmpDir}}
	server := &Server{
		State:       state,
		FileManager: fileManager,
	}

	handler := handleStatus(server)

	t.Run("GET method returns status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Error("Expected success=true")
		}

		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be an object")
		}

		if status, ok := data["status"].(string); !ok || status != "running" {
			t.Errorf("Expected status 'running', got %v", data["status"])
		}

		diskTotal, ok := data["disk_total"].(float64)
		if !ok || diskTotal <= 0 {
			t.Errorf("Expected disk_total > 0, got %v", data["disk_total"])
		}
		diskFree, ok := data["disk_free"].(float64)
		if !ok || diskFree < 0 {
			t.Errorf("Expected disk_free >= 0, got %v", data["disk_free"])
		}
	})
	// ... "Non-GET method returns 405" subtest unchanged
```

Leave the rest of the file (the `Non-GET method returns 405` subtest) as-is.

**Step 2: Run test to verify it fails**

Run: `go test ./src/internal/api/ -run TestHandleStatus -v`
Expected: FAIL — `data["disk_total"]` is `nil` (field doesn't exist yet in the JSON response)

**Step 3: Implement**

`src/internal/api/endpoint_status.go`, full new contents:

```go
package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/version"
	"net/http"
	"time"
)

type StatusResponse struct {
	Status    string    `json:"status" example:"running"`
	LastCheck time.Time `json:"last_check" example:"2024-01-01T00:00:00Z"`
	HasError  bool      `json:"has_error" example:"false"`
	Version   string    `json:"version" example:"1.2.0"`
	DiskTotal uint64    `json:"disk_total" example:"500107862016"`
	DiskFree  uint64    `json:"disk_free" example:"128849018880"`
}

// @Summary      Get daemon status
// @Description  Returns the current status of the daemon, including last check time, error state, and disk space for the configured save path
// @Tags         status
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=StatusResponse}
// @Failure      405  {object}  SuccessResponse
// @Router       /status [get]
func handleStatus(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		status, lastCheck, hasError := server.State.GetAll()

		var diskTotal, diskFree uint64
		if cfg, err := server.FileManager.LoadConfigs(); err == nil && cfg.SavePath != "" {
			diskTotal, diskFree, _ = files.DiskSpace(cfg.SavePath)
		}

		response := StatusResponse{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
			Version:   version.Version,
			DiskTotal: diskTotal,
			DiskFree:  diskFree,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
```

Disk stat errors (unreadable path, `SavePath` unset) are deliberately swallowed — `diskTotal`/`diskFree` stay `0`, the handler still returns `200`, and the frontend treats `disk_total === 0` as "no data" (Task 9). This matches the pattern the design doc calls for: never let a missing/bad disk path break the status endpoint.

**Step 4: Run test to verify it passes**

Run: `go test ./src/internal/api/ -run TestHandleStatus -v`
Expected: PASS

**Step 5: Run full backend test suite**

Run: `go test ./...`
Expected: all packages pass

**Step 6: Commit**

```bash
git add src/internal/api/endpoint_status.go src/internal/api/endpoint_status_test.go
git commit -m "feat: incluir espaço em disco em GET /api/v1/status"
```

---

### Task 5: Regenerate Swagger docs

**Step 1: Run**

```bash
swag init -g src/cmd/daemon/main.go -o docs/swagger
```

**Step 2: Verify**

Run: `git diff --stat docs/swagger`
Expected: `docs.go`, `swagger.json`, `swagger.yaml` show the two new `StatusResponse` fields.

**Step 3: Commit**

```bash
git add docs/swagger
git commit -m "docs: regenerar swagger com campos de disco"
```

---

### Task 6: Frontend — `StatusResponse` type

**Files:**
- Modify: `src/internal/frontend/src/lib/api/client.ts:70-75`

**Step 1: Edit**

```ts
export interface StatusResponse {
  status: string
  last_check: string
  has_error: boolean
  version: string
  disk_total: number
  disk_free: number
}
```

**Step 2: Commit**

```bash
git add src/internal/frontend/src/lib/api/client.ts
git commit -m "feat: adicionar campos de disco ao tipo StatusResponse"
```

---

### Task 7: Frontend — byte formatting + low-space helper (TDD)

**Files:**
- Modify: `src/internal/frontend/src/lib/utils/status.ts`
- Modify: `src/internal/frontend/tests/unit/status.utils.test.ts`

**Step 1: Write the failing test**

Append to `status.utils.test.ts`:

```ts
import { formatBytes, isDiskSpaceLow } from '../../src/lib/utils/status.js'

describe('formatBytes', () => {
  it('formats bytes as GB with one decimal', () => {
    expect(formatBytes(500_107_862_016)).toBe('466.0 GB')
  })

  it('returns 0.0 GB for zero', () => {
    expect(formatBytes(0)).toBe('0.0 GB')
  })
})

describe('isDiskSpaceLow', () => {
  it('is true when free/total is below 10%', () => {
    expect(isDiskSpaceLow(5, 100)).toBe(true)
  })

  it('is false when free/total is at or above 10%', () => {
    expect(isDiskSpaceLow(10, 100)).toBe(false)
  })

  it('is false when total is 0 (no data)', () => {
    expect(isDiskSpaceLow(0, 0)).toBe(false)
  })
})
```

**Step 2: Run test to verify it fails**

Run: `cd src/internal/frontend && bun run test:unit`
Expected: FAIL — `formatBytes`/`isDiskSpaceLow` are not exported from `status.ts`

**Step 3: Implement**

Append to `src/internal/frontend/src/lib/utils/status.ts`:

```ts
const GIB = 1024 ** 3
// ponytail: 10% hardcoded — promote to a config field if users ever ask to tune it
const LOW_DISK_SPACE_RATIO = 0.1

export function formatBytes(bytes: number): string {
  return `${(bytes / GIB).toFixed(1)} GB`
}

export function isDiskSpaceLow(free: number, total: number): boolean {
  if (total <= 0) return false
  return free / total < LOW_DISK_SPACE_RATIO
}
```

**Step 4: Run test to verify it passes**

Run: `cd src/internal/frontend && bun run test:unit`
Expected: PASS

**Step 5: Commit**

```bash
git add src/internal/frontend/src/lib/utils/status.ts src/internal/frontend/tests/unit/status.utils.test.ts
git commit -m "feat: adicionar formatBytes e isDiskSpaceLow"
```

---

### Task 8: i18n labels

**Files:**
- Modify: `src/internal/frontend/messages/en.json`
- Modify: `src/internal/frontend/messages/pt-BR.json`

**Step 1: Add key after `status_card_library` in both files**

`en.json`:
```json
  "status_card_disk": "Disk Space",
  "status_disk_free_of_total": "{free} free of {total}",
```

`pt-BR.json`:
```json
  "status_card_disk": "Espaço em Disco",
  "status_disk_free_of_total": "{free} livres de {total}",
```

**Step 2: Recompile**

```bash
cd src/internal/frontend && bun run i18n:compile
```

**Step 3: Verify**

Run: `grep -r "status_card_disk" src/internal/frontend/src/lib/i18n/ | head -3`
Expected: generated `messages.js` (or per-locale files) now export `status_card_disk`.

**Step 4: Commit**

```bash
git add src/internal/frontend/messages/en.json src/internal/frontend/messages/pt-BR.json src/internal/frontend/src/lib/i18n
git commit -m "feat: adicionar textos i18n para espaço em disco"
```

---

### Task 9: Frontend — disk space card in `Status.svelte`

**Files:**
- Modify: `src/internal/frontend/src/routes/Status.svelte`

**Step 1: Add imports and translation keys**

In the `<script>` block, extend the import from `../lib/utils/status.js`:

```ts
import { filterAnimes, sortAnimes, computeNextCheckIn, formatBytes, isDiskSpaceLow, type SortKey, type SortDir } from "../lib/utils/status.js";
```

Add to the `T` translation object (after `cardLibrary`):

```ts
    cardDisk: m.status_card_disk(),
```

**Step 2: Add a reactive flag**

Near `$: totalEpisodes = ...`:

```ts
  $: diskSpaceLow = status ? isDiskSpaceLow(status.disk_free, status.disk_total) : false;
```

**Step 3: Widen the stat grid and add the card**

Change the grid container:

```svelte
    <div class="grid grid-cols-2 lg:grid-cols-5 gap-3">
```

Add a new card after the "Totals" card (before the closing `</div>` of the grid):

```svelte
      <!-- Disk space -->
      {#if status.disk_total > 0}
        <div class="card bg-base-200 border border-base-300">
          <div class="card-body p-4 gap-1">
            <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardDisk}</span>
            <span class="text-base font-medium {diskSpaceLow ? 'text-error' : 'text-base-content'}">
              {formatBytes(status.disk_free)}
            </span>
            <span class="text-xs text-base-content/40">
              {$locale && m.status_disk_free_of_total({ free: '', total: formatBytes(status.disk_total) })}
            </span>
          </div>
        </div>
      {/if}
```

Note: `m.status_disk_free_of_total` takes a `{total}` param only in effect here since `free` is already shown on the line above — simplify the message to just take `total`:

Revise the i18n string from Task 8 to:

`en.json`: `"status_disk_free_of_total": "free of {total}"`
`pt-BR.json`: `"status_disk_free_of_total": "livres de {total}"`

And the usage:

```svelte
              {$locale && m.status_disk_free_of_total({ total: formatBytes(status.disk_total) })}
```

Re-run `bun run i18n:compile` after this correction (Task 8's compile step already ran once; this just changes the string content, no new key).

**Step 4: Manual check**

```bash
cd src/internal/frontend && bun run check
```
Expected: no new TypeScript errors.

```bash
cd src/internal/frontend && bun run dev
```
Open the dashboard in a browser (with the daemon running and `save_path` configured, or just eyeball that the card is absent when `disk_total` is 0) and confirm:
- 5 stat cards render in a row on desktop, 2-per-row on mobile.
- The disk card shows a sane free/total.
- Temporarily set `LOW_DISK_SPACE_RATIO` higher (e.g. `0.99`) to confirm the red-text path renders, then revert.

**Step 5: Commit**

```bash
git add src/internal/frontend/src/routes/Status.svelte src/internal/frontend/messages/en.json src/internal/frontend/messages/pt-BR.json src/internal/frontend/src/lib/i18n
git commit -m "feat: exibir espaço em disco na dashboard"
```

---

### Task 10: Update docs

**Files:**
- Modify: `docs/agents/architecture.md`
- Modify: `docs/agents/decisions.md`

**Step 1: `architecture.md` — add `DiskSpace` to the `files` package file map**

In the `### src/internal/files/filemanager.go` symbol table (or add a new one-line entry right after it, since `DiskSpace` lives in sibling files `diskspace_unix.go`/`diskspace_windows.go`), add:

```markdown
| `DiskSpace(path)` | Cross-platform total/free bytes for the filesystem containing `path` (`diskspace_unix.go` for Linux/Darwin via `syscall.Statfs`, `diskspace_windows.go` via `golang.org/x/sys/windows.GetDiskFreeSpaceEx`) |
```

**Step 2: `decisions.md` — add entry 19**

Append after entry 18:

```markdown
---

### 19. Disk space is read via OS stat on `SavePath`, not qBittorrent's API

**Location:** `internal/files/diskspace_unix.go`, `internal/files/diskspace_windows.go`; `internal/api/endpoint_status.go` (`handleStatus`).

**What it looks like:** Reading disk space with a raw filesystem syscall on a local path, when qBittorrent's WebUI already exposes `free_space_on_disk` via `/api/v2/sync/maindata` — reaching for a platform-specific syscall pair looks like reinventing something the existing HTTP dependency already provides.

**Why it's right:** qBittorrent's endpoint only reports free space, not total capacity — the dashboard needs both ("tamanho total, tamanho disponível" per `docs/TODO.md`). The syscall approach is the only one of the two that can answer that. The known trade-off: if qBittorrent's actual save directory isn't the same filesystem the daemon process sees (e.g. daemon and qBittorrent split across hosts/containers with misaligned mounts), the numbers reflect the daemon's view of `SavePath`, not necessarily qBittorrent's real disk. Accepted for now since this project targets the common single-box setup; `handleStatus` swallows stat errors (empty/unreadable `SavePath`) rather than surfacing them, so a bad path just hides the disk card instead of breaking `/api/v1/status`.

**Don't "fix" by:** switching to qBittorrent's `free_space_on_disk` alone (loses total capacity) or trying to detect cross-host mount mismatches — no reliable way to do that from the daemon's side without an out-of-band signal from qBittorrent itself.
```

**Step 3: Commit**

```bash
git add docs/agents/architecture.md docs/agents/decisions.md
git commit -m "docs: documentar files.DiskSpace e decisão de design"
```

---

### Task 11: Final verification

**Step 1: Backend**

```bash
go test ./...
GOOS=windows GOARCH=amd64 go build ./...
```
Expected: all pass; Windows cross-compile succeeds.

**Step 2: Frontend**

```bash
cd src/internal/frontend
bun run check
bun run test:unit
bun run build
```
Expected: no type errors, all unit tests pass, production build succeeds.

**Step 3: Report to user**

Summarize what changed and remind them: **do not commit/push further** — they push manually (per project convention already committed at each task above; final review is theirs).
