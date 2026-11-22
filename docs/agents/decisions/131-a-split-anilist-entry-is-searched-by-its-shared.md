# 131. A split AniList entry is searched by its shared base title, with the prequel's episodes added, when every other search came back empty

**Location:** `internal/daemon/search.go` — `searchNyaaForSingleEpisode` (the last fallback),
`splitEntryContinuation` and `suffixOrdinal`, over `nyaa.CutTitleSuffix` (the same cut the
romaji-base + English-subtitle variant uses); pinned by `TestSearchSingleEpisode_SplitEntry*`,
`TestSplitEntryContinuation_OrdinalRule` and `TestSuffixOrdinal` in
`internal/daemon/search_split_test.go`.

**What it looks like:** a third search that drops the entry's own suffix, ignores season and part,
and shifts the episode by an ONA prequel's count — the very prequel format
[#9](009-prequel-offset-gate-requires-part-2.md) / `ComputeEpisodeOffset` refuse to count.

**Why it's right:** AniList sometimes cuts one show into entries that share a romaji base and differ
only after a final ` - `: Steel Ball Run (210482) is `JoJo no Kimyou na Bouken: Steel Ball Run - 2nd
& 3rd STAGE`, with the 1-episode ONA `… - 1st STAGE` as PREQUEL. The groups release it as one show:
Erai-raws posts `JoJo no Kimyou na Bouken: Steel Ball Run - 02` for AniList episode 1, and the
TVDB-style rows (`S06E02`, ToonsHub/VARYG) carry a season this entry doesn't have. Every query built
from the entry's titles contains "2nd 3rd stage", so Erai never came back; the anime failed every
cycle with "No torrent found". Measured with `make debug-anime ID=210482`: 0 magnets before, all 9
aired episodes resolved to the right Erai release after. The shared base, not the format, is the
evidence that the prequel's episodes are part of the count, so an ONA counts here; a movie never
does. It runs last, so it only acts where the normal and the offset searches found nothing.

**Both suffixes need an ordinal: 1 on the prequel, later on the entry.** "Last" is not rare: right
after an episode airs, before any group uploads, the other searches come back empty every cycle. A
shared base alone then shifts arcs that groups number per entry (`<Base> - Arc A-hen` / `Arc B-hen`,
11 episodes: ep 3 searches `<Base> 14` and grabs some other season's `<Base> - 14`). And the offset
is only the immediate prequel's count, while groups count from the first entry: `3rd STAGE` after a
12-episode `2nd STAGE` would search `<Base> 13`, a real release that is episode 1 of the 2nd STAGE.
The relation nodes carry no relations of their own, so the entries before the prequel can't be
summed; the fallback refuses instead. `suffixOrdinal` reads `1st`/`2nd`…, `first`/`second`/`third`,
and `Stage`/`Part`/`Season`/`Cour N`; a bare number (`Arc 2-hen`) is not an ordinal.

**The pack path is left out on purpose.** Measured on 2026-11-22 with the real pack functions:
the axis half works — a `- 01 ~ 12` pack under delta 1 is recorded as 0–11 — but organize doesn't
read that delta. `FilesInPackWindow` slices by the record's `SeriesOffset`, which is the series
index offset (0 here, the ONA prequel isn't in the chain), so the library links would be one
episode off:

| pack files | `SeriesOffset` 0 (today) | `SeriesOffset` 1 |
|---|---|---|
| `01–12` (continuous, Erai) | wrong: `01` (1st STAGE) → ep 1, 12 kept | right: `02`→1 … `12`→11 |
| `02–12` (continuous) | wrong: `02`→2 … `12`→12 | right |
| `01–11` (relative) | right | wrong: `01`→0, `11`→10 |

Doing it right means recording the CHOSEN pack delta for organize instead of the series offset
(`episodes.go`, the `SeriesOffset` of the record), which changes
[#116](116-restart-per-season-packs-flatten-onto-the.md)
for every multi-entry series — too wide for a rare title shape. Without it, a split entry with
every episode pending still downloads one by one through this fallback.

**Don't "fix" by:**
- Folding it into `ComputeEpisodeOffset`: that one is gated by part ≥ 2 and counts an ONA only
  under an ONA entry on purpose ([#9](009-prequel-offset-gate-requires-part-2.md),
  [#133](133-an-ona-entry-counts-its-ona-prequel-in-the-season.md)); letting every ONA in there would shift every sequel with a side ONA.
- Running it before the normal search: a group that names the second entry by its full title and
  numbers it from 1 would lose to a base-title release of a different episode.
- Accepting `S06E..` rows for an entry with no season: "S06" is TVDB's season of the whole
  franchise, and the season filter is what keeps "Show S02E03" off "Show".
- Adding the base title and the prequel's episodes to the pack search (`searchNyaaForAnime` /
  `newPackAxis`) alone: the pack gets picked and organize links its files one episode off (table
  above). Its singles are also on the base ruler, and `magnetsByEpisode` would map them unshifted.
- Matching on a common prefix instead of the exact base before ` - `: "<Show> - Part 1" sequels of a
  different arc would share a prefix and get an offset that isn't theirs.
- Dropping the ordinal check "because the base already matches": arc suffixes share the base too,
  and the empty-right-after-airing search makes the fallback run on them every cycle.
- Accepting a prequel whose ordinal is above 1 with its own count: that offset points at the
  prequel's episodes. Summing the chain needs the prequel's relations, which the list query
  doesn't fetch.
