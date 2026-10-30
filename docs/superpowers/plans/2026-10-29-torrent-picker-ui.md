# Torrent picker UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the whole-title torrent picker an identity header, a Nyaa-labeled search, two independent confirms (list vs magnet), packs-only copy, and a description pane that keeps tables and overflow.

**Architecture:** `PreviewWholeTitleTorrents` already called `GetMediaByID`; return that identity on `GET /animes/{id}/torrent-candidates` so a picker reload still has a title. Split the single confirm into **Download this** (selected row) and **Download magnet** (pasted URI). Paste no longer overrides the list button ([decisions.md #111](../../agents/decisions.md)). The sanitizer allow-list grows table tags so Nyaa HTML tables stay tables.

**Tech Stack:** Go 1.25, existing `daemon` + `api` + `nyaa`, Svelte 5 + svelte-spa-router + Vitest + Testing Library, Paraglide (`en.json` / `pt-BR.json`).

## Global Constraints

- Spec: [2026-10-28-torrent-picker-ui-design.md](../specs/2026-10-28-torrent-picker-ui-design.md). Same two routes. No new endpoint. No new CLI verb (`animes candidates` already dumps the payload).
- Out of scope: episode picker (`v2.9.0`); showing dropped singles; changing `min_seeders` / size ceilings / cap 8 / variants; markdown pipe-art reconstruction; adding `seasonYear` to `GetMediaByID`; replace-coverage changes; persisting `pickerCache`.
- Repo language is English (code, comments, logs, docs). Portuguese only in `pt-BR.json`.
- Error wrapping: `fmt.Errorf("failed to <action>: %w", err)`. Handlers are closures over `*Server`. Responses go through `JSONSuccess` / `JSONError` / `JSONInternalError`.
- Import order: internal, stdlib, third-party.
- Nyaa is mocked (`nyaa.MockNyaaHttpGet`). AniList is mocked (`anilist.MockAniListDo` / existing preview fixtures).
- Do **not** `git commit` or `git push`. CLAUDE.md: the user commits. Each "Commit" step is the proposed subject only.
- After any Go change: `go test ./...`. After frontend change: `cd src/internal/frontend && bun run test:component -- tests/component/TorrentPicker.test.ts` (and `AddAnime.test.ts` if that file changed). After i18n JSON change: `bun run i18n:compile`.
- #108.5 stays: API still accepts a magnet without `name` / `nyaa_id` / `size_bytes`.
- `cover_image` is `coverImage.medium` only. Do not fall back to `large`. Do not add `seasonYear` to `GetMediaByID`.
- At most one solid accent on the screen: **Download this**. Search and **Download magnet** are ghost.
- Decision number is **#111** (highest existing is #110).

---

## File map

**Modify**

- `src/internal/nyaa/detail.go` — allow-list gains `table`, `thead`, `tbody`, `tfoot`, `tr`, `th`, `td`, `hr`
- `src/tests/unit/nyaa_test.go` — sanitizer keeps `<table>` and `<hr>`, still strips `<script>`
- `src/internal/daemon/preview.go` — `PreviewWholeTitleTorrents` returns `WholeTitlePreview` (query + media + items)
- `src/internal/daemon/preview_test.go` — callers of the old 3-return signature; media identity on a 200
- `src/internal/api/endpoint_torrent_candidates.go` — `media` on `TorrentCandidatesResponse`
- `src/internal/api/endpoint_torrent_candidates_test.go` — JSON includes `media` from `standaloneMediaJSON`
- `src/internal/frontend/src/lib/api/client.ts` — `TorrentCandidates.media`
- `src/internal/frontend/src/lib/stores/pickerCache.ts` — optional `media` for first paint from Add
- `src/internal/frontend/src/routes/AddAnime.svelte` — put search-card identity into `pickerCache`
- `src/internal/frontend/src/routes/TorrentPicker.svelte` — layout, two confirms, packs-only copy
- `src/internal/frontend/tests/component/TorrentPicker.test.ts` — spec tests; delete paste-overrides-list test
- `src/internal/frontend/tests/component/AddAnime.test.ts` — cache now carries `media`
- `src/internal/frontend/messages/en.json`, `pt-BR.json` — new picker keys
- `docs/guides/webui.md` — picker subsection
- `docs/agents/architecture.md` — picker paragraph
- `docs/agents/decisions.md` — **#111**
- `docs/TODO.md` — `v2.7.2` bullet stays until it ships (do not mark released in this change; the spec row in README drops only when implemented — drop it in the docs task of this same change, matching "when it ships" as this implementation)
- `docs/superpowers/specs/README.md` — drop the torrent-picker-ui row
- `docs/superpowers/specs/2026-10-28-torrent-picker-ui-design.md` — delete after implementation (executed specs are deleted; git history keeps them)

**Do not create new files** except the decision is an entry in the existing `decisions.md`.

---

### Task 1: Keep Nyaa tables in the description sanitizer

**Files:**
- Modify: `src/internal/nyaa/detail.go` (`descriptionAllowedTags`)
- Test: `src/tests/unit/nyaa_test.go` (`TestFetchTorrentDetail_SanitisesHTMLAndListsFiles` or a sibling)

**Interfaces:**
- Consumes: `sanitizeDescriptionHTML` allow-list; `FetchTorrentDetail`
- Produces: HTML that still contains `<table>`, `<thead>`, `<tbody>`, `<tr>`, `<th>`, `<td>`, `<hr>`; still without `<script>` / `onclick`

- [ ] **Step 1: Write the failing test**

Extend `TestFetchTorrentDetail_SanitisesHTMLAndListsFiles` in `src/tests/unit/nyaa_test.go` so the fixture description includes a table and an `hr`, and assert they survive. Keep the existing script/iframe/onclick assertions.

Inside the `#torrent-description` div of the existing HTML fixture, add:

```html
  <table><thead><tr><th>Res</th></tr></thead><tbody><tr><td>1080p</td></tr></tbody></table>
  <hr>
  <script>alert(1)</script>
```

(The existing fixture already has a `<script>`; add the table and `hr` next to it.)

Add assertions after the existing sanitiser checks:

```go
	if !strings.Contains(d.DescriptionHTML, "<table") || !strings.Contains(d.DescriptionHTML, "<td") {
		t.Errorf("table tags unwrapped: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, "<hr") {
		t.Errorf("hr stripped: %s", d.DescriptionHTML)
	}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./src/tests/unit/ -run TestFetchTorrentDetail_SanitisesHTMLAndListsFiles -count=1`

Expected: FAIL — `table tags unwrapped` because `table` is not in `descriptionAllowedTags` and unknown tags unwrap deepest-first.

- [ ] **Step 3: Add the tags to the allow-list**

In `src/internal/nyaa/detail.go`, `descriptionAllowedTags`:

```go
var descriptionAllowedTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "pre": true, "code": true, "blockquote": true,
	"br": true, "em": true, "strong": true, "b": true, "i": true, "a": true, "img": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true,
	"hr": true,
}
```

Still no `style` / `class` / event handlers. `a.href` and `img.src` stay the only attributes (`cleanDescriptionAttrs` is unchanged).

- [ ] **Step 4: Run the test and watch it pass**

Run: `go test ./src/tests/unit/ -run TestFetchTorrentDetail_SanitisesHTMLAndListsFiles -count=1`

Expected: PASS.

Then: `go test ./...`

- [ ] **Step 5: Commit (proposed subject only — do not run git commit)**

`fix: keep nyaa description tables instead of unwrapping them`

---

### Task 2: Return AniList identity on the candidates payload

**Files:**
- Modify: `src/internal/daemon/preview.go`
- Modify: `src/internal/daemon/preview_test.go` (every `PreviewWholeTitleTorrents` call)
- Modify: `src/internal/api/endpoint_torrent_candidates.go`
- Modify: `src/internal/api/endpoint_torrent_candidates_test.go`
- Test: same Go tests

**Interfaces:**
- Consumes: `anilist.GetMediaByID` already called in `PreviewWholeTitleTorrents`; `getAnimeTitleSafe`; `anime.Media.CoverImage.Medium`, `Format`, `Episodes`
- Produces:

```go
// daemon/preview.go
type PickerMedia struct {
	Title      string
	CoverImage string // coverImage.medium; empty means omit on the JSON
	Format     string
	Episodes   *int
}

type WholeTitlePreview struct {
	Query string
	Media PickerMedia
	Items []TorrentCandidate
}

func PreviewWholeTitleTorrents(configs *files.Config, mediaID int, query string) (WholeTitlePreview, error)
```

```go
// api/endpoint_torrent_candidates.go
type TorrentCandidateMedia struct {
	Title      string `json:"title"`
	CoverImage string `json:"cover_image,omitempty"`
	Format     string `json:"format"`
	Episodes   *int   `json:"episodes,omitempty"`
}

type TorrentCandidatesResponse struct {
	Query string                 `json:"query"`
	Media TorrentCandidateMedia  `json:"media"`
	Items []TorrentCandidate     `json:"items"`
}
```

`animes candidates` prints this via `printValue`; no CLI change. `GetTorrentCandidates` already unmarshals into `TorrentCandidatesResponse`.

- [ ] **Step 1: Write the failing API test**

In `src/internal/api/endpoint_torrent_candidates_test.go`, at the end of `TestTorrentCandidates_RankedPacks` (after the existing item assertions), add:

```go
	if data.Media.Title != "Standalone Anime" {
		t.Fatalf("media.title: want Standalone Anime, got %q", data.Media.Title)
	}
	if data.Media.Format != "TV" {
		t.Fatalf("media.format: want TV, got %q", data.Media.Format)
	}
	if data.Media.Episodes == nil || *data.Media.Episodes != 24 {
		t.Fatalf("media.episodes: want 24, got %v", data.Media.Episodes)
	}
	if data.Media.CoverImage != "" {
		t.Fatalf("standaloneMediaJSON has empty medium; cover_image must omit, got %q", data.Media.CoverImage)
	}
```

`standaloneMediaJSON` (`src/internal/api/standalone_test.go`) already has english/romaji `"Standalone Anime"`, format `TV`, episodes `24`, `coverImage.medium` empty. Do not change that fixture's `large` field; the spec uses **medium only**.

Also add a daemon-level assertion in `TestPreviewWholeTitleTorrents_SeriesPacksOnlyRankedAndCapped` once the signature has changed (Step 3). For now the API test is the red.

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./src/internal/api/ -run TestTorrentCandidates_RankedPacks -count=1`

Expected: FAIL — `media.title` empty / zero value because `TorrentCandidatesResponse` has no `Media`.

- [ ] **Step 3: Change `PreviewWholeTitleTorrents` to return `WholeTitlePreview`**

In `src/internal/daemon/preview.go`:

```go
type PickerMedia struct {
	Title      string
	CoverImage string
	Format     string
	Episodes   *int
}

type WholeTitlePreview struct {
	Query string
	Media PickerMedia
	Items []TorrentCandidate
}

func pickerMediaFrom(anime *anilist.MediaList) PickerMedia {
	m := PickerMedia{
		Title:      getAnimeTitleSafe(*anime),
		CoverImage: anime.Media.CoverImage.Medium,
		Format:     string(anime.Media.Format),
		Episodes:   anime.Media.Episodes,
	}
	return m
}

func PreviewWholeTitleTorrents(configs *files.Config, mediaID int, query string) (WholeTitlePreview, error) {
	empty := WholeTitlePreview{}
	anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return empty, fmt.Errorf("failed to fetch anime %d from AniList: %w", mediaID, err)
	}
	if anime == nil {
		return empty, fmt.Errorf("anime %d does not exist on AniList", mediaID)
	}
	media := pickerMediaFrom(anime)
	// ... existing search / filter / cap ...
	// every `return used, nil, err` becomes `return WholeTitlePreview{Query: used, Media: media}, err`
	// the success return:
	return WholeTitlePreview{Query: used, Media: media, Items: toCandidates(results)}, nil
}
```

On Nyaa failure the media was already fetched — include it on the error return's `Query`/`Media` if that keeps the function simple, but the handler still maps Nyaa errors to 502 and does not send the body. Do not invent a 200-with-error path.

Update every caller in `preview_test.go`:

```go
preview, err := PreviewWholeTitleTorrents(...)
used, items := preview.Query, preview.Items
```

The unknown-media test still expects an error and empty items. After the series-packs test succeeds, assert:

```go
	if preview.Media.Title != "Standalone Anime" {
		t.Fatalf("media title: got %q", preview.Media.Title)
	}
	if preview.Media.Format != "TV" {
		t.Fatalf("media format: got %q", preview.Media.Format)
	}
```

- [ ] **Step 4: Map media in the candidates handler**

```go
		preview, err := daemon.PreviewWholeTitleTorrents(config, id, r.URL.Query().Get("q"))
		// existing error branches unchanged, using preview only on success
		JSONSuccess(w, http.StatusOK, TorrentCandidatesResponse{
			Query: preview.Query,
			Media: TorrentCandidateMedia{
				Title:      preview.Media.Title,
				CoverImage: preview.Media.CoverImage,
				Format:     preview.Media.Format,
				Episodes:   preview.Media.Episodes,
			},
			Items: mapTorrentCandidates(preview.Items),
		})
```

- [ ] **Step 5: Run tests**

Run: `go test ./src/internal/daemon/ -run PreviewWholeTitleTorrents -count=1`

Run: `go test ./src/internal/api/ -run TestTorrentCandidates -count=1`

Run: `go test ./...`

Expected: PASS. CLI tests that construct `TorrentCandidatesResponse{Query, Items}` still compile because `Media` is optional (zero value).

- [ ] **Step 6: Commit (proposed subject only)**

`feat: return anilist identity on torrent candidates`

---

### Task 3: Two confirms, identity header, Nyaa search, packs-only copy

**Files:**
- Modify: `src/internal/frontend/messages/en.json`, `pt-BR.json`
- Modify: `src/internal/frontend/src/lib/api/client.ts` (`TorrentCandidates.media`)
- Modify: `src/internal/frontend/src/lib/stores/pickerCache.ts`
- Modify: `src/internal/frontend/src/routes/AddAnime.svelte` (cache `media` from the search card)
- Modify: `src/internal/frontend/src/routes/TorrentPicker.svelte`
- Test: `src/internal/frontend/tests/component/TorrentPicker.test.ts`
- Test: `src/internal/frontend/tests/component/AddAnime.test.ts` (cache shape)

**Interfaces:**
- Consumes: `GET` payload `media` from Task 2; `Cover.svelte` (`src`, `alt`, `radiusClass`); `Button` variants `solid` | `ghost`; existing `addStandaloneAnime` / `replaceAnimeWithMagnet`
- Produces: two independent confirms; header identity; Nyaa-labeled search with a Search button; pack count line; magnet strip without a sticky page footer

Frontend types:

```ts
export interface TorrentCandidateMedia {
  title: string
  cover_image?: string
  format: string
  episodes?: number
}

export interface TorrentCandidates {
  query: string
  media: TorrentCandidateMedia
  items: TorrentCandidate[]
}

export interface PickerCache {
  mediaId: number
  query: string
  items: TorrentCandidate[]
  error: ApiError | null
  media?: TorrentCandidateMedia
}
```

i18n keys (en → pt-BR):

| Key | en | pt-BR |
|---|---|---|
| `picker_query_label` | Nyaa search | Busca no Nyaa |
| `picker_search` | Search | Buscar |
| `picker_hint` | Pick the pack. The first is what the daemon would download. | Escolha o lote. O primeiro é o que o daemon baixaria. |
| `picker_pack_count` | `{count} packs · single episodes stay off this screen` | `{count} lotes · episódios avulsos ficam fora desta tela` |
| `picker_back_add` | ← Add anime | ← Adicionar anime |
| `picker_back_replace` | ← {title} | ← {title} |
| `picker_magnet_label` | Or paste a magnet | Ou cole um magnet |
| `picker_magnet_confirm` | Download magnet | Baixar magnet |

Keep `picker_confirm` ("Download this" / "Baixar este") on the list button. Keep `picker_query_placeholder` as the input's `aria-label` / empty placeholder. Keep `picker_title` as the kicker "Choose torrent". Drop the breadcrumb (do not use `add_title` / `nav_status` as crumbs). `picker_back` ("Back") is unused after the sticky footer goes; leave the key in the JSON (do not delete unused keys unless a test asserts the file has no orphans).

- [ ] **Step 1: Add i18n keys and compile**

Add the eight keys to both JSON files (place them next to the existing `picker_*` keys). Then:

```bash
cd src/internal/frontend && bun run i18n:compile
```

- [ ] **Step 2: Write the failing frontend tests**

In `TorrentPicker.test.ts`:

1. Default `getTorrentCandidates` mock grows `media`:

```ts
  getTorrentCandidates.mockResolvedValue({
    query: 'Frieren',
    media: {
      title: 'Frieren: Beyond Journey\'s End',
      cover_image: 'https://example/cover.jpg',
      format: 'TV',
      episodes: 28,
    },
    items: [
      candidate(),
      candidate({ name: 'Other pack', nyaa_id: 222, magnet: 'magnet:?xt=urn:btih:bbb' }),
    ],
  })
```

Every other `mockResolvedValue` / `mockResolvedValueOnce` in this file that returns candidates must include `media` (title at least). The cache test's `pickerCache.set` may omit `media` — that is the "until media arrives" path.

2. **Delete** the test `'POSTs the pasted magnet instead of the selected row when that field is non-empty'`.

3. Add:

```ts
describe('TorrentPicker — header', () => {
  it('shows media.title and back is ← Add anime on add', async () => {
    await mountPicker()
    expect(screen.getByText("Frieren: Beyond Journey's End")).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '← Add anime' })).toHaveAttribute('href', '#/add')
  })

  it('replace back link is ← {title}', async () => {
    await mountPicker('#/status/21/torrents')
    expect(screen.getByRole('link', { name: '← Frieren: Beyond Journey\'s End' })).toHaveAttribute(
      'href',
      '#/status/21',
    )
  })
})

