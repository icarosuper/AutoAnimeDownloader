# Source Ranking for Finished Series Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rank Blu-ray above WEB on series whose AniList **media** status is `FINISHED` (standalone, packs, and singles), while weekly `RELEASING` keeps today's source-free episode sort.

**Architecture:** `nyaa.SortTorrentResults` stays the RELEASING sort (no source). A new `nyaa.SortFinishedSeriesResults` is a second full `sort.SliceStable` over `CriteriaOrder` with `source` allowed. The daemon applies it via `rankSeriesResults(results, finished)` after scrape and before partition/pick, at the three call sites that already have `Media.Status`. `nyaa` does not import `anilist`. Health bands and the empty-source=`web-dl` comparator are package-wide (movies included). Existing `criteria_order` is not migrated.

**Tech Stack:** Go (daemon + `internal/nyaa`), Svelte 5 Priorities screen, paraglide i18n (`messages/en.json` + `messages/pt-BR.json`).

**Spec:** `docs/superpowers/specs/2026-10-24-source-ranking-finished-series-design.md`

**Worktree:** `/tmp/aad-worktrees/source-priority-finished` on branch `source-priority-finished` (base: master). Do not write in the main checkout.

## Global Constraints

- Everything in English: identifiers, comments, logs, docs, commit subjects. Portuguese only in `messages/pt-BR.json`. Found Portuguese in a file being touched → translate it in the same change.
- Conventional Commits, English, subject line only, no body, no trailing period, all lowercase after `type:`.
- `go test ./...` after any code change. If GOCACHE is permission-denied, set `GOCACHE` to a writable dir under the worktree (e.g. `/tmp/aad-worktrees/source-priority-finished/.gocache`) rather than skipping tests.
- No new config field, no new endpoint, no CLI command. `config set` already writes `priorities.*`.
- Do not auto-migrate a saved `criteria_order`.
- Do not put `audio` in the series sort. Do not apply source to `RELEASING` / `HIATUS` / `CANCELLED`.
- Do not change `ScrapNyaa` / `ScrapNyaaForAnime` signatures; they keep calling `SortTorrentResults`.
- Do not implement the torrent picker (`preview.go` / `GET /animes/{id}/torrent-candidates`).
- TDD: write the failing test, watch it fail for the right reason, then the minimal production code.
- New decision number is **#106** (this branch's `decisions.md` ends at #104; `v2.7.0` already claimed #105 for FetchTorrentDetail — skip 105 to avoid a number collision).
- NEVER commit or push from a request the user did not make; this plan's per-task commits are requested.
- Comments explain **why**, not **what**. Non-obvious decisions point at `decisions.md #N`.

---

## File structure

| File | Responsibility |
|---|---|
| `src/internal/nyaa/priorities.go` | Default `criteria_order`, `healthTierFloors`, `sourceCompare`, `finishedSeriesCriteria`, `SortFinishedSeriesResults` |
| `src/internal/nyaa/nyaa.go` | Export `ExtractSource`; `SortTorrentResults` comment; `SortMovieResults` comment |
| `src/internal/nyaa/nyaa_regex.go` | `[BD]`/`\bBD\b` after specific BD patterns; drop bare `\bTV\b`; keep `TVRip`/`HDTV`/`[TV]` |
| `src/tests/unit/priorities_test.go` | Health bands, default order, TokensAreReachable walks Sources, sourceCompare, SortFinishedSeriesResults worked examples, SortTorrentResults ignores source |
| `src/internal/nyaa/priorities_health_test.go` | Package-nyaa `healthTier` exact values (unexported) |
| `src/internal/daemon/search.go` | `rankSeriesResults` |
| `src/internal/daemon/episodes.go` | Re-rank after `searchAnime` and after `searchSingleEpisode` |
| `src/internal/daemon/debug.go` | Same two re-ranks as production |
| `src/internal/daemon/manual_download.go` | Re-rank `ManualDownloadEpisode` with `details.isFinished` |
| `src/internal/daemon/rank_test.go` | FINISHED vs RELEASING pack pick |
| `src/internal/frontend/src/routes/Priorities.svelte` | Sources no longer `moviesOnly`; new scope hint |
| `src/internal/frontend/messages/en.json` | Scope + criteria-order note |
| `src/internal/frontend/messages/pt-BR.json` | Portuguese translations of those two strings |
| `docs/agents/config.md` | `criteria_order` default, `sources` scope, health-band note next to `min_seeders` |
| `docs/agents/architecture.md` | Sorting paragraph: finished-series source subset, floors `10/50` |
| `docs/agents/decisions.md` | #106 + revise #69 |
| `docs/guides/webui.md` | Priorities blurb |
| `docs/superpowers/specs/2026-10-24-source-ranking-finished-series-design.md` | Status → implemented (last task) |

