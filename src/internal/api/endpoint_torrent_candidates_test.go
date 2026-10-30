package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
)

const movieMediaJSON = `{"data":{"Media":{
	"id": 600, "episodes": 1, "format": "MOVIE", "status": "FINISHED",
	"title": {"english": "Standalone Movie", "romaji": "Standalone Movie"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

const frierenSeason2MediaJSON = `{"data":{"Media":{
	"id": 700, "episodes": 24, "format": "TV", "status": "FINISHED",
	"title": {"english": "Frieren Season 2", "romaji": "Sousou no Frieren Season 2"},
	"synonyms": [], "relations": {"edges": []},
	"coverImage": {"large": "", "medium": ""},
	"airingSchedule": {"nodes": []}
}}}`

func candidatesConfig() *files.Config {
	c := standaloneConfig()
	c.MinSeeders = 1
	c.MaxBatchTorrentSizeGB = 100
	c.MaxEpisodeTorrentSizeGB = 5
	return c
}

func candidatesServer() *Server {
	return standaloneWriteServer(&mockFileManager{configs: candidatesConfig()})
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
			t.Errorf("candidates must not fetch /view: %s", u)
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

func getTorrentCandidates(t *testing.T, server *Server, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/animes/" + id + "/torrent-candidates"
	if query != "" {
		path += "?q=" + url.QueryEscape(query)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handleTorrentCandidates(server)(rec, req)
	return rec
}

func getTorrentCandidateDetail(t *testing.T, server *Server, id, nyaaID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/"+id+"/torrent-candidates/"+nyaaID, nil)
	req.SetPathValue("id", id)
	req.SetPathValue("nyaaId", nyaaID)
	rec := httptest.NewRecorder()
	handleTorrentCandidateDetail(server)(rec, req)
	return rec
}

func decodeCandidates(t *testing.T, rec *httptest.ResponseRecorder) TorrentCandidatesResponse {
	t.Helper()
	var resp struct {
		Data TorrentCandidatesResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	return resp.Data
}

func decodeCandidateDetail(t *testing.T, rec *httptest.ResponseRecorder) TorrentCandidateDetail {
	t.Helper()
	var resp struct {
		Data TorrentCandidateDetail `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	return resp.Data
}

func TestTorrentCandidates_RankedPacks(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()

	bestPack := "[SubsPlease] Standalone Anime (01-28) [1080p]"
	single := "[SubsPlease] Standalone Anime - 01 [1080p]"
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(2000, "[Erai-raws] Standalone Anime (01-12) [720p]", "8.0 GiB", "400", "10"),
		previewListingRow(1001, bestPack, "12.0 GiB", "50", "4"),
		previewListingRow(3000, single, "300 MiB", "200", "5"),
	), nil)()

	rec := getTorrentCandidates(t, candidatesServer(), "21", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	data := decodeCandidates(t, rec)
	if data.Query == "" {
		t.Fatal("query must be the title variant that was searched")
	}
	if len(data.Items) == 0 {
		t.Fatal("want ranked packs, got none")
	}
	for _, item := range data.Items {
		if item.Name == single {
			t.Fatal("series candidates must discard singles")
		}
		if !item.IsBatch {
			t.Fatalf("series candidates must be packs only, got %q", item.Name)
		}
	}
	if data.Items[0].Name != bestPack {
		t.Fatalf("index 0: want 1080p pack %q, got %q", bestPack, data.Items[0].Name)
	}
	got := data.Items[0]
	if got.RangeStart != 1 || got.RangeEnd != 28 {
		t.Fatalf("range from name 01-28: want 1-28, got %d-%d", got.RangeStart, got.RangeEnd)
	}
	if got.NyaaID != 1001 {
		t.Fatalf("nyaa_id: want 1001, got %d", got.NyaaID)
	}
	if got.Magnet == "" {
		t.Fatal("magnet must be filled")
	}
	if got.Seeders != 50 {
		t.Fatalf("seeders: want 50, got %d", got.Seeders)
	}
	if got.Fansub != "subsplease" {
		t.Fatalf("fansub: want subsplease, got %q", got.Fansub)
	}
	if got.Resolution != "1080p" {
		t.Fatalf("resolution: want 1080p, got %q", got.Resolution)
	}
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
}