describe('TorrentPicker — search', () => {
  it('has a Nyaa search label and a Search button; Enter still refetches; empty submit does not', async () => {
    await mountPicker()
    expect(screen.getByText('Nyaa search')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument()

    const field = screen.getByRole('searchbox')
    await fireEvent.input(field, { target: { value: '' } })
    await fireEvent.submit(field.closest('form') as HTMLFormElement)
    expect(getTorrentCandidates).toHaveBeenCalledTimes(1)

    await fireEvent.input(field, { target: { value: 'frieren batch' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() =>
      expect(getTorrentCandidates).toHaveBeenLastCalledWith(21, 'frieren batch', expect.anything()),
    )
  })
})

describe('TorrentPicker — pack count', () => {
  it('shows the pack count for a 2-row list', async () => {
    await mountPicker()
    expect(screen.getByText('2 packs · single episodes stay off this screen')).toBeInTheDocument()
  })

  it('shows the pack count for a 1-row list', async () => {
    getTorrentCandidates.mockResolvedValue({
      query: 'Frieren',
      media: { title: 'Frieren: Beyond Journey\'s End', format: 'TV', episodes: 28 },
      items: [candidate()],
    })
    await mountPicker()
    expect(screen.getByText('1 packs · single episodes stay off this screen')).toBeInTheDocument()
  })
})
```

Paraglide does not pluralise this string — the spec's English is `{count} packs` for 1 and 2. Do not invent "1 pack".

4. Confirm tests:

```ts
  it('Download this POSTs the selected row even when the magnet field is filled', async () => {
    await mountPicker()
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:pasted' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))
    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, {
        magnet: 'magnet:?xt=urn:btih:aaa',
        name: '[SubsPlease] Frieren (01-16) [1080p]',
        nyaa_id: 12345,
        size_bytes: 12_000_000_000,
      }),
    )
    expect(addStandaloneAnime.mock.calls[0][1]).toHaveProperty('name')
  })

  it('Download magnet is disabled when the field is empty', async () => {
    await mountPicker()
    expect(screen.getByRole('button', { name: 'Download magnet' })).toBeDisabled()
  })

  it('Download magnet POSTs { magnet } only', async () => {
    await mountPicker()
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:pasted' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download magnet' }))
    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, { magnet: 'magnet:?xt=urn:btih:pasted' }),
    )
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('name')
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('nyaa_id')
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('size_bytes')
  })

  it('replace: list button uses the selected magnet; magnet button uses the pasted URI', async () => {
    await mountPicker('#/status/21/torrents')
    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))
    await waitFor(() =>
      expect(replaceAnimeWithMagnet).toHaveBeenCalledWith(21, 'magnet:?xt=urn:btih:aaa'),
    )

    replaceAnimeWithMagnet.mockClear()
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:pasted' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download magnet' }))
    await waitFor(() =>
      expect(replaceAnimeWithMagnet).toHaveBeenCalledWith(21, 'magnet:?xt=urn:btih:pasted'),
    )
  })
