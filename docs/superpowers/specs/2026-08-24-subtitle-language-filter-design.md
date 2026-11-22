# Subtitle language filter in the priorities

**Date:** 2026-08-24
**Status:** designed, not implemented
**Closes:** [backlog/subtitle-language-filter.md](../../backlog/subtitle-language-filter.md)
**Scope:** `internal/nyaa` (config, signal, sorting, search category),
`internal/daemon/search.go` (hard filter), `internal/files` (config field),
`frontend/routes/Priorities.svelte` (UI).

---

## The problem, and why the obvious answer is wrong

The question that opened the design was: *"would we have to open the torrent pages to read the
description, or else list the torrent's files, to find that out?"*

Both hypotheses were measured against the real Nyaa on 2026-08-24. Both are wrong as the main
path, and the measurement found a third blocker that neither of them saw.

### Listing the torrent's files does not work

Anime subtitles are **softsubs, muxed inside the `.mkv`**. The view page of
`[Erai-raws] Mushoku Tensei III - 09 [MultiSub]` (id 2150451) has a `#### File list` with **one
file**: the `.mkv` itself. No loose `.ass`.

This also holds for AnimeTosho's `files[]` described in [sources.md](../../agents/sources.md):
it gives `filename`/`size`/`crc32`, and none of them knows the subtitle language. The file list
answers "which episodes does the pack cover", not "what subtitles are inside".

### The description works, but for few uploaders

The view page (`/view/<id>`, ~17 KB, 1 GET) carries a free-form block. Erai-raws:

```
`Subtitles Info:` **English** (CR) ASS **Portuguese** (Brazilian_CR) ASS
**Spanish** (Latin_America_CR) ASS **Arabic** (CR) ASS ... **Polish** (CR) ASS
```

ToonsHub uses another format (`Subtitles:** English, Arabic, ...`). SubsPlease publishes nothing.
The native PT-BR groups don't either — they put it in the title.

**You can't search for it.** `?q=Brazilian_CR&c=1_2` returns **0 results**: Nyaa does not
index the description. The view page can only be reached one id at a time.

### The real blocker: the category is fixed to English

`nyaa.go:461`, `nyaa.go:624` and `nyaa.go:765` hardcode `c=1_2` = *Anime - English-translated*.

Every **native** PT-BR release lives in `c=1_3` = *Anime - Non-English-translated*. **Today the
daemon sees none of them.** No amount of description reading fixes that, because those torrents
never even show up in the search.

And in `c=1_3` the marker comes in the title, for free:

```
[WF] BLEACH: Sennen Kessen-hen - 44 [WEB-DL AMZN 1080p H.264 DDP][PT-BR]
[Pharaoh] Yu-Gi-Oh! 1998 - 06 [1080p][HEVC x265 10bits][PT-BR]
[AT] Paranoia Agent (Mousou Dairinin) [1080p][PT_br][Legendado][AnimesTorrent.com]
[Kanshin] Card Captor Sakura 720p (PT-BR) (legendado)
[Unison FanSub] Naruto Classic - 12 (Portuguese sub)
```

### `c=1_0` is a trap

The lazy way out would be to swap `c=1_2` for `c=1_0` (*Anime*, all subcategories) and let the
rest of the pipeline filter. Measured, `q=frieren`:

| category | English-translated | Non-English-translated |
|---|---|---|
| `c=1_0` | **73** | **2** |
| `c=1_2` | 75 | 0 |
| `c=1_3` | 0 | 75 |

`s=seeders&o=desc` buries the PT-BR under the English release, which has an order of magnitude
more seeders. It is the **exact shape** of the bug in
[decisions/059](../../agents/decisions/059-batch-eligibility-stopped-being-metadata-and.md)
(pack buried under loose episodes). Seeing `1_3` takes a **separate search**, not a wider parameter.

---

## The two PT-BR populations

| | where | signal in the title | measured coverage |
|---|---|---|---|
| **A. MultiSub** (Erai-raws, ToonsHub) | `c=1_2` — already downloaded today | only `[MultiSub]` / `Multi Subs`, no enumeration | ~every airing anime |
| **B. Native PT-BR** (WF, Pharaoh, AT, Kanshin) | `c=1_3` — invisible today | `[PT-BR]`, `(Legendado)` | sparse: 2 hits for Frieren, **0** for One Piece and Mushoku Tensei |

