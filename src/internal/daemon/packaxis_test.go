package daemon

import (
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
)

// entryOf builds the AniList entry as it arrives at the pass: the title (where the part comes
// from) and the PREQUEL relation with its episode count (where the "continues by season"
// hypothesis comes from).
func entryOf(id int, title string, prequelEpisodes int) anilist.MediaList {
	media := anilist.Media{Id: id, Title: anilist.Title{Romaji: &title}}
	if prequelEpisodes > 0 {
		media.Relations = anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{{
			RelationType: "PREQUEL",
			Node:         anilist.MediaRelationNode{Episodes: &prequelEpisodes, Format: anilist.MediaFormatTV},
		}}}
	}
	return anilist.MediaList{Id: id, Media: media}
}

// The FOUR pack-numbering conventions measured in sources.md ("Pack granularity and numbering"),
// each under the real entry it appears in. What is asserted is the range CONVERTED to the
// entry's local numbering: that is what goes into the record and what coverage ownership
// (decisions.md #78) reconverts to the absolute axis.
func TestPackAxis_TheFourNumberingConventions(t *testing.T) {
	cases := []struct {
		name       string
		anime      anilist.MediaList
		offset     int // absolute series axis (decisions.md #77)
		total      int // episodes of the ENTRY
		pack       string
		start, end int
		convention string
	}{
		{
			name:       "restarts per season",
			anime:      entryOf(146065, "Mushoku Tensei II: Isekai Ittara Honki Dasu", 12),
			offset:     23,
			total:      13,
			pack:       "[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu - 00 ~ 12 [BATCH]",
			start:      0,
			end:        12,
			convention: "the entry's own numbering, starting at zero",
		},
		{
			name:       "restarts per part",
			anime:      entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 13),
			offset:     36,
			total:      12,
			pack:       "[EMBER] Mushoku Tensei II (Season 2 | Part 2) 01 ~ 12 (Batch)",
			start:      1,
			end:        12,
			convention: "the entry's own numbering",
		},
		{
			name:       "continues by season",
			anime:      entryOf(127720, "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2", 11),
			offset:     11,
			total:      12,
			pack:       "[Judas] Mushoku Tensei - Isekai Ittara Honki Dasu - 01 ~ 23 [BD 1080p][Batch]",
			start:      -10,
			end:        12,
			convention: "whole-season pack: starts 10 episodes BEFORE this entry's ep 1",
		},
		{
			name:       "absolute by series",
			anime:      entryOf(131681, "Shingeki no Kyojin: The Final Season Part 2", 16),
			offset:     75,
			total:      12,
			pack:       "[SubsPlease] Shingeki no Kyojin - 76-87 (1080p) [Batch]",
			start:      1,
			end:        12,
			convention: "absolute franchise numbering",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := map[int]anilist.Series{tc.anime.Media.Id: {Key: 1, Offset: tc.offset}}
			axis := newPackAxis(tc.anime, index, tc.total)

			picked := pickBatches(newPackSet([]nyaa.TorrentResult{pack(tc.pack, 1)}, nil), axis, 1, tc.total)
			if picked.len() != 1 {
				t.Fatalf("%s: the pack should cover the pending window, got %d packs", tc.convention, picked.len())
			}

			_, info := picked.covering(axis, 1)
			if info.StartEpisode != tc.start || info.EndEpisode != tc.end {
				t.Fatalf("%s: want local range %d-%d, got %d-%d",
					tc.convention, tc.start, tc.end, info.StartEpisode, info.EndEpisode)
			}
		})
	}
}

