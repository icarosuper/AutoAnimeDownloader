# 113. PackFileRange min-max skips extras/OVA; an isolated total+1 drops; next cour stays

**Location:** `nyaa/nyaa.go` — `PackFileRange`, `skipPackRangeEpisode`, `dropIsolatedTotalPlusOne`; `nyaa/nyaa_regex.go` — `reFile*`; `daemon/search.go` — `packDetail` passes the AniList entry total. Pinned by `TestPackFileRange_ExtrasDoNotStretchTheDeclaredSpan` and `TestRemoveEpisodesAndLinks_RemovesKOnPackOnceOVAIsOutOfSpan`.

**What it looks like:** `PackFileRange(url, totalEpisodes)` still returns min..max, but OP/ED/PV/Menu, `S00E`, OVA/OAD, `.5`, and episode 0 (`S02E00`, `- 00`) files do not feed the span. When `totalEpisodes` is known and the main run is present (some number ≤ total), a single unmarked `total+1` is dropped. A contiguous run past the total stays.

**Why it's right:** `[MTBB] K-ON! S1` stored 1–19 because `S00E19 (OVA)` parsed as episode 19. `hasUnclaimedContent` then saw 13 < 19 and kept the torrent after the anime left the list ([#74](074-the-pack-exclusion-guards-unit-is-the-torrents.md)). The file list is 13 TV + one OVA; 14–18 do not exist. Extras must not stretch the declared span. OVA/`.5`/episode 0 are this anime's files (they download, get hardlinked into the library, and leave with the torrent) and still must not become `BatchEnd`.

Measured 2026-10-29 (`live_extras_measure_test.go`, 70 packs): OP/ED-class is `NCOP`/`NCED`/`Opening`/`Ending`/`Creditless`/token `OP`/`ED`/`PV`/`Menu` and parent folder `NC`/`Creditless`. Include-as-claimed (out of min-max): `OVA`/`OAD`/`S00E`/`.5`. Unmarked `total+1` with run length 1 showed up three times (Honoka/Kira/DeadFish). Mushoku `- 12`…`- 23` under cour 1 (total 11) has no extra marker — that run is the next cour.

`hasMovieMarker` is too wide on a **file** name (`EP 03 - Special Training!`). Immediate parent only: the torrent root is often `(S1 + OVA + Specials)`. Isolated `total+1` requires a number ≤ total already in the set, otherwise a Part 2 pack numbered 13–24 with entry total 12 loses 13.

**Don't "fix" by:** capping `declaredSpan` at `AnimeTotalEpisodes` without looking at files (deletes the 1–23 pack under cour 1); `force` on the automatic pass; applying `hasMovieMarker` to files; dropping every number > total (that is the next cour); treating the torrent-root folder as extras because its name contains `OVA`.
