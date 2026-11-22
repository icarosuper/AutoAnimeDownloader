# 61. A pack episode is immune to the limit prune AND to the watched keep-set

**Location:** `daemon/episodes.go` — `handleAlreadySavedEpisode` (deletion guard), `buildWatchedKeepSet` (keep-set guard).

**What it looks like:** `handleAlreadySavedEpisode` refuses to delete a saved episode with `IsBatch: true` even when it is above the per-anime limit; `buildWatchedKeepSet` never includes a pack episode in the candidates for "keep even if watched".

**Why it's right:** a pack is one torrent — deleting the record of ONE of its episodes frees no byte, the whole torrent keeps seeding on disk. Without the guard in `handleAlreadySavedEpisode`, the cycle after the one that brought the pack would delete the records above the limit, the space gate would reopen (the record vanished, the byte did not), and the next cycle would try to download again — a download-and-delete loop over a torrent that never left disk. Without the guard in `buildWatchedKeepSet`: `watched_episodes_to_keep` has no granularity **inside** a pack — keeping 3 of a pack's 100 episodes does not keep 3 episodes, it keeps the whole pack (`canRemoveTorrent` returns `false` for the entire group while ANY of the 100 records is outside the delete-set), so including pack episodes in the keep-set does not change the result, it only confuses the count.

The counter (`downloadedEpisodes`) still increments for pack episodes **below** the limit — only deletion above the limit is ignored. It is that normal increment, plus immunity to the prune, that gives the **rotation of successive packs with no new config**: the current pack accumulates records past the limit (immune), the next cycle searches again, `pickBatches` picks the next still-uncovered pack, and so on — only braked by `max_batch_torrent_size_gb` (per torrent) and `checkDiskSpace`.

**Don't "fix" by:**
- Deleting pack records above the limit "so the limit always holds": the torrent stays on disk, and the next cycle downloads again (download-and-delete loop).
- Giving the keep-set granularity inside a pack "to honour `watched_episodes_to_keep` to the letter": the real unit is the torrent, not the record — there is no "keep 3 episodes" of a single file.
