# 110. `.chapters` is leftover metadata, same class as `.nfo`

**Location:** `internal/files/filemanager.go` (`nfoOnlyFolder`, `removeLeftover`).

**What it looks like:** after v2.7.0 the sweep deleted nfo-only folders (Prisma Illya and five
more on the homelab), and still left four others that "only had `.nfo` files". Those four all
had a hidden `.chapters/` directory of `*_chapters.xml` that Jellyfin extracts from the mkv.
File managers hide dot-directories, so the folder looks nfo-only. `nfoOnlyFolder` used to
return false on any `IsDir()`, which is how #106 protected a hand-made `extras/` or a
`Season 01/` — and `.chapters` walked into that rule.

**Why it's right:** `.chapters` is Jellyfin's sibling of the episode `.nfo`, not a user folder.
Treating it as leftover keeps the sweep conservative (a `poster.jpg` or any other
subdirectory still stays) without widening it to "no video". `os.Remove` refuses a non-empty
dir, so the sweep has to empty `.chapters` before removing it (`removeLeftover`); FileSystem
still has no `RemoveAll`.

**Don't "fix" by:** widening the sweep to "no video files" (that's the contextual wipe,
#106); adding `RemoveAll` to `FileSystem`; treating every hidden directory as leftover.
