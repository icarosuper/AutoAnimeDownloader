# Query, filters and pack selection

Steps 4–6 of the [debug-anime](SKILL.md) flow: Nyaa should have the anime, but the search doesn't
find it or the pipeline throws it away. Decisions are linked from `docs/agents/decisions.md` — read
only the entry linked.

## 4. The exact search URL

In `debug.jsonl`, or in the real log with debug level:

```bash
grep '"Searching Nyaa' ~/.autoAnimeDownloader/daemon.log | grep '"anime_name":"<Anime>"'
```

Fetch it and list the torrent names:

```bash
curl -sL "<url>" | grep -E 'title="[^"]{20,}' | grep 'href="/view/'
```

Zero results → the query is wrong (step 5). Results, but the anime still fails → filtering (step 6).

## 5. The search query

Built from the title variants: `search.go:buildTitleVariants` → `nyaa.GenerateSearchTitleVariants`
(`nyaa/nyaa.go`), which tries clean romaji → original romaji → clean english → original english → (last) romaji base
title + English subtitle, the way Erai-raws names OVAs ("… Honki Dasu Part 2 - Eris the Goblin
Slayer"). A one-episode entry (OVA, special) whose release has no number is found as an unlabeled
pack, not an episode: see [decisions.md #123](../../../docs/agents/decisions.md#123-a-one-episode-entry-covers-episode-1-even-when-the-magnet-declares-no-range).
`ExtractAnimeSeasonPart` (`daemon/helpers.go`) decides the season/part that shape the query and the
filter.

Search [nyaa.si](https://nyaa.si) by hand with each variant. If none works: the anime may not be on
Nyaa, or the fansub community uses another title — compare against what's actually posted, and
consider a better variant (strip the subtitle after `:`, shorter prefixes).

`searchNyaaWithVariants` (`daemon/search.go`) **stops at the first variant that returns anything**,
romaji first. That was measured and stays
([decisions.md #76](../../../docs/agents/decisions.md#76-searchnyaawithvariants-stops-at-the-first-variant-that-returns-any-result--measured-and-it-stays-that-way)):
after #75 there is no anime where a later variant finds a pack romaji didn't. A real case is the
evidence that reopens it — bring it, don't change the loop on a hunch.

## 6. Result filtering

Results exist but none reach `attemptDownloadWithRetries`. Trace `nyaa/nyaa.go`:

**6a. Ignore list** — `ShouldIgnore` (`nyaa/priorities.go`). Any substring of
`priorities.ignore_list` drops the release, case-insensitive; defaults `[dub]`, `[raw]`,
`[hardcoded]`, `[hc]`, `re-encode`. The list is user-editable (Priorities screen): read the real
`config.json`, not the defaults.

**6b. Episode number** — `reEpisodePatterns`. A single-episode result is kept only if its extracted
number matches. Check for a naming pattern the regexes don't cover.

**6c. Season/part** — if `ExtractAnimeSeasonPart` returns a season or part, results are filtered to
match; a wrong extraction drops everything. Sequels titled only with a roman numeral ("… II") go
through `ExtractSeason`'s roman fallback (`reRomanSeason` in `nyaa_regex.go`,
[decisions.md #20](../../../docs/agents/decisions.md#20-extractseason-has-a-roman-numeral-fallback-kept-out-of-reseasonpatterns));
if one still fails, compare how the numeral is spelled in the torrent vs. the AniList title.

**6d. Batch detection** — `reBatchPatterns`. Single-episode searches drop packs on purpose: the pack
path runs earlier over the same anime search and should have caught it
([decisions.md #59](../../../docs/agents/decisions.md#59-batch-eligibility-stopped-being-metadata-and-became-a-result-filter)).

**6e. Title match / Jaccard** — `titleMatchesQuery` (`nyaa_match.go`), before the episode/season/batch
filters. Two checks:
1. every query title token must appear in the torrent name;
2. Jaccard(query tokens, torrent tokens) ≥ 0.8 for queries of ≤ 3 tokens (strict — keeps "SAO" off
   "SAO Alternative Gun Gale Online"), ≥ 0.4 for ≥ 4 tokens (fansubs that embed both titles).

Torrent-side metadata missing from `titleTechnicalTokens` inflates the token count and sinks the
Jaccard — the most common cause of `results: 0` when Nyaa clearly has it. Compute it:

```
query tokens   = extractTitleTokens("<search variant>")
torrent tokens = extractTitleTokens("<torrent name>")
jaccard        = |intersection| / |union|
```

Below the bar, check the second attempt before touching the allowlist: the match repeats both checks
on each `altTitleCandidates(<tail after the marker>)`
([decisions.md #75](../../../docs/agents/decisions.md#75-truncation-at-the-marker-is-the-first-attempt-of-the-title-match-not-the-only-one)).
Only then add the offending tokens to `titleTechnicalTokens` (streaming tags `nf`/`amzn`/`cr`, codec
fragments `eac3`/`ddp2`, `multisub`/`multi`/`subs`, …).

### 6f. No pack survives — downloads episode by episode with a pack on Nyaa

Symptom: `Attempting to download episode` for every episode and **no**
`Using batch torrents to cover the pending window` (`daemon/episodes.go`), while
`Found anime torrents on Nyaa` shows results that are all single episodes. Filtering, not a missing
pack.

Find **which** filter killed each pack (this closed Mushoku Tensei II Part 2, `166873`):

1. Take the raw names: the pass's `Raw Nyaa row` lines (`debug.jsonl` or `daemon.log`).
2. For **each** pack name, run `IsBatch`, `ExtractSeason`, `ExtractPart` and `titleMatchesQuery`
   with the query the daemon actually used (the variant, not the AniList title) — a throwaway test
   in `src/tests/unit/` is the quickest harness.
3. Build the table `torrent × batch × part × match`. The `false` column is the culprit, and each row
   can die for a different reason.

Already fixed — if one of these reappears, it is a regression or a format the fix doesn't see:
- `truncateAtFirstMarker` eating the alternative title → `altTitleCandidates` (#75). Brackets and
  loose text stay out on purpose.
- Hard part filter killing a whole-season pack, and `ExtractPart` reading `Part 1 + Part 2` as `1`
  → on the pack path, part only rejects a declared conflict and a two-part name returns `nil`
  ([decisions.md #79](../../../docs/agents/decisions.md#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses);
  `docs/agents/sources.md`, "Pack granularity and numbering").

Still open by design:
- **The pack's range is on another ruler** (continuous season, absolute across the series), so
  `pickBatches` doesn't pick it although it survived the filters. Run `newPackAxis`/`localRange`
  (`daemon/episodes.go`) with the entry's episode total: if no hypothesis covers, the heuristic
  needs another one; the tie-break is the pack's file count.

**The comparison that closes it fast:** run the same pass on the **sibling** entry with no "Part"
in the title (Part 1 of the same season). If the sibling finds a pack and Part 2 doesn't, a
part-dependent filter is still on the path.
