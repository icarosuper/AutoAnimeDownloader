# 75. Truncation at the marker is the first attempt of the title match, not the only one

**Location:** `internal/nyaa/nyaa_match.go` (`titleMatchesQuery`, `altTitleCandidates`,
`tokensMatchQuery`, `firstMarkerIndex`). Pinned by `TestTitleMatchesQuery_AltTitleAfterMarker` and
`TestTitleMatchesQuery_AltTitleDoesNotAcceptWrongAnime` (`src/tests/unit/nyaa_test.go`), with real
names collected from Nyaa.

**What it looks like:** `titleMatchesQuery` runs the two checks (every query token present +
Jaccard) first on the name truncated at the marker — exactly like #18 — and, only if that fails,
repeats over **each** alternative title that appears AFTER the marker, one at a time. Each stretch
between parentheses and each slash-separated segment is its own candidate.

**Why it's right:** #18's premise ("everything after the marker is noise") breaks on a whole family
of names. EMBER writes
`[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [...] (Mushoku Tensei II:
Isekai Ittara Honki Dasu Part 2) (Batch)`: the `Season 2` marker is INSIDE a group, and the full
romaji comes three groups later. Truncating left `[mushoku tensei jobless reincarnation]`, and the
romaji query was rejected for a missing token — the anime's ONLY Part 2 packs died before reaching
the batch path. Diddy, NTRX, Fuchs and VARYG do the same with a slash instead of parentheses.

**Why one candidate per stretch, not a single bag of tokens.** That was the first attempt, and
measuring made it worse: joining the English title from the prefix with the romaji from the footer
dilutes both (Jaccard 0.78 on the EMBER case, against **1.00** of the romaji compared alone), and
in `debug-batch` that let a stretch lend a token to its neighbour — Yameii's loose releases of
*SAO Alternative: Gun Gale Online* started matching the query of *Sword Art Online II*. Compared
stretch by stretch, each title is evaluated as a title.

**Why the second attempt only runs after the first fails.** That is what preserves all of #18.
Truncation is what protects a short query from sinking in Jaccard: the pack
`[EMBER] Kimetsu no Yaiba (2024) (Season 4) [...] (Kimetsu no Yaiba: Hashira Geiko-hen | Demon
Slayer: Kimetsu no Yaiba Hashira Training Arc) (Batch)` matches the 3-token query on the truncation
(1.00) and never reaches the candidates — where it would score 0.5 and be rejected. Expanding the
tokens for everyone would swap one false negative for another.

**Measured in `make debug-batch`** (corpus of `scripts/robustness-animes.txt`, before and after): no
match lost; gains on `108465`, where EMBER's season 1 BDRip pack started matching and became the
best option (492 seeders, against 65 of the Judas that was leading). On `166873`, which motivated
the investigation, EMBER's three Part 2 packs started surviving the filter (they were zero).

**Known limitation, PRE-EXISTING and not introduced here:** the query `Sword Art Online II` matches
the loose releases of *SAO Alternative: Gun Gale Online II* (Jaccard 0.57, above the 0.4 of a long
query). Erai-raws releases of that same spinoff already matched before this change, through the
truncation path — what changed was only consistency across releases. Fixing that is touching
`jaccardThreshold`, which is another feature.

**Don't "fix" by:** tokenizing the whole name (brings episode title and release tag, and sinks
Jaccard), nor joining the candidates into one set (measured above), nor creating a threshold of its
own for the second attempt (magic number fitted to five samples).
