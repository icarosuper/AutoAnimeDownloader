package daemon

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
)

const mediaForMovie600 = `{"data": {"Media": {
	"id": 600, "format": "MOVIE", "status": "FINISHED", "episodes": 1,
	"title": {"english": "Standalone Movie", "romaji": "Standalone Movie"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

const mediaForFrierenSeason2 = `{"data": {"Media": {
	"id": 700, "format": "TV", "status": "FINISHED", "episodes": 24,
	"title": {"english": "Frieren Season 2", "romaji": "Sousou no Frieren Season 2"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

func previewTestConfig() *files.Config {
	return &files.Config{
		MinSeeders:              1,
		MaxBatchTorrentSizeGB:   100,
		MaxEpisodeTorrentSizeGB: 5,
	}
}

func previewListingRow(viewID int, name, size, seeders, leechers string) string {
	return fmt.Sprintf(`<tr>
	  <td></td>
	  <td><a href="/view/%d" title="%s">%s</a></td>
	  <td><a></a><a href="magnet:?xt=urn:btih:%d">magnet</a></td>
	  <td>%s</td>
	  <td>2024-01-01 10:00</td>
	  <td>%s</td>
	  <td>%s</td>
	</tr>`, viewID, name, name, viewID, size, seeders, leechers)
}

func previewListingHTML(rows ...string) string {
	return `<!doctype html><html><body><table class="torrent-list"><tbody>` +
		strings.Join(rows, "\n") +
		`</tbody></table></body></html>`
}

func mockNyaaListing(t *testing.T, html string, urls *[]string) func() {
	t.Helper()
	empty := previewListingHTML()
	return nyaa.MockNyaaHttpGet(func(u string) (*http.Response, error) {
		if urls != nil {
			*urls = append(*urls, u)
		}
		if strings.Contains(u, "/view/") {
			t.Errorf("preview must not fetch /view: %s", u)
		}
		body := html
		if strings.Contains(u, "&p=") {
			body = empty
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
}

func TestPreviewWholeTitleTorrents_SeriesPacksOnlyRankedAndCapped(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	bestPack := "[SubsPlease] Standalone Anime (01-28) [1080p]"
	single := "[SubsPlease] Standalone Anime - 01 [1080p]"
	rows := []string{
		previewListingRow(2000, "[Erai-raws] Standalone Anime (01-12) [720p]", "8.0 GiB", "400", "10"),
	}
	for i := 0; i < 8; i++ {
		rows = append(rows, previewListingRow(2100+i,
			fmt.Sprintf("[ASW] Standalone Anime (01-%d) [720p]", 12+i),
			"6.0 GiB", "80", "2"))
	}
	rows = append(rows,
		previewListingRow(1001, bestPack, "12.0 GiB", "50", "4"),
		previewListingRow(3000, single, "300 MiB", "200", "5"),
	)
	var urls []string
	defer mockNyaaListing(t, previewListingHTML(rows...), &urls)()

	preview, err := PreviewWholeTitleTorrents(previewTestConfig(), 500, "")
	if err != nil {
		t.Fatalf("PreviewWholeTitleTorrents: %v", err)
	}
	used, items := preview.Query, preview.Items
	if used == "" {
		t.Fatal("usedQuery must be the title variant that was searched")
	}
	if preview.Media.Title != "Standalone Anime" {
		t.Fatalf("media title: got %q", preview.Media.Title)
	}
	if preview.Media.Format != "TV" {
		t.Fatalf("media format: got %q", preview.Media.Format)
	}
	if len(items) == 0 {
		t.Fatal("want ranked packs, got none")
	}
	if len(items) > maxTorrentCandidates {
		t.Fatalf("cap is %d, got %d items", maxTorrentCandidates, len(items))
	}
	if len(items) != maxTorrentCandidates {
		t.Fatalf("9 passing packs should cap at %d, got %d", maxTorrentCandidates, len(items))
	}

	for _, item := range items {
		if item.Name == single {
			t.Fatal("series preview must discard singles")
		}
		if !item.IsBatch {
			t.Fatalf("series preview must be packs only, got %q", item.Name)
		}
	}

	res1080 := "1080p"
	res720 := "720p"
	ranked := nyaa.SortTorrentResults([]nyaa.TorrentResult{
		{Name: bestPack, Seeders: "50", Leechers: 4, Resolution: &res1080, Size: 12 << 30},
		{Name: "[Erai-raws] Standalone Anime (01-12) [720p]", Seeders: "400", Leechers: 10, Resolution: &res720, Size: 8 << 30},
	})
	if items[0].Name != ranked[0].Name {
		t.Fatalf("index 0: want %q (SortTorrentResults winner), got %q", ranked[0].Name, items[0].Name)
	}
	if items[0].RangeStart != 1 || items[0].RangeEnd != 28 {
		t.Fatalf("range from name 01-28: want 1-28, got %d-%d", items[0].RangeStart, items[0].RangeEnd)
	}
	if items[0].NyaaID != 1001 {
		t.Fatalf("NyaaID from DetailURL: want 1001, got %d", items[0].NyaaID)
	}
	if items[0].Magnet == "" {
		t.Fatal("magnet must be filled")
	}
	if items[0].Seeders != 50 {
		t.Fatalf("seeders: want 50, got %d", items[0].Seeders)
	}
}

func TestPreviewWholeTitleTorrents_EmptyListingIsNotAnError(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()
	defer mockEmptyNyaa()()

	preview, err := PreviewWholeTitleTorrents(previewTestConfig(), 500, "")
	if err != nil {
		t.Fatalf("empty listing is not an error: %v", err)
	}
	used, items := preview.Query, preview.Items
	if len(items) != 0 {
		t.Fatalf("want no items, got %d", len(items))
	}
	if used == "" {
		t.Fatal("usedQuery must still be the searched variant")
	}
}

func TestPreviewWholeTitleTorrents_CustomQuerySkipsVariantsAndSeasonPart(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForFrierenSeason2)()

	pack := "[Judas] Frieren (01-28) [1080p]"
	var urls []string
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(1664, pack, "10.0 GiB", "80", "3"),
	), &urls)()

	query := "[judas] frieren"
	preview, err := PreviewWholeTitleTorrents(previewTestConfig(), 700, query)
	if err != nil {
		t.Fatalf("PreviewWholeTitleTorrents: %v", err)
	}
	used, items := preview.Query, preview.Items
	if used != query {
		t.Fatalf("usedQuery: want %q, got %q", query, used)
	}
	if len(items) != 1 || items[0].Name != pack {
		t.Fatalf("pack without a season marker must survive nil,nil scrape, got %+v", items)
	}

	if len(urls) == 0 {
		t.Fatal("expected a Nyaa search")
	}
	encoded := url.QueryEscape(query)
	for _, u := range urls {
		if !strings.Contains(u, "q="+encoded) && !strings.Contains(u, "q="+strings.ReplaceAll(encoded, "+", "%20")) {
			t.Fatalf("Nyaa URL must carry the custom q, got %s", u)
		}
		if strings.Contains(strings.ToLower(u), "season") || strings.Contains(u, "sousou") {
			t.Fatalf("custom q must skip title variants and season, got %s", u)
		}
	}
}

func TestPreviewWholeTitleTorrents_MovieSetsNyaaID(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForMovie600)()

	movie := "[SubsPlease] Standalone Movie (2022) [1080p]"
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(2001, movie, "1.5 GiB", "120", "6"),
	), nil)()

	preview, err := PreviewWholeTitleTorrents(previewTestConfig(), 600, "")
	if err != nil {
		t.Fatalf("PreviewWholeTitleTorrents: %v", err)
	}
	items := preview.Items
	if len(items) != 1 {
		t.Fatalf("want 1 movie row, got %d", len(items))
	}
	if items[0].Name != movie {
		t.Fatalf("want %q, got %q", movie, items[0].Name)
	}
	if items[0].NyaaID != 2001 {
		t.Fatalf("movie NyaaID: want 2001, got %d", items[0].NyaaID)
	}
}

func TestPreviewWholeTitleTorrents_UnknownMedia(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, `{"data": {"Media": null}}`)()
	defer mockEmptyNyaa()()

	preview, err := PreviewWholeTitleTorrents(previewTestConfig(), 999999, "")
	if err == nil {
		t.Fatal("unknown AniList id must error")
	}
	if !strings.Contains(err.Error(), "anime 999999 does not exist on AniList") {
		t.Fatalf("want the same error DownloadStandaloneAnime uses, got %v", err)
	}
	if preview.Items != nil {
		t.Fatalf("want no items on unknown media, got %+v", preview.Items)
	}
}

func TestPreviewWholeTitleTorrents_NyaaUnavailable(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})()

	_, err := PreviewWholeTitleTorrents(previewTestConfig(), 500, "")
	if err == nil {
		t.Fatal("Nyaa transport failure must error")
	}
	if !errors.Is(err, nyaa.ErrNyaaUnavailable) {
		t.Fatalf("want ErrNyaaUnavailable, got %v", err)
	}
}
