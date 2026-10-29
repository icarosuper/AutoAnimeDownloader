# Unclaimed extras keep a pack after a list switch

**Date:** 2026-10-28
**Status:** measured 2026-10-29 — method C locked; marker list from the sample; `PackFileRange` not changed yet
**Origin:** switching `anilist_usernames` from JediahSk (18 titles, including K-ON!) to D4NIDx
(5 titles, no K-ON!) left `[MTBB] K-ON! S1 (BD 1080p)` downloading. Other off-list torrents
left. The user expected the whole previous list to stop.
**Scope:** extras must not stretch the range `PackFileRange` stores. The #74 guard
(`hasUnclaimedContent`, `canRemoveTorrent`, `forgettableKeys`) stays. Automatic pass still
never uses `force`. No new endpoint, no CLI, no config field.

This is the extras/OVA case of [decisions.md #74](../../agents/decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records),
not a failed list fetch and not a second cour.

**Do not change `PackFileRange` yet.** The sample landed; the remaining lock is how an
included OVA (user addendum) is stored so `S00E19` does not become episode 19.

---

## What happened

`delete_watched_episodes` is true. `delete_statuses` is empty. `download_statuses` is
`CURRENT` / `REPEATING`. The automatic pass never passes `force` into
`removeEpisodesAndLinks`.

| time | event |
|------|--------|
| 16:24:33 | Pass on JediahSk, 18 animes |
| 16:24:48 | K-ON! (AniList media **5680**) added. Torrent name `[MTBB] K-ON! S1 (BD 1080p)`, hash `fd9da971…`. Records stored as `K-ON! 1-19` (`batch_start=1`, `batch_end=19`) from the file list. `anime_total_episodes=13`. 11 records, episodes 3–13 (progress was already 2) |
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

The first write-up treated the gap 14–19 as six extras/OVA files. The Nyaa file list says
otherwise — see below.

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

The two situations are identical in the data the **guard** sees today:

| | K-ON! extras (this bug) | cour 1 of a season pack (#74) |
|--|--|--|
| `anime_id`s on the hash | one | one (cour 2 has no records yet) |
| `AnimeTotalEpisodes` | 13 | 11 |
| declared span | 19 | 23 |
| all records in the delete set | yes | yes |
| content past the entry's total | extras/OVA (misread) | the next cour |

They are **not** identical in the file list. That is the split method C uses.

---

## Workaround

Delete the torrent from Downloads. That path calls `RemoveTorrentWithEpisodes` with
`force`, so the guard does not apply.

---

## Discovery: the stored range was a min–max lie

Pack: `[MTBB] K-ON! S1 (BD 1080p)`, [nyaa.si/view/1816035](https://nyaa.si/view/1816035),
hash `fd9da97199bf9a0ff811c76c273b63cc07824eed`.

Video files:

- `[MTBB] K-ON! - 01` … `- 13` — the 13 TV episodes
- `[MTBB] K-ON! - S00E19 (OVA)` — **one** extra. No files 14–18.

`PackFileRange` (`nyaa/nyaa.go`) takes min..max of `ExtractEpisodeNumber` over video files.
The first pattern in `reEpisodePatterns` is `S\d{1,2}E(\d{1,4})`, so `S00E19` becomes
episode **19**. Span stored: 1–19. `hasUnclaimedContent` then sees 13 < 19.

`PackFileRange` already drops files with **no** readable number (NCOP, Opening). A numbered
extra still stretches the range. That is the ceiling #84 already named, from the other
side: extras folders were left out of a per-folder count because `Creditless Opening 1`
reads as episode 1.

Same title, two other groups, same shape:

| pack | extra file |
|------|------------|
| [koala] K-ON! Season 1, [nyaa.si/view/1816929](https://nyaa.si/view/1816929) | `Specials/K-ON.S00E19…` |
| [Some-Stuffs] K-On! Season 1, [nyaa.si/view/2057836](https://nyaa.si/view/2057836) | `K-On! 14` — **no** OVA/`S00` marker |

Future-cour files look like the main episodes, contiguous past the entry total. Mushoku
Tensei S1 cour 1 is 11 episodes; a season pack is `- 01` … `- 23` or `S01E01` … `S01E23`.
EMBER's S1+S2 BD also has `S01E24 [OVA]-Eris the Goblin Slayer` sitting **on** that
contiguous axis with an OVA marker.

The **pack name** is a weak signal. `[MTBB] K-ON! S1` and `[MTBB] Mushoku Tensei S1` both
say `S1`; the second covers two cours. #84 already measured that 19 of 24 packs declare no
range in the name. Method C reads **file** names.

`hasUnclaimedContent` only sees `BatchStart`/`BatchEnd` already stored. The file list is
gone by deletion time unless the torrent is re-listed. C therefore runs at **ingest**
(`PackFileRange` / the assign path that writes the range), not as a new branch of the
guard.

---

## Locked: method C — extras do not stretch the declared span

Addendum after the sample: OVA / OAD / `.5` bonus episodes are claimed content; OP / ED /
NCOP / NCED / PV / Menu / Creditless are not. An included OVA still must not feed
min–max (`S00E19` is not episode 19). See the measurement.

Rule, from the 2026-10-29 sample:

1. OP/ED-class files (`NCOP`, `NCED`, `Opening`, `Ending`, `Creditless`, token `OP`/`ED`,
   `PV`/`Teaser`/`Preview`, `Menu` / `BD menu`, parent folder `NC` / `Creditless`) **do
   not enter** the range `PackFileRange` returns. Most already have no integer.
2. A file numbered `total+1, total+2, …` in the same shape as the main episodes **stays**
   in the range. That is the next cour. #74 keeps seeing unclaimed content.
3. An unmarked extra at `total+1` with run length 1 (`K-ON! - 14`, `EP 14 Live House!`,
   DeadFish `- 25`) is a third criterion — three same-entry packs in 70.
4. OVA / OAD / `S00E` / `.5` are claimed content. Their number still must not feed
   min–max (`S00E19` is not episode 19). How that is stored is the remaining lock
   before `PackFileRange` changes.

Effects on the observed pack, once (4) lands: stored range becomes 1–13, `claimed == span`,
the torrent leaves on a list switch **and** when the same list finishes the anime.

#74 tests stay as they are. Add a pin next to
`TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack` that uses K-ON!'s **files** (13 TV +
`S00E19 (OVA)`), not the phantom span 19, and states the torrent leaves. Keep the 1–23 /
total 11 case green.

`hasMovieMarker` is too wide on a **file** name (`Special Training!`). File-level extras
use `OVA`/`OAD`/`S00E`, not the torrent-name helper.

Do not cap `declaredSpan` at `AnimeTotalEpisodes` without looking at files. That deletes
the 1–23 pack under cour 1.

Do not `force` the automatic pass. Do not treat "one `anime_id`, every record in the
delete set" as enough.

---

## Rejected (kept for the next chat)

- **A — off-list is a different deletion class.** Presence via `GetAnimeInfoByIDs`: a
  media on no tracked account (any status) bypasses the unclaimed guard; AniList failure
  keeps the pack. Matches "the whole previous list should stop", including a live
  split-cour pack. Left on the table. C fixes the lying range; A is a different slice
  (account gone vs extra on a pack still in range).
- **B — username-set change as an event.** Too narrow: removing one anime without
  swapping accounts, or finishing K-ON! on the same list, still sticks.
- **C via sequel-gap.** Unclaimed is extras iff a TV sequel exists and cannot fit in the
  gap; no sequel → keep (#74). K-ON!! has 13 episodes and the hole is 6, so it would have
  dropped this pack — but the hole was phantom. File names are the cheaper, local split.
  Sequel walk only stores PREQUEL today (`seriesLink`).

---

## Measurement (landed 2026-10-29)

Instrument: `src/internal/nyaa/live_extras_measure_test.go` (`TestLive_ExtrasVsNextCour`,
offline pins `TestExtrasBuckets_KnownPackFiles`). Same protocol as #84: first 8 pack
rows, seeder order, 1.5s between requests. User addendum applied while bucketing: OVA /
OAD / `.5` bonus episodes are episode-like; OP / ED / NCOP / NCED / PV / Menu /
Creditless are not.

```
AAD_LIVE_NYAA=1 go test ./src/internal/nyaa/ -run TestLive_ExtrasVsNextCour -count=1 -v -timeout 25m
```

70 unique packs, 1886 video files. Queries and AniList totals:

| query | total | also |
|-------|-------|------|
| `K-ON! S1` / `K-On! batch` | 13 | |
| `mushoku tensei batch` / `mushoku tensei s1` | 11 (cour 1) | 23 (S1 whole) |
| `kaguya-sama batch` | 12 | |
| `steins gate batch` | 24 | |
| `haruhi batch` | 14 (2006) | 28 (2009) |
| `fate ubw batch` | 12 (cour 1) | 25 (season) |
| `hyouka batch` | 22 | |
| `shingeki no kyojin batch` | 25 | |
| `konosuba batch` | 10 | |
| `one punch man batch` | 12 | |

Pack-name kind (not used to classify files): range=23, s1-only=22, none=21, part=4.

Auto buckets (raw, including complete-series / wrong-season noise):
`main=1228 next_cour=312 unnumbered=107 op_ed=95 ova_s00=92 isolated=38 pv_menu=7 half_ep=4 ova_on_axis=3`.

Markers that showed up: Opening/Ending, Creditless, NCOP/NCED, folder-NC, OP/ED, PV,
Menu, folder Specials/Extras, OVA/OAD, S00E, `.5`.

### Same-entry extras (hand-reviewed)

Complete-series and wrong-season hits (Kaguya S3 ep 13 vs S1=12, Konosuba S3 / Bakuen vs
S1=10, AoT S2–S4 numbered 26–87 vs S1=25, Haruhi 2009 1–28 vs 2006=14) inflate
`next_cour` and `isolated`. The bug class is the same-entry extra:

| shape | packs in this sample | example |
|-------|----------------------|---------|
| `S00E##` with `(OVA)` | 1 | [MTBB] K-ON! `S00E19 (OVA)` → ep **19**, range 1–19 |
| `S00E##` with no OVA word | 1 | [MiniMTBB] K-ON! `S00E19` → still ep 19 |
| unmarked `total+1` | 3 | HonokaSubs `K-ON! - 14`; Kira-Fansub `EP 14 - Live House!`; DeadFish Steins;Gate `- 25` |
| OVA on the contiguous axis | 3 | EMBER `S01E24 [OVA]`; Judas/Trix Steins;Gate `S01E25 (OVA)` / `- 25 (OVA)` |
| OVA as `.5` | 3 | MTBB Mushoku `- 17.5 (OVA)`; `S01E17.5`; Hyouka `11.5` / `- 11.5` |
| OVA with no number | several | Kaguya `Season 2 OVA.mkv`; OPM `Ova1.mkv`…`Ova6.mkv`; Fate UBW `- 00.mkv` (Sunny Day) |
| folder `Specials` / `NC` / `Extras` / `Creditless` / `BD menu` | common | NCOP/NCED, Creditless Opening N, `OP.mkv`, Extra001–016 |

koala / Some-Stuffs were not in the first 8 seeder rows this time. HonokaSubs and
Kira-Fansub filled the unmarked-14 slot.

### Answers

**1. Exclude list (showed up, and they are OP/ED-class):** `NCOP`, `NCED`, `Opening`,
`Ending`, `Creditless`, token `OP`/`ED`, `PV`/`Teaser`/`Preview`, `Menu` / `BD menu`,
parent folder `NC` / `Creditless`. BD `Extra001`–`Extra016` have no episode number
today, so they already stay out of the range.

**Include list (showed up, user addendum):** `OVA`/`OAD`/`ONA` on the file, `S00E`,
`.5`/`.25`/`.75`, parent folder `Specials` when the file is an OVA/special episode.

`hasMovieMarker` is too wide on a **file** name: Kira-Fansub `EP 03 - Special Training!`
matches `\sspecial\s`. File-level extras use `OVA`/`OAD`/`S00E`, not the torrent-name
helper.

**2. Unmarked extra (`K-On! 14`):** three same-entry packs in 70. Real, not a one-off.
A third criterion (isolated `total+1` with no marker, run length 1) is justified for
Honoka/Kira/DeadFish. `S00E` already catches MiniMTBB without needing `(OVA)`.

**3. Next cour survives dropping extras.** MTBB Mushoku S1 files `- 12` … `- 23` have
no extra marker; dropping OVA/OP/ED leaves 1–23, which is the #74 range under cour 1
(total 11). EMBER's `S01E24 [OVA]` sits on top of that axis; dropping it leaves 1–23.
No next-cour file in the sample carried an OP/ED marker.

### What including OVA does to min–max

`.5` files currently **do not stretch** `PackFileRange`: `ExtractEpisodeNumber` is
integer-only, so `- 17.5 (OVA)` and `Hyouka - 11.5` yield no number (or `S01E17.5`
truncates to 17). Episode `00` is also dropped (`episodeNum > 0`).

`S00E19` and `S01E25 (OVA)` **do** stretch, because they parse as 19 and 25. Putting
those files in the stored range keeps `claimed < span` on K-ON! (13 < 19) and on
Steins;Gate (24 < 25). Including OVA as **claimed content** is compatible with method
C only if that number does not enter min–max — `S00E` is a season-0 extra, not episode
19.

### Instrument notes (not production)

- `isExtrasFolder` on the whole path false-positives when the torrent root is
  `(S1 + OVA + Specials)`. Immediate parent folder only.
- `\bS00\b` misses `S00E19` (`E` is a word character). Match `S00E`.
- `ExtractEpisodeNumber` has ` 05.mkv` and no ` 05.mp4`, so MystVortex `K-On! - 01.mp4`
  is unnumbered. Adjacent cap, not this change.

Lock the include/exclude lists above and the pin that K-ON! 13 TV + `S00E19 (OVA)`
stores 1–13 (torrent leaves) while Mushoku 1–23 / total 11 stays. Then change
`PackFileRange`.
