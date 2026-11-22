# 74. The pack-exclusion guard's unit is the torrent's contents, not the list of records

**Location:** `internal/daemon/episodes.go` (`canRemoveTorrent`, `hasUnclaimedContent`,
`declaredSpan`, `forgettableKeys`, `handleSavedEpisodes`, `unclaimedHolds`). The second
gate is [#117](117-off-list-bypasses-the-unclaimed-pack-guard.md). Pinned by
`TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent`,
`TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack`,
`TestRemoveEpisodesAndLinks_RemovesPackDownloadedMidWatch`,
`TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`,
`TestRemoveEpisodesAndLinks_RemovesPackOnceEveryCourClaimsIt`,
`TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard` and
`TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent` (`internal/daemon/episodes_test.go`).

**What it looks like:** the guard that decides whether a shared torrent can leave no longer asks
only "is every record of this hash in the delete set?". It also compares the range declared by the
records (`BatchStart`/`BatchEnd`) with the episodes **claimed** by the group's entries, and
refuses the removal when unclaimed content remains (`hasUnclaimedContent`). Three pieces make the
count close:

- **Who claims is the entry, by its `AnimeTotalEpisodes`** — not the number of records.
  A record is only born for an episode the daemon downloaded: a pack `01-11` taken with progress 5
  has 6 records, and a blocked episode shortens the group the same way. The episodes the entry did
  not record are still hers. An old record does not carry the total (`0`); then the record count
  of that `anime_id` is what is left, which is the behaviour before this fix.
- **`declaredSpan` measures per `anime_id`, not on the union** — each entry stores the range in
  *its* local numbering (#79), so `1..23` under cour 1 and `-10..12` under cour 2 are the same
  23-episode pack; unioning the two would give 34, a pack that does not exist.
- **The pack record that stayed on disk is not deleted** (`forgettableKeys`). It is the only place
  the declared range exists — without it no future cour can adopt the pack (`findCoveringPack`
  requires the range, #78) and the claimed-content count never closes again.

`removeEpisodesAndLinks` gained a `force` parameter for **manual** torrent deletion
(`RemoveTorrentWithEpisodes`, #32): there the unit of deletion is the torrent and who decided was
the user — without that the click would return success without deleting anything. The automatic
pass never uses `force`. On top of that, `handleSavedEpisodes` passes `savedEpisodes ∪ newEpisodes`
to `removeEpisodesAndLinks`, not only the pre-pass snapshot.

**Why it's right:** "every record" is not "all the content". A **season** pack downloaded under
cour 1 (which only has 11 episodes on AniList) writes 11 records but carries 23 episode files —
cour 2's 12 have no record because they have not been downloaded yet. Watching cour 1 put the 11
records in the delete set, the guard concluded "nobody survives" and wiped from disk content the
user never saw. The aggravation of the same pass is the snapshot half: when cour 2 becomes
`CURRENT` and the pass picks the same pack for the new media id, `Session.Add` reuses the infohash
and the new records exist only in `newEpisodes` — invisible to the guard, which deleted the
just-adopted file.

**Why not the record count** (it was the first version of this decision, and it leaked disk): it
confuses "episode of another cour, that nobody downloaded" with "episode of this entry that the
daemon did not need to record". A pack `01-11` of an 11-episode entry, taken with progress 5, has 6
records; `6 < 11` marked unclaimed content **forever**, the torrent never left disk and the records
were deleted anyway — a permanent orphan pack, which is the opposite of the protection the guard
exists to give. Comparing episode **numbers** also does not work: the same hash can have records of
different media ids, each on its own ruler. The entry's total is the only datum that says how much
of the pack belongs to it without needing a conversion.

**Known ceiling (`ponytail:` in the code):** a pack whose range neither the name nor Nyaa's file
list resolved stores `BatchEnd == 0` (unknown, see `hasDeclaredRange` in #79) and stays
undetectable — the guard falls back to the old behaviour. After #84 that is the remainder of a rare
case (detail page down, unreadable file name); before it was **every** pack with no range in the
name.

**Don't "fix" by:** deriving the range from the min/max of the saved episodes (that is the
information that is missing, not one you can reconstruct — it was the bug that motivated
`BatchStart`/`BatchEnd` existing), nor reloading `episodes.json` inside `removeEpisodesAndLinks`
(Phase 3 is sequential on purpose and already has both pieces in memory).
