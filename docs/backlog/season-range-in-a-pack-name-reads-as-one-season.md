# A season range in a pack name reads as one season

Found on 2026-11-22 while fixing the `S01-05` episode range (`aec2064`). Not started. Delete this
file when it is fixed or dropped.

## Symptom

`ExtractSeason` gives a multi-season pack a single season, or none:

| Name | `ExtractSeason` | Carries |
|---|---|---|
| `[Trix] Kimetsu no Yaiba S01-05 … (Batch)` | 1 | seasons 1-5 |
| `[Group] Anime Season 1-3 [BD 1080p] (Batch)` | 1 | seasons 1-3 |
| `[Group] Anime Seasons 1 ~ 3 [BD 1080p]` | none | seasons 1-3 |

The first two matter. On the pack path, a requested season drops a pack whose season differs
(`ScrapNyaaForAnime`: `requestedSeason != nil && (season == nil || *season != *requestedSeason)`).
So for a season-3 entry the Trix pack, which contains season 3, is dropped as "season 1". Not
measured live. It is inferred from the filter, and the golden case only covers season 1.

## Cause

`reSeasonPatterns` (`nyaa/nyaa_regex.go`) takes the first number after `S` / `Season`. `Season\s*`
also misses the plural `Seasons`. `BatchInfo` has one `Season int`, so the range has nowhere to go.

## What to check

- Whether `ExtractSeason` (or a new `ExtractSeasonRange`) should return lo..hi, with the pack-path
  filter accepting a requested season inside the range. A pack with seasons restarting is then
  placed on the series axis by `packAxis` / `PackFileRange` ([decisions/116](../agents/decisions/116-restart-per-season-packs-flatten-onto-the.md)).
- The guard `reSeasonRangePrefix` already recognizes the same three shapes on the episode side, so
  the two should share a pattern.
- Pin it with a golden case on a season-2+ entry of Kimetsu (`make golden-record`), with `picked`
  set before the fix. The unit test `TestExtractBatchInfo_ExtractsRangeAndSeason` currently asserts
  season 0 for `Seasons 1 ~ 3`, which is today's behavior; change it with the fix.
