package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"context"
	"fmt"
	"sync"
	"time"
)

type StartLoopPayload struct {
	FileManager *files.FileManager
	Interval    time.Duration
	State       *State
}

type StartLoopFuncType func(StartLoopPayload) func(newInterval time.Duration)

func createStartFunc(p StartLoopPayload) func(d time.Duration, c context.Context) {
	return func(d time.Duration, c context.Context) {
		go func() {
			for {
				// verifica cancelamento antes de executar
				select {
				case <-c.Done():
					logger.Logger.Info().Msg("Verification loop stopped")
					return
				default:
				}

				p.State.SetStatus(StatusChecking)
				animeVerification(c, p.FileManager, p.State)
				p.State.SetStatus(StatusRunning)

				// aguarda duração ou cancelamento
				select {
				case <-time.After(d):
					continue
				case <-c.Done():
					logger.Logger.Info().Msg("Verification loop stopped")
					return
				}
			}
		}()
	}
}

func StartLoop(p StartLoopPayload) func(newInterval time.Duration) {
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())

	start := createStartFunc(p)

	start(p.Interval, ctx)

	return func(newDur time.Duration) {
		mu.Lock()
		// para o loop atual
		cancel()
		// cria novo contexto/cancel para o próximo loop
		ctx, cancel = context.WithCancel(context.Background())
		start(newDur, ctx)
		mu.Unlock()
	}
}

func animeVerification(ctx context.Context, fileManager *files.FileManager, state *State) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to load configs")
		state.SetLastCheckError(err)
		return
	}

	torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, configs.QBittorrentUrl, configs.SavePath, configs.CompletedAnimePath)

	downloadedTorrents, err := fetchDownloadedTorrents(torrentsService)
	if err != nil {
		state.SetLastCheckError(err)
		return
	}

	anilistResponse, err := searchAnilist(configs)
	if err != nil {
		state.SetLastCheckError(err)
		return
	}

	savedEpisodes, err := fileManager.LoadSavedEpisodes()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to load saved episodes")
		state.SetLastCheckError(err)
		return
	}

	animes := anilistResponse.Data.Page.MediaList
	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []int
	var idsToDelete []int

	start := time.Now()
	for _, anime := range animes {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			logger.Logger.Info().Msg("Verification cancelled")
			state.SetLastCheckError(nil) // Avoids stale error reports
			return
		default:
		}

		processAnimeEpisodes(
			configs,
			torrentsService,
			anime,
			downloadedTorrents,
			savedEpisodes,
			&newEpisodes,
			&checkedEpisodes,
			&idsToDelete,
		)
	}
	elapsed := time.Since(start)

	handleSavedEpisodes(fileManager, configs, torrentsService, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     idsToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
	})

	state.SetLastCheck(time.Now())
	state.SetLastCheckError(nil)

	if err := fileManager.DeleteEmptyFolders(configs.SavePath, configs.CompletedAnimePath); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to delete empty folders")
	}

	avgTime := time.Duration(0)
	if len(checkedEpisodes) > 0 {
		avgTime = elapsed / time.Duration(len(checkedEpisodes))
	}

	logger.Logger.Info().
		Int("animes_checked", len(anilistResponse.Data.Page.MediaList)).
		Int("episodes_checked", len(checkedEpisodes)).
		Int("episodes_downloaded", len(newEpisodes)).
		Dur("total_time", elapsed).
		Dur("avg_time_per_episode", avgTime).
		Msg("Verification completed")
}

type handleEpisodesData struct {
	savedEpisodes   []files.EpisodeStruct
	idsToDelete     []int
	checkedEpisodes []int
	newEpisodes     []files.EpisodeStruct
}

func buildTorrentsMap(torrents []torrents.Torrent) map[string]bool {
	torrentsMap := make(map[string]bool, len(torrents))
	for _, torrent := range torrents {
		torrentsMap[torrent.Name] = true
	}
	return torrentsMap
}

func buildSavedEpisodesMap(episodes []files.EpisodeStruct) map[int]bool {
	episodesMap := make(map[int]bool, len(episodes))
	for _, episode := range episodes {
		episodesMap[episode.EpisodeID] = true
	}
	return episodesMap
}

