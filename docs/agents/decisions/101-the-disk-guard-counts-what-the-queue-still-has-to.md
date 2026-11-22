# 101. The disk guard counts what the queue still has to write, not only free space

**Location:** `daemon/diskbudget.go` (`diskBudget`, `newDiskBudget`); `checkDiskSpace`'s
`reservedBytes` in `daemon/helpers.go`; built once per pass in `daemon/verification.go` and in
`DownloadStandaloneAnime`; used by `packsThatFit` and `attemptDownloadWithRetries`;
`files.EpisodeStruct.TorrentSize`; `torrents.InfoHash`.

A clean run on a disk with 99 GB free and a tight slack floor (~6 GB of headroom) queued
58 torrents, ~130 GiB. The guard was right about every big pack (29 refused) and wrong about the
sum: it compared each torrent with the free space NOW, and a torrent only takes its space as it
downloads (rain creates sparse files). Each ~1.4 GiB loose episode fit on its own.

**The rule:** a torrent fits when `size + reserved` fits, where `reserved` is what every incomplete
torrent of the session still has to write plus everything this pass already added.

- **Per pass, shared by the parallel animes.** `reserve` checks and counts under one lock — two
  animes must not both take the last free bytes.
- **Keyed by infohash** (`torrents.InfoHash` on the magnet, before `Add`). The N episodes of a pack
  add the same magnet N times and pay once; a torrent already in the session pays nothing.
- **A queued torrent's size comes from the record.** `BytesTotal` is 0 until the metadata arrives,
  and a queued torrent never started. So the record now keeps the size Nyaa announced
  (`torrent_size`). Remaining = `BytesTotal` (or the recorded size) minus what's written, with the
  bitfield ratio as the written amount when it's larger (a paused torrent zeroes `BytesCompleted`).
- **`packsThatFit` asks the same budget.** Asking free space alone there would let a pack win the
  window and lose it at the add — #97's bug again.

**Cost accepted:** a torrent with neither metadata nor a recorded size (records older than
`torrent_size`, hand-pasted magnets) reserves nothing until its metadata arrives.

**Amended by [#111](111-a-pack-that-loses-at-reserve-does-not-keep-the.md):** a pack that
passes `packsThatFit` and then loses at `reserve` no longer keeps the window. The next pack or
the per-episode candidates run in the same pass.

**Not here:** the queue (`torrents/queue.go`) doesn't gate promotion by disk. The guard stops the
over-commit when torrents are added; a queue already over-committed (from before this) still
downloads everything it holds.

**Manual downloads pass `reservedBytes = 0`** (`manual_download.go`): a magnet the user pastes is
their call, and its size is usually unknown anyway.

**Don't "fix" by:** summing reservations per record instead of per hash (a pack would count once
per episode); checking free space alone in `packsThatFit`; or splitting `reserve` into a check and a
separate count (the parallel animes would race between the two).