```

5. File hover class — after the description pane tests, once a file row is in the document:

```ts
  it('file row has a hover class', async () => {
    await mountPicker()
    const row = await screen.findByTitle('Frieren_01.mkv')
    expect(row.closest('li')?.className).toMatch(/hover:bg-control/)
  })
```

The magnet field currently uses `placeholder` "Or paste a magnet URI" (`picker_magnet_placeholder`). After the layout change the accessible name is that placeholder **or** the new label. Prefer `getByLabelText('Or paste a magnet URI')` if `aria-label` stays on the input; if you move the accessible name to `picker_magnet_label`, update the queries to `'Or paste a magnet'`. Keep `aria-label={T.magnetPlaceholder}` so existing placeholder copy still names the field.

6. `AddAnime.test.ts` cache assertions grow `media` matching the search card (`result()` helper's `title`, `cover`, `format`, `episodes`). If the helper has no cover/format, pass what it has (`title` is enough; `media` may be `{ title: result.title, format: result.format, episodes: result.episodes, cover_image: result.cover }`).

- [ ] **Step 3: Run tests and watch them fail**

Run: `cd src/internal/frontend && bun run test:component -- tests/component/TorrentPicker.test.ts`

Expected: FAIL — missing keys / missing Search button / paste still wins on **Download this** / no pack count.

- [ ] **Step 4: Implement the picker**

`pickerCache.ts`: add optional `media?: TorrentCandidateMedia`.

`AddAnime.svelte`: when setting the cache (success and NYAA_UNAVAILABLE), include:

```ts
media: {
  title: result.title,
  cover_image: result.cover,
  format: result.format,
  episodes: result.episodes || undefined,
},
```

`TorrentPicker.svelte` behavior:

- `let media: TorrentCandidateMedia | null = null`. On cache hit, `media = cached.media ?? null`. On `loadCandidates` success, `media = found.media ?? media`.
- `canDownloadThis = Boolean(selected?.magnet) && !confirming && !loading` — independent of `pastedMagnet`.
- `canDownloadMagnet = Boolean(pastedMagnet.trim()) && !confirming && !loading`.
- Split `confirm()` into `confirmSelected()` and `confirmMagnet()`:
  - add selected: `{ magnet, name, nyaa_id, size_bytes }` from the row
  - add magnet: `{ magnet: pastedMagnet.trim() }` only
  - replace selected: `replaceAnimeWithMagnet(mediaId, selected.magnet)`
  - replace magnet: `replaceAnimeWithMagnet(mediaId, pastedMagnet.trim())`
  - both still `pop` (add) or `push(/status/:id)` (replace) and `pickerCache.clear()`
- Point at the decision: `// Two confirms: paste does not override the list button (decisions.md #111).`