---

### Task 1: Health bands `{10, 50}`

**Files:**
- Modify: `src/internal/nyaa/priorities.go` (`healthTierFloors` and its comment, ~lines 115–117)
- Modify: `src/tests/unit/priorities_test.go` (comments on the existing health tests that name the old 100–399 band)
- Create: `src/internal/nyaa/priorities_health_test.go`

**Interfaces:**
- Consumes: existing `healthTier(r TorrentResult) int` and `ParseSeeders`
- Produces: `healthTierFloors = []int{10, 50}` so seeders 3–9 → tier 0, 10–49 → 1, 50+ → 2

- [ ] **Step 1: Write the failing package test**

Create `src/internal/nyaa/priorities_health_test.go`:

```go
package nyaa

import "testing"

func TestHealthTier_FloorsTenAndFifty(t *testing.T) {
	cases := []struct {
		seeders string
		want    int
	}{
		{"9", 0},
		{"10", 1},
		{"49", 1},
		{"50", 2},
	}
	for _, tc := range cases {
		got := healthTier(TorrentResult{Seeders: tc.seeders})
		if got != tc.want {
			t.Errorf("healthTier(%s seeders) = %d, want %d", tc.seeders, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./src/internal/nyaa/ -run TestHealthTier_FloorsTenAndFifty -count=1`

Expected: FAIL — `healthTier(9 seeders)` is 2 (floors 1/5/20/100/400), want 0.

- [ ] **Step 3: Change the floors**

In `src/internal/nyaa/priorities.go`, replace:

```go
// healthTierFloors are the floors of the health tiers: seeders 0 / 1-4 / 5-19 / 20-99 /
// 100-399 / 400+ ⇒ tier 0..5. Each tier is ~4-5x the previous.
var healthTierFloors = []int{1, 5, 20, 100, 400}
```

with:

```go
// healthTierFloors are the floors of the health tiers: seeders 0–9 / 10–49 / 50+ ⇒ tier
// 0..2. min_seeders (default 3) already dropped 0–2, so 10 is the first split among
// survivors. 50 vs 5000 is the same download on a home connection (decisions.md #106).
var healthTierFloors = []int{10, 50}
```

Leave `healthTier` itself unchanged — it already walks floors.

- [ ] **Step 4: Run the new test and the existing health tests**

Run: `go test ./src/internal/nyaa/ ./src/tests/unit/ -run 'TestHealthTier_FloorsTenAndFifty|TestHealthTiers_' -count=1`

Expected: PASS. The two unit tests (12 vs 400 across tiers; 150 vs 300 same tier) still hold with the new bands.

- [ ] **Step 5: Update stale comments in the unit tests**

In `src/tests/unit/priorities_test.go`, `TestHealthTiers_SameTierLetsFansubDecide` currently says `150 and 300 are both in the 100-399 tier`. Change that comment to `150 and 300 are both in the 50+ tier`.

In the same file, `TestCodec_UntaggedNameIsNotPenalized` has a Portuguese comment `// hevc primeiro` — translate to `// hevc first`.

- [ ] **Step 6: Commit**

```bash
git add src/internal/nyaa/priorities.go src/internal/nyaa/priorities_health_test.go src/tests/unit/priorities_test.go
git commit -m "feat: compare health in 10/50 seeder bands"
```

---

### Task 2: Extract `[BD]`, drop bare `TV`, export `ExtractSource`

**Files:**
- Modify: `src/internal/nyaa/nyaa_regex.go` (`reSourcePatterns`, ~lines 115–127)
- Modify: `src/internal/nyaa/nyaa.go` (rename `extractSource` → `ExtractSource`, update the two call sites in `priorities.go` as well)
- Modify: `src/internal/nyaa/priorities.go` (`criterionCompare["source"]` currently calls `extractSource`)
- Modify: `src/tests/unit/priorities_test.go` (`TestDefaultPriorities_TokensAreReachable`)
- Modify: `src/tests/unit/nyaa_test.go` (add `TestExtractSource_*` next to the other extract tests, or in `priorities_test.go`)

**Interfaces:**
- Consumes: `reSourcePatterns` ordered first-match
- Produces: exported `func ExtractSource(name string) string`; `[BD]`/`\bBD\b` → `"BD"`; `(TV)` without `TVRip`/`[TV]` → `""`

- [ ] **Step 1: Write the failing extractor tests**

