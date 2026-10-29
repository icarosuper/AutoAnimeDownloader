package nyaa

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Third live sample: extras vs next-cour FILES. Separate from TestLive_PackNumberingRestart
// / TestLive_MultiSeasonPacks (#84 caps). Gated on AAD_LIVE_NYAA=1.
//
// Policy under measurement (user addendum 2026-10-28): OVA/OAD/.5 bonus episodes are
// episode-like content; OP/ED/NCOP/NCED/PV/Menu/Creditless are not. The buckets still
// split those two so the spec can lock the marker list from what actually showed up.

type extrasQuery struct {
	query    string
	total    int
	altTotal int
	altLabel string
}

var extrasSampleQueries = []extrasQuery{
	{query: "K-ON! S1", total: 13},
	{query: "K-On! batch", total: 13},
	{query: "mushoku tensei batch", total: 11, altTotal: 23, altLabel: "S1 whole"},
	{query: "mushoku tensei s1", total: 11, altTotal: 23, altLabel: "S1 whole"},
	{query: "kaguya-sama batch", total: 12},
	{query: "steins gate batch", total: 24},
	{query: "haruhi batch", total: 14, altTotal: 28, altLabel: "2009"},
	{query: "fate ubw batch", total: 12, altTotal: 25, altLabel: "season"},
	{query: "hyouka batch", total: 22},
	{query: "shingeki no kyojin batch", total: 25},
	{query: "konosuba batch", total: 10},
	{query: "one punch man batch", total: 12},
}

// videoFile is one video in a pack's file list, numbered or not.
type videoFile struct {
	folder string
	name   string
	ep     *int
	frac   string
}

const (
	bucketOPED       = "op_ed"
	bucketPVMenu     = "pv_menu"
	bucketHalf       = "half_ep"
	bucketOVAonAxis  = "ova_on_axis"
	bucketOVAs00     = "ova_s00"
	bucketNextCour   = "next_cour"
	bucketIsolated   = "isolated_unmarked"
	bucketUnnumbered = "unnumbered_other"
	bucketMain       = "main_1_to_total"
)

var (
	// Decimal episode tokens that fansubs actually use. Restricted so 5.1 / 2.0 audio
	// and 10.0 do not count as .5 bonus episodes.
	reFracEpisode = regexp.MustCompile(`(?i)(?:S\d{1,2}E|\s-\s+|\bE|\bEpisode\s+|\s)(\d{1,3}\.(?:25|5|50|75))\b`)
	rePackPart    = regexp.MustCompile(`(?i)\bpart\s*\d`)
	rePackS1      = regexp.MustCompile(`(?i)(?:\bS0?1\b|season\s*1)`)
	reSxxExx      = regexp.MustCompile(`(?i)S\d{1,2}E\d`)
	reDashEp      = regexp.MustCompile(`(?i)\s-\s+\d`)
	reEprefix     = regexp.MustCompile(`(?i)\bE\d`)
	reBrackets    = regexp.MustCompile(`(?i)\[\d{2,3}\]`)
	reS00         = regexp.MustCompile(`(?i)S00E`)
	reOVAtoken    = regexp.MustCompile(`(?i)\b(?:ova|oad|ona)\d*\b`)
	reNCOPNCED    = regexp.MustCompile(`(?i)\bNC(?:OP|ED)\d*\b`)
	reOpening     = regexp.MustCompile(`(?i)\b(?:opening|ending)s?\b`)
	reOPtoken     = regexp.MustCompile(`(?i)(?:^|[\s\[\]_\-\.])(?:OP|ED)(?:\s|$|\d|[\s\]_])`)
	rePVtoken     = regexp.MustCompile(`(?i)\b(?:PV|preview|trailer|teaser)\d*\b`)
	reMenutoken   = regexp.MustCompile(`(?i)\bmenus?\b`)
	reSPtoken     = regexp.MustCompile(`(?i)(?:^|[\s\[\]_\-\.])SP\d*(?:$|[\s\]_\-\.])`)
)

func extractFractionalEpisode(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	if m := reFracEpisode.FindStringSubmatch(name); len(m) > 1 {
		return m[1]
	}
	return ""
}

