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

func TestGetMagnetWithoutError(t *testing.T) {
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

// func TestGetMagnetOfFullAnime(t *testing.T) {
// 	// TODO: Implementar esse teste
// 	// set html with torrents for specific episodes and full series
// 	// module must pick full series torrent
// }

func TestGetCorrectAnime(t *testing.T) {
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

func TestGetEpisodeOfCompleteAnime(t *testing.T) {
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

func TestGetCorrectMagnetsOfSeason1Anime(t *testing.T) {
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

func TestGetCorrectMagnetsOfSeason2Anime(t *testing.T) {
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

func TestGetCorrectMagnetsOfSeason3Anime(t *testing.T) {
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
