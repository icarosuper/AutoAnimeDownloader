# 94. A stalled torrent is dropped through the failure path, and only one that is *trying* counts as stalled

**Location:** `daemon/stalled.go` (`dropStalledTorrents`, `stalling`, `stalledTorrentTimeout`),
called in `daemon/verification.go` right after `backend.List()`.

Negative fact: **there is no** retry policy of its own, blacklist, attempt counter or config for
the threshold. A torrent at zero peers for more than an hour enters `HandleTorrentFailure`, the
same function that handles a torrent rain stopped with an error — `download_failed` webhook,
leaves the session keeping the bytes, and the next pass searches again
([#24](024-a-failed-torrent-is-dropped-from-the-session-and.md)).

**What looks wrong, part one:** dropping instead of pausing. Pausing looks gentler and frees the
same `max_concurrent_downloads` slot. It does not work: for the queue, paused is "paused by the
user", it occupies no slot and **never rises on its own** (`torrents/queue.go`). The stalled
download would become stalled, silent and permanent — which is exactly the state this function
exists to end.

**What looks wrong, part two:** the status filter. Only `downloading` and `downloading_metadata`
are candidates. It looks like an oversight not to cover `verifying`/`allocating`, but zero peers
on those two is local work in progress, not a dead swarm. And the three excluded are mandatory,
not optional: a **queued** torrent is at zero peers because it has not even started
(`queue.markQueued`), **paused** because the user told it to stop, and **complete** because nobody
is pulling from it. Without that filter the function would drop the entire queue an hour after
the app came up.

**Why the clock is package-level and in memory:** only the pass writes it and nothing outside the
daemon needs to read it. A restart re-observes stalls from zero, and that costs at most one extra
`check_interval` cycle. The prune at the end of each call is what stops a stamp from weeks ago
from immediately dropping a torrent that stalled, came back and stalled again.

**Do not confuse with `frontend/src/lib/stores/stallTracker.ts`.** Both measure the same thing and
do not talk: the frontend's fires the "no seeds" chip at 10 minutes and dies on a page reload; the
daemon's drops the torrent in one hour. The thresholds are different on purpose — painting a
warning and dropping a download do not deserve the same patience.

**Don't "fix" by:** promoting the threshold to config with no request (the cost of being wrong is
one extra pass, not a lost download — the bytes stay), nor turning the drop into a blacklist. #24
still holds: if the re-add becomes observed churn, then persisted state is discussed.
