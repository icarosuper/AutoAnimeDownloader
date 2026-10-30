# Off-list bypass of the unclaimed pack guard

**Date:** 2026-10-30
**Status:** designed, not implemented — branch `flatten-restart-packs`
**Origin:** a multi-season pack downloaded under S1 stays on disk after S1 leaves watching, even
when no other season of that pack is in anyone's watching list. The keep is only wanted while
some claimed season is still in watching, or when another season of the pack is adopted in the
same pass.
**Scope:** `internal/daemon/episodes.go` (`removeEpisodesAndLinks`, `canRemoveTorrent`,
`forgettableKeys`, `handleSavedEpisodes`, `deleteEpisodesByStatus`),
`internal/daemon/verification.go` (pass the watching set and `newEpisodes` into status
deletion), tests in `internal/daemon/episodes_test.go`. Docs: [decisions.md](../../agents/decisions.md)
**#117** (amends #74, does not replace it), [architecture.md](../../agents/architecture.md)
deletion paragraph, [TODO.md](../../TODO.md) Method A line, #116's "keeps the torrent" sentence.
No new endpoint, config field, or CLI command.

---

## The problem

[#74](../../agents/decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records)
compares the pack's declared span with the episodes **claimed** by the group's entries. A S1+S2
pack stored as `1..24` under a 12-episode S1 has unclaimed content, so `canRemoveTorrent` refuses
and `forgettableKeys` keeps the records. That is what [#116](../../agents/decisions.md#116-restart-per-season-packs-flatten-onto-the-concatenated-axis-organize-slices-by-anilist-id)
relies on: completing S1 with S2 still absent from watching leaves the torrent for S2 to adopt
later via `findCoveringPack`.

The keep is too broad. After S1 leaves watching, and nobody has any season of that pack in
watching, the files sit on disk until the user happens to add S2 — or forever, if they never do.

## What this is not

- Not dropping `hasUnclaimedContent`. Watched-episode pruning (`delete_watched_episodes`) can put
  every **record** of S1 in the delete set while S1 is still `CURRENT`. Without the guard, that
  prune would take S2's files with it. The guard stays; it gains a second gate.
- Not storing sequel AniList ids at S1 download ([#116](../../agents/decisions.md#116-restart-per-season-packs-flatten-onto-the-concatenated-axis-organize-slices-by-anilist-id)
  / [#71](../../agents/decisions.md#71-anilist-has-no-franchise-id-the-prequel-chain-is-the-only-source-and-searching-by-name-is-a-trap)).
  If S2 is in watching this pass, `findCoveringPack` writes S2 records into `newEpisodes` and those
  records survive the delete set. No SEQUEL walk at deletion time.
- Not a grace period. The next pass after S1 has left and S2 has not been adopted deletes. Adding
  S2 later re-downloads. If both list changes land before the next pass, S2 adopts in that pass
  and the torrent stays.
- Not a change to manual torrent deletion. `force` still bypasses the guard
  (`RemoveTorrentWithEpisodes`).
- Not a change to per-episode API delete. That path does not know the watching set and must not
  drop a multi-season pack on a click (`watching == nil` keeps today's #74 behaviour).

## The rule

A pack with unclaimed content is held **only while** at least one claimed `anime_id` of the
group is in this pass's watching set.

Watching set = media ids in the pass's download list: the union of configured accounts'
`DownloadStatuses` (default `CURRENT`, `REPEATING`) plus standalones processed this pass.
One account having the anime in a download status is enough to keep ([#43](../../agents/decisions.md#43-an-animes-identity-is-mediaid-status-is-a-per-account-question-download--or-deletion--and)
OR). PLANNING is not watching unless the user put it in `DownloadStatuses`.

| Situation | Result |
|---|---|
| S1 still in watching; all S1 records in the delete set (watched prune); S2 not in watching | keep (unclaimed + S1 in watching) |
| S1 left watching; S2 not in watching; no S2 records | delete on this pass |
| S1 left watching; S2 adopted this pass (`newEpisodes` share the hash) | keep (sibling records survive the delete set) |
| S1 left watching; another account still has S1 in `DownloadStatuses` | keep (S1 still in the watching set) |
| Manual `DELETE /torrents/{hash}` | delete (`force`) |
| Manual per-episode delete, `watching == nil` | keep if unclaimed (today's #74) |

Same-pass adoption is already paid for: `handleSavedEpisodes` merges `savedEpisodes ∪ newEpisodes`
before `removeEpisodesAndLinks` ([#74](../../agents/decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records)
defect A). This spec does not add a second adoption path.

Accepted miss: S2 is in watching this pass but the anime was not processed (error, cancel). No S2
records, S1 off-list → the pack leaves. The next pass re-downloads. No SEQUEL query to cover it.

## Wiring

### Predicate

`removeEpisodesAndLinks` gains `watching map[int]bool`. Nil vs empty is the gate:

- `watching == nil` — caller is not the automatic pass. `hasUnclaimedContent` holds as today.
- `watching != nil` — even empty. Unclaimed holds only if some `ep.AnimeID` of the group is a
  key in the map.

`canRemoveTorrent` and `forgettableKeys` share one helper (name in the implementation, e.g.
`unclaimedHolds`): unclaimed content **and** (watching is nil **or** the group has a watching
anime). `force` still skips both.

`hasUnclaimedContent` / `declaredSpan` stay as they are. Claimed is still the entry's
`AnimeTotalEpisodes`, span is still per `anime_id`.

### Automatic pass

`verification.go` builds one watching map from `animes`
(`anilistResponse.Data.Page.MediaList`: download-list union plus standalones) and passes that
same map to both deletion calls. The map is always non-nil on this path (empty when the pass
tracked nobody).

`handleEpisodesData` gains `watching`. `handleSavedEpisodes` forwards it to
`removeEpisodesAndLinks`. Unit tests set it: empty map for S1-left / S2-absent, map containing
S1 for keep-while-watching. Leaving it nil in a `handleSavedEpisodes` test is the API default
(unclaimed holds) and would hide a miss of this spec.

`deleteEpisodesByStatus` receives the same map. It also receives `newEpisodes` and uses
`savedEpisodes ∪ newEpisodes`: it runs **before** `handleSavedEpisodes` and today sees only the
pre-pass snapshot, so off-list bypass without S2's new records would delete the torrent S2 just
adopted (defect A on the status path). Status deletion still runs when `DeleteWatchedEpisodes`
is false.

### Manual paths

`RemoveEpisodesWithLinks` passes `watching == nil`.
`RemoveTorrentWithEpisodes` keeps `force=true`; watching is irrelevant.

## Tests

Flip (today they require the torrent to wait for S2 not in watching):

- `TestHandleSavedEpisodes_KaguyaMultiSeasonPackWaitsForS2NotInWatching`
- `TestHandleSavedEpisodes_ContinuousTwoSeasonPackWaitsForS2NotInWatching`

They pass `checkedEpisodes: nil`. After the change they also pass a non-nil empty `watching`
map; the torrent **leaves** and the S1 records are forgotten. Rename them to say the pack is
dropped, not that it waits.

Pin the keep-while-watching case, through `handleSavedEpisodes`: S1 keys in `checkedEpisodes`
and in the delete set, `watching` contains S1, span `1..24`, total 12 → torrent stays, records
stay.

Pin the predicate at `removeEpisodesAndLinks`:

- all records in the delete set, span `1..23`, `watching` empty (non-nil) → torrent **leaves**
- same records, `watching` contains that `anime_id` → torrent **stays**
- `watching == nil` → torrent **stays** (`TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent`
  / `KeepsRecordsOfHeldPack` stay green)

Keep green:

- `TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack`
- `TestRemoveEpisodesAndLinks_RemovesPackDownloadedMidWatch`
- `TestRemoveEpisodesAndLinks_RemovesKOnPackOnceOVAIsOutOfSpan`
- `TestRemoveEpisodesAndLinks_RemovesPackOnceEveryCourClaimsIt`
- `TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard`
- `TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent` (S2 in `newEpisodes` and
  `checkedEpisodes`)

`deleteEpisodesByStatus` needs a pin: S1 deletable by status, S2 records only in `newEpisodes`,
watching contains S2 → torrent stays.

## Docs

- **#117** next after #116. Index line at the top of `decisions.md`. The entry is the second
  gate on #74: off-list bypasses unclaimed; nil watching is the API default; status deletion
  sees `newEpisodes`. Point at this spec's tests.
- **#74** stays. #117 is the amendment, not a rewrite.
- **#116** "Completing S1 with S2 still absent from watching keeps the torrent" becomes: the
  flattened span still exceeds S1's total (so #74 can see unclaimed content); the torrent is
  kept only while a claimed season is in watching (#117). Flattening does not change.
- `architecture.md` deletion bullet: a batch leaves when every record is in the delete set
  **and** (no unclaimed content, **or** no claimed anime of the group is in this pass's
  watching set).
- `TODO.md` "Method A (off-list bypasses the unclaimed guard) stays on the table" — executed
  by this spec; close that sentence.

## Out of scope

- Walking SEQUEL at deletion to keep a pack for an S2 that is in watching but was not
  processed this pass.
- Changing `DownloadStatuses` / `DeleteStatuses` defaults.
- Frontend copy or a setting for "keep multi-season packs after leaving watching".
- Re-ingesting packs already stored with a wrong span (#113 leftover 1–19 K-ON! records).
  Those still need `force` or a re-add; off-list bypass only helps once the anime has left
  watching, which is the case this spec already deletes.
