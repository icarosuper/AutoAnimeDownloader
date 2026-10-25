# Standalone torrent picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When adding a standalone anime or replacing an anime's torrent, show the ranked whole-title candidates (Nyaa description on demand, optional custom query) and let the user confirm or swap before anything is saved.

**Architecture:** The daemon already searches, filters (`ShouldIgnore`, `min_seeders`, size ceilings, packs-only or movie) and ranks (Priorities). This work adds a **preview** of that list (cap 8, no add) and an add-with-magnet path that records coverage the way the pass does. The web UI is a full-page picker; the CLI lists candidates and can add with `--magnet`. Description HTML is fetched only for the selected row.

**Tech Stack:** Go 1.25, existing `nyaa` + `daemon` + `api` packages, urfave/cli, Svelte 5 + svelte-spa-router + Vitest, Paraglide i18n (`en.json` / `pt-BR.json`).

## Global Constraints

- Spec: [2026-10-23-standalone-torrent-picker-design.md](../specs/2026-10-23-standalone-torrent-picker-design.md). Out of scope: per-episode picker, Nyaa browser, changing `POST /animes/{id}/replace` coverage, `AAD_LIVE_NYAA`.
- Repo language is English (code, comments, logs, docs). Portuguese only in `pt-BR.json`. If you touch `nyaa.go`, translate the leftover Portuguese comments at lines 109–118 (`Prioridade 2` / `Prioridade 4`).
- Error wrapping: `fmt.Errorf("failed to <action>: %w", err)`. Handlers are closures over `*Server`. Responses go through `JSONSuccess` / `JSONError` / `JSONInternalError` / `JSONDownloadError`.
- Import order: internal, stdlib, third-party.
- No new Go module unless a task says so. Sanitize HTML with `goquery` (already a dep).
- Nyaa is mocked (`nyaa.MockNyaaHttpGet`). AniList is mocked (`anilist.MockAniListDo`).
- CLI ships in the same change as the endpoints ([conventions.md](../../agents/conventions.md) checklist).
- Do **not** `git commit` or `git push`. CLAUDE.md: the user commits. Each "Commit" step is the proposed subject only.
- Do **not** start the per-episode picker. That is a later spec.
- After any Go change: `go test ./...`. After frontend change: `cd src/internal/frontend && bun run test:component` (and `test:unit` if you touched `lib/`).
- Ranking is **global** `nyaa.ActivePriorities()` from config. `files.AnimeSettings` only has `Progress`; there is no per-anime priority override. The spec's "per-anime settings feed the ranking" means "same ranking the pass uses", which is global.
- `POST /standalone-animes` body grows optional fields the spec example omitted, because coverage and the disk guard cannot be derived from a magnet URI:

```json
{"media_id": 21, "magnet": "magnet:?xt=…", "name": "[SubsPlease] Frieren (01-28) [1080p]", "nyaa_id": 1664122, "size_bytes": 9019431321}
```

  `magnet` optional (absent = today's auto-pick). `name` feeds `ExtractBatchInfo`. `nyaa_id` feeds one `PackFileRange` when the name has no range. `size_bytes` feeds `checkDiskSpace`. CLI `--magnet` without name/nyaa_id is the unknown-range path (`added: 0`, torrent in session).
- Candidates handler must **not** fetch `/view/<id>`. `maxPackDetailFetches = 3` stays a pass concern.

---

## File map

**Create**

- `src/internal/nyaa/detail.go` — `FetchTorrentDetail`, HTML sanitizer, file-list flatten, `ParseNyaaID`
- `src/internal/daemon/preview.go` — `PreviewWholeTitleTorrents` (search + partition + sort, cap 8, no add)
- `src/internal/daemon/preview_test.go`
- `src/internal/api/endpoint_torrent_candidates.go` — GET candidates + GET detail
- `src/internal/api/endpoint_torrent_candidates_test.go`
- `src/internal/frontend/src/routes/TorrentPicker.svelte`
- `src/internal/frontend/src/lib/stores/pickerCache.ts`
- `src/internal/frontend/tests/component/TorrentPicker.test.ts`

**Modify**

- `src/internal/nyaa/nyaa.go` — `DetailURL` on movie rows; Portuguese comments
- `src/tests/unit/nyaa_test.go` — movie `DetailURL`; detail scrape / sanitizer
- `src/internal/daemon/standalone.go` — `DownloadStandaloneAnimeWithMagnet` (add-then-id, coverage, rollback)
- `src/internal/daemon/standalone_test.go`
- `src/internal/api/endpoint_standalone_animes.go` — optional magnet / name / nyaa_id / size_bytes
- `src/internal/api/endpoint_standalone_animes_test.go`
- `src/internal/api/server.go` — two new routes
- `src/internal/api/client.go` + `client_test.go`
- `src/cmd/cli/animes.go` + `animes_test.go`
- `src/internal/frontend/src/lib/api/client.ts`
- `src/internal/frontend/src/App.svelte` — routes (longer paths first)
- `src/internal/frontend/src/routes/AddAnime.svelte`
- `src/internal/frontend/src/routes/AnimeDetail.svelte` — whole-title replace goes to the picker; per-episode magnet modal stays
- `src/internal/frontend/messages/en.json`, `pt-BR.json`
- `src/internal/frontend/tests/component/AddAnime.test.ts`
- `docs/agents/architecture.md`, `docs/guides/cli.md`, `docs/guides/webui.md`, `docs/agents/decisions.md` (entry **#105**), `docs/TODO.md` (leave the episode-picker bullet)

---

### Task 1: Movie `DetailURL` + Nyaa view-page scrape

**Files:**
- Modify: `src/internal/nyaa/nyaa.go` (movie `parseRow` around the `TorrentResult{...}` literal that currently omits `DetailURL`; Portuguese comments at lines 109–118)
- Create: `src/internal/nyaa/detail.go`
- Test: `src/tests/unit/nyaa_test.go`

**Interfaces:**
- Consumes: `fetchNyaaPage`, `getNyaaBaseURL`, `fileListEntryName`, `parseSize`, `detailURL`
- Produces:

```go
type TorrentFile struct {
    Name      string `json:"name"`
    SizeBytes int64  `json:"size_bytes"`
}

type TorrentDetail struct {
    DescriptionHTML string        `json:"description_html"`
    Files           []TorrentFile `json:"files"`
    NyaaURL         string        `json:"nyaa_url"`
}

func ParseNyaaID(detailURL string) int
func FetchTorrentDetail(nyaaID int) (TorrentDetail, error)
```

`FetchTorrentDetail` builds `getNyaaBaseURL()+"/view/"+strconv.Itoa(nyaaID)` itself. It never takes a URL from the caller. Errors (message-driven, no new sentinels unless you already need `errors.Is` at the API layer — then two is fine, matching `ErrInsufficientDiskSpace`):

- `nyaaID <= 0` → `fmt.Errorf("invalid nyaa id %d", nyaaID)`
- HTTP 404 → `fmt.Errorf("torrent %d not found: %w", nyaaID, ErrTorrentNotFound)` with `var ErrTorrentNotFound = errors.New("torrent not found")`
- network / non-200 other than 404 → `fmt.Errorf("failed to fetch nyaa view %d: %w", nyaaID, ErrNyaaUnavailable)` with `var ErrNyaaUnavailable = errors.New("nyaa unavailable")`

- [ ] **Step 1: Write the failing tests** in `src/tests/unit/nyaa_test.go`

```go
func TestScrapNyaaForMovie_CapturesTheDetailURL(t *testing.T) {
	restore := mockHttpGet(mockHtml([]string{"[SubsPlease] Suzume no Tojimari (2022) [1080p]"}))
	defer restore()
	results, err := nyaa.ScrapNyaaForMovie("Suzume no Tojimari", true)
	if err != nil {
		t.Fatalf("ScrapNyaaForMovie: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want at least one movie row")
	}
	if want := "https://nyaa.si/view/1"; results[0].DetailURL != want {
		t.Errorf("DetailURL: want %q, got %q", want, results[0].DetailURL)
	}
}

func TestParseNyaaID(t *testing.T) {
	if got := nyaa.ParseNyaaID("https://nyaa.si/view/1664122"); got != 1664122 {
		t.Errorf("got %d", got)
	}
	if got := nyaa.ParseNyaaID(""); got != 0 {
		t.Errorf("empty: got %d", got)
	}
}

func TestFetchTorrentDetail_SanitisesHTMLAndListsFiles(t *testing.T) {
	html := `<!doctype html><html><body>
<div id="torrent-description">
  <p>Safe <strong>text</strong></p>
  <script>alert(1)</script>
  <iframe src="https://evil.example"></iframe>
  <a href="https://nyaa.si/view/1" onclick="alert(1)">link</a>
  <img src="/img/shot.png">
</div>
<div class="torrent-file-list panel-body">
  <ul>
    <li><a class="folder">Frieren</a>
      <ul>
        <li>Frieren - 01.mkv<span class="file-size">(312.1 MiB)</span></li>
        <li>Frieren - 01.srt<span class="file-size">(40.0 KiB)</span></li>
      </ul>
    </li>
  </ul>
</div>
</body></html>`
	defer nyaa.MockNyaaHttpGet(func(u string) (*http.Response, error) {
		if !strings.HasSuffix(u, "/view/1664122") {
			t.Errorf("unexpected url %s", u)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})()

	d, err := nyaa.FetchTorrentDetail(1664122)
	if err != nil {
		t.Fatalf("FetchTorrentDetail: %v", err)
	}
	if strings.Contains(d.DescriptionHTML, "<script") || strings.Contains(d.DescriptionHTML, "iframe") || strings.Contains(d.DescriptionHTML, "onclick") {
		t.Errorf("unsanitised HTML: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, "<p>") || !strings.Contains(d.DescriptionHTML, "Safe") {
		t.Errorf("lost formatting: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, `src="https://nyaa.si/img/shot.png"`) {
		t.Errorf("relative img src not resolved: %s", d.DescriptionHTML)
	}
	if d.NyaaURL != "https://nyaa.si/view/1664122" {
		t.Errorf("NyaaURL = %q", d.NyaaURL)
	}
	if len(d.Files) != 2 {
		t.Fatalf("files = %#v, want 2 (video and subtitle; folders are not rows)", d.Files)
	}
	if d.Files[0].Name != "Frieren/Frieren - 01.mkv" {
		t.Errorf("nested name = %q", d.Files[0].Name)
	}
}

func TestFetchTorrentDetail_Nyaa404(t *testing.T) {
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})()
	_, err := nyaa.FetchTorrentDetail(1)
	if err == nil || !errors.Is(err, nyaa.ErrTorrentNotFound) {
		t.Fatalf("want ErrTorrentNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/tests/unit/ -run 'TestScrapNyaaForMovie_CapturesTheDetailURL|TestParseNyaaID|TestFetchTorrentDetail' -count=1`

Expected: FAIL (`FetchTorrentDetail` undefined, movie `DetailURL` empty).

- [ ] **Step 3: Implement**

Movie `parseRow` in `ScrapNyaaForMovie`: add `DetailURL: detailURL(cells.Eq(1).Find("a").Not(".comments").AttrOr("href", ""))` to the `TorrentResult` literal, same as `ScrapNyaaForAnime`.

`detail.go`:

- `ParseNyaaID`: parse the last path segment after `/view/`. Non-digits → 0.
- `FetchTorrentDetail`: `fetchNyaaPage` the constructed URL. `#torrent-description` inner HTML through a sanitizer. File list: walk `.torrent-file-list li` the way `walkFileList` in `live_pack_measure_test.go` walks folders, but emit **every** file (video and other), never folder rows. Nested names keep `parent/child`. Size from `span.file-size` via existing `parseSize` after trimming `()`.
- Sanitizer (goquery, no new dep): allow `p`, `h1`–`h6`, `ul`, `ol`, `li`, `pre`, `code`, `blockquote`, `br`, `em`, `strong`, `b`, `i`, `a`, `img`. On `a`, keep only `href` if it is `http`/`https`/`mailto`. On `img`, keep only `src` if it is `http`/`https`; resolve relative against `getNyaaBaseURL()`. Strip `script`, `iframe`, `style`, and every `on*` attribute. Unwrap unknown tags (keep children).

Translate `nyaa.go` lines 109–118 to English (`Priority 2: original romaji` / `Priority 4: original English`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/tests/unit/ -run 'TestScrapNyaaForMovie_CapturesTheDetailURL|TestParseNyaaID|TestFetchTorrentDetail|TestPackFileRange|TestScrapNyaaForAnime_CapturesTheDetailURL' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: scrape nyaa view pages and fill movie detail urls`

---

### Task 2: Daemon preview of ranked whole-title torrents

**Files:**
- Create: `src/internal/daemon/preview.go`, `src/internal/daemon/preview_test.go`
- Reuse: `searchNyaaForAnime`, `searchNyaaForMovie`, `partitionSearchResults`, `filterSearchResults`, `ExtractBatchInfo`, `ParseNyaaID`, `isAnimeMovie`, `GetMediaByID`

**Interfaces:**
- Consumes: AniList media (format, titles, synonyms), `files.Config` (`MinSeeders`, `MaxBatchTorrentSizeGB`, `MaxEpisodeTorrentSizeGB`), existing search helpers
- Produces:

```go
type TorrentCandidate struct {
    Name       string
    Magnet     string
    SizeBytes  int64
    Seeders    int
    Leechers   int
    Fansub     string
    Resolution string
    IsBatch    bool
    RangeStart int // ExtractBatchInfo from the NAME only. 0 = unknown. Do not fetch /view.
    RangeEnd   int
    NyaaID     int
}

const maxTorrentCandidates = 8

func PreviewWholeTitleTorrents(configs *files.Config, mediaID int, query string) (usedQuery string, items []TorrentCandidate, err error)
```

`query == ""`: title-variant loop (existing `searchNyaaWithVariants` / `searchNyaaForAnime` or `searchNyaaForMovie`). `usedQuery` is the variant that produced the list (the one `searchNyaaWithVariants` actually searched — thread it out; if that is too invasive, return the first generated variant that the search used, matching what the field should prefill).

`query != ""`: one Nyaa search with that string as `q`. Skip title variants and pass `season=nil, part=nil` into `ScrapNyaaForAnime`. Movie: `ScrapNyaaForMovie(query, true)`. Still apply `ShouldIgnore` (inside scrape), `titleMatchesQuery` against that `q` (inside scrape), then `filterSearchResults` (size + seeders), then packs-only via `partitionSearchResults` for series (discard singles). Rank is already `SortTorrentResults` / `SortMovieResults` from the scrape.

Cap at 8 after filter+partition. Empty list is `(usedQuery, nil, nil)`, not an error. AniList unknown id: same error `DownloadStandaloneAnime` uses (`anime %d does not exist on AniList`). Nyaa transport error: wrap `nyaa.ErrNyaaUnavailable` so the handler can 502.

Do **not** call `PackFileRange` / `FetchTorrentDetail` here.

- [ ] **Step 1: Write the failing tests** in `src/internal/daemon/preview_test.go`

Reuse `mockAniListRouter` / media JSON from `standalone_test.go` (TV with 24 episodes, and a MOVIE). Mock Nyaa with listing HTML of several packs plus a single, ranked so the best pack is first.

Cases:

1. Series, daemon query: `items` are packs only, len ≤ 8, index 0 is the same torrent `SortTorrentResults` would put first among packs that pass seeders/size. `RangeStart`/`RangeEnd` come from the name (`01-28` → 1, 28). `NyaaID` parsed from `DetailURL`.
2. Empty listing → `items` empty, `err == nil`.
3. `query = "[judas] frieren"`: captured Nyaa URL contains that `q`; season/part are not added by the daemon (the scrape gets `nil, nil`).
4. Movie media: movie rows, `NyaaID` set (depends on Task 1).
5. Unknown media id → error, no panic.

Do not assert against `pickBatches` coverage (that fetches detail pages). Assert against the filtered+sorted pack list, which is what the picker shows.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run Preview -count=1`

Expected: FAIL (`PreviewWholeTitleTorrents` undefined).

- [ ] **Step 3: Implement `preview.go`**

Sketch:

```go
func PreviewWholeTitleTorrents(configs *files.Config, mediaID int, query string) (string, []TorrentCandidate, error) {
    anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
    // same ErrFromCache handling as DownloadStandaloneAnime
    if anime == nil {
        return "", nil, fmt.Errorf("anime %d does not exist on AniList", mediaID)
    }
    var results []nyaa.TorrentResult
    used := strings.TrimSpace(query)
    if isAnimeMovie(*anime) {
        if used == "" {
            results = searchNyaaForMovie(anime.Media.Title, true)
            used = firstTitleVariant(anime.Media.Title)
        } else {
            var err error
            results, err = nyaa.ScrapNyaaForMovie(used, true)
            if err != nil { return used, nil, fmt.Errorf("failed to search nyaa: %w", err) }
        }
        results, _ = filterSearchResults(results, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
    } else {
        if used == "" {
            results = searchNyaaForAnime(anime.Media.Title, anime.Media.Synonyms, nil)
            used = firstTitleVariant(anime.Media.Title)
        } else {
            var err error
            results, err = nyaa.ScrapNyaaForAnime(used, nil, nil, nil)
            if err != nil { return used, nil, fmt.Errorf("failed to search nyaa: %w", err) }
        }
        packs, _, _ := partitionSearchResults(configs, *anime, results)
        results = packs
    }
    if len(results) > maxTorrentCandidates {
        results = results[:maxTorrentCandidates]
    }
    return used, toCandidates(results), nil
}
```

`toCandidates` maps `MagnetLink` → `Magnet`, `ParseSeeders(Seeders)` → `Seeders`, `ExtractBatchInfo(Name)` → range, `ParseNyaaID(DetailURL)` → `NyaaID`, dereference `Resolution`.

`firstTitleVariant` can call `buildTitleVariants` if it is exported; if it is unexported, either export a tiny helper or duplicate the first element of `nyaa.GenerateSearchTitleVariants`. Prefer reusing `buildTitleVariants` / `GenerateSearchTitleVariants` — do not invent a third variant list.

If `searchNyaaWithVariants` swallows Nyaa errors (it currently `continue`s), a total failure currently looks like an empty list. For the **preview**, a transport error on the first (or only) attempt must surface as `ErrNyaaUnavailable` so the picker can show "Nyaa down". When `query != ""` you already get the error from `ScrapNyaaFor*`. When `query == ""`, if every variant errors, return `ErrNyaaUnavailable`; if they all return empty, return empty items.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run Preview -count=1` then `go test ./src/internal/daemon/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: preview ranked whole-title torrents without adding them`

---

### Task 3: Add-with-magnet, coverage, rollback

**Files:**
- Modify: `src/internal/daemon/standalone.go`
- Test: `src/internal/daemon/standalone_test.go`

**Interfaces:**
- Consumes: `addAndPrioritize`, `ExtractBatchInfo`, `PackFileRange`, `EpisodeList`, `AddStandaloneAnime`, `RemoveStandaloneAnime`, `saveEpisodesToFile` / `UpsertEpisodes`
- Produces:

```go
func DownloadStandaloneAnimeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, configs *files.Config, mediaID int, magnet, name string, nyaaID int, sizeBytes int64) (int, error)
```

Order (this is the decision the handler must not invert):

1. `backend.Ensure` + `addAndPrioritize(magnet, sizeBytes)` — if this fails, **do not** write the standalone id.
2. `fm.AddStandaloneAnime(mediaID)` — if this fails, leave the torrent (same as a pass that added then failed to save); return the error.
3. Resolve coverage and `saveEpisodesToFile` / upsert the new records. If this fails, `fm.RemoveStandaloneAnime(mediaID)` then return the error. Return `len(records)` (0 is valid).

Coverage:

- Movie (`isAnimeMovie`): write the movie record(s) the way `resolveMovie` does — every aired node, or a synthetic episode 1 if none. `IsBatch` false. Skip name/file-list range.
- Series: `info := nyaa.ExtractBatchInfo(name)`. If `info.EndEpisode == 0` and `nyaaID > 0`, `PackFileRange(getNyaaBaseURL()+"/view/"+id)` via `nyaa.PackFileRange` on the absolute URL (`ParseNyaaID` round-trip: build the URL the same way `detailURL` does). If still unknown: **write no episode records** (decisions.md #84). If known: for each aired `EpisodeList` node with `TimeUntilAiring <= 0` and `Episode` inside `[Start, End]`, write a record with that hash, `IsBatch: true`, `BatchStart`/`BatchEnd` from `info`. Do not invent `1..total`.

Do not call `handleSavedEpisodes`. Do not mark uncovered aired episodes.

`DownloadStandaloneAnime` (no magnet) stays as it is.

- [ ] **Step 1: Write the failing tests**

```go
func TestDownloadStandaloneAnimeWithMagnet_WritesOnlyCoveredEpisodes(t *testing.T) {
    // media 24 episodes, all aired. name "(01-12)". magnet added.
    // want 12 saved episodes, standalone id present, backend.List() has the torrent.
}

func TestDownloadStandaloneAnimeWithMagnet_UnknownRangeWritesNoEpisodes(t *testing.T) {
    // name with no range, nyaaID 0 (so no PackFileRange). want 0 episodes, id present, torrent present.
}

func TestDownloadStandaloneAnimeWithMagnet_AddFailureDoesNotWriteID(t *testing.T) {
    backend := torrents.NewFakeBackend()
    backend.AddErr = fmt.Errorf("refused")
    // want standalone id absent, no episodes.
}
```

Use the existing AniList fixtures in `standalone_test.go`. FakeBackend for the session.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run DownloadStandaloneAnimeWithMagnet -count=1`

Expected: FAIL (function undefined).

- [ ] **Step 3: Implement**

Keep `DownloadStandaloneAnime` untouched. New function in `standalone.go`. Build episode structs the same shape `ManualDownloadAnimeWithMagnet` uses (`ManuallyManaged: true`, `DownloadDate: time.Now()`, shared hash), except the coverage filter and `IsBatch`/`BatchStart`/`BatchEnd`.

For `PackFileRange` you need a detail URL. Build `nyaa` URL with the same `getNyaaBaseURL()` the package uses — `PackFileRange` already lives in `nyaa`, so pass `""` when `nyaaID == 0`, otherwise you need an exported builder. Add `func ViewURL(nyaaID int) string` next to `ParseNyaaID` if `getNyaaBaseURL` is unexported (it is). Do not duplicate the env override.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run 'DownloadStandaloneAnime' -count=1`

Expected: PASS, including existing no-magnet tests.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: add standalone magnet with pack coverage and rollback`

---

### Task 4: API candidates + detail + POST magnet

**Files:**
- Create: `src/internal/api/endpoint_torrent_candidates.go`, `src/internal/api/endpoint_torrent_candidates_test.go`
- Modify: `src/internal/api/server.go` (register both GET routes near the other `/animes/{id}/…` lines)
- Modify: `src/internal/api/endpoint_standalone_animes.go` (`StandaloneAnimeRequest` + magnet branch)
- Modify: `src/internal/api/endpoint_standalone_animes_test.go`

**Interfaces:**
- Consumes: `daemon.PreviewWholeTitleTorrents`, `nyaa.FetchTorrentDetail`, `daemon.DownloadStandaloneAnimeWithMagnet`
- Produces: HTTP as specified

Routes (Go 1.22 patterns; more specific is a different pattern, order does not matter):

```
GET  /api/v1/animes/{id}/torrent-candidates
GET  /api/v1/animes/{id}/torrent-candidates/{nyaaId}
POST /api/v1/standalone-animes   (existing, body extended)
```

JSON types in `api` (what `client.go` and the frontend share conceptually):

```go
type TorrentCandidate struct {
    Name       string `json:"name"`
    Magnet     string `json:"magnet"`
    SizeBytes  int64  `json:"size_bytes"`
    Seeders    int    `json:"seeders"`
    Leechers   int    `json:"leechers"`
    Fansub     string `json:"fansub"`
    Resolution string `json:"resolution"`
    IsBatch    bool   `json:"is_batch"`
    RangeStart int    `json:"range_start"`
    RangeEnd   int    `json:"range_end"`
    NyaaID     int    `json:"nyaa_id"`
}

type TorrentCandidatesResponse struct {
    Query string              `json:"query"`
    Items []TorrentCandidate  `json:"items"`
}

type StandaloneAnimeRequest struct {
    MediaID   int    `json:"media_id"`
    Magnet    string `json:"magnet,omitempty"`
    Name      string `json:"name,omitempty"`
    NyaaID    int    `json:"nyaa_id,omitempty"`
    SizeBytes int64  `json:"size_bytes,omitempty"`
}
```

Handler rules:

- `{id}` invalid → 400 `INVALID_ID`
- AniList does not know the id → 404 `ANIME_NOT_FOUND`
- Nyaa down on candidates → 502 `NYAA_UNAVAILABLE`
- Empty items → 200 `{"query":"…","items":[]}`
- `q` present and non-empty → pass through; empty `q` → daemon query
- `{nyaaId}` non-numeric or ≤ 0 → 400 `INVALID_NYAA_ID` (spec: "digits only")
- Detail 404 → 404 `TORRENT_NOT_FOUND`
- Detail Nyaa down → 502 `NYAA_UNAVAILABLE`
- `{id}` on the detail URL is unused for the fetch; still validate it as a positive int so a junk prefix 400s
- POST: existing 409s run first (library, already-*). Then if `Magnet == ""`, today's `AddStandaloneAnime` then `DownloadStandaloneAnime`. If `Magnet != ""` and does not have prefix `magnet:`, 400 `INVALID_MAGNET`. If magnet present: **do not write the id in the handler** — `DownloadStandaloneAnimeWithMagnet` does. Response stays `{"added": N}`.

- [ ] **Step 1: Write the failing API tests**

Candidates: ranked packs, empty 200, `?q=` hits Nyaa with that q, movie path has `nyaa_id`, AniList 404, Nyaa 502.

Detail: sanitised HTML (reuse a fixture with `<script>`), file list, non-numeric `nyaaId` 400, Nyaa 404 → `TORRENT_NOT_FOUND`.

POST with magnet: 12 of 24 written; unknown range `added=0` and torrent in `FakeBackend.List()`; `AddErr` → id absent; POST without magnet still passes existing tests.

Look at `endpoint_standalone_animes_test.go` for `standaloneWriteServer`, `mockEmptyNyaa`, `postStandalone`. Add `postStandaloneMagnet` that sends the extended body.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/api/ -run 'TorrentCandidate|StandaloneAdd_WithMagnet' -count=1`

Expected: FAIL (handlers / fields missing).

- [ ] **Step 3: Implement handlers + routes**

`handleTorrentCandidates` GET dispatch. `r.URL.Query().Get("q")`. Map `daemon.TorrentCandidate` → API struct (they can be the same shape; either share the daemon type with json tags or copy — prefer a mapping function in the handler so the daemon type stays free of API json tags).

`handleTorrentCandidateDetail`: `strconv.Atoi(r.PathValue("nyaaId"))`.

Register in `SetupRoutes`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/internal/api/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: expose torrent candidates and add standalone with magnet`

---

### Task 5: API client + CLI

**Files:**
- Modify: `src/internal/api/client.go`, `src/internal/api/client_test.go`
- Modify: `src/cmd/cli/animes.go`, `src/cmd/cli/animes_test.go`
- Modify: `docs/guides/cli.md`

**Interfaces:**
- Consumes: the three HTTP contracts from Task 4
- Produces:

```go
func (c *Client) GetTorrentCandidates(id int, query string) (*TorrentCandidatesResponse, error)
func (c *Client) AddStandaloneAnime(req StandaloneAnimeRequest) (*StandaloneAnimeAddResponse, error)
```

Change `AddStandaloneAnime(mediaID int)` to take `StandaloneAnimeRequest` (only caller is the CLI). Update `client_test.go` table: existing add still posts `{"media_id":21}`; add a row for magnet; add `GetTorrentCandidates` with and without `q`.

CLI:

```
animes candidates <id> [--query STRING]
animes add <id> [--magnet URI]
```

`animes replace` is unchanged. No CLI command for the sanitised HTML.

`handleAnimesCandidates`: print the value (`printValue`); `--json` is the existing agent contract. Human table: name, size, seeders, nyaa_id, magnet truncated is fine if `printValue` already dumps structs — follow how `animes` / `animes search` print, do not invent a new table helper unless the dump is unreadable.

- [ ] **Step 1: Write the failing client + CLI tests**

`client_test.go`: add cases next to `AddStandaloneAnime`.

`animes_test.go`: httptest server, `animes candidates 21` hits `GET /api/v1/animes/21/torrent-candidates`; `--query foo` adds `q=foo`; `animes add 21 --magnet magnet:?xt=urn:btih:abc` posts the magnet.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/api/ -run TestClientMethodsHitExpectedPaths -count=1` and `go test ./src/cmd/cli/ -run 'Candidates|AddMagnet' -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement client methods and CLI subcommand/flag**

`GetTorrentCandidates`: `q` only in the query string when non-empty.

`animesCommand` Subcommands: insert `candidates` near `search`. `add` gains `&cli.StringFlag{Name: "magnet"}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/internal/api/ ./src/cmd/cli/ -count=1`

Expected: PASS.

Update `docs/guides/cli.md` Animes section:

```
autoanimedownloader animes candidates 154587
autoanimedownloader animes candidates 154587 --query "frieren s01"
autoanimedownloader animes add 154587
autoanimedownloader animes add 154587 --magnet "magnet:?xt=…"
```

One sentence: `candidates` is the ranked whole-title list the picker shows; `add --magnet` skips auto-pick.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: add animes candidates and add --magnet`

---

### Task 6: Frontend client, cache, picker route

**Files:**
- Modify: `src/internal/frontend/src/lib/api/client.ts`
- Create: `src/internal/frontend/src/lib/stores/pickerCache.ts`
- Create: `src/internal/frontend/src/routes/TorrentPicker.svelte`
- Modify: `src/internal/frontend/src/App.svelte`
- Modify: `src/internal/frontend/messages/en.json`, `pt-BR.json`
- Test: `src/internal/frontend/tests/component/TorrentPicker.test.ts`

**Interfaces:**
- Consumes: GET candidates, GET detail, POST standalone (magnet), POST replace
- Produces: `#/add/:id/torrents` and `#/status/:id/torrents`

`client.ts` types and functions:

```ts
export interface TorrentCandidate {
  name: string
  magnet: string
  size_bytes: number
  seeders: number
  leechers: number
  fansub: string
  resolution: string
  is_batch: boolean
  range_start: number
  range_end: number
  nyaa_id: number
}

export interface TorrentCandidates {
  query: string
  items: TorrentCandidate[]
}

export interface TorrentCandidateDetail {
  description_html: string
  files: { name: string; size_bytes: number }[]
  nyaa_url: string
}

export async function getTorrentCandidates(id: number, query?: string, signal?: AbortSignal): Promise<TorrentCandidates>
export async function getTorrentCandidateDetail(id: number, nyaaId: number, signal?: AbortSignal): Promise<TorrentCandidateDetail>
export async function addStandaloneAnime(mediaId: number, opts?: { magnet?: string; name?: string; nyaa_id?: number; size_bytes?: number }): Promise<{ added: number }>
```

`getTorrentCandidates` / detail / add use `{ silent: true, signal }` so the picker toasts itself (same as AddAnime search). `replaceAnimeWithMagnet` already exists.

`pickerCache.ts`: module writable holding `{ mediaId, query, items, error: ApiError | null } | null`. `set` on Add navigation. `clear` after successful confirm. Reload of the picker URL: cache miss → fetch.

`App.svelte` routes — **longer paths first**:

```ts
"/add/:id/torrents": TorrentPicker,
"/add": AddAnime,
"/status/:id/torrents": TorrentPicker,
"/status/:id": AnimeDetail,
```

Picker behaviour (one component, branch on `location` prefix):

- Params: `id` from svelte-spa-router.
- Back: `#/add` or `#/status/:id`. Breadcrumb matches.
- On mount: if cache matches this `id`, use it; else `getTorrentCandidates(id)`.
- Query field prefilled with `query`. Enter with non-empty → fetch with `q`, abort previous (AbortController, same as AddAnime). Enter on empty → no request.
- Custom query empty list: stay, show empty-state copy. Do not POST.
- List: index 0 selected, labelled recommended. Click/tap selects. Selecting fires detail GET for that `nyaa_id` only.
- Description pane: `description_html` via `{@html}` (already sanitised). File list below. Link `nyaa_url` (`rel="noopener noreferrer"` `target="_blank"`). Detail failure: error in the pane + Nyaa link still built as `https://nyaa.si/view/${nyaa_id}` if `nyaa_url` missing; Confirm stays enabled.
- Nyaa-down on candidates: error in the list area; query field still works.
- Footer: Back / Confirm ("Download this"). Confirm on add: `addStandaloneAnime(id, { magnet, name, nyaa_id, size_bytes })` of the selected row, or of the pasted magnet if that field is non-empty (paste is the fallback; no name/nyaa_id then). Success: existing add toasts, `pop()` to `#/add`. 409: toast, stay. Confirm on replace: `replaceAnimeWithMagnet(id, magnet)`, toast, back to `#/status/:id`.
- Magnet paste: input at the bottom of the page, not a modal.
- Layout: `lg:` two columns (list | pane); default one column, list then pane. Footer sticky. Use existing `Button`, tokens (`rounded-card`, `border-default`, `bg-card`, `text-heading`, …). No new CSS framework.

i18n keys (add to both JSON files; Portuguese in `pt-BR.json`):

| key | en | pt-BR |
|---|---|---|
| `picker_title` | Choose torrent | Escolher torrent |
| `picker_recommended` | Recommended | Recomendado |
| `picker_query_placeholder` | Search on Nyaa… | Buscar no Nyaa… |
| `picker_confirm` | Download this | Baixar este |
| `picker_back` | Back | Voltar |
| `picker_empty` | No packs, try another query | Nenhum lote, tente outra busca |
| `picker_nyaa_error` | Nyaa is unavailable. Try another query. | O Nyaa está indisponível. Tente outra busca. |
| `picker_detail_error` | Could not load the Nyaa page. | Não foi possível carregar a página do Nyaa. |
| `picker_open_nyaa` | Open on Nyaa | Abrir no Nyaa |
| `picker_magnet_placeholder` | Or paste a magnet URI | Ou cole um magnet |
| `picker_files` | Files | Arquivos |
| `picker_seeders` | {count} seeders | {count} seeders |
| `detail_replace_btn_anime` | Replace torrent | Substituir torrent |

Keep `detail_replace_btn_anime` key, change the English/Portuguese strings from "Replace Anime with Magnet" / "Substituir Anime com Magnet".

- [ ] **Step 1: Write the failing TorrentPicker tests** (`TorrentPicker.test.ts`, `AddAnime.test.ts` style)

Mock `getTorrentCandidates`, `getTorrentCandidateDetail`, `addStandaloneAnime`, `replaceAnimeWithMagnet`. svelte-spa-router params: the component reads `params` from the Router; in tests, render `TorrentPicker` the same way other routed pages are rendered — if `AddAnime` is rendered directly, TorrentPicker will need the id. Pattern: export nothing extra; read `params` from `svelte-spa-router`'s `push`/`location` **or** take nothing and parse `window.location.hash`. Prefer parsing hash `#/add/21/torrents` and `#/status/21/torrents` so the test can `window.location.hash = '#/add/21/torrents'` before render. If the Router injects params only when mounted through `App`, render through a tiny wrapper that sets hash and uses the same `routes` map as `App.svelte`.

Cases from the spec:

1. Non-empty items: first row selected (recommended), detail GET for that `nyaa_id`.
2. Enter on query field calls candidates with `q=`; new index 0 is recommended.
3. Confirm POSTs the selected magnet (and name / nyaa_id / size_bytes).
4. Description failure: list intact, error in the pane, Confirm enabled.
5. Empty custom query on Enter: no request.
6. Paste magnet + Confirm: POST that magnet.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/internal/frontend && bun run test:component -- tests/component/TorrentPicker.test.ts`

Expected: FAIL (module missing).

- [ ] **Step 3: Implement client, cache, route, picker, i18n**

Format size with the existing filesize helper if one exists (`lib/domain/format.ts`); otherwise a short local formatter (GiB/MiB). Do not invent a new design language.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/internal/frontend && bun run test:component -- tests/component/TorrentPicker.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: add torrent picker screen with on-demand nyaa page`

---

### Task 7: Add flow + replace flow

**Files:**
- Modify: `src/internal/frontend/src/routes/AddAnime.svelte`
- Modify: `src/internal/frontend/tests/component/AddAnime.test.ts`
- Modify: `src/internal/frontend/src/routes/AnimeDetail.svelte` (whole-title replace button)
- Test: existing AnimeDetail tests that click "Replace Anime with Magnet" — update the assertion from opening a modal to navigating to `#/status/{id}/torrents`

**Interfaces:**
- Consumes: `getTorrentCandidates`, `addStandaloneAnime`, `pickerCache`, `push` from `svelte-spa-router`
- Produces: Add card → picker or silent POST; detail "Replace torrent" → picker. Per-episode replace modal **stays**.

Add card `add()`:

1. `getTorrentCandidates(result.id)` (no `q`).
2. **GET error `NYAA_UNAVAILABLE`:** `pickerCache.set({ mediaId, query: '', items: [], error })`, `push('/add/'+id+'/torrents')`. Nothing saved.
3. **200, `items.length === 0`:** `addStandaloneAnime(result.id)` with no magnet — today's toast. Do not open the picker.
4. **Non-empty:** `pickerCache.set({ mediaId, query, items, error: null })`, `push('/add/'+id+'/torrents')`.
5. Other errors: existing `addErrorMessage` toast.

AnimeDetail: the button that currently sets `replaceAnimeOpen = true` becomes `push('/status/'+animeId+'/torrents')`. Delete the whole-title magnet modal (`replaceAnimeOpen`, `replaceAnimeMagnet`, `confirmReplaceAnime`). Keep the per-episode magnet modal.

- [ ] **Step 1: Write the failing AddAnime tests**

Extend the client mock with `getTorrentCandidates`. Cases:

```ts
it('empty first search adds without magnet and does not open the picker', async () => {
  getTorrentCandidates.mockResolvedValue({ query: 'One Piece', items: [] })
  addStandaloneAnime.mockResolvedValue({ added: 2 })
  // click Add
  expect(addStandaloneAnime).toHaveBeenCalledWith(21, undefined) // or with no magnet fields
  // hash stays off /add/:id/torrents
})

it('non-empty first search navigates to the picker and does not POST', async () => {
  getTorrentCandidates.mockResolvedValue({ query: 'Frieren', items: [candidate] })
  // click Add
  expect(addStandaloneAnime).not.toHaveBeenCalled()
  expect(window.location.hash).toMatch(/#\/add\/21\/torrents/)
})
```

Mock `svelte-spa-router`'s `push` if hash is not writable in jsdom — spy `push` and assert `push('/add/21/torrents')`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/internal/frontend && bun run test:component -- tests/component/AddAnime.test.ts`

Expected: FAIL (Add still POSTs immediately).

- [ ] **Step 3: Implement AddAnime + AnimeDetail**

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/internal/frontend && bun run test:component`

Expected: PASS, including AnimeDetail suites (batch row still must not call `replaceAnimeWithMagnet` on pack-row replace — that was already true; the header button now navigates).

- [ ] **Step 5: Commit (proposed, do not run)**

`feat: open torrent picker from add and replace`

---

### Task 8: Docs + decision #105

**Files:**
- Modify: `docs/agents/architecture.md` — API table rows + a negative fact
- Modify: `docs/guides/cli.md` (if anything left from Task 5)
- Modify: `docs/guides/webui.md` — Add anime + Anime Detail + a short picker section
- Modify: `docs/agents/decisions.md` — index line + entry **#105** (highest today is #104)
- Modify: `docs/TODO.md` — leave the episode-picker bullet; the first 2.7 bullet stays until this ships

**Interfaces:** none.

Architecture API table (next to the standalone POST row):

```
| `GET` | `/api/v1/animes/{id}/torrent-candidates?q=` | `handleTorrentCandidates` | `endpoint_torrent_candidates.go` — ranked whole-title list, cap 8. Empty `items` is 200. `{id}` need not be tracked. `q` skips title variants and season/part. Does not fetch `/view/<id>`. 502 `NYAA_UNAVAILABLE` |
| `GET` | `/api/v1/animes/{id}/torrent-candidates/{nyaaId}` | `handleTorrentCandidateDetail` | sanitised Nyaa page + file list. `{nyaaId}` digits only. `{id}` unused for the fetch. 404 `TORRENT_NOT_FOUND` |
| `POST` | `/api/v1/standalone-animes` | `handleStandaloneAnimeAdd` | body `{"media_id":21,"magnet":"…"}` with `magnet` optional; optional `name`/`nyaa_id`/`size_bytes` for coverage and the disk guard. No magnet: auto-pick. Magnet: add torrent then write the id; records only covered aired episodes |
```

Negative fact in the daemon or API file map: **the picker does not persist**. Candidates live in a frontend memory cache for one navigation; a reload of `#/add/:id/torrents` fetches again. Custom `q` is not saved.

Decision #105, four facts the spec asked to record, plus the POST extra fields:

Title something like: **Empty first search skips the picker; custom `q` drops variants; description is on demand; add-with-magnet writes the id after the torrent.**

Body, self-contained: (1) first Add search with `items: []` POSTs without magnet (singles / track-with-zero); a later empty custom query does not. (2) custom `q` skips title variants and season/part; ignore / seeders / size / pack-or-movie / ranking / `titleMatchesQuery` still run. (3) `/view/<id>` only for the selected row. (4) magnet add: session then id; Add failure leaves no id; later failure runs `RemoveStandaloneAnime`. (5) `name` / `nyaa_id` / `size_bytes` are optional on POST because a magnet URI cannot rebuild coverage or the Nyaa size.

Index at the top of `decisions.md`: add `#105` next to `#104`.

webui.md: Add anime mentions the picker; new subsection **Torrent picker** (`/add/:id/torrents`, `/status/:id/torrents`); Anime Detail "Replace torrent" opens it; magnet paste is on that page.

- [ ] **Step 1: Edit the docs**

- [ ] **Step 2: `go test ./...`** and `cd src/internal/frontend && bun run test:component && bun run test:unit`

Expected: PASS.

- [ ] **Step 3: Commit (proposed, do not run)**

`docs: record torrent picker endpoints and decision 105`

---

## Self-review

**Spec coverage**

| Spec requirement | Task |
|---|---|
| Movie `DetailURL` | 1 |
| Sanitised `#torrent-description` + file list, SSRF-safe URL | 1, 4 |
| Preview cap 8, filters, ranking, custom `q` skips variants/season-part | 2 |
| Empty items 200; Nyaa 502; AniList 404 | 4 |
| POST magnet coverage + rollback; POST without magnet unchanged | 3, 4 |
| CLI `candidates` / `add --magnet`; no HTML command | 5 |
| Add: empty first search skips picker; Nyaa error still opens; cache one navigation | 6, 7 |
| Replace uses same screen; paste on the page; per-episode picker out of scope | 7 |
| Docs + decision | 8 |

**Type consistency:** `TorrentCandidate` fields stay `name/magnet/size_bytes/seeders/leechers/fansub/resolution/is_batch/range_start/range_end/nyaa_id` from daemon → API → TS. `StandaloneAnimeRequest` keeps `media_id` and adds `magnet`, `name`, `nyaa_id`, `size_bytes`.

**Not in this plan:** per-episode picker, changing replace coverage, fetching `/view/<id>` in the candidates handler, deleting the spec (that happens when the work is merged and TODO is ticked).