func animeIsInExcludedList(anime anilist.MediaList, excludedList string) bool {
	for listName, isInList := range anime.CustomLists {
		if listName == excludedList && isInList {
			return true
		}
	}
	return false
}

func handleSavedEpisodes(fileManager *files.FileManager, configs *files.Config, torrentsService *torrents.TorrentService, data handleEpisodesData) {
	episodesNotInWatching := identifyEpisodesNotInWatching(data.savedEpisodes, data.checkedEpisodes)
	episodeIdsToDelete := append(data.idsToDelete, episodesNotInWatching...)

	saveEpisodesToFile(fileManager, data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		if err := fileManager.DeleteEpisodesFromFile(episodeIdsToDelete); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to delete episodes from file")
		}

		hashesToDelete := extractEpisodesHashes(data.savedEpisodes, episodeIdsToDelete)
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
		if !checkedMap[savedEp.EpisodeID] {
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

func saveEpisodesToFile(fileManager *files.FileManager, newEpisodes []files.EpisodeStruct) {
	if err := fileManager.SaveEpisodesToFile(newEpisodes); err != nil {
		logger.Logger.Warn().Err(err).Int("count", len(newEpisodes)).Msg("Failed to save episodes to file")
	} else if len(newEpisodes) > 0 {
		logger.Logger.Info().Int("count", len(newEpisodes)).Msg("Saved episodes to file")
	}
}

func fetchDownloadedTorrents(torrentsService *torrents.TorrentService) ([]torrents.Torrent, error) {
	torrents, err := torrentsService.GetDownloadedTorrents()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to connect to qBittorrent")
		return nil, fmt.Errorf("failed to connect to qBittorrent: %w", err)
	}

	logger.Logger.Debug().Int("count", len(torrents)).Msg("Fetched downloaded torrents")
	return torrents, nil
}

func searchAnilist(configs *files.Config) (*anilist.AniListResponse, error) {
	if configs.AnilistUsername == "" || configs.SavePath == "" {
		err := fmt.Errorf("missing required configuration: Anilist username or save path")
		logger.Logger.Error().
			Err(err).
			Str("anilist_username", configs.AnilistUsername).
			Str("save_path", configs.SavePath).
			Msg("Missing required configuration: Anilist username or save path")
		return nil, err
	}

	anilistResponse, err := anilist.SearchAnimes(configs.AnilistUsername)
	if err != nil {
		logger.Logger.Error().Err(err).Stack().
			Str("username", configs.AnilistUsername).
			Msg("Failed to search animes on Anilist")
		return nil, fmt.Errorf("failed to search animes on Anilist: %w", err)
	}

	logger.Logger.Info().
		Str("username", configs.AnilistUsername).
		Int("animes_found", len(anilistResponse.Data.Page.MediaList)).
		Msg("Successfully fetched animes from Anilist")

	return anilistResponse, nil
}

func searchNyaaForSingleEpisode(ep anilist.AiringNode, titles anilist.Title) (result []nyaa.TorrentResult) {
	// Tenta buscar com Romaji primeiro, depois com English
	titleVariants := []string{*titles.Romaji}
	if *titles.Romaji != *titles.English {
		titleVariants = append(titleVariants, *titles.English)
	}

	var nyaaResponse []nyaa.TorrentResult
	var err error

	for _, titleVariant := range titleVariants {
		nyaaResponse, err = nyaa.ScrapNyaa(titleVariant, ep.Episode)
		if err != nil {
			logger.Logger.Debug().
				Err(err).
				Str("title", titleVariant).
				Int("episode", ep.Episode).
				Msg("Error searching Nyaa")
			continue
		}
		if nyaaResponse != nil {
			logger.Logger.Debug().
				Str("title", titleVariant).
				Int("episode", ep.Episode).
				Int("torrents_found", len(nyaaResponse)).
				Msg("Found torrents on Nyaa")
			break
		}
	}

	return nyaaResponse
}

func attemptDownloadWithRetries(configs *files.Config, torrentsService *torrents.TorrentService, magnets []string, anime anilist.MediaList, fileName string) (hash string) {
	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))

	for i := range maxAttempts {
		logger.Logger.Debug().
			Str("episode", fileName).
			Int("attempt", i+1).
			Int("max_attempts", configs.EpisodeRetryLimit).
			Msg("Attempting to download episode")

		hash := torrentsService.DownloadTorrent(magnets[i], *anime.Media.Title.English, fileName, anime.Media.Status == anilist.MediaStatusFinished)
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

func processAnimeEpisodes(
	configs *files.Config,
	torrentsService *torrents.TorrentService,
	anime anilist.MediaList,
	torrents []torrents.Torrent,
	savedEpisodes []files.EpisodeStruct,
	newEpisodes *[]files.EpisodeStruct,
	checkedEpisodes *[]int,
	idsToDelete *[]int,
) {
	title := anime.Media.Title.Romaji
	logger.Logger.Info().
		Str("anime", *title).
		Str("english_title", *anime.Media.Title.English).
		Msg("Processing anime episodes")

	torrentsMap := buildTorrentsMap(torrents)
	savedEpisodesMap := buildSavedEpisodesMap(savedEpisodes)

	downloadedEpisodesOfAnime := 0
	episodes := anime.Media.AiringSchedule.Nodes
	var episodesToDownload []anilist.AiringNode

	for _, ep := range episodes {
		*checkedEpisodes = append(*checkedEpisodes, ep.ID)
		epName := fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode)

		isInTorrents := torrentsMap[epName]
		alreadySaved := savedEpisodesMap[ep.ID]

		shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, alreadySaved, &downloadedEpisodesOfAnime, isInTorrents)

		if shouldDownload {
			episodesToDownload = append(episodesToDownload, ep)
		} else if shouldDelete {
			*idsToDelete = append(*idsToDelete, ep.ID)
		}
	}

	if len(episodesToDownload) == 0 {
		return
	}

	var multipleResult []nyaa.TorrentResult
	if len(episodesToDownload) > 1 {
		var eps []int
		for _, ep := range episodesToDownload {
			eps = append(eps, ep.Episode)
		}

		result, err := nyaa.ScrapNyaaForMultipleEpisodes(*anime.Media.Title.Romaji, eps)

		if err == nil {
			multipleResult = result
		}
	}

	for _, ep := range episodesToDownload {
		epName := fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode)
		var magnets []string

		if len(multipleResult) > 0 {
			for _, result := range multipleResult {
				if *result.Episode == ep.Episode {
					magnets = append(magnets, result.MagnetLink)
				}
			}
		}

		if len(magnets) == 0 {
			results := searchNyaaForSingleEpisode(ep, anime.Media.Title)
			for _, result := range results {
				magnets = append(magnets, result.MagnetLink)
			}
		}

		hash := attemptDownloadWithRetries(configs, torrentsService, magnets, anime, epName)

		if hash != "" {
			*newEpisodes = append(*newEpisodes, files.EpisodeStruct{
				EpisodeID:   ep.ID,
				EpisodeHash: hash,
				EpisodeName: epName,
			})
		}
	}

	ensureAnimeIsInCompletedFolder(torrentsService, anime, configs, savedEpisodes)
}

func checkEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool) (shouldDownload bool, shouldDelete bool) {
	epName := fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode)

	if shouldSkipEpisode(configs, ep, anime, epName) {
		return false, alreadySaved
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

func ensureAnimeIsInCompletedFolder(torrentsService *torrents.TorrentService, anime anilist.MediaList, configs *files.Config, savedEpisodes []files.EpisodeStruct) {
	completeAnimePathIsSet := configs.CompletedAnimePath != "" && configs.CompletedAnimePath != configs.SavePath
	animeIsFinished := anime.Media.Status == anilist.MediaStatusFinished

	if !animeIsFinished || !completeAnimePathIsSet {
		return
	}

	// Só vai chegar aqui pra animes completos com episódios novos
	// Também só chega aqui após baixar todos os episódios do anime
	// Não precisa preocupar com checagens redundantes

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

	animeName := *anime.Media.Title.English
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
