# 31. Download directory is derived from the library

**Location:** `internal/files/filemanager.go` (`Config.DownloadPath`), `internal/files/librarian.go` (`ProbePath`), `internal/daemon/migration.go`.

**What it looks like:** `save_path` disappeared from the config; the download directory is `<completed_anime_path>/.torrents`, computed on every use.

**Why it's right:** the same-filesystem constraint from `#21` was a trap that only showed up as an error on save; deriving the path turns it into an invariant. Two required fields with no clear difference confused the user, for whom only the library matters. `ProbePath` still exists because exFAT/FAT32/some SMB have no hardlinks at all. The `.ignore` + the leading-dot prefix keep Jellyfin out of the download folder.

**Don't "fix" by:** reintroducing `save_path` as a config field or environment variable; removing the guard that makes `DeleteEmptyFolders` skip `.torrents`.

**Amendment (2026-08-24): the migration was REMOVED.** `daemon.MigrateSavePath`, the
`Config.SavePath` field, zeroing it on `PUT /config`, and the six migration tests no longer exist.
What remains of the decision is only the derived path (`Config.DownloadPath`).

A consequence that needs to stay explicit: an installation that **has not migrated yet** (a
`config.json` with `save_path` filled in, from an earlier version) loses the link to the data.
The field is now unknown, JSON ignores it, rain opens the derived path, finds it empty and
**re-downloads the entire library**. Moving `<save_path>/*` to `<completed_anime_path>/.torrents`
before upgrading became a manual step, documented in the README.

The three things the migration guaranteed and now nobody does: the rename preserves the inode (seed
and hardlinks survive), the root marker (`torrents.RootMarkerName`) travels with the data —
without that the next `Ensure` sees a folder swap and zeros `LibraryPaths` (see #34) — and the
operation was idempotent with a retry every pass. Anyone reintroducing the migration needs all
three, not just the rename.
