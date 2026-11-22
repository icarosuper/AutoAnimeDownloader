# 133. An ONA entry counts its ONA prequel in the season offset, while the series axis still walks TV prequels only

**Location:** `internal/daemon/helpers.go` — `ComputeEpisodeOffset` (the `format` parameter; a
TV/TV_SHORT prequel returns at once, an ONA prequel is kept only under an ONA entry and returned
only when no TV prequel follows); its callers `searchNyaaForSingleEpisode` (`search.go`, reached by the pass and by
`ManualDownloadEpisode`) and `newPackAxis` (`episodes.go`). Pinned by
`TestSearchSingleEpisode_ONASplitCountsTheONAPrequel`,
`TestSearchSingleEpisode_TVEntryIgnoresTheONAPrequel`,
`TestManualDownloadEpisode_ONASplitUsesTheOffsetFallback` (`internal/daemon/search_ona_offset_test.go`), `TestComputeEpisodeOffset_ONAEntryCountsONAPrequelOnly` and
`TestComputeEpisodeOffset_ONAEntryPrefersTheTVPrequel` (`tests/unit/season_part_test.go`).
Amends [#9](009-prequel-offset-gate-requires-part-2.md).

**What it looks like:** [#9](009-prequel-offset-gate-requires-part-2.md) says ONAs never enter the
offset, yet an ONA prequel is counted — and only sometimes, depending on the entry's own format.
`anilist.prequelOf` still skips ONAs, so the two "same" selections now disagree.

**Why it's right:** `JoJo no Kimyou na Bouken: Stone Ocean Part 2` (146722) found no per-episode
torrent. Every Stone Ocean entry is a Netflix ONA, and the single-episode groups number the show
continuously: AniList 131942 is episodes 1–12, so 146722 episode 1 is `[inid4c] … Stone Ocean 13`.
The normal search hard-filters on `part=2` and no single says "Part 2"; the offset fallback is what
should find it, but with a TV-only rule the offset was 0 and the fallback never ran. A streaming
series cut into ONA parts is one show, so its ONA prequel is part of the count. An ONA side story of
a TV series is still left out, because the entry's own format is TV. Movies and OVAs never count.

Measured live (22 Nov 2026), `searchNyaaForSingleEpisode`, old rule → new rule: 146722 ep 1 0 → 1
(`Stone Ocean 13 v2`), ep 13 0 → 1 (`Stone Ocean 25`), ep 26 0 → 1 (`Stone Ocean 38 v2`). The
other AniList entries the change touches (ONA, part ≥ 2, ONA prequel), found by searching AniList
ONAs for "Part" / "Cour" / "2nd Season": Kengan Ashura Part 2 (111048), Kengan Ashura Season 2
Part 2 (169692), 7SEEDS 2 (112686), Gokushufudou Part 2 (132193), Liang Bu Yi Part 2 (138351),
BEASTARS FINAL SEASON Part 2 (187267), BAKI-DOU Part 2 (210032). Every one returned 0 singles
under both rules: Nyaa has only packs for these Netflix drops. BEASTARS shows the fallback running
as meant (it searched `beastars final season 24` and found only the `13 ~ 24` batch, which the
episode path rejects). The Baki chain before BAKI-DOU carries no part, so the gate keeps it at 0. On the pack path the change only adds a candidate to `packAxis`, and the
end-on-the-last-episode tie-break ([#79](079-pack-choice-asks-does-it-cover-the-window-not-is.md))
still picks among the hypotheses.

`anilist.prequelOf` (the absolute series axis, `Series.Offset`) did **not** change. Moving it would
move coverage ownership ([#78](078-a-torrents-unit-of-ownership-is-coverage-on-the.md))
and organize slicing for every ONA chain, and the singles bug didn't need it. The same fix also
taught the ` 05 (` episode pattern (`nyaa_regex.go`) to allow a `vN` before the `(`, because
`Stone Ocean 13 v2 (1080p)` had no episode number.

**Don't "fix" by:**
- Counting an ONA prequel for every entry: an ONA side story of a TV season would shift that
  season's search to an episode that doesn't exist ([#9](009-prequel-offset-gate-requires-part-2.md)).
- Returning the first counted edge in AniList's order: an ONA "Part 2" whose PREQUEL edges are
  [ONA side story (3 eps), TV Part 1 (12 eps)] would get offset 3. With both, the TV prequel is the
  count and the ONA is a side story; the ONA counts only when there is no TV prequel.
- Dropping the part gate for ONA entries: Baki and other ONA sequels number from 1 per entry.
- Making `prequelOf` follow the same rule "for consistency": that is a separate hypothesis with
  ownership and organize behind it ([#79](079-pack-choice-asks-does-it-cover-the-window-not-is.md)).
  Change it only with its own measurement.
