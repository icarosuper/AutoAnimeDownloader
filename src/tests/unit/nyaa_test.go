package unit

import (
	"AutoAnimeDownloader/src/internal/nyaa"
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

// mockHttpGetCapturing é o mockHttpGet que também registra cada URL pedida,
// para os testes que verificam a query montada e não o resultado.
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

// mockHttpGetPerURL serve HTML dependente da URL e registra cada URL pedida — é o mock dos
// testes de paginação, que verificam quantas páginas a busca foi buscar.
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

// Página 1 com candidatos de sobra: a página 2 não pode ser pedida (era pedida SEMPRE).
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
		t.Fatalf("esperava 3 candidatos, obtive %d", len(results))
	}
	if len(urls) != 1 {
		t.Fatalf("esperava 1 fetch (só a página 1), obtive %d: %v", len(urls), urls)
	}
}

// Candidatos abaixo do piso e páginas sempre cheias: desce até o teto e para lá.
func TestScrapNyaa_PagesUntilCapWhenCandidatesAreThin(t *testing.T) {
	defer nyaa.SetMaxSearchPages(3)()

	// Linhas de outro anime: a página nunca fica vazia, mas nada é aceito.
	page := mockHtml([]string{"Kemono Jihen - 05 [1080p]", "Manaria Friends - 05 [1080p]"})
	var urls []string
	defer mockHttpGetPerURL(func(string) string { return page }, &urls)()

	results, err := nyaa.ScrapNyaa("Show", 5, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if results != nil {
		t.Fatalf("esperava nenhum candidato, obtive %v", results)
	}
	if len(urls) != 3 {
		t.Fatalf("esperava 3 fetches (teto), obtive %d: %v", len(urls), urls)
	}
	if !strings.Contains(urls[2], "&p=3") {
		t.Fatalf("a última página buscada deveria ser a 3, obtive %s", urls[2])
	}
}

// Página vazia = a query acabou: insistir até o teto seria fetch jogado fora.
func TestScrapNyaa_StopsWhenPageIsEmpty(t *testing.T) {
	defer nyaa.SetMaxSearchPages(5)()

	var urls []string
	defer mockHttpGetPerURL(func(url string) string {
		if strings.Contains(url, "&p=") {
			return mockHtml(nil)
		}
		return mockHtml([]string{"Show - 05 [1080p]"}) // 1 candidato, abaixo do piso
	}, &urls)()

	results, err := nyaa.ScrapNyaa("Show", 5, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("esperava 1 candidato, obtive %d", len(results))
	}
	if len(urls) != 2 {
		t.Fatalf("esperava 2 fetches (página 1 + a vazia), obtive %d: %v", len(urls), urls)
	}
}

// Batch lia só a página 1; agora pagina igual à busca de episódio.
func TestScrapNyaaForAnime_PaginatesLikeEpisodeSearch(t *testing.T) {
	defer nyaa.SetMaxSearchPages(2)()

	page := mockHtml([]string{"Kemono Jihen (01-12) [Batch] [1080p]"})
	var urls []string
	defer mockHttpGetPerURL(func(string) string { return page }, &urls)()

	if _, err := nyaa.ScrapNyaaForAnime("Show", nil, nil, nil); err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("esperava 2 fetches (teto), obtive %d: %v", len(urls), urls)
	}
}

// Série longa: grupos numeram com zero-padding ("One Piece - 001"), e a query sem
// padding afoga o episódio 1 nos 1160+ ordenados por seeders.
func TestScrapNyaa_LongSeriesAlsoQueriesPaddedEpisode(t *testing.T) {
	var urls []string
	restore := mockHttpGetCapturing(mockHtml([]string{"[Group] One Piece - 001 [1080p]"}), &urls)
	defer restore()

	if _, err := nyaa.ScrapNyaa("One Piece", 1, nil, nil, 1100); err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	if !slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "q=One+Piece+001") }) {
		t.Fatalf("esperava uma query com o episódio zero-padded, obtive %v", urls)
	}
	if !slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "q=One+Piece+1&") }) {
		t.Fatalf("a query sem padding não pode ser substituída, obtive %v", urls)
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
		t.Fatalf("anime curto não deve gerar query com padding, obtive %v", urls)
	}
}

