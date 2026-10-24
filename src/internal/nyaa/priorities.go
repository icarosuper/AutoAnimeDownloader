package nyaa

import (
	"sort"
	"strings"
	"sync/atomic"
)

// Priorities defines the ordered lists that govern torrent ranking and filtering.
// Index in the list = priority (lower = better). An item missing from the list
// is treated as worst (goes to the end).
type Priorities struct {
	// CriteriaOrder is the order in which sort criteria are applied.
	// Valid values: uncensored, source, resolution, codec, fansub, audio, health, size.
	CriteriaOrder []string `json:"criteria_order"`
	Fansubs       []string `json:"fansubs"`
	Resolutions   []string `json:"resolutions"`
	Sources       []string `json:"sources"`
	Codecs        []string `json:"codecs"`
	Audio         []string `json:"audio"`
	// IgnoreList: substrings (case-insensitive) that discard the release outright.
	IgnoreList []string `json:"ignore_list"`
}

// DefaultPriorities faithfully reproduces the original hardcoded maps/regexes.
// All lowercase (the lookup compares against lowercase tokens).
func DefaultPriorities() Priorities {
	return Priorities{
		// health comes before fansub: it was 7th of 8 in a lexicographic sort and was never
		// reached (resolution/fansub decided first), so a torrent with 12 seeders beat one
		// with 400 for being from a better-placed fansub.
		CriteriaOrder: []string{"uncensored", "source", "resolution", "health", "codec", "fansub", "audio", "size"},
		Fansubs: []string{
			"subsplease", "erai-raws", "judas", "toonshub", "asw",
			"ember", "hd-zone", "kamig", "remix", "aniverse", "dub", "raw",
		},
		// Only the canonical tokens ExtractResolution can return: "4k"/"uhd" become
		// "2160p", "fhd" becomes "1080p", "hd" becomes "720p". 4K below 720p is deliberate —
		// 4K anime is almost always an upscale of a 1080p master.
		Resolutions: []string{"1080p", "720p", "1440p", "2160p", "480p"},
		// BDRemux is the untouched bitstream from the disc; BDRip is a reencode.
		Sources: []string{"bdremux", "bd", "bdrip", "web-dl", "webrip", "tv", "dvd", "hdtv"},
		// H.264 first: plays directly on any player, no server-side transcode (which burns
		// the subtitle into the video and desyncs it). Whoever prefers a smaller file
		// switches via the preset on the priorities screen. "x265"/"x264" don't go in
		// because ExtractCodec already canonicalizes them to "hevc"/"h.264" — they would
		// be inert tokens.
		Codecs:     []string{"h.264", "hevc", "av1", "xvid"},
		Audio:      []string{"flac", "dts-hd", "truehd", "ddp", "aac", "ac3", "mp3"},
		IgnoreList: []string{"[dub]", "[raw]", "[hardcoded]", "[hc]", "re-encode"},
	}
}

// active holds the priorities in use. Initialized with the defaults in package
// init so the sort functions work even without SetPriorities (e.g. tests that
// call SortTorrentResults directly).
var active atomic.Pointer[Priorities]

func init() {
	d := DefaultPriorities()
	active.Store(&d)
}

// ActivePriorities returns the priorities in use (never nil).
func ActivePriorities() Priorities { return *active.Load() }

// SetPriorities applies new priorities and returns a function that restores the
// previous value (same pattern as MockNyaaHttpGet — avoids leaking global state in tests).
func SetPriorities(p Priorities) (restore func()) {
	prev := active.Load()
	active.Store(&p)
	return func() { active.Store(prev) }
}

// priorityIndex returns the index of token (lowercase) in list, or len(list)
// (worst) if missing. token is compared in lowercase.
func priorityIndex(list []string, token string) int {
	t := strings.ToLower(token)
	for i, v := range list {
		if v == t {
			return i
		}
	}
	return len(list)
}

// ShouldIgnore reports whether the release should be discarded for matching some
// item (substring, case-insensitive) of the active IgnoreList.
func ShouldIgnore(torrentName string) bool {
	nameLower := strings.ToLower(torrentName)
	for _, pat := range ActivePriorities().IgnoreList {
		if pat != "" && strings.Contains(nameLower, strings.ToLower(pat)) {
			return true
		}
	}
	return false
}

