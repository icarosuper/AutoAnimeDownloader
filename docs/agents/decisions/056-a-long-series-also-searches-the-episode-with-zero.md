# 56. A long series also searches the episode with zero-padding, and the gate is `LastAiredEpisode`

**Location:** `nyaa/nyaa.go` — `episodeQueries`, `longSeriesEpisodes`, `ScrapNyaa`; `anilist/episodes.go` — `LastAiredEpisode`; `daemon/episodes.go` and `daemon/manual_download.go` — `seriesLength` / `totalEpisodes`.

**What it looks like:** `ScrapNyaa` does two searches (`one piece 1` and `one piece 001`) on a long anime, and "is it long?" comes from `anilist.LastAiredEpisode`, not from `Media.Episodes`.

**Why it's right:**

**(a) `q=one piece 1` never finds episode 1.** The query matches everything that contains "1" and, ordered by seeders, the first hundreds of rows are episodes 1160-1173. Measured: 375 rows without episode 1. Groups number a long series with padding (`[Judas] One Piece 001-574`, `[Erai-raws] Naruto Shippuuden - 001 ~ 079`).

**(b) ADDITIONAL variant, not a replacement.** Padding is convention, not a rule: whoever numbers loose (`Show 5`) is still caught by the simple query. Both lists enter the same pool, deduped by magnet.

**(c) The gate is `LastAiredEpisode`, and that matters.** The plan asked for `Media.Episodes > 100`, but on a RELEASING with no announced end that field is **nil** — i.e. it would be nil exactly on One Piece, the case the change exists to fix. `LastAiredEpisode` already combines the three sources (schedule, `nextAiringEpisode - 1`, and `media.episodes` only when FINISHED) and returns ~1173 there. Covered by `TestSingleEpisodeSearch_ReceivesSeriesLength`, which uses an anime in the One Piece shape (RELEASING, `Media.Episodes == nil`).

**(d) The 100 ceiling and `episode < 100`.** Each variant costs fetches, so padding only on a long series; and episode >= 100 already has 3 digits, where the variant would be identical to the simple one.

**(e) An error on one variant does not cancel the other.** `ScrapNyaa` only returns an error if NO variant answered; if the padded query's page fails, the search continues with what the other brought.

**Don't "fix" by:**
- Swapping the gate for `Media.Episodes` "which is the real total" (see (c)).
- Replacing the simple query with the padded one (see (b)).
- Always searching both forms: doubles Nyaa traffic on every short anime, where padding changes nothing (see (d)).
