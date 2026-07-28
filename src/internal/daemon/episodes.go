package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"fmt"
	"time"
)

type handleEpisodesData struct {
	savedEpisodes   []files.EpisodeStruct
	idsToDelete     []int
	checkedEpisodes []int
	newEpisodes     []files.EpisodeStruct
}

func processAnimeEpisodes(
	configs *files.Config,
	backend torrents.TorrentBackend,
	anime anilist.MediaList,
	dlTorrents []torrents.TorrentInfo,
	savedEpisodes []files.EpisodeStruct,
	blockedMap map[int]bool,
	customQuery string,
	searcher nyaaSearcher,
) animeProcessResult {
	var result animeProcessResult
	animeTitle := getAnimeTitleSafe(anime)
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Processing anime episodes")

	torrentsHashSet := buildTorrentsHashSet(dlTorrents)
	savedEpisodesMap := buildSavedEpisodesMap(savedEpisodes)
	savedEpisodesFullMap := buildSavedEpisodesFullMap(savedEpisodes)

	downloadedEpisodesOfAnime := 0
	episodes := anime.Media.AiringSchedule.Nodes
	keepSet := buildWatchedKeepSet(configs.WatchedEpisodesToKeep, episodes, savedEpisodesMap, anime.Progress)
	var episodesToDownload []anilist.AiringNode

	for _, ep := range episodes {
		result.checkedEpisodes = append(result.checkedEpisodes, ep.ID)

		isInTorrents := episodeInTorrents(savedEpisodesFullMap[ep.ID].EpisodeHash, torrentsHashSet)
		alreadySaved := savedEpisodesMap[ep.ID]

		shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, alreadySaved, &downloadedEpisodesOfAnime, isInTorrents, keepSet[ep.ID])

		if shouldDownload && !blockedMap[ep.ID] {
			episodesToDownload = append(episodesToDownload, ep)
		} else if shouldDelete {
			if savedEp, ok := savedEpisodesFullMap[ep.ID]; !ok || !savedEp.ManuallyManaged {
				result.idsToDelete = append(result.idsToDelete, ep.ID)
			}
		}
	}

	magnetsForEpisodes := resolveSearchStrategy(anime, animeTitle, episodesToDownload, customQuery, searcher)

	for _, ep := range episodesToDownload {
		epName := fmt.Sprintf("%s - Episode %d", animeTitle, ep.Episode)

		resolved := magnetsForEpisodes[ep.ID]
		magnets := resolved.magnets
		skipSubfolder := resolved.skipSubfolder
		if resolved.overrideName != "" {
			epName = resolved.overrideName
		}

		// Fallback: individual episode search
		if len(magnets) == 0 {
			for _, tr := range searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery) {
				magnets = append(magnets, tr.MagnetLink)
			}
		}

		notifications.Notify(configs, notifications.NewEpisode, animeTitle, ep.Episode, "")

		hash := attemptDownloadWithRetries(configs, backend, magnets, epName)

		if hash != "" {
			totalEpisodes := 0
			if anime.Media.Episodes != nil {
				totalEpisodes = *anime.Media.Episodes
			}
			result.newEpisodes = append(result.newEpisodes, files.EpisodeStruct{
				EpisodeID:          ep.ID,
				AnimeID:            anime.Id,
				AnimeTotalEpisodes: totalEpisodes,
				AnimeName:          animeTitle,
				EpisodeHash:        hash,
				EpisodeName:        epName,
				EpisodeNumber:      ep.Episode,
				IsBatch:            skipSubfolder,
				DownloadDate:       time.Now(),
			})
			// Completion is handled event-driven: the session's onComplete callback (and
			// the reconciliation pass as a safety net) enqueue JobOrganize, which hardlinks
			// the finished files into the library and fires the completion webhook.
		} else {
			reason := notifications.ReasonDownloadRejected
			if len(magnets) == 0 {
				reason = notifications.ReasonNotFound
			}
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, reason)
		}
	}

	return result
}

type resolvedMagnets struct {
	magnets       []string
	skipSubfolder bool
	overrideName  string
}

