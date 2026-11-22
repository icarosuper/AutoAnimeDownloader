# Manual download skips the single-episode search fallbacks

Found on 2026-11-22 while fixing Stone Ocean Part 2 (decisions #133). Not started. Delete this file
when it is fixed or dropped.

## Symptom

`src/internal/daemon/manual_download.go:212` calls `searchNyaaForSingleEpisode` with `nil` synonyms
and an empty `anilist.MediaRelations{}`:

```go
results := rankSeriesResults(searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, nil, anilist.MediaRelations{}, details.mediaList.Media.Format, anilist.LastAiredEpisode(details.mediaList)), details.isFinished)
```

With no relations, both fallbacks after the normal search are dead on this path:

- the offset fallback (`ComputeEpisodeOffset`, decisions #9 / #133) always gets offset 0;
- the split-entry fallback (`splitEntryContinuation`, decisions #131) never finds a prequel.

With no synonyms, `ExtractAnimeSeasonPart` reads season and part from the titles only, so an entry
whose season or part lives only in a synonym is searched without it. The scheduled path
(`episodes.go:271`, `debug.go:137`) passes `anime.Media.OwnSynonyms()` and `anime.Media.Relations`.

Concrete case: a manual download of Stone Ocean Part 2 (146722) episode 1 still finds nothing,
while the daemon's pass finds `[inid4c] … Stone Ocean 13 v2 (1080p)`. Same for Steel Ball Run
2nd & 3rd STAGE (210482).

## What to check

- The data is already there: `resolveAnimeDetails` (`manual_download.go:65`) fills
  `details.mediaList` from `GetAnimeInfo` / `GetMediaByID`, and both queries ask for synonyms and
  relations (`mediaByIDFields` in `anilist/standalone.go:136`). Its own comment even says relations
  are fetched "to build the Nyaa query". So the empty values look like an oversight, not a choice.
- Still confirm before changing: grep `docs/agents/decisions/` for `manual_download` / `ManualDownload`. The
  line dates back to the early Portuguese-era commits (`8ef8233`, `ed611cb`), before the fallbacks
  existed.
- Fix: pass `Media.OwnSynonyms()` and `Media.Relations`, like the scheduled path. Test: a
  manual-download test with a mocked listing where only the offset fallback finds the episode
  (reuse `stoneOceanRows` from `search_ona_offset_test.go`).
