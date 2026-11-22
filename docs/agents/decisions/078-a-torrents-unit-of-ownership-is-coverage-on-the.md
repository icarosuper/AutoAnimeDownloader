# 78. A torrent's unit of ownership is coverage on the absolute axis, not the `(anime_id, episode)` key

**Location:** `src/internal/daemon/coverage.go` — `resolveSeriesIndex`, `adoptCoveredEpisodes`,
`findCoveringPack`, `dropAdopted`; `packDisplayName` (`episodes.go`), shared with
`assignBatches`; called in `processAnimeEpisodes` (`episodes.go`) and in
`DownloadStandaloneAnime` (`standalone.go`). Pinned by `coverage_test.go`. Consumes #77; it is
the other side of #74.

**The problem.** AniList splits cour and part into **separate media ids**, each numbering from 1;
Nyaa packs are almost always of the **whole season**, crossing that boundary. With ownership
expressed as `files.EpisodeKey{AnimeID, Episode}`, the `1..23` pack downloaded under cour 1 (11
episodes) is invisible to cour 2 — which has no record, does not know the file is already on disk,
and re-downloads the whole season from Nyaa. Real case: Mushoku Tensei S1.

**The rule:** before any search, a pending episode is adopted when there is a pack record, **on
any `anime_id`**, whose declared range contains the episode **on the series' absolute axis**.
`abs = Series.Offset(anime_id) + local number` (#77) — comparing `1..23` with `1..12` without
converting is comparing different rulers.

**Adopting is writing the record pointing at the hash that already exists**, exactly what would
happen if the search found the same torrent again (`Session.Add` reuses the infohash). That is
what makes the rest of the machine work with no new exception: #74 starts seeing the siblings of
the other cour, and `organizeTorrent` hardlinks the files that never had an owner — it already
treats a partially organized group without re-notifying. The range is **copied as the owner
declares it** (it is the torrent name's) and **converted to the adopter's local numbering** —
`findCoveringPack` returns the owner's offset with the record exactly for that. Every record
stores the range on its own entry's ruler (#79) and that is how it is read back; copying `1..23`
from cour 1 under cour 2 (offset 11) would make that record be read as the absolute `12..34`, and
a cour 3 would find coverage for an episode the pack does not have — marked downloaded, never
searched, file nonexistent. Pinned by `TestCoverageOwnership_AdoptedRangeDoesNotShiftForTheNextCour`.

**The index seed includes the `anime_id`s from `episodes.json`, not only the pass's.** The
previous cour is `COMPLETED` and therefore left the pass's universe — and it is **its** offset
that converts the pack's range. Without that seed the feature would not see the pack that
motivated it. It costs almost nothing: width is free (`perPage` 50, one query = one unit — #72)
and a good part of the extra seed would already enter the walk for being an ancestor of the
pass's animes.

**Four gates, all mandatory** — each covers a different way of adopting wrong:

| Gate | Without it |
|---|---|
| Declared range (`BatchStart > 0`) | a pack with no range in the name stores `0`, and unknown would become "covers everything" — same ceiling as #74 |
| Torrent still in the session | a record with a dead hash: episode with an owner and no file, invisible to the re-descent |
| Same `Series.Key` | "absolute 12" would match any other anime's pack |
| Not a movie | `prequelOf` only follows `TV`/`TV_SHORT`, so a post-season movie inherits its offset and its "episode 1" falls inside the pack's range. Coverage by range does not distinguish the two |

**The filter runs at BOTH selection points.** `selectEpisodes` is pure and does not know about
adoption, so the second pass (limit lifted, pack path) would bring the adopted ones back and the
daemon would download what it just adopted.

**Discarded alternative: persist `AbsStart`/`AbsEnd` on `EpisodeStruct` at download time.** The
lookup would be local and offline, but every record from before the feature would be born with `0`
— indistinguishable from "unknown" — i.e. it **would not solve the case that motivated it**, it
would only hold for what was downloaded after; and the backfill would need the derivation anyway.
Worse: a persisted value rots. If AniList corrects an episode count or adds a missing relation,
the stored number stays wrong forever; the derived one self-corrects on the next TTL. Storing as
source of truth a derivable number creates a second truth nobody invalidates. **Reopen only if**
the lookup shows up as a **measured** cost — and then it becomes a derived cache, never a source.

**Don't "fix" by:** comparing `BatchStart`/`BatchEnd` of different media ids without converting;
converting the stored range to the adopter's local numbering (breaks `declaredSpan`); adopting
with an unknown range, with the torrent out of the session or without checking `Series.Key`;
moving adoption inside `selectEpisodes` (it runs twice — would duplicate the records); notifying
`NewEpisode`/`DownloadCompleted` on an adoption (nothing was downloaded, and the completion
webhook already went out when the pack landed).
