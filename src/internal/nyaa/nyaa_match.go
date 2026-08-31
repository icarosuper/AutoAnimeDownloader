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
	// WEB-DL splits into "web" + "dl" after stripping hyphens; add both
	"web": true, "dl": true, "rip": true,
	// Audio codecs
	"flac": true, "aac": true, "dts": true, "ddp": true, "ac3": true,
	"opus": true, "truehd": true, "mp3": true,
	// Versioned audio codec tags (e.g. AAC2.0 → "aac2", DDP2.0 → "ddp2", EAC3)
	"eac3": true, "aac2": true, "ddp2": true,
	// Encoding details
	"remux": true, "hi10": true, "hi10p": true, "10bit": true, "8bit": true,
	// Language tags (unambiguous 3-char codes)
	"eng": true, "jpn": true,
	// Streaming service source tags
	"nf": true, "amzn": true, "cr": true,
	// Known fansubs
	"subsplease": true, "erai": true, "raws": true, "judas": true,
	"toonshub": true, "asw": true, "ember": true,
	// Season/episode/structure keywords
	"season": true, "episode": true, "cour": true, "part": true,
	// Batch/collection markers (type already checked by IsBatch/isMovie)
	"batch": true, "complete": true, "movie": true, "ova": true, "ona": true, "special": true, "film": true,
	// Subtitle/audio track tags
	"multisub": true, "multi": true, "subs": true, "dual": true, "audio": true,
	// Release type tags
	"repack": true,
	// Misc unambiguous tags
	"uncensored": true,
}

var (
	reMatchNonAlpha   = regexp.MustCompile(`[^a-z0-9]+`)
	reMatchAllDigits  = regexp.MustCompile(`^\d+$`)
	reMatchVersionSfx = regexp.MustCompile(`^\d+v\d+$`)            // 03v2
	reMatchSxEx       = regexp.MustCompile(`^s\d+e\d+$`)           // s01e05
	reMatchSeasonOnly = regexp.MustCompile(`^s\d+$`)               // s1, s2, s01
	reMatchEpPrefix   = regexp.MustCompile(`^ep\d+$`)              // ep05, ep015
	reMatchEpShort    = regexp.MustCompile(`^e\d+$`)               // e05, e015
	reMatchResToken   = regexp.MustCompile(`^\d{3,4}p$`)           // 1080p, 720p
	reMatchOrdinal    = regexp.MustCompile(`^\d+(?:st|nd|rd|th)$`) // 1st, 2nd
	reMatchHexHash    = regexp.MustCompile(`^[0-9a-f]{6,10}$`)
	reLeadingBracket  = regexp.MustCompile(`^\s*(?:\[[^\]]*\]|\([^)]*\))\s*`)
	reBracketGroup    = regexp.MustCompile(`\[[^\]]*\]`)
	reParenGroup      = regexp.MustCompile(`\(([^)]*)\)`)
)

// firstMarkerIndex returns the index of the earliest season/episode marker in the name
// (S01E05, " - 05", "Episode 3", "Season 2", etc), or -1 when there is none.
func firstMarkerIndex(name string) int {
	earliest := -1
	for _, p := range reEpisodePatterns {
		if loc := p.re.FindStringIndex(name); loc != nil && (earliest == -1 || loc[0] < earliest) {
			earliest = loc[0]
		}
	}
	for _, p := range reSeasonPatterns {
		if loc := p.re.FindStringIndex(name); loc != nil && (earliest == -1 || loc[0] < earliest) {
			earliest = loc[0]
		}
	}
	return earliest
}

// truncateAtFirstMarker cuts a torrent name at the earliest season/episode marker. Most of what
// follows is episode title, resolution, codec and fansub noise that inflates the Jaccard union
// enough to sink genuine matches, so it's dropped rather than tokenized.
//
// ATENCAO: o corte NAO e sempre seguro. Varios grupos escrevem um titulo alternativo do anime
// DEPOIS do marcador — o EMBER poe o marcador dentro de "(Season 2 | Part 2)" e o romaji completo
// tres grupos adiante. Por isso quem casa titulo nao usa so este corte: titleMatchesQuery tenta
// tambem os candidatos de altTitleCandidates.
func truncateAtFirstMarker(name string) string {
	if earliest := firstMarkerIndex(name); earliest > 0 {
		return name[:earliest]
	}
	return name
}

