package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
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
	torrentsService *torrents.TorrentService,
	anime anilist.MediaList,
	dlTorrents []torrents.Torrent,
	savedEpisodes []files.EpisodeStruct,
	blockedMap map[int]bool,
	customQuery string,
	jobQueue *JobQueue,
) animeProcessResult {
	var result animeProcessResult
	animeTitle := getAnimeTitleSafe(anime)
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Processing anime episodes")

	torrentsMap := buildTorrentsMap(dlTorrents)
	savedEpisodesMap := buildSavedEpisodesMap(savedEpisodes)
	savedEpisodesFullMap := buildSavedEpisodesFullMap(savedEpisodes)

	downloadedEpisodesOfAnime := 0
	episodes := anime.Media.AiringSchedule.Nodes
	keepSet := buildWatchedKeepSet(configs.WatchedEpisodesToKeep, episodes, savedEpisodesMap, anime.Progress)
	var episodesToDownload []anilist.AiringNode

	for _, ep := range episodes {
		result.checkedEpisodes = append(result.checkedEpisodes, ep.ID)
		epName := fmt.Sprintf("%s - Episode %d", animeTitle, ep.Episode)

		isInTorrents := torrentsMap[epName]
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

	magnetsForEpisodes := resolveSearchStrategy(anime, animeTitle, episodesToDownload, customQuery)

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
			for _, tr := range searchNyaaForSingleEpisode(ep, anime.Media.Title, customQuery) {
				magnets = append(magnets, tr.MagnetLink)
			}
		}

		hash := attemptDownloadWithRetries(configs, torrentsService, magnets, anime, epName, skipSubfolder)

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
				DownloadDate:       time.Now(),
			})

			if configs.RenameFilesForJellyfin && !skipSubfolder {
				if jobQueue != nil {
					jobQueue.EnqueueRenameFile(hash, animeTitle, ep.Episode)
				} else {
					go func() { torrentsService.RenameEpisodeFile(hash, animeTitle, ep.Episode) }()
				}
			}
		}
	}

	enqueueOrMoveToCompletedFolder(torrentsService, anime, configs, savedEpisodes, jobQueue)
	return result
}

type resolvedMagnets struct {
	magnets       []string
	skipSubfolder bool
	overrideName  string
}

// resolveSearchStrategy picks the best Nyaa search strategy for the anime and returns
// magnets keyed by episode ID. Tries movie → batch → multi-episode in priority order.
func resolveSearchStrategy(anime anilist.MediaList, animeTitle string, episodesToDownload []anilist.AiringNode, customQuery string) map[int]resolvedMagnets {
	result := make(map[int]resolvedMagnets, len(episodesToDownload))
	animeIsFinished := anime.Media.Status == anilist.MediaStatusFinished
	animeIsMovie := isAnimeMovie(anime)

	// Strategy 1: Movies
	if animeIsMovie {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Detected movie - searching for movie torrent")

		movieResult := searchNyaaForMovie(anime.Media.Title, true, customQuery)

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

		requestedSeason := extractSeasonFromAnime(anime)
		batchResult := searchNyaaForBatch(anime.Media.Title, requestedSeason, customQuery)

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

	multipleResult := searchNyaaForMultipleEpisodes(anime.Media.Title, eps, customQuery)
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
	if animeIsInExcludedList(anime, configs.ExcludedList) {
		logger.Logger.Debug().
			Str("episode", epName).
			Str("excluded_list", configs.ExcludedList).
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

func attemptDownloadWithRetries(configs *files.Config, torrentsService *torrents.TorrentService, magnets []string, anime anilist.MediaList, fileName string, skipSubfolder bool) (hash string) {
	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))

	for i := range maxAttempts {
		logger.Logger.Debug().
			Str("episode", fileName).
			Int("attempt", i+1).
			Int("max_attempts", configs.EpisodeRetryLimit).
			Bool("skip_subfolder", skipSubfolder).
			Msg("Attempting to download episode")

		hash := torrentsService.DownloadTorrentWithOptions(magnets[i], getAnimeTitleSafe(anime), fileName, anime.Media.Status == anilist.MediaStatusFinished, skipSubfolder)
		if hash != "" {
			logger.Logger.Info().
				Str("episode", fileName).
				Str("hash", hash).
				Msg("Successfully added episode to qBittorrent")
			return hash
		}
	}

	logger.Logger.Warn().
		Str("episode", fileName).
		Int("attempts", maxAttempts).
		Msg("Failed to download episode after all attempts")
	return ""
}

// deleteEpisodesByStatus deletes episodes for animes in the delete-status list.
func deleteEpisodesByStatus(deleteResp *anilist.AniListResponse, fileManager FileManagerInterface, torrentsService *torrents.TorrentService, savedEpisodes []files.EpisodeStruct) {
	if deleteResp == nil {
		return
	}

	logger.Logger.Debug().
		Msg("Running status-based episode deletion")

	deleteAnimeIDs := make(map[int]bool, len(deleteResp.Data.Page.MediaList))
	for _, anime := range deleteResp.Data.Page.MediaList {
		deleteAnimeIDs[anime.Id] = true
	}

	logger.Logger.Debug().
		Int("anilist_entries", len(deleteResp.Data.Page.MediaList)).
		Int("saved_episodes", len(savedEpisodes)).
		Msg("Status-based deletion: fetched AniList entries")

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

	if err := fileManager.DeleteEpisodesFromFile(idsToDelete); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to delete episodes by status from file")
	}

	hashesToDelete := extractEpisodesHashes(savedEpisodes, idsToDelete)
	if len(hashesToDelete) > 0 {
		if err := torrentsService.DeleteTorrents(hashesToDelete); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to delete torrents by status")
		} else {
			logger.Logger.Info().Int("count", len(hashesToDelete)).Msg("Deleted torrents by status")
		}
	}
}