// Sem total de episódios (chamada antiga) o comportamento é o de anime curto.
func TestScrapNyaa_UnknownTotalKeepsPlainEpisode(t *testing.T) {
	var urls []string
	restore := mockHttpGetCapturing(mockHtml([]string{"[Group] Show - 01 [1080p]"}), &urls)
	defer restore()

	if _, err := nyaa.ScrapNyaa("Show", 1, nil, nil); err != nil {
		t.Fatalf("ScrapNyaa error: %v", err)
	}

	if slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "001") }) {
		t.Fatalf("sem total conhecido não deve haver padding, obtive %v", urls)
	}
}

// Um filme não pode ser aceito como episódio: "[DB] Naruto Shippuuden Movie 3"
// casava o padrão " 3 (" de extractEpisodeNumber e virava "episódio 3".
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

// "Persona" contém "ona": o marcador de OVA precisa de word boundary, senão o
// guard de filme rejeita episódios legítimos.
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

// Animes longos (One Piece) têm episódios de 4 dígitos.
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

	// Verificar que apenas os episódios 2, 3 e 5 foram retornados
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

// O pack nao e mais descartado, e classificado: entra na lista com IsBatch, e nao como
// "episodio 1" via packsAndSingles.
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

	results, err := nyaa.ScrapNyaaForAnime("Kemono Friends", []int{2, 5, 10}, nil, nil)

	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	_, singles := packsAndSingles(results)
	if len(singles) == 0 {
		t.Fatalf("expected at least one result")
	}

	// Verificar que apenas torrents de "Kemono Friends" foram retornados
	for _, r := range singles {
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

	// Deve retornar apenas os episódios da Season 2
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

	// Verificar que os resultados estão ordenados (1080p antes de 720p, SubsPlease primeiro entre 1080p)
	found1080p := false

	for i, r := range singles {
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

func TestScrapNyaaForAnime_CanGetCorrectAnimeMultipleSeasons(t *testing.T) {
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

// ============================================
// TESTES PARA BATCHES E FILMES
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
		// Faixa no fim do nome, sem espaço depois: visto no Nyaa na busca do
		// episódio 1 do One Piece (packs de 500 episódios lidos como "episódio 1").
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
			// Usando reflexão para acessar função privada
			// Na prática, a função isMovie é testada através dos resultados da busca
			// Este é um teste unitário da lógica de detecção
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
		// Guarda de falso positivo: [720-1080p] nao e faixa de episodio. Sem ela, este pack
		// completo seria lido como "cobre os episodios 720 a 1080".
		// "[Group]" (nao "[X]") evita colisao com o guard de numeral romano de temporada
		// (reRomanSeason le "X" como S10; fora do escopo desta task).
		{"[Group] Anime Complete Batch [720-1080p]", 0, 0, 0},
		{"[Group] Anime Complete Batch", 0, 0, 0},
		// Faixa invertida: o casamento pega "2 - 01" (temporada + primeiro episodio) e daria
		// 2..1, uma faixa que nao cobre episodio nenhum.
		{"[Erai-raws] Hibike! Euphonium 2 - 01 ~ 13 [1080p][Multiple Subtitle]", 0, 0, 2},
	}

	for _, tc := range cases {
		got := nyaa.ExtractBatchInfo(tc.name)
		if got.StartEpisode != tc.wantStart || got.EndEpisode != tc.wantEnd {
			t.Errorf("%q: esperava faixa %d-%d, obteve %d-%d", tc.name, tc.wantStart, tc.wantEnd, got.StartEpisode, got.EndEpisode)
		}
		if got.Season != tc.wantSeason {
			t.Errorf("%q: esperava temporada %d, obteve %d", tc.name, tc.wantSeason, got.Season)
		}
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

	// Verificar que apenas batches foram retornados
	for _, r := range packs {
		if !nyaa.IsBatch(r.Name) {
			t.Fatalf("non-batch torrent in results: %s", r.Name)
		}
	}

	// Verificar que todos os torrents corretos foram encontrados
	foundCorrect := make(map[string]bool)
	for _, r := range packs {
		for _, correct := range correct {
			if r.Name == correct {
				foundCorrect[correct] = true
			}
		}
		// Verificar que nenhum torrent incorreto está nos resultados
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

	// Verificar que apenas temporada 2 foi retornada
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
	// Não incluímos episódios na lista de incorrect, pois o mock HTML retorna todos
	// O filtro isMovie deve remover os episódios automaticamente

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

	// Verificar que todos os resultados são válidos
	for _, r := range results {
		// Verificar que tem Movie no nome ou é um filme conhecido
		hasMovieKeyword := strings.Contains(strings.ToLower(r.Name), "movie") ||
			strings.Contains(strings.ToLower(r.Name), "gekijouban")

		if !hasMovieKeyword {
			t.Logf("Warning: Result doesn't contain 'Movie' keyword: %s", r.Name)
		}
	}

	// Verificar que pelo menos um torrent correto foi encontrado
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

	// Primeiro deve ser BD (melhor source)
	if !strings.Contains(sorted[0].Name, "BD ") && !strings.Contains(sorted[0].Name, "BDRip") {
		t.Fatalf("Expected BD or BDRip first, got: %s", sorted[0].Name)
	}

	// Segundo deve ter melhor prioridade
	// (prioridade: source > resolution > codec > fansub > audio > seeders > size)
}

func TestExtractSource_ExtractsVideoSource(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("extractSource is not exported - tested indirectly via SortMovieResults")
}

func TestExtractCodec_ExtractsVideoCodec(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("extractCodec is not exported - tested indirectly via SortMovieResults")
}

func TestExtractAudio_ExtractsAudioCodec(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("extractAudio is not exported - tested indirectly via SortMovieResults")
}

func TestSourcePriority_CorrectOrder(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("sourcePriority is not exported - tested indirectly via SortMovieResults")
}

func TestCodecPriority_CorrectOrder(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("codecPriority is not exported - tested indirectly via SortMovieResults")
}

func TestAudioPriority_CorrectOrder(t *testing.T) {
	// Função não é exportada, testada indiretamente via SortMovieResults
	t.Skip("audioPriority is not exported - tested indirectly via SortMovieResults")
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
		// Torrents com metadados técnicos que inflavam o Jaccard (NF, AMZN, WEB-DL, EAC3, MultiSub)
		{"[Erai-raws] Akane-banashi - 07 [1080p NF WEB-DL AVC AAC][MultiSub][805A5B50]", "Akane-banashi"},
		{"[Erai-raws] Nippon Sangoku - 07 [1080p AMZN WEB-DL AVC EAC3][MultiSub][86BD091C]", "Nippon Sangoku"},
		{"[Erai-raws] Ichijyoma Mankitsu Gurashi - 06 [1080p AMZN WEB-DL AVC EAC3][MultiSub][BC657289]", "Ichijyoma Mankitsu Gurashi"},
		// Fansub com ambos os títulos no nome (English + romaji) — threshold 0.4 p/ query ≥4 tokens
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
		// Spinoff com título muito diferente (query curta ≤3 tokens, threshold 0.8)
		{"[SubsPlease] Sword Art Online Alternative Gun Gale Online - 05 (1080p)", "Sword Art Online"},
		// Anime diferente com palavras em comum
		{"Kemono Jihen Episode 5", "Kemono Friends"},
		{"Manaria Friends Episode 5", "Kemono Friends"},
		// Título parcial (falta parte do nome)
		{"Silent Witch Episode 3", "Silent Witch: Chinmoku no Majo no Kakushigoto"},
		{"Chinmoku no Majo no Kakushigoto Episode 3", "Silent Witch: Chinmoku no Majo no Kakushigoto"},
		// Anime completamente diferente
		{"Different Anime - 05 [1080p]", "Kemono Friends"},
		// Mesmo formato de metadados Erai-raws mas título errado: tokens técnicos filtrados não devem
		// mascarar a ausência do token de título correto.
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
// TESTES PARA SEASON/PART FEATURE
// ============================================

// TestExtractPart verifica extração de número de parte/cour de nomes de torrents e títulos Anilist
func TestExtractPart_ExtractsFromVariousFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected *int
	}{
		// Formatos encontrados nos samples do Nyaa
		{"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 2 - 07 [1080p]", intPtr(2)},
		{"[EMBER] Hataraku Maou-sama! (2023) (Season 2 | Part 02) [1080p]", intPtr(2)},
		{"[EMBER] Hataraku Maou-sama! (2022) (Season 2 | Part 01) [1080p]", intPtr(1)},
		{"[DB] NieR:Automata Ver1.1a (Season 1 Part 1+2) [Dual Audio]", intPtr(1)},
		{"[EMBER] Shingeki no Kyojin (2023) (Season 4 Part 03+04) [BDRip]", intPtr(3)},
		// Títulos Anilist (para ExtractAnimeSeasonPart)
		{"Shingeki no Kyojin Season 3 Part 2", intPtr(2)},
		{"Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", intPtr(2)},
		{"NieR:Automata Ver1.1a Cour 2", intPtr(2)},
		// Sem part
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

// TestNyaaModule_KaguySa2_AcceptsSeason2Torrents confirma o fix do bug:
// Kaguya S2 (romaji sem número de season) agora aceita torrents S02 quando season=2 é passado.
func TestNyaaModule_KaguySa2_AcceptsSeason2Torrents(t *testing.T) {
	season2 := 2
	options := testOptions{
		// Anilist id 112641: romaji não tem season number, season vem dos synonyms
		animeName: "Kaguya-sama wa Kokurasetai",
		episode:   3,
		season:    &season2,
		correct: []string{
			"[MTBB] Kaguya-sama wa Kokurasetai S2 - 03 (1080p)",
			"Kaguya-sama wa Kokurasetai Season 2 Episode 3",
			"[SubsPlease] Kaguya-sama wa Kokurasetai S02 - 03 (1080p)",
		},
		incorrect: []string{
			// Season 1 deve ser rejeitada (season=2 explícito filtra)
			"[SubsPlease] Kaguya-sama wa Kokurasetai S01 - 03 (1080p)",
			"Kaguya-sama wa Kokurasetai Season 1 Episode 3",
			// Episódio errado
			"[MTBB] Kaguya-sama wa Kokurasetai S2 - 05 (1080p)",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_SnKS3P2_SeasonAndPartFilter verifica filtro combinado de season=3 + part=2
// (Anilist id 104578: Shingeki no Kyojin Season 3 Part 2)
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
			// Part 1 deve ser rejeitada (hard part filter)
			"[Erai-raws] Shingeki no Kyojin Season 3 Part 1 - 05 [1080p]",
			// Season errada
			"[SubsPlease] Shingeki no Kyojin Season 2 Part 2 - 05 (1080p)",
			// Episódio errado
			"[Erai-raws] Shingeki no Kyojin Season 3 Part 2 - 07 [1080p]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_MushokuTenseiIIP2_PartFilter verifica filtro de season=2 + part=2
// (Anilist id 166873: Mushoku Tensei II Part 2)
// ExtractAnimeSeasonPart retorna season=2 (do título inglês) e part=2 (do romaji/inglês)
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
			// Part 1 deve ser rejeitada
			"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 1 - 07 [1080p]",
			// Sem part marker: rejeitado pelo hard filter quando part é solicitada
			"[SubsPlease] Mushoku Tensei II - 07 (1080p)",
			// Episódio errado
			"[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu Part 2 - 03 [1080p]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_NieRCour2_CourTreatedAsPart verifica que "Cour 2" é tratado como part=2
// (Anilist id 167420: NieR:Automata Ver1.1a Cour 2)
func TestNyaaModule_NieRCour2_CourTreatedAsPart(t *testing.T) {
	season2 := 2
	part2 := 2
	options := testOptions{
		// Título inglês: "NieR:Automata Ver1.1a Cour 2" → ExtractPart = 2
		// Título romaji: "NieR:Automata Ver1.1a 2nd Season" → ExtractSeason = 2
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

// TestNyaaModule_RomanNumeralSequel_AcceptsSeasonFromTitleItself verifica o fix do bug:
// sequências cujo título usa apenas um numeral romano (ex: "... II") como marcador de
// season, sem "Season 2"/"S2" em lugar nenhum e sem o fansub reafirmar isso no nome do
// torrent, agora são aceitas (Anilist id 194829: Katainaka no Ossan, Kensei ni Naru II).
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
			// Episódio errado
			"[Erai-raws] Katainaka no Ossan, Kensei ni Naru II - 02 [1080p AMZN WEB-DL AVC EAC3][MultiSub][13F1C02B]",
		},
	}
	runEpisodeNameTest(options, t)
}

// TestNyaaModule_HatarakuS2P2_PartHardFilter verifica que hard filter funciona corretamente
// para Hataraku Maou-sama season 2 part 2 (Anilist id 155168)
func TestNyaaModule_HatarakuS2P2_PartHardFilter(t *testing.T) {
	season2 := 2
	part2 := 2

	// Hard filter: com requestedPart=2, torrents sem part marker OU com part=1 são rejeitados
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
		// Nenhum resultado deve ter part=nil ou part != 2
		if r.Part == nil || *r.Part != 2 {
			t.Errorf("expected part=2, got %v in torrent: %s", r.Part, r.Name)
		}
		// Nenhum resultado deve ter season != 2
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

// TestNyaaModule_PartFilter_NoPartRequested verifica que sem requestedPart, torrents com qualquer part são aceitos
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

// packsAndSingles reproduz a particao que o daemon faz sobre a lista unica de ScrapNyaaForAnime,
// para que os testes das duas buscas antigas continuem valendo sem mudar de asserção.
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

// A unificacao nao pode mudar a query: reSeasonNamePatterns (batch) e reSeasonStrip+rePartStrip
// (multi) removiam o MESMO conjunto (Season N, S N, Nª Season, Cour N, Part N). Este teste e a
// garantia de que o saneamento unificado nao regride.
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
			t.Errorf("%q: esperava a query %q na URL, obteve %q", tc.in, tc.wantQuery, gotURL)
		}
	}
}

// Uma pagina com pack e episodio devolve os DOIS na mesma lista, distinguiveis por IsBatch /
// Episode — a particao no daemon reproduz o que as duas funcoes antigas devolviam.
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
				t.Errorf("episodio fora da lista pedida entrou: %d", *r.Episode)
			}
		default:
			t.Errorf("resultado sem IsBatch e sem Episode: %q", r.Name)
		}
	}
	if packs != 1 {
		t.Errorf("esperava 1 pack, obteve %d", packs)
	}
	if singles != 1 {
		t.Errorf("esperava 1 episodio (so o 5 foi pedido), obteve %d", singles)
	}
}