// altTitleCandidates extrai, do texto que vem DEPOIS do marcador, os titulos alternativos do
// anime: o conteudo de cada grupo entre parenteses e cada segmento separado por barra. Colchetes
// e texto solto ficam de fora de proposito — sao titulo de episodio, resolucao, codec e tag de
// release.
//
// UM CANDIDATO POR TRECHO, e nao um saco unico de tokens. O EMBER poe o titulo em ingles antes do
// marcador e o romaji depois; juntar os dois num conjunto so dilui os dois no Jaccard (medido:
// 0.78, contra 1.00 do romaji sozinho). Separados, cada titulo e comparado como titulo, e um
// trecho de outro anime nao empresta token para o vizinho.
func altTitleCandidates(tail string) []string {
	tail = reBracketGroup.ReplaceAllString(tail, " ")

	var out []string
	for _, m := range reParenGroup.FindAllStringSubmatch(tail, -1) {
		out = append(out, strings.Split(m[1], "|")...)
	}
	// Barras FORA de parenteses: Diddy, NTRX e Fuchs escrevem o romaji depois de um "|" solto.
	// O primeiro segmento e o resto do marcador, nao um titulo — por isso [1:].
	if parts := strings.Split(reParenGroup.ReplaceAllString(tail, " "), "|"); len(parts) > 1 {
		out = append(out, parts[1:]...)
	}
	return out
}

// extractTitleTokens returns meaningful title tokens from a torrent name or anime title,
// stripping technical metadata (resolution, codec, fansub, episode/season numbers, etc.)
func extractTitleTokens(name string) []string {
	return titleTokens(truncateAtFirstMarker(name))
}

func titleTokens(name string) []string {
	// A leading fansub tag ("[SubsPlease] ", "(Erai-raws) ") survives truncation
	// since it sits before the marker; strip it so it doesn't count as a title token.
	name = reLeadingBracket.ReplaceAllString(name, "")
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

// jaccardThreshold returns the Jaccard threshold based on query length.
// Short queries (≤3 tokens) need a high threshold to prevent spinoffs like
// "SAO" matching "SAO Alternative Gun Gale Online". Long queries (≥4 tokens)
// can use a lower threshold because some fansubs embed both the English and
// Japanese title in the torrent name, inflating the union without indicating
// a wrong match (e.g. ToonsHub including both titles in parentheses).
func jaccardThreshold(queryLen int) float64 {
	if queryLen >= 4 {
		return 0.4
	}
	return 0.8
}

// titleMatchesQuery checks if a torrent name matches a search query using two complementary checks:
//  1. All query title tokens must be present in the torrent title (prevents partial-title matches
//     like "Chinmoku no Majo" matching a query that also requires "Silent Witch")
//  2. Jaccard similarity >= threshold (prevents spinoffs/sequels with extra title words,
//     e.g. "SAO Alternative Gun Gale Online" for a "SAO" query)
func titleMatchesQuery(torrentName, query string) bool {
	queryTokens := extractTitleTokens(query)

	// If query has no meaningful tokens, fall back to permissive match
	if len(queryTokens) == 0 {
		return true
	}

	if tokensMatchQuery(extractTitleTokens(torrentName), queryTokens) {
		return true
	}

	// So entao os titulos alternativos que vem depois do marcador, um de cada vez. Tentar isto
	// APENAS quando o corte falha e o que preserva o comportamento de hoje inteiro: o corte no
	// marcador e o que protege query curta de afundar no Jaccard, e o pack do EMBER de Kimetsu no
	// Yaiba (query de 3 tokens) casa no corte e nunca chega aqui.
	marker := firstMarkerIndex(torrentName)
	if marker <= 0 {
		return false
	}
	for _, alt := range altTitleCandidates(torrentName[marker:]) {
		if tokensMatchQuery(titleTokens(alt), queryTokens) {
			return true
		}
	}
	return false
}

// tokensMatchQuery aplica as duas checagens do match de titulo a um conjunto de tokens ja pronto.
func tokensMatchQuery(torrentTokens, queryTokens []string) bool {
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
	return jaccardSimilarity(torrentTokens, queryTokens) >= jaccardThreshold(len(queryTokens))
}

// TitleMatchesQuery is the exported version for testing.
func TitleMatchesQuery(torrentName, query string) bool {
	return titleMatchesQuery(torrentName, query)
}