Add to `src/tests/unit/priorities_test.go` (package `unit`, so it needs the exported name):

```go
func TestExtractSource_BracketBD(t *testing.T) {
	got := nyaa.ExtractSource("[Group] Anime S01 [BD] [1080p]")
	if got != "BD" {
		t.Fatalf("ExtractSource([BD]) = %q, want BD", got)
	}
}

func TestExtractSource_BareTVInTitleIsNotASource(t *testing.T) {
	got := nyaa.ExtractSource("[SubsPlease] Lucky Star (TV) - 01 (1080p)")
	if strings.EqualFold(got, "TV") {
		t.Fatalf("ExtractSource on (TV) format marker returned %q", got)
	}
}

func TestExtractSource_TVRipAndBracketTVStillMatch(t *testing.T) {
	if got := nyaa.ExtractSource("[Group] Anime 01 [TVRip]"); !strings.EqualFold(got, "TV") {
		t.Fatalf("TVRip → TV, got %q", got)
	}
	if got := nyaa.ExtractSource("[Group] Anime 01 [TV]"); !strings.EqualFold(got, "TV") {
		t.Fatalf("[TV] → TV, got %q", got)
	}
}

func TestExtractSource_SpecificBDPatternsStillWin(t *testing.T) {
	if got := nyaa.ExtractSource("[Group] Anime [BDRip] [1080p]"); !strings.EqualFold(got, "BDRip") {
		t.Fatalf("BDRip must win over bare BD, got %q", got)
	}
	if got := nyaa.ExtractSource("[Group] Anime [BDRemux] [1080p]"); !strings.EqualFold(got, "BDRemux") {
		t.Fatalf("BDRemux must win over bare BD, got %q", got)
	}
}
```

Extend `TestDefaultPriorities_TokensAreReachable` with a Sources loop **after** the existing Codecs/Resolutions loops:

```go
	for _, source := range d.Sources {
		got := strings.ToLower(nyaa.ExtractSource("[Group] Anime - 01 [" + source + "]"))
		if got != source {
			t.Errorf("default source %q is inert: ExtractSource returns %q", source, got)
		}
	}
```

- [ ] **Step 2: Run the new tests and watch them fail**

Run: `go test ./src/tests/unit/ -run 'TestExtractSource_|TestDefaultPriorities_TokensAreReachable' -count=1`

Expected:
- `TestExtractSource_BracketBD` fails: `ExtractSource` undefined **or**, after the rename-only, returns `""` because `[BD]` is not in the patterns.
- `TestExtractSource_BareTVInTitleIsNotASource` fails: current `\bTV\b` matches `(TV)` and returns `"TV"`.
- `TestDefaultPriorities_TokensAreReachable` fails on `"bd"` (and possibly `"tv"`): `[bd]` does not match today.

If the compiler error is `ExtractSource undefined`, that is the expected RED for the export. Do not implement the regex until the tests exist.

- [ ] **Step 3: Export and update the regex**

Rename `extractSource` to `ExtractSource` in `nyaa.go` and update every call site (`priorities.go` `criterionCompare["source"]`).

In `nyaa_regex.go`, replace `reSourcePatterns` with:

```go
	reSourcePatterns = []struct {
		re     *regexp.Regexp
		source string
	}{
		{regexp.MustCompile(`(?i)BDRemux`), "BDRemux"},
		{regexp.MustCompile(`(?i)BDRip|BD\s*Ray`), "BDRip"},
		{regexp.MustCompile(`(?i)Blu-ray|Blu\s*ray`), "BD"},
		{regexp.MustCompile(`(?i)\[BD\]|\bBD\b`), "BD"},
		{regexp.MustCompile(`(?i)WEB-DL`), "WEB-DL"},
		{regexp.MustCompile(`(?i)WEBRip`), "WEBRip"},
		{regexp.MustCompile(`(?i)HDTV`), "HDTV"},
		{regexp.MustCompile(`(?i)\bTV\s*Rip|\[TV\]`), "TV"},
		{regexp.MustCompile(`(?i)DVD`), "DVD"},
	}
```

The `[BD]`/`\bBD\b` line is **after** BDRemux/BDRip/Blu-ray so those still win. `\bBD\b` does not match `BDRip` (D and R are both word characters). Bare `\bTV\b` is gone; `TVRip` and `[TV]` stay so the default list token `tv` remains reachable.

- [ ] **Step 4: Re-run the tests**