Layout (replace the current header / form / sticky footer):

- Text back link: add → `m.picker_back_add()` href `#/add`; replace → `m.picker_back_replace({ title: media?.title || m.picker_title() })` href `#/status/:id`. Spec: until media arrives, replace back is `← Choose torrent`.
- Header row: `Cover` in a `h-[68px] w-[48px] shrink-0 overflow-hidden` box, `src={media?.cover_image}`, `alt={media?.title ?? ''}`, `radiusClass="rounded-field"`. Kicker `picker_title`. Title `media?.title`. Meta: `format` (underscores to spaces) plus, when `episodes` is set, ` · ${episodes} episodes` — use `m.status_episodes_count({ count })` for the episodes half so pt-BR stays translated. No year.
- Hint: `picker_hint`.
- Form: visible `<span>`/`<label>` `picker_query_label`. Input with a persistent `Nyaa` badge (a `Chip variant="neutral"` or a `span` with `rounded-badge` inside the field, text `Nyaa`, `aria-hidden`). `Button variant="ghost" type="submit"` labelled `picker_search`. Empty submit remains a no-op.
- After the form, when not loading: `{n} packs · …` via `m.picker_pack_count({ count: items.length })`, including `n = 0` and Nyaa-error empty list (list loaded). Show it whenever `!loading` after the first attempt (cache hit or fetch settled), including `listError`.
- Grid: `grid grid-cols-1 gap-4 md:grid-cols-[1.15fr_0.85fr] md:items-start` (list slightly wider; stack below `md`).
- **Download this** sits in a footer **inside the list card**, `justify-end`, `variant="solid"`, `disabled={!canDownloadThis}`. Not in a page sticky footer.
- Detail: wrap `{@html detail.description_html}` in

