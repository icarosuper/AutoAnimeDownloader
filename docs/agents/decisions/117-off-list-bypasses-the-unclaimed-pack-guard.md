# 117. Off-list bypasses the unclaimed pack guard

**Location:** `internal/daemon/episodes.go` (`unclaimedHolds`, `canRemoveTorrent`,
`forgettableKeys`, `removeEpisodesAndLinks`, `handleSavedEpisodes`, `deleteEpisodesByStatus`),
`internal/daemon/verification.go` (watching map from `animes`, `newEpisodes` into status
deletion). Pinned by `TestRemoveEpisodesAndLinks_DropsUnclaimedPackWhenWatchingEmpty`,
`TestRemoveEpisodesAndLinks_KeepsUnclaimedPackWhenAnimeStillWatching`,
`TestHandleSavedEpisodes_KaguyaMultiSeasonPackDropsWhenS2NotInWatching`,
`TestHandleSavedEpisodes_ContinuousTwoSeasonPackDropsWhenS2NotInWatching`,
`TestHandleSavedEpisodes_KeepsUnclaimedPackWhileS1StillWatching` and
`TestDeleteEpisodesByStatus_NewEpisodesProtectSharedTorrent`
(`internal/daemon/episodes_test.go`).

**What it looks like:** `#74`'s unclaimed-content hold is gated on this pass's watching set
(media ids in `DownloadStatuses` of any account, plus standalones). A S1+S2 pack stored as
`1..24` under a 12-episode S1 still has unclaimed content, but once S1 has left watching and
S2 was not adopted, `canRemoveTorrent` returns true and `forgettableKeys` forgets the
records. While S1 is still in watching, watched-episode pruning does not drop the pack.
`watching == nil` is the API default and keeps today's `#74` hold. Manual torrent delete
still passes `force`.

**Why it's right:** keeping the torrent after S1 left, in case S2 might enter watching later,
left multi-season packs on disk forever when the user never added S2. Same-pass adoption is
already paid for: `handleSavedEpisodes` merges `newEpisodes`, and status deletion now uses
the same snapshot so it cannot delete the hash S2 just adopted. Sequel ids are still not
stored (#116 / #71). Empty vs nil is the gate: the automatic pass always passes a non-nil
map (`make` even when `animes` is empty).

**Don't "fix" by:** dropping `hasUnclaimedContent` (watched pruning of S1 would then delete
S2's files while S1 is still CURRENT); walking SEQUEL at deletion; treating a nil watching
map as off-list (a per-episode API delete would drop the pack).
