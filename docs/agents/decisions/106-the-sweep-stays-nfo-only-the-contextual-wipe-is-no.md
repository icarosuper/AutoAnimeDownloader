# 106. The sweep stays nfo-only; the contextual wipe is "no video"

**Location:** `internal/files/librarian.go` (`RemoveFromLibrary` sidecar, `RemoveFolderIfNoVideo`);
`internal/files/filemanager.go` (`deleteEmptyFolders`); `internal/daemon/episodes.go`
(`removeEpisodesAndLinks` unique dirs).

**What it looks like:** leftover `tvshow.nfo` and Jellyfin episode sidecars keep a library folder
alive after the videos are gone, so `DeleteEmptyFolders` looking for `len(subEntries) == 0` never
fires. The obvious one-line fix is "treat nfo-only as empty" in the sweep, or "delete the whole
folder when we delete the last episode".

**Why it's right:** three cooperating rules, not one:

- **Sidecar in `RemoveFromLibrary`:** Jellyfin writes `Anime - E05.nfo` next to the video. We never
  write that file (Jellyfin guesses the episode from the filename and would overwrite ours), but
  we delete it with the video so a surviving sibling is not left with a stray sidecar. The
  signature does not change; every caller inherits it. The batch guard still skips the call, so
  the sidecar stays while the pack sibling survives.
- **Contextual wipe (`RemoveFolderIfNoVideo`):** `removeEpisodesAndLinks` is the single deletion
  chokepoint. After the hardlink loop it asks the unique `filepath.Dir` of the paths it actually
  passed: if the disk has no video left (same extension list as Organize), the folder and
  everything in it go — `tvshow.nfo`, leftover episode nfos, Jellyfin artwork. API deletes then
  free the folder immediately, instead of waiting for the next verification pass. "No video" is
  the disk, not `episodes.json`.
- **Sweep stays conservative:** it walks **every** child of `completed_anime_path`, including
  folders the user may have created by hand. It only deletes empty directories and directories
  whose remaining entries are leftover Jellyfin metadata: `.nfo` files and a `.chapters` dir
  (#110). A `poster.jpg` or any other subdirectory keeps the folder. Widening it
  to "no video files" would delete a hand-made folder of artwork. Existing nfo-only orphans
  have no records left, so the contextual wipe never sees them — that is what the sweep is
  for. It still skips `.torrents`.

Accepted limit: an orphan with `poster.jpg` and no records stays on disk. Cleaning those is a
manual delete.

**Don't "fix" by:** writing episode nfos ourselves; widening the sweep to "no video"; dropping the
sweep now that the contextual wipe exists; adding `RemoveAll` to `FileSystem`; calling this from
`DownloadStandaloneAnime` (#49); changing `writeShowNFO` / `cleanupIfEmpty` / the
"nfo after hardlinks" order (#44).