```html
<div class="max-h-80 overflow-x-auto overflow-y-auto overflow-wrap-anywhere [&_a]:text-accent [&_img]:max-w-full [&_pre]:overflow-x-auto [&_pre]:whitespace-pre-wrap [&_table]:border [&_table]:border-default [&_th]:border [&_th]:border-default [&_td]:border [&_td]:border-default [&_th]:px-2 [&_td]:px-2">
```

  Use `overflow-wrap: anywhere` via `break-anywhere` if the Tailwind build has it; otherwise a style attribute `overflow-wrap: anywhere`. `img max-w-full` stays.

- File rows: `class="flex justify-between gap-2 hover:bg-control"` and the name span `hover:text-heading`. Truncate + `title` stay.
- Magnet strip **below the grid**, not sticky: label `picker_magnet_label`, field, ghost **Download magnet** `disabled={!canDownloadMagnet}`. Filling the field does not clear or restyle the list selection and does not change **Download this**.
- Remove the page-level sticky footer and the second Back next to confirm. Remove the `ChevronRight` breadcrumb.

- [ ] **Step 5: Run frontend tests**

Run: `cd src/internal/frontend && bun run test:component -- tests/component/TorrentPicker.test.ts tests/component/AddAnime.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit (proposed subject only)**

`feat: split picker confirm into list and magnet actions`

---

### Task 4: Docs — decision #111, architecture, webui, spec retirement

**Files:**
- Modify: `docs/agents/decisions.md` (Index + entry **#111**)
- Modify: `docs/agents/architecture.md` (picker paragraph)
- Modify: `docs/guides/webui.md` (Torrent picker subsection)
- Modify: `docs/TODO.md` (`v2.7.2` — keep the bullet; do not stamp released until the version is cut; the spec is now implemented — rewrite the bullet as implemented, without a release date, same pattern as other "implemented, ships in this minor" lines, OR leave as the v2.7.2 section that this change fulfills. Mark the spec as implemented in the bullet text.)
- Modify: `docs/superpowers/specs/README.md` — drop the torrent-picker-ui row
- Delete: `docs/superpowers/specs/2026-10-28-torrent-picker-ui-design.md` (executed specs are deleted; git history keeps them)
- `TorrentPicker.svelte` already points at #111 from Task 3

**Interfaces:** none.

- [ ] **Step 1: Decision #111**

Index line (after #110):

```
- [**#111** — Two picker confirms; a paste does not override Download this](#111-two-picker-confirms-a-paste-does-not-override-download-this)
```

Entry at the end of the file:

```markdown
### 111. Two picker confirms; a paste does not override Download this

**Location:** `TorrentPicker.svelte` (`confirmSelected` / `confirmMagnet`);
`GET /api/v1/animes/{id}/torrent-candidates` `media` object.

**The rule:** the picker has two confirms. **Download this** always POSTs the selected
row (add: magnet + `name` / `nyaa_id` / `size_bytes`; replace: that magnet).
**Download magnet** POSTs the trimmed field (add: `{ magnet }` only; replace: that
URI). A non-empty paste does not change **Download this**. Both may be enabled at
once; the click chooses. #108.5 stays: the API still accepts a magnet without
coverage fields. `media` on the candidates payload is the `GetMediaByID` already
done for the search — no second AniList round-trip, no `seasonYear`.

**Why:** one solid button that took `pastedMagnet.trim() || selected.magnet` made a
paste silently override the highlighted row. The screen is a confirm of a ranked
pack and a magnet escape hatch, not one control with two meanings.

**Don't "fix" by:** merging the two buttons again; reading `GET /animes/{id}` for
the add-mode header (the anime is not tracked yet); widening `GetMediaByID` for a
year.
```

- [ ] **Step 2: architecture.md picker paragraph**

In the Routes paragraph that mentions `TorrentPicker.svelte`, add: no page-level sticky footer; two confirms (list vs magnet); `media` on the candidates payload is the AniList identity for a reload. Point at #111.

- [ ] **Step 3: webui.md picker subsection**

Replace the current "Confirm on Add posts the selected magnet (or the pasted one)" sentence with: identity header (cover + title + format · episodes); Nyaa-labeled search with a Search button; packs-only count line; **Download this** in the list card footer; magnet strip below the grid with **Download magnet**. Paste does not override the list button.

- [ ] **Step 4: Specs README + delete the spec + TODO**

Drop the torrent-picker-ui row from `docs/superpowers/specs/README.md`. Delete `docs/superpowers/specs/2026-10-28-torrent-picker-ui-design.md`. In `docs/TODO.md` `v2.7.2`, rewrite the bullet as implemented (no release stamp — that happens when the version is cut).

- [ ] **Step 5: Commit (proposed subject only)**

`docs: record picker two-confirms as decision 111`

---

## Self-review

**Spec coverage**

| Spec section | Task |
|---|---|
| Header (back, cover, kicker, title, meta, hint) | 3 |
| Nyaa search label, badge, Search button, empty no-op | 3 |
| Pack count including n=0/1 | 3 |
| Master/detail ratio, Download this in list footer | 3 |
| Detail overflow + table CSS | 3 |
| Magnet strip, Download magnet, no sticky footer | 3 |
| Two confirms / paste does not win | 3 (deletes the old test) |
| `media` on candidates payload | 2 |
| `pickerCache` may hold media | 3 |
| Sanitizer table tags | 1 |
| i18n keys | 3 |
| Go + frontend tests | 1, 2, 3 |
| Docs / #111 / spec retirement | 4 |

**Placeholders:** none.

**Type consistency:** `PickerMedia` (daemon) → `TorrentCandidateMedia` (API JSON) → `TorrentCandidateMedia` (TS). `WholeTitlePreview` is the only return of `PreviewWholeTitleTorrents`.
