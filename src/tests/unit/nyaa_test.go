package unit

import (
	"AutoAnimeDownloader/src/internal/nyaa"

	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

type testOptions struct {
	animeName string
	episode   int
	season    *int
	part      *int
	correct   []string
	incorrect []string
}

func getRow(name string) string {
	parsedName := strings.ReplaceAll(name, " ", ".")

	return `<tr>
	  <td></td>
	  <td><a href="/view/1" title="` + parsedName + `">` + name + `</a></td>
	  <td><a></a><a href="magnet:?xt=urn:btih:` + parsedName + `-magnet">magnet</a></td>
	  <td></td>
	  <td>` + time.Now().Format("2006-01-02 15:04") + `</td>
	  <td>100</td>
	</tr>`
}

func mockHtml(options []string) string {
	var rows []string
	for _, opt := range options {
		rows = append(rows, getRow(opt))
	}

	return `<!doctype html>
	<html>
	<body>
	<table class="torrent-list">
	<tbody>
		` + strings.Join(rows, "\n") + `
	</tbody>
	</table>
	</body>
	</html>`
}

func mockHttpGet(sampleHTML string) func() {
	return nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		r := strings.NewReader(sampleHTML)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(r),
			Header:     make(http.Header),
		}, nil
	})
}

// mockHttpGetCapturing is mockHttpGet that also records each requested URL, for the tests that
// check the built query and not the result.
func mockHttpGetCapturing(sampleHTML string, urls *[]string) func() {
	return nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		*urls = append(*urls, url)
		r := strings.NewReader(sampleHTML)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(r),
			Header:     make(http.Header),
		}, nil
	})
}

func TestNyaaModule_CanGetMagnet(t *testing.T) {
	html := mockHtml([]string{"My.Show.S01E02.1080p"})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaa("My.Show", 2, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	r := results[0]
	if r.Name != "My.Show.S01E02.1080p" {
		t.Fatalf("unexpected name: %s", r.Name)
	}
	if r.MagnetLink == "" {
		t.Fatalf("expected magnet link")
	}
	if r.Episode == nil || *r.Episode != 2 {
		t.Fatalf("expected episode 2, got %v", r.Episode)
	}
}

func TestNyaaModule_CanGetCorrectAnime_WithSpecificEpisode(t *testing.T) {
	options := testOptions{
		animeName: "Kemono Friends",
		episode:   5,
		correct: []string{
			"Kemono Friends Episode 5",
		},
		incorrect: []string{
			"Kemono Jihen Episode 5",
			"Manaria Friends Episode 5",
		},
	}

	runEpisodeNameTest(options, t)
}

func TestNyaaModule_CanGetCorrectAnime_WithSpecialCharactersDifferences(t *testing.T) {
	options := testOptions{
		animeName: "Silent Witch: Chinmoku no Majo no Kakushigoto",
		episode:   3,
		correct: []string{
			"[SubsPlease] Silent Witch - Chinmoku no Majo no Kakushigoto - 03v2 (1080p)",
			"Silent Witch - Chinmoku no Majo no Kakushigoto Episode 3",
			"Silent Witch - Chinmoku no Majo no Kakushigoto - 03",
		},
		incorrect: []string{
			"Silent Witch - Chinmoku no Majo no Kakushigoto - 02",
			"Silent Witch - Chinmoku no Majo no Kakushigoto - 04",
			"Silent Witch Episode 3",
			"Chinmoku no Majo no Kakushigoto Episode 3",
		},
	}

	runEpisodeNameTest(options, t)
}

// mockHttpGetPerURL serves URL-dependent HTML and records each requested URL — it's the mock of the
// pagination tests, which check how many pages the search fetched.
func mockHttpGetPerURL(html func(url string) string, urls *[]string) func() {
	return nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
		*urls = append(*urls, url)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(html(url))),
			Header:     make(http.Header),
		}, nil
	})
}

// Page 1 with plenty of candidates: page 2 must not be requested (it used to be requested ALWAYS).
func TestScrapNyaa_StopsOnFirstPageWhenEnoughCandidates(t *testing.T) {
	defer nyaa.SetMaxSearchPages(5)()

	page := mockHtml([]string{
		"Show - 05 [1080p]",
		"Show - 05 [720p]",
		"Show - 05 [480p]",
	})
	var urls []string
	defer mockHttpGetPerURL(func(string) string { return page }, &urls)()

	results, err := nyaa.ScrapNyaa("Show", 5, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 candidates, got %d", len(results))
	}
	if len(urls) != 1 {
		t.Fatalf("want 1 fetch (page 1 only), got %d: %v", len(urls), urls)
	}
}

// Candidates below the floor and pages always full: goes down to the ceiling and stops there.
func TestScrapNyaa_PagesUntilCapWhenCandidatesAreThin(t *testing.T) {
	defer nyaa.SetMaxSearchPages(3)()

	// Rows from another anime: the page never gets empty, but nothing is accepted.
	page := mockHtml([]string{"Kemono Jihen - 05 [1080p]", "Manaria Friends - 05 [1080p]"})
	var urls []string
	defer mockHttpGetPerURL(func(string) string { return page }, &urls)()

	results, err := nyaa.ScrapNyaa("Show", 5, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if results != nil {
		t.Fatalf("want no candidate, got %v", results)
	}
	if len(urls) != 3 {
		t.Fatalf("want 3 fetches (ceiling), got %d: %v", len(urls), urls)
	}
	if !strings.Contains(urls[2], "&p=3") {
		t.Fatalf("the last fetched page should be 3, got %s", urls[2])
	}
}

// Empty page = the query is over: insisting up to the ceiling would be a wasted fetch.
func TestScrapNyaa_StopsWhenPageIsEmpty(t *testing.T) {
	defer nyaa.SetMaxSearchPages(5)()

	var urls []string
	defer mockHttpGetPerURL(func(url string) string {
		if strings.Contains(url, "&p=") {
			return mockHtml(nil)
		}
		return mockHtml([]string{"Show - 05 [1080p]"}) // 1 candidate, below the floor
	}, &urls)()

	results, err := nyaa.ScrapNyaa("Show", 5, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(results))
	}
	if len(urls) != 2 {
		t.Fatalf("want 2 fetches (page 1 + the empty one), got %d: %v", len(urls), urls)
	}
}

// Batch read only page 1; now it paginates like the episode search.
func TestScrapNyaaForAnime_PaginatesLikeEpisodeSearch(t *testing.T) {
	defer nyaa.SetMaxSearchPages(2)()

	page := mockHtml([]string{"Kemono Jihen (01-12) [Batch] [1080p]"})
	var urls []string
	defer mockHttpGetPerURL(func(string) string { return page }, &urls)()

	if _, err := nyaa.ScrapNyaaForAnime("Show", nil, nil, nil); err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("want 2 fetches (ceiling), got %d: %v", len(urls), urls)
	}
}

// Long series: groups number with zero-padding ("One Piece - 001"), and the unpadded query drowns
// episode 1 among the 1160+ sorted by seeders.
func TestScrapNyaa_LongSeriesAlsoQueriesPaddedEpisode(t *testing.T) {
	var urls []string
	restore := mockHttpGetCapturing(mockHtml([]string{"[Group] One Piece - 001 [1080p]"}), &urls)
	defer restore()

	if _, err := nyaa.ScrapNyaa("One Piece", 1, nil, nil, 1100); err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	if !slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "q=One+Piece+001") }) {
		t.Fatalf("want a query with the zero-padded episode, got %v", urls)
	}
	if !slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "q=One+Piece+1&") }) {
		t.Fatalf("the unpadded query must not be replaced, got %v", urls)
	}
}

func TestScrapNyaa_ShortSeriesKeepsPlainEpisode(t *testing.T) {
	var urls []string
	restore := mockHttpGetCapturing(mockHtml([]string{"[Group] Takopi - 01 [1080p]"}), &urls)
	defer restore()

	if _, err := nyaa.ScrapNyaa("Takopi", 1, nil, nil, 12); err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	if slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "001") }) {
		t.Fatalf("a short anime must not build a padded query, got %v", urls)
	}
}

// Without the episode total (old call) the behavior is the short anime one.
func TestScrapNyaa_UnknownTotalKeepsPlainEpisode(t *testing.T) {
	var urls []string
	restore := mockHttpGetCapturing(mockHtml([]string{"[Group] Show - 01 [1080p]"}), &urls)
	defer restore()

	if _, err := nyaa.ScrapNyaa("Show", 1, nil, nil); err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	if slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "001") }) {
		t.Fatalf("without a known total there must be no padding, got %v", urls)
	}
}

// A movie must not be accepted as an episode: "[DB] Naruto Shippuuden Movie 3" matched
// extractEpisodeNumber's " 3 (" pattern and became "episode 3".
func TestNyaaModule_IgnoresMovieOnEpisodeSearch(t *testing.T) {
	options := testOptions{
		animeName: "Naruto Shippuuden",
		episode:   3,
		correct: []string{
			"[Erai-raws] Naruto Shippuuden - 03 [1080p]",
		},
		incorrect: []string{
			"[DB] Naruto Shippuuden Movie 3 (Eng Sub).avi",
			"[Group] Naruto Shippuuden OVA 3 (1080p)",
			"[Group] Naruto Shippuuden Special 3 (1080p)",
		},
	}

	runEpisodeNameTest(options, t)
}

// "Persona" contains "ona": the OVA marker needs a word boundary, otherwise the movie guard rejects
// legitimate episodes.
func TestNyaaModule_OvaMarkerDoesNotMatchTitleSubstring(t *testing.T) {
	options := testOptions{
		animeName: "Persona Trinity Soul",
		episode:   5,
		correct: []string{
			"[Group] Persona Trinity Soul - 05 [1080p]",
		},
		incorrect: []string{
			"[Group] Persona Trinity Soul - 06 [1080p]",
		},
	}

	runEpisodeNameTest(options, t)
}

func TestNyaaModule_CanGetEpisode_OfCompleteAnime(t *testing.T) {
	options := testOptions{
		animeName: "Lucky Star",
		episode:   15,
		correct: []string{
			"Lucky Star Episode 15",
			"Lucky Star EP015",
			"Lucky Star E015",
			"Lucky Star - 15",
			"Lucky Star 15",
			"Lucky Star S01E15",
			"Lucky Star 1st Season - 15",
			"Lucky Star 1st Season E015",
		},
		incorrect: []string{
			"Lucky Star Episode 16",
			"Lucky Star Episode 05",
			"[Moe] Lucky☆Star 01-24+OVA [BD] [1080p FLAC]",
			"[DBD-Raws][幸运星/Lucky Star][01-24TV全集+OVA+SP][1080P][BDRip][HEVC-10bit][日英双语][简繁外挂][FLACx2][MKV](らき☆すた)",
			"[Tsundere] Lucky Star [BDRip h264 1280x720 FLAC]",
			"Lucky Star (2007) [Doki][1280x720 Hi10P BD FLAC]",
			"Lucky Star E01-24[Virus123][Bluray][1080p][Dual Audio]",
		},
	}

	runEpisodeNameTest(options, t)
}

func TestNyaaModule_CanGetCorrectMagnets_OfSeason1Anime(t *testing.T) {
	options := testOptions{
		animeName: "Kemono Friends",
		episode:   5,
		correct: []string{
			"Kemono Friends Episode 5",
			"Kemono Friends EP05",
			"Kemono Friends E05",
			"Kemono Friends - 5",
			"Kemono Friends 5",
			"Kemono Friends S01E05",
			"Kemono Friends 1st Season - 05",
			"Kemono Friends 1st Season E05",
		},
		incorrect: []string{
			"Kemono Friends Episode 6",
			"Kemono Friends Season 2 Episode 5",
			"Kemono Friends S01E03",
			"Kemono Friends S02E05",
			"Kemono Friends 2nd Season - 05",
			"Kemono Friends Season 3 Episode 5",
		},
	}

	runEpisodeNameTest(options, t)
}

