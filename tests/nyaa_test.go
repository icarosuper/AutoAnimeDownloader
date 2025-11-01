package tests

import (
	"AutoAnimeDownloader/modules/nyaa"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testOptions struct {
	animeName string
	episode   int
	correct   []string
	incorrect []string
}

func getRow(name string) string {
	parsedName := strings.ReplaceAll(name, " ", ".")

	return `<tr>
	  <td></td>
	  <td><a title="` + parsedName + `">` + name + `</a></td>
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

func TestNyaaModule_CanGetMagnet(t *testing.T) {
	html := mockHtml([]string{"My.Show.S01E02.1080p"})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaa("My.Show", 2)
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
	options := testOptions{
		animeName: "Machikado Mazoku Season 2",
		episode:   7,
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
	options := testOptions{
		animeName: "SPY x FAMILY Season 3",
		episode:   3,
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

func runEpisodeNameTest(opt testOptions, t *testing.T) {
	html := mockHtml(append(opt.correct, opt.incorrect...))
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaa(opt.animeName, opt.episode)

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

	// Verificar se está ordenado corretamente: 1080p > 720p > 480p
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

	// Verificar se está ordenado corretamente por fansub: SubsPlease > Erai-Raws > Judas > ToonsHub > ASW
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

	// Verificar se 1080p vem antes de 720p, independente do fansub
	// Entre os 1080p, SubsPlease deve vir primeiro
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

	// Torrents com resolução conhecida devem vir antes dos sem resolução
	if sorted[0].Resolution == nil {
		t.Fatalf("expected first result to have resolution, got nil")
	}
	if sorted[1].Resolution != nil {
		t.Fatalf("expected last result to have nil resolution, got %s", *sorted[1].Resolution)
	}
}

func TestScrapNyaaForMultipleEpisodes_CanGetMultipleEpisodes(t *testing.T) {
	options := []string{
		"My.Show.S01E02.1080p",
		"My.Show.S01E03.1080p",
		"My.Show.S01E05.1080p",
		"My.Show.S01E10.1080p",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForMultipleEpisodes("My.Show", []int{2, 3, 5})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verificar que apenas os episódios 2, 3 e 5 foram retornados
	episodes := make(map[int]bool)
	for _, r := range results {
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

func TestScrapNyaaForMultipleEpisodes_CanFilterByAnimeTitle(t *testing.T) {
	options := testOptions{
		animeName: "Kemono Friends",
		episode:   0, // não usado neste teste
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

	results, err := nyaa.ScrapNyaaForMultipleEpisodes("Kemono Friends", []int{2, 5, 10})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	// Verificar que apenas torrents de "Kemono Friends" foram retornados
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.Name), "kemono friends") {
			t.Fatalf("unexpected anime in results: %s", r.Name)
		}

		// Verificar que nenhum torrent incorreto está nos resultados
		for _, incorrect := range options.incorrect {
			if r.Name == incorrect {
				t.Fatalf("found incorrect torrent in results: %s", r.Name)
			}
		}
	}
}

func TestScrapNyaaForMultipleEpisodes_ReturnsNilWhenNoResults(t *testing.T) {
	html := mockHtml([]string{"Some.Other.Show.S01E01.1080p"})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForMultipleEpisodes("My.Show", []int{2, 3, 5})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results when no episodes found, got %v", results)
	}
}

func TestScrapNyaaForMultipleEpisodes_WithSeasonFiltering(t *testing.T) {
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
	results, err := nyaa.ScrapNyaaForMultipleEpisodes("Show Season 2", []int{5, 10})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}

	// Deve retornar apenas os episódios da Season 2
	for _, r := range results {
		if r.Season == nil || *r.Season != 2 {
			t.Fatalf("expected season 2, got %v in %s", r.Season, r.Name)
		}
	}
}

func TestScrapNyaaForMultipleEpisodes_ResultsAreSorted(t *testing.T) {
	options := []string{
		"Show [SubsPlease] Episode 2 720p",
		"Show [Judas] Episode 2 1080p",
		"Show [Ember] Episode 2 1080p",
		"Show [SubsPlease] Episode 2 1080p",
	}

	html := mockHtml(options)
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForMultipleEpisodes("Show", []int{2})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	// Verificar que os resultados estão ordenados (1080p antes de 720p, SubsPlease primeiro entre 1080p)
	found1080p := false

	for i, r := range results {
		if r.Resolution != nil && *r.Resolution == "1080p" {
			found1080p = true
			// SubsPlease deve ser o primeiro resultado 1080p
			if i == 0 && !strings.Contains(r.Name, "SubsPlease") {
				t.Fatalf("expected first result to be SubsPlease 1080p")
			}
		}
		if r.Resolution != nil && *r.Resolution == "720p" {
			// 720p deve vir depois de 1080p
			if found1080p && i == 0 {
				t.Fatalf("expected 1080p to come before 720p")
			}
		}
	}
}

func TestScrapNyaaForMultipleEpisodes_CanGetCorrectAnimeMultipleSeasons(t *testing.T) {
	options := testOptions{
		animeName: "Lucky Star",
		episode:   0, // não usado neste teste
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

	results, err := nyaa.ScrapNyaaForMultipleEpisodes("Lucky Star", []int{5, 10, 15})

	if err != nil {
		t.Fatalf("ScrapNyaaForMultipleEpisodes error: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	foundCorrect := make(map[string]bool)
	for _, r := range results {
		// Nenhum torrent incorreto deve estar nos resultados
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
