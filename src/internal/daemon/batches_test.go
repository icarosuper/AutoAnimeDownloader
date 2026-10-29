package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func pack(name string, n int) nyaa.TorrentResult {
	return nyaa.TorrentResult{Name: name, MagnetLink: fakeMagnet(n), IsBatch: true}
}

func TestPickBatches_OnePackCoversTheWindow(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	if got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 150, 161)); len(got) != 1 || got[0] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("first pending 150 must pick only the second pack, got %v", got)
	}
	if got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 1, 12)); len(got) != 1 || got[0] != "[X] Anime 001-100 [1080p]" {
		t.Errorf("first pending 1 must pick only the first pack, got %v", got)
	}
	if got := pickBatches(newPackSet(results, nil), packAxis{}, 500, 511); got.len() != 0 {
		t.Errorf("with no covering pack, want empty (the flow falls to loose episodes), got %v", batchNames(got))
	}
}

// Window crossing the boundary: 95..106 doesn't fit in a 100-episode pack, so BOTH go in, in order.
func TestPickBatches_WindowCrossingTheBoundaryPicksBoth(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 95, 106))
	if len(got) != 2 || got[0] != "[X] Anime 001-100 [1080p]" || got[1] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("want both packs in order, got %v", got)
	}
}

func TestPickBatches_CeilingOffCoversEverything(t *testing.T) {
	results := []nyaa.TorrentResult{
		pack("[X] Anime 1-100 [1080p]", 1),
		pack("[X] Anime 101-200 [1080p]", 2),
		pack("[X] Anime 201-300 [1080p]", 3),
	}

	if got := pickBatches(newPackSet(results, nil), packAxis{}, 1, math.MaxInt); got.len() != 3 {
		t.Errorf("with the ceiling off, want the three packs, got %v", batchNames(got))
	}
}

// The list order IS the quality order (SortTorrentResults ran before), so "the first that covers
// the cursor" is "the best that covers the cursor".
func TestPickBatches_RespectsQualityOrder(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[Good] Anime 001-100 [1080p]", 1), pack("[Worse] Anime 001-100 [480p]", 2)}

	got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 1, 12))
	if len(got) != 1 || got[0] != "[Good] Anime 001-100 [1080p]" {
		t.Errorf("want the first of the sorted list, got %v", got)
	}
}

// Termination: coveringBatch only returns a pack with EndEpisode >= cursor, so the cursor strictly
// grows and no pack is chosen twice.
func TestPickBatches_Terminates(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 50-60 [1080p]", 1)}

	got := pickBatches(newPackSet(results, nil), packAxis{}, 55, math.MaxInt)
	if got.len() != 1 {
		t.Errorf("want exactly 1 pack, got %d (%v)", got.len(), batchNames(got))
	}
}

// A pack without a range in its name counts as a complete pack: it covers the rest of the window
// and ends the search.
func TestPickBatches_UnknownRangeCountsAsComplete(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1), pack("[X] Anime 001-100", 2)}

	got := pickBatches(newPackSet(results, nil), packAxis{}, 1, math.MaxInt)
	if got.len() != 1 || got.items[0].torrent.Name != "[X] Anime Complete Batch" {
		t.Errorf("want only the complete pack, got %v", batchNames(got))
	}
}

// Two packs chosen: each episode gets the magnet of the pack of ITS range, and the overrideNames
// differ (on the downloads screen two packs of the same anime would show identical names).
func TestAssignBatches_EachEpisodeGetsItsOwnPack(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	first := pack("[X] Anime 001-100 [1080p]", 1)
	second := pack("[X] Anime 101-200 [1080p]", 2)

	episodes := anilist.EpisodeList(anime, 1)
	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{first, second}, nil))

	if len(covered) != 200 {
		t.Fatalf("want 200 covered episodes, got %d", len(covered))
	}
	if magnets[50].candidates[0].MagnetLink != first.MagnetLink || magnets[150].candidates[0].MagnetLink != second.MagnetLink {
		t.Error("each episode must get the magnet of the pack of its range")
	}
	if magnets[50].overrideName == magnets[150].overrideName {
		t.Errorf("the overrideNames must differ, both are %q", magnets[50].overrideName)
	}
	if magnets[50].overrideName != "Anime 1-100" {
		t.Errorf("want \"Anime 1-100\", got %q", magnets[50].overrideName)
	}
	if !magnets[50].skipSubfolder {
		t.Error("a pack episode needs skipSubfolder")
	}
}