func TestNyaaModule_CanGetCorrectMagnets_OfSeason2Anime(t *testing.T) {
	season2 := 2
	options := testOptions{
		animeName: "Machikado Mazoku Season 2",
		episode:   7,
		season:    &season2,
		correct: []string{
			"Machikado Mazoku Season 2 Episode 7",
			"Machikado Mazoku Season 2 Episode 07",
			"Machikado Mazoku Season 2 EP07",
			"Machikado Mazoku Season 2 E07",
			"Machikado Mazoku Season 2 - 7",
			"Machikado Mazoku S2 - 7",
			"Machikado Mazoku S02E07",
			"Machikado Mazoku 2nd Season - 07",
			"Machikado Mazoku 2 - 07",
			"Machikado Mazoku 2 EP07",
		},
		incorrect: []string{
			"Machikado Mazoku Episode 07",
			"Machikado Mazoku Season 3 Episode 7",
			"Machikado Mazoku S02E03",
			"Machikado Mazoku 2nd Season - 03",
			"Machikado Mazoku Season 2 Episode 17",
			"Different Mazoku S03E07",
		},
	}

	runEpisodeNameTest(options, t)
}

func TestNyaaModule_CanGetCorrectMagnets_OfSeason3Anime(t *testing.T) {
	season3 := 3
	options := testOptions{
		animeName: "SPY x FAMILY Season 3",
		episode:   3,
		season:    &season3,
		correct: []string{
			"SPY x FAMILY Season 3 Episode 3",
			"SPY x FAMILY Season 3 EP03",
			"SPY x FAMILY Season 3 E03",
			"SPY x FAMILY Season 3 - 3",
			"SPY x FAMILY S03E03",
			"SPY x FAMILY 3rd Season - 03",
		},
		incorrect: []string{
			"SPY x FAMILY Episode 3",
			"SPY x FAMILY Season 2 Episode 3",
			"SPY x FAMILY S02E03",
			"SPY x FAMILY 2nd Season - 03",
			"SPY x FAMILY Season 3 Episode 13",
		},
	}

	runEpisodeNameTest(options, t)
}

// Long anime (One Piece) have 4-digit episodes.
func TestNyaaModule_CanGetFourDigitEpisode(t *testing.T) {
	runEpisodeNameTest(testOptions{
		animeName: "One Piece",
		episode:   1123,
		correct: []string{
			"[SubsPlease] One Piece - 1123 (1080p) [A6D8A84C].mkv",
			"[Erai-raws] One Piece - 1123 [1080p CR WEB-DL AVC EAC3][MultiSub][7C62698E]",
			"[ToonsHub] One Piece EP1123 1080p CR WEB-DL AAC2.0 H.264 (Multi-Subs)",
			"ONE PIECE S01E1123 1080p NF WEB-DL AAC2.0 H 264-VARYG (Multi-Subs)",
		},
		incorrect: []string{
			"[SubsPlease] One Piece - 1124 (1080p) [5C59247E].mkv",
			"[HatSubs] One Piece 1123-1133 (WEB 1080p)",
		},
	}, t)
}

func runEpisodeNameTest(opt testOptions, t *testing.T) {
	html := mockHtml(append(opt.correct, opt.incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaa(opt.animeName, opt.episode, opt.season, opt.part)

	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("no results found")
	}

	foundCorrect := make(map[string]bool)
	for _, r := range results {
		for _, opt := range opt.incorrect {
			if r.Name == opt {
				t.Fatalf("found incorrect torrent in results: %+v", r)
			}
		}

		for _, opt := range opt.correct {
			if r.Name == opt {
				foundCorrect[opt] = true
			}
		}
	}

	if len(foundCorrect) != len(opt.correct) {
		notFound := []string{}
		for _, opt := range opt.correct {
			if !foundCorrect[opt] {
				notFound = append(notFound, opt)
			}
		}

		t.Fatalf("not all correct torrents found, missing: %v", strings.Join(notFound, ", "))
	}
}

func TestSortTorrentResults_ByResolution(t *testing.T) {
	resolution720p := "720p"
	resolution1080p := "1080p"
	resolution480p := "480p"

	results := []nyaa.TorrentResult{
		{Name: "Anime Episode 1 720p", Resolution: &resolution720p},
		{Name: "Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "Anime Episode 1 480p", Resolution: &resolution480p},
	}

	sorted := nyaa.SortTorrentResults(results)

	// Check it's sorted correctly: 1080p > 720p > 480p
	if *sorted[0].Resolution != "1080p" {
		t.Fatalf("expected first result to be 1080p, got %s", *sorted[0].Resolution)
	}
	if *sorted[1].Resolution != "720p" {
		t.Fatalf("expected second result to be 720p, got %s", *sorted[1].Resolution)
	}
	if *sorted[2].Resolution != "480p" {
		t.Fatalf("expected third result to be 480p, got %s", *sorted[2].Resolution)
	}
}

func TestSortTorrentResults_ByFansub_SameResolution(t *testing.T) {
	resolution1080p := "1080p"

	results := []nyaa.TorrentResult{
		{Name: "[Erai-Raws] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[SubsPlease] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[Judas] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[ToonsHub] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[ASW] Anime Episode 1 1080p", Resolution: &resolution1080p},
	}

	sorted := nyaa.SortTorrentResults(results)

	// Check it's sorted correctly by fansub: SubsPlease > Erai-Raws > Judas > ToonsHub > ASW
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("expected first result to be SubsPlease, got %s", sorted[0].Name)
	}
	if !strings.Contains(sorted[1].Name, "Erai-Raws") {
		t.Fatalf("expected second result to be Erai-Raws, got %s", sorted[1].Name)
	}
	if !strings.Contains(sorted[2].Name, "Judas") {
		t.Fatalf("expected third result to be Judas, got %s", sorted[2].Name)
	}
	if !strings.Contains(sorted[3].Name, "ToonsHub") {
		t.Fatalf("expected fourth result to be ToonsHub, got %s", sorted[3].Name)
	}
	if !strings.Contains(sorted[4].Name, "ASW") {
		t.Fatalf("expected fifth result to be ASW, got %s", sorted[4].Name)
	}
}

func TestSortTorrentResults_MixedPriorities(t *testing.T) {
	resolution720p := "720p"
	resolution1080p := "1080p"

	results := []nyaa.TorrentResult{
		{Name: "[Judas] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[SubsPlease] Anime Episode 1 720p", Resolution: &resolution720p},
		{Name: "[Ember] Anime Episode 1 1080p", Resolution: &resolution1080p},
		{Name: "[SubsPlease] Anime Episode 1 1080p", Resolution: &resolution1080p},
	}

	sorted := nyaa.SortTorrentResults(results)

	// Check 1080p comes before 720p, regardless of fansub. Among the 1080p ones, SubsPlease must come
	// first
	if *sorted[0].Resolution != "1080p" {
		t.Fatalf("expected first result to be 1080p, got %s", *sorted[0].Resolution)
	}
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("expected SubsPlease to be first in 1080p results, got %s", sorted[0].Name)
	}
	if *sorted[3].Resolution != "720p" {
		t.Fatalf("expected last result to be 720p, got %s", *sorted[3].Resolution)
	}
}

func TestSortTorrentResults_WithUnknownResolution(t *testing.T) {
	resolution1080p := "1080p"

	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime Episode 1", Resolution: nil},
		{Name: "[Ember] Anime Episode 1 1080p", Resolution: &resolution1080p},
	}

	sorted := nyaa.SortTorrentResults(results)

	// Torrents with a known resolution must come before those without
	if sorted[0].Resolution == nil {
		t.Fatalf("expected first result to have resolution, got nil")
	}
	if sorted[1].Resolution != nil {
		t.Fatalf("expected last result to have nil resolution, got %s", *sorted[1].Resolution)
	}
}

