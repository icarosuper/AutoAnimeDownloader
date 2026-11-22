# 55. `health` compares a seeders BAND, and it comes before `fansub`

**Location:** `nyaa/priorities.go` — `criterionCompare["health"]`, `healthTier`, `healthTierFloors`, `DefaultPriorities().CriteriaOrder`; `nyaa/nyaa.go` — `torrentHealthScore` (log only).

**What it looks like:** the `health` comparator does not use `torrentHealthScore`; it uses a band index (`0 / 1-4 / 5-19 / 20-99 / 100-399 / 400+`). And `health` is in 4th position of the default order, not 7th.

**Why it's right:**

**(a) The sort is lexicographic, so a raw score = health alone decides or never decides.** With a continuous score, a 1-seeder difference already settles the comparison and no criterion after `health` is ever consulted. Comparing bands, 400 seeders beats 12 (bands 5 and 3), but 150 vs 300 ties and `fansub` decides — which is the intended behaviour.

**(b) `health` in 7th position was never reached.** Measured on With You: the 3 candidates were 1080p, `fansub` broke the tie (subsplease=0, judas=2, ember=5) and `health` was not consulted — a torrent with 12 seeders would beat one with 400 by fansub order. The new order is `uncensored, source, resolution, health, codec, fansub, audio, size`.

**(c) Fixed band, not a ratio.** "Tie unless one has 2x the other" looks better (no edge effect), but it is not transitive — 100 ~ 150 ~ 220 and 100 < 220 — and `sort.SliceStable` with an intransitive comparator returns arbitrary order. The cost of a fixed band is the edge: 399 and 400 fall into different bands. Accepted on purpose; any bucketing has an edge.

**(d) `torrentHealthScore` stayed log-only.** It combines seeders with the seeders/leechers ratio, information the band (seeders only) discards. It is worth as a diagnosis in `matched_torrents` (`t=` band, `h=` score), not as a criterion.

**Rollout note:** an existing `config.json` **already has** `criteria_order` written and `LoadConfigs` unmarshals over the default, so the new order does NOT arrive on its own in an old install. The user edits by hand or clicks "restore defaults" on the Priorities page.

**Don't "fix" by:**
- Putting the comparator back on `torrentHealthScore` "because it uses more information" (see (a) and (d)).
- Swapping the band for a ratio/percent tolerance (see (c)).
- Migrating an existing config's `criteria_order` automatically: silently reordering the preference the user saved is worse than the rollout note.
