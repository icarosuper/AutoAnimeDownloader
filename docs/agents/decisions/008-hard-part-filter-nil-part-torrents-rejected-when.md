# 8. Hard part filter — nil-part torrents rejected when requestedPart is set

**Location:** `internal/nyaa/nyaa.go` — `ScrapNyaa`, `ScrapNyaaForAnime`

**What it looks like:** When `requestedPart != nil`, torrents whose name has no part marker are rejected, even though they might be the right episode. Looks overly strict.

**Why it's right:** Split-season animes (e.g. Mushoku Tensei II Part 1 / Part 2) have the same episode numbers in both halves. Without a hard part filter, Part 1 torrents would be downloaded for Part 2 entries. A torrent with no part marker in its name is ambiguous and must be treated as the wrong release when the caller knows it wants a specific part. The false-negative cost (missing a valid torrent) is lower than the false-positive cost (wrong episode downloaded silently).

**Don't "fix" by:** accepting nil-part torrents as "probably correct." The offset fallback (see decision 9) handles the rare case where no part-marked torrent exists.