func handleSavedEpisodes(fileManager FileManagerInterface, configs *files.Config, torrentsService *torrents.TorrentService, data handleEpisodesData) {
	episodesNotInWatching := identifyEpisodesNotInWatching(data.savedEpisodes, data.checkedEpisodes)

	saveEpisodesToFile(fileManager, data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		if err := fileManager.DeleteEpisodesFromFile(data.idsToDelete); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to delete episodes from file")
		}

		allHashIds := append(data.idsToDelete, episodesNotInWatching...)
		hashesToDelete := extractEpisodesHashes(data.savedEpisodes, allHashIds)
		if len(hashesToDelete) > 0 {
			if err := torrentsService.DeleteTorrents(hashesToDelete); err != nil {
				logger.Logger.Warn().Err(err).Int("count", len(hashesToDelete)).Msg("Failed to delete torrents")
			} else {
				logger.Logger.Info().Int("count", len(hashesToDelete)).Msg("Deleted torrents")
			}
		}
	}
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

func extractEpisodesHashes(savedEpisodes []files.EpisodeStruct, episodeIDs []int) []string {
	hashMap := make(map[int]string)
	for _, savedEp := range savedEpisodes {
		hashMap[savedEp.EpisodeID] = savedEp.EpisodeHash
	}

	var hashes []string
	for _, id := range episodeIDs {
		if hash, exists := hashMap[id]; exists {
			hashes = append(hashes, hash)
		}
	}

	return hashes
}

func saveEpisodesToFile(fileManager FileManagerInterface, newEpisodes []files.EpisodeStruct) {
	if err := fileManager.SaveEpisodesToFile(newEpisodes); err != nil {
		logger.Logger.Warn().Err(err).Int("count", len(newEpisodes)).Msg("Failed to save episodes to file")
	} else if len(newEpisodes) > 0 {
		logger.Logger.Info().Int("count", len(newEpisodes)).Msg("Saved episodes to file")
	}
}

// enqueueOrMoveToCompletedFolder schedules a move-to-completed job when a job queue is
// available, otherwise executes the move synchronously (fallback for tests / nil queue).
func enqueueOrMoveToCompletedFolder(torrentsService *torrents.TorrentService, anime anilist.MediaList, configs *files.Config, savedEpisodes []files.EpisodeStruct, jobQueue *JobQueue) {
	completeAnimePathIsSet := configs.CompletedAnimePath != "" && configs.CompletedAnimePath != configs.SavePath
	animeIsFinished := anime.Media.Status == anilist.MediaStatusFinished

	if !animeIsFinished || !completeAnimePathIsSet {
		return
	}

	savedEpisodesMap := make(map[int]string)
	for _, ep := range savedEpisodes {
		savedEpisodesMap[ep.EpisodeID] = ep.EpisodeHash
	}

	var animeHashes []string
	for _, ep := range anime.Media.AiringSchedule.Nodes {
		if hash, exists := savedEpisodesMap[ep.ID]; exists {
			animeHashes = append(animeHashes, hash)
		}
	}

	if len(animeHashes) == 0 {
		return
	}

	animeName := getAnimeTitleSafe(anime)

	if jobQueue != nil {
		logger.Logger.Info().
			Str("anime", animeName).
			Int("torrents_count", len(animeHashes)).
			Msg("Scheduling move of completed anime to completed folder")
		jobQueue.EnqueueMoveToCompleted(animeHashes, animeName)
		return
	}

	logger.Logger.Info().
		Str("anime", animeName).
		Int("torrents_count", len(animeHashes)).
		Msg("Moving completed anime to completed folder")

	if err := torrentsService.SendAnimeToCompletedFolder(animeHashes, animeName); err != nil {
		logger.Logger.Error().Err(err).Stack().
			Str("anime", animeName).
			Int("torrents_count", len(animeHashes)).
			Msg("Failed to move torrents to completed folder")
	} else {
		logger.Logger.Info().
			Str("anime", animeName).
			Int("torrents_count", len(animeHashes)).
			Msg("Successfully moved torrents to completed folder")
	}
}
