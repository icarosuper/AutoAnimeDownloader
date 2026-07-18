# Download Media-Status Filter — Design

Add a config selector for which **airing statuses of the anime** (`RELEASING`, `FINISHED`, …) are eligible for download — mirroring the existing `download_statuses` selector, which filters the **user's list status** (`CURRENT`, `PLANNING`, …).

Two distinct AniList concepts, easy to confuse:
- `MediaListStatus` (user's relationship to the anime): already filtered by `DownloadStatuses` → `download_statuses`.
- `MediaStatus` (the anime's own airing state): **this feature**, new `download_media_statuses`.

Both enums already exist in `src/internal/anilist/anilist.go` (`MediaListStatus`, `MediaStatus`).

## Scope decisions

- **Download-only.** The filter blocks downloading; it does **not** hide animes from the frontend list. `endpoint_animes.go` (which also reads `DownloadStatuses`) is left untouched.
- **Default: `["RELEASING", "FINISHED"]`.** Changes silent behavior for existing configs on upgrade (today there is no media-status filter, so all pass) — accepted, per product decision.
- **`NOT_YET_RELEASED` is not offered** as a selectable option — it can never have episodes to download. Selectable set: `RELEASING`, `FINISHED`, `CANCELLED`, `HIATUS`.
- **Whitelist semantics.** Empty selection ⇒ nothing passes ⇒ nothing downloads. Mirrors the existing `download_statuses` behavior (empty = no matches); a deselect-all is the user's explicit choice.

## Approach

The GraphQL query filters server-side on `status_in: [MediaListStatus]` — that's the *list* status, and `MediaStatus` cannot be added to the same `mediaList` filter. So the media-status filter is applied **client-side, at a single chokepoint** after the fetch, in `getAllAnimes` (`verification.go`). One `continue` guard there; every downstream download decision inherits it. No GraphQL change.

## Backend

- **`filemanager.go`**
  - New field on `Config`:
    ```go
    DownloadMediaStatuses []string `json:"download_media_statuses"`
    ```
  - `getDefaultConfig()`: `DownloadMediaStatuses: []string{"RELEASING", "FINISHED"}`.
  - **Migration is free**: `LoadConfigs` does `config := getDefaultConfig()` then `json.Unmarshal(file, config)`. Old config files lack the key, so the field keeps the default. No migration code needed.

- **`verification.go`** (`getAllAnimes`, the append loop ~L293): skip animes whose `Media.Status` isn't in the allowed set:
  ```go
  if !mediaStatusAllowed(ml.Media.Status, configs.DownloadMediaStatuses) {
      continue
  }
  ```
  Small helper (same file or `anilist`): membership check of `string(status)` against the slice. Log skipped animes at debug level for troubleshooting parity with the rest of the pipeline.

No new endpoint, no new config-save path, no GraphQL edit.

## Frontend (`src/internal/frontend/src/routes/Config.svelte`)

Clone the existing `download_statuses` chip block (L266–285):
- New constant `ALL_MEDIA_STATUSES = ["RELEASING", "FINISHED", "CANCELLED", "HIATUS"]` (no `NOT_YET_RELEASED`).
- New `toggleDownloadMediaStatus(status)` — copy of `toggleDownloadStatus`, bound to `config.download_media_statuses`.
- New label/hint from the `T` map + `statusLabels` entries for the media statuses.

i18n: add `config_label_download_media_statuses` / `config_hint_download_media_statuses` and the four status labels to both `messages/pt-BR.json` and `messages/en.json`.

## Docs

- `docs/agents/config.md`: new row for `DownloadMediaStatuses` / `download_media_statuses`, valid values `RELEASING`, `FINISHED`, `CANCELLED`, `HIATUS`, default `["RELEASING","FINISHED"]`.
- `docs/agents/architecture.md`: note the media-status filter in `getAllAnimes` data flow.
- Swagger regen (`swag init -g src/cmd/daemon/main.go -o docs/swagger`) — `Config` struct changed.

## Testing

- Unit test for `mediaStatusAllowed`: allowed status passes, disallowed skipped, empty slice = nothing passes.
- Extend `getAllAnimes` / verification test (or `endpoint_config_test.go`) to assert an anime with a non-selected `Media.Status` is filtered out and a selected one is kept.
- `go test ./...` green.

## Out of scope

- No delete-side counterpart (`delete_statuses` stays list-status only).
- No hiding filtered animes from the frontend list.
