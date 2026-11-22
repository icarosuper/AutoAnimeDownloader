# BAKI-DOU Part 2: a "Season 2" synonym that means cour 2 drops every pack

Found on 2026-11-22 while measuring decisions #133. Not started. Delete this file when it is fixed
or dropped.

## Symptom

`BAKI-DOU Part 2` (AniList **210032**, ONA, 12 episodes, prequel `BAKI-DOU` 176509, ONA, 13
episodes) downloads nothing: `make debug-anime ID=210032` → 0 magnets on all 12 episodes. Nyaa has
the packs, and the pack search returns them as raw rows; `ScrapNyaaForAnime` drops every one, so
`matched_torrents` is `[]` on all four title variants.

## What Nyaa has

All packs, no singles (Netflix drop):

- `[Erai-raws] Baki-dou - 14 ~ 25 [1080p NF WEB-DL AVC EAC3][MultiSub] [BATCH]` (also WEBRip HEVC
  and 720p): continuous numbering, the exact window of this entry
- `[ToonsHub] BAKI-DOU The Invincible Samurai S01E14-E25 1080p NF WEB-DL … (BAKI-DOU Part 2, …) [BATCH]`
- `BAKI DOU The Invincible Samurai S01 Part2 1080p NF WEB-DL … -VARYG (BAKI-DOU, …)`
- the Part 1 packs: `[Erai-raws] Baki-dou - 01 ~ 13`, `[ToonsHub] … S01E01-E13`, `… S01 … -VARYG`

## Cause (from the debug trace)

AniList media:

- title: romaji `BAKI-DOU Part 2`, english `BAKI-DOU: The Invincible Samurai Part 2`, native
  `刃牙道 第2クール` ("2nd cour")
- synonyms: `BAKI-DOU: The Invincible Samurai Season 2`, `BAKI-DOU: O Samurai Invencível Parte 2`

`ExtractAnimeSeasonPart` reads **season = 2** from the synonym `… Season 2`, but that "Season 2" is
the second cour, the same thing as "Part 2". On the pack path (`nyaa.go`, the `IsBatch` branch of
`ScrapNyaaForAnime`) a requested season is a hard filter:

```go
if requestedSeason != nil && (season == nil || *season != *requestedSeason) {
	return
}
```

- Erai `Baki-dou - 14 ~ 25`: no season in the name → `season == nil` → dropped
- ToonsHub `S01E14-E25` and VARYG `S01 Part2`: season 1 ≠ 2 → dropped

So no pack reaches `packAxis`, where delta 13 (`ComputeEpisodeOffset`, now counting the ONA prequel,
decisions #133) would make the Erai `14 ~ 25` cover the window exactly.

Same family as decisions #132 (season/part read from a synonym that doesn't mean what it says), but
#132 only filters out synonyms equal to a manga/novel title; this one is a synonym of the anime
itself.

## What to check

- Whether a title with both a part marker and a season marker, in different variants, should trust
  the part: here the titles say Part 2 and only one synonym says Season 2. Look at how
  `ExtractAnimeSeasonPart` merges titles and synonyms, and at decisions #8, #10 (`Cour N` = `Part
  N`), #79 and #132 before changing it.
- Alternative: on the pack path, a pack with **no** season marker passes when the season came only
  from a synonym, and coverage decides (#79). Weigh it against the cost: other-season packs of
  multi-season shows coming in.
- Sanity-check the fix against BEASTARS FINAL SEASON Part 2 (187267), whose synonym `BEASTARS
  Season 3 Part 2` gives season 3 **correctly** (EMBER `(Season 3)` and VARYG `S03` packs are found
  through it). A rule that drops the synonym's season there would lose those packs.
- TDD: a failing test with the real BAKI-DOU rows and media above (mocked listing, as in
  `src/internal/daemon/search_ona_offset_test.go`), then `go test ./...`, then
  `make debug-anime ID=210032` should pick the Erai `14 ~ 25` pack.