func nameShape(name string) string {
	n := strings.ReplaceAll(name, "_", " ")
	switch {
	case reSxxExx.MatchString(n):
		return "SxxExx"
	case reDashEp.MatchString(n):
		return "dash"
	case reEprefix.MatchString(n):
		return "E-prefix"
	case reBrackets.MatchString(n):
		return "brackets"
	default:
		return "other"
	}
}

func packNameKind(name string) string {
	info := ExtractBatchInfo(name)
	switch {
	case info.EndEpisode > 0:
		return "range"
	case rePackPart.MatchString(name):
		return "part"
	case rePackS1.MatchString(name):
		return "s1-only"
	default:
		return "none"
	}
}

// fileMarkerFlags reads the file name and parent folder. Measurement-only.
type fileMarkerFlags struct {
	tokens []string
	opED   bool
	pvMenu bool
	ova    bool
	s00    bool
}

func (f fileMarkerFlags) exclude() bool { return f.opED || f.pvMenu }
func (f fileMarkerFlags) extra() bool   { return f.ova || f.s00 }

func detectMarkers(folder, name string) fileMarkerFlags {
	var f fileMarkerFlags
	add := func(tok string) {
		for _, t := range f.tokens {
			if t == tok {
				return
			}
		}
		f.tokens = append(f.tokens, tok)
	}
	lowFolder := strings.ToLower(folder)
	lowName := strings.ToLower(name)
	blob := lowFolder + "/" + lowName

	if reNCOPNCED.MatchString(name) || strings.Contains(blob, "ncop") || strings.Contains(blob, "nced") {
		f.opED = true
		add("NCOP/NCED")
	}
	if strings.Contains(blob, "creditless") {
		f.opED = true
		add("Creditless")
	}
	if reOpening.MatchString(name) {
		f.opED = true
		add("Opening/Ending")
	}
	if reOPtoken.MatchString(name) && !reNCOPNCED.MatchString(name) {
		f.opED = true
		add("OP/ED")
	}
	if strings.Contains(lowFolder, "/nc") || strings.HasPrefix(lowFolder, "nc") ||
		strings.Contains(lowFolder, "ncop") || strings.Contains(lowFolder, "nced") {
		f.opED = true
		add("folder-NC")
	}

	if rePVtoken.MatchString(name) || strings.Contains(lowFolder, "/pv") ||
		strings.Contains(lowFolder, "preview") {
		f.pvMenu = true
		add("PV")
	}
	if reMenutoken.MatchString(name) || strings.Contains(lowFolder, "menu") {
		f.pvMenu = true
		add("Menu")
	}

	if reOVAtoken.MatchString(name) {
		f.ova = true
		add("OVA/OAD")
	}
	if reS00.MatchString(name) {
		f.s00 = true
		add("S00")
	}
	if reSPtoken.MatchString(name) {
		f.ova = true
		add("SP")
	}
	// Only the immediate parent folder. The torrent root is often
	// "(S1 + OVA + Specials)", which must not mark every episode as an extra.
	parent := immediateFolder(folder)
	lowParent := strings.ToLower(parent)
	if isExtrasFolder(parent) {
		if strings.Contains(lowParent, "special") || strings.Contains(lowParent, "extra") ||
			strings.Contains(lowParent, "bonus") {
			f.ova = true
			add("folder-Specials")
		}
	}
	return f
}

