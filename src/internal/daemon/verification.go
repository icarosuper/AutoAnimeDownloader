package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"context"
	"fmt"
	"sync"
	"time"
)

// animeProcessResult holds the per-anime outputs from processAnimeEpisodes.
type animeProcessResult struct {
	newEpisodes     []files.EpisodeStruct
	checkedEpisodes []int
	idsToDelete     []int
}

// maxConcurrentAnimes limits simultaneous Nyaa HTTP searches to avoid rate limiting.
const maxConcurrentAnimes = 5

func AnimeVerification(ctx context.Context, fileManager FileManagerInterface, state *State, jobQueue *JobQueue) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to load configs")
		state.SetLastCheckError(err)
		return
	}

	if !isConfigComplete(configs) {
		logger.Logger.Warn().Msg("Missing required configuration, opening browser to config page")
		go func() {
			time.Sleep(500 * time.Millisecond)
			webUIURL := getWebUiURL()
			if err := openBrowserToConfig(webUIURL); err != nil {
				logger.Logger.Warn().Err(err).Msg("Failed to open browser to configuration page")
			}
		}()

		state.SetLastCheckError(fmt.Errorf("missing required configuration for daemon (Anilist username, save path or qBittorrent URL)"))
		return
	}

	qBittorrentURL := getQBittorrentURL(configs.QBittorrentUrl)
	torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)

	// Phase 1: fetch all independent data sources in parallel.
	var (
		downloadedTorrents []torrents.Torrent
		anilistResponse    *anilist.AniListResponse
		savedEpisodes      []files.EpisodeStruct
		blockedEpisodes    []int
		animeSettingsMap   map[int]files.AnimeSettings
		deleteListResponse *anilist.AniListResponse

		errTorrents error
		errAnilist  error
		errEpisodes error
	)

	var fetchWg sync.WaitGroup

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		downloadedTorrents, errTorrents = fetchDownloadedTorrents(torrentsService)
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		anilistResponse, errAnilist = searchAnilist(configs)
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		var e error
		savedEpisodes, e = fileManager.LoadSavedEpisodes()
		if e != nil {
			logger.Logger.Error().Err(e).Stack().Msg("Failed to load saved episodes")
			errEpisodes = e
		}
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		var e error
		blockedEpisodes, e = fileManager.LoadBlockedEpisodes()
		if e != nil {
			logger.Logger.Warn().Err(e).Msg("Failed to load blocked episodes, continuing without block list")
			blockedEpisodes = []int{}
		}
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		var e error
		animeSettingsMap, e = fileManager.LoadAllAnimeSettings()
		if e != nil {
			logger.Logger.Warn().Err(e).Msg("Failed to load anime settings, using defaults")
			animeSettingsMap = map[int]files.AnimeSettings{}
		}
	}()

	if len(configs.DeleteStatuses) > 0 {
		fetchWg.Add(1)
		go func() {
			defer fetchWg.Done()
			var e error
			deleteListResponse, e = anilist.GetAllCurrentAnime(configs.AnilistUsername, configs.DeleteStatuses)
			if e != nil {
				logger.Logger.Warn().Err(e).Msg("Failed to fetch AniList animes for delete statuses")
			}
		}()
	}

	fetchWg.Wait()

	if errTorrents != nil {
		state.SetLastCheckError(errTorrents)
		return
	}
	if errAnilist != nil {
		state.SetLastCheckError(errAnilist)
		return
	}
	if errEpisodes != nil {
		state.SetLastCheckError(errEpisodes)
		return
	}

	blockedMap := make(map[int]bool, len(blockedEpisodes))
	for _, id := range blockedEpisodes {
		blockedMap[id] = true
	}

	animes := anilistResponse.Data.Page.MediaList

	var idsToDelete []int
	for _, anime := range animes {
		if isInDeleteStatuses(configs.DeleteStatuses, anime.Status) {
			for _, ep := range savedEpisodes {
				if ep.AnimeID == anime.Id && !ep.ManuallyManaged {
					idsToDelete = append(idsToDelete, ep.EpisodeID)
				}
			}
		}
	}

	// Phase 2: process each anime concurrently, bounded by maxConcurrentAnimes.
	sem := make(chan struct{}, maxConcurrentAnimes)
	resultCh := make(chan animeProcessResult, len(animes))

	var animeWg sync.WaitGroup
	start := time.Now()

outer:
	for _, anime := range animes {
		select {
		case <-ctx.Done():
			break outer
		default:
		}

		if isInDeleteStatuses(configs.DeleteStatuses, anime.Status) {
			continue
		}

		customQuery := ""
		if s, ok := animeSettingsMap[anime.Id]; ok {
			customQuery = s.CustomSearchQuery
		}

		animeWg.Add(1)
		go func(a anilist.MediaList, q string) {
			defer animeWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			select {
			case <-ctx.Done():
				return
			default:
			}

			resultCh <- processAnimeEpisodes(configs, torrentsService, a, downloadedTorrents, savedEpisodes, blockedMap, q, jobQueue)
		}(anime, customQuery)
	}

	animeWg.Wait()
	close(resultCh)
	elapsed := time.Since(start)

	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []int
	for r := range resultCh {
		newEpisodes = append(newEpisodes, r.newEpisodes...)
		checkedEpisodes = append(checkedEpisodes, r.checkedEpisodes...)
		idsToDelete = append(idsToDelete, r.idsToDelete...)
	}

	select {
	case <-ctx.Done():
		logger.Logger.Info().Msg("Verification cancelled")
		state.SetLastCheckError(nil)
		return
	default:
	}

	// Phase 3: sequential cleanup (file writes must not overlap).
	deleteEpisodesByStatus(deleteListResponse, fileManager, torrentsService, savedEpisodes)

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

func fetchDownloadedTorrents(torrentsService *torrents.TorrentService) ([]torrents.Torrent, error) {
	downloadedTorrents, err := torrentsService.GetDownloadedTorrents()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to connect to qBittorrent")
		return nil, fmt.Errorf("failed to connect to qBittorrent: %w", err)
	}

	logger.Logger.Debug().Int("count", len(downloadedTorrents)).Msg("Fetched downloaded torrents")
	return downloadedTorrents, nil
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

	anilistResponse, err := anilist.GetAllCurrentAnime(configs.AnilistUsername, configs.DownloadStatuses)
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
