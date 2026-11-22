package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
)

// Stone Ocean Part 2 (AniList 146722): Netflix split Stone Ocean into ONA entries, and the
// single-episode groups number it continuously from the first one, so AniList episode 1 is
// "Stone Ocean 13". No single says "Part 2", so only the offset fallback can find it.
func stoneOceanRows() string {
	return previewListingHTML(
		previewListingRow(1, "[inid4c] JoJo's Bizarre Adventure - Stone Ocean 13 v2 (1080p)", "1.4 GiB", "30", "1"),
		previewListingRow(2, "[inid4c] JoJo's Bizarre Adventure - Stone Ocean 14 (1080p)", "1.4 GiB", "30", "1"),
		previewListingRow(4, "[inid4c] JoJo's Bizarre Adventure - Stone Ocean 38 v2 (1080p)", "1.4 GiB", "30", "1"),
		previewListingRow(3, "[Anime Chap] JoJo's Bizarre Adventure Part 6: Stone Ocean S05E13 [WEB 1080p] - Episode 13", "1.4 GiB", "30", "1"),
	)
}

func stoneOceanMedia() (anilist.Title, anilist.MediaRelations) {
	twelve := 12
	titles := anilist.Title{
		Romaji:  strPtr("JoJo no Kimyou na Bouken: Stone Ocean Part 2"),
		English: strPtr("JoJo's Bizarre Adventure: STONE OCEAN Part 2"),
	}
	relations := anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{
		{RelationType: "PREQUEL", Node: anilist.MediaRelationNode{
			Title:    anilist.Title{Romaji: strPtr("JoJo no Kimyou na Bouken: Stone Ocean")},
			Episodes: &twelve,
			Format:   anilist.MediaFormatONA,
		}},
	}}
	return titles, relations
}

func TestSearchSingleEpisode_ONASplitCountsTheONAPrequel(t *testing.T) {
	defer mockNyaaListing(t, stoneOceanRows(), nil)()

	titles, relations := stoneOceanMedia()
	for ep, want := range map[int]string{1: "Stone Ocean 13 v2", 2: "Stone Ocean 14", 26: "Stone Ocean 38 v2"} {
		results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: ep}, titles, nil, relations, anilist.MediaFormatONA, 26)
		if len(results) != 1 || !strings.Contains(results[0].Name, want) {
			t.Errorf("episode %d must resolve to %q, got %v", ep, want, torrentNames(results))
		}
	}
}

func TestSearchSingleEpisode_TVEntryIgnoresTheONAPrequel(t *testing.T) {
	defer mockNyaaListing(t, stoneOceanRows(), nil)()

	// An ONA before a TV entry is a side story, not part of the count.
	titles, relations := stoneOceanMedia()
	if results := searchNyaaForSingleEpisode(anilist.AiringNode{Episode: 1}, titles, nil, relations, anilist.MediaFormatTV, 26); len(results) != 0 {
		t.Fatalf("a TV entry must not add an ONA prequel's episodes, got %v", torrentNames(results))
	}
}
