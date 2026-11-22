# 29. Startup reconciliation keys off empty `LibraryPaths`, not off the hardlink missing from disk

**Location:** `internal/daemon/verification.go` — `reconcileLibrary`, called at the top of every verification pass. Pinned by `TestReconcileLibrary_Marker` in `internal/daemon/orchestration_test.go`.

**What it looks like:** Reconciliation decides whether an episode still needs organizing by looking only at the **record** (`LibraryPaths` empty → enqueue `JobOrganize`). It never stats the paths, so a record whose `LibraryPaths` point at files the user has since deleted is treated as already organized and skipped. The embedded-client design (§2) actually specified the opposite — check whether the hardlink exists on disk — so this reads like an unfinished implementation.

**Why it's right:** Statting the paths would turn every pass into a repair loop that **resurrects files the user deleted on purpose**. Deleting from the library is a legitimate user action (freeing the library while leaving the torrent seeding, or pruning something they no longer want), and the seeded file is still there for the hardlink to be recreated from — so a disk-based check would recreate it on the next pass, every pass, forever, with no way to opt out short of deleting the episode record too. The record is the daemon's own statement of "I have organized this"; the disk belongs to the user. Keeping them separate means reconciliation only ever fixes work the daemon failed to finish (a crash between torrent completion and organize, or a session restarted before `JobOrganize` ran), which is exactly its purpose.

The deviation is intentional and better than the design. It is also what makes reconciliation idempotent and cheap: `EnqueueOrganize` dedupes by hash, so three consecutive passes over the same unorganized torrent produce one job.

**Don't "fix" by:** adding a `Stat` on `LibraryPaths` to "detect" missing library files, or clearing `LibraryPaths` when the paths no longer resolve — both reintroduce the resurrection loop. A user who wants the link back should redownload or replace the episode, which resets `LibraryPaths` through the merge in decision 27.

**One exception, see decision 34:** when the *entire download root* is detected as swapped (moved/trashed/replaced folder), `clearLibraryPathsAfterRootSwap` wipes the records once. That is a different event from a per-file deletion and is edge-triggered, so it cannot become the loop this decision guards against.
