package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/nyaa"
)

func strPtr(s string) *string { return &s }

// Steel Ball Run (AniList 210482): AniList splits one show into "- 1st STAGE" (1-episode ONA) and
// "- 2nd & 3rd STAGE". Every group names it by the base title and counts on from the 1st STAGE, so
// AniList episode 1 is "Steel Ball Run - 02". The TVDB-numbered rows (S06E..) stay out: their season
// is not this entry's.
func steelBallRunRows() string {
	return previewListingHTML(
		previewListingRow(1, "[Erai-raws] JoJo no Kimyou na Bouken: Steel Ball Run - 01 [1080p NF WEB-DL AVC AAC][MultiSub][AAAAAAAA]", "1.4 GiB", "300", "1"),
		previewListingRow(2, "[Erai-raws] JoJo no Kimyou na Bouken: Steel Ball Run - 02 [1080p NF WEB-DL AVC AAC][MultiSub][C269BEB0]", "1.4 GiB", "200", "1"),
		previewListingRow(3, "[ToonsHub] JoJos Bizarre Adventure S06E02 1080p NF WEB-DL DUAL AAC2.0 H.264 (JoJo no Kimyou na Bouken: Steel Ball Run - 2nd - 3rd STAGE, Dual-Audio, Multi-Subs)", "1.4 GiB", "100", "1"),
	)
}

func steelBallRunMedia(prequelRomaji string, prequelFormat anilist.MediaFormat) (anilist.Title, []string, anilist.MediaRelations) {
	one := 1
	titles := anilist.Title{
		Romaji:  strPtr("JoJo no Kimyou na Bouken: Steel Ball Run - 2nd & 3rd STAGE"),
		English: strPtr("STEEL BALL RUN JoJo's Bizarre Adventure 2nd - 3rd STAGE"),
	}
	synonyms := []string{"JoJo's Bizarre Adventure: Part 7–Steel Ball Run", "SBR"}
	relations := anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{
		{RelationType: "PREQUEL", Node: anilist.MediaRelationNode{
			Title:    anilist.Title{Romaji: strPtr(prequelRomaji)},
			Episodes: &one,
			Format:   prequelFormat,
		}},
		{RelationType: "ADAPTATION", Node: anilist.MediaRelationNode{
			Title:  anilist.Title{Romaji: strPtr("JoJo no Kimyou na Bouken: Steel Ball Run")},
			Format: anilist.MediaFormatManga,
		}},
	}}
	return titles, synonyms, relations
}

func TestSearchSingleEpisode_SplitEntryUsesBaseTitleAndPrequelOffset(t *testing.T) {
	defer mockNyaaListing(t, steelBallRunRows(), nil)()

	titles, synonyms, relations := steelBallRunMedia("JoJo no Kimyou na Bouken: Steel Ball Run - 1st STAGE", anilist.MediaFormatONA)
	results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, anilist.MediaFormatONA, 11)

	if len(results) != 1 || !strings.Contains(results[0].Name, "Steel Ball Run - 02") {
		t.Fatalf("episode 1 must resolve to the base title's episode 02, got %v", torrentNames(results))
	}
}

func TestSearchSingleEpisode_SplitEntryNeedsTheSameBaseTitle(t *testing.T) {
	defer mockNyaaListing(t, steelBallRunRows(), nil)()

	// A prequel with another base is a different show: no base-title search, no offset.
	titles, synonyms, relations := steelBallRunMedia("JoJo no Kimyou na Bouken: Ougon no Kaze - Part 1", anilist.MediaFormatONA)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, anilist.MediaFormatONA, 11); len(results) != 0 {
		t.Fatalf("a prequel with another base title must not trigger the fallback, got %v", torrentNames(results))
	}
}

func TestSearchSingleEpisode_SplitEntryIgnoresMoviePrequel(t *testing.T) {
	defer mockNyaaListing(t, steelBallRunRows(), nil)()

	titles, synonyms, relations := steelBallRunMedia("JoJo no Kimyou na Bouken: Steel Ball Run - 1st STAGE", anilist.MediaFormatMovie)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, anilist.MediaFormatONA, 11); len(results) != 0 {
		t.Fatalf("a movie prequel is not part of the episode count, got %v", torrentNames(results))
	}
}