// resolveSearchStrategy picks the best Nyaa search strategy for the anime and returns
// magnets keyed by episode ID. Tries movie → batch → multi-episode in priority order.
func resolveSearchStrategy(anime anilist.MediaList, animeTitle string, episodesToDownload []anilist.AiringNode, customQuery string, searcher nyaaSearcher) map[int]resolvedMagnets {
	result := make(map[int]resolvedMagnets, len(episodesToDownload))
	animeIsFinished := anime.Media.Status == anilist.MediaStatusFinished
	animeIsMovie := isAnimeMovie(anime)

	// Strategy 1: Movies
	if animeIsMovie {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Detected movie - searching for movie torrent")

		movieResult := searcher.searchMovie(anime.Media.Title, true, customQuery)

		if len(episodesToDownload) == 0 && len(movieResult) > 0 {
			fakeEp := anilist.AiringNode{ID: 0, Episode: 1}
			episodesToDownload = append(episodesToDownload, fakeEp)
			logger.Logger.Info().
				Str("anime", animeTitle).
				Msg("Created fake episode for movie download")
		}

		if len(movieResult) > 0 {
			for _, ep := range episodesToDownload {
				result[ep.ID] = resolvedMagnets{
					magnets:       []string{movieResult[0].MagnetLink},
					skipSubfolder: true,
					overrideName:  animeTitle,
				}
			}
			logger.Logger.Info().
				Str("anime", animeTitle).
				Str("torrent", movieResult[0].Name).
				Msg("Using movie torrent")
			return result
		}
	}

	if len(episodesToDownload) == 0 {
		return result
	}

	// Strategy 2: Finished anime → batch
	if animeIsFinished && !animeIsMovie && len(episodesToDownload) > 1 {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Detected finished anime - searching for batch torrent")

		batchResult := searcher.searchBatch(anime.Media.Title, anime.Media.Synonyms, customQuery)

		if len(batchResult) > 0 {
			for _, ep := range episodesToDownload {
				result[ep.ID] = resolvedMagnets{
					magnets:       []string{batchResult[0].MagnetLink},
					skipSubfolder: true,
					overrideName:  animeTitle,
				}
			}
			logger.Logger.Info().
				Str("anime", animeTitle).
				Str("torrent", batchResult[0].Name).
				Msg("Using batch torrent for finished anime")
			return result
		}
	}

	// Strategy 3: Multiple episodes search
	var eps []int
	for _, ep := range episodesToDownload {
		eps = append(eps, ep.Episode)
	}

	multipleResult := searcher.searchMultiple(anime.Media.Title, anime.Media.Synonyms, eps, customQuery)
	if len(multipleResult) > 0 {
		byEpisode := make(map[int][]nyaa.TorrentResult)
		for _, tr := range multipleResult {
			if tr.Episode != nil {
				byEpisode[*tr.Episode] = append(byEpisode[*tr.Episode], tr)
			}
		}
		for _, ep := range episodesToDownload {
			if trs, ok := byEpisode[ep.Episode]; ok {
				var magnets []string
				for _, tr := range trs {
					magnets = append(magnets, tr.MagnetLink)
				}
				result[ep.ID] = resolvedMagnets{magnets: magnets}
			}
		}
	}

	return result
}

func buildWatchedKeepSet(n int, episodes []anilist.AiringNode, savedEpisodesMap map[int]bool, progress int) map[int]bool {
	if n <= 0 {
		return nil
	}
	var watchedSaved []int
	for _, ep := range episodes {
		if ep.Episode <= progress && savedEpisodesMap[ep.ID] {
			watchedSaved = append(watchedSaved, ep.ID)
		}
	}
	keepSet := make(map[int]bool)
	start := len(watchedSaved) - n
	if start < 0 {
		start = 0
	}
	for _, id := range watchedSaved[start:] {
		keepSet[id] = true
	}
	return keepSet
}

func checkEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched bool) (shouldDownload bool, shouldDelete bool) {
	epName := fmt.Sprintf("%s - Episode %d", getAnimeTitleSafe(anime), ep.Episode)

	if shouldSkipEpisode(configs, ep, anime, epName) {
		return false, alreadySaved && !keepWatched
	}

	if alreadySaved {
		return handleAlreadySavedEpisode(configs, downloadedEpisodes, isInTorrents, epName)
	}

	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", configs.MaxEpisodesPerAnime).
			Msg("Skipping episode: max episodes per anime reached")
		return false, false
	}

	*downloadedEpisodes++
	return true, false
}

func shouldSkipEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaList, epName string) bool {
	if animeIsInExcludedList(anime, configs.ExcludedLists) {
		logger.Logger.Debug().
			Str("episode", epName).
			Msg("Skipping episode: in excluded list")
		return true
	}

	if ep.Episode <= anime.Progress {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("episode_number", ep.Episode).
			Int("progress", anime.Progress).
			Msg("Skipping episode: already watched")
		return true
	}

	if ep.TimeUntilAiring > 0 {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("time_until_airing", ep.TimeUntilAiring).
			Msg("Skipping episode: not yet aired")
		return true
	}

	return false
}

func handleAlreadySavedEpisode(configs *files.Config, downloadedEpisodes *int, isInTorrents bool, epName string) (shouldDownload bool, shouldDelete bool) {
	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		logger.Logger.Info().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", configs.MaxEpisodesPerAnime).
			Msg("Deleting episode: max episodes exceeded")
		return false, true
	}

	*downloadedEpisodes++

	if isInTorrents {
		logger.Logger.Debug().
			Str("episode", epName).
			Msg("Skipping episode: already downloaded")
		return false, false
	}

	logger.Logger.Info().
		Str("episode", epName).
		Msg("Redownloading episode: was missing from torrents")
	return true, false
}

func attemptDownloadWithRetries(configs *files.Config, backend torrents.TorrentBackend, magnets []string, fileName string) (hash string) {
	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))

	for i := range maxAttempts {
		logger.Logger.Debug().
			Str("episode", fileName).
			Int("attempt", i+1).
			Int("max_attempts", configs.EpisodeRetryLimit).
			Msg("Attempting to download episode")

		h, err := backend.Add(magnets[i])
		if err != nil {
			logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Failed to add torrent to embedded client")
			continue
		}
		if h != "" {
			logger.Logger.Info().
				Str("episode", fileName).
				Str("hash", h).
				Msg("Successfully added episode to embedded torrent client")
			return h
		}
	}

	logger.Logger.Warn().
		Str("episode", fileName).
		Int("attempts", maxAttempts).
		Msg("Failed to download episode after all attempts")
	return ""
}

// RemoveEpisodesWithLinks removes the given episodes from the saved-episodes file and frees
// their disk space (library hardlink + seeding torrent, with the batch guard applied).
// Exposed for API handlers (manual delete / redownload / replace): it returns an error when the
// record could not actually leave the saved-episodes file, so the handler can answer 500 and
// abort instead of adding a new torrent that the stale record would shadow.
func RemoveEpisodesWithLinks(fm FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}
	return removeEpisodesAndLinks(fm, backend, librarian, ids, saved, false)
}

// RemoveTorrentOptions configures RemoveTorrentWithEpisodes.
type RemoveTorrentOptions struct {
	// KeepData, when true, keeps both the library hardlink and the seeding copy (see
	// removeEpisodesAndLinks — they share an inode, so keeping only one frees no space).
	KeepData bool
	// Block, when true, blocks every episode in the group before removing its records, so the
	// automatic loop does not re-download it on the next pass.
	Block bool
}

// RemoveTorrentWithEpisodes removes a torrent by hash and every saved episode sharing that hash,
// as a single unit: the deletion boundary here is the torrent, not the episode, so a batch's
// episodes always leave together. Exposed for the manual "delete torrent" API handler.
//
// An orphan torrent (no saved episode matches the hash — added by hand, or its record already
// gone) is removed directly via backend.Remove and that call's error is returned as-is; Block is
// meaningless there since there is no episode id to block.
func RemoveTorrentWithEpisodes(
	fm FileManagerInterface,
	backend torrents.TorrentBackend,
	librarian files.Librarian,
	hash string,
	opts RemoveTorrentOptions,
) error {
	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}

	var group []files.EpisodeStruct
	for _, ep := range saved {
		if ep.EpisodeHash == hash {
			group = append(group, ep)
		}
	}

	if len(group) == 0 {
		return backend.Remove(hash, opts.KeepData)
	}

	if opts.Block {
		for _, ep := range group {
			if err := fm.BlockEpisode(ep.EpisodeID); err != nil {
				logger.Logger.Warn().Err(err).Int("episode_id", ep.EpisodeID).Msg("Failed to block episode before torrent removal")
			}
		}
	}

	ids := make([]int, 0, len(group))
	for _, ep := range group {
		ids = append(ids, ep.EpisodeID)
	}

	return removeEpisodesAndLinks(fm, backend, librarian, ids, saved, opts.KeepData)
}