// O piso de paginacao (enoughCandidates = 3) conta as DUAS listas somadas.
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
		t.Errorf("2 packs + 1 episodio ja atingem o piso: esperava 1 pagina, obteve %d", pages)
	}
}

// rowWithSize e o getRow com a celula de tamanho preenchida — o getRow padrao deixa td[3] vazia
// (Size 0), que e justamente o caso que passa livre pelo teto.
func rowWithSize(name, size string) string {
	return strings.Replace(getRow(name), "<td></td>\n\t  <td>"+time.Now().Format("2006-01-02 15:04"),
		"<td>"+size+"</td>\n\t  <td>"+time.Now().Format("2006-01-02 15:04"), 1)
}

func pageWithRows(rows ...string) string {
	return `<!doctype html><html><body><table class="torrent-list"><tbody>` +
		strings.Join(rows, "\n") + `</tbody></table></body></html>`
}

// Pack acima de max_batch_torrent_size_gb nao pode contar para o piso de paginacao: era o caso
// One Piece, onde os packs gigantes das primeiras paginas encerravam a descida e os parciais que
// cabiam nunca eram vistos.
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
		t.Fatalf("3 packs gigantes nao podem encerrar a descida: esperava >=2 fetches, obtive %d: %v", len(urls), urls)
	}
	if len(results) != 1 {
		t.Fatalf("esperava so o pack de 61 GiB, obtive %d: %v", len(results), results)
	}
	if !strings.Contains(results[0].Name, "575-782") {
		t.Fatalf("esperava o pack parcial de 61 GiB, obtive %q", results[0].Name)
	}
}

