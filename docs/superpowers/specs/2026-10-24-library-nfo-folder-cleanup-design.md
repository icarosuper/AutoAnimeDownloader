# Library folder cleanup: episode NFO sidecars, last-video folder wipe, nfo-only sweep

**Date:** 2026-10-24
**Status:** designed, not implemented
**Origin:** `DeleteEmptyFolders` only removes a library directory when `ReadDir` returns zero
entries. `Organize` writes `tvshow.nfo` into that directory (decisions.md #44), and Jellyfin's
NFO saver writes one `*.nfo` per episode next to the video (`Anime - E05.mkv` →
`Anime - E05.nfo`). After the videos are deleted the folder is never empty, so it stays in the
library forever. The sweep also only runs at the end of the verification pass: an API delete
leaves the leftover nfo until the next cycle, and with the current emptiness test it leaves it
for good.
**Scope:** `internal/files` (`RemoveFromLibrary`, new `RemoveFolderIfNoVideo`,
`DeleteEmptyFolders`), `internal/daemon` (`removeEpisodesAndLinks`). No new endpoint, config
field, or CLI command. No episode-nfo writer.

---

## The problem

A finished anime, a watched-prune that takes the last file, or a manual delete removes the
library hardlinks and the torrent. What remains in `<completed_anime_path>/<AnimeName>/` is:

- `tvshow.nfo` (written by us)
- `Anime - E01.nfo` … `E0N.nfo` (written by Jellyfin after it identifies the episode from the
  filename)

`deleteEmptyFolders` requires `len(subEntries) == 0`. Those files fail the test. The user sees a
folder full of NFO icons and no video.

`removeEpisodesAndLinks` already has the paths (`LibraryPaths`) and is the single chokepoint for
every deletion (status prune, watched prune, API episode delete, torrent delete, replace,
standalone remove). The sweep at the end of the pass does not: it walks the library root with no
idea which anime just left. Existing nfo-only folders have no records left, so a contextual
cleanup never visits them again.

## What this is not

- Not an episode-nfo writer. `tvshow.nfo` exists because Jellyfin guesses the **show** wrong from
  the folder name. It guesses the **episode** from `Anime - E05.mkv`. Jellyfin's NFO saver then
  caches that. `EpisodeStruct` has the torrent filename, not an AniList episode title; a file we
  wrote would be overwritten on the next scan. Sidecar **deletion** still happens, whoever
  created the file.
- Not a change to `RenameFilesForJellyfin`. Show id and episode number are different problems.
  Default stays `false`.
- Not a change to `writeShowNFO` / `cleanupIfEmpty` / the "nfo after hardlinks" order
  ([decisions.md #44](../../agents/decisions.md#44-tvshownfo-is-written-after-the-hardlinks-never-overwrites-and-failing-it-does-not-fail-organize)).
- Not a recursive walk of the whole library. The sweep still looks at immediate children of
  `completed_anime_path` and still skips `.torrents`.
- Not a config flag. Leftover nfo folders are always garbage.

---

## Design

Three cooperating rules. (1) and (2) run inside `removeEpisodesAndLinks` so every delete path
gets them. (3) stays at the end of the verification pass for folders that no record points at
any more.

### 1. Sidecar of the episode

`Librarian.RemoveFromLibrary(path)` already deletes one library hardlink; a missing file is not
an error. After the video is gone (removed, or already missing), it also deletes the sibling
with the same basename and extension `.nfo`:

```
<dir>/Fatekaleid liner Prisma☆Illya - E05.mkv
<dir>/Fatekaleid liner Prisma☆Illya - E05.nfo   ← also removed
```

Rules:

- Construct the sidecar as `strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"`
  (lowercase). Jellyfin on Linux writes that. A leftover `.NFO` on a case-sensitive
  disk is not a second lookup: it goes with the folder in (2) when no video remains,
  or stays next to a surviving video.
- If `path` itself already has extension `.nfo` (any case), do not look for a further sidecar.
- A missing sidecar is not an error.
- If the video `Remove` fails and the file is still on disk, leave the sidecar (Jellyfin still
  has an episode).
- A sidecar `Remove` failure is logged and does not fail `RemoveFromLibrary` once the video is
  gone.
- Batch guard unchanged: when `removeEpisodesAndLinks` skips `RemoveFromLibrary` because a
  pack sibling survives, the sidecar stays too.
- `keepData=true` still skips the whole library loop.

Signature of `RemoveFromLibrary` does not change. Every caller inherits the sidecar behaviour.

### 2. Folder with no videos left

After the hardlink loop in `removeEpisodesAndLinks` (`keepData=false`), collect the unique
`filepath.Dir` of every path that was actually passed to `RemoveFromLibrary`. For each of those
directories call a new method:

```go
// RemoveFolderIfNoVideo deletes dir and everything in it when it contains no video
// files (recursive). A remaining video — including extras/NCOP that Organize
// hardlinked — leaves the folder untouched. A missing directory is not an error.
RemoveFolderIfNoVideo(dir string) error
```

"No video" is the disk, not `episodes.json`. Reuse `collectVideoFiles` / `isVideoFile` (same
extension list as Organize). A leftover `.srt`, poster, or `tvshow.nfo` does **not** keep the
folder; a leftover `.mkv` does.

When the folder is wiped, everything goes: `tvshow.nfo`, leftover episode nfos, Jellyfin
artwork, empty subdirectories. Implementation walks with `ReadDir` + `Remove`; `FileSystem`
does not gain `RemoveAll`.

Failure is best-effort: log and continue. `removeEpisodesAndLinks` still drops the episode
records. The method is added to the `Librarian` interface. The three test stubs
(`stubLibrarian`, `spyLibrarian`, `trackingLibrarian`) gain a no-op. `testLibrarian()` already
returns `NewLibrarian` and needs no change.

Do not call this on `.torrents` or on `completed_anime_path` itself. `LibraryPaths` are always
`<completed>/<AnimeName>/<file>`; `filepath.Dir` is the anime folder. Skip `dir == ""`.

### 3. Sweep for orphans

`FileManager.deleteEmptyFolders` keeps its place at the end of the verification pass and its
`.torrents` skip. The emptiness test becomes:

The directory is removable when every remaining entry is a **file** whose extension is `.nfo`
(case-insensitive). Zero entries is the same rule. A subdirectory, `poster.jpg`, `.DS_Store`,
or anything else keeps the folder.

This is deliberately stricter than (2). The sweep walks **every** child of the library root,
including folders the user may have created by hand. It only deletes what is unambiguously our
leftover (empty, or nfo-only). Folders we just emptied of video are already gone via (2),
including any Jellyfin artwork.

To delete: remove each leftover `.nfo`, then `Remove` the directory. Log at Info on success,
Warn on failure, same as today. The message becomes "Deleted leftover library folder" because
the folder may have contained nfo files.

Accepted limit: an orphan folder that has **no video, no records, and a non-nfo file**
(Jellyfin `poster.jpg` left behind from a previous version) is left on disk. (2) never sees it.
Cleaning those is a manual delete. Do not widen the sweep to "no video files".

---

## Call flow

```
removeEpisodesAndLinks (keepData=false)
  for each deleted episode that may drop its library files
      RemoveFromLibrary(video)          // 1. video + sibling .nfo
  for each unique dir of those paths
      RemoveFolderIfNoVideo(dir)        // 2. whole folder if no video left
  backend.Remove torrents
  DeleteEpisodesFromFile
  EpisodeDeleted webhooks

verification pass, after handleSavedEpisodes
  DeleteEmptyFolders(CompletedAnimePath) // 3. nfo-only orphans
```

API deletes (`RemoveEpisodesWithLinks`, `RemoveTorrentWithEpisodes`) go through
`removeEpisodesAndLinks`, so (1) and (2) run immediately. They do not run the sweep; the next
pass still does, for orphans.

`DownloadStandaloneAnime` still does not call `DeleteEmptyFolders` (decisions.md #49). Adding
an anime is not an occasion to delete anything. If that path ever deleted episodes it would go
through `removeEpisodesAndLinks` and pick up (1)(2) for free.

---

## Error handling

Same policy as today's library deletion: freeing disk is best-effort; failing to drop the
saved-episode records is still returned to the caller.

| Failure | Behaviour |
|---|---|
| Video `Remove` fails, file still there | Return that error from `RemoveFromLibrary`; leave sidecar |
| Video already missing | Success; still try sidecar |
| Sidecar missing | Success |
| Sidecar `Remove` fails | Log, return nil from `RemoveFromLibrary` (video is gone) |
| `RemoveFolderIfNoVideo` cannot read or delete | Log, `removeEpisodesAndLinks` continues |
| Sweep cannot read one child | Log, continue with the others (today's behaviour) |
| Sweep `Remove` fails | Log, continue |

---

## Tests

TDD: write these first, then the code.

`internal/files/librarian_test.go` — `RemoveFromLibrary`:

- Video + sibling `.nfo` → both gone.
- Video only → video gone, no error.
- `.nfo` only (video already missing) → nfo gone, no error.
- Path that is itself `foo.nfo` → that file gone, no `foo` sidecar hunt.
- Empty path → no-op.

`internal/files/librarian_test.go` — `RemoveFolderIfNoVideo`:

- Folder with `tvshow.nfo` and episode nfos, no video → directory gone.
- Folder with one `.mkv` and nfos → directory and files remain.
- Folder with only `poster.jpg` (no video) → directory gone (contextual is aggressive).
- Missing directory → nil.
- Empty path → no-op.

`internal/files/filemanager_test.go` — `DeleteEmptyFolders` (extend the existing table):

- Folder with only `tvshow.nfo` and `Anime - E01.nfo` → deleted.
- Truly empty folder → still deleted.
- Folder with `tvshow.nfo` and `video.mkv` → kept.
- Folder with `tvshow.nfo` and `poster.jpg` → kept (sweep is conservative).
- `.torrents/abc` empty → kept.
- Empty path argument → error, unchanged.

`internal/daemon` — `removeEpisodesAndLinks` with a real librarian and temp library:

- Deleting the last episode of an anime removes the video, its sidecar, and the folder.
- Deleting one episode of two leaves the sibling video, its sidecar, `tvshow.nfo`, and the
  folder.
- `keepData=true` leaves video, sidecar, and folder.

Existing `TestDeleteEmptyFolders` / `TestRemoveFromLibrary` / mock `DeleteEmptyFolders`
continue to compile. The three `Librarian` stubs grow a no-op `RemoveFolderIfNoVideo`.

---

## Docs (same change as the code)

- `docs/agents/architecture.md` — On-Disk Layout **Deletion** bullet: removing a library
  hardlink also removes the sibling `.nfo`; a library folder with no remaining video is
  removed; `DeleteEmptyFolders` removes empty and nfo-only folders and still skips
  `.torrents`.
- `docs/agents/decisions.md` — **#106** (next after #105): why the sweep stays conservative
  (nfo-only) while the contextual wipe is "no video"; why we delete Jellyfin's episode nfos
  without writing them; why `RemoveFromLibrary` grew the sidecar rather than a second call
  site. Index line at the top.
- `docs/superpowers/specs/README.md` — this spec, until it is executed and deleted.

No CLI/config/environment doc: no new surface.

---

## Files

| File | Change |
|---|---|
| `src/internal/files/librarian.go` | Sidecar in `RemoveFromLibrary`; `RemoveFolderIfNoVideo` + recursive delete helper; method on the interface |
| `src/internal/files/librarian_test.go` | Cases above |
| `src/internal/files/filemanager.go` | `deleteEmptyFolders` nfo-only criterion |
| `src/internal/files/filemanager_test.go` | Cases above |
| `src/tests/unit/files_test.go` | Existing empty-folder tests still pass; add nfo-only if that file stays the mock-FS coverage |
| `src/internal/daemon/episodes.go` | After the hardlink loop, unique dirs → `RemoveFolderIfNoVideo` |
| `src/internal/daemon/episodes_test.go` / `orchestration_test.go` | Last-episode wipes folder; keepData does not; `spyLibrarian` no-op |
| `src/internal/api/endpoint_config_test.go` | `stubLibrarian` no-op |
| `src/internal/api/endpoint_torrents_test.go` | `trackingLibrarian` no-op |
| `docs/agents/architecture.md` | Deletion bullet |
| `docs/agents/decisions.md` | #106 + index |

`FileSystem` is unchanged. `cleanupIfEmpty` is unchanged.
