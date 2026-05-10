package nyaa

import (
	"regexp"
	"strings"
)

// titleTechnicalTokens are tokens found in torrent names that are NOT part of the anime title.
// Only unambiguous technical terms are included to avoid accidentally filtering title words.
var titleTechnicalTokens = map[string]bool{
	// Video codecs
	"hevc": true, "x265": true, "x264": true, "h264": true, "h265": true,
	"av1": true, "xvid": true, "avc": true,
	// Rip sources (multi-char only to avoid ambiguity)
	"bdrip": true, "bdremux": true, "webdl": true, "webrip": true, "hdtv": true, "bluray": true,
	// Audio codecs
	"flac": true, "aac": true, "dts": true, "ddp": true, "ac3": true,
	"opus": true, "truehd": true, "mp3": true,
	// Encoding details
	"remux": true, "hi10": true, "hi10p": true, "10bit": true, "8bit": true,
	// Language tags (unambiguous 3-char codes)
	"eng": true, "jpn": true,
	// Known fansubs
	"subsplease": true, "erai": true, "raws": true, "judas": true,
	"toonshub": true, "asw": true, "ember": true,
	// Season/episode/structure keywords
	"season": true, "episode": true, "cour": true, "part": true,
	// Batch/collection markers (type already checked by isBatch/isMovie)
	"batch": true, "complete": true, "movie": true, "ova": true, "ona": true, "special": true, "film": true,
	// Misc unambiguous tags
	"uncensored": true,
}

var (
	reMatchNonAlpha   = regexp.MustCompile(`[^a-z0-9]+`)
	reMatchAllDigits  = regexp.MustCompile(`^\d+$`)
	reMatchVersionSfx = regexp.MustCompile(`^\d+v\d+$`)      // 03v2
	reMatchSxEx       = regexp.MustCompile(`^s\d+e\d+$`)     // s01e05
	reMatchSeasonOnly = regexp.MustCompile(`^s\d+$`)          // s1, s2, s01
	reMatchEpPrefix   = regexp.MustCompile(`^ep\d+$`)         // ep05, ep015
	reMatchEpShort    = regexp.MustCompile(`^e\d+$`)          // e05, e015
	reMatchResToken   = regexp.MustCompile(`^\d{3,4}p$`)      // 1080p, 720p
	reMatchOrdinal    = regexp.MustCompile(`^\d+(?:st|nd|rd|th)$`) // 1st, 2nd
	reMatchHexHash    = regexp.MustCompile(`^[0-9a-f]{6,10}$`)
)

// extractTitleTokens returns meaningful title tokens from a torrent name or anime title,
// stripping technical metadata (resolution, codec, fansub, episode/season numbers, etc.)
func extractTitleTokens(name string) []string {
	lower := strings.ToLower(name)
	cleaned := reMatchNonAlpha.ReplaceAllString(lower, " ")
	parts := strings.Fields(cleaned)

	seen := make(map[string]bool, len(parts))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || seen[p] {
			continue
		}
		if isTitleTechnicalToken(p) {
			continue
		}
		seen[p] = true
		result = append(result, p)
	}
	return result
}

func isTitleTechnicalToken(s string) bool {
	if titleTechnicalTokens[s] {
		return true
	}
	if reMatchAllDigits.MatchString(s) {
		return true
	}
	if reMatchVersionSfx.MatchString(s) {
		return true
	}
	if reMatchSxEx.MatchString(s) {
		return true
	}
	if reMatchSeasonOnly.MatchString(s) {
		return true
	}
	if reMatchEpPrefix.MatchString(s) {
		return true
	}
	if reMatchEpShort.MatchString(s) {
		return true
	}
	if reMatchResToken.MatchString(s) {
		return true
	}
	if reMatchOrdinal.MatchString(s) {
		return true
	}
	if reMatchHexHash.MatchString(s) {
		return true
	}
	return false
}

func jaccardSimilarity(aTokens, bTokens []string) float64 {
	if len(aTokens) == 0 && len(bTokens) == 0 {
		return 1.0
	}
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0.0
	}

	aSet := make(map[string]bool, len(aTokens))
	for _, t := range aTokens {
		aSet[t] = true
	}

	bSet := make(map[string]bool, len(bTokens))
	for _, t := range bTokens {
		bSet[t] = true
	}

	intersection := 0
	for t := range aSet {
		if bSet[t] {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

const titleJaccardThreshold = 0.8

// titleMatchesQuery checks if a torrent name matches a search query using two complementary checks:
//  1. All query title tokens must be present in the torrent title (prevents partial-title matches
//     like "Chinmoku no Majo" matching a query that also requires "Silent Witch")
//  2. Jaccard similarity >= threshold (prevents spinoffs/sequels with extra title words,
//     e.g. "SAO Alternative Gun Gale Online" for a "SAO" query)
func titleMatchesQuery(torrentName, query string) bool {
	torrentTokens := extractTitleTokens(torrentName)
	queryTokens := extractTitleTokens(query)

	// If query has no meaningful tokens, fall back to permissive match
	if len(queryTokens) == 0 {
		return true
	}
	if len(torrentTokens) == 0 {
		return false
	}

	torrentSet := make(map[string]bool, len(torrentTokens))
	for _, t := range torrentTokens {
		torrentSet[t] = true
	}

	// All query tokens must be present in the torrent title
	for _, qt := range queryTokens {
		if !torrentSet[qt] {
			return false
		}
	}

	// Jaccard similarity prevents accepting spinoffs/sequels with many extra title words
	return jaccardSimilarity(torrentTokens, queryTokens) >= titleJaccardThreshold
}

// TitleMatchesQuery is the exported version for testing.
func TitleMatchesQuery(torrentName, query string) bool {
	return titleMatchesQuery(torrentName, query)
}
