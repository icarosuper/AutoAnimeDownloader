# 128. download_failed reports a real failure once per episode and torrent; not found and a full disk are waiting, not failures

**Location:** `internal/daemon/episodes.go` — `processAnimeEpisodes` (the `len(candidates) == 0`
branch sends nothing; a disk-full refusal skips the `failed` map); `internal/daemon/helpers.go` —
`HandleTorrentFailure`; `internal/daemon/health.go` — `firstFailure`, `clearStuck`. Pinned by
`TestDownloadFailed_OncePerEpisodeAndTorrent`, `TestProcessAnimeEpisodes_NoMagnets_SkipsNewEpisodeWebhook`,
`TestProcessAnimeEpisodes_DiskFullDoesNotNotifyDownloadFailed`.

**What it looks like:** a search that found nothing, or an add refused for lack of space, sends no
`download_failed`; a torrent that keeps failing is reported only the first time.

**Why it's right:** before this, `download_failed` went out on every pass for all of it, and almost
none of it was actionable at that moment. "No torrent found" fired every `check_interval` per
episode — also on passes the miss backoff skipped, and for awaiting-earlier episodes that were never
searched — while a weekly episode is routinely missing for the first hours after it airs. A full disk
is a state of the volume: the fix is the same for every episode, and it repeated every pass while
the disk stayed full. Firing only once instead would have been wrong the other way: the user would
see one "failed" and then silence, even after the episode downloaded. So failure is separated from
waiting:

- not found → `episode_stuck`, once it lasts 48 h (per-episode clock);
- disk full → `disk_low`'s `blocked` level, once until space is freed;
- add refused / torrent dropped → `download_failed` once per (episode, infohash), persisted in
  `notification_state` (`failed_torrents`, [#127](127-a-state-alert-fires-on-the-edge-and-remembers-it.md)) and pruned when the episode is
  organized. The key is the hash, never the reason text: `stalled: no peers for 1h0m` changes on
  every drop. A **different** torrent failing for the same episode is new information and fires.

**Breaking for subscribers:** whoever subscribed `download_failed` stops receiving "no torrent
found" and "insufficient disk space" (the reason constants are gone). The release notes must say so.

**Don't "fix" by:**
- Bringing back the not-found push "because the user wants to know": `episode_stuck` tells them once
  it matters, and the report screen shows it immediately.
- Keying the dedup on the reason text: every stall has a new duration in it, so nothing would dedup.
- Deduping per episode only: a new torrent failing after a dead one was replaced is news.
