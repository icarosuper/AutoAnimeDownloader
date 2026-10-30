package nyaa

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Live measurement of the two pack caps from Stage 1 of the TODO:
//
//  1. numbering that RESTARTS per season: PackFileRange now concatenates overlapping
//     season groups (decisions.md #116). This instrument still prints per-folder spans so
//     a future measurement can catch a flatten regression.
//  2. pack cap by episode instead of by total size (sources.md, "Detail page",
//     item 2): only holds if the current cap is rejecting a pack the user wants.
//
// Only runs with AAD_LIVE_NYAA=1, so `go test ./...` doesn't depend on the network.

// folderStats are the video files of one folder of the file list.
type folderStats struct {
	path     string
	episodes []int
	seasons  map[int]int // season read from the FILE NAME -> how many files
	bytes    int64
}

func (f folderStats) span() (int, int) {
	lo, hi := f.episodes[0], f.episodes[0]
	for _, e := range f.episodes {
		if e < lo {
			lo = e
		}
		if e > hi {
			hi = e
		}
	}
	return lo, hi
}

// walkFileList walks the file-list tree assigning each video file to the folder that
// contains it. Same reading as PackFileRange (IsVideoFile + ExtractEpisodeNumber), only
// grouped by folder instead of flattened into a single min..max.
func walkFileList(sel *goquery.Selection, path string, out *[]folderStats) {
	cur := folderStats{path: path, seasons: map[int]int{}}
	sel.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
		if link := li.Children().First(); link.Is("a.folder") {
			child := strings.TrimSpace(link.Text())
			sub := child
			if path != "" {
				sub = path + "/" + child
			}
			walkFileList(li.ChildrenFiltered("ul"), sub, out)
			return
		}
		name := fileListEntryName(li)
		if !IsVideoFile(name) {
			return
		}
		cur.bytes += parseSize(strings.Trim(strings.TrimSpace(li.Find("span.file-size").Text()), "()"))
		if s := ExtractSeason(name); s != nil {
			cur.seasons[*s]++
		}
		if ep := ExtractEpisodeNumber(name); ep != nil {
			cur.episodes = append(cur.episodes, *ep)
		}
	})
	if len(cur.episodes) > 0 {
		*out = append(*out, cur)
	}
}

// isExtrasFolder recognizes a folder whose files are NOT episodes but whose name still
// yields a number to ExtractEpisodeNumber (NCOP 01, PV 02...). It doesn't exist in
// production code — that's exactly the measurement of how much they dirty the range.
func isExtrasFolder(path string) bool {
	low := strings.ToLower(path)
	for _, marker := range []string{"extra", "creditless", "/nc", "special", "/pv", "bonus", "menu"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

type packRow struct {
	name      string
	detailURL string
	size      int64
	seeders   int
}

// listPacks returns the first PACK rows of a search, in seeder order — same protocol as
// the #84 measurement.
func listPacks(t *testing.T, query string, limit int) []packRow {
	t.Helper()
	url := fmt.Sprintf("%s/?f=0&c=1_2&q=%s&s=seeders&o=desc", getNyaaBaseURL(), strings.ReplaceAll(query, " ", "+"))
	doc, err := fetchNyaaPage(url)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	var rows []packRow
	doc.Find(".torrent-list tbody tr").EachWithBreak(func(_ int, tr *goquery.Selection) bool {
		cols := tr.Find("td")
		link := cols.Eq(1).Find("a").Last()
		name := strings.TrimSpace(link.AttrOr("title", link.Text()))
		if name == "" || !IsBatch(name) || ShouldIgnore(name) {
			return true
		}
		rows = append(rows, packRow{
			name:      name,
			detailURL: detailURL(link.AttrOr("href", "")),
			size:      parseSize(strings.TrimSpace(cols.Eq(3).Text())),
			seeders:   ParseSeeders(strings.TrimSpace(cols.Eq(5).Text())),
		})
		return len(rows) < limit
	})
	return rows
}

type verdict struct {
	restarts   bool
	unresolved bool
	extras     bool
	bytesPerEp int64
}

func measurePack(t *testing.T, p packRow) (v verdict) {
	t.Helper()
	defer time.Sleep(1500 * time.Millisecond) // be polite to nyaa.si

	doc, err := fetchNyaaPage(p.detailURL)
	if err != nil {
		t.Logf("  ! detail unavailable: %v", err)
		return verdict{unresolved: true}
	}

	var folders []folderStats
	walkFileList(doc.Find(".torrent-file-list > ul"), "", &folders)

	// Equivalent to PackFileRange over the same document (same filters), without spending a
	// second request on the same page.
	fileInfo, ok := BatchInfo{}, false
	total := 0
	seen := map[int]int{}
	for _, f := range folders {
		lo, hi := f.span()
		if !ok || lo < fileInfo.StartEpisode {
			fileInfo.StartEpisode = lo
		}
		if !ok || hi > fileInfo.EndEpisode {
			fileInfo.EndEpisode = hi
		}
		ok = true
		total += len(f.episodes)
		for _, e := range f.episodes {
			seen[e]++
		}
	}
	dupes := 0
	for _, n := range seen {
		if n > 1 {
			dupes++
		}
	}

	nameInfo := ExtractBatchInfo(p.name)
	t.Logf("%s", p.name)
	t.Logf("  %s | S:%d | %s", p.detailURL, p.seeders, formatSize(p.size))
	t.Logf("  range from NAME:   %d..%d", nameInfo.StartEpisode, nameInfo.EndEpisode)
	if ok {
		t.Logf("  PackFileRange:     %d..%d  (span %d)", fileInfo.StartEpisode, fileInfo.EndEpisode,
			fileInfo.EndEpisode-fileInfo.StartEpisode+1)
	} else {
		t.Logf("  PackFileRange:     UNRESOLVED")
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].path < folders[j].path })
	for _, f := range folders {
		lo, hi := f.span()
		t.Logf("    folder %-55q %3d files  eps %d..%d  seasons-in-name %v  %s",
			f.path, len(f.episodes), lo, hi, f.seasons, formatSize(f.bytes))
	}
	t.Logf("  video files with a number: %d | numbers repeated across folders: %d", total, dupes)

	v.restarts, v.unresolved = dupes > 0, !ok
	for _, f := range folders {
		if isExtrasFolder(f.path) {
			v.extras = true
			t.Logf("  >>> EXTRAS FOLDER COUNTED AS EPISODE: %q (%d files)", f.path, len(f.episodes))
		}
	}
	if total > 0 && p.size > 0 {
		v.bytesPerEp = p.size / int64(total)
		t.Logf("  SIZE PER EPISODE: %s  (whole pack %s)", formatSize(v.bytesPerEp), formatSize(p.size))
	}
	if dupes > 0 {
		t.Logf("  >>> NUMBERING RESTARTS: PackFileRange covers %d numbers, the pack has %d files",
			fileInfo.EndEpisode-fileInfo.StartEpisode+1, total)
	}
	return v
}

