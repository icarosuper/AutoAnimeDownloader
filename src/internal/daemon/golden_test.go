package daemon

// Golden set for the search/match pipeline: every diagnosed case, replayed offline through the same
// RunAnimeDebug pass `make debug-anime` runs. Each case is a folder in testdata/golden/:
//
//   case.json  anime id, why the case exists, and the torrent each searched episode must pick
//              ("picked": "" = it must find nothing)
//   http.json  every AniList and Nyaa response the pass got when it was recorded
//
// Recording (live network, read-only): `make golden-record ID=<mediaId> NAME=<slug>`. It runs on
// files.DefaultConfig() with no accounts, so the outcome depends on neither the recorder's config
// nor their lists. A new case.json comes out with what the pipeline does NOW: when the case is a
// bug, edit "picked" to the right torrent first, watch TestGoldenSet fail, then fix. Re-recording an
// existing case rewrites only http.json and keeps its "searched", so a hand-set pick survives; the
// log says where the fresh outcome differs.
//
// GOLDEN_UPDATE=1 rewrites every case.json from the current outcome — only after reading why each
// pick moved; `git diff testdata/golden` is the review.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

const goldenDir = "testdata/golden"

type goldenCase struct {
	AnimeID int `json:"anime_id"`
	// Note says what went wrong and what the case pins. Mandatory: a case nobody can explain gets
	// "updated" the first time it fails instead of read.
	Note     string      `json:"note"`
	Searched []goldenRun `json:"searched"`
}

// goldenEpisode is one episode the pass searched. Picked "" means it must find nothing.
type goldenEpisode struct {
	Episode int
	Picked  string
}

// goldenRun is how case.json spells the outcome: consecutive episodes with the same pick are one
// entry ("1-11", or "5" alone), since a pack covers a run and the same 75-character name repeated
// per episode buries the one line that differs.
type goldenRun struct {
	Episodes string `json:"episodes"`
	Picked   string `json:"picked"`
}

// eachRun walks n items sorted by episode and calls emit once per run: consecutive episodes for
// which same(first, j) holds, spelled "N" or "N-M" like case.json. first is the run's first index.
func eachRun(n int, episode func(i int) int, same func(first, j int) bool, emit func(span string, first int)) {
	for i := 0; i < n; {
		j := i
		for j+1 < n && episode(j+1) == episode(j)+1 && same(i, j+1) {
			j++
		}
		span := strconv.Itoa(episode(i))
		if j > i {
			span += "-" + strconv.Itoa(episode(j))
		}
		emit(span, i)
		i = j + 1
	}
}

func toRuns(eps []goldenEpisode) []goldenRun {
	runs := []goldenRun{}
	eachRun(len(eps),
		func(i int) int { return eps[i].Episode },
		func(first, j int) bool { return eps[j].Picked == eps[first].Picked },
		func(span string, first int) {
			runs = append(runs, goldenRun{Episodes: span, Picked: eps[first].Picked})
		})
	return runs
}

func fromRuns(t *testing.T, runs []goldenRun) []goldenEpisode {
	t.Helper()
	var eps []goldenEpisode
	for _, r := range runs {
		lo, hi, isRange := strings.Cut(r.Episodes, "-")
		first, err := strconv.Atoi(lo)
		last := first
		if err == nil && isRange {
			last, err = strconv.Atoi(hi)
		}
		if err != nil || last < first {
			t.Fatalf("episodes %q: want \"N\" or \"N-M\"", r.Episodes)
		}
		for ep := first; ep <= last; ep++ {
			eps = append(eps, goldenEpisode{Episode: ep, Picked: r.Picked})
		}
	}
	return eps
}

// goldenExchange is one recorded request. URL is the request URI without scheme and host, so a
// recording replays the same under NYAA_URL; Body (AniList's GraphQL request) tells the AniList
// queries apart, since they all POST to the same URL.
type goldenExchange struct {
	Method   string `json:"method"`
	URL      string `json:"url"`
	Body     string `json:"body,omitempty"`
	Status   int    `json:"status"`
	Response string `json:"response"`
}

// requestURI drops scheme and host, so a recording replays the same under NYAA_URL.
func requestURI(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.RequestURI()
	}
	return rawURL
}

func exchangeKey(method, rawURL, body string) string {
	return method + " " + requestURI(rawURL) + "\n" + body
}

// runGoldenPass runs the debug pass on the default config, with the nyaa package globals the config
// pushes applied for the duration — through the same files.ApplyNyaaSettings LoadConfigs calls.
func runGoldenPass(t *testing.T, animeID int) *DebugSummary {
	t.Helper()
	cfg := files.DefaultConfig()
	defer files.ApplyNyaaSettings(cfg)()

	summary, err := RunAnimeDebug(animeID, cfg, &debugMockFileManager{})
	if err != nil {
		t.Fatalf("RunAnimeDebug(%d): %v", animeID, err)
	}
	return summary
}