func TestScrapNyaaForAnime_CanGetMultipleEpisodes(t *testing.T) {
	options := []string{
		"My.Show.S01E02.1080p",
		"My.Show.S01E03.1080p",
		"My.Show.S01E05.1080p",
		"My.Show.S01E10.1080p",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("My.Show", []int{2, 3, 5}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)
	if len(singles) != 3 {
		t.Fatalf("expected 3 results, got %d", len(singles))
	}

	// Check that only episodes 2, 3 and 5 were returned
	episodes := make(map[int]bool)
	for _, r := range singles {
		if r.Episode != nil {
			episodes[*r.Episode] = true
		}
	}

	if !episodes[2] || !episodes[3] || !episodes[5] {
		t.Fatalf("expected episodes 2, 3, and 5, got %v", episodes)
	}
	if episodes[10] {
		t.Fatalf("episode 10 should not be in results")
	}
}

// The pack is no longer dropped, it's classified: it goes into the list with IsBatch, not as
// "episode 1" via packsAndSingles.
func TestScrapNyaaForAnime_ClassifiesBatchInsteadOfDiscarding(t *testing.T) {
	options := []string{
		"[Erai-raws] Naruto - 001 ~ 220 [480p][Multiple Subtitle]",
		"[DB] Naruto Movie 1 (Eng Sub).avi",
		"[Erai-raws] Naruto - 002 [480p]",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Naruto", []int{1, 2}, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}

	packs, singles := packsAndSingles(results)
	for _, r := range singles {
		if strings.Contains(r.Name, "Movie") {
			t.Fatalf("movie leaked into single-episode results: %s", r.Name)
		}
	}
	if len(singles) != 1 {
		t.Fatalf("expected only the single-episode result, got %d: %v", len(singles), singles)
	}
	if len(packs) != 1 {
		t.Fatalf("expected the batch classified as pack, got %d: %v", len(packs), packs)
	}
}

func TestScrapNyaaForAnime_CanFilterByAnimeTitle(t *testing.T) {
	options := testOptions{
		animeName: "Kemono Friends",
		episode:   0, // not used in this test
		correct: []string{
			"Kemono Friends Episode 2",
			"Kemono Friends Episode 5",
			"Kemono Friends Episode 10",
		},
		incorrect: []string{
			"Kemono Jihen Episode 2",
			"Manaria Friends Episode 5",
			"Different Show Episode 10",
		},
	}

	html := mockHtml(append(options.correct, options.incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Kemono Friends", []int{2, 5, 10}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)
	if len(singles) == 0 {
		t.Fatalf("expected at least one result")
	}

	// Check that only "Kemono Friends" torrents were returned
	for _, r := range singles {
		if !strings.Contains(strings.ToLower(r.Name), "kemono friends") {
			t.Fatalf("unexpected anime in results: %s", r.Name)
		}

		// Check that no incorrect torrent is in the results
		for _, incorrect := range options.incorrect {
			if r.Name == incorrect {
				t.Fatalf("found incorrect torrent in results: %s", r.Name)
			}
		}
	}
}

func TestScrapNyaaForAnime_ReturnsNilWhenNoResults(t *testing.T) {
	html := mockHtml([]string{"Some.Other.Show.S01E01.1080p"})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("My.Show", []int{2, 3, 5}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results when no episodes found, got %v", results)
	}
}

func TestScrapNyaaForAnime_WithSeasonFiltering(t *testing.T) {
	options := []string{
		"Show Season 2 Episode 5",
		"Show Season 2 Episode 10",
		"Show Season 3 Episode 5",
		"Show Season 3 Episode 10",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	// Solicitar especificamente Season 2
	season2 := 2
	results, err := nyaa.ScrapNyaaForAnime("Show Season 2", []int{5, 10}, &season2, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)

	// Must return only the Season 2 episodes
	for _, r := range singles {
		if r.Season == nil || *r.Season != 2 {
			t.Fatalf("expected season 2, got %v in %s", r.Season, r.Name)
		}
	}
}

func TestScrapNyaaForAnime_ResultsAreSorted(t *testing.T) {
	options := []string{
		"Show [SubsPlease] Episode 2 720p",
		"Show [Judas] Episode 2 1080p",
		"Show [Ember] Episode 2 1080p",
		"Show [SubsPlease] Episode 2 1080p",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Show", []int{2}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)

	if len(singles) == 0 {
		t.Fatalf("expected at least one result")
	}

	// Check the results are sorted (1080p before 720p, SubsPlease first among the 1080p)
	found1080p := false

	for i, r := range singles {
		if r.Resolution != nil && *r.Resolution == "1080p" {
			found1080p = true
			// SubsPlease must be the first 1080p result
			if i == 0 && !strings.Contains(r.Name, "SubsPlease") {
				t.Fatalf("expected first result to be SubsPlease 1080p")
			}
		}
		if r.Resolution != nil && *r.Resolution == "720p" {
			// 720p must come after 1080p
			if found1080p && i == 0 {
				t.Fatalf("expected 1080p to come before 720p")
			}
		}
	}
}

func TestScrapNyaaForAnime_CanGetCorrectAnimeMultipleSeasons(t *testing.T) {
	options := testOptions{
		animeName: "Lucky Star",
		episode:   0, // not used in this test
		correct: []string{
			"Lucky Star Episode 5",
			"Lucky Star Episode 10",
			"Lucky Star Episode 15",
			"Lucky Star S01E05",
			"Lucky Star S01E10",
			"Lucky Star 1st Season - 15",
		},
		incorrect: []string{
			"Lucky Star Season 2 Episode 5",
			"Lucky Star S02E10",
			"Lucky Star 2nd Season - 15",
		},
	}

	html := mockHtml(append(options.correct, options.incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Lucky Star", []int{5, 10, 15}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)

	if len(singles) == 0 {
		t.Fatalf("expected at least one result")
	}

	foundCorrect := make(map[string]bool)
	for _, r := range singles {
		// No incorrect torrent may be in the results
		for _, incorrect := range options.incorrect {
			if r.Name == incorrect {
				t.Fatalf("found incorrect torrent in results: %s", r.Name)
			}
		}

		// Rastrear torrents corretos encontrados
		for _, correct := range options.correct {
			if r.Name == correct {
				foundCorrect[correct] = true
			}
		}
	}

	if len(foundCorrect) == 0 {
		t.Fatalf("expected to find at least one correct torrent")
	}
}

// ============================================
// TESTS FOR BATCHES AND MOVIES
// ============================================

func TestIsBatch_DetectsBatchTorrents(t *testing.T) {
	tests := []struct {
		name     string
		torrent  string
		expected bool
	}{
		{
			name:     "Explicit batch keyword",
			torrent:  "[SubsPlease] Anime Series (Batch) [1080p]",
			expected: true,
		},
		{
			name:     "Batch with brackets",
			torrent:  "[Group] Anime [Batch]",
			expected: true,
		},
		{
			name:     "Range 01-28",
			torrent:  "[Group] Anime (01-28) [1080p]",
			expected: true,
		},
		{
			name:     "Range 01 ~ 28",
			torrent:  "[Group] Anime (01 ~ 28) [1080p]",
			expected: true,
		},
		{
			name:     "Range without brackets",
			torrent:  "Anime 01-12 [1080p]",
			expected: true,
		},
		{
			name:     "Complete keyword",
			torrent:  "[Group] Anime Series (Complete) [1080p]",
			expected: true,
		},
		{
			name:     "Complete series",
			torrent:  "[Group] Anime Complete Series [1080p]",
			expected: true,
		},
		{
			name:     "Complete season",
			torrent:  "[Group] Anime Season 1 (Complete) [1080p]",
			expected: true,
		},
		{
			name:     "Unofficial batch",
			torrent:  "[Group] Anime (Unofficial Batch) [1080p]",
			expected: true,
		},
		{
			name:     "S01 in brackets",
			torrent:  "[Group] Anime [S01]",
			expected: true,
		},
		{
			name:     "S01 with BD info",
			torrent:  "[Group] Anime (2025) - S01 (BD 1080p)",
			expected: true,
		},
		{
			name:     "S01 with dot notation",
			torrent:  "[Group] Anime.S01.1080p.BluRay",
			expected: true,
		},
		{
			name:     "S01 with season keyword",
			torrent:  "[Group] Anime (2025) S01 (Season 1) (1080p)",
			expected: true,
		},
		{
			name:     "S01 with space and resolution",
			torrent:  "[Group] Anime S01 1080p WEB-DL",
			expected: true,
		},
		{
			name:     "Season 01 in brackets",
			torrent:  "[Group] Anime (Season 01) [1080p] (Batch)",
			expected: true,
		},
		{
			name:     "Season keyword",
			torrent:  "[Group] Anime Season 1 Complete [1080p]",
			expected: true,
		},
		{
			name:     "Single episode with dash",
			torrent:  "[Group] Anime - 05 [1080p]",
			expected: false,
		},
		{
			name:     "Fansub at end with hyphen",
			torrent:  "Apocalypse.Hotel.S01.1080p.BluRay.Remux.FLAC.2.0.H.264-LaCroiX",
			expected: true,
		},
		{
			name:     "Fansub at end with hyphen and brackets",
			torrent:  "[LaCroiX] Apocalypse.Hotel.S01.1080p.BluRay.Remux.FLAC.2.0.H.264",
			expected: true,
		},
		{
			name:     "Single episode with EP",
			torrent:  "[Group] Anime EP05 [1080p]",
			expected: false,
		},
		{
			name:     "Single episode S01E05",
			torrent:  "[Group] Anime S01E05 [1080p]",
			expected: false,
		},
		// Range at the end of the name, with no space after: seen on Nyaa in the search for One Piece
		// episode 1 (500-episode packs read as "episode 1").
		{
			name:     "Range at end of name",
			torrent:  "One Piece EP 001-501",
			expected: true,
		},
		{
			name:     "Range before file extension",
			torrent:  "One Piece 001-501.mkv",
			expected: true,
		},
		{
			name:     "Range before bracket",
			torrent:  "[Judas] One Piece 001-574[BD 1080p]",
			expected: true,
		},
		{
			name:     "Single episode at end of name",
			torrent:  "Lucky Star 15",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nyaa.IsBatch(tt.torrent)
			if result != tt.expected {
				t.Errorf("isBatch(%q) = %v, want %v", tt.torrent, result, tt.expected)
			}
		})
	}
}

func TestIsMovie_DetectsMovies(t *testing.T) {
	tests := []struct {
		name     string
		torrent  string
		anime    string
		expected bool
	}{
		{
			name:     "Movie in brackets",
			torrent:  "[Group] Your Name (Movie) [1080p]",
			anime:    "Your Name",
			expected: true,
		},
		{
			name:     "Movie in square brackets",
			torrent:  "[Group] Suzume [Movie] [1080p]",
			anime:    "Suzume no Tojimari",
			expected: true,
		},
		{
			name:     "Movie with space",
			torrent:  "[Group] Anime Movie [1080p]",
			anime:    "Anime",
			expected: true,
		},
		{
			name:     "Gekijouban",
			torrent:  "[Group] Anime Gekijouban [1080p]",
			anime:    "Anime",
			expected: true,
		},
		{
			name:     "OVA in parentheses",
			torrent:  "[Group] Anime (OVA) [1080p]",
			anime:    "Anime",
			expected: true,
		},
		{
			name:     "ONA",
			torrent:  "[Group] Anime ONA [1080p]",
			anime:    "Anime",
			expected: true,
		},
		{
			name:     "Special",
			torrent:  "[Group] Anime Special [1080p]",
			anime:    "Anime",
			expected: true,
		},
		{
			name:     "Known movie - Your Name",
			torrent:  "[Group] Kimi no Na wa [1080p]",
			anime:    "Your Name",
			expected: true,
		},
		{
			name:     "Known movie - Suzume",
			torrent:  "[Group] Suzume no Tojimari [1080p]",
			anime:    "Suzume",
			expected: true,
		},
		{
			name:     "TV series episode",
			torrent:  "[Group] Anime - 05 [1080p]",
			anime:    "Anime",
			expected: false,
		},
		{
			name:     "TV series with episode",
			torrent:  "[Group] Anime Episode 5 [1080p]",
			anime:    "Anime",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using reflection to reach a private function. In practice, isMovie is tested through the
			// search results. This is a unit test of the detection logic
		})
	}
}

func TestExtractBatchInfo_ExtractsRangeAndSeason(t *testing.T) {
	cases := []struct {
		name       string
		wantStart  int
		wantEnd    int
		wantSeason int
	}{
		{"[Judas] Anime 01-12 [1080p]", 1, 12, 0},
		{"[SubsPlease] Anime (001-100) [1080p]", 1, 100, 0},
		{"[Group] Anime S2 01-24 [BD]", 1, 24, 2},
		// False-positive guard: [720-1080p] is not an episode range. Without it, this complete pack would
		// be read as "covers episodes 720 to 1080". "[Group]" (not "[X]") avoids colliding with the
		// roman-numeral season guard (reRomanSeason reads "X" as S10; out of this task's scope).
		{"[Group] Anime Complete Batch [720-1080p]", 0, 0, 0},
		{"[Group] Anime Complete Batch", 0, 0, 0},
		// Inverted range: the match takes "2 - 01" (season + first episode) and would give 2..1, a range
		// that covers no episode.
		{"[Erai-raws] Hibike! Euphonium 2 - 01 ~ 13 [1080p][Multiple Subtitle]", 0, 0, 2},
		// Release year: measured on "[EMBER] Mushoku Tensei ... (2021-2022) (Season 1 ...)", which became
		// the range 2021..2022 and vanished from the pack choice for covering no episode.
		{"[EMBER] Anime (2021-2022) (Season 1) [BDRip] (Batch)", 0, 0, 1},
		// A rejected match doesn't end the scan: the real range comes after the year.
		{"[Group] Anime (2021-2022) 01-24 [BD]", 1, 24, 0},
		// The band only catches years — a long series numbered above 1000 is still a range.
		{"[Judas] One Piece 1001-1100 [1080p]", 1001, 1100, 0},
	}

	for _, tc := range cases {
		got := nyaa.ExtractBatchInfo(tc.name)
		if got.StartEpisode != tc.wantStart || got.EndEpisode != tc.wantEnd {
			t.Errorf("%q: want range %d-%d, got %d-%d", tc.name, tc.wantStart, tc.wantEnd, got.StartEpisode, got.EndEpisode)
		}
		if got.Season != tc.wantSeason {
			t.Errorf("%q: want season %d, got %d", tc.name, tc.wantSeason, got.Season)
		}
	}
}

// A group that separates with "_" instead of a space: every episode pattern wants a space around
// the number, so the whole name matched none and the file went into the library with its raw name.
func TestExtractEpisodeNumber_UnderscoreSeparatedNames(t *testing.T) {
	cases := []struct {
		name string
		want *int
	}{
		{"[DB]Vinland Saga_-_01_(Dual Audio_10bit_BD1080p_x265).mkv", intPtr(1)},
		{"[DB]Vinland Saga_-_24_(Dual Audio_10bit_BD1080p_x265).mkv", intPtr(24)},
		// An opening/ending without an episode number stays without a number.
		{"[DB]Vinland Saga_-_NCED01_(10bit_BD1080p_x265).mkv", nil},
	}

	for _, tc := range cases {
		got := nyaa.ExtractEpisodeNumber(tc.name)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%q: want no episode, got %d", tc.name, *got)
		case tc.want != nil && got == nil:
			t.Errorf("%q: want episode %d, got none", tc.name, *tc.want)
		case tc.want != nil && *got != *tc.want:
			t.Errorf("%q: want episode %d, got %d", tc.name, *tc.want, *got)
		}
	}
}

// The dash terminator is space / v2 / end / '[' — a dot is none of those, so " - 01.mkv" only
// numbered because a later pattern required the .mkv suffix. MystVortex ships .mp4 (and IsVideoFile
// accepts a dozen other extensions), so those files were unnumbered. A catch-all "dot ends the
// number" would also read "- 17.5 (OVA)" as 17.
func TestExtractEpisodeNumber_LooseNumberBeforeVideoExtension(t *testing.T) {
	cases := []struct {
		name string
		want *int
	}{
		{"[MystVortex] K-On! - 01.mp4", intPtr(1)},
		{"[MystVortex] K-On! - 13.MP4", intPtr(13)},
		{"Anime 01.mkv", intPtr(1)},
		{"Anime 01.avi", intPtr(1)},
		{"Anime 01.mov", intPtr(1)},
		{"Anime 01.m4v", intPtr(1)},
		{"Anime 01.webm", intPtr(1)},
		{"Anime 01.flv", intPtr(1)},
		{"Anime 01.wmv", intPtr(1)},
		{"Anime 01.ts", intPtr(1)},
		{"Anime 01.mpg", intPtr(1)},
		{"Anime 01.mpeg", intPtr(1)},
		{"Anime 01.ogm", intPtr(1)},
		// A subtitle is not a video file: the suffix must be one IsVideoFile accepts.
		{"Anime 01.srt", nil},
		{"Anime 01.txt", nil},
		// Half-episode: the integer parser must not take 17 from the ".5".
		{"Mushoku Tensei - 17.5 (OVA).mkv", nil},
		{"Hyouka - 11.5.mkv", nil},
	}

	for _, tc := range cases {
		got := nyaa.ExtractEpisodeNumber(tc.name)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%q: want no episode, got %d", tc.name, *got)
		case tc.want != nil && got == nil:
			t.Errorf("%q: want episode %d, got none", tc.name, *tc.want)
		case tc.want != nil && *got != *tc.want:
			t.Errorf("%q: want episode %d, got %d", tc.name, *tc.want, *got)
		}
	}
}

// fileListHtml builds Nyaa's detail page with the file tree, in the real format: one <li> per file,
// with the size in a <span> inside the SAME <li>, and folders as <a.folder>.
func fileListHtml(folder string, files []string) string {
	var items []string
	for _, f := range files {
		items = append(items, `<li><i class="fa fa-file"></i>`+f+` <span class="file-size">(1.0 GiB)</span></li>`)
	}
	return `<!doctype html><html><body>
	<div class="torrent-file-list panel-body">
		<ul>
			<li>
				<a href="" class="folder"><i class="fa fa-folder-open"></i>` + folder + `</a>
				<ul>` + strings.Join(items, "\n") + `</ul>
			</li>
		</ul>
	</div>
	</body></html>`
}

func fileListFileItem(name string) string {
	return `<li><i class="fa fa-file"></i>` + name + ` <span class="file-size">(1.0 GiB)</span></li>`
}

// fileListHtmlNested is a torrent root with one extra subfolder (Specials, NC, …).
func fileListHtmlNested(root string, files []string, subfolder string, subfiles []string) string {
	var items []string
	for _, f := range files {
		items = append(items, fileListFileItem(f))
	}
	var nested []string
	for _, f := range subfiles {
		nested = append(nested, fileListFileItem(f))
	}
	items = append(items, `<li>
				<a href="" class="folder"><i class="fa fa-folder-open"></i>`+subfolder+`</a>
				<ul>`+strings.Join(nested, "\n")+`</ul>
			</li>`)
	return `<!doctype html><html><body>
	<div class="torrent-file-list panel-body">
		<ul>
			<li>
				<a href="" class="folder"><i class="fa fa-folder-open"></i>` + root + `</a>
				<ul>` + strings.Join(items, "\n") + `</ul>
			</li>
		</ul>
	</div>
	</body></html>`
}

type fileListDir struct {
	name  string
	files []string
}

func fileListHtmlFolders(root string, dirs []fileListDir) string {
	var inner []string
	for _, d := range dirs {
		var items []string
		for _, f := range d.files {
			items = append(items, fileListFileItem(f))
		}
		inner = append(inner, `<li>
				<a href="" class="folder"><i class="fa fa-folder-open"></i>`+d.name+`</a>
				<ul>`+strings.Join(items, "\n")+`</ul>
			</li>`)
	}
	return `<!doctype html><html><body>
	<div class="torrent-file-list panel-body">
		<ul>
			<li>
				<a href="" class="folder"><i class="fa fa-folder-open"></i>` + root + `</a>
				<ul>` + strings.Join(inner, "\n") + `</ul>
			</li>
		</ul>
	</div>
	</body></html>`
}

func numberedFiles(pattern string, from, to int) []string {
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf(pattern, i))
	}
	return out
}

func TestPackFileRange_ReadsTheRangeFromTheFileList(t *testing.T) {
	// The measured case: "(Season 2 | Part 2) … (Batch)" declares no range in the name and carries
	// 13..24.
	restore := mockHttpGet(fileListHtml("[EMBER] Anime (Season 2 | Part 2) (Batch)", []string{
		"[EMBER] Anime - 13.mkv", "[EMBER] Anime - 14.mkv", "[EMBER] Anime - 24.mkv",
	}))
	defer restore()

	info, ok := nyaa.PackFileRange("https://nyaa.si/view/1891610", 0)
	if !ok {
		t.Fatal("want the range read from the files")
	}
	if info.StartEpisode != 13 || info.EndEpisode != 24 {
		t.Errorf("want 13-24, got %d-%d", info.StartEpisode, info.EndEpisode)
	}
}

// The FOLDER name usually carries the whole release's range, and it must not count; NCOP/NCED and
// non-video files don't widen the range either.
func TestPackFileRange_IgnoresFoldersExtrasAndNonVideo(t *testing.T) {
	restore := mockHttpGet(fileListHtml("[Judas] Anime 001-574 [1080p]", []string{
		"[Judas] Anime - 01.mkv", "[Judas] Anime - 02.mkv",
		"NCOP.mkv", "[Judas] Anime - 99.txt", "[Judas] Anime - 300.ass",
	}))
	defer restore()

	info, ok := nyaa.PackFileRange("https://nyaa.si/view/1", 0)
	if !ok || info.StartEpisode != 1 || info.EndEpisode != 2 {
		t.Errorf("want 1-2 (ok=%v), got %d-%d", ok, info.StartEpisode, info.EndEpisode)
	}
}

// Without a readable number there's no range to assert: the caller treats it as unknown, not as
// 1..total.
func TestPackFileRange_UnreadableFileNamesReportUnknown(t *testing.T) {
	restore := mockHttpGet(fileListHtml("[Group] Anime", []string{"Opening.mkv", "Interview.mkv"}))
	defer restore()

	if _, ok := nyaa.PackFileRange("https://nyaa.si/view/1", 0); ok {
		t.Error("want an unknown range when no file gives the number")
	}
}

func TestPackFileRange_NoURLDoesNotFetch(t *testing.T) {
	fetched := false
	restore := nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		fetched = true
		return nil, io.EOF
	})
	defer restore()

	if _, ok := nyaa.PackFileRange("", 0); ok || fetched {
		t.Error("without a detail URL there is nothing to fetch")
	}
}