Run: `go test ./src/tests/unit/ ./src/internal/nyaa/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/internal/nyaa/nyaa.go src/internal/nyaa/nyaa_regex.go src/internal/nyaa/priorities.go src/tests/unit/priorities_test.go
git commit -m "feat: match [BD] as source and ignore (TV) in titles"
```

---

### Task 3: Empty source is `web-dl`; finished-series sort; new default order

**Files:**
- Modify: `src/internal/nyaa/priorities.go` (`DefaultPriorities.CriteriaOrder`, `criterionCompare["source"]`, `episodeCriteria` stays without source, add `finishedSeriesCriteria` + `SortFinishedSeriesResults`)
- Modify: `src/internal/nyaa/nyaa.go` (`SortTorrentResults` comment — source stays out of the RELEASING sort; `SortMovieResults` comment currently lists a stale hardcoded order, rewrite it to "uses CriteriaOrder as-is")
- Modify: `src/tests/unit/priorities_test.go` (new tests below; extend `TestDefaultCriteriaOrder_HealthBeforeFansub`)

**Interfaces:**
- Consumes: `ExtractSource`, `sortByCriteria`, `filterCriteria`, `ActivePriorities().CriteriaOrder`
- Produces:
  - `func SortFinishedSeriesResults(results []TorrentResult) []TorrentResult`
  - `sourceCompare` used by `criterionCompare["source"]` (movies + finished series)
  - default `CriteriaOrder`: `uncensored, resolution, health, source, codec, fansub, audio, size`
  - `episodeCriteria` still omits `source` and `audio`

- [ ] **Step 1: Write the failing tests**

Add to `src/tests/unit/priorities_test.go`:

```go
func res(s string) *string { return &s }

func TestSourceCompare_UntaggedTiesWEBDL(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p)", Resolution: &r1080, Seeders: "100"},
		{Name: "[Erai-raws] Anime - 07 (1080p) WEB-DL", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortMovieResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("untagged must tie WEB-DL on source and let fansub decide, got %s", sorted[0].Name)
	}
}

func TestSourceCompare_UntaggedLosesToBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p)", Resolution: &r1080, Seeders: "100"},
		{Name: "[Group] Anime - 07 [BD] [1080p]", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortMovieResults(results)
	if !strings.Contains(sorted[0].Name, "[BD]") {
		t.Fatalf("untagged WEB must lose to [BD], got %s", sorted[0].Name)
	}
}

func TestSortTorrentResults_IgnoresSource(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p) WEB-DL", Resolution: &r1080, Seeders: "100"},
		{Name: "[Ember] Anime - 07 [BD] [1080p]", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("RELEASING sort must ignore source and let fansub decide, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_HealthyWEBBeatsFragileBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime [BD] [1080p]", Resolution: &r1080, Seeders: "8"},
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "80"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "WEB-DL") {
		t.Fatalf("health must beat source, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_SameBandPrefersBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "400"},
		{Name: "[Group] Anime [BD] [1080p]", Resolution: &r1080, Seeders: "60"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "[BD]") {
		t.Fatalf("same health band must prefer BD, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_ResolutionBeforeSource(t *testing.T) {
	r1080, r720 := "1080p", "720p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime [BD] [720p]", Resolution: &r720, Seeders: "200"},
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "200"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "WEB-DL") {
		t.Fatalf("1080p WEB must beat 720p BD, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_UncensoredFirst(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime (1080p) WEB-DL uncensored", Resolution: &r1080, Seeders: "8"},
		{Name: "[Group] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "80"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(strings.ToLower(sorted[0].Name), "uncensored") {
		t.Fatalf("uncensored must win even on a worse health band, got %s", sorted[0].Name)
	}
}

func TestDefaultCriteriaOrder_ResolutionHealthSource(t *testing.T) {
	order := nyaa.DefaultPriorities().CriteriaOrder
	want := []string{"uncensored", "resolution", "health", "source", "codec", "fansub", "audio", "size"}
	if len(order) != len(want) {
		t.Fatalf("CriteriaOrder %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("CriteriaOrder %v, want %v", order, want)
		}
	}
}
```

Check `isUncensored` so the last test's name actually matches. If the helper looks for `uncen` as a substring, `"uncensored"` in the name is enough. If it requires `[Uncensored]`, use that tag instead.

- [ ] **Step 2: Run the tests and watch them fail**

Run: `go test ./src/tests/unit/ -run 'TestSourceCompare_|TestSortTorrentResults_IgnoresSource|TestSortFinishedSeriesResults_|TestDefaultCriteriaOrder_ResolutionHealthSource' -count=1`

