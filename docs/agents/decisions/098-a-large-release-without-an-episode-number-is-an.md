# 98. A large release without an episode number is an unlabeled pack, and only its file list makes it a pack

**Location:** `nyaa/nyaa.go` (`minUnlabeledPackBytes`, `TorrentResult.UnlabeledPack`, the
`unlabeled` case of `ScrapNyaaForAnime`); `daemon/episodes.go` (`packSet.covering`);
`nyaa/nyaa_match.go` (`truncateAtFirstMarker` dropping bracket groups from a markerless name).

Old anime is released as one torrent named only by the title and the source:
`[man] Azumanga Daioh [BD 1080p HEVC FLAC]` (82 seeders) carries no "batch", no range, no
"complete". It wasn't `IsBatch`, had no episode number, and was dropped — Azumanga was left with
dead loose episodes while ~15 live packs sat on the same page. Two things blocked it: the
classification, and the title match (with no marker to cut at, `[BD 1080p HEVC FLAC]` was
tokenized and sank the Jaccard below 0.8).

**What looks inconsistent with [#84](084-a-pack-without-a-range-in-its-name-gets-its.md): an unknown range doesn't count as "covers
everything" for this pack.** A labeled pack with an unresolved range still does (a Nyaa outage
must not zero everyone's packs). An unlabeled one is a pack only by guess — size ≥ 1 GiB, no
number, no movie marker — so it must prove it with the file list (`PackFileRange`); without it,
`covering` skips it. It shares the `maxPackDetailFetches` budget.

**Don't "fix" by:** lowering `minUnlabeledPackBytes` (a short or web special, like the 19 MiB
`Azumanga Web Daioh`, would compete); or letting an unresolved unlabeled pack count as complete.
