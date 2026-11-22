# 10. "Cour N" treated as Part N, not as a distinct concept

**Location:** `internal/nyaa/nyaa_regex.go` — `rePartPatterns`, `reCourMarker`; `internal/nyaa/nyaa_match.go` — `firstMarkerIndex`; `internal/daemon/helpers.go` — `ExtractAnimeSeasonPart`

**What it looks like:** `Cour 2` is extracted by `ExtractPart`, not by a separate cour field. Looks like it conflates two different concepts.

**Why it's right:** Anilist uses "Cour" and "Part" interchangeably for split seasons (e.g. NieR:Automata Ver1.1a Cour 2). On Nyaa, release groups also use both terms for the same split. A separate cour field would require all search/filter logic to handle two parallel fields with identical semantics. Mapping cour→part keeps the filter logic and tests uniform and matches how the data actually appears in the wild.

**Don't "fix" by:** adding a separate `Cour *int` field to `TorrentResult` or a separate extraction path.

**`ExtractSeason` does not read a cour either.** Until 2026-11-07 `reSeasonPatterns` also held a
`Cour N` pattern, so an English title like "Mushoku Tensei: Jobless Reincarnation Cour 2 - Eris the
Goblin Slayer" (141534, the second half of season 1) came out as season 2 AND part 2, and the pack
path dropped every release without a season marker. "Cour N" now lives in `reCourMarker`, which only
`firstMarkerIndex` uses — still a place to cut a torrent name, never a season. Pinned by
`TestExtractAnimeSeasonPart_MushokuEris_CourIsNotASeason`.
