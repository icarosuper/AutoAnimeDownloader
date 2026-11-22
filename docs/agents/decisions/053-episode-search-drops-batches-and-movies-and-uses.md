# 53. Episode search drops batches and movies, and uses `hasMovieMarker` — not `isMovie`

**Location:** `nyaa/nyaa.go` — `parseRow` of `ScrapNyaa` and of `ScrapNyaaForAnime` (at the time of this decision, `ScrapNyaaForMultipleEpisodes`, later fused into `ScrapNyaaForAnime`), `isMovie`, `hasMovieMarker`; `nyaa/nyaa_regex.go` — `reOvaPattern`.

**What it looks like:** two guards at the start of both episode `parseRow`s (`IsBatch` and `hasMovieMarker`), and a `hasMovieMarker` that is `isMovie` **minus** the last branch.

**Why it's right:**

**(a) Without the batch guard, a 220-episode pack enters as "episode 1".** `[Erai-raws] Naruto - 001 ~ 220 [480p]` matches `- 001` in `ExtractEpisodeNumber`, and the multi-episode search (unlike the single) did not filter batches. Result measured, at the time: 220 episodes in one record, with `IsBatch: false`, punching through the then batch ceiling (`max_batch_episodes`, removed by the batch-by-filter spec) **and** `max_episodes_per_anime` — and organized as a single episode (renamed "Naruto - E01" with `rename_files_for_jellyfin`). Discard rather than accept as a batch because the pack search ran **earlier** in strategy resolution: a batch worth taking would already have been caught there, and what leaks here is exactly what the ceiling rejected. The guard itself (mark `IsBatch`/discard leaked packs from inside the episode parser) still holds in `ScrapNyaaForAnime` today.

**(a2) The guard only holds if `IsBatch` recognizes the range.** The range pattern required a space AFTER the second number (`\s\d{2,4}\s*[-~]\s*\d{2,4}\s`), so `One Piece EP 001-501` and `One Piece 001-501.mkv` passed as "episode 1" — the same bug as (a) through another hole, measured live on the One Piece episode 1 search (2 of the 7 candidates were 500-episode packs). The end of the range now accepts space, end of name, `.` or `[`.

**(b) `isMovie`'s `!reHasEpisode && !IsBatch` branch cannot become an episode guard.** `reHasEpisode` only covers `- 05`, `episode 05` and `S01E05`; `ExtractEpisodeNumber` also covers `EP05`, `E05`, `[05]`, `" 05 ("`, `" 05.<video ext>"` and `" 5"`. Calling all of `isMovie` would reject every release in those forms (the "Lucky Star EP015 / E015 / 15" tests catch that). Hence the split: `hasMovieMarker` (keywords + OVA/ONA + special) serves both sides, and `isMovie` remains `hasMovieMarker` plus the "has no episode marker" fallback that only makes sense on a movie search.

**(c) `reOvaPattern` needs `\b`.** It was `\(?(ova|ona)\)?`, with no boundary: "ona" matches inside **Persona**, "ova" inside **Nova**. While the pattern was only used in the permissive direction (`ScrapNyaaForMovie`) that passed; as an episode guard it would become rejection of the whole anime.

**Don't "fix" by:**
- Swapping `hasMovieMarker` for `isMovie` in the episode guard "to reuse the function" (see (b)).
- Accepting the leaked batch by marking `IsBatch: true` instead of discarding (see (a)).
- Taking `\b` off `reOvaPattern` to "also catch `(OVA)`" — `\b` already matches before `(` and after `)` (see (c)).