Expected:
- `SortFinishedSeriesResults` undefined
- `TestSourceCompare_UntaggedLosesToBD` fails: empty source is worst, so untagged already loses to BD today — **this one may PASS on current code**. If it passes, keep it; it locks the new comparator's BD side.
- `TestSourceCompare_UntaggedTiesWEBDL` fails: empty source is worst, so tagged WEB-DL wins over untagged SubsPlease.
- `TestDefaultCriteriaOrder_ResolutionHealthSource` fails: current order still has `source` before `resolution`.

- [ ] **Step 3: Implement**

In `priorities.go`:

1. Change `DefaultPriorities().CriteriaOrder` to:

```go
		CriteriaOrder: []string{"uncensored", "resolution", "health", "source", "codec", "fansub", "audio", "size"},
```

Update the comment above it: health before fansub stays; source now sits after health and after resolution so a healthy 1080p WEB beats a fragile or 720p BD.

2. Replace the `"source"` entry in `criterionCompare` with `sourceCompare`.

3. Add:

```go
// sourceCompare ranks by the sources list. An empty extract (weekly SubsPlease, most WEB
// packs) canonicalizes to web-dl so it ties a tagged WEB-DL and loses to BD — the codec-style
// "either side empty ⇒ tie" would make untagged WEB tie a tagged BD and throw the criterion
// away (decisions.md #106).
func sourceCompare(a, b TorrentResult) int {
	return sourcePriority(canonicalSource(ExtractSource(a.Name))) - sourcePriority(canonicalSource(ExtractSource(b.Name)))
}

func canonicalSource(source string) string {
	if source == "" {
		return "web-dl"
	}
	return source
}

var finishedSeriesCriteria = map[string]bool{
	"uncensored": true, "resolution": true, "health": true, "source": true,
	"codec": true, "fansub": true, "size": true,
}

// SortFinishedSeriesResults is the series sort when Media.Status is FINISHED: same
// criteria as SortTorrentResults plus source. The daemon calls this as a second full
// sort; ScrapNyaa* keep using SortTorrentResults (decisions.md #106).
func SortFinishedSeriesResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, filterCriteria(ActivePriorities().CriteriaOrder, finishedSeriesCriteria))
}
```

Keep `episodeCriteria` without `source`. Keep `audio` out of `finishedSeriesCriteria`.

4. In `nyaa.go`, rewrite the `SortTorrentResults` comment: source stays out because a weekly episode is WEB-DL in almost every release; finished series go through `SortFinishedSeriesResults`. Rewrite the `SortMovieResults` comment to: uses `CriteriaOrder` as-is (every criterion, including source and audio).

- [ ] **Step 4: Re-run the new tests plus the existing sort tests**

Run: `go test ./src/tests/unit/ ./src/internal/nyaa/ -count=1`

Expected: PASS. `TestSortMovieResults_SortsByTechnicalQuality` still wants a BD first among same-resolution no-seeders rows — still true because source still ranks BD above WEB-DL after the health/resolution tie.

If `TestDefaultCriteriaOrder_HealthBeforeFansub` still exists, it remains valid (`resolution < health < fansub`). Leave it; the new exact-slice test is the stricter lock.

- [ ] **Step 5: Commit**

```bash
git add src/internal/nyaa/priorities.go src/internal/nyaa/nyaa.go src/tests/unit/priorities_test.go
git commit -m "feat: rank finished series by source after health and resolution"
```

---

### Task 4: Daemon re-ranks after scrape

**Files:**
- Modify: `src/internal/daemon/search.go` (add `rankSeriesResults`)
- Modify: `src/internal/daemon/episodes.go` (two call sites: after `searchAnime` before `partitionSearchResults`; after `searchSingleEpisode` before `filterSearchResults`). Update the `partitionSearchResults` comment that says the relative order comes only from `SortTorrentResults`.
- Modify: `src/internal/daemon/debug.go` (same two call sites as production)
- Modify: `src/internal/daemon/manual_download.go` (`ManualDownloadEpisode` after `searchNyaaForSingleEpisode`, using `details.isFinished` which is already stored and unused)
- Create: `src/internal/daemon/rank_test.go`

**Interfaces:**
- Consumes: `nyaa.SortFinishedSeriesResults([]nyaa.TorrentResult) []nyaa.TorrentResult`
- Produces:

```go
func rankSeriesResults(results []nyaa.TorrentResult, finished bool) []nyaa.TorrentResult
```

Gate: `anime.Media.Status == anilist.MediaStatusFinished` (not `MediaListStatus`). Movies keep `searchMovie` → `SortMovieResults`; do not pass movies through this helper.