// criterionCompare maps criterion name → comparator (a better than b ⇒ <0).
var criterionCompare = map[string]func(a, b TorrentResult) int{
	"uncensored": func(a, b TorrentResult) int { return boolBetter(isUncensored(a.Name), isUncensored(b.Name)) },
	"resolution": func(a, b TorrentResult) int { return resCompare(a, b) },
	"fansub":     func(a, b TorrentResult) int { return fansubPriority(a.Name) - fansubPriority(b.Name) },
	"source": func(a, b TorrentResult) int {
		return sourcePriority(extractSource(a.Name)) - sourcePriority(extractSource(b.Name))
	},
	"codec": codecCompare,
	"audio": func(a, b TorrentResult) int {
		return audioPriority(extractAudio(a.Name)) - audioPriority(extractAudio(b.Name))
	},
	"health": func(a, b TorrentResult) int { return healthTier(b) - healthTier(a) }, // higher tier is better
	"size":   func(a, b TorrentResult) int { return sizeCompare(a, b) },
}

// healthTierFloors are the floors of the health tiers: seeders 0–9 / 10–49 / 50+ ⇒ tier
// 0..2. min_seeders (default 3) already dropped 0–2, so 10 is the first split among
// survivors. 50 vs 5000 is the same download on a home connection (decisions.md #106).
var healthTierFloors = []int{10, 50}

// healthTier returns the torrent's health tier (higher = healthier).
//
// The criterion compares TIER, not the raw score, because the sort is lexicographic: with
// the raw score any 1-seeder difference already decided and no criterion after health was
// consulted. Comparing tier makes 400 seeders beat 12 and makes 150 vs 300 tie, leaving the
// next criterion (fansub) to decide — which is the intended behavior.
//
// A fixed tier instead of a ratio (e.g. "tie unless one has 2x the other") on purpose:
// ratio is not transitive (100 ~ 150 ~ 220, but 100 < 220) and sort.SliceStable with an
// intransitive comparator returns an arbitrary order.
func healthTier(r TorrentResult) int {
	seeders := ParseSeeders(r.Seeders)
	tier := 0
	for _, floor := range healthTierFloors {
		if seeders >= floor {
			tier++
		}
	}
	return tier
}

func boolBetter(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return -1
	}
	return 1
}

// codecCompare breaks ties by codec, but only when BOTH names carry a codec tag.
// A name without a tag ("[SubsPlease] Anime - 07 (1080p) [HASH].mkv" doesn't say H.264
// anywhere) returns a tie instead of going to the end: priorityIndex treats an unknown
// token as worst, and without this guard every untagged release would lose to any tagged
// x265 — exactly the opposite of what the codec list configures. Same rule as an
// unreadable size in filterBySize: data that couldn't be read is no reason to punish
// the release.
func codecCompare(a, b TorrentResult) int {
	ca, cb := ExtractCodec(a.Name), ExtractCodec(b.Name)
	if ca == "" || cb == "" {
		return 0
	}
	return codecPriority(ca) - codecPriority(cb)
}

// resCompare reproduces the current logic: both with a resolution → by index;
// whoever has a resolution beats whoever is nil; both nil → tie.
func resCompare(a, b TorrentResult) int {
	switch {
	case a.Resolution != nil && b.Resolution != nil:
		return resolutionPriority(*a.Resolution) - resolutionPriority(*b.Resolution)
	case a.Resolution != nil:
		return -1
	case b.Resolution != nil:
		return 1
	default:
		return 0
	}
}

// sizeCompare reproduces the current rule: smaller is better, but only breaks ties when
// the resolutions are equal (both non-nil and equal); otherwise a tie.
func sizeCompare(a, b TorrentResult) int {
	if a.Resolution != nil && b.Resolution != nil && *a.Resolution == *b.Resolution {
		switch {
		case a.Size < b.Size:
			return -1
		case a.Size > b.Size:
			return 1
		}
	}
	return 0
}

// sortByCriteria sorts results applying the criteria in criteria in the given order,
// skipping those not in the registry.
func sortByCriteria(results []TorrentResult, criteria []string) []TorrentResult {
	sorted := make([]TorrentResult, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		for _, c := range criteria {
			cmp, ok := criterionCompare[c]
			if !ok {
				continue
			}
			if d := cmp(sorted[i], sorted[j]); d != 0 {
				return d < 0
			}
		}
		return false
	})
	return sorted
}

var episodeCriteria = map[string]bool{"uncensored": true, "resolution": true, "codec": true, "fansub": true, "health": true, "size": true}

func filterCriteria(order []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(order))
	for _, c := range order {
		if allowed[c] {
			out = append(out, c)
		}
	}
	return out
}
