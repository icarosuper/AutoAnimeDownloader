# 44. `tvshow.nfo` is written AFTER the hardlinks, never overwrites, and failing it does not fail organize

**Location:** `internal/files/librarian.go` (`writeShowNFO`, called at the end of `Organize`, and `BackfillShowNFOs`); `internal/daemon/jobs.go` (`organizeTorrent` fills `OrganizeRequest.AnimeID`); `cmd/daemon/main.go` (backfill call at boot).

**What it looks like:** the nfo looks like it should be written right after `MkdirAll` (it is folder metadata, not file metadata), and an error writing it looks like an `Organize` error.

**Why it's right:** three things, all because of invariants that already existed:

- **After the links:** `Organize`'s rollback is `cleanupIfEmpty`, which uses `os.Remove` and only works on an **empty** folder. An nfo written before a hardlink that fails (cross-device, permission) would leave the folder non-empty and an orphan directory in the library on every retry.
- **Does not overwrite:** if the user corrected the match by hand in Jellyfin (which rewrites the nfo), the next episode of the same anime cannot undo that. `Organize` runs once per torrent, so this happens all the time.
- **Only logs on failure:** the hardlinks are the product; returning an error here would make the job retry (backoff, then drop) work that already succeeded, and `LibraryPaths` would never be written.

`AnimeID` is AniList's **media** id (#43) — it comes from an already-migrated `EpisodeStruct.AnimeID`. `AnimeID == 0` (old unresolvable record) simply skips the nfo: Jellyfin goes back to guessing by name, which is the previous behaviour.

**The backfill exists because `Organize` does not re-run:** `organizeTorrent` exits early when the hash's episodes already have `LibraryPaths`, and `reconcileLibrary` only enqueues whoever has empty `LibraryPaths`. Without `BackfillShowNFOs` an already-finished anime would never get the file (an in-progress one would get it on the next episode). It runs **every boot** and has no "already ran" flag: "does not overwrite" already makes it idempotent, and a flag would only create one more piece of state to desync. The folder comes from `LibraryPaths`, not from `sanitizeName(AnimeName)` — the only path that is guaranteed to match what exists on disk after manual renames or sanitization changes.

**It is gated by the id migration** (`main.go`, `else` branch of `MigrateAnimeIDsToMedia`): with ids in the old format the nfo would go out with an ENTRY id, and because it is never rewritten that would be permanent. It runs in the `else` on purpose — a migration failure only postpones the backfill to the next boot.

**Requires in Jellyfin:** the `jellyfin-plugin-anilist` plugin as a metadata provider; without it `uniqueid type="anilist"` is ignored.

**Don't "fix" by:** moving the write next to `MkdirAll`; always overwriting "to keep it in sync"; propagating the write error to `Organize`'s return; giving the backfill a config flag; deriving the backfill folder from the anime name; running the backfill before/independent of the id migration.
