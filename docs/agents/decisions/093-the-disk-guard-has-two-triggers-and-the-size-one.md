# 93. The disk guard has two triggers, and the size one still applies with `min_free_disk_gb = 0`

**Location:** `daemon/helpers.go` (`checkDiskSpace`, `DiskLow`), called in `daemon/episodes.go`
(`attemptDownloadWithRetries`, once before the loop and once per candidate) and in
`daemon/manual_download.go` (`addAndPrioritize`). `GET /status` `disk_low` is `DiskLow`.

`checkDiskSpace` receives `torrentSizeBytes` and refuses for two different reasons:

1. **the torrent does not fit** in `free - reserved` — runs whenever the size is known, including with
   `min_free_disk_gb = 0`;
2. **leftover after the torrent and the reservation would fall below the GiB floor** — only runs with
   `min_free_disk_gb > 0`.

**What looks wrong:** a config documented as "`0` = off" that does not turn the whole guard off.
Deliberate. The GiB floor is the user's policy ("I want 10 GiB of slack"), and turning policy off is
their right. "This torrent is bigger than the disk" is not policy, it is arithmetic: the download
will fill the volume and stall in the middle either way, and letting it in fulfills no wish.

The slack is a quantity of bytes. It used to be `min_free_disk_percent`, which measured a fraction
of the volume: 10% of 50 GB is 5 GB, 10% of 1 TB is 93 GB. Same config, unrelated meanings, and a
catch-up queue of ~80 GB on a 928 GiB disk tripped the guard with 177 GB still free. The field is
now `min_free_disk_gb`, default 10, `_gb` = GiB (2^30) like the torrent size ceilings. An old
`min_free_disk_percent` is ignored (unknown JSON key) and the install gets 10 GiB — no migration
of the old percentage, which would have preserved the 93 GiB floor this change exists to drop.

`disk_low` is this add-gate for a torrent of size 0, including reserved. The Status banner and the
disk-card color follow that flag.

**Amended by [#101](101-the-disk-guard-counts-what-the-queue-still-has-to.md):** the size is compared with the free space MINUS what the torrents already added still have to write, not with the free space alone.

**Why the size did not come earlier:** it always existed on the Nyaa result
(`nyaa.TorrentResult.Size`, the same datum as the `max_batch_torrent_size_gb`/
`max_episode_torrent_size_gb` ceilings), but it was lost on the way — `resolvedMagnets` stored
`magnets []string`. Today it stores `candidates []nyaa.TorrentResult` exactly so `Size` reaches
`Add`. If someone one day "simplifies" the field back to a list of strings, the guard goes blind
again.

**Size 0 = unknown, and it passes.** That is the pasted-magnet case: the size would only appear
after the metadata, and refusing for not knowing would bar a legitimate download. Then the guard
is the GiB floor it always was — see `ManualDownloadEpisodeWithMagnet`/`ManualDownloadAnimeWithMagnet`,
which pass `0` on purpose.

**The check is per candidate, not one before the loop.** Each candidate has a size, and the second
in the list may fit where the first did not — skipping one does not end the attempt. The
size-less check **before** the loop still exists, and it is something else: a disk already full
today means no candidate will do, and trying N times would only fill the log.