func torrentNames(results []nyaa.TorrentResult) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Name)
	}
	return names
}

func splitEntry(romaji, prequelRomaji string, prequelEpisodes int) (anilist.Title, anilist.MediaRelations) {
	return anilist.Title{Romaji: strPtr(romaji)}, anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{
		{RelationType: "PREQUEL", Node: anilist.MediaRelationNode{
			Title:    anilist.Title{Romaji: strPtr(prequelRomaji)},
			Episodes: &prequelEpisodes,
			Format:   anilist.MediaFormatONA,
		}},
	}}
}

// A third split entry only sees its immediate prequel ("2nd STAGE", 12 eps), not the 1st STAGE
// before it, so prequel-count shifting would search "Base 13" — episode 1 of the 2nd STAGE.
func TestSearchSingleEpisode_SplitEntryRefusesAPrequelThatIsNotTheFirst(t *testing.T) {
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(1, "[Erai-raws] Base Show - 13 [1080p]", "1.4 GiB", "300", "1"),
	), nil)()

	titles, relations := splitEntry("Base Show - 3rd STAGE", "Base Show - 2nd STAGE", 12)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, nil, relations, anilist.MediaFormatONA, 12); len(results) != 0 {
		t.Fatalf("3rd STAGE ep 1 must not resolve to the 2nd STAGE's episode, got %v", torrentNames(results))
	}
}

// Arc suffixes carry no ordinal: groups may number each arc from 1, and an empty search right
// after airing must not fall through to another season's "Base - 14".
func TestSearchSingleEpisode_SplitEntryRefusesSuffixesWithoutAnOrdinal(t *testing.T) {
	defer mockNyaaListing(t, previewListingHTML(
		previewListingRow(1, "[Other] Base Show - 14 [1080p]", "1.4 GiB", "300", "1"),
	), nil)()

	titles, relations := splitEntry("Base Show - Arc B-hen", "Base Show - Arc A-hen", 11)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 3}, titles, nil, relations, anilist.MediaFormatONA, 12); len(results) != 0 {
		t.Fatalf("arc suffixes without an ordinal must not trigger the fallback, got %v", torrentNames(results))
	}
}

func TestSplitEntryContinuation_OrdinalRule(t *testing.T) {
	cases := []struct {
		entry, prequel string
		want           bool
	}{
		{"Base - 2nd & 3rd STAGE", "Base - 1st STAGE", true},
		{"Base - Second Season", "Base - First Season", true},
		{"Base - Part 2", "Base - Part 1", true},
		{"Base - Cour 2", "Base - Cour 1", true},
		{"Base - 3rd STAGE", "Base - 2nd STAGE", false},  // prequel is not the first
		{"Base - Arc B-hen", "Base - Arc A-hen", false},  // no ordinal at all
		{"Base - Final Arc", "Base - 1st STAGE", false},  // entry has no ordinal
		{"Base - 2nd STAGE", "Base - Prologue", false},   // prequel has no ordinal
		{"Base - 1st STAGE", "Base - 1st Season", false}, // entry ordinal is not later
		{"Base - Arc 2-hen", "Base - Arc 1-hen", false},  // a bare number is not an ordinal
	}
	for _, tc := range cases {
		titles, relations := splitEntry(tc.entry, tc.prequel, 12)
		if _, _, ok := splitEntryContinuation(titles, relations); ok != tc.want {
			t.Errorf("%q after %q: want %v, got %v", tc.entry, tc.prequel, tc.want, ok)
		}
	}
}

func TestSuffixOrdinal(t *testing.T) {
	cases := map[string]int{"2nd & 3rd STAGE": 2, "1st STAGE": 1, "Part 1": 1, "season 3": 3, "THIRD SEASON": 3, "Stage2": 2, "11th Cour": 11}
	for in, want := range cases {
		if got, ok := suffixOrdinal(in); !ok || got != want {
			t.Errorf("suffixOrdinal(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"Arc A-hen", "Arc 2-hen", "Final", "Ni no Shou", ""} {
		if got, ok := suffixOrdinal(in); ok {
			t.Errorf("suffixOrdinal(%q) = %d, want no ordinal", in, got)
		}
	}
}
