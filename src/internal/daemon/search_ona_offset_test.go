package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/torrents"
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

// A manual download of Stone Ocean Part 2 episode 1 must reach the offset fallback like the
// scheduled pass does, so it needs the media's synonyms and relations.
func TestManualDownloadEpisode_ONASplitUsesTheOffsetFallback(t *testing.T) {
	// The same media as stoneOceanMedia(), as the GetMediaByID response.
	const media = `{"data": {"Media": {
		"id": 146722, "format": "ONA", "status": "FINISHED", "episodes": 26,
		"title": {"english": "JoJo's Bizarre Adventure: STONE OCEAN Part 2", "romaji": "JoJo no Kimyou na Bouken: Stone Ocean Part 2"},
		"synonyms": [],
		"relations": {"edges": [{"relationType": "PREQUEL", "node": {
			"title": {"romaji": "JoJo no Kimyou na Bouken: Stone Ocean"}, "episodes": 12, "format": "ONA"}}]},
		"coverImage": {"large": "", "medium": ""},
		"airingSchedule": {"nodes": []}
	}}}`
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, media)()
	defer mockNyaaListing(t, stoneOceanRows(), nil)()

	configs := standaloneTestConfig()
	backend := &magnetRecorder{FakeBackend: torrents.NewFakeBackend()}
	ep, err := ManualDownloadEpisode(&mockFileManagerForEpisodes{}, backend, 146722, 1, configs)
	if err != nil {
		t.Fatalf("the offset fallback must find episode 1 as \"Stone Ocean 13\": %v", err)
	}
	// Row 1 of stoneOceanRows is "Stone Ocean 13 v2".
	if len(backend.magnets) != 1 || backend.magnets[0] != "magnet:?xt=urn:btih:1" || ep.EpisodeNumber != 1 {
		t.Fatalf("want episode 1 downloaded from \"Stone Ocean 13 v2\", added %v, got %+v", backend.magnets, ep)
	}
}

// magnetRecorder records which magnet was added: the fixture magnets ("btih:1") aren't valid info
// hashes, so the fake can't derive a hash from them and the test hands it one.
type magnetRecorder struct {
	*torrents.FakeBackend
	magnets []string
}

func (m *magnetRecorder) Add(magnet string) (string, error) {
	m.magnets = append(m.magnets, magnet)
	m.NextHash = "hash-" + magnet
	return m.FakeBackend.Add(magnet)
}
