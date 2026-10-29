package nyaa

import (
	"regexp"
	"sort"
	"strings"
)

// Pre-compiled regexes used across the nyaa package.
// Compiling on first call vs at package init is a significant speedup since
// these are called inside per-torrent loops.

var (
	// Season stripping from query names
	reSeasonStrip = regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)

	// Batch detection patterns
	// A roman numeral closing a title, with an optional ": subtitle" (romanSequelBase).
	reRomanSeasonTitle = regexp.MustCompile(`^(.+?)\s+(II|III|IV|V|VI|VII|VIII|IX)(?:\s*:.*)?$`)

	reBatchPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\(batch\)|\[batch\]|\s+batch\s+`),
		regexp.MustCompile(`(?i)\(unofficial\s+batch\)`),
		regexp.MustCompile(`(?i)\(\d{1,4}\s*[-~]\s*\d{1,4}\)`),
		// The end of the range can be a space, the end of the name, an extension or a bracket: requiring
		// a space let "One Piece EP 001-501" and "One Piece 001-501.mkv" through — 500-episode packs that
		// the episode search read as "episode 1" (see decisions.md #53).
		regexp.MustCompile(`(?i)\s\d{2,4}\s*[-~]\s*\d{2,4}(?:\s|$|\.|\[)`),
		regexp.MustCompile(`(?i)\s\d{2,4}~\d{2,4}\s`),
		regexp.MustCompile(`(?i)\(complete\)|\[complete\]|\s+complete\s+`),
		regexp.MustCompile(`(?i)complete\s+series|complete\s+season`),
		regexp.MustCompile(`(?i)\(season\s+\d+.*complete\)`),
		regexp.MustCompile(`(?i)\(s\d{1,2}\)|\[s\d{1,2}\]`),
		regexp.MustCompile(`(?i)\(season\s+\d+\)|\[season\s+\d+\]`),
		regexp.MustCompile(`(?i)\(\d{1,2}st\s+season\)|\(\d{1,2}nd\s+season\)|\(\d{1,2}rd\s+season\)|\(\d{1,2}th\s+season\)`),
		regexp.MustCompile(`(?i)\bseason\s+\d+\s*\([^)]*\)`),
		regexp.MustCompile(`(?i)\.s\d{1,2}\.`),
		regexp.MustCompile(`(?i)\bs\d{1,2}\s+\(.*bd`),
		regexp.MustCompile(`(?i)s\d{1,2}\s+\(1080p\)|s\d{1,2}\s+\(720p\)|s\d{1,2}\s+\(480p\)`),
		regexp.MustCompile(`(?i)s\d{1,2}\s+1080p|s\d{1,2}\s+720p`),
		regexp.MustCompile(`(?i)s\d{1,2}\.+1080p|s\d{1,2}\.+720p`),
	}

	// Batch info extraction
	reBatchRange = regexp.MustCompile(`(?i)(\d{1,4})\s*[-~]\s*(\d{1,4})`)

	// Movie keyword patterns
	reMovieKeywords = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\(movie\)|\[movie\]|\smovie\s`),
		regexp.MustCompile(`(?i)\(film\)|\[film\]`),
		regexp.MustCompile(`(?i)gekijouban`),
		regexp.MustCompile(`(?i)theater\s+version`),
		regexp.MustCompile(`(?i)theatrical`),
	}
	// \b is required: without it "ona" matches inside "Persona", "ova" inside "Nova", and the anime
	// title is read as an OVA marker.
	reOvaPattern     = regexp.MustCompile(`(?i)\b(ova|ona)\b|original\s+(video|net)\s+animation`)
	reSpecialPattern = regexp.MustCompile(`(?i)\(special\)|\[special\]|\sspecial\s|(tv\s+special|episode\s+sp)`)
	reHasEpisode     = regexp.MustCompile(`(?i)(-\s?\d{1,4}|episode\s*\d{1,4}|s\d{1,2}e\d{1,4})`)

	// Fansub extraction
	reFansub = regexp.MustCompile(`(?i)^\[([^\]]+)\]|^\(([^\)]+)\)`)

	// Episode number patterns (ordered by specificity)
	reEpisodePatterns = []struct {
		re   *regexp.Regexp
		desc string
	}{
		// 4 digits: long anime (One Piece 1123+). Exception: `[05]` stays at 3 digits because `[2025]` (a
		// year) is far more common than a 4-digit episode in brackets.
		{regexp.MustCompile(`(?i)S\d{1,2}E(\d{1,4})`), "S01E05"},
		{regexp.MustCompile(`(?i)\s+-\s+(\d{1,4})(?:\s|v\d+|$|\[)`), "- 05, - 05v2"},
		{regexp.MustCompile(`(?i)\s+-\s+(\d{1,4})\s*\(`), "- 05 (1080p)"},
		{regexp.MustCompile(`(?i)EP\s*(\d{1,4})\b`), "EP05"},
		{regexp.MustCompile(`(?i)Episode\s*(\d{1,4})\b`), "Episode 05"},
		{regexp.MustCompile(`(?i)\bE(\d{1,4})\b`), "E05"},
		{regexp.MustCompile(`(?i)\[(\d{1,3})\]`), "[05]"},
		{regexp.MustCompile(`(?i)\s(\d{1,4})\s*\(`), " 05 ("},
		{videoExtEpisodeRegexp(), " 05.<video ext>"},
		{regexp.MustCompile(`(?i)\s(\d{1,4})$`), " 5"},
	}

	// Season number patterns (ordered by specificity)
	reSeasonPatterns = []struct {
		re   *regexp.Regexp
		desc string
	}{
		{regexp.MustCompile(`(?i)\bS(\d{1,2})E`), "S01E05"},
		{regexp.MustCompile(`(?i)\bS(\d{1,2})\b`), "S01"},
		{regexp.MustCompile(`(?i)Season\s*(\d{1,2})`), "Season 1"},
		{regexp.MustCompile(`(?i)(\d{1,2})(?:st|nd|rd|th)\s+Season`), "1st Season"},
		{regexp.MustCompile(`(?i)\b(\d{1,2})\s+-\s+\d{1,2}`), "2 - 07"},
		{regexp.MustCompile(`(?i)\b(\d{1,2})\s+EP\d{1,3}`), "2 EP07"},
		{regexp.MustCompile(`(?i)Cour\s*(\d{1,2})`), "Cour 1"},
	}

	// Roman numeral season marker, e.g. "Katainaka no Ossan, Kensei ni Naru II".
	// Tried as a fallback after reSeasonPatterns (lower confidence than an explicit
	// "Season 2"/"S2" marker) since fansub groups often release sequels using the
	// anime's own title verbatim, with no separate season marker beyond the numeral.
	// Kept out of reSeasonPatterns because that slice also drives truncateAtFirstMarker
	// (nyaa_match.go), which must not cut the title before a numeral that's part of it.
	reRomanSeason = regexp.MustCompile(`\b(II|III|IV|V|VI|VII|VIII|IX|X)\b`)

	// Resolution patterns
	reResolutionPatterns = []struct {
		re   *regexp.Regexp
		desc string
	}{
		{regexp.MustCompile(`(?i)\b(\d{3,4}p)\b`), "1080p"},
		{regexp.MustCompile(`(?i)\b(\d{3,4}x\d{3,4})\b`), "1920x1080"},
		{regexp.MustCompile(`(?i)\b(4K)\b`), "4K"},
		{regexp.MustCompile(`(?i)\b(8K)\b`), "8K"},
		{regexp.MustCompile(`(?i)\b(F?HD)\b`), "HD/FHD"},
		{regexp.MustCompile(`(?i)\b(UHD)\b`), "UHD"},
	}

	// Source patterns
	reSourcePatterns = []struct {
		re     *regexp.Regexp
		source string
	}{
		{regexp.MustCompile(`(?i)BDRemux`), "BDRemux"},
		{regexp.MustCompile(`(?i)BDRip|BD\s*Ray`), "BDRip"},
		{regexp.MustCompile(`(?i)Blu-ray|Blu\s*ray`), "BD"},
		{regexp.MustCompile(`(?i)\[BD\]|\bBD\b`), "BD"},
		{regexp.MustCompile(`(?i)WEB-DL`), "WEB-DL"},
		{regexp.MustCompile(`(?i)WEBRip`), "WEBRip"},
		{regexp.MustCompile(`(?i)HDTV`), "HDTV"},
		{regexp.MustCompile(`(?i)\bTV\s*Rip|\[TV\]`), "TV"},
		{regexp.MustCompile(`(?i)DVD`), "DVD"},
	}

	// Codec patterns
	reCodecPatterns = []struct {
		re    *regexp.Regexp
		codec string
	}{
		{regexp.MustCompile(`(?i)HEVC|\s*x265\s*`), "HEVC"},
		{regexp.MustCompile(`(?i)AV1`), "AV1"},
		{regexp.MustCompile(`(?i)H\.264|\s*x264\s*`), "H.264"},
		{regexp.MustCompile(`(?i)XviD`), "XviD"},
	}

	// Audio patterns
	reAudioPatterns = []struct {
		re    *regexp.Regexp
		audio string
	}{
		{regexp.MustCompile(`(?i)FLAC`), "FLAC"},
		{regexp.MustCompile(`(?i)DTS-HD`), "DTS-HD"},
		{regexp.MustCompile(`(?i)TrueHD`), "TrueHD"},
		{regexp.MustCompile(`(?i)DDP|Dolby\s*Digital\s*Plus`), "DDP"},
		{regexp.MustCompile(`(?i)AAC`), "AAC"},
		{regexp.MustCompile(`(?i)AC3|Dolby\s*Digital`), "AC3"},
		{regexp.MustCompile(`(?i)MP3`), "MP3"},
	}

	// Season/Part name stripping patterns (used to sanitize batch search queries)
	reSeasonNamePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s+Season\s*\d+`),
		regexp.MustCompile(`(?i)\s+S\s*\d+`),
		regexp.MustCompile(`(?i)\s+\d+(?:st|nd|rd|th)\s+Season`),
		regexp.MustCompile(`(?i)\s+Cour\s*\d+`),
		regexp.MustCompile(`(?i)\s+Part\s*\d+`),
	}

	// Part/Cour number extraction (ordered by specificity)
	rePartPatterns = []struct {
		re   *regexp.Regexp
		desc string
	}{
		{regexp.MustCompile(`(?i)\|\s*Part\s*0*(\d+)`), "| Part 02"},
		{regexp.MustCompile(`(?i)\(Part\s*(\d+)\)`), "(Part 2)"},
		{regexp.MustCompile(`(?i)\[Part\s*(\d+)\]`), "[Part 2]"},
		{regexp.MustCompile(`(?i)\bPart\s*(\d+)\b`), "Part 2"},
		{regexp.MustCompile(`(?i)\bCour\s*(\d+)\b`), "Cour 2"},
	}

	// Any part/cour declaration in the name, to detect MULTIPLE ones: "(Part 1 + Part 2)" and "(Season
	// 4 Part 03+04)" are not packs of part 1/3 (see ExtractPart). The optional group covers the
	// compact form, where the second number doesn't repeat the word "Part".
	reAnyPart = regexp.MustCompile(`(?i)\b(?:part|cour)\s*0*(\d+)(?:\s*[+&]\s*0*(\d+))?`)

	// Part stripping from query names (companion to reSeasonStrip)
	rePartStrip = regexp.MustCompile(`(?i)\s+(?:part\s*\d+|cour\s*\d+)`)

	// Size parsing
	reParseSizeRe = regexp.MustCompile(`(?i)^([\d.]+)\s*([KMGT]?i?B)$`)

	// Everything that isn't a letter, digit or space, for removeSpecialCharacters. It DELETES what
	// matches ("Re:Zero" -> "rezero"); not to be confused with reMatchNonAlpha (nyaa_match.go), which
	// replaces with a space ("Re:Zero" -> "re zero"). The two normalizations differ on purpose: this
	// one builds the search query, that one tokenizes to match titles.
	reSpecialChars = regexp.MustCompile(`[^a-z0-9\s]`)
)

// videoExtEpisodeRegexp is the " 05.mkv" pattern over every key of videoExtensions. Built from the
// map so a new extension cannot number in IsVideoFile and stay unnumbered here. The dash terminator
// stays without `\.`: `- 17.5 (OVA)` would then parse as 17.
func videoExtEpisodeRegexp() *regexp.Regexp {
	exts := make([]string, 0, len(videoExtensions))
	for ext := range videoExtensions {
		exts = append(exts, regexp.QuoteMeta(strings.TrimPrefix(ext, ".")))
	}
	sort.Strings(exts)
	return regexp.MustCompile(`(?i)\s(\d{1,4})\.(?:` + strings.Join(exts, "|") + `)`)
}