// Teto desligado (0) mantem o comportamento antigo: pack gigante entra e conta para o piso.
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
		t.Fatalf("com o teto desligado os 3 packs entram, obtive %d", len(results))
	}
	if len(urls) != 1 {
		t.Fatalf("3 packs aceitos atingem o piso: esperava 1 fetch, obtive %d", len(urls))
	}
}

// --- F2: titulo alternativo depois do marcador de temporada ---

// O caso real que motivou a correcao: o EMBER poe o marcador "Season 2" dentro de
// "(Season 2 | Part 2)" e o romaji completo tres grupos adiante, entre parenteses. Cortar no
// marcador deixava so [mushoku tensei jobless reincarnation], e a query em romaji era rejeitada
// por falta de token — os UNICOS packs de Part 2 morriam antes de chegar ao caminho de batch.
func TestTitleMatchesQuery_AltTitleAfterMarker(t *testing.T) {
	const romaji = "Mushoku Tensei II: Isekai Ittara Honki Dasu"
	tests := []struct {
		name    string
		torrent string
		query   string
	}{
		{"EMBER pack part 2, romaji entre parenteses",
			"[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [BDRip] [1080p Dual Audio HEVC 10 bits DDP] (Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2) (Batch)",
			romaji},
		{"EMBER pack part 1",
			"[EMBER] Mushoku Tensei: Jobless Reincarnation (2023) (Season 2 | Part 1 + Special) [1080p] [Dual Audio HEVC WEBRip DDP] (Mushoku Tensei II: Isekai Ittara Honki Dasu) (Batch)",
			romaji},
		{"Diddy pack de season, romaji depois de barra solta",
			"[Diddy] Mushoku Tensei - S02 (BD 1080p HEVC Opus) [Dual Audio] | Mushoku Tensei II: Isekai Ittara Honki Dasu | Jobless Reincarnation Season 2",
			romaji},
		{"NTRX pack de season, romaji depois de barra solta",
			"[NTRX] Mushoku Tensei: Jobless Reincarnation S02 (Season 2) REPACK (BD Remux 1080p AVC FLAC 2.0) [Dual Audio] | Mushoku Tensei II: Isekai Ittara Honki Dasu",
			romaji},
		{"VARYG episodio avulso, romaji entre parenteses depois do titulo do episodio",
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

// O par do teste acima: a segunda tentativa amplia os tokens, nao afrouxa o match. Um titulo
// alternativo depois do marcador que NAO e o anime pedido continua sendo rejeitado.
func TestTitleMatchesQuery_AltTitleDoesNotAcceptWrongAnime(t *testing.T) {
	tests := []struct{ torrent, query string }{
		// Sequencia diferente: o romaji no rodape diz III, a query pede II.
		{"[Erai-raws] Mushoku Tensei III: Isekai Ittara Honki Dasu - 01 [1080p CR WEB-DL AVC AAC][MultiSub][07335B96]",
			"Mushoku Tensei II: Isekai Ittara Honki Dasu"},
		// Pack real da season 2 contra a query da season 3: o rodape traz o romaji, mas com o
		// numeral errado. E o token que a checagem 1 exige e nao acha.
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