The consequence that steers everything else: **for airing anime, the PT-BR already on disk
came from Erai-raws without anyone knowing.** For backlog, native PT-BR exists but is rare per anime.
A design that ignores population A ships a feature that doesn't work precisely in the common case.

### Per-group heuristics don't work — measured

The temptation is to say "Erai-raws and ToonsHub always have PT-BR" and skip the view page. Erai-raws is
consistent (3/3 distinct releases with `**Portuguese** (Brazilian_CR)`, identical format), but
only because it is Crunchyroll and nothing else. ToonsHub, 3/3 distinct:

| release | source | subtitles |
|---|---|---|
| Daemons of the Shadow Realm S01E20 | NF | English, Chinese (Traditional), Indonesian, Korean, Malay, Thai, Vietnamese — **no Portuguese** |
| Chainsmoker Cat S01E08 | NF | …, **Portuguese (Brazil)**, … |
| BLEACH TYBW S01E43 | DSNP | with Portuguese |

ToonsHub's "Multi Subs" reflects the regional set of the source streaming service — Netflix Asia versus
Netflix Brazil. The heuristic would be wrong ~1/3 of the time, **and it would be wrong by saying "it has it" when it doesn't**,
which is the direction that breaks hard mode. That is why hard mode really does need the view page.

---

## Decisions

