# 119. `disable_seeding` means no upload at all, and a released torrent is not a lost one

**Location:** `internal/torrents/session.go` — `sessionOptions.noUpload`, `rainConfig`;
`internal/torrents/sessionmanager.go` — `SetUploadDisabled`, the rebuild in `ensure`;
`internal/daemon/jobs.go` — `releaseTorrent`; `files.EpisodeStruct.TorrentReleased`, read in
`selectEpisodes`, `removeEpisodesAndLinks` and `clearLibraryPathsAfterRootSwap`;
`reconcileLibrary(…, disableSeeding)`. Pinned by `TestRainConfigNoUploadClosesEveryUploadPath`,
`TestSessionManagerEnsureRebuildsWhenUploadSettingChanges`,
`TestOrganizeTorrent_DisableSeedingReleasesTheTorrent`,
`TestDisableSeeding_ReleasesTorrentsOrganizedBefore`,
`TestSelectEpisodes_ReleasedTorrentIsNotMissing` and the rest of `daemon/disable_seeding_test.go`.

**What it looks like:** a "disable seeding" switch that also changes how the torrent DOWNLOADS,
rebuilds the whole rain session to apply, and a new flag on every record instead of just
removing the torrent after organize.

**Why it's right:** the request (issue #40) is "don't upload", for legal reasons. Removing the
torrent after organize stops seeding but not uploading: BitTorrent uploads to peers WHILE it
downloads. rain has no "upload off": its upload speed limit treats 0 as unlimited. What works is
never unchoking a peer (`UnchokedPeers` and `OptimisticUnchokedPeers` 0) **and** an empty BEP 6
allowed-fast set (`AllowedFastSet` 0). The allowed-fast set is the pieces a peer may request even
while choked, and rain serves 10 of them by default. With only the unchokes at zero, every peer
still got those 10. rain reads all three once, when the session opens, so changing the setting
rebuilds the session. The cost: peers favor whoever uploads back, so downloads can be slower.

**The flag on the record:** for the pass, a record whose torrent is missing from the session is a
lost download, and it fetches it again ("Redownloading episode: was missing from torrents"). A
released torrent is missing on purpose. Without `TorrentReleased`, every organized episode would be
downloaded again on the next pass. The flag is written BEFORE the removal: a crash in between leaves
a flagged record whose torrent is still there, which the next reconciliation removes. The other order
would leave an unflagged record with no torrent, which would be downloaded again. It stays set if the
setting is turned off later, since the file is still in the library. It is cleared only by a root swap,
because then the library it lived in is gone.

**A multi-season pack goes right away**, unclaimed holds included ([#117](117-off-list-bypasses-the-unclaimed-pack-guard.md)):
whoever turns seeding off doesn't want data parked on disk. If S2 enters watching later it downloads
again. `findCoveringPack` does not read the flag on purpose, since it adopts what is ON DISK and a
released pack is not.

**Don't "fix" by:** using `SpeedLimitUpload` (0 = unlimited); zeroing the unchokes and leaving
`AllowedFastSet`; also stopping the torrent at completion: organize runs right after completion and
removes it, and with no upload a completed torrent has nothing to send anyway, so a stop in between
only adds a state to keep in sync; treating `TorrentReleased` as "in session" in `findCoveringPack`.