func goldenOutcome(summary *DebugSummary) []goldenEpisode {
	out := []goldenEpisode{}
	for _, ep := range summary.Episodes {
		if ep.WouldSearch {
			out = append(out, goldenEpisode{Episode: ep.Episode, Picked: ep.Picked})
		}
	}
	return out
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenSet(t *testing.T) {
	dirs, _ := filepath.Glob(filepath.Join(goldenDir, "*", "case.json"))
	if len(dirs) == 0 {
		t.Fatalf("no cases under %s", goldenDir)
	}
	for _, casePath := range dirs {
		dir := filepath.Dir(casePath)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			var c goldenCase
			readJSON(t, casePath, &c)
			if strings.TrimSpace(c.Note) == "" || strings.HasPrefix(c.Note, "TODO") {
				t.Errorf("%s: write the note — what went wrong and what this case pins", casePath)
			}
			var recorded []goldenExchange
			readJSON(t, filepath.Join(dir, "http.json"), &recorded)

			got := goldenOutcome(replayGolden(t, c.AnimeID, recorded))
			if os.Getenv("GOLDEN_UPDATE") != "" {
				if t.Failed() { // requests missing from the recording got a 404: not an outcome to keep
					return
				}
				c.Searched = toRuns(got)
				writeJSON(t, casePath, c)
				return
			}
			diffGolden(t, fromRuns(t, c.Searched), got)
		})
	}
}

// diffGolden reports every episode whose pick moved, collapsed into runs like case.json: a pack that
// stops winning is one line, not twelve.
func diffGolden(t *testing.T, want, got []goldenEpisode) {
	t.Helper()
	diffGoldenWith(t.Errorf, want, got)
}

func diffGoldenWith(report func(format string, args ...any), want, got []goldenEpisode) {
	const notSearched = "\x00"
	byEpisode := map[int]*[2]string{} // [want, got]
	set := func(eps []goldenEpisode, side int) {
		for _, e := range eps {
			if byEpisode[e.Episode] == nil {
				byEpisode[e.Episode] = &[2]string{notSearched, notSearched}
			}
			byEpisode[e.Episode][side] = e.Picked
		}
	}
	set(want, 0)
	set(got, 1)
	sorted := make([]int, 0, len(byEpisode))
	for ep := range byEpisode {
		sorted = append(sorted, ep)
	}
	sort.Ints(sorted)

	show := func(name string) string {
		switch name {
		case notSearched:
			return "(not searched)"
		case "":
			return "(nothing)"
		}
		return name
	}
	type moved struct {
		episode   int
		want, got string
	}
	var diffs []moved
	for _, ep := range sorted {
		if p := byEpisode[ep]; p[0] != p[1] {
			diffs = append(diffs, moved{ep, p[0], p[1]})
		}
	}
	eachRun(len(diffs),
		func(i int) int { return diffs[i].episode },
		func(first, j int) bool { return diffs[j].want == diffs[first].want && diffs[j].got == diffs[first].got },
		func(span string, first int) {
			report("episodes %s:\n  want %s\n  got  %s", span, show(diffs[first].want), show(diffs[first].got))
		})
}

// replayGolden serves the recording to RunAnimeDebug. A request that is not in it fails the case:
// the pipeline now asks something it did not ask when the case was recorded, and the answer is to
// re-record, not to guess.
func replayGolden(t *testing.T, animeID int, recorded []goldenExchange) *DebugSummary {
	t.Helper()
	byKey := make(map[string]goldenExchange, len(recorded))
	for _, x := range recorded {
		byKey[exchangeKey(x.Method, x.URL, x.Body)] = x
	}
	var mu sync.Mutex
	missing := map[string]bool{}
	serve := func(method, rawURL, body string) (*http.Response, error) {
		x, ok := byKey[exchangeKey(method, rawURL, body)]
		if !ok {
			mu.Lock()
			missing[method+" "+rawURL] = true
			mu.Unlock()
			x = goldenExchange{Status: http.StatusNotFound}
		}
		return &http.Response{StatusCode: x.Status, Body: io.NopCloser(strings.NewReader(x.Response)), Header: make(http.Header)}, nil
	}

	defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, err := readRequestBody(req)
		if err != nil {
			return nil, err
		}
		return serve(req.Method, req.URL.String(), body)
	})()
	defer nyaa.MockNyaaHttpGet(func(u string) (*http.Response, error) { return serve(http.MethodGet, u, "") })()

	summary := runGoldenPass(t, animeID)
	for req := range missing {
		t.Errorf("not in the recording: %s — the pipeline asks something new; re-record with make golden-record", req)
	}
	return summary
}

