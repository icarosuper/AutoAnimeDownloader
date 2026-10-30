package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"testing"
	"time"
)

// The real case: Mushoku Tensei S1 downloaded as ONE torrent of the whole season (1..23) under the
// cour 1 entry (108465, 11 episodes). Cour 2 (127720) is another media id, with its own numbering
// 1..12, and without ownership by coverage it downloads the whole season again from Nyaa.
const (
	cour1ID    = 108465
	cour2ID    = 127720
	seasonHash = "aa11bb22cc33dd44ee55"
)

// cour2Anime builds the cour 2 entry with n episodes already aired, all pending.
func cour2Anime(n int) anilist.MediaList {
	title := "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2"
	nodes := make([]anilist.AiringNode, n)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 2000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	return anilist.MediaList{
		Id:       cour2ID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:             cour2ID,
			Status:         anilist.MediaStatusFinished,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
}

// seasonPackOnDisk are the 11 records of cour 1, all pointing to the pack that declares 1..23.
func seasonPackOnDisk() []files.EpisodeStruct {
	saved := make([]files.EpisodeStruct, 11)
	for i := range saved {
		saved[i] = files.EpisodeStruct{
			AnimeID:       cour1ID,
			AnimeName:     "Mushoku Tensei: Isekai Ittara Honki Dasu",
			EpisodeHash:   seasonHash,
			EpisodeNumber: i + 1,
			IsBatch:       true,
			BatchStart:    1,
			BatchEnd:      23,
			DownloadDate:  time.Now(),
		}
	}
	return saved
}

// countingSearcher fails the test if any search is fired, and returns a pack that would match
// everything — if the adoption doesn't work, the test flags it AND the daemon would have
// downloaded.
func countingSearcher(hit *bool) nyaaSearcher {
	return nyaaSearcher{
		searchAnime: func(_ anilist.Title, _ []string, _ []int) []nyaa.TorrentResult {
			*hit = true
			return []nyaa.TorrentResult{{Name: "[EMBER] Mushoku Tensei (2021) (Season 1) [Batch]", MagnetLink: "magnet:?xt=urn:btih:outro", IsBatch: true}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ int) []nyaa.TorrentResult {
			*hit = true
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool) []nyaa.TorrentResult { return nil },
	}
}

func coverageConfigs() *files.Config {
	return &files.Config{MaxEpisodesPerAnime: 12, EpisodeRetryLimit: 1}
}

// TestCoverageOwnership_SeasonPackCoversNextCour: the 1..23 pack downloaded under cour 1 covers, on
// the absolute axis, the 12 episodes of cour 2 (offset 11 → abs 12..23). Nothing goes to Nyaa,
// nothing is downloaded, and the 12 new records point to the hash already on disk.
func TestCoverageOwnership_SeasonPackCoversNextCour(t *testing.T) {
	searched := false
	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(
		coverageConfigs(), backend, cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "[EMBER] Mushoku Tensei (2021) (Season 1) [Batch]", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if searched {
		t.Error("must not search Nyaa: the already downloaded pack covers the whole window")
	}
	if len(backend.List()) > 0 {
		t.Errorf("must not add a torrent, but the client has %d", len(backend.List()))
	}
	if len(result.newEpisodes) != 12 {
		t.Fatalf("expected 12 adopted records, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.AnimeID != cour2ID || ep.EpisodeHash != seasonHash || !ep.IsBatch {
			t.Fatalf("adopted record is wrong: %+v", ep)
		}
		// The range is CONVERTED to cour 2's ruler: cour 1's 1..23 pack (offset 0) is -10..12 under cour
		// 2 (offset 11). It's exactly what assignBatches would record if the search found the same
		// torrent again (packAxis.localRange, decisions.md #79).
		if ep.BatchStart != -10 || ep.BatchEnd != 12 {
			t.Fatalf("the adopted record's range should be -10-12, got %d-%d", ep.BatchStart, ep.BatchEnd)
		}
		if ep.EpisodeName != "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2 1-12" {
			t.Fatalf("the displayed name must not leak the negative range, got %q", ep.EpisodeName)
		}
	}
}

// The recorded range is read back adding the offset of the record's OWN anime_id. If the adoption
// copied the owner's range without converting, the cour 2 record (1..23 under offset 11) would be
// read as the absolute 12..34 and a third cour would find coverage for an episode the pack doesn't
// have — marked as downloaded, never searched, nonexistent file.
func TestCoverageOwnership_AdoptedRangeDoesNotShiftForTheNextCour(t *testing.T) {
	const cour3ID = 146065
	index := map[int]anilist.Series{
		cour1ID: {Key: cour1ID, Offset: 0},
		cour2ID: {Key: cour1ID, Offset: 11},
		cour3ID: {Key: cour1ID, Offset: 23},
	}
	hashes := map[string]bool{seasonHash: true}
	pending := anilist.EpisodeList(cour2Anime(12), 1)

	adopted := adoptCoveredEpisodes(cour2Anime(12), "Cour 2", 12, pending, seasonPackOnDisk(), index, hashes)
	if len(adopted) != 12 {
		t.Fatalf("expected the 12 of cour 2 adopted, got %d", len(adopted))
	}

	// Cour 3 starts at absolute 24, one step after the end of the pack.
	cour3 := cour2Anime(13)
	cour3.Id, cour3.Media.Id = cour3ID, cour3ID
	saved := append(seasonPackOnDisk(), adopted...)
	if leaked := adoptCoveredEpisodes(cour3, "Cour 3", 13, anilist.EpisodeList(cour3, 1), saved, index, hashes); len(leaked) != 0 {
		t.Fatalf("the pack ends at absolute 23: cour 3 can't adopt anything, adopted %d (ep %d)", len(leaked), leaked[0].EpisodeNumber)
	}
}

// TestCoverageOwnership_UncoveredEpisodesStillSearch: cour 2 has 14 episodes, the pack covers up to
// absolute 23 (= local 12). The two left over still go to the search — partial adoption can't
// become "everything is solved".
func TestCoverageOwnership_UncoveredEpisodesStillSearch(t *testing.T) {
	searched := false
	configs := coverageConfigs()
	configs.MaxEpisodesPerAnime = 14

	result := processAnimeEpisodes(
		configs, torrents.NewFakeBackend(), cour2Anime(14),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if !searched {
		t.Error("episodes 13 and 14 aren't covered by the pack: the search has to run")
	}
	adopted := 0
	for _, ep := range result.newEpisodes {
		if ep.EpisodeHash == seasonHash {
			adopted++
		}
	}
	if adopted != 12 {
		t.Errorf("expected 12 adopted episodes, got %d", adopted)
	}
}

// TestCoverageOwnership_OtherSeriesIsNotOwnership: without the Key comparison, "absolute 12" would
// match the pack of any other anime that has 12 in its range.
func TestCoverageOwnership_OtherSeriesIsNotOwnership(t *testing.T) {
	searched := false

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: 16498, Offset: 11}, // another series, same offset
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if !searched {
		t.Error("a pack from another series isn't ownership: the search has to run")
	}
}

// TestCoverageOwnership_PackNotInSessionIsNotOwnership: a record pointing to a torrent no longer in
// the session proves nothing — adopting would give a record without a file, invisible to the
// re-download.
func TestCoverageOwnership_PackNotInSessionIsNotOwnership(t *testing.T) {
	searched := false

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		nil, // empty session
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if !searched {
		t.Error("a torrent outside the session isn't ownership: the search has to run")
	}
}

// TestCoverageOwnership_UnknownRangeIsNotOwnership: a pack without a range in its name records
// BatchStart == 0. Unknown isn't "covers everything" — same ceiling as canRemoveTorrent
// (decisions.md #74).
func TestCoverageOwnership_UnknownRangeIsNotOwnership(t *testing.T) {
	searched := false

	saved := seasonPackOnDisk()
	for i := range saved {
		saved[i].BatchStart, saved[i].BatchEnd = 0, 0
	}

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		saved,
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if !searched {
		t.Error("a pack without a declared range isn't ownership: the search has to run")
	}
}

const kaguyaS2ID = 112641

func kaguyaS2Anime(n int) anilist.MediaList {
	title := "Kaguya-sama: Love is War Season 2"
	nodes := make([]anilist.AiringNode, n)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 3000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	return anilist.MediaList{
		Id:       kaguyaS2ID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:             kaguyaS2ID,
			Status:         anilist.MediaStatusFinished,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
}

// Restart packs flatten onto 1-N at download, so S2 adopts the EMBER S1+S2 hash the same
// way Mushoku cour 2 adopts a 1-23 pack. nyaa.si/view/1355132.
func TestCoverageOwnership_KaguyaRestartPackCoversS2(t *testing.T) {
	defer mockKaguyaFileList(emberKaguyaS1S2HTML())()

	info, ok := nyaa.PackFileRange("https://nyaa.si/view/1355132", 12)
	if !ok {
		t.Fatal("want PackFileRange to read the EMBER S1+S2 file list")
	}
	if info.StartEpisode != 1 || info.EndEpisode != 24 {
		t.Fatalf("flattened range: want 1-24, got %d-%d", info.StartEpisode, info.EndEpisode)
	}

	const hash = "kaguyaemberhashkaguyaemberhashkaguyaem"
	saved := packRange(kaguyaS1ID, 1, 12, 12, info.StartEpisode, info.EndEpisode, hash)
	searched := false
	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(
		coverageConfigs(), backend, kaguyaS2Anime(12),
		[]torrents.TorrentInfo{{Name: "[EMBER] Kaguya-sama (Season 1+2) [Batch]", Hash: hash}},
		saved,
		map[int]anilist.Series{
			kaguyaS1ID: {Key: kaguyaS1ID, Offset: 0},
			kaguyaS2ID: {Key: kaguyaS1ID, Offset: 12},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched), nil,
	)

	if searched {
		t.Error("must not search Nyaa: the flattened S1+S2 pack covers S2")
	}
	if len(backend.List()) > 0 {
		t.Errorf("must not add a torrent, but the client has %d", len(backend.List()))
	}
	if len(result.newEpisodes) != 12 {
		t.Fatalf("expected 12 adopted records, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.AnimeID != kaguyaS2ID || ep.EpisodeHash != hash || !ep.IsBatch {
			t.Fatalf("adopted record is wrong: %+v", ep)
		}
		if ep.BatchStart != -11 || ep.BatchEnd != 12 {
			t.Fatalf("S1's 1-24 pack under offset 12 is -11-12, got %d-%d", ep.BatchStart, ep.BatchEnd)
		}
		if ep.SeriesOffset != 12 {
			t.Fatalf("adopted SeriesOffset: want 12, got %d", ep.SeriesOffset)
		}
		if ep.AnimeSeason == nil || *ep.AnimeSeason != 2 {
			t.Fatalf("adopted AnimeSeason: want 2, got %v", ep.AnimeSeason)
		}
	}
}
