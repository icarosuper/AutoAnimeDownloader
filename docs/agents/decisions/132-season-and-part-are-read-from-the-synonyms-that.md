# 132. Season and part are read from the synonyms that name the anime, never from one that is the title of its manga or novel

**Location:** `internal/anilist/anilist.go` — `Media.OwnSynonyms`; in `internal/daemon`,
`seasonPartOf` (`helpers.go`) for the season/part reads. `processAnimeEpisodes` and `RunAnimeDebug`
compute `OwnSynonyms()` once, read season and part from it once, and hand them to the anime and
episode searches, `adoptCoveredEpisodes`, `partitionSearchResults` and `newPackAxis`; the preview
reads them once with `seasonPartOf`; pinned by
`TestExtractAnimeSeasonPart_SteelBallRun_SourceTitleSynonymIsNotAPart` and
`TestOwnSynonyms_KeepsSynonymWithoutMatchingSource` in `src/tests/unit/season_part_test.go`.

**What it looks like:** a synonym silently dropped before reading season and part, while the
dashboard search (`api/endpoint_animes.go`) still uses all of them.

**Why it's right:** AniList lists the source's title among the anime's synonyms. Steel Ball Run
(210482) has the synonym `JoJo's Bizarre Adventure: Part 7–Steel Ball Run`, which is exactly the
English title of its `ADAPTATION` manga. JoJo's "Part 7" numbers the manga's story arcs, but
`ExtractPart` read it as part 7 of the anime: the hard part filter of the episode search then
rejected every release (none says "Part 7"), and the wrong part also fed the pack search and the
offset gate ([#9](009-prequel-offset-gate-requires-part-2.md)). A synonym equal to the title of a
manga, novel or one-shot relation names the source, so its numbers count the source. Anime
synonyms keep feeding season and part as before — Kaguya-sama's "Season 2" is still read. For
dashboard search the source name is a fine hit, so that path keeps every synonym.

**Don't "fix" by:**
- Dropping synonyms from season/part extraction altogether: some sequels carry their season only in
  a synonym (Kaguya-sama 112641, Attack on Titan Final Season).
- Matching a JoJo-shaped pattern ("Part N–Title") in `rePartPatterns`: the same regex reads torrent
  names, and the evidence that the number is the source's is the relation, not the punctuation.
- Filtering in `ExtractAnimeSeasonPart` itself: it takes titles and synonyms only, and the anime
  search (`nyaaSearcher.searchAnime`) has no relations to pass — filtering at the `Media` keeps the
  signatures as they are.