func immediateFolder(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

type classifiedFile struct {
	videoFile
	bucket  string
	markers fileMarkerFlags
	shape   string
}

func classifyVideos(files []videoFile, total int) []classifiedFile {
	flags := make([]fileMarkerFlags, len(files))
	for i, f := range files {
		flags[i] = detectMarkers(f.folder, f.name)
	}

	// Unmarked numbers past total (the next-cour candidate run).
	unmarkedPast := map[int]bool{}
	for i, f := range files {
		if f.ep == nil || flags[i].exclude() || flags[i].extra() {
			continue
		}
		if *f.ep > total {
			unmarkedPast[*f.ep] = true
		}
	}
	unmarkedRun := 0
	for i := total + 1; unmarkedPast[i]; i++ {
		unmarkedRun++
	}

	// Same run, extra-marked files allowed on the axis (EMBER S01E24 [OVA]).
	anyPast := map[int]bool{}
	for i, f := range files {
		if f.ep == nil || flags[i].exclude() {
			continue
		}
		if *f.ep > total {
			anyPast[*f.ep] = true
		}
	}
	anyRun := 0
	for i := total + 1; anyPast[i]; i++ {
		anyRun++
	}

	out := make([]classifiedFile, 0, len(files))
	for i, f := range files {
		cf := classifiedFile{videoFile: f, markers: flags[i], shape: nameShape(f.name)}
		n, hasN := 0, f.ep != nil
		if hasN {
			n = *f.ep
		}
		switch {
		case flags[i].opED:
			cf.bucket = bucketOPED
		case flags[i].pvMenu:
			cf.bucket = bucketPVMenu
		case f.frac != "":
			cf.bucket = bucketHalf
		case flags[i].extra() && hasN && n > total && n <= total+anyRun:
			cf.bucket = bucketOVAonAxis
		case flags[i].extra():
			cf.bucket = bucketOVAs00
		case hasN && n > total && unmarkedRun >= 2 && n <= total+unmarkedRun:
			cf.bucket = bucketNextCour
		case hasN && n > total:
			cf.bucket = bucketIsolated
		case !hasN:
			cf.bucket = bucketUnnumbered
		default:
			cf.bucket = bucketMain
		}
		out = append(out, cf)
	}
	return out
}

func walkAllVideosSel(sel *goquery.Selection, path string, out *[]videoFile) {
	sel.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
		if link := li.Children().First(); link.Is("a.folder") {
			child := strings.TrimSpace(link.Text())
			sub := child
			if path != "" {
				sub = path + "/" + child
			}
			walkAllVideosSel(li.ChildrenFiltered("ul"), sub, out)
			return
		}
		name := fileListEntryName(li)
		if !IsVideoFile(name) {
			return
		}
		*out = append(*out, videoFile{
			folder: path,
			name:   name,
			ep:     ExtractEpisodeNumber(name),
			frac:   extractFractionalEpisode(name),
		})
	})
}

func minMaxNumbered(files []classifiedFile, keep func(classifiedFile) bool) (lo, hi, n int, ok bool) {
	for _, f := range files {
		if f.ep == nil || !keep(f) {
			continue
		}
		if !ok || *f.ep < lo {
			lo = *f.ep
		}
		if !ok || *f.ep > hi {
			hi = *f.ep
		}
		n++
		ok = true
	}
	return lo, hi, n, ok
}

type extrasTally struct {
	packs               int
	files               int
	bucket              map[string]int
	marker              map[string]int
	packName            map[string]int
	packsWithIsolated   int
	packsWithNextCour   int
	packsWithOVA        int
	packsWithHalf       int
	packsStretchNow     int
	packsStretchKeepOVA int // drop OP/ED only (user addendum)
	packsStretchDropOVA int // drop OP/ED and OVA/S00
}

func newExtrasTally() extrasTally {
	return extrasTally{
		bucket:   map[string]int{},
		marker:   map[string]int{},
		packName: map[string]int{},
	}
}

