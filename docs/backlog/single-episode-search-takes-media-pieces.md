# The single-episode search takes pieces of the media, so a caller can pass them empty

Found on 2026-11-22 while running /finish on the manual-download fallback fix (`8497d70`). Not
started. Delete this file when it is fixed or dropped.

## Symptom

`searchNyaaForSingleEpisode(ep, titles, synonyms, relations, format, totalEpisodes)` (`search.go`)
and the `searchSingleEpisode` func field behind it take title, synonyms, relations and format as
separate arguments. Every production caller (`episodes.go`, `debug.go`, `manual_download.go`) takes
all four from the same `anilist.Media`. Passing `nil` and `anilist.MediaRelations{}` still
type-checks, and then the offset and split-entry fallbacks silently never run. That is the bug
`8497d70` fixed on the manual path, and only a user-visible miss revealed it.

## What to check

- Change the signature to `(ep anilist.AiringNode, media anilist.Media, totalEpisodes int)` and call
  `media.OwnSynonyms()` inside. `totalEpisodes` stays separate, since it comes from
  `anilist.LastAiredEpisode(mediaList)`, not from `Media`.
- The callers' precomputed `OwnSynonyms()` is no reason to keep the current shape: they need it for
  their own `ExtractAnimeSeasonPart` and `searchAnime` calls, and `searchNyaaForSingleEpisode`
  already rereads season and part from the synonyms on every call.
- The cost is test churn: 6 fake implementations of the func field (`coverage_test.go`,
  `episodes_test.go`, `limits_test.go`) and 7 direct calls (`search_split_test.go`,
  `search_ona_offset_test.go`).
- `searchAnime` takes `(titles, synonyms, …)` the same way, but has no relations or format to drop.
  It's optional there.