// The count tiebreak: two packs cover episode 1 under the SAME 12-episode entry, and only the
// span tells them apart. Without it delta 0 would match both and the season pack would be
// recorded as if it started at episode 1 of this entry.
func TestPackAxis_SpanDecidesBetweenLocalAndSeasonPack(t *testing.T) {
	anime := entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 13)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 36}}, 12)

	local := nyaa.ExtractBatchInfo("[EMBER] Mushoku Tensei II (Season 2 | Part 2) 01 ~ 12 (Batch)")
	if got, ok := axis.localRange(local, 1); !ok || got.StartEpisode != 1 || got.EndEpisode != 12 {
		t.Fatalf("a 12-episode pack is local: want 1-12, got %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}

	season := nyaa.ExtractBatchInfo("[Diddy] Mushoku Tensei II - 01 ~ 25 [BD]")
	if got, ok := axis.localRange(season, 1); !ok || got.StartEpisode != -12 || got.EndEpisode != 12 {
		t.Fatalf("a 25-episode pack is the season: want -12-12, got %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}
}

// The range read from the file list goes through the SAME axis as the range read from the
// name: groups use the same four conventions in the files as in the title. This is the
// measured EMBER case "(Season 2 | Part 2) … (Batch)" — name with no range, files 13..24,
// Part 2 entry of 12 episodes. Without the conversion, 13..24 would cover no episode of this
// entry.
func TestPackSet_DetailRangeGoesThroughTheAxis(t *testing.T) {
	anime := entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 12)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 12}}, 12)

	set := newPackSet([]nyaa.TorrentResult{pack("[EMBER] Mushoku Tensei II (Season 2 | Part 2) (Batch)", 1)},
		func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
			return nyaa.BatchInfo{StartEpisode: 13, EndEpisode: 24}, true
		})

	got, info := set.covering(axis, 1)
	if got == nil {
		t.Fatal("the pack covers episode 1 of this entry on the season axis")
	}
	if info.StartEpisode != 1 || info.EndEpisode != 12 {
		t.Fatalf("want the converted range 1-12, got %d-%d", info.StartEpisode, info.EndEpisode)
	}
}

// When in doubt, delta 0: with no series information no alternative hypothesis exists, and the
// pack is read in the entry's numbering — the always-on behavior, which is what the zero-value
// delivers.
func TestPackAxis_ZeroValueKeepsRelativeNumbering(t *testing.T) {
	info := nyaa.ExtractBatchInfo("[X] Anime 01-12 [Batch]")
	got, ok := packAxis{}.localRange(info, 1)
	if !ok || got.StartEpisode != 1 || got.EndEpisode != 12 {
		t.Fatalf("want 1-12 in relative numbering, got %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}
}

// A pack that covers the episode under no hypothesis does not enter: the search falls through
// to a loose episode, not to a pack that does not have what is wanted.
func TestPackAxis_NoHypothesisCoversRejectsThePack(t *testing.T) {
	anime := entryOf(166873, "Anime Part 2", 13)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 36}}, 12)

	if picked := pickBatches(newPackSet([]nyaa.TorrentResult{pack("[X] Anime 200-212 [Batch]", 1)}, nil), axis, 1, 12); picked.len() != 0 {
		t.Fatalf("want no pack, got %v", batchNames(picked))
	}
}

// The recorded range is the converted one (it may start at zero or below), but the displayed
// name clamps at 1: "Anime -10-12" means nothing on the downloads screen.
func TestAssignBatches_RecordsConvertedRangeAndClampsTheDisplayName(t *testing.T) {
	anime := entryOf(127720, "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2", 11)
	axis := newPackAxis(anime, map[int]anilist.Series{127720: {Key: 1, Offset: 11}}, 12)

	episodes := []anilist.AiringNode{{Episode: 1}, {Episode: 2}}
	batch := pack("[Judas] Mushoku Tensei - 01 ~ 23 [BD 1080p][Batch]", 1)

	covered, magnets := assignBatches("Anime", axis, episodes, newPackSet([]nyaa.TorrentResult{batch}, nil))
	if len(covered) != 2 {
		t.Fatalf("want both episodes covered, got %d", len(covered))
	}
	got := magnets[1]
	if got.batchStart != -10 || got.batchEnd != 12 {
		t.Fatalf("want recorded range -10-12, got %d-%d", got.batchStart, got.batchEnd)
	}
	if got.overrideName != "Anime 1-12" {
		t.Fatalf("want displayed name \"Anime 1-12\", got %q", got.overrideName)
	}
}

// The converted range reaches the exclusion guard and coverage ownership: the "unknown"
// sentinel is the END, so a season pack recorded under a later cour (negative start) still
// declares the 23 content episodes it has — which is what stops pruning from deleting the
// cour that has not been downloaded yet (decisions.md #74).
func TestDeclaredSpan_ConvertedRangeStartingBeforeEpisodeOne(t *testing.T) {
	group := []files.EpisodeStruct{
		{AnimeID: 127720, EpisodeNumber: 1, BatchStart: -10, BatchEnd: 12},
		{AnimeID: 127720, EpisodeNumber: 2, BatchStart: -10, BatchEnd: 12},
	}

	if got := declaredSpan(group); got != 23 {
		t.Fatalf("want span 23 (the whole pack), got %d", got)
	}
	if !hasDeclaredRange(group[0]) {
		t.Fatal("a negative start is a declared range, not unknown")
	}
	if hasDeclaredRange(files.EpisodeStruct{BatchStart: 0, BatchEnd: 0}) {
		t.Fatal("a pack with no range in the name is still unknown")
	}
}