func (s *extrasTally) addPack(classified []classifiedFile, total int, nameKind string) {
	s.packs++
	s.packName[nameKind]++
	sawIsolated, sawNext, sawOVA, sawHalf := false, false, false, false
	for _, f := range classified {
		s.files++
		s.bucket[f.bucket]++
		for _, tok := range f.markers.tokens {
			s.marker[tok]++
		}
		switch f.bucket {
		case bucketIsolated:
			sawIsolated = true
		case bucketNextCour:
			sawNext = true
		case bucketOVAs00, bucketOVAonAxis:
			sawOVA = true
		case bucketHalf:
			sawHalf = true
		}
	}
	if sawIsolated {
		s.packsWithIsolated++
	}
	if sawNext {
		s.packsWithNextCour++
	}
	if sawOVA {
		s.packsWithOVA++
	}
	if sawHalf {
		s.packsWithHalf++
	}

	_, nowHi, _, nowOK := minMaxNumbered(classified, func(classifiedFile) bool { return true })
	_, keepOVAHi, _, keepOVAOK := minMaxNumbered(classified, func(f classifiedFile) bool {
		return f.bucket != bucketOPED && f.bucket != bucketPVMenu
	})
	_, dropOVAHi, _, dropOK := minMaxNumbered(classified, func(f classifiedFile) bool {
		switch f.bucket {
		case bucketOPED, bucketPVMenu, bucketOVAs00, bucketOVAonAxis, bucketHalf:
			return false
		default:
			return true
		}
	})
	stretches := func(hi int, ok bool) bool { return ok && hi > total }
	if stretches(nowHi, nowOK) {
		s.packsStretchNow++
	}
	if stretches(keepOVAHi, keepOVAOK) {
		s.packsStretchKeepOVA++
	}
	if stretches(dropOVAHi, dropOK) {
		s.packsStretchDropOVA++
	}
}

func (s extrasTally) report(t *testing.T) {
	t.Helper()
	t.Logf("SUMMARY packs=%d files=%d", s.packs, s.files)
	t.Logf("SUMMARY buckets: %s", formatCountMap(s.bucket))
	t.Logf("SUMMARY markers that showed up: %s", formatCountMap(s.marker))
	t.Logf("SUMMARY pack-name kind: %s", formatCountMap(s.packName))
	t.Logf("SUMMARY packs with isolated unmarked: %d | next-cour run: %d | OVA/S00: %d | .5: %d",
		s.packsWithIsolated, s.packsWithNextCour, s.packsWithOVA, s.packsWithHalf)
	t.Logf("SUMMARY packs whose PackFileRange max > AniList total: now=%d | drop OP/ED only (keep OVA)=%d | drop OP/ED+OVA=%d",
		s.packsStretchNow, s.packsStretchKeepOVA, s.packsStretchDropOVA)
}