func readRequestBody(req *http.Request) (string, error) {
	if req.Body == nil {
		return "", nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	return string(data), nil
}

// TestGoldenRecord records a new case from the live AniList and Nyaa. Skipped unless GOLDEN_RECORD
// is set — run it through `make golden-record ID=<mediaId> NAME=<slug>`.
func TestGoldenRecord(t *testing.T) {
	idStr := os.Getenv("GOLDEN_RECORD")
	if idStr == "" {
		t.Skip("set GOLDEN_RECORD=<mediaId> GOLDEN_NAME=<slug> (make golden-record) to record a case")
	}
	animeID, err := strconv.Atoi(idStr)
	if err != nil {
		t.Fatalf("GOLDEN_RECORD=%q is not an AniList media id", idStr)
	}
	name := os.Getenv("GOLDEN_NAME")
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(name) {
		t.Fatalf("GOLDEN_NAME=%q: give the case a kebab-case slug (e.g. mushoku-part2-pack)", name)
	}
	dir := filepath.Join(goldenDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var recorded []goldenExchange
	seen := map[string]bool{}
	keep := func(x goldenExchange) {
		mu.Lock()
		defer mu.Unlock()
		x.URL = requestURI(x.URL)
		if k := exchangeKey(x.Method, x.URL, x.Body); !seen[k] {
			seen[k] = true
			recorded = append(recorded, x)
		}
	}
	client := &http.Client{Timeout: 30 * time.Second}

	defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, err := readRequestBody(req)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		keep(goldenExchange{Method: req.Method, URL: req.URL.String(), Body: body, Status: resp.StatusCode, Response: string(data)})
		resp.Body = io.NopCloser(bytes.NewReader(data))
		return resp, nil
	})()
	defer nyaa.MockNyaaHttpGet(func(u string) (*http.Response, error) {
		resp, err := client.Get(u)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		keep(goldenExchange{Method: http.MethodGet, URL: u, Status: resp.StatusCode, Response: trimNyaaPage(data)})
		resp.Body = io.NopCloser(bytes.NewReader(data))
		return resp, nil
	})()

	summary := runGoldenPass(t, animeID)

	writeJSON(t, filepath.Join(dir, "http.json"), recorded)
	fresh := goldenOutcome(summary)
	casePath := filepath.Join(dir, "case.json")
	if _, err := os.Stat(casePath); err == nil {
		// Keep the expected picks: one may be a hand-set right answer for a bug not fixed yet.
		var prev goldenCase
		readJSON(t, casePath, &prev)
		if want := fromRuns(t, prev.Searched); !slices.Equal(want, fresh) {
			t.Logf("case.json kept; the fresh outcome differs from it (GOLDEN_UPDATE=1 accepts it):")
			diffGoldenWith(t.Logf, want, fresh)
		}
		t.Logf("re-recorded %d requests into %s (%s)", len(recorded), dir, summary.AnimeName)
		return
	}
	writeJSON(t, casePath, goldenCase{AnimeID: animeID, Note: "TODO: what went wrong and what this case pins", Searched: toRuns(fresh)})
	t.Logf("recorded %d requests into %s (%s). case.json holds what the pipeline does NOW: write the note, "+
		"and if this case is a bug, set \"picked\" to the right torrent before fixing", len(recorded), dir, summary.AnimeName)
}

var (
	betweenTags  = regexp.MustCompile(`>\s+<`)
	magnetExtras = regexp.MustCompile(`&.*$`)
)

// trimNyaaPage keeps only what the parsers read — the rows of a search page's results table, the
// file list of a view page — and strips them down to text, href and class: icons, the header row,
// title attributes and the magnet's name and tracker list are most of Nyaa's bytes and none of the
// pipeline's input. The magnet keeps its info hash, which is what dedupe compares.
func trimNyaaPage(page []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return string(page)
	}
	for _, sel := range []string{".torrent-list", ".torrent-file-list"} {
		s := doc.Find(sel).First()
		if s.Length() == 0 {
			continue
		}
		s.Find("thead, i, img").Remove()
		s.Find("*").AddSelection(s).Each(func(_ int, e *goquery.Selection) {
			n := e.Nodes[0]
			n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool { return a.Key != "href" && a.Key != "class" })
			if href, ok := e.Attr("href"); ok && strings.HasPrefix(href, "magnet:") {
				e.SetAttr("href", magnetExtras.ReplaceAllString(href, ""))
			}
		})
		html, err := goquery.OuterHtml(s)
		if err != nil {
			break
		}
		return betweenTags.ReplaceAllString(html, "><")
	}
	return fmt.Sprintf("<html><body><!-- trimmed: no results table or file list in %d bytes --></body></html>", len(page))
}