// deleteEpisodesByStatus deletes episodes for animes in the delete-status list.
func deleteEpisodesByStatus(deleteResp *anilist.AniListResponse, fileManager FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, savedEpisodes []files.EpisodeStruct) {
	if deleteResp == nil {
		return
	}

	logger.Logger.Debug().
		Msg("Running status-based episode deletion")

	deleteAnimeIDs := make(map[int]bool, len(deleteResp.Data.Page.MediaList))
	for _, anime := range deleteResp.Data.Page.MediaList {
		deleteAnimeIDs[anime.Id] = true
	}

	var idsToDelete []int
	for _, ep := range savedEpisodes {
		if deleteAnimeIDs[ep.AnimeID] && !ep.ManuallyManaged {
			idsToDelete = append(idsToDelete, ep.EpisodeID)
		}
	}

	if len(idsToDelete) == 0 {
		logger.Logger.Debug().Msg("Status-based deletion: no matching episodes found")
		return
	}

	logger.Logger.Info().
		Int("count", len(idsToDelete)).
		Msg("Deleting episodes for animes with delete statuses")

	// Best-effort: a failure here must not abort the verification pass.
	if err := removeEpisodesAndLinks(fileManager, backend, librarian, idsToDelete, savedEpisodes, false); err != nil {
		logger.Logger.Warn().Err(err).Msg("Status-based deletion: failed to delete episodes from file")
	}
}

func handleSavedEpisodes(fileManager FileManagerInterface, configs *files.Config, backend torrents.TorrentBackend, librarian files.Librarian, data handleEpisodesData) {
	episodesNotInWatching := identifyEpisodesNotInWatching(data.savedEpisodes, data.checkedEpisodes)

	saveEpisodesToFile(fileManager, data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		allIds := append(append([]int{}, data.idsToDelete...), episodesNotInWatching...)
		// Best-effort: a failure here must not abort the verification pass.
		if err := removeEpisodesAndLinks(fileManager, backend, librarian, allIds, data.savedEpisodes, false); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to delete episodes from file")
		}
	}
}

// removeEpisodesAndLinks deletes episodes and frees their disk space by removing BOTH links:
// the library hardlink and the seeding copy (via the torrent). A torrent is only removed
// when every one of its saved episodes is being deleted (batch guard) — a torrent with
// surviving siblings is kept, and for batches its library files are only removed when the
// whole torrent goes (raw filenames can't be safely mapped to a single episode).
//
// Freeing disk space is best-effort (a failed hardlink/torrent removal is logged and skipped),
// but a failure to drop the records from the saved-episodes file is returned: the caller decides
// whether that is fatal (API handlers) or merely logged (the automatic loop).
//
// keepData, when true, skips the library-hardlink removal loop entirely and is passed through to
// backend.Remove. Library files and the seeding copy are the same inode (hardlinks), so keeping
// one but not the other frees no disk space — keep_data is honestly binary: both stay or both go.
func removeEpisodesAndLinks(fm FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, idsToDelete []int, savedEpisodes []files.EpisodeStruct, keepData bool) error {
	if len(idsToDelete) == 0 {
		return nil
	}
	deleteSet := make(map[int]bool, len(idsToDelete))
	for _, id := range idsToDelete {
		deleteSet[id] = true
	}

	byHash := make(map[string][]files.EpisodeStruct)
	for _, ep := range savedEpisodes {
		if ep.EpisodeHash != "" {
			byHash[ep.EpisodeHash] = append(byHash[ep.EpisodeHash], ep)
		}
	}

	// Remove library hardlinks (skipped when keepData: keeping the library copy while the
	// torrent is removed anyway does not double as "keep everything" since it's the same inode).
	if !keepData {
		for _, ep := range savedEpisodes {
			if !deleteSet[ep.EpisodeID] {
				continue
			}
			removingTorrent := ep.EpisodeHash == "" || allEpisodesInDeleteSet(byHash[ep.EpisodeHash], deleteSet)
			if ep.IsBatch && !removingTorrent {
				// Keep batch library files while siblings survive (can't identify a single
				// episode's raw-named file safely). Freed only when the whole torrent goes.
				continue
			}
			for _, p := range ep.LibraryPaths {
				if err := librarian.RemoveFromLibrary(p); err != nil {
					logger.Logger.Warn().Err(err).Str("path", p).Msg("Failed to remove library hardlink")
				}
			}
		}
	}

	// Remove torrents (seeding copy) with no surviving siblings.
	removedHashes := make(map[string]bool)
	for _, ep := range savedEpisodes {
		if !deleteSet[ep.EpisodeID] || ep.EpisodeHash == "" || removedHashes[ep.EpisodeHash] {
			continue
		}
		if allEpisodesInDeleteSet(byHash[ep.EpisodeHash], deleteSet) {
			if err := backend.Remove(ep.EpisodeHash, keepData); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", ep.EpisodeHash).Msg("Failed to remove torrent")
			} else {
				logger.Logger.Info().Str("hash", ep.EpisodeHash).Msg("Removed torrent (seeding copy)")
			}
			removedHashes[ep.EpisodeHash] = true
		}
	}

	if err := fm.DeleteEpisodesFromFile(idsToDelete); err != nil {
		return fmt.Errorf("failed to delete episodes from file: %w", err)
	}

	return nil
}

