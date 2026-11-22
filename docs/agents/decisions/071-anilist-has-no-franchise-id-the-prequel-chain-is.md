# 71. AniList has no franchise id: the `PREQUEL` chain is the only source, and searching by name is a trap

**Location:** `src/internal/anilist` (queries that ask for `relations`); `daemon.ComputeEpisodeOffset`
(`daemon/helpers.go`). Complements #9, which covers the offset *gate*; here is its **source**.

**What it looks like:** you can identify "the series" — the whole franchise, across cour and part —
by some AniList id, or, failing that, by a name search (`search: "Monogatari"`).

**Why it's wrong — measured by introspection on 28 Aug 2026.** The `Media` type has 55 fields and
**none** of franchise:

```
id, idMal, title, type, format, status, description, startDate, endDate, season, seasonYear,
seasonInt, episodes, duration, chapters, volumes, countryOfOrigin, isLicensed, source, hashtag,
trailer, updatedAt, coverImage, bannerImage, genres, synonyms, averageScore, meanScore,
popularity, isLocked, trending, favourites, tags, relations, characters, staff, studios,
isFavourite, isFavouriteBlocked, isAdult, nextAiringEpisode, airingSchedule, trends,
externalLinks, streamingEpisodes, rankings, mediaListEntry, reviews, recommendations, stats,
siteUrl, autoCreateForumThread, isRecommendationBlocked, isReviewBlocked, modNotes
```

There is no "series" entity in the schema, and `idMal` has **the same granularity** (one id per
cour). The only edge between cours is `relations`. Walking `relationType == "PREQUEL"` filtering
`format ∈ {TV, TV_SHORT}` is not one option among several — it is the only source available.

**A name search is the shortcut that corrupts everything.** It would be one request, and it was
measured:

- `search: "Monogatari"` → 28 TV results, almost all other works (`Ore Monogatari!!`,
  `Shouwa Monogatari`, `Perrine Monogatari`, `Gokinjo Monogatari`).
- `search: "Shingeki no Kyojin"` → 7 results, including `Shingeki! Kyojin Chuugakkou` — the
  12-episode chibi parody, which would enter any episode sum and shift the whole series.
- And it does not return **order**: the sequence would have to be inferred from the titles, which
  is exactly the string-parsing problem that already takes packs down in `nyaa_match.go`.

The `relations` graph is authoritative; the name is not.

**Three `relations` facts that define any implementation:**

1. **`Media.relations` accepts no arguments.** No server-side filter by `relationType`, no
   pagination: every query brings *all* the edges, and filtering is always the client's job.
2. **`relations` can be nested, but AniList cuts at 2 levels.** Tested with depths 1 to 8: the body
   saturates at 5865 bytes from depth 3, and at the third level `relations` comes present with
   `edges: []`. Confirmed on Monogatari, Mushoku Tensei and Shingeki: **2 PREQUEL hops per query,
   always.**
3. **`edges: []` at the cut level does NOT mean root.** This is the serious trap: at the level
   where AniList stops expanding, the response is indistinguishable from an anime that genuinely
   has no prequel. Treating the two as equal stops the walk early and produces a
   **silently wrong** offset. Whoever implements this needs to mark which nodes arrived in an
   authoritative position (query levels 0 and 1) and re-fetch the cut-level ones before concluding
   anything about them. The ceiling of 2 is observed behaviour, not documented — the loop must
   consume what comes and enqueue what is missing, without assuming any depth.

**The chain was validated against the real API** (28 Aug 2026), and the strong validation is
Shingeki: the accumulated sum gives `76..87` for Final Season Part 2, which is **exactly** the
numbering SubsPlease and Erai-raws use on the files. The chain reproduces the real naming
convention with no adjustment. Monogatari (7 levels, crooked chronological order) also closes
coherently.

**Erring to 0 is cheap; erring to a number is not.** A missing offset falls back to relative
numbering, which a good part of the groups use. A wrong offset sends the search to an episode that
does not exist. Every doubt in the walk resolves to 0.

**Don't "fix" by:** looking for a franchise id in the schema "that must exist somewhere"; using
`idMal` as series identity; replacing the walk with `search:` and the franchise name; treating
`edges: []` as root; assuming depth 2 as a contract; including `MOVIE`/`OVA`/`ONA` in the walk
(see #9).