// Method C: extras/OVA do not feed min-max; a contiguous run past the entry total stays (next cour);
// an unmarked total+1 with run length 1 drops. Pins for decisions.md #113.
func TestPackFileRange_ExtrasDoNotStretchTheDeclaredSpan(t *testing.T) {
	kOnTV := []string{
		"[MTBB] K-ON! - 01.mkv", "[MTBB] K-ON! - 13.mkv",
	}
	mushokuTV := []string{
		"[MTBB] Mushoku Tensei - 01.mkv", "[MTBB] Mushoku Tensei - 12.mkv", "[MTBB] Mushoku Tensei - 23.mkv",
	}

	cases := []struct {
		name   string
		html   string
		total  int
		wantLo int
		wantHi int
	}{
		{
			name:   "MTBB K-ON S00E19 OVA stores 1-13",
			html:   fileListHtml("[MTBB] K-ON! S1", append(append([]string{}, kOnTV...), "[MTBB] K-ON! - S00E19 (OVA).mkv")),
			total:  13,
			wantLo: 1, wantHi: 13,
		},
		{
			name:   "MiniMTBB S00E19 without OVA word still stores 1-13",
			html:   fileListHtml("[MiniMTBB] K-ON! S1", append(append([]string{}, kOnTV...), "[MiniMTBB] K-ON! - S00E19.mkv")),
			total:  13,
			wantLo: 1, wantHi: 13,
		},
		{
			name:   "Honoka unmarked 14 is isolated total+1",
			html:   fileListHtml("[HonokaSubs] K-ON!", append(append([]string{}, kOnTV...), "[HonokaSubs] K-ON! - 14.mkv")),
			total:  13,
			wantLo: 1, wantHi: 13,
		},
		{
			name:   "Mushoku 1-23 under cour-1 total 11 stays (next cour)",
			html:   fileListHtml("[MTBB] Mushoku Tensei S1", mushokuTV),
			total:  11,
			wantLo: 1, wantHi: 23,
		},
		{
			name:   "EMBER S01E24 OVA sits on the Mushoku axis and drops",
			html:   fileListHtml("[EMBER] Mushoku Tensei S1", append(append([]string{}, mushokuTV...), "[EMBER] Mushoku Tensei - S01E24 [OVA].mkv")),
			total:  11,
			wantLo: 1, wantHi: 23,
		},
		{
			name:   "Steins;Gate S01E25 OVA drops; 1-24 stays",
			html:   fileListHtml("[Judas] Steins;Gate", []string{"[Judas] Steins;Gate - 01.mkv", "[Judas] Steins;Gate - 24.mkv", "[Judas] Steins;Gate - S01E25 (OVA).mkv"}),
			total:  24,
			wantLo: 1, wantHi: 24,
		},
		{
			name:   "DeadFish unmarked 25 is isolated total+1",
			html:   fileListHtml("[DeadFish] Steins;Gate", []string{"[DeadFish] Steins;Gate - 01.mkv", "[DeadFish] Steins;Gate - 24.mkv", "[DeadFish] Steins;Gate - 25.mkv"}),
			total:  24,
			wantLo: 1, wantHi: 24,
		},
		{
			name:   "Creditless Opening 1 does not pull min down to 1",
			html:   fileListHtml("[Group] Anime", []string{"[Group] Anime - 13.mkv", "[Group] Anime - 24.mkv", "Creditless Opening 1.mkv"}),
			total:  12, // Part 2 numbering: 13 is the start, not isolated total+1
			wantLo: 13, wantHi: 24,
		},
		{
			name:   "NCOP 01 in folder NC stays out",
			html:   fileListHtmlNested("[Group] Anime", []string{"[Group] Anime - 13.mkv", "[Group] Anime - 24.mkv"}, "NC", []string{"NCOP 01.mkv"}),
			total:  12,
			wantLo: 13, wantHi: 24,
		},
		{
			name:   "S00E19 in Specials does not stretch; torrent-root OVA in the folder name does not mark TV files",
			html:   fileListHtmlNested("(S1 + OVA + Specials)", []string{"[MTBB] K-ON! - 01.mkv", "[MTBB] K-ON! - 13.mkv"}, "Specials", []string{"[MTBB] K-ON! - S00E19 (OVA).mkv"}),
			total:  13,
			wantLo: 1, wantHi: 13,
		},
		{
			name:   "half-episode 11.5 does not stretch Hyouka 1-22",
			html:   fileListHtml("[Group] Hyouka", []string{"[Group] Hyouka - 01.mkv", "[Group] Hyouka - 22.mkv", "[Group] Hyouka - 11.5.mkv"}),
			total:  22,
			wantLo: 1, wantHi: 22,
		},
		{
			name:   "Special Training in the episode title is still episode 3",
			html:   fileListHtml("[Kira-Fansub] K-ON!", []string{"[Kira-Fansub] K-ON! EP 01.mkv", "[Kira-Fansub] K-ON! EP 03 - Special Training!.mkv", "[Kira-Fansub] K-ON! EP 13.mkv"}),
			total:  13,
			wantLo: 1, wantHi: 13,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := mockHttpGet(tc.html)
			defer restore()
			info, ok := nyaa.PackFileRange("https://nyaa.si/view/1", tc.total)
			if !ok {
				t.Fatal("want the range read from the files")
			}
			if info.StartEpisode != tc.wantLo || info.EndEpisode != tc.wantHi {
				t.Errorf("want %d-%d, got %d-%d", tc.wantLo, tc.wantHi, info.StartEpisode, info.EndEpisode)
			}
		})
	}
}

