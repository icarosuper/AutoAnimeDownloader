# 67. The verification pass is one at a time, and a concurrent one is dropped — not queued

`AnimeVerification` takes `verificationMu.TryLock()` and **returns immediately** if another pass is running.

The pass reads `episodes.json` **once** at the start (phase 1) and only writes at the end (phase 3, sequential). Two simultaneous passes — the loop's and `POST /check`'s, two `/check`s, or a `/check` right after an `UpdateInterval` — see the same stale list: the second sees nothing of what the first just downloaded and downloads everything **again**. That is how an anime already covered by a pack (`01 ~ 11`, episodes 6..11 recorded) gained loose torrents of episodes 10 and 11, and the second pass's write overwrote the pack records with the loose hashes — two torrents of the same episodes on disk and the screen showing the pack and the loose ones side by side.

Dropping, not queueing, is deliberate: the dropped pass would read exactly the state the in-flight pass is correcting, and the next loop tick covers what is left. Queueing would only postpone the same stale read.

**Don't "fix" by:**
- Reloading `episodes.json` in the middle of the pass "to see what the other saved" — the window stays open between the read and the write, and the two writes still fight.
- Swapping for `Lock()` (queue) — becomes a queue of redundant passes that run with stale state, each holding the next.
- Taking `TryLock` out because "`POST /check` already runs in a goroutine" — the goroutine is what creates the concurrency, not what solves it.