// tally accumulates the summary of a sample.
type tally struct {
	packs, restarts, unresolved, extras int
	perEp                               []int64
}

func (s *tally) add(v verdict) {
	s.packs++
	if v.restarts {
		s.restarts++
	}
	if v.unresolved {
		s.unresolved++
	}
	if v.extras {
		s.extras++
	}
	if v.bytesPerEp > 0 {
		s.perEp = append(s.perEp, v.bytesPerEp)
	}
}

func (s *tally) report(t *testing.T) {
	t.Helper()
	sort.Slice(s.perEp, func(i, j int) bool { return s.perEp[i] < s.perEp[j] })
	t.Logf("SUMMARY: %d packs | %d with numbering that restarts | %d with no readable range | %d with extras folder counted",
		s.packs, s.restarts, s.unresolved, s.extras)
	if n := len(s.perEp); n > 0 {
		t.Logf("SUMMARY size/episode: min %s | median %s | max %s",
			formatSize(s.perEp[0]), formatSize(s.perEp[n/2]), formatSize(s.perEp[n-1]))
	}
}

func runSample(t *testing.T, queries []string, perQuery int) {
	t.Helper()
	var sum tally
	for _, q := range queries {
		t.Logf("=== %s ===", q)
		for _, p := range listPacks(t, q, perQuery) {
			sum.add(measurePack(t, p))
		}
		time.Sleep(1500 * time.Millisecond)
	}
	sum.report(t)
}

// TestLive_PackNumberingRestart measures how common a pack whose numbering restarts per
// season is, in the NEUTRAL sample: `?q=<anime>+batch&s=seeders`, same first 8 pack rows as #84.
func TestLive_PackNumberingRestart(t *testing.T) {
	if os.Getenv("AAD_LIVE_NYAA") != "1" {
		t.Skip("set AAD_LIVE_NYAA=1 to run")
	}
	queries := strings.Split(os.Getenv("AAD_LIVE_QUERIES"), "|")
	if len(queries) == 1 && queries[0] == "" {
		queries = []string{
			"shingeki no kyojin batch",
			"mushoku tensei batch",
			"vinland saga batch",
			"jujutsu kaisen batch",
			"one piece batch",
			"steins gate batch",
		}
	}
	runSample(t, queries, 8)
}

// TestLive_MultiSeasonPacks measures the TARGET case: searches that aim at multi-season
// packs, where the #84 cap should hurt.
func TestLive_MultiSeasonPacks(t *testing.T) {
	if os.Getenv("AAD_LIVE_NYAA") != "1" {
		t.Skip("set AAD_LIVE_NYAA=1 to run")
	}
	runSample(t, []string{
		"shingeki no kyojin season 1 - 3",
		"mushoku tensei complete",
		"konosuba complete",
		"overlord season 1-4",
		"my hero academia complete",
		"haikyuu complete",
	}, 4)
}
