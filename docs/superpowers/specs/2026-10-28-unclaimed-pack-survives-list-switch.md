# Unclaimed extras keep a pack after a list switch

**Date:** 2026-10-28
**Status:** observed, not designed — no locked fix
**Origin:** switching `anilist_usernames` from JediahSk (18 titles, including K-ON!) to D4NIDx
(5 titles, no K-ON!) left `[MTBB] K-ON! S1 (BD 1080p)` downloading. Other off-list torrents
left. The user expected the whole previous list to stop.
**Scope:** `internal/daemon/episodes.go` (`hasUnclaimedContent`, `canRemoveTorrent`,
`forgettableKeys`, `handleSavedEpisodes`, `identifyEpisodesNotInWatching`). Automatic pass
only — `force` is out of scope. No new endpoint, no CLI, no config field until a design
locks one.

This is the extras/OVA case of [decisions.md #74](../../agents/decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records),
not a failed list fetch and not a second cour.

---

## What happened

`delete_watched_episodes` is true. `delete_statuses` is empty. `download_statuses` is
`CURRENT` / `REPEATING`. The automatic pass never passes `force` into
`removeEpisodesAndLinks`.

| time | event |
|------|--------|
| 16:24:33 | Pass on JediahSk, 18 animes |
| 16:24:48 | K-ON! (AniList media **5680**) added. Torrent name `[MTBB] K-ON! S1 (BD 1080p)`, hash `fd9da971…`. Records stored as `K-ON! 1-19` (`batch_start=1`, `batch_end=19`) from the file list — extras/OVA on the same media, not K-ON!! (a different id). `anime_total_episodes=13`. 11 records, episodes 3–13 (progress was already 2) |
| 16:33:29 | Still JediahSk; K-ON! 3–13 skipped as already downloaded |
| 16:33:55 | Pass on D4NIDx, 5 animes, no K-ON! |
| 16:33:59 | `Removed torrent (seeding copy)` for the rest of the previous list |
| 16:34:00 | `fd9da971…` is **not** removed. The queue resumes it |

`downloaded_episodes` still holds the 11 K-ON! records after the D4NIDx pass.

The list switch did stop **new** K-ON! work. What kept going is the torrent already in rain.

---

## Why the torrent stayed

Off-list deletion is the same path as "watched, drop it":

1. `identifyEpisodesNotInWatching` returns every saved key that is not in this pass's
   `checkedEpisodes` and is not `ManuallyManaged`. All 11 K-ON! keys qualify.
2. `handleSavedEpisodes` only runs that delete when `DeleteWatchedEpisodes` is true.
3. `canRemoveTorrent` requires every hash sibling in the delete set **and**
   `!hasUnclaimedContent`.
4. `hasUnclaimedContent` is `claimed < declaredSpan`. Claimed is
   `max(record count, AnimeTotalEpisodes)` per `anime_id`. Here `span=19`,
   `claimed=13` → unclaimed.
5. `forgettableKeys` then **keeps** the records of a held pack, so a later cour can still
   adopt it (`findCoveringPack` needs the range).
6. Rain still has the torrent. The queue treats it as a live download and resumes it.

The other JediahSk torrents had `claimed == span` (or no declared range), so they left.

K-ON!! is a separate AniList media. The six files past episode 13 are extras/OVA/specials
on media 5680. #74 cannot tell that gap from "cour 1 of a 1–23 season pack, cour 2 not
born yet".

---

## What this is not

- Not a stuck AniList username. The D4NIDx pass really saw 5 animes.
- Not `delete_watched_episodes` off. It is on; the singles and fully-covered packs did
  leave.
- Not a missing `force` on the automatic pass. #74 is explicit: the automatic pass never
  uses it. Manual delete on Downloads already bypasses the guard
  (`TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard`).
- Not "the pack belongs to two lists". Identity is `media_id`; the new list simply does
  not contain 5680.

---

## The trap

Treating "one `anime_id`, every record in the delete set" as enough to drop the torrent
would delete a 1–23 season pack when cour 1 leaves `CURRENT` before cour 2 exists. That
is the case #74 exists to protect
(`TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent`,
`TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`).

The two situations are identical in the data the guard sees:

| | K-ON! extras (this bug) | cour 1 of a season pack (#74) |
|--|--|--|
| `anime_id`s on the hash | one | one (cour 2 has no records yet) |
| `AnimeTotalEpisodes` | 13 | 11 |
| declared span | 19 | 23 |
| all records in the delete set | yes | yes |
| content past the entry's total | extras/OVA | the next cour |

---

## Workaround

Delete the torrent from Downloads. That path calls `RemoveTorrentWithEpisodes` with
`force`, so the guard does not apply.

---

## Open

No design is locked. A fix has to split extras-on-this-media from a future cour without
reopening #74. Candidates that have **not** been measured:

- Off-list (`identifyEpisodesNotInWatching`) is a different deletion class than "this
  entry watched its own episodes".
- A FINISHED single-cour with no sequel in `relations` can treat files past
  `AnimeTotalEpisodes` as extras of this media.
- The automatic pass still must not `force` a live season pack.

Pin any chosen rule with a test next to `TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`
that uses K-ON!'s numbers (span 19, total 13, one `anime_id`, every record in the delete
set) and states the intended outcome.