func formatCountMap(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func logClassified(t *testing.T, files []classifiedFile, total int) {
	t.Helper()
	interesting := 0
	for _, f := range files {
		if f.bucket == bucketMain {
			continue
		}
		interesting++
		ep := "-"
		if f.ep != nil {
			ep = fmt.Sprintf("%d", *f.ep)
		}
		tok := strings.Join(f.markers.tokens, ",")
		path := f.name
		if f.folder != "" {
			path = f.folder + "/" + f.name
		}
		t.Logf("    [%s] ep=%s frac=%q shape=%s markers=%s  %s",
			f.bucket, ep, f.frac, f.shape, tok, path)
	}
	if interesting == 0 {
		t.Logf("    (no files outside 1..%d without markers)", total)
	}
}

func measureExtrasPack(t *testing.T, p packRow, q extrasQuery) []classifiedFile {
	t.Helper()
	defer time.Sleep(1500 * time.Millisecond)

	doc, err := fetchNyaaPage(p.detailURL)
	if err != nil {
		t.Logf("  ! detail unavailable: %v", err)
		return nil
	}
	var files []videoFile
	walkAllVideosSel(doc.Find(".torrent-file-list > ul"), "", &files)
	classified := classifyVideos(files, q.total)

	nameInfo := ExtractBatchInfo(p.name)
	kind := packNameKind(p.name)
	lo, hi, n, ok := minMaxNumbered(classified, func(classifiedFile) bool { return true })
	t.Logf("%s", p.name)
	t.Logf("  %s | S:%d | %s | pack-name %s (name range %d..%d) | AniList total %d",
		p.detailURL, p.seeders, formatSize(p.size), kind, nameInfo.StartEpisode, nameInfo.EndEpisode, q.total)
	if ok {
		t.Logf("  PackFileRange now: %d..%d (%d numbered files)%s", lo, hi, n,
			stretchNote(hi, q.total))
	} else {
		t.Logf("  PackFileRange now: UNRESOLVED")
	}
	if q.altTotal > 0 {
		alt := classifyVideos(files, q.altTotal)
		alo, ahi, an, aok := minMaxNumbered(alt, func(classifiedFile) bool { return true })
		if aok {
			t.Logf("  vs %s total=%d: range %d..%d (%d files)%s | buckets %s",
				q.altLabel, q.altTotal, alo, ahi, an, stretchNote(ahi, q.altTotal),
				bucketSummary(alt))
		}
		logClassified(t, alt, q.altTotal)
		t.Logf("  -- files vs primary total %d --", q.total)
	}
	t.Logf("  buckets: %s", bucketSummary(classified))
	logClassified(t, classified, q.total)
	return classified
}

func stretchNote(hi, total int) string {
	if hi > total {
		return fmt.Sprintf("  STRETCHES past %d by %d", total, hi-total)
	}
	return ""
}

func bucketSummary(files []classifiedFile) string {
	m := map[string]int{}
	for _, f := range files {
		m[f.bucket]++
	}
	return formatCountMap(m)
}

func TestLive_ExtrasVsNextCour(t *testing.T) {
	if os.Getenv("AAD_LIVE_NYAA") != "1" {
		t.Skip("set AAD_LIVE_NYAA=1 to run")
	}
	sum := newExtrasTally()
	seen := map[string]bool{}
	for _, q := range extrasSampleQueries {
		t.Logf("=== %s  (AniList total %d%s) ===", q.query, q.total, altNote(q))
		for _, p := range listPacks(t, q.query, 8) {
			if seen[p.detailURL] {
				t.Logf("  skip duplicate %s", p.detailURL)
				continue
			}
			seen[p.detailURL] = true
			classified := measureExtrasPack(t, p, q)
			if classified == nil {
				continue
			}
			sum.addPack(classified, q.total, packNameKind(p.name))
		}
		time.Sleep(1500 * time.Millisecond)
	}
	sum.report(t)
}

func altNote(q extrasQuery) string {
	if q.altTotal == 0 {
		return ""
	}
	return fmt.Sprintf(", also %s=%d", q.altLabel, q.altTotal)
}

func vf(folder, name string) videoFile {
	return videoFile{
		folder: folder,
		name:   name,
		ep:     ExtractEpisodeNumber(name),
		frac:   extractFractionalEpisode(name),
	}
}

func TestExtrasBuckets_KnownPackFiles(t *testing.T) {
	t.Parallel()

	// MTBB K-ON! S1: 13 TV + S00E19 (OVA). Episode 19 must not look like a next cour.
	mtbb := make([]videoFile, 0, 14)
	for i := 1; i <= 13; i++ {
		mtbb = append(mtbb, vf("", fmt.Sprintf("[MTBB] K-ON! - %02d.mkv", i)))
	}
	mtbb = append(mtbb, vf("", "[MTBB] K-ON! - S00E19 (OVA).mkv"))
	got := classifyVideos(mtbb, 13)
	assertBucket(t, got, "[MTBB] K-ON! - S00E19 (OVA).mkv", bucketOVAs00)
	assertNoBucket(t, got, bucketNextCour)
	assertNoBucket(t, got, bucketIsolated)

	mini := append([]videoFile{}, mtbb[:13]...)
	mini = append(mini, vf("", "[MiniMTBB] K-ON! - S00E19 (BD 1080p).mkv"))
	got = classifyVideos(mini, 13)
	assertBucket(t, got, "[MiniMTBB] K-ON! - S00E19 (BD 1080p).mkv", bucketOVAs00)

	root := "[Judas] One Punch Man (Seasons 1-2 + OVAs + Specials) [BD]/[Judas] One-Punch Man S1"
	got = classifyVideos([]videoFile{vf(root, "[Judas] One-Punch Man - S01E01.mkv")}, 12)
	assertBucket(t, got, "[Judas] One-Punch Man - S01E01.mkv", bucketMain)

	// koala: OVA under Specials/.
	koala := []videoFile{
		vf("", "K-ON.S01E01.mkv"),
		vf("", "K-ON.S01E13.mkv"),
		vf("Specials", "K-ON.S00E19.mkv"),
	}
	got = classifyVideos(koala, 13)
	assertBucket(t, got, "K-ON.S00E19.mkv", bucketOVAs00)

	// Some-Stuffs: unmarked 14. Residue the spec named.
	stuffs := []videoFile{
		vf("", "K-On! 01.mkv"),
		vf("", "K-On! 13.mkv"),
		vf("", "K-On! 14.mkv"),
	}
	got = classifyVideos(stuffs, 13)
	assertBucket(t, got, "K-On! 14.mkv", bucketIsolated)

	// Mushoku cour 1 total=11, files 01-23: 12-23 are the next cour.
	mushoku := make([]videoFile, 0, 24)
	for i := 1; i <= 23; i++ {
		mushoku = append(mushoku, vf("", fmt.Sprintf("[EMBER] Mushoku Tensei - %02d.mkv", i)))
	}
	got = classifyVideos(mushoku, 11)
	assertBucket(t, got, "[EMBER] Mushoku Tensei - 12.mkv", bucketNextCour)
	assertBucket(t, got, "[EMBER] Mushoku Tensei - 23.mkv", bucketNextCour)
	assertBucket(t, got, "[EMBER] Mushoku Tensei - 01.mkv", bucketMain)

	// EMBER S01E24 [OVA] sitting on the contiguous axis past a 23-ep season.
	ember := append([]videoFile{}, mushoku...)
	ember = append(ember, vf("", "[EMBER] Mushoku Tensei S01E24 [OVA]-Eris the Goblin Slayer.mkv"))
	got = classifyVideos(ember, 23)
	assertBucket(t, got, "[EMBER] Mushoku Tensei S01E24 [OVA]-Eris the Goblin Slayer.mkv", bucketOVAonAxis)

	// OP/ED must not stretch and must not land in next-cour.
	withOP := []videoFile{
		vf("", "[MTBB] K-ON! - 01.mkv"),
		vf("", "[MTBB] K-ON! - 13.mkv"),
		vf("", "[MTBB] K-ON! - NCOP.mkv"),
		vf("Creditless", "Opening 1.mkv"),
		vf("PV", "PV01.mkv"),
	}
	got = classifyVideos(withOP, 13)
	assertBucket(t, got, "[MTBB] K-ON! - NCOP.mkv", bucketOPED)
	assertBucket(t, got, "Opening 1.mkv", bucketOPED)
	assertBucket(t, got, "PV01.mkv", bucketPVMenu)

	// Hyouka 11.5 is a bonus episode, not episode 11 and not unnumbered.
	hyouka := []videoFile{
		vf("", "[Group] Hyouka - 11.mkv"),
		vf("", "[Group] Hyouka - 11.5.mkv"),
		vf("", "[Group] Hyouka - 22.mkv"),
	}
	got = classifyVideos(hyouka, 22)
	assertBucket(t, got, "[Group] Hyouka - 11.5.mkv", bucketHalf)
	if got[1].frac != "11.5" {
		t.Fatalf("11.5 frac = %q", got[1].frac)
	}

	// Konosuba OVA packed as 11, total 10: on-axis extra, not a 2+ next-cour run.
	konosuba := []videoFile{
		vf("", "[Group] Konosuba - 10.mkv"),
		vf("", "[Group] Konosuba - 11 (OVA).mkv"),
	}
	got = classifyVideos(konosuba, 10)
	assertBucket(t, got, "[Group] Konosuba - 11 (OVA).mkv", bucketOVAonAxis)
}

func assertBucket(t *testing.T, files []classifiedFile, name, want string) {
	t.Helper()
	for _, f := range files {
		if f.name == name {
			if f.bucket != want {
				t.Fatalf("%s: bucket %s, want %s (ep=%v frac=%q markers=%v)",
					name, f.bucket, want, f.ep, f.frac, f.markers.tokens)
			}
			return
		}
	}
	t.Fatalf("%s: not in classified list", name)
}

func assertNoBucket(t *testing.T, files []classifiedFile, bucket string) {
	t.Helper()
	for _, f := range files {
		if f.bucket == bucket {
			t.Fatalf("did not want any file in %s, got %s", bucket, f.name)
		}
	}
}
