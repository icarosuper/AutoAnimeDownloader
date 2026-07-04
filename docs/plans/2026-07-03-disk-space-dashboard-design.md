# Disk Space on Dashboard — Design

Implements the first bullet of `docs/TODO.md`: show total/available disk space on the main dashboard. The alert sub-bullet is scoped down to a hardcoded red-text threshold — no config field, no separate alert mechanism.

## Approach

Read disk usage via OS-level stat on `Config.SavePath` (the directory torrents are saved to). Rejected alternative: querying qBittorrent's WebUI API (`/api/v2/sync/maindata` → `free_space_on_disk`) — it only reports free space, not total capacity, so it can't satisfy "tamanho total, tamanho disponível" on its own.

**Known limitation**: if qBittorrent writes to a path that isn't the same filesystem the daemon process sees (e.g. daemon and qBittorrent split across hosts/containers with misaligned mounts), the reported numbers reflect the daemon's view of the path, not necessarily qBittorrent's actual disk. Acceptable for the common single-box setup this project targets; to be noted in `decisions.md`.

## Backend

- `src/internal/files/diskspace_unix.go` (build tag `linux || darwin`): `DiskSpace(path string) (total, free uint64, err error)` via `syscall.Statfs`.
- `src/internal/files/diskspace_windows.go` (build tag `windows`): same signature via `golang.org/x/sys/windows.GetDiskFreeSpaceEx`. `golang.org/x/sys` is already an indirect dependency — promote to direct in `go.mod`.
- `handleStatus` (`endpoint_status.go`): loads config via `server.FileManager.LoadConfigs()` (existing pattern), calls `DiskSpace(cfg.SavePath)`, adds `disk_total` / `disk_free` (bytes, `uint64`) to `StatusResponse`. On empty `SavePath` or stat error, both fields are `0` — no error surfaced, handler still returns 200.

No new endpoint, no new config field, no new polling loop — piggybacks on the existing `GET /api/v1/status` call already made on dashboard mount.

## Frontend

- `client.ts`: add `disk_total` / `disk_free` to `StatusResponse` interface.
- `Status.svelte`: new stat card next to the existing 4 (Daemon/Last Check/Next Check/Library) showing free/total (e.g. "120 GB free of 500 GB") with a small progress bar. Card is omitted entirely when `disk_total === 0` (unset/unreadable). Free/total ratio below a hardcoded threshold (10%) renders the value/bar in red — marked with a `// ponytail:` comment noting the hardcoded threshold and that a configurable one is the upgrade path if ever needed.

## Docs

- `docs/agents/architecture.md`: add `disk_total`/`disk_free` to the `/api/v1/status` response description, add `DiskSpace` to the file map for `files` package.
- `docs/agents/decisions.md`: note the "disk stat reflects daemon's filesystem view, not necessarily qBittorrent's" limitation.
- Swagger regen (`swag init`) since `StatusResponse` changed.

## Testing

- Unit test for `DiskSpace` on the running OS (sanity: total ≥ free ≥ 0, no error for a valid path; error for a bogus path).
- Existing `endpoint_status_test.go` extended to assert the new fields are present (mocked or real temp dir).