// Partial coverage: the episodes outside do NOT fall into loose episodes (on One Piece that would
// fire ~1000 ReasonNotFound notifications per pass). The cut is a prefix.
func TestAssignBatches_TruncatesUncoveredEpisodes(t *testing.T) {
	anime := animeWithEpisodes(500, anilist.MediaStatusReleasing, false, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1)}, nil))

	if len(covered) != 100 {
		t.Fatalf("want 100 covered episodes, got %d", len(covered))
	}
	if covered[99].Episode != 100 {
		t.Errorf("the cut must keep the prefix, last covered = %d", covered[99].Episode)
	}
	if _, ok := magnets[101]; ok {
		t.Error("an episode outside the coverage must not get a magnet")
	}
}

// A pack without a range in its name covers everything: no truncation, and the name falls back to
// the anime title.
func TestAssignBatches_UnknownRangeUsesAnimeTitle(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)}, nil))

	if len(covered) != 26 {
		t.Fatalf("want 26 covered episodes, got %d", len(covered))
	}
	if magnets[1].overrideName != "Anime" {
		t.Errorf("an unknown range must use the anime title, got %q", magnets[1].overrideName)
	}
}

// The pack's REAL range (the one in the torrent name) must survive into the record: the detail
// screen built "Episodes 6–11" for a 01-11 pack downloaded with 5 episodes already watched, because
// it rebuilt the range from the min/max of the SAVED episodes — which are only the pending ones.
func TestAssignBatches_CarriesTheRealRange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 6)

	_, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime 01-11 [1080p]", 1)}, nil))

	if magnets[6].batchStart != 1 || magnets[6].batchEnd != 11 {
		t.Errorf("want the pack range (1-11), got %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
}

// A range neither the name nor the file list resolved records ZERO, not 1..total. The pack is still
// used (covering treats it as complete), but the record doesn't assert a coverage nobody checked —
// 1..total fed ownership by coverage with a guess (decisions.md #84).
func TestAssignBatches_UnresolvedRangeLeavesZeroes(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 6)

	_, magnets := assignBatches("Anime", packAxis{totalEpisodes: 26}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)}, nil))

	if magnets[6].batchStart != 0 || magnets[6].batchEnd != 0 {
		t.Errorf("an unresolved range must stay zero, got %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
	if magnets[6].overrideName != "Anime" {
		t.Errorf("without a range the displayed name is just the anime's, got %q", magnets[6].overrideName)
	}
}

// The bug, end to end: 11 episodes, 5 watched, pack 01-11. Only the pending ones become records
// (6..11), but ALL carry the pack's range.
func TestProcessAnimeEpisodes_SavesThePackRange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	anime.Progress = 5
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-11 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("want the 6 pending episodes recorded, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.BatchStart != 1 || ep.BatchEnd != 11 {
			t.Errorf("episode %d: want range 1-11, got %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// A pack without a range in its name and without a readable detail page: the download happens all
// the same (Nyaa may be down, and refusing the pack would zero everyone's packs), but the recorded
// range stays unknown instead of 1..total.
func TestProcessAnimeEpisodes_UnresolvedRangeLeavesTheRecordWithoutARange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	anime.Progress = 5
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime (Season 1+OVA) [Batch]", MagnetLink: fakeMagnet(2)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("want the 6 pending episodes recorded, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.BatchStart != 0 || ep.BatchEnd != 0 {
			t.Errorf("episode %d: want an unknown range, got %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// THE BUG, end to end: the pack declares no range in its name and carries only episodes 1..6 of an
// 11-episode entry. Before, "unknown range" counted as "covers everything" and all 11 became
// records pointing to the pack's hash — 5 episodes marked as downloaded with no file at all, and
// nothing reconciles records with the disk afterwards. With the file list, only the 6 that exist
// become records; the rest comes back on the next pass.
func TestProcessAnimeEpisodes_DetailPageRangeStopsPhantomEpisodes(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime [Batch]", MagnetLink: fakeMagnet(3), DetailURL: "/view/1"}}, nil, nil, nil)
	searcher.packRange = func(nyaa.TorrentResult, int) (nyaa.BatchInfo, bool) {
		return nyaa.BatchInfo{StartEpisode: 1, EndEpisode: 6}, true
	}

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("want records only for the 6 episodes the pack has, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.EpisodeNumber > 6 {
			t.Errorf("episode %d is not in the pack and must not become a record", ep.EpisodeNumber)
		}
		if ep.BatchStart != 1 || ep.BatchEnd != 6 {
			t.Errorf("episode %d: want the range read from the files (1-6), got %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// The detail page is only fetched for a pack whose NAME gives no range, and only up to the budget.
func TestPackSet_FetchesDetailOnlyForRangelessPacksWithinBudget(t *testing.T) {
	fetches := 0
	detail := func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		fetches++
		return nyaa.BatchInfo{}, false
	}

	// The first pack already declares its range and covers the window: no extra request goes out.
	pickBatches(newPackSet([]nyaa.TorrentResult{pack("[X] Anime 01-12 [1080p]", 1), pack("[X] Anime [Batch]", 2)}, detail), packAxis{}, 1, 12)
	if fetches != 0 {
		t.Errorf("a pack with a range in its name must cost no request, got %d", fetches)
	}

	// No pack covers the window by its name's range: the rangeless ones are looked up, up to the
	// ceiling.
	rangeless := make([]nyaa.TorrentResult, 0, maxPackDetailFetches+2)
	for i := 0; i < maxPackDetailFetches+2; i++ {
		rangeless = append(rangeless, pack("[X] Anime [Batch]", i+10))
	}
	fetches = 0
	set := newPackSet(rangeless, detail)
	for i := range set.items {
		set.rangeOf(i)
	}
	if fetches != maxPackDetailFetches {
		t.Errorf("want at most %d detail requests, got %d", maxPackDetailFetches, fetches)
	}
}

// A detail page that doesn't answer isn't retried in the same pass: assignBatches scans the packs
// once per episode, and without memory that would become one request per episode.
func TestPackSet_RemembersAFailedDetailLookup(t *testing.T) {
	fetches := 0
	set := newPackSet([]nyaa.TorrentResult{pack("[X] Anime [Batch]", 1)}, func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		fetches++
		return nyaa.BatchInfo{}, false
	})

	for i := 0; i < 5; i++ {
		set.rangeOf(0)
	}
	if fetches != 1 {
		t.Errorf("want a single attempt, got %d", fetches)
	}
}

// Medalist (season 1, no prequel) once covered episodes 3-9 with a "Season 2" pack numbered 1-9.
// A first-season entry drops packs marked with a later season; a sequel keeps them.
func TestPartitionSearchResults_FirstSeasonEntryDropsLaterSeasonPacks(t *testing.T) {
	two, one := 2, 1
	s2 := pack("[Sylvar] Medalist Season 2 (BD Remux 1080p)", 1)
	s2.Season = &two
	s1 := pack("[EMBER] Medalist (2025) (Season 1) (Batch)", 2)
	s1.Season = &one
	romaji := "Medalist"
	anime := anilist.MediaList{Media: anilist.Media{Title: anilist.Title{Romaji: &romaji}}}

	packs, _, _ := partitionSearchResults(&files.Config{}, anime, []nyaa.TorrentResult{s2, s1})
	if len(packs) != 1 || packs[0].Name != s1.Name {
		t.Errorf("first-season entry: want only the season 1 pack, got %v", packs)
	}

	anime.Media.Relations = anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{{
		RelationType: "PREQUEL",
		Node:         anilist.MediaRelationNode{Format: anilist.MediaFormatTV},
	}}}
	if packs, _, _ := partitionSearchResults(&files.Config{}, anime, []nyaa.TorrentResult{s2, s1}); len(packs) != 2 {
		t.Errorf("sequel without a season marker: want both packs kept, got %d", len(packs))
	}
}

// An unlabeled pack is a pack only if its file list says so: unknown range after the detail page
// never counts as "covers everything" for it, unlike a labeled pack.
func TestPickBatches_UnlabeledPackNeedsTheFileRange(t *testing.T) {
	unlabeled := pack("[man] Azumanga Daioh [BD 1080p HEVC FLAC]", 1)
	unlabeled.UnlabeledPack = true
	noDetail := func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) { return nyaa.BatchInfo{}, false }
	fromFiles := func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		return nyaa.BatchInfo{StartEpisode: 1, EndEpisode: 26}, true
	}

	if got := pickBatches(newPackSet([]nyaa.TorrentResult{unlabeled}, noDetail), packAxis{}, 1, 12); got.len() != 0 {
		t.Errorf("unresolved unlabeled pack must not be picked, got %v", batchNames(got))
	}
	if got := pickBatches(newPackSet([]nyaa.TorrentResult{unlabeled}, fromFiles), packAxis{}, 1, 12); got.len() != 1 {
		t.Errorf("unlabeled pack whose files cover the window must be picked, got %v", batchNames(got))
	}
}