// allEpisodesInDeleteSet reports whether every episode in the group is in the delete set
// (i.e. no sibling survives, so the shared torrent can be removed).
func allEpisodesInDeleteSet(group []files.EpisodeStruct, deleteSet map[int]bool) bool {
	if len(group) == 0 {
		return true
	}
	for _, ep := range group {
		if !deleteSet[ep.EpisodeID] {
			return false
		}
	}
	return true
}

func identifyEpisodesNotInWatching(savedEpisodes []files.EpisodeStruct, checkedEpisodes []int) []int {
	checkedMap := make(map[int]bool)
	for _, id := range checkedEpisodes {
		checkedMap[id] = true
	}

	var episodesToDelete []int
	for _, savedEp := range savedEpisodes {
		if !checkedMap[savedEp.EpisodeID] && !savedEp.ManuallyManaged {
			episodesToDelete = append(episodesToDelete, savedEp.EpisodeID)
		}
	}

	return episodesToDelete
}

// saveEpisodesToFile persists freshly downloaded episodes, merging over any existing record
// with the same EpisodeID. FileManager.SaveEpisodesToFile dedupes by EpisodeID and silently
// discards updates, which would leave a re-downloaded episode with its stale hash (breaking the
// JobOrganize join) and a stale EpisodeNumber (producing "Anime - E00.mkv" for records saved
// before EpisodeNumber existed). UpsertEpisodes alone would clobber ManuallyManaged, so the
// merge is done here: download metadata is refreshed, LibraryPaths is reset (the file on disk
// is a new one and must be organized again) and ManuallyManaged — a user flag — is preserved.
func saveEpisodesToFile(fileManager FileManagerInterface, newEpisodes []files.EpisodeStruct) {
	if len(newEpisodes) == 0 {
		return
	}

	existing, err := fileManager.LoadSavedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load saved episodes before merge, falling back to append-only save")
		if err := fileManager.SaveEpisodesToFile(newEpisodes); err != nil {
			logger.Logger.Warn().Err(err).Int("count", len(newEpisodes)).Msg("Failed to save episodes to file")
			return
		}
		logger.Logger.Info().Int("count", len(newEpisodes)).Msg("Saved episodes to file")
		return
	}

	existingByID := make(map[int]files.EpisodeStruct, len(existing))
	for _, ep := range existing {
		existingByID[ep.EpisodeID] = ep
	}

	merged := make([]files.EpisodeStruct, 0, len(newEpisodes))
	for _, ep := range newEpisodes {
		merged = append(merged, mergeSavedEpisode(existingByID[ep.EpisodeID], ep))
	}

	if err := fileManager.UpsertEpisodes(merged); err != nil {
		logger.Logger.Warn().Err(err).Int("count", len(merged)).Msg("Failed to save episodes to file")
		return
	}
	logger.Logger.Info().Int("count", len(merged)).Msg("Saved episodes to file")
}

// mergeSavedEpisode merges a freshly downloaded record over the existing saved one (zero value
// when the episode is new). Every download field comes from the new record; only LibraryPaths
// (reset — the old hardlinks point at the previous release) and ManuallyManaged (a user flag the
// automatic loop must never clear) get special treatment.
func mergeSavedEpisode(existing, updated files.EpisodeStruct) files.EpisodeStruct {
	merged := updated
	merged.LibraryPaths = nil
	merged.ManuallyManaged = updated.ManuallyManaged || existing.ManuallyManaged
	return merged
}
