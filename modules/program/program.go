package program

import (
	"AutoAnimeDownloader/modules/anilist"
	"AutoAnimeDownloader/modules/files"
	"AutoAnimeDownloader/modules/nyaa"
	"AutoAnimeDownloader/modules/torrents"
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

type StartLoopPayload struct {
	FileManager            *files.FileManager
	Interval               time.Duration
	ShowError              func(string, string)
	UpdateEpisodesListView func()
	SetLoading             func(bool)
}

type StartLoopFuncType func(StartLoopPayload) func(newInterval time.Duration)

func createStartFunc(p StartLoopPayload) func(d time.Duration, c context.Context) {
	return func(d time.Duration, c context.Context) {
		go func() {
			for {
				// verifica cancelamento antes de executar
				select {
				case <-c.Done():
					return
				default:
				}

				p.SetLoading(true)
				animeVerification(p.FileManager, p.ShowError, p.UpdateEpisodesListView)
				p.SetLoading(false)

				// aguarda duração ou cancelamento
				select {
				case <-time.After(d):
					continue
				case <-c.Done():
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

func animeVerification(fileManager *files.FileManager, showError func(string, string), updateEpisodesListView func()) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		fmt.Printf("Failed to load configs: %v\n", err)
		showError("Erro de configuração", "Não foi possível carregar as configurações.")
		return
	}

	torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, configs.QBittorrentUrl, configs.SavePath, configs.CompletedAnimePath)

	downloadedTorrents := fetchDownloadedTorrents(torrentsService, showError)
	if downloadedTorrents == nil {
		return
	}

	anilistResponse := searchAnilist(configs, showError)
	if anilistResponse == nil {
		return
	}

	savedEpisodes, err := fileManager.LoadSavedEpisodes()
	if err != nil {
		fmt.Printf("Failed to load saved episodes: %v\n", err)
		showError("Erro", "Não foi possível carregar os episódios salvos.")
		return
	}

	animes := anilistResponse.Data.Page.MediaList
	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []int
	var idsToDelete []int

	start := time.Now()
	for _, anime := range animes {
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

	updateEpisodesListView()

	if err := fileManager.DeleteEmptyFolders(configs.SavePath); err != nil {
		fmt.Printf("Warning: failed to delete empty folders: %v\n", err)
	}

	fmt.Printf("Animes checados: %d\n", len(anilistResponse.Data.Page.MediaList))
	fmt.Printf("Episódios checados: %d\n", len(checkedEpisodes))
	fmt.Printf("Novos episódios: %d\n", len(newEpisodes))
	fmt.Printf("Tempo decorrido: %s\n", elapsed)
	fmt.Printf("Média de %s por episódio\n", elapsed/time.Duration(len(checkedEpisodes)))
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
			fmt.Printf("Warning: failed to delete episodes: %v\n", err)
		}

		hashesToDelete := extractEpisodesHashes(data.savedEpisodes, episodeIdsToDelete)
		torrentsService.DeleteTorrents(hashesToDelete)
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
		fmt.Printf("Warning: failed to save episodes: %v\n", err)
	}
}

func fetchDownloadedTorrents(torrentsService *torrents.TorrentService, showError func(string, string)) []torrents.Torrent {
	torrents, err := torrentsService.GetDownloadedTorrents()
	if err != nil {
		showError("Erro de conexão", "Houve um problema ao tentar conectar ao qBittorrent. Por favor, verifique a URL nas configurações.")
		return nil
	}

	return torrents
}

func searchAnilist(configs *files.Config, showError func(string, string)) *anilist.AniListResponse {
	if configs.AnilistUsername == "" || configs.SavePath == "" {
		fmt.Println("Nome de usuário ou caminho de salvamento faltando.")
		showError("Configuração necessária", "Por favor, configure seu nome de usuário do AniList e o caminho de salvamento nas configurações.")
		return nil
	}

	anilistResponse, err := anilist.SearchAnimes(configs.AnilistUsername)
	if err != nil {
		fmt.Printf("Erro ao buscar animes no AniList: %v\n", err)
		showError("Erro de conexão", "Houve um problema ao tentar conectar ao AniList. Por favor, verifique seu nome de usuário nas configurações.")
		return nil
	}

	return anilistResponse
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
			fmt.Printf("Error searching Nyaa with title '%s': %v\n", titleVariant, err)
			continue
		}
		if nyaaResponse != nil {
			break
		}

		fmt.Printf("Found %d torrents for %s episode %02d\n", len(nyaaResponse), titleVariant, ep.Episode)
	}

	return nyaaResponse
}

func attemptDownloadWithRetries(configs *files.Config, torrentsService *torrents.TorrentService, magnets []string, anime anilist.MediaList, fileName string) (hash string) {
	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))

	for i := range maxAttempts {
		fmt.Printf("Attempting to download %s (attempt %d/%d)\n", fileName, i+1, configs.EpisodeRetryLimit)
		hash := torrentsService.DownloadTorrent(magnets[i], *anime.Media.Title.English, fileName, anime.Media.Status == anilist.MediaStatusFinished)
		if hash != "" {
			fmt.Printf("Successfully added %s to qBittorrent\n", fileName)
			return hash
		}
	}

	fmt.Printf("Failed to download %s after %d attempts\n", fileName, configs.EpisodeRetryLimit)
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
	fmt.Printf("\n----------------------------------------\n\n")
	fmt.Printf("%s\n\n", *title)

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
		fmt.Printf("Skipping %s (max episodes per anime reached)\n", epName)
		return false, false
	}

	*downloadedEpisodes++
	return true, false
}

func shouldSkipEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaList, epName string) bool {
	if animeIsInExcludedList(anime, configs.ExcludedList) {
		fmt.Printf("Skipping %s (in excluded list)\n", epName)
		return true
	}

	if ep.Episode <= anime.Progress {
		fmt.Printf("Skipping %s (already watched)\n", epName)
		return true
	}

	if ep.TimeUntilAiring > 0 {
		fmt.Printf("Skipping %s (not yet aired)\n", epName)
		return true
	}

	return false
}

func handleAlreadySavedEpisode(configs *files.Config, downloadedEpisodes *int, isInTorrents bool, epName string) (shouldDownload bool, shouldDelete bool) {
	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		fmt.Printf("Deleting %s (max episodes exceeded)\n", epName)
		return false, true
	}

	*downloadedEpisodes++

	if isInTorrents {
		fmt.Printf("Skipping %s (already downloaded)\n", epName)
		return false, false
	}

	fmt.Printf("Redownloading %s (was missing from torrents)\n", epName)
	return true, false
}

func ensureAnimeIsInCompletedFolder(torrentsService *torrents.TorrentService, anime anilist.MediaList, configs *files.Config, savedEpisodes []files.EpisodeStruct) {
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

	animeName := *anime.Media.Title.Romaji
	completedPath := filepath.Join(configs.CompletedAnimePath, animeName)
	fmt.Printf("Moving completed anime: %s\n", animeName)

	// Move all torrents of this anime to the completed folder
	for _, ep := range anime.Media.AiringSchedule.Nodes {
		if err := torrentsService.SetTorrentSavePath(animeHashes, completedPath); err == nil {
			fmt.Printf("Moved torrent for %s - Episode %d to completed folder\n", animeName, ep.Episode)
		} else {
			fmt.Printf("Warning: failed to move torrent for episode %d of '%s': %v\n", ep.Episode, animeName, err)
		}
	}
}
