# 81. Report-code precedence is a cascade, not a set

**Location:** `daemon/report.go` — `searchIssue`.

**What it looks like:** an episode can satisfy three conditions at once — every candidate above the ceiling, every one below the seeders floor, and `len(magnets) == 0`. `searchIssue` picks **one** code with a `switch` whose order looks arbitrary, and ignores that the other conditions are also true.

**Why it's right:** the order is the business rule. When a filter empties the list, "no torrent found" is **also** true — and it is the least actionable of the three, besides being a lie: there were eight. "Every candidate was 8 GB and your ceiling is 3 GB" tells the user what to do (raise the ceiling, or adjust the custom query). "No torrent found" says nothing, and sends them looking on Nyaa for a torrent that had already been found.

Same discipline as `deriveAnimeChip`'s cascade (`lib/domain/animeState.ts`): the first condition that matches wins, and the order is the rule.

**Accepted trade-off:** an episode cut by both filters shows up only as `all_above_size_limit`. Deliberate — the user solves one at a time, and the size one is what they control directly.

**Don't "fix" by:**
- Returning a list of codes "because all three conditions hold" — the UI would start showing three lines per episode, two of them noise.
- Reordering to put `no_torrent_found` first "because it's the most common case" — common is not the criterion; actionable is.
- Feeding the cascade with `partitionSearchResults`'s `packStats` — the pack is decided for the anime, not for the episode, and the per-episode fallback is who has the last word on that episode.
