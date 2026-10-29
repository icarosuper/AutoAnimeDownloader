package daemon

import (
	"errors"
	"fmt"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

// appendStandaloneAnimes adds the standalone animes to the pass's universe, and is called AFTER
// DedupeByMedia (see searchAnilist).
//
// It does two things, in this order:
//
//  1. Automatic removal: every standalone id already in the processed list leaves the file. The
//     real entry already won the dedupe, so what the pass downloads doesn't change — what changes is
//     that the file stops growing, the "Standalone" chip disappears from the screen and is_standalone
//     stops lying about where that anime came from.
//
//     Accepted consequence: if the user later removes the anime from the AniList list, the
//     standalone record was already consumed and the pass stops covering it — with
//     delete_watched_episodes on, its episodes fall into identifyEpisodesNotInWatching and leave the
//     disk, the same path as any anime removed from the list. The alternative (telling an "active
//     standalone" from a "dormant standalone") would cost new state in the file, in AnimeInfo and on
//     the screen to cover a path standalone users don't take. Whoever wants the anime back adds it
//     again.
//
//  2. Append: each standalone still missing comes in as GetMediaByIDs' synthetic MediaList, all
//     fetched in a single query.
//     There's no media status filter here on purpose — whoever asked for the anime by hand wants it
//     tracked while it's NOT_YET_RELEASED too, which is half the point of the feature.
func appendStandaloneAnimes(fileManager FileManagerInterface, merged []anilist.MediaList, standaloneIDs []int) []anilist.MediaList {
	if len(standaloneIDs) == 0 {
		return merged
	}

	inList := make(map[int]bool, len(merged))
	for _, ml := range merged {
		inList[ml.Media.Id] = true
	}

	pending := make([]int, 0, len(standaloneIDs))
	for _, mediaID := range standaloneIDs {
		if !inList[mediaID] {
			pending = append(pending, mediaID)
			continue
		}
		if err := fileManager.RemoveStandaloneAnime(mediaID); err != nil {
			logger.Logger.Warn().Err(err).Int("media_id", mediaID).
				Msg("Failed to drop the standalone record of an anime now tracked by a list")
			continue
		}
		logger.Logger.Info().Int("media_id", mediaID).
			Msg("Standalone anime is now in an AniList list; dropped its standalone record")
	}
	if len(pending) == 0 {
		return merged
	}

	medias, err := anilist.GetMediaByIDs(pending, anilist.PriorityCritical)
	switch {
	case errors.Is(err, anilist.ErrFromCache):
		logger.Logger.Warn().
			Msg("Serving standalone animes from the local cache: AniList is unreachable")
	case err != nil:
		logger.Logger.Warn().Err(err).
			Msg("Failed to fetch standalone animes from AniList; skipping the missing ones this pass")
	}

	for _, mediaID := range pending {
		ml, ok := medias[mediaID]
		if !ok {
			continue // already warned by the batch error
		}
		if ml == nil {
			logger.Logger.Warn().Int("media_id", mediaID).
				Msg("AniList does not know this standalone media id; skipping it this pass")
			continue
		}

		merged = append(merged, *withStandaloneProgress(fileManager, ml))
		inList[mediaID] = true
	}

	return merged
}

// withStandaloneProgress gives the standalone's synthetic MediaList the progress saved in
// AnimeSettings.
//
// It's the alternative to a parallel path: with the data in place, shouldSkipEpisode,
// firstEpisodeToConsider, buildWatchedKeepSet, the watched pruning and EpisodesWatched work on a
// standalone without any "if isStandalone". A read failure counts as progress 0 — which is the
// behavior from before this field.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}

// DownloadStandaloneAnime downloads a standalone anime's already aired episodes the moment it's
// added, without waiting for the next cycle. It returns how many episodes came in.
//
// It does exactly three things: ensures the torrent session, processes that anime's episodes and
// saves the new records.
//
// It MUST NOT call handleSavedEpisodes. That calls identifyEpisodesNotInWatching, which compares ALL
// saved episodes against the checkedEpisodes it receives and returns the uncovered ones for
// deletion: with a single anime's episodes in hand and delete_watched_episodes on, that would delete
// the rest of the whole library.
//
// It doesn't call deleteEpisodesByStatus or DeleteEmptyFolders either — adding an anime is no
// occasion to delete anything.
func DownloadStandaloneAnime(fm FileManagerInterface, backend torrents.TorrentBackend, configs *files.Config, mediaID int) (int, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return 0, err
	}

	anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
	// The local cache will do: the standalone download asked for by hand needs the title and the
	// episode range, and Nyaa didn't go down along with AniList.
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return 0, fmt.Errorf("failed to fetch anime %d from AniList: %w", mediaID, err)
	}
	if anime == nil {
		return 0, fmt.Errorf("anime %d does not exist on AniList", mediaID)
	}
	anime = withStandaloneProgress(fm, anime)

	savedEpisodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return 0, fmt.Errorf("failed to load saved episodes: %w", err)
	}

	blockedEpisodes, err := fm.LoadBlockedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load blocked episodes, continuing without block list")
		blockedEpisodes = nil
	}
	blockedMap := make(map[files.EpisodeKey]bool, len(blockedEpisodes))
	for _, k := range blockedEpisodes {
		blockedMap[k] = true
	}

	// Same ownership by coverage as the pass: adding a standalone whose season is already on disk
	// (downloaded under the previous cour) must not download anything again (coverage.go).
	seriesIndex := resolveSeriesIndex([]int{mediaID}, savedEpisodes)

	// A download asked for by hand searches every episode now: the pass's miss backoff would make
	// the button do nothing for up to a day.
	searcher := defaultNyaaSearcher()
	searcher.misses = nil
	torrentsNow := backend.List()
	result := processAnimeEpisodes(configs, backend, *anime, torrentsNow, savedEpisodes, seriesIndex, blockedMap, searcher, newDiskBudget(torrentsNow, savedEpisodes))

	saveEpisodesToFile(fm, result.newEpisodes)

	return len(result.newEpisodes), nil
}