func TestTorrentCandidates_EmptyIs200(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()
	defer mockEmptyNyaa()()

	rec := getTorrentCandidates(t, candidatesServer(), "21", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	data := decodeCandidates(t, rec)
	if data.Items == nil {
		t.Fatal("items must be [] not null")
	}
	if len(data.Items) != 0 {
		t.Fatalf("want no items, got %d", len(data.Items))
	}
	if data.Query == "" {
		t.Fatal("query must still be the searched variant")
	}
}

func TestTorrentCandidates_CustomQueryHitsNyaa(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, frierenSeason2MediaJSON)()

	pack := "[Judas] Frieren (01-28) [1080p]"
	var urls []string
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(1664, pack, "10.0 GiB", "80", "3"),
	), &urls)()

	query := "[judas] frieren"
	rec := getTorrentCandidates(t, candidatesServer(), "700", query)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	data := decodeCandidates(t, rec)
	if data.Query != query {
		t.Fatalf("query: want %q, got %q", query, data.Query)
	}
	if len(data.Items) != 1 || data.Items[0].Name != pack {
		t.Fatalf("pack without a season marker must survive custom q, got %+v", data.Items)
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

func TestTorrentCandidates_MovieSetsNyaaID(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, movieMediaJSON)()

	movie := "[SubsPlease] Standalone Movie (2022) [1080p]"
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(2001, movie, "1.5 GiB", "120", "6"),
	), nil)()

	rec := getTorrentCandidates(t, candidatesServer(), "600", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	data := decodeCandidates(t, rec)
	if len(data.Items) != 1 {
		t.Fatalf("want 1 movie row, got %d", len(data.Items))
	}
	if data.Items[0].Name != movie {
		t.Fatalf("want %q, got %q", movie, data.Items[0].Name)
	}
	if data.Items[0].NyaaID != 2001 {
		t.Fatalf("movie nyaa_id: want 2001, got %d", data.Items[0].NyaaID)
	}
}

func TestTorrentCandidates_UnknownMediaIs404(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, `{"data":{"Media":null}}`)()
	defer mockEmptyNyaa()()

	rec := getTorrentCandidates(t, candidatesServer(), "999999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "ANIME_NOT_FOUND" {
		t.Fatalf("want ANIME_NOT_FOUND, got %s", got)
	}
}

func TestTorrentCandidates_NyaaUnavailable(t *testing.T) {
	defer mockStandaloneAniList(emptyListJSON, standaloneMediaJSON)()
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})()

	rec := getTorrentCandidates(t, candidatesServer(), "21", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "NYAA_UNAVAILABLE" {
		t.Fatalf("want NYAA_UNAVAILABLE, got %s", got)
	}
}

func TestTorrentCandidates_InvalidID(t *testing.T) {
	rec := getTorrentCandidates(t, candidatesServer(), "abc", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "INVALID_ID" {
		t.Fatalf("want INVALID_ID, got %s", got)
	}
}

const torrentDetailHTML = `<!doctype html><html><body>
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

func TestTorrentCandidateDetail_SanitisesHTMLAndListsFiles(t *testing.T) {
	defer nyaa.MockNyaaHttpGet(func(u string) (*http.Response, error) {
		if !strings.HasSuffix(u, "/view/1664122") {
			t.Errorf("unexpected url %s", u)
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(torrentDetailHTML)),
			Header:     make(http.Header),
		}, nil
	})()

	rec := getTorrentCandidateDetail(t, candidatesServer(), "21", "1664122")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	d := decodeCandidateDetail(t, rec)
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
		t.Errorf("nyaa_url = %q", d.NyaaURL)
	}
	if len(d.Files) != 2 {
		t.Fatalf("files = %#v, want 2 (video and subtitle; folders are not rows)", d.Files)
	}
	if d.Files[0].Name != "Frieren/Frieren - 01.mkv" {
		t.Errorf("nested name = %q", d.Files[0].Name)
	}
	if d.Files == nil {
		t.Fatal("files must be [] not null")
	}
}

func TestTorrentCandidateDetail_FilesEmptySliceNotNull(t *testing.T) {
	html := `<!doctype html><html><body>
<div id="torrent-description"><p>no files</p></div>
</body></html>`
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(html)),
			Header:     make(http.Header),
		}, nil
	})()

	rec := getTorrentCandidateDetail(t, candidatesServer(), "21", "42")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	d := decodeCandidateDetail(t, rec)
	if d.Files == nil {
		t.Fatal("files must be [] not null when the page has no list")
	}
	if len(d.Files) != 0 {
		t.Fatalf("want no files, got %#v", d.Files)
	}
}

func TestTorrentCandidateDetail_NonNumericNyaaID(t *testing.T) {
	rec := getTorrentCandidateDetail(t, candidatesServer(), "21", "abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "INVALID_NYAA_ID" {
		t.Fatalf("want INVALID_NYAA_ID, got %s", got)
	}
}

func TestTorrentCandidateDetail_InvalidAnimeID(t *testing.T) {
	rec := getTorrentCandidateDetail(t, candidatesServer(), "nope", "1664122")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "INVALID_ID" {
		t.Fatalf("want INVALID_ID, got %s", got)
	}
}

func TestTorrentCandidateDetail_Nyaa404(t *testing.T) {
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})()

	rec := getTorrentCandidateDetail(t, candidatesServer(), "21", "1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "TORRENT_NOT_FOUND" {
		t.Fatalf("want TORRENT_NOT_FOUND, got %s", got)
	}
}

func TestTorrentCandidateDetail_NyaaUnavailable(t *testing.T) {
	defer nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})()

	rec := getTorrentCandidateDetail(t, candidatesServer(), "21", "1")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "NYAA_UNAVAILABLE" {
		t.Fatalf("want NYAA_UNAVAILABLE, got %s", got)
	}
}
