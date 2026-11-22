# 83. The pack name's range is the first plausible match, not the first match

**Location:** `nyaa/nyaa.go` — `batchRange`, called by `ExtractBatchInfo`; `nyaa/nyaa_regex.go` — `reBatchRange`.

**What it looks like:** a `FindAll` with `continue` on three guards where a `FindString` with an `if` would fit, and a band of magic numbers (1900..2100) in the middle. Looks like speculative defence against cases that do not happen.

**Why it's right:** `reBatchRange` is `(\d{1,4})\s*[-~]\s*(\d{1,4})` — it matches **any** pair of numbers joined by `-` or `~`, and in a release name most of those pairs are not an episode. The three measured:

| Name | Match | What it is |
|---|---|---|
| `[Group] Anime Complete Batch [720-1080p]` | `720-1080` | resolution |
| `[Erai-raws] Hibike! Euphonium 2 - 01 ~ 13 …` | `2 - 01` | season + first episode |
| `[EMBER] Mushoku Tensei … (2021-2022) (Season 1 …)` | `2021-2022` | release year |

The year is what forced the change: it does **not** fall into the safe fallback. `EndEpisode == 0` means "unknown range" and every caller treats it as a complete pack; `2021..2022` is a *declared* range that no `packAxis` hypothesis covers, so `coveringBatch` rejects the pack and it vanishes from the choice without a log line that explains. It was a complete BDRip pack, first placed by seeders, discarded by a parenthesis.

The band `start >= 1900 && last <= 2100` only catches a year: no series reaches episode 1900, and `One Piece 1001-1100` — the real case that forced `\d{1,4}` in [#51](051-an-episode-number-accepts-4-digits-except-inside.md) — passes intact.

**`FindAll` and not `FindString`** because a rejected match must not end the search: in `[Group] Anime (2021-2022) 01-24 [BD]` the real range comes **after** the year.

**Accepted trade-off:** `FindAll` is **non-overlapping**, so Hibike still has no range. The match `2 - 01` consumes the `01`, and the following `01 ~ 13` is never seen. It stays as-is on purpose: scanning with overlap (restarting one character after each rejection) produces junk sub-matches — a rejected `2021-2022` would become `021-2022`, which passes the band and stores the range 21..2022. A pack with no range falls on the safe path; a pack with a wrong range lies to ownership by coverage ([#78](078-a-torrents-unit-of-ownership-is-coverage-on-the.md)).

**Don't "fix" by:**
- Swapping for a token sanitizer that strips `[...]`/`(...)` before matching: the year comes in parentheses, but so does `(001-100)` — that is where half the groups put the real range.
- Narrowing the year band to "only 19xx/20xx with a difference <= 5": the difference between the years distinguishes nothing the band does not already distinguish, and it becomes one more magic number.
- Scanning with overlap to save Hibike (see the trade-off).
