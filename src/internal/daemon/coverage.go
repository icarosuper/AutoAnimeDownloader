package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"time"
)

// OWNERSHIP BY COVERAGE
//
// The daemon's unit of ownership is the files.EpisodeKey{AnimeID, Episode} key, and a season pack
// spans several AnimeIDs: AniList splits cour and part into separate media ids, each with its own
// numbering from 1. The result without this: cour 2 has no record at all, doesn't know its file is
// already on disk inside the pack downloaded under cour 1, and downloads it again.
//
// The conversion between the two rulers is the series' absolute axis (anilist.Series, decisions.md
// #77): abs = Offset(AnimeID) + local number. Comparing 1..23 with 1..12 without converting is
// comparing different rulers.

// resolveSeriesIndex resolves the absolute axis of the pass's animes PLUS that of every anime_id
// that already has an episode on disk.
//
// The ones on disk are exactly the ones missing: the previous cour, already COMPLETED, left the
// pass's universe, and it's ITS offset that converts the pack's BatchStart/BatchEnd — recorded in
// that entry's local numbering — to the series axis. Without that seed, ownership by coverage
// wouldn't see the pack that caused the bug.
//
// An error aborts nothing: GetSeriesIndex returns what it managed to resolve, and an id missing
// from the map only means that anime doesn't take part in the adoption this pass.
func resolveSeriesIndex(animeIDs []int, savedEpisodes []files.EpisodeStruct) map[int]anilist.Series {
	seed := make([]int, 0, len(animeIDs)+len(savedEpisodes))
	seen := make(map[int]bool, len(animeIDs)+len(savedEpisodes))
	add := func(id int) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		seed = append(seed, id)
	}
	for _, id := range animeIDs {
		add(id)
	}
	for _, ep := range savedEpisodes {
		add(ep.AnimeID)
	}

	index, err := anilist.GetSeriesIndex(seed, anilist.PriorityCritical)
	if err != nil {
		logger.Logger.Warn().Err(err).
			Int("seed", len(seed)).
			Int("resolved", len(index)).
			Msg("Failed to resolve part of the series index; coverage ownership is partial this pass")
	}
	return index
}

// adoptCoveredEpisodes adopts the pending episodes whose content is ALREADY on disk: a pack
// downloaded under another entry of the same series whose declared range, on the absolute axis,
// contains the requested episode.
//
// Adopting is recording the record pointing to the hash that already exists — exactly what would
// happen if the search found the same torrent again, since Session.Add reuses the infohash. The
// gain is not going through Nyaa nor downloading again what's there; as a bonus the deletion guard
// starts seeing the siblings from the other cour (decisions.md #74) and organize hardlinks the
// files that never had an owner (organizeTorrent already handles a partially organized group,
// without notifying again).
//
// Movies stay out: prequelOf only follows TV/TV_SHORT, so a post-season movie inherits the season's
// offset and its "episode 1" would fall inside that season pack's range. Coverage by range can't
// tell the two apart.
func adoptCoveredEpisodes(
	anime anilist.MediaList,
	animeTitle string,
	totalEpisodes int,
	pending []anilist.AiringNode,
	savedEpisodes []files.EpisodeStruct,
	seriesIndex map[int]anilist.Series,
	torrentsHashSet map[string]bool,
) []files.EpisodeStruct {
	if len(pending) == 0 || isAnimeMovie(anime) {
		return nil
	}
	series, ok := seriesIndex[anime.Media.Id]
	if !ok {
		return nil
	}

	var adopted []files.EpisodeStruct
	for _, ep := range pending {
		owner, ownerOffset := findCoveringPack(savedEpisodes, seriesIndex, torrentsHashSet, series, series.Offset+ep.Episode)
		if owner == nil {
			continue
		}

		// The range is CONVERTED to the adopter's local numbering, never copied as the owner declares it.
		// Every record keeps the range on ITS entry's ruler (packAxis.localRange, decisions.md #79) and
		// that's how it's read back — findCoveringPack adds the offset of the record's own anime_id.
		// Copying cour 1's 1..23 under cour 2 would make that record read as the absolute 12..34: a cour
		// 3 would find coverage for an episode the pack doesn't have, and would end up with an owner and
		// no file.
		delta := ownerOffset - series.Offset
		batchStart, batchEnd := owner.BatchStart+delta, owner.BatchEnd+delta

		logger.Logger.Info().
			Str("anime", animeTitle).
			Int("episode", ep.Episode).
			Int("absolute_episode", series.Offset+ep.Episode).
			Int("owner_anime_id", owner.AnimeID).
			Str("hash", owner.EpisodeHash).
			Msg("Adopting episode already covered by a downloaded pack")

		adopted = append(adopted, files.EpisodeStruct{
			AnimeID:            anime.Media.Id,
			AnimeTotalEpisodes: totalEpisodes,
			AnimeName:          animeTitle,
			EpisodeHash:        owner.EpisodeHash,
			EpisodeName:        packDisplayName(animeTitle, batchStart, batchEnd),
			EpisodeNumber:      ep.Episode,
			IsBatch:            true,
			BatchStart:         batchStart,
			BatchEnd:           batchEnd,
			// Same torrent, same size: once the owner's records are pruned, these are the ones the
			// disk budget reads it from (newDiskBudget).
			TorrentSize:  owner.TorrentSize,
			DownloadDate: time.Now(),
		})
	}
	return adopted
}

// findCoveringPack returns the pack record, in ANY anime_id, whose declared range contains absEp on
// the absolute axis — and whose torrent is still in the session. Along with it comes the owner's
// offset, which is what allows rewriting the range on the adopter's ruler.
//
// Three gates, all mandatory: a declared range (a pack without a range in its name records BatchEnd
// == 0, and unknown isn't "covers everything" — same ceiling as canRemoveTorrent), a live torrent
// (adopting a hash that no longer exists would leave the episode with a record and no file,
// invisible to the re-download) and the SAME series (without the Key comparison, "absolute 12"
// would match the pack of any other anime).
func findCoveringPack(
	savedEpisodes []files.EpisodeStruct,
	seriesIndex map[int]anilist.Series,
	torrentsHashSet map[string]bool,
	series anilist.Series,
	absEp int,
) (*files.EpisodeStruct, int) {
	for i := range savedEpisodes {
		ep := &savedEpisodes[i]
		if !hasDeclaredRange(*ep) {
			continue
		}
		if !episodeInTorrents(ep.EpisodeHash, torrentsHashSet) {
			continue
		}
		owner, ok := seriesIndex[ep.AnimeID]
		if !ok || owner.Key != series.Key {
			continue
		}
		if owner.Offset+ep.BatchStart <= absEp && absEp <= owner.Offset+ep.BatchEnd {
			return ep, owner.Offset
		}
	}
	return nil, 0
}

// dropAdopted removes from the list the episodes the adoption already resolved.
//
// It runs at BOTH selection points: selectEpisodes is pure and doesn't know about the adoption, so
// the second run (limit lifted, pack path) would bring the adopted ones back and the daemon would
// download what it just adopted.
func dropAdopted(episodes []anilist.AiringNode, adopted []files.EpisodeStruct) []anilist.AiringNode {
	if len(adopted) == 0 {
		return episodes
	}
	taken := make(map[int]bool, len(adopted))
	for _, ep := range adopted {
		taken[ep.EpisodeNumber] = true
	}
	remaining := make([]anilist.AiringNode, 0, len(episodes))
	for _, ep := range episodes {
		if !taken[ep.Episode] {
			remaining = append(remaining, ep)
		}
	}
	return remaining
}
