# 79. Pack choice asks "does it cover the window?", not "is it part N?" — and pack numbering is a guess among three hypotheses

**Location:** `src/internal/daemon/episodes.go` — `packAxis`, `newPackAxis`, `packAxis.localRange`,
`coveringBatch`, `pickBatches`, `assignBatches`, `hasDeclaredRange`, `declaredSpan`;
`src/internal/nyaa/nyaa.go` — `ExtractPart`, `declaredParts` and the `IsBatch` branch of
`ScrapNyaaForAnime`. Pinned by `packaxis_test.go` and the tables of `nyaa_test.go`. Consumes #77
(absolute axis) and is the other side of #78 (ownership by coverage).

**The problem.** AniList splits the season into cours/parts, each entry numbering from 1; groups
release the pack of the **whole season** and each one picks whichever ruler it wants (`sources.md`,
"Pack granularity and numbering": restarts per season, restarts per part, continuous across the
season, absolute across the series). Two consequences, both measured on Mushoku Tensei:

1. **The hard part filter zeroed the packs of every "Part N" entry.** With `requestedPart != nil`,
   a pack **without** a part marker was discarded — and a whole-season pack normally has no such
   marker, even though it contains the requested episodes.
2. **The name's range was read on the wrong ruler.** `01 ~ 23` under a 12-episode entry was
   compared with `1..12` as if it were the same numbering.

**The rule:** on the pack path, `part` only rejects a **declared conflict** (the pack says "Part 1"
and we want 2). With no marker, the pack passes and who decides is **range coverage**. The episode
path keeps the hard filter: there the marker is the only way to tell one part's "- 05" from the
other's "- 05".

**Known ceiling — a pack with no part marker AND no range in the name is decided by nobody.**
`coveringBatch` returns that pack without consulting the axis (there is no number to convert) and
it counts as complete, so "who decides is coverage" holds for a pack **with** a range, not for
this one. A `[Group] Anime S4 [Batch]` that in practice is only Part 1 passes the same as the
whole-season pack — before this decision it was rejected in the scraper, and rejecting it was
worse: it zeroed the packs of every "Part N" entry, which is problem 1 above. Accepting is the
measured choice; the missing tie-break was **Nyaa's detail-page file list** (at the time, an item
in `docs/TODO.md`), which says how many files the pack has and with what numbering. **RESOLVED in
almost every case (see #84):** that read exists (`nyaa.PackFileRange`, consumed on demand by
`packSet.rangeOf`), and the range it returns enters through the same `packAxis` as the name's
range. What remains is the residue where neither the name nor the files resolve (detail page down,
unreadable file name): then the range stores zero and the pack is still used as complete — this
entry's ambiguity, reduced to the rare case.

**`packAxis` is three hypotheses and a tie-break.** Because the name's convention cannot be read,
the daemon tests how much to add to the local number to reach the group's ruler: **0** (relative
to the entry), **`ComputeEpisodeOffset`** (continuous across the season — the immediate prequel's
total) and **`anilist.Series.Offset`** (absolute across the series, #77). Only a hypothesis that
actually **covers the requested episode** competes; among those that cover, the winner is the one
that makes the pack **end on the entry's last episode**. That tie-break is `sources.md`'s "file
count" expressed by the name's span: a 25-episode pack under a 12-episode entry cannot be local, a
12 under the same entry cannot be anything else. In doubt (none hits the end), delta 0 holds: a
missing offset falls back to relative numbering, which a good part of the groups use, while a
wrong offset picks a pack that does not have the episode.

**The STORED range is the converted one, and it can start at zero or below.** A season pack
downloaded under cour 2 starts before that entry's episode 1: `-12..12` is the correct
representation, and it is what makes `owner.Offset + BatchStart` (#78) return the right absolute
range and `declaredSpan` (#74) count the pack's 25 content episodes. That is why the "unknown
range" sentinel became the **end** (`hasDeclaredRange`: `BatchEnd > 0`) and no longer
`BatchStart <= 0` — which already lied about `[Erai-raws] … - 00 ~ 12`. Only the **displayed name**
cuts the start at 1: "-10-12" says nothing on screen, and what matters there is this entry's
slice.

**`ExtractPart` returns `nil` for a name with two parts.** "(Part 1 + Part 2)", "(Part 1+2)" and
"(Season 4 Part 03+04)" cover both halves; returning the first number made the pack be rejected
exactly for the second. With no unique number, who decides is coverage.

**The two "first TV/TV_SHORT PREQUEL" selections stay duplicated on purpose** (and since
[#133](133-an-ona-entry-counts-its-ona-prequel-in-the-season.md) they also differ on an ONA entry, where only the one-hop selection counts an ONA).
`anilist.prequelOf` walks the **whole** chain (hypothesis "absolute across the series");
`daemon.ComputeEpisodeOffset` takes **one** hop and is gated by `part >= 2` (hypothesis
"continuous across the season"). They are different hypotheses of the same guess — unifying them
would erase the distinction the heuristic uses. The `TODO(F8)`s that pointed one at the other
became this note.

**Measurement (29 Aug 2026).** `make debug-batch` over `scripts/robustness-animes.txt`: episodes
searched and found identical to the previous run on the 8 animes (only seeders varied, live data).
On the motivating cases, all with 12/12 magnets: `166873` and `127720` (Mushoku Tensei) and
`131681` (AoT Final Season Part 2), where the pack `[EMBER] … (Season 4 Part 03+04)` started
surviving the filter.

**Don't "fix" by:** going back to requiring a part marker on the pack path; relaxing the part
filter on the **episode** path; treating a range start ≤ 0 as unknown; converting the stored range
"to make it positive" (breaks #74 and #78); adding a numbering hypothesis without a tie-break that
distinguishes it from the others; unifying `prequelOf` with `ComputeEpisodeOffset`.
