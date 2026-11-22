# 51. An episode number accepts 4 digits, except inside brackets

**Location:** `nyaa/nyaa_regex.go` — `reEpisodePatterns`, `reBatchRange`, `reHasEpisode`, `reBatchPatterns`; `nyaa/nyaa.go` — `ExtractEpisodeNumber` (ceiling `< 10000`).

**What it looks like:** every episode pattern uses `\d{1,4}`, except `\[(\d{1,3})\]`.

**Why it's right:** with `\d{1,3}` One Piece (ep. 1123+) matched no pattern, `ExtractEpisodeNumber` returned `nil` and **every** Nyaa result was discarded — the log showed 30 good lines and `results: 0`. `[05]` stays at 3 digits because `[2025]` (a year) is far more common in a torrent name than a 4-digit episode inside brackets, and a year read as an episode discards the right torrent.

**Don't "fix" by:** extending `\[(\d{1,3})\]` to 4 digits "for consistency".
