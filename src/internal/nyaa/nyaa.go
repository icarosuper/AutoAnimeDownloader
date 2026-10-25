package nyaa

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"AutoAnimeDownloader/src/internal/logger"

	"github.com/PuerkitoBio/goquery"
)

// httpGet is an indirection for http.Get so tests can replace it.
var httpGet = http.Get

// TorrentResult is one torrent result from Nyaa
type TorrentResult struct {
	Name       string    `json:"name"`
	Date       time.Time `json:"date"`
	Seeders    string    `json:"seeders"`
	Leechers   int       `json:"leechers,omitempty"`
	MagnetLink string    `json:"magnetLink"`
	Episode    *int      `json:"episode,omitempty"`
	Resolution *string   `json:"resolution,omitempty"`
	Season     *int      `json:"season,omitempty"`
	Part       *int      `json:"part,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Fansub     string    `json:"fansub,omitempty"`
	IsBatch    bool      `json:"IsBatch,omitempty"`
	// UnlabeledPack marks a pack recognized only by having no episode number and a pack-sized
	// size ("[man] Azumanga Daioh [BD 1080p HEVC FLAC]"). Its range must come from the file list:
	// unlike a labeled pack, an unknown range never counts as "covers everything".
	UnlabeledPack bool `json:"-"`
	// DetailURL is the row's /view/<id> page. PackFileRange and FetchTorrentDetail consume it; the
	// anime and movie searches fill it.
	DetailURL string `json:"detailUrl,omitempty"`
}

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".webm": true, ".flv": true, ".wmv": true, ".ts": true, ".mpg": true,
	".mpeg": true, ".ogm": true,
}

// IsVideoFile reports whether the name has a video extension. It lives in this package, not in
// files, because both read the same file list of a pack — the Librarian after downloading,
// PackFileRange before — and files imports nyaa (the reverse would be a cycle).
func IsVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(name))]
}

func getNyaaBaseURL() string {
	if url := os.Getenv("NYAA_URL"); url != "" {
		return url
	}
	return "https://nyaa.si"
}

// MockNyaaHttpGet allows tests or callers to replace the httpGet function used by
// this package. It returns a function that when called will restore the
// previous httpGet implementation.
func MockNyaaHttpGet(fn func(string) (*http.Response, error)) (restore func()) {
	prev := httpGet
	if fn == nil {
		// If caller passes nil, do nothing and return a no-op restore
		return func() { httpGet = prev }
	}
	httpGet = fn
	return func() { httpGet = prev }
}

// GenerateSearchTitleVariants builds several search variants for a title, preferring clean versions
// (no special characters) over the originals. IMPORTANT: this function must be used ONLY to build
// search queries, never to modify torrent titles returned by Nyaa
func GenerateSearchTitleVariants(romaji, english string) []string {
	variants := []string{}
	seen := make(map[string]bool)

	// Helper that adds a variant if it wasn't added yet
	addVariant := func(title string) {
		if title == "" || seen[title] {
			return
		}
		seen[title] = true
		variants = append(variants, title)
	}

	// A sequel numbered in roman ("…Darou ka V: Houjou no Megami-hen") is released as "…Darou ka
	// S5 - 08": neither the numeral nor the subtitle is in the release name, so every variant
	// carrying them missed the live releases. The bare base title goes first; ExtractSeason
	// already reads the numeral as the season, and that filter keeps the other seasons out.
	if base, ok := romanSequelBase(romaji, english); ok {
		addVariant(base)
	}

	// Priority 1: clean romaji (no special characters)
	if romaji != "" {
		cleanRomaji := removeSpecialCharacters(romaji)
		addVariant(cleanRomaji)

		// Priority 2: original romaji
		addVariant(romaji)
	}

	// Priority 3: clean English (no special characters)
	if english != "" && english != romaji {
		cleanEnglish := removeSpecialCharacters(english)
		addVariant(cleanEnglish)

		// Priority 4: original English
		addVariant(english)
	}

	return variants
}

// romanSequelBase is the title without the roman numeral that closes it, and without the
// ": subtitle" after it ("…Darou ka V: Houjou no Megami-hen" → "…Darou ka"). It answers only when
// the English title carries the SAME numeral as a word, so a name that merely ends in one ("Lupin
// III", English "Lupin the 3rd") keeps its variants as they were.
func romanSequelBase(romaji, english string) (string, bool) {
	m := reRomanSeasonTitle.FindStringSubmatch(strings.TrimSpace(romaji))
	if m == nil {
		return "", false
	}
	for _, word := range strings.Fields(english) {
		if strings.TrimSuffix(word, ":") == m[2] {
			return m[1], true
		}
	}
	return "", false
}

// removeSpecialCharacters lowercases and keeps only letters, digits and spaces. It used to live in
// its own stringutil package, with this single function and this single caller.
func removeSpecialCharacters(s string) string {
	s = strings.ToLower(s)
	s = reSpecialChars.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// extractFansub extracts the fansub name from the torrent title. It looks for patterns like
// [FANSUB] or (FANSUB)
func extractFansub(name string) string {
	matches := reFansub.FindStringSubmatch(name)
	if len(matches) > 1 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}
	return ""
}

// IsBatch reports whether the torrent is a batch (multiple episodes)
func IsBatch(name string) bool {
	nameLower := strings.ToLower(name)
	for _, re := range reBatchPatterns {
		if re.MatchString(nameLower) {
			return true
		}
	}
	return false
}

// BatchInfo holds the information extracted from a batch torrent
type BatchInfo struct {
	StartEpisode int
	EndEpisode   int
	Season       int
}

// ExtractBatchInfo extracts the details of a batch. Consumed outside the package by the daemon: its
// range is what decides WHICH episodes get the pack's magnet (pickBatches).
func ExtractBatchInfo(torrentName string) BatchInfo {
	info := BatchInfo{}

	if start, last, ok := batchRange(torrentName); ok {
		info.StartEpisode, info.EndEpisode = start, last
	}

	if season := ExtractSeason(torrentName); season != nil {
		info.Season = *season
	}

	return info
}

// batchRange returns the first PLAUSIBLE range in the name, not the first match: reBatchRange
// matches any pair of numbers joined by "-" or "~", and many of them aren't episodes.
//
// FindAll (not FindString) because a rejected match doesn't end the search — "[EMBER] ...
// (2021-2022) ..." has the year BEFORE where a real range would be. It's non-overlapping, which
// keeps "Hibike! Euphonium 2 - 01 ~ 13" without a range: the "01" is consumed by the inverted match
// "2 - 01" and "01 ~ 13" is never seen. An unknown range falls into EndEpisode == 0, which the
// caller treats as coverage to resolve from the file list.
//
// Indices (not FindAllStringSubmatch) because the resolution guard needs the character RIGHT AFTER
// the match.
func batchRange(torrentName string) (int, int, bool) {
	for _, loc := range reBatchRange.FindAllStringSubmatchIndex(torrentName, -1) {
		// "[720-1080p]" is not a 361-episode range.
		if end := loc[1]; end < len(torrentName) && strings.ContainsRune("pPiI", rune(torrentName[end])) {
			continue
		}
		start, errStart := strconv.Atoi(torrentName[loc[2]:loc[3]])
		last, errEnd := strconv.Atoi(torrentName[loc[4]:loc[5]])
		if errStart != nil || errEnd != nil {
			continue
		}
		// An inverted range is not a range: in "Hibike! Euphonium 2 - 01 ~ 13" the match takes "2 - 01"
		// (season number + first episode) and yields 2..1, which covers no episode — the pack vanished
		// from pickBatches and a worse pack was downloaded instead.
		if last <= start {
			continue
		}
		// Release year: "[EMBER] ... (2021-2022) ..." became the range 2021..2022, which no axis
		// hypothesis covers — the complete pack was silently dropped. The band only catches years: no
		// series reaches episode 1900, and One Piece "1001-1100" passes.
		if start >= 1900 && last <= 2100 {
			continue
		}
		return start, last, true
	}
	return 0, 0, false
}

// PackFileRange returns the episode range a pack REALLY carries, read from the file list on the
// detail page (`/view/<id>`), in the numbering the group used in the files — the same ruler as the
// name, so the caller converts it with the same packAxis.
//
// It exists because the name doesn't answer: measured on 24 packs of three anime, 19 carry no range
// at all (sources.md, "Detail page"). Without this call, an unknown range counts as "covers
// everything" and the daemon records as downloaded episodes the pack doesn't have. The case that
// forced it: "[EMBER] … (Season 2 | Part 2) … (Batch)" declares no range and carries files 13..24 —
// ZERO intersection with the 1..12 the daemon recorded.
//
// (info, false) when the detail page doesn't answer, has no file list, or no video file gives a
// number: there's no range to assert, and the caller decides what to do with the "don't know".
//
// The range is min..max of the numbers read, so a pack whose files RESTART numbering per season
// (S01-S04, one folder each) gets the largest season's range instead of the total. MEASURED on
// 2026-08-30 and kept on purpose: it's 6% of packs, packAxis's delta 0 still picks all of them
// correctly, and a tie-break by file count per folder would count extras ("Creditless Opening 1" ->
// episode 1) as episodes. Numbers in decisions.md #84.
func PackFileRange(detailURL string) (BatchInfo, bool) {
	if detailURL == "" {
		return BatchInfo{}, false
	}
	doc, err := fetchNyaaPage(detailURL)
	if err != nil {
		logger.Logger.Debug().Err(err).Str("url", detailURL).Msg("Failed to fetch the torrent detail page")
		return BatchInfo{}, false
	}

	first, last, files := 0, 0, 0
	doc.Find(".torrent-file-list li").Each(func(_ int, s *goquery.Selection) {
		// A folder, not a file: its name usually carries the range of the whole release ("… 001-206
		// [4x3]"), which is exactly the number that doesn't count toward coverage.
		if s.Children().First().Is("a.folder") {
			return
		}
		name := fileListEntryName(s)
		if !IsVideoFile(name) {
			return
		}
		// An unreadable number stays out: that's how NCOP/NCED and extras stay outside the range instead
		// of widening it.
		episode := ExtractEpisodeNumber(name)
		if episode == nil {
			return
		}
		if files == 0 || *episode < first {
			first = *episode
		}
		if *episode > last {
			last = *episode
		}
		files++
	})

	if files == 0 {
		logger.Logger.Debug().Str("url", detailURL).Msg("Torrent detail page has no readable episode number")
		return BatchInfo{}, false
	}

	logger.Logger.Debug().
		Str("url", detailURL).
		Int("files", files).
		Int("first_episode", first).
		Int("last_episode", last).
		Msg("Pack range read from the torrent file list")

	return BatchInfo{StartEpisode: first, EndEpisode: last}, true
}

// detailURL makes the title column's href absolute ("/view/1323474"). Empty when the row has no
// link — without it PackFileRange has nothing to fetch and the range stays unknown.
func detailURL(href string) string {
	if !strings.HasPrefix(href, "/") {
		return ""
	}
	return getNyaaBaseURL() + href
}

// fileListEntryName is the file name in a list row, without the size that comes in the same <li>
// ("… - 001.mkv (259.9 MiB)") and without the children of a sublist.
func fileListEntryName(s *goquery.Selection) string {
	entry := s.Clone()
	entry.Find("ul").Remove()
	entry.Find("span.file-size").Remove()
	return strings.TrimSpace(entry.Text())
}

// isMovie reports whether the torrent is a movie. If isFormatMovie is true, it uses the AniList
// format (more reliable)
func isMovie(torrentName, animeName string, isFormatMovie bool) bool {
	if isFormatMovie {
		return true
	}

	if hasMovieMarker(torrentName) {
		return true
	}

	if !reHasEpisode.MatchString(torrentName) && !IsBatch(torrentName) {
		return true
	}

	return false
}

// hasMovieMarker reports whether the name carries an explicit movie, OVA/ONA or special marker.
// It's the part of isMovie that works as a guard in the episode search: isMovie's "has no episode
// marker" branch can't be used there because reHasEpisode is narrower than ExtractEpisodeNumber (it
// doesn't cover EP05, E05, [05], " 05 (" etc.) and would reject legitimate episodes.
func hasMovieMarker(torrentName string) bool {
	for _, re := range reMovieKeywords {
		if re.MatchString(torrentName) {
			return true
		}
	}
	return reOvaPattern.MatchString(torrentName) || reSpecialPattern.MatchString(torrentName)
}

// enoughCandidates is the floor of accepted candidates that makes the search stop going down pages.
// Nyaa returns results sorted by seeders desc, so past a handful of candidates the next page only
// brings worse options — 3 already gives the ranking a choice.
const enoughCandidates = 3

// maxSearchPages is the page ceiling per search (config max_search_pages), pushed by
// files.LoadConfigs. Same pattern as SetPriorities.
var maxSearchPages atomic.Int32

func init() { maxSearchPages.Store(5) }

// SetMaxSearchPages applies the page ceiling per search and returns a function that restores the
// previous value (the SetPriorities/MockNyaaHttpGet pattern). A value <= 0 counts as 1 page.
func SetMaxSearchPages(pages int) (restore func()) {
	prev := maxSearchPages.Load()
	maxSearchPages.Store(int32(pages))
	return func() { maxSearchPages.Store(prev) }
}

// ActiveMaxSearchPages returns the ceiling in use, never below 1 (pair of ActivePriorities).
func ActiveMaxSearchPages() int {
	if p := int(maxSearchPages.Load()); p > 1 {
		return p
	}
	return 1
}

// maxBatchSizeBytes is the pack size ceiling (config max_batch_torrent_size_gb) in BYTES, pushed by
// files.LoadConfigs like maxSearchPages. The daemon already drops packs above the ceiling in
// filterBySize, but dropping ONLY there is too late: enoughCandidates counts what the search
// accepted, so three giant packs on page 1 end the descent and the partial packs that would fit
// stay on the following pages, never seen (One Piece: the 587 GiB / 171 GiB packs stop the search
// before the 61 GiB / 20 GiB / 16 GiB ones). Applying the ceiling here keeps the counter honest —
// only packs the daemon can still use count.
//
// 0 turns it off, and is the default: with no config push (tests, the package used alone) the
// behavior is the old one.
var maxBatchSizeBytes atomic.Int64

// SetMaxBatchTorrentSizeGB applies the pack size ceiling and returns a function that restores the
// previous value (the SetMaxSearchPages pattern). A value <= 0 turns the filter off.
func SetMaxBatchTorrentSizeGB(gb float64) (restore func()) {
	prev := maxBatchSizeBytes.Load()
	if gb <= 0 {
		maxBatchSizeBytes.Store(0)
	} else {
		maxBatchSizeBytes.Store(int64(gb * 1024 * 1024 * 1024))
	}
	return func() { maxBatchSizeBytes.Store(prev) }
}

// batchTooBig reports whether the pack exceeds the ceiling in use. Size 0 (not parsed) passes, the
// same rule as daemon.filterBySize — a size that couldn't be read is no reason to drop it.
func batchTooBig(size int64) bool {
	max := maxBatchSizeBytes.Load()
	return max > 0 && size > 0 && size > max
}

// fetchSearchPages fetches page 1 of nyaaURL and goes on to the next ones WHILE there are rows and
// accepted() is below floor, up to ActiveMaxSearchPages().
//
// parse receives the page document and returns how many ROWS it had (not how many were accepted):
// an empty page means the query is over, and insisting would be a wasted fetch.
//
// Adaptive because page 2 used to be fetched ALWAYS: on a search already solved on page 1 — most of
// them — that was a wasted fetch, and that saving is what pays for the higher ceiling without
// raising the average traffic against nyaa.si.
//
// Sequential on purpose: each page is only requested because the previous one wasn't enough. See
// decisions.md #57 for why there's no parallel burst here.
//
// It only returns an error if PAGE 1 fails (without it there's no search); a failure on a later
// page silently ends the descent, which is the best-effort behavior page 2 already had.
func fetchSearchPages(nyaaURL string, floor int, accepted func() int, parse func(*goquery.Document) int) error {
	maxPages := ActiveMaxSearchPages()
	for page := 1; page <= maxPages; page++ {
		pageURL := nyaaURL
		if page > 1 {
			pageURL = fmt.Sprintf("%s&p=%d", nyaaURL, page)
		}

		doc, err := fetchNyaaPage(pageURL)
		if err != nil {
			if page == 1 {
				return err
			}
			return nil
		}

		rows := parse(doc)
		if rows == 0 || accepted() >= floor {
			return nil
		}
	}
	return nil
}

// parsePagesWith returns the parse for fetchSearchPages from each search's parseRow.
func parsePagesWith(parseRow func(int, *goquery.Selection)) func(*goquery.Document) int {
	return func(doc *goquery.Document) int {
		rows := doc.Find(".torrent-list tbody tr")
		rows.Each(parseRow)
		return rows.Length()
	}
}

// fetchNyaaPage fetches a single Nyaa results page and returns the parsed document.
func fetchNyaaPage(nyaaURL string) (*goquery.Document, error) {
	logger.Logger.Debug().Str("url", nyaaURL).Msg("Fetching Nyaa page")

	resp, err := httpGet(nyaaURL)
	if err != nil {
		logger.Logger.Debug().Err(err).Str("url", nyaaURL).Msg("Failed to fetch Nyaa page")
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Logger.Debug().Int("status_code", resp.StatusCode).Str("url", nyaaURL).Msg("Nyaa returned non-200 status")
		return nil, &nyaaHTTPError{Status: resp.StatusCode}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %v", err)
	}
	return doc, nil
}

// torrentSummaries formats each result as "name | S:412/L:3 | 1.4GiB | t=5 h=4.21", in the order
// given (i.e. already sorted by priority when logged after SortTorrentResults). t is the health
// band (what the health criterion compares); h is the raw score, informational only.
//
// The name alone isn't enough to audit "why did this one win": the fields that decide the sort
// (seeders, size, health) were left out and reproducing the choice meant repeating the search by
// hand. An unknown size (parse failed) shows as "?".
func torrentSummaries(results []TorrentResult) []string {
	summaries := make([]string, len(results))
	for i, r := range results {
		summaries[i] = fmt.Sprintf("%s | S:%d/L:%d | %s | t=%d h=%.2f",
			r.Name, ParseSeeders(r.Seeders), r.Leechers, formatSize(r.Size), healthTier(r), torrentHealthScore(r))
	}
	return summaries
}

// formatSize renders bytes for the log. For human reading only — nothing parses it back.
func formatSize(bytes int64) string {
	switch {
	case bytes <= 0:
		return "?"
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGiB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.0fMiB", float64(bytes)/(1<<20))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// deduplicateByMagnet removes duplicate TorrentResult entries by magnet link.
func deduplicateByMagnet(results []TorrentResult) []TorrentResult {
	seen := make(map[string]bool, len(results))
	unique := make([]TorrentResult, 0, len(results))
	for _, r := range results {
		if !seen[r.MagnetLink] {
			seen[r.MagnetLink] = true
			unique = append(unique, r)
		}
	}
	return unique
}

// longSeriesEpisodes is the episode total from which the episode search also tries the zero-padded
// query. See episodeQueries.
const longSeriesEpisodes = 100

// episodeQueries returns the episode's search queries: the plain one ("one piece 1") and, for a
// long series, also the one padded to 3 digits ("one piece 001").
//
// Without padding, `q=one piece 1` matches everything containing "1" and, sorted by seeders, the
// first few hundred rows are episodes 1160+ — episode 1 never shows up. Groups number long series
// with padding (`[Judas] One Piece 001-574`, `[Erai-raws] Naruto Shippuuden - 001 ~ 079`).
//
// It's an ADDITIONAL variant, not a replacement: padding is a convention, not a rule, and the plain
// query keeps catching those who number without it. It only kicks in above longSeriesEpisodes
// because each variant costs fetches, and an episode >= 100 already has 3 digits (padding would
// change nothing).
func episodeQueries(query string, episode, totalEpisodes int) []string {
	queries := []string{fmt.Sprintf("%s %d", query, episode)}
	if totalEpisodes > longSeriesEpisodes && episode < 100 {
		queries = append(queries, fmt.Sprintf("%s %03d", query, episode))
	}
	return queries
}

// ScrapNyaa searches Nyaa for torrents by anime name and episode. requestedSeason and requestedPart
// are extracted upstream from the AniList data. totalEpisodes (optional) is the anime's episode
// total, used only to decide the query's zero-padding — 0/missing counts as unknown.
func ScrapNyaa(animeName string, episode int, requestedSeason, requestedPart *int, totalEpisodes ...int) ([]TorrentResult, error) {
	total := 0
	if len(totalEpisodes) > 0 {
		total = totalEpisodes[0]
	}

	sanitizedRomajiName := reSeasonStrip.ReplaceAllString(animeName, "")
	sanitizedRomajiName = rePartStrip.ReplaceAllString(sanitizedRomajiName, "")

	query := strings.TrimSpace(sanitizedRomajiName)

	buildURL := func(q string) string {
		params := url.Values{}
		params.Set("f", "0")       // Filter: no filter
		params.Set("c", "1_2")     // Category: anime (english)
		params.Set("q", q)         // Search query with the episode
		params.Set("s", "seeders") // Sort by seeders
		params.Set("o", "desc")    // Descending order
		return fmt.Sprintf("%s/?%s", getNyaaBaseURL(), params.Encode())
	}

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		// Find all the cells (td) in the current row
		cells := s.Find("td")

		// Extract each cell's data by position. Prefer the link's visible text (name with spaces). Some
		// sites fill the title attribute with dots instead of spaces (the project's tests)
		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")

		// Extract the size (cells.Eq(3) on Nyaa)
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)

		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := ParseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		if name != "" {
			logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")
		}

		// Check whether the torrent must be ignored (dub, raw, hardcoded, etc.)
		if ShouldIgnore(name) {
			return
		}

		// Check whether it's a batch - ignore it for a single-episode search
		if IsBatch(name) {
			return
		}

		// A movie/OVA/special is not an episode: "Naruto Shippuuden Movie 3" matches
		// ExtractEpisodeNumber's " 3 (" pattern and would pass as episode 3.
		if hasMovieMarker(name) {
			return
		}

		// Extract information from the torrent name
		var animeEpisode *int
		var season *int
		var resolution *string
		var fansub string

		if name != "" {
			animeEpisode = ExtractEpisodeNumber(name)
			season = ExtractSeason(name)
			res := ExtractResolution(name)
			resolution = &res
			fansub = extractFansub(name)
		}

		// Filter by base title (make sure the torrent belongs to the anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Filter by season
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else {
			if season != nil && *season != 1 {
				return
			}
		}

		// Filter by part (hard filter: rejects a torrent with no marker or the wrong part)
		part := ExtractPart(name)
		if requestedPart != nil {
			if part == nil || *part != *requestedPart {
				return
			}
		}

		// Requires an exact episode match
		if animeEpisode == nil || *animeEpisode != episode {
			return
		}

		// Add the result to the array
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Part:       part,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
		})
	}

	var searched bool
	var firstErr error
	for _, q := range episodeQueries(query, episode, total) {
		nyaaURL := buildURL(q)

		logger.Logger.Debug().
			Str("url", nyaaURL).
			Str("anime_name", animeName).
			Int("episode", episode).
			Msg("Searching Nyaa for single episode")

		err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		searched = true
	}
	// Only fails if NO variant answered: a query that errors can't cancel the other.
	if !searched {
		return nil, firstErr
	}

	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("episode", episode).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa results for single episode")

	if len(results) == 0 {
		return nil, nil // No result found
	}

	// Sort results by quality and fansub
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
}

// minUnlabeledPackBytes is the smallest torrent without an episode number that is read as an
// unlabeled pack. Below it the row is far more likely a special, a recap or a short ONA than a
// season ("[Obscurerips] Azumanga Web Daioh" is 19 MiB).
const minUnlabeledPackBytes = 1 << 30

// ScrapNyaaForAnime fetches the anime's results page ONCE and returns packs and episodes in the
// SAME list. The two separate searches this function replaces built the same URL
// (?f=0&c=1_2&q=<title>&s=seeders&o=desc) and disagreed only on the row filter, which cost two
// descents of up to max_search_pages over the same HTML, per anime, per cycle.
//
// A pack row comes in with IsBatch: true; an episode row comes in with Episode != nil. The daemon
// partitions them (partitionSearchResults), which is where the pack and episode size ceilings
// differ.
//
// It returns ONE list on purpose: searchNyaaWithVariants stops at the first title variant with a
// result, and "result" is a non-empty slice — a pair signature would require generalizing it.
func ScrapNyaaForAnime(animeName string, episodes []int, requestedSeason, requestedPart *int) ([]TorrentResult, error) {
	query := strings.TrimSpace(extractSeasonFromName(animeName))

	params := url.Values{}
	params.Set("f", "0")   // Filter: no filter
	params.Set("c", "1_2") // Category: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders")
	params.Set("o", "desc")

	nyaaURL := fmt.Sprintf("%s/?%s", getNyaaBaseURL(), params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Int("episodes_count", len(episodes)).
		Msg("Searching Nyaa for anime (packs + episodes)")

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		if name == "" {
			return
		}
		logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")

		if ShouldIgnore(name) {
			return
		}
		// Filter by base title (make sure the torrent belongs to the anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		season := ExtractSeason(name)
		part := ExtractPart(name)
		res := ExtractResolution(name)

		row := TorrentResult{
			Name:       name,
			Seeders:    strings.TrimSpace(cells.Eq(5).Text()),
			Leechers:   ParseSeeders(strings.TrimSpace(cells.Eq(6).Text())),
			MagnetLink: cells.Eq(2).Find("a").Eq(1).AttrOr("href", ""),
			Season:     season,
			Part:       part,
			Resolution: &res,
			Size:       parseSize(strings.TrimSpace(cells.Eq(3).Text())),
			Fansub:     extractFansub(name),
			DetailURL:  detailURL(cells.Eq(1).Find("a").Not(".comments").AttrOr("href", "")),
		}

		// A whole-series release often names nothing but the title and the source — old anime
		// especially. Without this it fell through to the episode path, had no number, and the
		// anime was left with dead loose episodes while live packs sat on the same page.
		unlabeled := !IsBatch(name) && !hasMovieMarker(name) && ExtractEpisodeNumber(name) == nil && row.Size >= minUnlabeledPackBytes
		if IsBatch(name) || unlabeled {
			// Season on the pack path: with no explicit request, a pack of any season will do. That was the
			// old batch search's behavior, and it differs from the episode path right below — which is why
			// the two filters can't be merged. A first-season entry still drops later-season packs, in the
			// daemon (partitionSearchResults), where the relations are known.
			if requestedSeason != nil && (season == nil || *season != *requestedSeason) {
				return
			}
			// Part on the pack path: only a declared CONFLICT is rejected. A pack without a part marker
			// isn't the wrong pack — it's the normal release format, because the pack is the whole season
			// and it's AniList that splits the season into cours (sources.md, "Pack granularity and
			// numbering"). Requiring the marker zeroed the packs of every "Part N" entry.
			//
			// From there on, the range's coverage decides, in the daemon (packAxis). A pack that ALSO has no
			// range in its name has no number to check: its range comes from the file list on the detail
			// page (PackFileRange), and that's what tells the whole-season pack from the Part 1-only pack
			// (decisions.md #84).
			if requestedPart != nil && part != nil && *part != *requestedPart {
				return
			}
			// A pack above the ceiling goes here and not in the daemon's filterBySize: accepting it now
			// would count toward enoughCandidates and end the descent before the partial packs that fit (see
			// maxBatchSizeBytes).
			if batchTooBig(row.Size) {
				logger.Logger.Debug().
					Str("torrent", name).
					Int64("size_bytes", row.Size).
					Msg("Batch above max_batch_torrent_size_gb, skipping row")
				return
			}
			row.IsBatch = true
			row.UnlabeledPack = unlabeled
			results = append(results, row)
			return
		}

		// A movie/OVA/special is not an episode: "Naruto Shippuuden Movie 3" matches
		// ExtractEpisodeNumber's " 3 (" pattern and would pass as episode 3.
		if hasMovieMarker(name) {
			return
		}
		// Season on the episode path: with no explicit request, only season 1 (or no marker) passes.
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else if season != nil && *season != 1 {
			return
		}
		if requestedPart != nil && (part == nil || *part != *requestedPart) {
			return
		}

		episode := ExtractEpisodeNumber(name)
		if episode == nil || !slices.Contains(episodes, *episode) {
			return
		}
		row.Episode = episode
		results = append(results, row)
	}

	// The pagination floor counts both lists together, so a pack the daemon will drop for size can't
	// count — that's why the pack ceiling arrives here by push (maxBatchSizeBytes), the same way as
	// max_search_pages. The seeders filter stays only in the daemon: it doesn't truncate the search
	// the same way, because Nyaa already returns results sorted by seeders desc.
	if err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow)); err != nil {
		return nil, err
	}
	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa results for anime")

	if len(results) == 0 {
		return nil, nil
	}

	// Sort the mixed list: partitioning afterwards keeps the relative order, so each list comes out
	// correctly sorted.
	return SortTorrentResults(results), nil
}

// ScrapNyaaForMovie searches movie torrents. It detects movies automatically and sorts by technical
// quality. isFormatMovie says whether AniList classifies it as a movie (format = MOVIE)
func ScrapNyaaForMovie(animeName string, isFormatMovie ...bool) ([]TorrentResult, error) {
	// If the optional parameter was passed, use it; otherwise assume false
	isMovieFormat := false
	if len(isFormatMovie) > 0 {
		isMovieFormat = isFormatMovie[0]
	}
	// Strip the season from the name if present (movies usually don't have one)
	sanitizedName := extractSeasonFromName(animeName)
	query := strings.TrimSpace(sanitizedName)

	// Build the URL with parameters
	params := url.Values{}
	params.Set("f", "0")   // Filter: no filter
	params.Set("c", "1_2") // Category: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders")
	params.Set("o", "desc")

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Msg("Searching Nyaa for movie")

	var results []TorrentResult

	// Parse the rows of the torrent table
	parseRow := func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)
		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := ParseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		if name != "" {
			logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")
		}

		// Check whether the torrent must be ignored
		if ShouldIgnore(name) {
			return
		}

		// Check whether it's a movie
		if !isMovie(name, animeName, isMovieFormat) {
			return
		}

		// Extract information
		res := ExtractResolution(name)
		resolution := &res
		fansub := extractFansub(name)

		// Filter by base title
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Add the result
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
			IsBatch:    false,
			DetailURL:  detailURL(cells.Eq(1).Find("a").Not(".comments").AttrOr("href", "")),
		})
	}

	if err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow)); err != nil {
		return nil, err
	}

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa movie results")

	if len(results) == 0 {
		return nil, nil // No movie found
	}

	// Sort the results using the movie-specific ordering
	sortedResults := SortMovieResults(results)
	return sortedResults, nil
}

// ExtractEpisodeNumber extracts the episode number from the torrent name, trying the patterns in
// priority order (most to least specific). The underscore becomes a space before anything else:
// every pattern in reEpisodePatterns wants a space around the number, and a group that separates
// with "_" ("[DB]Vinland Saga_-_01_(...).mkv") matched none. It applies to torrent names and, above
// all, to FILE names inside a pack — this is where the Librarian gets the number to rename to the
// Jellyfin pattern.
func ExtractEpisodeNumber(name string) *int {
	name = strings.ReplaceAll(name, "_", " ")
	for _, p := range reEpisodePatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if episodeNum, err := strconv.Atoi(matches[1]); err == nil {
				if episodeNum > 0 && episodeNum < 10000 {
					return &episodeNum
				}
			}
		}
	}
	return nil
}

// romanSeasonValues converts the roman numerals matched by reRomanSeason to their
// integer season number.
var romanSeasonValues = map[string]int{
	"II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
}

// ExtractSeason extracts the season number from the torrent name, trying the patterns in priority
// order
func ExtractSeason(name string) *int {
	for _, p := range reSeasonPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if seasonNum, err := strconv.Atoi(matches[1]); err == nil {
				if seasonNum > 0 && seasonNum < 100 {
					return &seasonNum
				}
			}
		}
	}
	if matches := reRomanSeason.FindStringSubmatch(name); len(matches) > 1 {
		if seasonNum, ok := romanSeasonValues[matches[1]]; ok {
			return &seasonNum
		}
	}
	return nil
}

// ExtractPart extracts the part/cour number from the torrent name or the AniList title, trying the
// patterns in priority order (most specific first).
//
// A name that declares MORE THAN ONE part has no part: "(Part 1 + Part 2)" and "(Season 4 Part
// 03+04)" cover both halves, and returning the first match made the pack read as "part 1's" —
// precisely the pack that serves both. Without a single number the answer is nil, and the range's
// coverage decides (see daemon.packAxis and sources.md, item 4).
func ExtractPart(name string) *int {
	if declared := declaredParts(name); len(declared) > 1 {
		return nil
	}

	for _, p := range rePartPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if partNum, err := strconv.Atoi(matches[1]); err == nil {
				if partNum > 0 && partNum < 100 {
					return &partNum
				}
			}
		}
	}
	return nil
}

// declaredParts are the DISTINCT part/cour numbers the name declares, without leading zeros.
func declaredParts(name string) map[int]bool {
	declared := make(map[int]bool, 2)
	for _, m := range reAnyPart.FindAllStringSubmatch(name, -1) {
		for _, group := range m[1:] {
			if n, err := strconv.Atoi(group); err == nil {
				declared[n] = true
			}
		}
	}
	return declared
}

// canonicalResolutions maps resolution aliases to the canonical tokens of the Priorities lists.
// Without it "4k" and "2160p" are the same resolution taking two different indexes of the list, and
// which one wins depends only on how the group wrote the torrent name. "8k" is left out on purpose:
// it keeps being returned raw and, not being in the default list, ranks as the worst — the right
// treatment for a resolution that doesn't exist in anime.
var canonicalResolutions = map[string]string{"4k": "2160p", "uhd": "2160p", "fhd": "1080p", "hd": "720p"}

// ExtractResolution extracts the resolution from the torrent name, already canonicalized (same
// contract as ExtractCodec, which returns "HEVC" for "x265").
func ExtractResolution(name string) string {
	for _, p := range reResolutionPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			res := strings.ToLower(matches[1])
			if canon, ok := canonicalResolutions[res]; ok {
				return canon
			}
			// "1920x1080" -> "1080p": the height is what names the resolution.
			if _, height, ok := strings.Cut(res, "x"); ok {
				return height + "p"
			}
			return res
		}
	}
	return ""
}

// parseNyaaDate parses Nyaa's date string into a time.Time
func parseNyaaDate(dateString string) (time.Time, error) {
	// Nyaa dates are typically in the "2025-07-06 14:49" format. Try different date formats
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04",
		"2006/01/02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateString); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse the date: %s", dateString)
}

// isWithinThreeMonths reports whether the date is within the last 3 months
func isWithinThreeMonths(date time.Time) bool {
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	return date.After(threeMonthsAgo)
}

// resolutionPriority returns a priority value for the resolution (lower = better). Based on Nyaa's
// rules (Section 3 of the rules document)
func resolutionPriority(resolution string) int {
	return priorityIndex(ActivePriorities().Resolutions, resolution)
}

// isUncensored reports whether the torrent has "Uncensored" in the title
func isUncensored(torrentName string) bool {
	return strings.Contains(strings.ToLower(torrentName), "uncensored")
}

// fansubPriority returns a priority value for the fansub (lower = better). Based on Nyaa's rules
// (Section 4 of the rules document)
func fansubPriority(torrentName string) int {
	nameLower := strings.ToLower(torrentName)
	fansubs := ActivePriorities().Fansubs
	best := len(fansubs)
	for i, f := range fansubs {
		if strings.Contains(nameLower, f) && i < best {
			best = i
		}
	}
	return best
}

// ExtractSource extracts the release source (BD, WEB-DL, TV, etc.)
func ExtractSource(name string) string {
	nameLower := strings.ToLower(name)
	for _, sp := range reSourcePatterns {
		if sp.re.MatchString(nameLower) {
			return sp.source
		}
	}
	return ""
}

// ExtractCodec extracts the video codec (HEVC, H.264, AV1, etc.)
func ExtractCodec(name string) string {
	nameLower := strings.ToLower(name)
	for _, cp := range reCodecPatterns {
		if cp.re.MatchString(nameLower) {
			return cp.codec
		}
	}
	return ""
}

// extractAudio extracts the audio codec (FLAC, DDP, AAC, etc.)
func extractAudio(name string) string {
	nameLower := strings.ToLower(name)
	for _, ap := range reAudioPatterns {
		if ap.re.MatchString(nameLower) {
			return ap.audio
		}
	}
	return ""
}

// sourcePriority returns the source priority (lower = better)
func sourcePriority(source string) int {
	return priorityIndex(ActivePriorities().Sources, source)
}

// codecPriority returns the codec priority (lower = better)
func codecPriority(codec string) int {
	return priorityIndex(ActivePriorities().Codecs, codec)
}

// audioPriority returns the audio priority (lower = better)
func audioPriority(audio string) int {
	return priorityIndex(ActivePriorities().Audio, audio)
}

// SortTorrentResults sorts torrents by quality, applying the CriteriaOrder criteria that apply to
// episodes (episodeCriteria): uncensored, resolution, codec, health, fansub and size. Source stays
// out because a weekly episode is WEB-DL in almost every release; finished series go through
// SortFinishedSeriesResults. Based on Nyaa's rules (Section 8 of the rules document)
func SortTorrentResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, filterCriteria(ActivePriorities().CriteriaOrder, episodeCriteria))
}

// ParseSeeders converts the seeders string to int (Nyaa's column can come as "-"). Consumed outside
// the package by the seeders floor in daemon.filterBySeeders.
func ParseSeeders(seedersStr string) int {
	seedersStr = strings.TrimSpace(seedersStr)
	if seedersStr == "" || seedersStr == "-" {
		return 0
	}
	seedersStr = strings.ReplaceAll(seedersStr, ",", "")
	if seeders, err := strconv.Atoi(seedersStr); err == nil {
		return seeders
	}
	return 0
}

// torrentHealthScore returns a health value that combines seeder count and
// seeder/leecher ratio. Higher = healthier swarm.
// Uses log scale so a torrent with 200 seeders isn't infinitely better than one with 100.
func torrentHealthScore(r TorrentResult) float64 {
	seeders := ParseSeeders(r.Seeders)
	if seeders == 0 {
		return 0.0
	}
	ratio := float64(seeders) / float64(seeders+r.Leechers+1)
	return ratio * math.Log(float64(seeders)+1)
}

// parseSize converts the size string to bytes. Common formats: "1.5 GiB", "500 MiB", "2.3 GiB",
// etc.
func parseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" || sizeStr == "-" {
		return 0
	}

	matches := reParseSizeRe.FindStringSubmatch(sizeStr)

	if len(matches) != 3 {
		return 0
	}

	// Convert the number to float
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	// Convert to bytes according to the unit
	unit := strings.ToUpper(matches[2])
	var multiplier int64 = 1

	switch unit {
	case "KB", "KIB":
		multiplier = 1024
	case "MB", "MIB":
		multiplier = 1024 * 1024
	case "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(value * float64(multiplier))
}

// extractSeasonFromName strips season information from the name for the base query
func extractSeasonFromName(name string) string {
	result := name
	for _, re := range reSeasonNamePatterns {
		result = re.ReplaceAllString(result, "")
	}
	return strings.TrimSpace(result)
}

// SortMovieResults sorts movie search results by quality. Uses CriteriaOrder as-is
// (every criterion, including source and audio).
func SortMovieResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, ActivePriorities().CriteriaOrder)
}