| # | Decision | Rejected alternative |
|---|---|---|
| 1 | Preference by default, with a global flag to turn it into a hard filter | Hard only (empties airing anime); preference only (doesn't serve whoever requires PT-BR) |
| 2 | Extra search in `c=1_3`, and **only** when a non-English language is configured | `c=1_0` (buries by seeders); always (doubles requests for those who don't use it) |
| 3 | The hard-mode flag is global, in `Priorities` | Per anime — fits in `AnimeSettings`, but costs a per-anime endpoint + UI. Noted in the TODO |
| 4 | View page only in hard mode, only for ambiguous candidates, with short-circuit and cache | View page for every top-N candidate (multiplies requests on the hot path for data only hard mode consumes) |
| 5 | Grep over the description text, no structured parser | Parser for the `Subtitles Info:` block — diverges between uploaders and breaks silently |
| 6 | Closed language vocabulary | Free list of strings: `c=1_3` wouldn't know whether it should run |

---

## Design

### 1. Config

In `internal/nyaa/priorities.go`:

```go
type Priorities struct {
    // ... existing fields
    // SubtitleLanguages is the subtitle language preference order, by known
    // key (see subtitleLangs). Empty = feature off.
    SubtitleLanguages []string `json:"subtitle_languages"`
    // RequireSubtitleLanguage turns the preference into a hard filter: a release
    // that has none of the languages in SubtitleLanguages is dropped.
    RequireSubtitleLanguage bool `json:"require_subtitle_language"`
}
```

`DefaultPriorities()` returns `nil` and `false`. An existing `config.json` loads with it off —
`LoadConfigs` deserializes on top of the defaults, so no installation wakes up with new
behavior. Same contract as `notifications.batch_window_seconds`
([config.md](../../agents/config.md)).

`CriteriaOrder` gains `"subtitle"` in the default, **between `health` and `fansub`**:

```go
CriteriaOrder: []string{"uncensored", "source", "resolution", "health", "subtitle", "fansub", "audio", "size"}
```

Language matters more than which group signed the release, but not to the point of accepting a dead torrent. Same
release caveat that already applies to `criteria_order`: an existing `config.json` **keeps the
order it already has** and never sees the new criterion until it is edited by hand or the user clicks "restore defaults".
With `subtitle` absent from the order, `sortByCriteria` simply skips it (the `if !ok { continue }` already
exists) — silent and correct degradation.

### 2. Language vocabulary

```go
type subtitleLang struct {
    TitleAliases []string // matched against the torrent name
    DescAliases  []string // matched against the view page body (superset)
    NyaaCategory string   // "1_2" or "1_3": where this language lives
}

var subtitleLangs = map[string]subtitleLang{
    "pt-br": {
        TitleAliases: []string{"pt-br", "ptbr", "pt_br", "pt br", "português", "portugues", "portuguese", "legendado"},
        DescAliases:  []string{"portugu", "pt-br", "brazil"},
        NyaaCategory: "1_3",
    },
    "en": {TitleAliases: []string{"eng", "english"}, DescAliases: []string{"english"}, NyaaCategory: "1_2"},
    "fr": {
        TitleAliases: []string{"vostfr", "subfrench", "french"},
        DescAliases:  []string{"french", "français", "francais"},
        NyaaCategory: "1_3",
    },
    "es-la": {
        TitleAliases: []string{"es-la", "esla", "castellano", "español", "espanol", "spanish", "subtitulado"},
        DescAliases:  []string{"spanish", "español", "espanol"},
        NyaaCategory: "1_3",
    },
}
```

**These four are the complete initial set** — `pt-br`, `en`, `fr`, `es-la`. It is not a sample
to be expanded during implementation: `fr` and `es-la` are in because they are cheap (same mechanism, just
more aliases) and because they validate that the design is not a PT-BR special case. A new language later
is one entry in the map and nothing more.

Closed on purpose (decision 6): it is `NyaaCategory` that says whether the `c=1_3` search needs to run.

**Two doors, two behaviors, and it is not a contradiction:** `handleUpdateConfig` **rejects** an
unknown key with HTTP 400 (it is a user typo, and failing loudly is what spares them from
finding out weeks later); `LoadConfigs` only **ignores** it with `logger.Warn` (a `config.json`
written by a future version, or edited by hand, must not stop the daemon from starting).

Case-insensitive substring matching over the name, as `ShouldIgnore` and `fansubPriority` already
do. **No `\b`** on the short hyphenated aliases (`pt-br` appears inside `[PT-BR]` and
`[1080p][PT_br]`), but `"pt br"` and `"eng"` need a boundary so they don't match inside a
word — `"eng"` would match in "Avenger", "Revenge", "Kingdom". A per-language regex, precompiled in
package init like the rest of `nyaa_regex.go`, is the way: the regexes run inside a
per-torrent loop and compiling at call time is slow (comment at the top of `nyaa_regex.go`).

### 3. Three states, not two

```go
type SubtitleMatch int

const (
    SubtitleNo      SubtitleMatch = iota // different language identified in the title
    SubtitleUnknown                      // multi marker without enumerating languages
    SubtitleYes                          // alias of the requested language in the title
)
```

| state | how you get there | example |
|---|---|---|
| `SubtitleYes` | alias of a `SubtitleLanguages` language in the title | `[WF] ... [PT-BR]`, `... (Legendado)` |
| `SubtitleUnknown` | `reMultiSub` matches and no specific alias matches | `[Erai-raws] ... [MultiSub]`, `[ToonsHub] ... [Multi Subs]` |
| `SubtitleNo` | none of the above | `[SubsPlease] ... (1080p)`, `... VOSTFR` when the request is pt-br |

```go
var reMultiSub = regexp.MustCompile(`(?i)\bmulti[\s._-]?subs?\b|\bmultiple\s+subtitle\b|\bmulti\s+subs\b`)
```

A release with no marker at all in `c=1_2` is `SubtitleNo` for pt-br by the general rule — the
category already states "English-translated". No need to reason about the category in code:
no alias and no multi marker falls into `SubtitleNo`.

**Known and accepted false negative:** a `c=1_3` release with no marker at all also falls into
`SubtitleNo`, even though it sits in a category that guarantees it is *not* English. Measured:
`[RLSP] One Piece 733-736 [BD 720p]` and `[KiyoshiiSubs] One Piece - 1172v2 [1080p]` are like that — the
language is only in the group name. Marking them `Unknown` by category would be more accurate, but in
hard mode it would fire a view page GET for every French, Hungarian or Russian release the `1_3`
search brings back — and those are most of it. It stays `No`: a PT-BR group without a marker is lost,
a flood of useless fetches is avoided. If demand shows up, the path is for the user to put the group in
`priorities.fansubs`, not for the category to become a signal.

`TorrentResult` gains two fields:

```go
SubtitleMatch SubtitleMatch `json:"-"`               // derived, not serialized
ViewURL       string        `json:"viewUrl,omitempty"` // "/view/<id>", only used in hard mode
```

`ViewURL` comes from `cells.Eq(1).Find("a").Not(".comments").AttrOr("href", "")` — today that cell is
read only for its text (the name) and the href is discarded. The three `parseRow` start filling it in.

### 4. Sorting — preference mode (the default)

New criterion in `criterionCompare`:

```go
"subtitle": func(a, b TorrentResult) int { return int(b.SubtitleMatch) - int(a.SubtitleMatch) },
```

Resulting order: **Yes < Unknown < No**.

`SubtitleUnknown` in the middle is deliberate. An Erai-raws `[MultiSub]` *probably* has PT-BR
(3/3 measured); demoting it to SubsPlease's level would throw away the best bet available with
no contrary evidence. Promoting it to `Yes` would be the lie the ToonsHub measurement
disproved.

`subtitle` goes into `episodeCriteria` (the subset `SortTorrentResults` uses) — otherwise the criterion
would only apply to movies, which is the rare case.

**In this mode the network cost is zero.** Everything comes from the title, which is already in memory.

### 5. Search in `c=1_3`

At the three call sites `c` stops being a literal:

```go
// nyaaCategories returns the categories to scan: always 1_2, plus 1_3 when
// some configured language lives there.
func nyaaCategories() []string
```

With `SubtitleLanguages` empty it returns `["1_2"]` — **request count identical to today's**.
With `pt-br` configured it returns `["1_2", "1_3"]`, and each search runs `fetchSearchPages` once
per category, concatenating into `results` before `SortTorrentResults`. The existing
`deduplicateByMagnet` takes care of overlap.

The page descent (`fetchSearchPages`, which only goes past page 1 while it has fewer than 3
accepted candidates) runs **per category**, not over the total: otherwise 3 English candidates on
page 1 of `1_2` would end the descent and `1_3` would never be scanned in depth. It is the same trap
as `c=1_0`, just inside the loop.

### 6. Hard mode

A fourth filter in `filterSearchResults` (`daemon/search.go:96`), after the size ceiling and the
seeders floor, and **only** when `RequireSubtitleLanguage`.

The signature has to change: today it is `filterSearchResults(results, maxGB, minSeeders)` and the
short-circuit depends on `EpisodeRetryLimit`. With three config parameters already in play, passing
the whole `*files.Config` is cheaper than a fourth scalar — and it is what the five call
sites already have at hand: `debug.go:138`, `episodes.go:216`, `:293`, `:407` and `:408`. It becomes
`filterSearchResults(results, maxGB, minSeeders, configs)`; `maxGB` stays explicit because it
alternates between `MaxBatchTorrentSizeGB` and `MaxEpisodeTorrentSizeGB` depending on the path.

The algorithm:

```
for each result, in the already-sorted order:
    SubtitleYes     → accept, no fetch
    SubtitleNo      → drop, no fetch
    SubtitleUnknown → 1 GET on ViewURL; strings.ToLower on the body; DescAliases regex
                      accept or drop according to the result
    stop resolving once EpisodeRetryLimit are accepted; the rest drop without fetch
```

**Short-circuit:** `attemptDownloadWithRetries` (`episodes.go:599`) only tries the first
`EpisodeRetryLimit` magnets. Resolving beyond that is a wasted fetch. Since the list arrives
sorted, stopping at `EpisodeRetryLimit` accepted costs exactly the same as resolving at
download time — **without** touching the `magnets []string` signature, which today has already flattened the `TorrentResult`
before the download (`episodes.go:218`).

**Cache:** a `map[string]bool` keyed by `ViewURL`, guarded by a `sync.RWMutex`, in the
`nyaa` package. No TTL and no persistence: the content of a Nyaa view page does not change, and losing the
cache on restart costs one fetch. Without it, an anime that never resolves would redo the fetches **on every
10-minute pass** — which is the bad case the design exists to avoid.

**Injectable fetcher**, following the `MockNyaaHttpGet`/`SetPriorities` pattern:

```go
func SetSubtitleFetcher(fn func(viewURL string) (string, error)) (restore func())
```

That way `filterSearchResults` stays a list→list function testable without the network.

**No structured parser** (decision 5): Erai-raws' `Subtitles Info:` and ToonsHub's `Subtitles:**`
already diverge today, and a third uploader will diverge again tomorrow. Grep over the whole
lowercased body.

**A network failure counts as acceptance.** A GET that times out is no evidence that PT-BR is
missing, and treating it as a rejection would turn network instability into "no episode downloads".
It logs at `Warn` and the result passes. A failure does **not** go into the cache — otherwise a network blip
would poison the verdict until restart.

### 7. Report

The language cut has to go into `search.go`'s `dropStats`, next to `sizeDropped` and
`seedersDropped`, and reach `searchIssue` (`episodes.go:231`). Without it the report says
"no torrent found" when the truth is "3 found, all without PT-BR" — and that is
exactly the difference between the user understanding or not understanding why the episode didn't download.
Same logic that motivated the original `dropStats`.

### 8. Frontend

`Priorities.svelte` is generic over `LISTS`, all `string[]`. `subtitle_languages` is `string[]`
and goes in as one more entry — but with two differences:

- **Closed vocabulary:** the add field becomes a `<select>` with the keys of
  `subtitleLangs`, instead of the free-text `<input>`. The keys come from
  `GET /config/priorities/defaults`, which starts returning the list of supported languages too —
  no new endpoint.
- **`require_subtitle_language` is a bool**, it doesn't fit the generic list: a checkbox right below,
  disabled while `subtitle_languages` is empty (the flag means nothing without a language
  configured).

Label in literal Portuguese, like the other `LISTS` entries (`Priorities.svelte:13-21`).

---

## Tests

In `nyaa/priorities_test.go` and `daemon/search_test.go`, following the mock pattern in
[testing.md](../../agents/testing.md).

**Unit — state detection.** Table title → `SubtitleMatch` for pt-br, covering the three
states: `[PT-BR]`, `PTBR`, `[PT_br]`, `(Legendado)`, `Portuguese sub` → `Yes`; `[MultiSub]`,
`Multi Subs`, `Multiple Subtitle` → `Unknown`; `[SubsPlease] ... (1080p)`, `... VOSTFR` → `No`.
Plus the boundary case: a title with `"Avenger"` must **not** match `en`.

**Unit — sorting.** Three results identical in everything but the state, confirming
Yes < Unknown < No, and that `subtitle` absent from `criteria_order` doesn't break the sort.

**Unit — hard mode with a mocked fetcher.** An `Unknown` whose body contains "portuguese (brazil)"
passes; an `Unknown` with only "english, korean, thai" drops; the **second** encounter with the same `ViewURL`
does not call the fetcher (cache); a fetcher error lets it pass and does **not** populate the cache.

**Unit — short-circuit.** A list with 10 `Unknown` and `EpisodeRetryLimit = 3` calls the fetcher at
most 3 times.

**Unit — categories.** `nyaaCategories()` returns `["1_2"]` with an empty list and with `["en"]`;
returns `["1_2","1_3"]` with `["pt-br"]`.

**Integration — the feature is free for those who don't use it.** With `subtitle_languages` empty, the
count of GETs to Nyaa in one check pass is **identical** to today's. It is the test that
protects decision 2, and the only one that fails if someone "simplifies" `nyaaCategories` later.

---

## Checklists to follow

Config field ([conventions.md](../../agents/conventions.md#adding-a-new-config-field--checklist)):
the field lives in `nyaa.Priorities`, which is already serialized inside `Config.Priorities` — nothing to
do in `filemanager.go` beyond the default coming from `DefaultPriorities()`. Validation in
`handleUpdateConfig`: reject an unknown language key with HTTP 400.

After implementation, per [CLAUDE.md](../../../CLAUDE.md):

1. `go test ./...`
2. `docs/agents/config.md` — rows for `priorities.subtitle_languages` and
   `priorities.require_subtitle_language`, with the release note about an existing `criteria_order`
   not gaining `subtitle` on its own
3. `docs/agents/architecture.md` — new `TorrentResult` fields, `nyaaCategories`
4. `docs/agents/decisions/` — entry for "why a separate search in `1_3` and not `c=1_0`",
   anchored on the Frieren 73/2 measurement
5. `docs/agents/sources.md` — the Nyaa section states "Today the daemon uses `f=0`" and describes `c=1_2`
   as fixed; it becomes conditional
6. `swag init -g src/cmd/daemon/main.go -o docs/swagger` — the
   `/config/priorities/defaults` payload changes

---

## Out of scope, deliberately

- **Per-anime override.** It makes sense (accept English for a shounen and require PT-BR for what you
  watch with someone else) and `AnimeSettings` is the place, but it costs a per-anime endpoint + UI. It stays
  out of this spec.
- **Listing the torrent's files.** Softsubs live inside the `.mkv`; there is nothing to list. Measured.
- **AnimeTosho as a language source.** Its `files[]` carries `filename`/`size`/`crc32` and none of them
  knows the subtitle language. It doesn't change the verdict in [sources.md](../../agents/sources.md).
- **Selecting the subtitle track in the player / remuxing.** The app delivers the file; which track
  Jellyfin picks is Jellyfin configuration.
- **`c=1_1` (AMV) and `c=1_4` (Raw).** Neither hosts subtitled episodes.