- [ ] **Step 1: Write the failing daemon tests**

Create `src/internal/daemon/rank_test.go`. Reuse `animeWithEpisodes`, `searcherFor`, `fakeMagnet`, `limitsConfig` from `limits_test.go` (same package `daemon`).

```go
package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func twoPacksWEBThenBD() []nyaa.TorrentResult {
	r1080 := "1080p"
	return []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime 01-12 (1080p) WEB-DL", MagnetLink: fakeMagnet(1), Resolution: &r1080, Seeders: "400"},
		{Name: "[Group] Anime 01-12 [BD] [1080p]", MagnetLink: fakeMagnet(2), Resolution: &r1080, Seeders: "60"},
	}
}

func TestProcessAnimeEpisodes_FinishedPrefersBDPack(t *testing.T) {
	anime := animeWithEpisodes(12, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor(twoPacksWEBThenBD(), nil, nil, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) == 0 {
		t.Fatal("expected a pack download")
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0002") {
		t.Fatalf("FINISHED must pick the BD pack, hash %s", result.newEpisodes[0].EpisodeHash)
	}
}

func TestProcessAnimeEpisodes_ReleasingKeepsScrapeOrder(t *testing.T) {
	anime := animeWithEpisodes(12, anilist.MediaStatusReleasing, true, "")
	searcher := searcherFor(twoPacksWEBThenBD(), nil, nil, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) == 0 {
		t.Fatal("expected a pack download")
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0001") {
		t.Fatalf("RELEASING must keep scrape order (WEB first), hash %s", result.newEpisodes[0].EpisodeHash)
	}
}

func TestRankSeriesResults_SingleEpisodeFallback(t *testing.T) {
	r1080 := "1080p"
	ep := 1
	singles := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 01 (1080p) WEB-DL", MagnetLink: fakeMagnet(1), Resolution: &r1080, Seeders: "400", Episode: &ep},
		{Name: "[Group] Anime - 01 [BD] [1080p]", MagnetLink: fakeMagnet(2), Resolution: &r1080, Seeders: "60", Episode: &ep},
	}
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor(nil, nil, singles, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) != 1 {
		t.Fatalf("want 1 episode, got %d", len(result.newEpisodes))
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0002") {
		t.Fatalf("FINISHED single fallback must pick BD, hash %s", result.newEpisodes[0].EpisodeHash)
	}
}
```

`fakeMagnet(n)` is `magnet:?xt=urn:btih:` + 36 zeros + `%04x` of n, so n=1 → hash ending `0001`, n=2 → `0002`. `parseInfoHash` returns that 40-char hex. `searcherFor` marks the first slice as `IsBatch: true`. Pack names carry `01-12` so `pickBatches` covers the window. `limitsConfig()` leaves `MinSeeders` at 0 (filter off).

- [ ] **Step 2: Run and watch FAIL**

Run: `go test ./src/internal/daemon/ -run 'TestProcessAnimeEpisodes_FinishedPrefersBDPack|TestProcessAnimeEpisodes_ReleasingKeepsScrapeOrder|TestRankSeriesResults_SingleEpisodeFallback' -count=1`

Expected: `FinishedPrefersBDPack` and `SingleEpisodeFallback` FAIL — both pick WEB (`0001`) because nothing re-ranks. `ReleasingKeepsScrapeOrder` may already PASS (scrape order is WEB first); keep it as the lock.

- [ ] **Step 3: Implement the helper and wire the three files**

In `search.go`:

```go
func rankSeriesResults(results []nyaa.TorrentResult, finished bool) []nyaa.TorrentResult {
	if finished {
		return nyaa.SortFinishedSeriesResults(results)
	}
	return results
}
```

In `episodes.go` `processAnimeEpisodes`:

```go
finished := anime.Media.Status == anilist.MediaStatusFinished
// ...
packs, singles, packStats := partitionSearchResults(configs, anime, rankSeriesResults(searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload)), finished))
```

and the single-episode fallback:

