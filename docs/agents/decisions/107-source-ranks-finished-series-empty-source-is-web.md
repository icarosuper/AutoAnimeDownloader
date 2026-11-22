# 107. source ranks finished series, empty source is web-dl, health bands are 10/50

**Location:** `nyaa/priorities.go` — `finishedSeriesCriteria`, `SortFinishedSeriesResults`,
`sourceCompare`, `healthTierFloors`; `nyaa/nyaa_regex.go` — `[BD]`; `daemon/search.go` —
`rankSeriesResults`.

**What it looks like:** `source` is in the default `criteria_order` and on the Priorities
screen but `SortTorrentResults` still ignores it. A second function re-sorts the same list
when the anime is FINISHED. Empty source is treated as `web-dl`, unlike codec's empty-tie.
Health bands collapsed from five floors to `{10, 50}`.

**Why it's right:**

(a) The gate is AniList Media.Status, not MediaListStatus. CURRENT/COMPLETED/REPEATING is
the user's watch state. A weekly show the user marked complete is still RELEASING on
AniList until it actually ends; a finished show on a CURRENT list is the backlog case.
nyaa does not import anilist — the daemon passes a bool.

(b) Empty source = web-dl, not a tie. Weekly names (`[SubsPlease] Anime - 07 (1080p)`)
carry no source token. `priorityIndex` would send them to the end (lose to tagged WEB-DL).
codec's "either side empty ⇒ tie" would also tie untagged WEB with tagged BD and throw
the criterion away.

(c) Health before source, resolution before source, in the default order. A lexicographic
sort never consults a later criterion once an earlier one differs, so source-before-health
would pick an 8-seeder BD over an 80-seeder WEB. Bands 10/50 are the split among
min_seeders survivors: 3–9 fragile, 10–49 completes, 50+ the user's connection is the
limit.

(d) No criteria_order migration. LoadConfigs unmarshals over defaults, same as #55.

(e) `[BD]` is the usual anime-pack tag and was invisible to the extractor. Bare `\bTV\b`
matched AniList's `(TV)` format marker in titles.

**Don't "fix" by:** putting source in episodeCriteria "so one sort does both"; gating on
MediaListStatus; auto-migrating criteria_order; treating empty source as a codec-style
tie; putting audio on episodes; auto-replacing a saved WEB episode when a BD appears.
