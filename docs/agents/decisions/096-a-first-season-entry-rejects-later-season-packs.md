# 96. A first-season entry rejects later-season packs, but a sequel without a marker doesn't

**Location:** `daemon/episodes.go` (`isFirstSeasonEntry`, the first case of
`partitionSearchResults`); the permissive rule it narrows is the pack branch of
`nyaa.ScrapNyaaForAnime`.

With no season requested, the nyaa package accepts a pack of **any** season and lets the range
coverage decide ([#79](079-pack-choice-asks-does-it-cover-the-window-not-is.md)). That broke on Medalist: the entry is season 1, progress 2, and
`[Sylvar] Medalist Season 2 (BD Remux)` declares the files 1..9 — delta 0 matched, and episodes
3..9 were assigned to the season 2 pack (it only didn't download because it didn't fit on disk).

**What looks inconsistent: the drop happens only when the entry is CERTAINLY season 1** — no season
marker in its titles (`ExtractAnimeSeasonPart`) and no `PREQUEL` edge of format TV/TV_SHORT. A
sequel whose title carries no marker still accepts packs of any season, because there the pack's
"Season 2" can be the right one under a different numbering, and coverage is still the better
judge. It lives in the daemon and not in the nyaa filter because only the daemon has the relations.

**Don't "fix" by:** moving it into `ScrapNyaaForAnime` (no relations there); or making the nyaa
pack filter strict for every unmarked entry (it zeroes the packs of every sequel without a marker).