```go
candidates, searchStats = filterSearchResults(rankSeriesResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, seriesLength), finished), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

Update the `partitionSearchResults` comment: the relative order is the ranked list the caller passed (`SortTorrentResults` inside the scrape, then `rankSeriesResults` when FINISHED).

In `debug.go`, wrap the same two searcher calls with `rankSeriesResults(..., details.mediaList.Media.Status == anilist.MediaStatusFinished)`. `details.isFinished` is already on the struct — use that.

In `manual_download.go` `ManualDownloadEpisode`, after `searchNyaaForSingleEpisode`:

```go
results := rankSeriesResults(searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, nil, anilist.MediaRelations{}, anilist.LastAiredEpisode(details.mediaList)), details.isFinished)
```

Do not touch `ManualDownloadEpisodeWithMagnet` (user-supplied magnet, no ranking). Do not touch `searchMovie` / `resolveMovie`.

- [ ] **Step 4: Re-run daemon tests**

Run: `go test ./src/internal/daemon/ -count=1`

Expected: PASS, including the three new tests.

- [ ] **Step 5: Commit**

```bash
git add src/internal/daemon/search.go src/internal/daemon/episodes.go src/internal/daemon/debug.go src/internal/daemon/manual_download.go src/internal/daemon/rank_test.go
git commit -m "feat: re-rank finished series torrents after nyaa scrape"
```

---

### Task 5: Priorities UI copy

**Files:**
- Modify: `src/internal/frontend/src/routes/Priorities.svelte`
- Modify: `src/internal/frontend/messages/en.json`
- Modify: `src/internal/frontend/messages/pt-BR.json`

**Interfaces:**
- Consumes: existing `moviesOnly` hint on a list heading
- Produces: `sources` uses a new scope string; `audio` keeps "movies only"; criteria-order note says audio is movies-only and source applies to movies and FINISHED series

- [ ] **Step 1: Update i18n source files**

`en.json`:

- Keep `priorities_scope_movies_only`: `"movies only"`
- Add `priorities_scope_movies_and_finished`: `"movies and finished series"`
- Replace `priorities_note_criteria_order` with: `"Audio applies only to movies. Source applies to movies and to series whose AniList media status is FINISHED."`

`pt-BR.json`:

- Keep `priorities_scope_movies_only`: `"só filmes"`
- Add `priorities_scope_movies_and_finished`: `"filmes e séries encerradas"`
- Replace `priorities_note_criteria_order` with: `"Audio vale só para filmes. Source vale para filmes e para séries cujo status de mídia no AniList é FINISHED."`

- [ ] **Step 2: Point the sources list at the new scope**

In `Priorities.svelte`, change the list descriptor from `moviesOnly?: boolean` to a scope message:

```ts
  const LISTS: { key: keyof Priorities; label: () => string; scope?: () => string }[] = [
    { key: "criteria_order", label: m.priorities_list_criteria_order },
    { key: "fansubs", label: m.priorities_list_fansubs },
    { key: "resolutions", label: m.priorities_list_resolutions },
    { key: "sources", label: m.priorities_list_sources, scope: m.priorities_scope_movies_and_finished },
    { key: "codecs", label: m.priorities_list_codecs },
    { key: "audio", label: m.priorities_list_audio, scope: m.priorities_scope_movies_only },
    { key: "ignore_list", label: m.priorities_list_ignore_list },
  ];