// DownloadStandaloneAnimeWithMagnet adds a user-chosen whole-title magnet, then the standalone
// id, then episode records for the pack's coverage. The torrent goes in first so a refused add
// cannot leave a tracked id with nothing to download. An unknown pack range writes no episode
// records (decisions.md #84).
func DownloadStandaloneAnimeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, configs *files.Config, mediaID int, magnet, name string, nyaaID int, sizeBytes int64) (int, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return 0, err
	}
	hash, err := addAndPrioritize(backend, magnet, sizeBytes, configs)
	if err != nil || hash == "" {
		return 0, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	if err := fm.AddStandaloneAnime(mediaID); err != nil {
		return 0, fmt.Errorf("failed to add standalone anime: %w", err)
	}

	records, err := standaloneMagnetCoverage(mediaID, hash, name, nyaaID)
	if err != nil {
		rollbackStandaloneID(fm, mediaID)
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	if err := fm.UpsertEpisodes(records); err != nil {
		rollbackStandaloneID(fm, mediaID)
		return 0, fmt.Errorf("failed to save episodes: %w", err)
	}
	return len(records), nil
}

func rollbackStandaloneID(fm FileManagerInterface, mediaID int) {
	if err := fm.RemoveStandaloneAnime(mediaID); err != nil {
		logger.Logger.Warn().Err(err).Int("media_id", mediaID).
			Msg("Failed to roll back standalone id after magnet coverage failed")
	}
}

func standaloneMagnetCoverage(mediaID int, hash, name string, nyaaID int) ([]files.EpisodeStruct, error) {
	anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, fmt.Errorf("failed to fetch anime %d from AniList: %w", mediaID, err)
	}
	if anime == nil {
		return nil, fmt.Errorf("anime %d does not exist on AniList", mediaID)
	}

	title := getAnimeTitleSafe(*anime)
	now := time.Now()
	if isAnimeMovie(*anime) {
		return movieMagnetRecords(*anime, mediaID, title, hash, now), nil
	}

	info := nyaa.ExtractBatchInfo(name)
	if info.EndEpisode == 0 && nyaaID > 0 {
		if fileInfo, ok := nyaa.PackFileRange(nyaa.ViewURL(nyaaID), mediaTotalEpisodes(*anime)); ok {
			info = fileInfo
		}
	}
	if info.EndEpisode == 0 {
		return nil, nil
	}

	var records []files.EpisodeStruct
	for _, node := range anilist.EpisodeList(*anime, 1) {
		airedAndCovered := node.TimeUntilAiring <= 0 && node.Episode >= info.StartEpisode && node.Episode <= info.EndEpisode
		if !airedAndCovered {
			continue
		}
		records = append(records, magnetEpisodeRecord(mediaID, title, hash, node.Episode, now, true, info.StartEpisode, info.EndEpisode))
	}
	return records, nil
}

func movieMagnetRecords(anime anilist.MediaList, mediaID int, title, hash string, now time.Time) []files.EpisodeStruct {
	var nodes []anilist.AiringNode
	for _, node := range anilist.EpisodeList(anime, 1) {
		if node.TimeUntilAiring <= 0 {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		nodes = append(nodes, anilist.AiringNode{Episode: 1})
	}

	records := make([]files.EpisodeStruct, 0, len(nodes))
	for _, node := range nodes {
		records = append(records, magnetEpisodeRecord(mediaID, title, hash, node.Episode, now, false, 0, 0))
	}
	return records
}

func magnetEpisodeRecord(mediaID int, title, hash string, episode int, now time.Time, isBatch bool, batchStart, batchEnd int) files.EpisodeStruct {
	return files.EpisodeStruct{
		AnimeID:         mediaID,
		AnimeName:       title,
		EpisodeHash:     hash,
		EpisodeName:     fmt.Sprintf("%s - Episode %d", title, episode),
		EpisodeNumber:   episode,
		IsBatch:         isBatch,
		BatchStart:      batchStart,
		BatchEnd:        batchEnd,
		DownloadDate:    now,
		ManuallyManaged: true,
	}
}
