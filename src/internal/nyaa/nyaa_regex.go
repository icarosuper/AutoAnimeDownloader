package nyaa

import "regexp"

// Pre-compiled regexes used across the nyaa package.
// Compiling on first call vs at package init is a significant speedup since
// these are called inside per-torrent loops.

var (
	// Season stripping from query names
	reSeasonStrip = regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)

	// Batch detection patterns
	reBatchPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\(batch\)|\[batch\]|\s+batch\s+`),
		regexp.MustCompile(`(?i)\(unofficial\s+batch\)`),
		regexp.MustCompile(`(?i)\(\d{1,3}\s*[-~]\s*\d{1,3}\)`),
		regexp.MustCompile(`(?i)\s\d{2,3}\s*[-~]\s*\d{2,3}\s`),
		regexp.MustCompile(`(?i)\s\d{2,3}~\d{2,3}\s`),
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
	reBatchRange    = regexp.MustCompile(`(?i)(\d{1,3})\s*[-~]\s*(\d{1,3})`)
	reBatchComplete = regexp.MustCompile(`(?i)complete|batch`)

	// Movie keyword patterns
	reMovieKeywords = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\(movie\)|\[movie\]|\smovie\s`),
		regexp.MustCompile(`(?i)\(film\)|\[film\]`),
		regexp.MustCompile(`(?i)gekijouban`),
		regexp.MustCompile(`(?i)theater\s+version`),
		regexp.MustCompile(`(?i)theatrical`),
	}
	reOvaPattern     = regexp.MustCompile(`(?i)\(?(ova|ona)\)?|original\s+(video|net)\s+animation`)
	reSpecialPattern = regexp.MustCompile(`(?i)\(special\)|\[special\]|\sspecial\s|(tv\s+special|episode\s+sp)`)
	reHasEpisode     = regexp.MustCompile(`(?i)(-\s?\d{1,3}|episode\s*\d{1,3}|s\d{1,2}e\d{1,3})`)

	// Fansub extraction
	reFansub = regexp.MustCompile(`(?i)^\[([^\]]+)\]|^\(([^\)]+)\)`)

	// Episode number patterns (ordered by specificity)
	reEpisodePatterns = []struct {
		re   *regexp.Regexp
		desc string
	}{
		{regexp.MustCompile(`(?i)S\d{1,2}E(\d{1,3})`), "S01E05"},
		{regexp.MustCompile(`(?i)\s+-\s+(\d{1,3})(?:\s|v\d+|$|\[)`), "- 05, - 05v2"},
		{regexp.MustCompile(`(?i)\s+-\s+(\d{1,3})\s*\(`), "- 05 (1080p)"},
		{regexp.MustCompile(`(?i)EP\s*(\d{1,3})\b`), "EP05"},
		{regexp.MustCompile(`(?i)Episode\s*(\d{1,3})\b`), "Episode 05"},
		{regexp.MustCompile(`(?i)\bE(\d{1,3})\b`), "E05"},
		{regexp.MustCompile(`(?i)\[(\d{1,3})\]`), "[05]"},
		{regexp.MustCompile(`(?i)\s(\d{1,3})\s*\(`), " 05 ("},
		{regexp.MustCompile(`(?i)\s(\d{1,3})\.mkv`), " 05.mkv"},
		{regexp.MustCompile(`(?i)\s(\d{1,3})$`), " 5"},
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
		{regexp.MustCompile(`(?i)WEB-DL`), "WEB-DL"},
		{regexp.MustCompile(`(?i)WEBRip`), "WEBRip"},
		{regexp.MustCompile(`(?i)HDTV`), "HDTV"},
		{regexp.MustCompile(`(?i)\bTV\s*Rip|\bTV\b`), "TV"},
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
		{regexp.MustCompile(`(?i)\|\s*Part\s*0*(\d+)`),  "| Part 02"},
		{regexp.MustCompile(`(?i)\(Part\s*(\d+)\)`),     "(Part 2)"},
		{regexp.MustCompile(`(?i)\[Part\s*(\d+)\]`),     "[Part 2]"},
		{regexp.MustCompile(`(?i)\bPart\s*(\d+)\b`),     "Part 2"},
		{regexp.MustCompile(`(?i)\bCour\s*(\d+)\b`),     "Cour 2"},
	}

	// Part stripping from query names (companion to reSeasonStrip)
	rePartStrip = regexp.MustCompile(`(?i)\s+(?:part\s*\d+|cour\s*\d+)`)

	// Size parsing
	reParseSizeRe = regexp.MustCompile(`(?i)^([\d.]+)\s*([KMGT]?i?B)$`)
)