```

In the `$:` `T` block, drop `moviesOnly: m.priorities_scope_movies_only()` (each list now carries its own `scope`). Map `scope: l.scope && l.scope()` into `T.lists`.

In the heading, replace `{#if moviesOnly}<span ...>({T.moviesOnly})</span>{/if}` with `{#if scope}<span class="normal-case tracking-normal font-normal text-subtle">({scope})</span>{/if}`.

The `#each` becomes `{#each T.lists as { key, label, scope } (key)}`.

Update the comment above `LISTS`: source applies to movies and FINISHED series; audio stays movies-only.

- [ ] **Step 3: Compile i18n if the generated messages module is needed locally**

From `src/internal/frontend`: `bun run i18n:compile`. The generated `src/lib/i18n/` is gitignored; compiling is for the local typecheck / later browser check, not a commit.

- [ ] **Step 4: Commit**

```bash
git add src/internal/frontend/src/routes/Priorities.svelte src/internal/frontend/messages/en.json src/internal/frontend/messages/pt-BR.json
git commit -m "feat: label source priority for movies and finished series"
```

Browser verification of this copy is done in Task 7 (or by the controller after Task 5 if a frontend dev server is already up). Do not start the daemon from this task.

---

### Task 6: Docs

**Files:**
- Modify: `docs/agents/config.md` (`Priorities.CriteriaOrder` default and episode-subset text; `Priorities.Sources` no longer "movie sort only"; `MinSeeders` row gets a one-line pointer that health **bands** are a code constant `{10, 50}`, not a config field)
- Modify: `docs/agents/architecture.md` sorting paragraph: source is in the finished-series subset; health floors `10/50`; pointer to #106
- Modify: `docs/agents/decisions.md` — Index line for #106; new entry; revise #69
- Modify: `docs/guides/webui.md` Priorities blurb
- Modify: `docs/superpowers/specs/2026-10-24-source-ranking-finished-series-design.md` status line: implemented
- Modify: `docs/TODO.md` only if the "No release — source ranking" bullet should mark the spec as implemented (keep the section; change "Spec designed" to "implemented")

**Interfaces:** none.

- [ ] **Step 1: `config.md`**

`Priorities.CriteriaOrder` default becomes `["uncensored","resolution","health","source","codec","fansub","audio","size"]`. Description: `SortTorrentResults` uses the episode subset (`uncensored`, `resolution`, `health`, `codec`, `fansub`, `size` — no `source`, no `audio`). `SortFinishedSeriesResults` is that subset plus `source`, used when `Media.Status == FINISHED`. `SortMovieResults` uses all. An existing `config.json` keeps the order it already has; restore-defaults (or a hand edit) is how an old install picks up the new relative order of source vs health vs resolution. Health **bands** do not wait on that click.

`Priorities.Sources`: drop "movie sort only". It ranks movies always and series when `Media.Status == FINISHED`. Empty extract canonicalizes to `web-dl`.

`MinSeeders` row: after the existing text, one sentence: ranking among survivors uses `healthTier` bands with floors 10 and 50 seeders (not a config field; see decisions.md #106).

- [ ] **Step 2: `architecture.md`**

Replace the Sorting paragraph's floors `1/5/20/100/400` with `10/50`. Add: `SortTorrentResults` omits `source`; `SortFinishedSeriesResults` includes it and the daemon applies that sort only when `Media.Status == FINISHED` (`rankSeriesResults` in `daemon/search.go`). Pointer to #106.

- [ ] **Step 3: `decisions.md`**

Index, after #104 (this file on master has no #105):

```
- [**#106** — source ranks finished series, empty source is web-dl, health bands are 10/50](#106-source-ranks-finished-series-empty-source-is-web-dl-health-bands-are-1050)
```

Revise #69:

- Replace "Only `source` stays out." with: source stays out of `episodeCriteria` (the RELEASING / weekly sort). It enters the series sort only through `SortFinishedSeriesResults` when `Media.Status == FINISHED` (decision #106).
- Replace "Don't put `source` in `episodeCriteria` too" with: don't put `source` in `episodeCriteria` for weekly airing; finished series use the dedicated sort.

New entry at the end of the file, same shape as the others (Location / What it looks like / Why it's right / Don't "fix" by):

```
### 106. source ranks finished series, empty source is web-dl, health bands are 10/50

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
```

- [ ] **Step 4: `webui.md`**

Priorities blurb: sources rank movies and series whose AniList media status is FINISHED; audio stays movies-only. Restore-defaults is how an existing install picks up the new `criteria_order`.

- [ ] **Step 5: Spec + TODO status**

Spec header `Status:` → `implemented`. TODO section "No release — source ranking for finished series": keep it (no release number yet); change "Spec designed on 2026-10-24" to "Implemented; ships as a patch whenever it is tagged."

- [ ] **Step 6: Commit**

```bash
git add docs/agents/config.md docs/agents/architecture.md docs/agents/decisions.md docs/guides/webui.md docs/superpowers/specs/2026-10-24-source-ranking-finished-series-design.md docs/TODO.md
git commit -m "docs: record source ranking for finished series"
```

---

## Self-review (spec coverage)

| Spec requirement | Task |
|---|---|
| Gate `Media.Status == FINISHED` | 4 |
| Helper `rankSeriesResults` / `SortFinishedSeriesResults`; `ScrapNyaa*` unchanged | 3, 4 |
| Call sites: `processAnimeEpisodes`, `RunAnimeDebug`, `ManualDownloadEpisode` (`isFinished`) | 4 |
| Default order `uncensored, resolution, health, source, codec, fansub, audio, size` | 3 |
| Health floors `{10, 50}` | 1 |
| Empty source = `web-dl` | 3 |
| Extractor `[BD]` / `\bBD\b`; drop bare `\bTV\b`; keep TVRip/`[TV]` | 2 |
| No `criteria_order` migration | 6 (documented); 3 (code does not migrate) |
| UI: sources not movies-only; note about audio vs source | 5 |
| Tests listed in spec | 1–4 |
| Docs: config, architecture, #106, #69 revision, webui | 6 |
| Out of scope: audio on episodes, auto-redownload, picker, min_seeders, RELEASING source | honored |

No placeholders. Signatures in later tasks match Task 3 (`SortFinishedSeriesResults`) and Task 4 (`rankSeriesResults(results, finished bool)`).
