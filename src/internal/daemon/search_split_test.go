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
	results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, 11)

	if len(results) != 1 || !strings.Contains(results[0].Name, "Steel Ball Run - 02") {
		t.Fatalf("episode 1 must resolve to the base title's episode 02, got %v", torrentNames(results))
	}
}

func TestSearchSingleEpisode_SplitEntryNeedsTheSameBaseTitle(t *testing.T) {
	defer mockNyaaListing(t, steelBallRunRows(), nil)()

	// A prequel with another base is a different show: no base-title search, no offset.
	titles, synonyms, relations := steelBallRunMedia("JoJo no Kimyou na Bouken: Ougon no Kaze - Part 1", anilist.MediaFormatONA)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, 11); len(results) != 0 {
		t.Fatalf("a prequel with another base title must not trigger the fallback, got %v", torrentNames(results))
	}
}

func TestSearchSingleEpisode_SplitEntryIgnoresMoviePrequel(t *testing.T) {
	defer mockNyaaListing(t, steelBallRunRows(), nil)()

	titles, synonyms, relations := steelBallRunMedia("JoJo no Kimyou na Bouken: Steel Ball Run - 1st STAGE", anilist.MediaFormatMovie)
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, synonyms, relations, 11); len(results) != 0 {
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