// Restart-per-season packs (S01E01-12 + S02E01-12) flatten onto the concatenated series axis
// so S1's stored span covers S2 the same way a continuous 1-24 pack already does. Measured
// file lists: Kaguya EMBER 1355132, Kaguya DB 1928670, AoT EMBER 1421679, Mushoku Anime Time
// 2169474. A movie folder that also restarts (DB First Kiss 01-04) rides along as residue.
func TestPackFileRange_RestartSeasonGroupsFlattenOntoConcatenatedAxis(t *testing.T) {
	emberS1S2 := fileListHtmlFolders("Kaguya-sama Love is War S01-S02 1080p Dual Audio BDRip 10 bits DD x265-EMBER", []fileListDir{
		{"Kaguya-sama Love is War S01 1080p Dual Audio BDRip 10 bits DD x265-EMBER", numberedFiles("S01E%02d-Episode Title [ABCDEF01].mkv", 1, 12)},
		{"Kaguya-sama Love is War S02 1080p Dual Audio BDRip 10 bits AAC x265-EMBER", numberedFiles("S02E%02d-Episode Title [ABCDEF02].mkv", 1, 12)},
	})
	dbS1S2S3 := fileListHtmlFolders("Kaguya-sama wa Kokurasetai: Tensai-tachi no Renai Zunousen", []fileListDir{
		{"Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen", append(
			[]string{
				"[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen_-_NCED01_(10bit_BD1080p_x265).mkv",
				"[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen_-_NCOP_(10bit_BD1080p_x265).mkv",
			},
			numberedFiles("[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen_-_%02d_(Dual Audio_10bit_BD1080p_x265).mkv", 1, 12)...,
		)},
		{"Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen 2nd Season", append(
			[]string{
				"[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen 2nd Season_-_NCED_(10bit_BD1080p_x265).mkv",
				"[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen 2nd Season_-_OVA_(10bit_WEB1080p_x265).mkv",
			},
			numberedFiles("[DB]Kaguya-sama wa Kokurasetai Tensai-tachi no Renai Zunousen 2nd Season_-_%02d_(Dual Audio_10bit_BD1080p_x265).mkv", 1, 12)...,
		)},
		{"Kaguya-sama wa Kokurasetai Ultra Romantic", append(
			[]string{
				"[DB]Kaguya-sama wa Kokurasetai Ultra Romantic Teaser - Ishigami Yuu wa Kataritai_-_PV_(10bit_BD1080p_x265).mkv",
				"[DB]Kaguya-sama wa Kokurasetai Ultra Romantic_-_NCOP_(10bit_BD1080p_x265).mkv",
			},
			numberedFiles("[DB]Kaguya-sama wa Kokurasetai Ultra Romantic_-_%02d_(Dual Audio_10bit_BD1080p_x265).mkv", 1, 13)...,
		)},
		{"Kaguya-sama wa Kokurasetai First Kiss wa Owaranai", append(
			[]string{
				"[DB]Kaguya-sama wa Kokurasetai First Kiss wa Owaranai_-_NCED_(10bit_BD1080p_x265).mkv",
			},
			numberedFiles("[DB]Kaguya-sama wa Kokurasetai First Kiss wa Owaranai_-_%02d_(Dual Audio_10bit_BD1080p_x265).mkv", 1, 4)...,
		)},
	})

	emberAoT := fileListHtmlFolders("Shingeki No Kyojin Full Collection", []fileListDir{
		{"Shingeki no Kyojin S01 1080p BDRip 10 bits x265-EMBER", numberedFiles("S01E%02d- Episode [14197707].mkv", 1, 25)},
		{"Shingeki no Kyojin S02 1080p BDRip 10 bits x265-EMBER", numberedFiles("S02E%02d- Episode [C5228D49].mkv", 1, 12)},
		{"Shingeki no Kyojin S03P01 1080p BDRip 10 bits x265-EMBER", numberedFiles("S03E%02d- Episode [FAC2D1C7].mkv", 1, 12)},
		{"Shingeki no Kyojin S03P02 1080p BDRip 10 bits x265-EMBER", numberedFiles("S03E%02d- Episode [3B28B611].mkv", 13, 22)},
		{"Shingeki no Kyojin S04P01 1080p BDRip 10 bits x265-EMBER", numberedFiles("S04E%02d- Episode [E4B0B00C].mkv", 1, 16)},
	})
	animeTimeMushoku := fileListHtmlFolders("Mushoku Tensei Jobless Reincarnation", []fileListDir{
		{"Season 1", append(
			numberedFiles("Mushoku Tensei Jobless Reincarnation - S01E%02d - Title.mkv", 1, 23),
			"Mushoku Tensei Jobless Reincarnation - S01E17.5 - Eris The Goblin Slayer.mkv",
		)},
		{"Season 2", append(
			[]string{"Mushoku Tensei Jobless Reincarnation - S02E00 - Guardian Fitz.mkv"},
			numberedFiles("Mushoku Tensei Jobless Reincarnation - S02E%02d - Title.mkv", 1, 24)...,
		)},
	})

	cases := []struct {
		name   string
		html   string
		total  int
		wantLo int
		wantHi int
	}{
		{
			name:   "Kaguya EMBER S1+S2 nyaa.si/view/1355132 is 12+12",
			html:   emberS1S2,
			total:  12,
			wantLo: 1, wantHi: 24,
		},
		{
			name:   "Kaguya DB S1+S2+S3+movie nyaa.si/view/1928670 is 12+12+13+4",
			html:   dbS1S2S3,
			total:  12,
			wantLo: 1, wantHi: 41,
		},
		{
			name:   "AoT EMBER S1+S2+S3+S4P1 nyaa.si/view/1421679 is 25+12+22+16",
			html:   emberAoT,
			total:  25,
			wantLo: 1, wantHi: 75,
		},
		{
			name:   "Mushoku Anime Time S1+S2 nyaa.si/view/2169474 is 23+24, skips 17.5 and S02E00",
			html:   animeTimeMushoku,
			total:  11,
			wantLo: 1, wantHi: 47,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := mockHttpGet(tc.html)
			defer restore()
			info, ok := nyaa.PackFileRange("https://nyaa.si/view/1", tc.total)
			if !ok {
				t.Fatal("want the range read from the files")
			}
			if info.StartEpisode != tc.wantLo || info.EndEpisode != tc.wantHi {
				t.Errorf("want %d-%d, got %d-%d", tc.wantLo, tc.wantHi, info.StartEpisode, info.EndEpisode)
			}
		})
	}
}

// Continuous numbering across season folders is already min-max of the axis. Flatten must
// not rewrite that as a sum of per-folder spans. Anime Time AoT (1739120) numbers 01-87
// through S04 Part 2; Mushoku 01-23 is one cour-split season on the same ruler.
func TestPackFileRange_ContinuousNumberingAcrossSeasonFoldersStaysMinMax(t *testing.T) {
	animeTimeAoT := fileListHtmlFolders("[Anime Time] Attack On Titan (Complete Series)", []fileListDir{
		{"[Anime Time] Attack on titan (Season 01)", numberedFiles("[Anime Time] Attack On Titan - %02d.mkv", 1, 25)},
		{"[Anime Time] Attack on titan (Season 02)", numberedFiles("[Anime Time] Attack On Titan - %02d.mkv", 26, 37)},
		{"[Anime Time] Attack on titan (Season 03)", numberedFiles("[Anime Time] Attack On Titan - %02d.mkv", 38, 59)},
		{"[Anime Time] Attack On Titan (Season 4 Part 1)", numberedFiles("[Anime Time] Attack On Titan - %02d.mkv", 60, 75)},
		{"[Anime Time] Attack On Titan (Season 4 Part 2)", numberedFiles("[Anime Time] Attack On Titan - %02d.mkv", 76, 87)},
	})
	mushokuContinuous := fileListHtml("[MTBB] Mushoku Tensei S1", numberedFiles("[MTBB] Mushoku Tensei - %02d.mkv", 1, 23))

	cases := []struct {
		name   string
		html   string
		total  int
		wantLo int
		wantHi int
	}{
		{
			name:   "AoT Anime Time S01-S04P2 nyaa.si/view/1739120 numbers 1-87",
			html:   animeTimeAoT,
			total:  25,
			wantLo: 1, wantHi: 87,
		},
		{
			name:   "Mushoku 1-23 under cour-1 total 11 stays next-cour span",
			html:   mushokuContinuous,
			total:  11,
			wantLo: 1, wantHi: 23,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := mockHttpGet(tc.html)
			defer restore()
			info, ok := nyaa.PackFileRange("https://nyaa.si/view/1", tc.total)
			if !ok {
				t.Fatal("want the range read from the files")
			}
			if info.StartEpisode != tc.wantLo || info.EndEpisode != tc.wantHi {
				t.Errorf("want %d-%d, got %d-%d", tc.wantLo, tc.wantHi, info.StartEpisode, info.EndEpisode)
			}
		})
	}
}

// https://nyaa.si/view/164103 is two Kaichou wa Maid-sama files (`!-23[`, `!-24[`), not a
// 3-season Kaguya pack. ExtractEpisodeNumber does not read a number from that name, so the
// file list cannot resolve a range.
func TestPackFileRange_Nyaa164103IsMaidSamaWithoutReadableRange(t *testing.T) {
	restore := mockHttpGet(fileListHtml("[AnS]-Kaichou_wa_Maid-sama!-23-24_XviD", []string{
		"[AnS]Kaichou_wa_Maid-sama!-23[396p-XviD+Mp3][955B2EF5].avi",
		"[AnS]Kaichou_wa_Maid-sama!-24[396p-XviD+Mp3][7B376783].avi",
	}))
	defer restore()

	if _, ok := nyaa.PackFileRange("https://nyaa.si/view/164103", 26); ok {
		t.Fatal("those Maid-sama filenames do not yield a pack range")
	}
}

// The detail URL comes from the title column and arrives absolute: it's the only path to
// PackFileRange.
func TestScrapNyaaForAnime_CapturesTheDetailURL(t *testing.T) {
	restore := mockHttpGet(mockHtml([]string{"[X] Frieren (01-28) [1080p]"}))
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Frieren", nil, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want at least one result")
	}
	if want := "https://nyaa.si/view/1"; results[0].DetailURL != want {
		t.Errorf("want %q, got %q", want, results[0].DetailURL)
	}
}

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

func TestViewURL(t *testing.T) {
	if got := nyaa.ViewURL(1664122); got != "https://nyaa.si/view/1664122" {
		t.Errorf("got %q", got)
	}
	if nyaa.ParseNyaaID(nyaa.ViewURL(42)) != 42 {
		t.Error("ViewURL must round-trip through ParseNyaaID")
	}
}

