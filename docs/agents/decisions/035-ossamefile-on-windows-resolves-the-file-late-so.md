# 35. `os.SameFile` on Windows resolves the file late, so FileInfo is not a snapshot in tests

**Location:** `internal/files/librarian_test.go` (`TestOrganizeReplacesDifferentFileAtDestination`, the `staleAlias`). Production use is the `os.SameFile` branch of `Organize` (`internal/files/librarian.go`).

**What it looks like:** the test creates an extra hardlink (`staleAlias`) to the old file **before** calling `Organize`, and then compares `os.Stat(staleAlias)` against the destination — instead of keeping an `os.Stat(dest)` from before the swap, which would be the obvious way.

**Why it's right:** on Linux `os.Stat` records dev+inode immediately, so a `FileInfo` is a snapshot of the file at that instant. On Windows it is not: `saveInfoFromPath` (`os/types_windows.go`) stores **only the path**, and `SameFile` calls `loadFileId`, which opens that path again at comparison time. A `FileInfo` taken from `dest` before the replacement starts pointing at the **new** file after it, and `os.SameFile(staleInfo, destInfo)` is always `true` — the test failed only in the `test-backend-windows` job, with the production code correct. Keeping a second name for the old file pins its identity on every platform, because `Organize` replaces with `Remove(dest)` + `link(src, dest)` and the alternate hardlink survives the `Remove`.

In the `Organize` branch the same `os.SameFile` is correct: both `Stat`s are done at comparison time, with both paths existing.

**Don't "fix" by:** replacing the alias with an `os.Stat(dest)` saved before the swap ("it's the same thing and reads better" — it is not, and it only breaks on Windows); marking the test `t.Skip` on Windows (cross-device/hardlink is exactly what that job exists to cover); deleting the old-file assertion because it looks redundant with the content one (a mutation that skips the replacement fails both, but the identity one is what distinguishes "relinked" from "overwrote in place").
