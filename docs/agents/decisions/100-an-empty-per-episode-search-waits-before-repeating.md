# 100. An empty per-episode search waits before repeating, and stops the later episodes

**Location:** `daemon/search.go` (`searchMisses`, `episodeMisses`, `nyaaSearcher.misses`);
`daemon/episodes.go` (`stuck` and the miss checks in `processAnimeEpisodes`'s download loop);
`daemon/standalone.go` (the manual download clears `misses`).

Two dead-swarm entries (Sound! Euphonium Shorts and its sequel) cost ~60 Nyaa requests per pass:
every pending episode fell into the per-episode search, and each one tried all four title variants
— every `check_interval` (10 min), forever. After the pack fixes (#96–#99) that was most of the
pass's traffic.

Two rules, both only for the per-episode fallback:

1. **Backoff per episode.** An episode whose search comes back with no candidate is not searched
   again for 1h, then 2h, 4h, 8h, 16h, capped at 24h. Finding a candidate clears it.
2. **Stop at the first miss.** Once an episode ends the pass without a torrent, the episodes after
   it skip the per-episode search in that pass. Episodes are released in order; if 3 isn't there,
   4 almost never is.

**Why this doesn't lose episodes:** the anime search (`ScrapNyaaForAnime`, one query, packs AND
loose episodes) still runs every pass. A pack or a live episode that shows up is caught there;
the skipped episodes still get whatever it found. A newly aired episode has no miss recorded and
is searched right away. A skipped episode keeps its report row with the stats of its last real
search; one that was never searched itself (skipped because an earlier one is missing) reports
`awaiting_earlier_episode` — `no_torrent_found` would claim a search that didn't happen.

**Cost accepted:** an episode that's never released (a fansub that skipped one) blocks the
per-episode search of the ones after it — they're left to the anime search. And the state is in
memory: a restart forgets it, at the price of one full round of searches.

**Don't "fix" by:** applying it to the manual download (the button would do nothing for up to a
day — `misses` is nil there) or to `RunAnimeDebug` (a diagnostic must show every episode's search —
the one deliberate gap in [#58](058-runanimedebug-mirrors-processanimeepisodes.md)'s mirror); or persisting it to disk.