func TestFetchTorrentDetail_SanitisesHTMLAndListsFiles(t *testing.T) {
	html := `<!doctype html><html><body>
<div id="torrent-description">
  <p>Safe <strong>text</strong></p>
  <table><thead><tr><th>Res</th></tr></thead><tbody><tr><td>1080p</td></tr></tbody></table>
  <hr>
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
	if !strings.Contains(d.DescriptionHTML, "<table") || !strings.Contains(d.DescriptionHTML, "<td") {
		t.Errorf("table tags unwrapped: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, "<hr") {
		t.Errorf("hr stripped: %s", d.DescriptionHTML)
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

func TestFetchTorrentDetail_RendersMarkdownDescription(t *testing.T) {
	// Live Nyaa stores markdown as escaped text with &#10; newlines (markdown-text),
	// then the browser renders it. The scraper sees the source, not the HTML.
	html := `<!doctype html><html><body>
<div markdown-text class="panel-body" id="torrent-description">## Title&#10;&#10;**General** | **Video**&#10;-- | --&#10;1080p | HEVC&#10;&#10;[cmp](https://slow.pics/c/abc)&#10;&lt;script&gt;alert(1)&lt;/script&gt;</div>
<div class="torrent-file-list panel-body"><ul></ul></div>
</body></html>`
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})()

	d, err := nyaa.FetchTorrentDetail(1664122)
	if err != nil {
		t.Fatalf("FetchTorrentDetail: %v", err)
	}
	if !strings.Contains(d.DescriptionHTML, "<h2") || !strings.Contains(d.DescriptionHTML, "Title") {
		t.Errorf("heading not rendered: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, "<table") || !strings.Contains(d.DescriptionHTML, "<td") {
		t.Errorf("markdown table not rendered: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, `href="https://slow.pics/c/abc"`) {
		t.Errorf("markdown link not rendered: %s", d.DescriptionHTML)
	}
	if strings.Contains(d.DescriptionHTML, "<script") || strings.Contains(d.DescriptionHTML, "alert(1)") {
		t.Errorf("raw HTML in markdown survived sanitiser: %s", d.DescriptionHTML)
	}
}

func TestFetchTorrentDetail_KeepsMarkdownImages(t *testing.T) {
	html := `<!doctype html><html><body>
<div markdown-text class="panel-body" id="torrent-description">**Discord Server Link:** [![alt text](https://i2.extraimage.info/pix/badge.png "Join EMBER")](https://discord.gg/ZtZXgJNc3M "Discord")&#10;![alt text](https://i.postimg.cc/PfwHY1xs/shot.png "SCREENS")&#10;&lt;img src="javascript:alert(1)"&gt;</div>
<div class="torrent-file-list panel-body"><ul></ul></div>
</body></html>`
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})()

	d, err := nyaa.FetchTorrentDetail(1664122)
	if err != nil {
		t.Fatalf("FetchTorrentDetail: %v", err)
	}
	if !strings.Contains(d.DescriptionHTML, `src="https://i2.extraimage.info/pix/badge.png"`) {
		t.Errorf("linked markdown image src dropped: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, `src="https://i.postimg.cc/PfwHY1xs/shot.png"`) {
		t.Errorf("markdown image src dropped: %s", d.DescriptionHTML)
	}
	if !strings.Contains(d.DescriptionHTML, `referrerpolicy="no-referrer"`) {
		t.Errorf("img missing no-referrer: %s", d.DescriptionHTML)
	}
	if strings.Contains(d.DescriptionHTML, "javascript:") {
		t.Errorf("javascript img src survived: %s", d.DescriptionHTML)
	}
	if strings.Count(d.DescriptionHTML, "<img") != 2 {
		t.Errorf("want 2 imgs (javascript one dropped), got: %s", d.DescriptionHTML)
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

func TestScrapNyaaForAnime_FindsBatchTorrents(t *testing.T) {
	correct := []string{
		"[SubsPlease] Frieren (01-28) [1080p]",
		"[Erai-Raws] Frieren Batch 01-28 [1080p]",
		"[Group] Frieren Complete Season [1080p]",
	}
	incorrect := []string{
		"[SubsPlease] Frieren - 05 [1080p]",     // Single episode
		"[Group] Different Anime Batch [1080p]", // Different anime
	}

	html := mockHtml(append(correct, incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Frieren", nil, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	packs, _ := packsAndSingles(results)

	if len(packs) == 0 {
		t.Fatalf("expected at least one batch result")
	}

	// Check that only batches were returned
	for _, r := range packs {
		if !nyaa.IsBatch(r.Name) {
			t.Fatalf("non-batch torrent in results: %s", r.Name)
		}
	}

	// Check that all the correct torrents were found
	foundCorrect := make(map[string]bool)
	for _, r := range packs {
		for _, correct := range correct {
			if r.Name == correct {
				foundCorrect[correct] = true
			}
		}
		// Check that no incorrect torrent is in the results
		for _, incorrect := range incorrect {
			if r.Name == incorrect {
				t.Fatalf("found incorrect torrent in results: %s", r.Name)
			}
		}
	}

	if len(foundCorrect) == 0 {
		t.Fatalf("expected to find at least one correct batch torrent")
	}
}

func TestScrapNyaaForAnime_FiltersBySeason(t *testing.T) {
	correct := []string{
		"[SubsPlease] Machikado Mazoku 2 (01-12) [1080p]",
		"[Group] Machikado Mazoku S2 Batch [1080p]",
	}
	incorrect := []string{
		"[SubsPlease] Machikado Mazoku (01-12) [1080p]", // Season 1
		"[Group] Machikado Mazoku 3 Batch [1080p]",      // Season 3
	}

	html := mockHtml(append(correct, incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	season := 2
	results, err := nyaa.ScrapNyaaForAnime("Machikado Mazoku 2", nil, &season, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	packs, _ := packsAndSingles(results)

	// Check that only season 2 was returned
	for _, r := range packs {
		if r.Season == nil || *r.Season != 2 {
			t.Fatalf("expected season 2, got %v for torrent: %s", r.Season, r.Name)
		}
	}
}

func TestScrapNyaaForMovie_FindsMovieTorrents(t *testing.T) {
	correct := []string{
		"[SubsPlease] Suzume no Tojimari Movie (1080p)",
		"[Group] Your Name (Movie) [1080p]",
	}
	// We don't include episodes in the incorrect list, since the mock HTML returns all of them. The
	// isMovie filter must remove the episodes automatically

	html := mockHtml(correct)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForMovie("Suzume no Tojimari")

	if err != nil {
		t.Fatalf("ScrapNyaaForMovie error: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one movie result")
	}

	// Check that all results are valid
	for _, r := range results {
		// Check it has Movie in the name or is a known movie
		hasMovieKeyword := strings.Contains(strings.ToLower(r.Name), "movie") ||
			strings.Contains(strings.ToLower(r.Name), "gekijouban")

		if !hasMovieKeyword {
			t.Logf("Warning: Result doesn't contain 'Movie' keyword: %s", r.Name)
		}
	}

	// Check that at least one correct torrent was found
	found := false
	for _, r := range results {
		for _, correct := range correct {
			if r.Name == correct {
				found = true
				break
			}
		}
	}

	if !found {
		t.Logf("Warning: Expected movie torrents not found. Got %d results:", len(results))
		for _, r := range results {
			t.Logf("  - %s", r.Name)
		}
	}
}

func TestSortMovieResults_SortsByTechnicalQuality(t *testing.T) {
	resolution1080p := "1080p"
	resolution720p := "720p"

	results := []nyaa.TorrentResult{
		{Name: "[Group] Movie WEB-DL 720p AAC [1080p]", Resolution: &resolution1080p},
		{Name: "[Group] Movie BDRip 1080p FLAC [1080p]", Resolution: &resolution1080p},
		{Name: "[Group] Movie BD 1080p FLAC [1080p]", Resolution: &resolution1080p},
		{Name: "[Group] Movie HDTV 720p AAC [720p]", Resolution: &resolution720p},
	}

	sorted := nyaa.SortMovieResults(results)

	// First must be BD (best source)
	if !strings.Contains(sorted[0].Name, "BD ") && !strings.Contains(sorted[0].Name, "BDRip") {
		t.Fatalf("Expected BD or BDRip first, got: %s", sorted[0].Name)
	}

	// Second must have the better priority (priority: source > resolution > codec > fansub > audio >
	// seeders > size)
}

func TestTitleMatchesQuery_AcceptsCorrectTorrents(t *testing.T) {
	tests := []struct {
		torrent string
		query   string
	}{
		{"[SubsPlease] Kemono Friends - 05 (1080p)", "Kemono Friends"},
		{"Kemono Friends Episode 5", "Kemono Friends"},
		{"[Erai-Raws] Lucky Star - 15 (720p)", "Lucky Star"},
		{"Lucky Star S01E15 1080p", "Lucky Star"},
		{"[SubsPlease] Silent Witch - Chinmoku no Majo no Kakushigoto - 03v2 (1080p)", "Silent Witch: Chinmoku no Majo no Kakushigoto"},
		{"SPY x FAMILY Season 3 - 03 [1080p]", "SPY x FAMILY"},
		{"[SubsPlease] Machikado Mazoku S2 - 07 (1080p)", "Machikado Mazoku"},
		{"My.Show.S01E02.1080p", "My.Show"},
		// Torrents with technical metadata that inflated the Jaccard (NF, AMZN, WEB-DL, EAC3, MultiSub)
		{"[Erai-raws] Akane-banashi - 07 [1080p NF WEB-DL AVC AAC][MultiSub][805A5B50]", "Akane-banashi"},
		{"[Erai-raws] Nippon Sangoku - 07 [1080p AMZN WEB-DL AVC EAC3][MultiSub][86BD091C]", "Nippon Sangoku"},
		{"[Erai-raws] Ichijyoma Mankitsu Gurashi - 06 [1080p AMZN WEB-DL AVC EAC3][MultiSub][BC657289]", "Ichijyoma Mankitsu Gurashi"},
		// Fansub with both titles in the name (English + romaji) — threshold 0.4 for a query of ≥4 tokens
		{"[ToonsHub] ReZERO -Starting Life in Another World- S04E06 1080p CR WEB-DL DUAL AAC2.0 H.264 (Re:Zero kara Hajimeru Isekai Seikatsu, Dual-Audio, Multi-Subs)", "rezero starting life in another world"},
	}
	for _, tt := range tests {
		t.Run(tt.torrent, func(t *testing.T) {
			if !nyaa.TitleMatchesQuery(tt.torrent, tt.query) {
				t.Errorf("TitleMatchesQuery(%q, %q) = false, want true", tt.torrent, tt.query)
			}
		})
	}
}

func TestTitleMatchesQuery_RejectsSpinoffsAndWrongAnime(t *testing.T) {
	tests := []struct {
		torrent string
		query   string
	}{
		// Spinoff with a very different title (short query ≤3 tokens, threshold 0.8)
		{"[SubsPlease] Sword Art Online Alternative Gun Gale Online - 05 (1080p)", "Sword Art Online"},
		// A different anime with words in common
		{"Kemono Jihen Episode 5", "Kemono Friends"},
		{"Manaria Friends Episode 5", "Kemono Friends"},
		// Partial title (part of the name missing)
		{"Silent Witch Episode 3", "Silent Witch: Chinmoku no Majo no Kakushigoto"},
		{"Chinmoku no Majo no Kakushigoto Episode 3", "Silent Witch: Chinmoku no Majo no Kakushigoto"},
		// Anime completamente diferente
		{"Different Anime - 05 [1080p]", "Kemono Friends"},
		// Same Erai-raws metadata format but the wrong title: filtered technical tokens must not mask the
		// absence of the correct title token.
		{"[Erai-raws] Different Show - 07 [1080p NF WEB-DL AVC AAC][MultiSub][AABBCCDD]", "Akane-banashi"},
		{"[Erai-raws] Other Anime - 06 [1080p AMZN WEB-DL AVC EAC3][MultiSub][11223344]", "Ichijyoma Mankitsu Gurashi"},
	}
	for _, tt := range tests {
		t.Run(tt.torrent, func(t *testing.T) {
			if nyaa.TitleMatchesQuery(tt.torrent, tt.query) {
				t.Errorf("TitleMatchesQuery(%q, %q) = true, want false", tt.torrent, tt.query)
			}
		})
	}
}

func TestNyaaModule_DoesNotReturnSpinoffs(t *testing.T) {
	options := testOptions{
		animeName: "Sword Art Online",
		episode:   5,
		correct: []string{
			"[SubsPlease] Sword Art Online - 05 (1080p)",
			"Sword Art Online Episode 5",
		},
		incorrect: []string{
			"[SubsPlease] Sword Art Online Alternative Gun Gale Online - 05 (1080p)",
			"[SubsPlease] Sword Art Online Progressive - 05 (1080p)",
		},
	}
	runEpisodeNameTest(options, t)
}

// ============================================
// TESTS FOR THE SEASON/PART FEATURE
// ============================================

// TestExtractPart checks extracting the part/cour number from torrent names and AniList titles
func TestExtractPart_ExtractsFromVariousFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected *int
	}{
		// Formats found in the Nyaa samples
		{"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 2 - 07 [1080p]", intPtr(2)},
		{"[EMBER] Hataraku Maou-sama! (2023) (Season 2 | Part 02) [1080p]", intPtr(2)},
		{"[EMBER] Hataraku Maou-sama! (2022) (Season 2 | Part 01) [1080p]", intPtr(1)},
		// A name that declares TWO parts isn't the first one's: the pack covers both halves, and
		// returning the first made the part filter reject it precisely for the second. Without a single
		// number the range's coverage decides (daemon.packAxis, decisions.md #79).
		{"[DB] NieR:Automata Ver1.1a (Season 1 Part 1+2) [Dual Audio]", nil},
		{"[EMBER] Shingeki no Kyojin (2023) (Season 4 Part 03+04) [BDRip]", nil},
		{"[Fuchs] Anime - S02 (Season 2) (Part 1 + Part 2) [BDRip]", nil},
		// Two declarations of the SAME number are still that number.
		{"[EMBER] Hataraku Maou-sama! (Season 2 | Part 02) (Part 2) [1080p]", intPtr(2)},
		// AniList titles (for ExtractAnimeSeasonPart)
		{"Shingeki no Kyojin Season 3 Part 2", intPtr(2)},
		{"Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", intPtr(2)},
		{"NieR:Automata Ver1.1a Cour 2", intPtr(2)},
		// No part
		{"[SubsPlease] Hataraku Maou-sama S2 - 13 (1080p)", nil},
		{"[SubsPlease] Shingeki no Kyojin - 07 (1080p)", nil},
		{"Kaguya-sama wa Kokurasetai?: Tensaitachi no Renai Zunousen", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := nyaa.ExtractPart(tt.input)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("ExtractPart(%q) = %d, want nil", tt.input, *got)
				}
			} else {
				if got == nil {
					t.Errorf("ExtractPart(%q) = nil, want %d", tt.input, *tt.expected)
				} else if *got != *tt.expected {
					t.Errorf("ExtractPart(%q) = %d, want %d", tt.input, *got, *tt.expected)
				}
			}
		})
	}
}

// TestNyaaModule_KaguySa2_AcceptsSeason2Torrents confirms the bug fix: Kaguya S2 (romaji without a
// season number) now accepts S02 torrents when season=2 is passed.
func TestNyaaModule_KaguySa2_AcceptsSeason2Torrents(t *testing.T) {
	season2 := 2
	options := testOptions{
		// AniList id 112641: the romaji has no season number, the season comes from the synonyms
		animeName: "Kaguya-sama wa Kokurasetai",
		episode:   3,
		season:    &season2,
		correct: []string{
			"[MTBB] Kaguya-sama wa Kokurasetai S2 - 03 (1080p)",
			"Kaguya-sama wa Kokurasetai Season 2 Episode 3",
			"[SubsPlease] Kaguya-sama wa Kokurasetai S02 - 03 (1080p)",
		},
		incorrect: []string{
			// Season 1 must be rejected (an explicit season=2 filters it)
			"[SubsPlease] Kaguya-sama wa Kokurasetai S01 - 03 (1080p)",
			"Kaguya-sama wa Kokurasetai Season 1 Episode 3",
			// Wrong episode
			"[MTBB] Kaguya-sama wa Kokurasetai S2 - 05 (1080p)",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_SnKS3P2_SeasonAndPartFilter checks the combined season=3 + part=2 filter (AniList
// id 104578: Shingeki no Kyojin Season 3 Part 2)
func TestNyaaModule_SnKS3P2_SeasonAndPartFilter(t *testing.T) {
	season3 := 3
	part2 := 2
	options := testOptions{
		animeName: "Shingeki no Kyojin Season 3 Part 2",
		episode:   5,
		season:    &season3,
		part:      &part2,
		correct: []string{
			"[Erai-raws] Shingeki no Kyojin Season 3 Part 2 - 05 [1080p]",
		},
		incorrect: []string{
			// Part 1 must be rejected (hard part filter)
			"[Erai-raws] Shingeki no Kyojin Season 3 Part 1 - 05 [1080p]",
			// Wrong season
			"[SubsPlease] Shingeki no Kyojin Season 2 Part 2 - 05 (1080p)",
			// Wrong episode
			"[Erai-raws] Shingeki no Kyojin Season 3 Part 2 - 07 [1080p]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_MushokuTenseiIIP2_PartFilter checks the season=2 + part=2 filter (AniList id
// 166873: Mushoku Tensei II Part 2). ExtractAnimeSeasonPart returns season=2 (from the English
// title) and part=2 (from the romaji/English)
func TestNyaaModule_MushokuTenseiIIP2_PartFilter(t *testing.T) {
	season2 := 2
	part2 := 2
	options := testOptions{
		animeName: "Mushoku Tensei II Isekai Ittara Honki Dasu Part 2",
		episode:   7,
		season:    &season2,
		part:      &part2,
		correct: []string{
			"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 2 - 07 [1080p]",
		},
		incorrect: []string{
			// Part 1 must be rejected
			"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 1 - 07 [1080p]",
			// No part marker: rejected by the hard filter when a part is requested
			"[SubsPlease] Mushoku Tensei II - 07 (1080p)",
			// Wrong episode
			"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 2 - 03 [1080p]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_NieRCour2_CourTreatedAsPart checks that "Cour 2" is treated as part=2 (AniList id
// 167420: NieR:Automata Ver1.1a Cour 2)
func TestNyaaModule_NieRCour2_CourTreatedAsPart(t *testing.T) {
	season2 := 2
	part2 := 2
	options := testOptions{
		// English title: "NieR:Automata Ver1.1a Cour 2" → ExtractPart = 2. Romaji title: "NieR:Automata
		// Ver1.1a 2nd Season" → ExtractSeason = 2
		animeName: "NieR Automata Ver1.1a Cour 2",
		episode:   5,
		season:    &season2,
		part:      &part2,
		correct: []string{
			"[SubsPlease] NieR Automata Ver1.1a Cour 2 - 05 (1080p)",
		},
		incorrect: []string{
			"[SubsPlease] NieR Automata Ver1.1a Cour 1 - 05 (1080p)",
			"[SubsPlease] NieR Automata Ver1.1a - 05 (1080p)",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_RomanNumeralSequel_AcceptsSeasonFromTitleItself checks the bug fix: sequels whose
// title uses only a roman numeral (e.g. "... II") as the season marker, with no "Season 2"/"S2"
// anywhere and without the fansub restating it in the torrent name, are now accepted (AniList id
// 194829: Katainaka no Ossan, Kensei ni Naru II).
func TestNyaaModule_RomanNumeralSequel_AcceptsSeasonFromTitleItself(t *testing.T) {
	season2 := 2
	options := testOptions{
		animeName: "Katainaka no Ossan, Kensei ni Naru II",
		episode:   1,
		season:    &season2,
		correct: []string{
			"[Erai-raws] Katainaka no Ossan, Kensei ni Naru II - 01 [1080p AMZN WEB-DL AVC EAC3][MultiSub][13F1C02B]",
		},
		incorrect: []string{
			// Wrong episode
			"[Erai-raws] Katainaka no Ossan, Kensei ni Naru II - 02 [1080p AMZN WEB-DL AVC EAC3][MultiSub][13F1C02B]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_HatarakuS2P2_PartHardFilter checks that the hard filter works correctly for
// Hataraku Maou-sama season 2 part 2 (AniList id 155168)
func TestNyaaModule_HatarakuS2P2_PartHardFilter(t *testing.T) {
	season2 := 2
	part2 := 2

	// Hard filter: with requestedPart=2, torrents without a part marker OR with part=1 are rejected
	html := mockHtml([]string{
		"[SubsPlease] Hataraku Maou-sama S2 - 01 (1080p)",        // ep=1, part=nil → rejeitado
		"[EMBER] Hataraku Maou-sama! (Season 2 | Part 01) Batch", // part=1 → rejeitado (isBatch)
		"[EMBER] Hataraku Maou-sama! (Season 2 | Part 02) Batch", // part=2 → rejeitado (isBatch)
		"[Erai-raws] Hataraku Maou-sama Part 2 - 01 [1080p]",     // part=2, ep=1 → aceito
	})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaa("Hataraku Maou-sama", 1, &season2, &part2)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	for _, r := range results {
		// No result may have part=nil or part != 2
		if r.Part == nil || *r.Part != 2 {
			t.Errorf("expected part=2, got %v in torrent: %s", r.Part, r.Name)
		}
		// No result may have season != 2
		if r.Season != nil && *r.Season != 2 {
			t.Errorf("expected season=2, got %d in torrent: %s", *r.Season, r.Name)
		}
	}

	found := false
	for _, r := range results {
		if r.Name == "[Erai-raws] Hataraku Maou-sama Part 2 - 01 [1080p]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Part 2 torrent to be found, got %d results", len(results))
	}
}

// TestNyaaModule_PartFilter_NoPartRequested checks that without requestedPart, torrents with any
// part are accepted
func TestNyaaModule_PartFilter_NoPartRequested(t *testing.T) {
	options := testOptions{
		animeName: "My Show Season 2",
		episode:   5,
		season:    intPtr(2),
		correct: []string{
			"My Show Season 2 Part 1 Episode 5",
			"My Show Season 2 Part 2 Episode 5",
			"My Show Season 2 Episode 5",
		},
	}
	runEpisodeNameTest(options, t)
}

func intPtr(n int) *int { return &n }

// A pack WITHOUT a part marker survives a "Part 2" search: the pack is the whole season (it's
// AniList that splits the season into cours), and requiring the marker zeroed the packs of every
// "Part N" entry — that's what left Mushoku Tensei II Part 2 without any pack. Only a declared
// conflict rejects; whether the pack serves is decided by the range's coverage, in the daemon
// (decisions.md #79).
func TestScrapNyaaForAnime_PackWithoutPartMarkerSurvivesPartRequest(t *testing.T) {
	options := []string{
		"[Judas] Mushoku Tensei - 01 ~ 23 [BD 1080p][Batch]",
		"[EMBER] Mushoku Tensei (Season 1 | Part 1) 01 ~ 11 [1080p][Batch]",
		"[EMBER] Mushoku Tensei (Season 1 | Part 2) 01 ~ 12 [1080p][Batch]",
	}

	restore := mockHttpGet(mockHtml(options))
	defer restore()

	part2 := 2
	results, err := nyaa.ScrapNyaaForAnime("Mushoku Tensei", []int{1}, nil, &part2)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}

	packs, _ := packsAndSingles(results)
	got := make(map[string]bool, len(packs))
	for _, p := range packs {
		got[p.Name] = true
	}
	if !got[options[0]] {
		t.Fatalf("a pack without a part marker should survive, got %v", got)
	}
	if got[options[1]] {
		t.Fatalf("a part 1 pack should not survive a part 2 search, got %v", got)
	}
	if !got[options[2]] {
		t.Fatalf("a part 2 pack should survive, got %v", got)
	}
}

// packsAndSingles reproduces the partition the daemon does over ScrapNyaaForAnime's single list, so
// the tests of the two old searches keep holding without changing their assertions.
func packsAndSingles(results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult) {
	for _, r := range results {
		switch {
		case r.IsBatch:
			packs = append(packs, r)
		case r.Episode != nil:
			singles = append(singles, r)
		}
	}
	return packs, singles
}

// The unification must not change the query: reSeasonNamePatterns (batch) and
// reSeasonStrip+rePartStrip (multi) removed the SAME set (Season N, S N, Nth Season, Cour N, Part
// N). This test guarantees the unified sanitizing doesn't regress.
func TestScrapNyaaForAnime_QueryMatchesTheTwoOldSearches(t *testing.T) {
	titles := []struct{ in, wantQuery string }{
		{"Show", "Show"},
		{"Show Season 2", "Show"},
		{"Show S2", "Show"},
		{"Show 2nd Season", "Show"},
		{"Show Cour 2", "Show"},
		{"Show Part 2", "Show"},
	}

	for _, tc := range titles {
		var gotURL string
		restore := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
			gotURL = url
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(mockHtml(nil)))}, nil
		})
		if _, err := nyaa.ScrapNyaaForAnime(tc.in, []int{1}, nil, nil); err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		restore()

		if !strings.Contains(gotURL, "q="+url.QueryEscape(tc.wantQuery)+"&") && !strings.HasSuffix(gotURL, "q="+url.QueryEscape(tc.wantQuery)) {
			t.Errorf("%q: want query %q in the URL, got %q", tc.in, tc.wantQuery, gotURL)
		}
	}
}

// A page with a pack and an episode returns BOTH in the same list, distinguishable by IsBatch /
// Episode — the partition in the daemon reproduces what the two old functions returned.
func TestScrapNyaaForAnime_ReturnsPacksAndEpisodesInOneList(t *testing.T) {
	html := mockHtml([]string{
		"[SubsPlease] Frieren (01-28) [1080p]",
		"[SubsPlease] Frieren - 05 [1080p]",
		"[SubsPlease] Frieren - 09 [1080p]",
	})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Frieren", []int{5}, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}

	var packs, singles int
	for _, r := range results {
		switch {
		case r.IsBatch:
			packs++
		case r.Episode != nil:
			singles++
			if *r.Episode != 5 {
				t.Errorf("an episode outside the requested list got in: %d", *r.Episode)
			}
		default:
			t.Errorf("result with neither IsBatch nor Episode: %q", r.Name)
		}
	}
	if packs != 1 {
		t.Errorf("want 1 pack, got %d", packs)
	}
	if singles != 1 {
		t.Errorf("want 1 episode (only 5 was requested), got %d", singles)
	}
}

// The pagination floor (enoughCandidates = 3) counts BOTH lists together.
func TestScrapNyaaForAnime_PaginationFloorCountsBothLists(t *testing.T) {
	pages := 0
	restore := nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		pages++
		html := mockHtml([]string{
			"[G] Show (01-12) [1080p]",
			"[G] Show (13-24) [1080p]",
			"[G] Show - 05 [1080p]",
		})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html))}, nil
	})
	defer restore()

	if _, err := nyaa.ScrapNyaaForAnime("Show", []int{5}, nil, nil); err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if pages != 1 {
		t.Errorf("2 packs + 1 episode already reach the floor: want 1 page, got %d", pages)
	}
}

// rowWithSize is getRow with the size cell filled — the default getRow leaves td[3] empty (Size 0),
// which is precisely the case that passes the ceiling freely.
func rowWithSize(name, size string) string {
	return strings.Replace(getRow(name), "<td></td>\n\t  <td>"+time.Now().Format("2006-01-02 15:04"),
		"<td>"+size+"</td>\n\t  <td>"+time.Now().Format("2006-01-02 15:04"), 1)
}

func pageWithRows(rows ...string) string {
	return `<!doctype html><html><body><table class="torrent-list"><tbody>` +
		strings.Join(rows, "\n") + `</tbody></table></body></html>`
}

// A pack above max_batch_torrent_size_gb must not count toward the pagination floor: that was the
// One Piece case, where the giant packs of the first pages ended the descent and the partial ones
// that fit were never seen.
func TestScrapNyaaForAnime_OversizedBatchDoesNotStopPagination(t *testing.T) {
	defer nyaa.SetMaxSearchPages(5)()
	defer nyaa.SetMaxBatchTorrentSizeGB(100)()

	var urls []string
	defer mockHttpGetPerURL(func(url string) string {
		if strings.Contains(url, "&p=2") {
			return pageWithRows(rowWithSize("[G] One Piece 575-782 [1080p] (Batch)", "61.0 GiB"))
		}
		if strings.Contains(url, "&p=") {
			return pageWithRows()
		}
		return pageWithRows(
			rowWithSize("[G] One Piece 001-1100 [1080p] (Batch)", "587.5 GiB"),
			rowWithSize("[G] One Piece 001-574 [1080p] (Batch)", "171.8 GiB"),
			rowWithSize("[G] One Piece 0892-1089 [1080p] (Batch)", "190.3 GiB"),
		)
	}, &urls)()

	results, err := nyaa.ScrapNyaaForAnime("One Piece", nil, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(urls) < 2 {
		t.Fatalf("3 giant packs must not end the descent: want >=2 fetches, got %d: %v", len(urls), urls)
	}
	if len(results) != 1 {
		t.Fatalf("want only the 61 GiB pack, got %d: %v", len(results), results)
	}
	if !strings.Contains(results[0].Name, "575-782") {
		t.Fatalf("want the 61 GiB partial pack, got %q", results[0].Name)
	}
}

// Ceiling off (0) keeps the old behavior: a giant pack goes in and counts toward the floor.
func TestScrapNyaaForAnime_BatchSizeCeilingOffKeepsOversizedPacks(t *testing.T) {
	defer nyaa.SetMaxSearchPages(5)()
	defer nyaa.SetMaxBatchTorrentSizeGB(0)()

	var urls []string
	defer mockHttpGetPerURL(func(string) string {
		return pageWithRows(
			rowWithSize("[G] One Piece 001-1100 [1080p] (Batch)", "587.5 GiB"),
			rowWithSize("[G] One Piece 001-574 [1080p] (Batch)", "171.8 GiB"),
			rowWithSize("[G] One Piece 0892-1089 [1080p] (Batch)", "190.3 GiB"),
		)
	}, &urls)()

	results, err := nyaa.ScrapNyaaForAnime("One Piece", nil, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("with the ceiling off the 3 packs go in, got %d", len(results))
	}
	if len(urls) != 1 {
		t.Fatalf("3 accepted packs reach the floor: want 1 fetch, got %d", len(urls))
	}
}

// --- F2: alternative title after the season marker ---

// The real case that motivated the fix: EMBER puts the "Season 2" marker inside "(Season 2 | Part
// 2)" and the full romaji three groups later, in parentheses. Cutting at the marker left only
// [mushoku tensei jobless reincarnation], and the romaji query was rejected for a missing token —
// the ONLY Part 2 packs died before reaching the batch path.
func TestTitleMatchesQuery_AltTitleAfterMarker(t *testing.T) {
	const romaji = "Mushoku Tensei II: Isekai Ittara Honki Dasu"
	tests := []struct {
		name    string
		torrent string
		query   string
	}{
		{"EMBER pack part 2, romaji in parentheses",
			"[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [BDRip] [1080p Dual Audio HEVC 10 bits DDP] (Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2) (Batch)",
			romaji},
		{"EMBER pack part 1",
			"[EMBER] Mushoku Tensei: Jobless Reincarnation (2023) (Season 2 | Part 1 + Special) [1080p] [Dual Audio HEVC WEBRip DDP] (Mushoku Tensei II: Isekai Ittara Honki Dasu) (Batch)",
			romaji},
		{"Diddy season pack, romaji after a loose bar",
			"[Diddy] Mushoku Tensei - S02 (BD 1080p HEVC Opus) [Dual Audio] | Mushoku Tensei II: Isekai Ittara Honki Dasu | Jobless Reincarnation Season 2",
			romaji},
		{"NTRX season pack, romaji after a loose bar",
			"[NTRX] Mushoku Tensei: Jobless Reincarnation S02 (Season 2) REPACK (BD Remux 1080p AVC FLAC 2.0) [Dual Audio] | Mushoku Tensei II: Isekai Ittara Honki Dasu",
			romaji},
		{"VARYG loose episode, romaji in parentheses after the episode title",
			"Mushoku Tensei Jobless Reincarnation S02E07 The Kidnapping and Confinement of Beast Girls 1080p NF WEB-DL AAC2.0 H 264-VARYG (Mushoku Tensei II: Isekai Ittara Honki Dasu, Multi-Subs)",
			romaji},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !nyaa.TitleMatchesQuery(tt.torrent, tt.query) {
				t.Errorf("TitleMatchesQuery(%q, %q) = false, want true", tt.torrent, tt.query)
			}
		})
	}
}

// The pair of the test above: the second attempt widens the tokens, it doesn't loosen the match. An
// alternative title after the marker that is NOT the requested anime is still rejected.
func TestTitleMatchesQuery_AltTitleDoesNotAcceptWrongAnime(t *testing.T) {
	tests := []struct{ torrent, query string }{
		// A different sequel: the romaji in the footer says III, the query asks for II.
		{"[Erai-raws] Mushoku Tensei III: Isekai Ittara Honki Dasu - 01 [1080p CR WEB-DL AVC AAC][MultiSub][07335B96]",
			"Mushoku Tensei II: Isekai Ittara Honki Dasu"},
		// A real season 2 pack against the season 3 query: the footer carries the romaji, but with the
		// wrong numeral. That's the token check 1 requires and doesn't find.
		{"[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [BDRip] [1080p Dual Audio HEVC 10 bits DDP] (Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2) (Batch)",
			"Mushoku Tensei III: Isekai Ittara Honki Dasu"},
	}
	for _, tt := range tests {
		t.Run(tt.torrent, func(t *testing.T) {
			if nyaa.TitleMatchesQuery(tt.torrent, tt.query) {
				t.Errorf("TitleMatchesQuery(%q, %q) = true, want false", tt.torrent, tt.query)
			}
		})
	}
}

// Old anime is often released as one torrent named only by the title and the source. Large and
// numberless, it is an unlabeled pack; small and numberless (a web short) it stays out.
func TestScrapNyaaForAnime_LargeNumberlessRowIsUnlabeledPack(t *testing.T) {
	html := pageWithRows(
		rowWithSize("[man] Azumanga Daioh [BD 1080p HEVC FLAC]", "19.2 GiB"),
		rowWithSize("[Obscurerips] Azumanga Web Daioh", "18.9 MiB"),
	)
	defer mockHttpGet(html)()

	results, err := nyaa.ScrapNyaaForAnime("Azumanga Daioh", []int{1, 2}, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(results) != 1 || !results[0].IsBatch || !results[0].UnlabeledPack {
		t.Fatalf("want only the BD release, as an unlabeled pack; got %+v", results)
	}
}

// The base title goes first: the roman-numeral variants never match "…Darou ka S5 - 08".
func TestGenerateSearchTitleVariants_RomanSeasonBaseFirst(t *testing.T) {
	got := nyaa.GenerateSearchTitleVariants("Dungeon ni Deai wo Motomeru no wa Machigatteiru Darou ka V: Houjou no Megami-hen", "Is It Wrong to Try to Pick Up Girls in a Dungeon? V")
	if len(got) == 0 || got[0] != "Dungeon ni Deai wo Motomeru no wa Machigatteiru Darou ka" {
		t.Errorf("want the base title first, got %v", got)
	}
	// The English title doesn't carry the numeral: a name, not a sequel.
	if got := nyaa.GenerateSearchTitleVariants("Lupin III", "Lupin the 3rd"); got[0] == "Lupin" {
		t.Errorf("Lupin III must keep its variants, got %v", got)
	}
}
