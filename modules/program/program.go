package program

import (
	"AutoAnimeDownloader/modules/anilist"
	"AutoAnimeDownloader/modules/files"
	"AutoAnimeDownloader/modules/nyaa"
	"AutoAnimeDownloader/modules/torrents"
	"context"
	"fmt"
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

func StartLoop(p StartLoopPayload) func(newInterval time.Duration) {
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())

	start := func(d time.Duration, c context.Context) {
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

	torrentsService := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, configs.QBittorrentUrl, configs.SavePath)

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

	torrentsMap := buildTorrentsMap(downloadedTorrents)
	savedEpisodesMap := buildSavedEpisodesMap(savedEpisodes)

	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []int
	var idsToDelete []int

	for _, anime := range anilistResponse.Data.Page.MediaList {
		processAnimeEpisodes(
			configs,
			torrentsService,
			anime,
			torrentsMap,
			savedEpisodesMap,
			&newEpisodes,
			&checkedEpisodes,
			&idsToDelete,
		)
	}

	handleSavedEpisodes(fileManager, configs, torrentsService, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     idsToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
	})

	updateEpisodesListView()

	time.Sleep(300 * time.Millisecond) // Pra que serve esse sleep? Não lembro por que coloquei isso

	if err := fileManager.DeleteEmptyFolders(configs.SavePath); err != nil {
		fmt.Printf("Warning: failed to delete empty folders: %v\n", err)
	}
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

func animeIsInExcludedList(anime anilist.MediaListEntry, excludedList string) bool {
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

func tryDownloadEpisode(configs *files.Config, torrentsService *torrents.TorrentService, ep anilist.AiringNode, titles anilist.Title, epName string) string {
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

	if nyaaResponse == nil {
		return ""
	}

	return attemptDownloadWithRetries(configs, torrentsService, nyaaResponse, titles.English, epName)
}

func attemptDownloadWithRetries(configs *files.Config, torrentsService *torrents.TorrentService, nyaaResponse []nyaa.TorrentResult, titleEnglish *string, epName string) string {
	maxAttempts := min(configs.EpisodeRetryLimit, len(nyaaResponse))

	for i := 0; i < maxAttempts; i++ {
		fmt.Printf("Attempting to download %s (attempt %d/%d)\n", epName, i+1, configs.EpisodeRetryLimit)
		hash := torrentsService.DownloadTorrent(nyaaResponse[i].MagnetLink, *titleEnglish, epName)
		if hash != "" {
			fmt.Printf("Successfully added %s to qBittorrent\n", epName)
			return hash
		}
	}

	fmt.Printf("Failed to download %s after %d attempts\n", epName, configs.EpisodeRetryLimit)
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func processAnimeEpisodes(
	configs *files.Config,
	torrentsService *torrents.TorrentService,
	anime anilist.MediaListEntry,
	torrentsMap map[string]bool,
	savedEpisodesMap map[int]bool,
	newEpisodes *[]files.EpisodeStruct,
	checkedEpisodes *[]int,
	idsToDelete *[]int,
) {
	title := anime.Media.Title.Romaji
	fmt.Printf("\n----------------------------------------\n\n")
	fmt.Printf("%s\n\n", *title)

	downloadedEpisodesOfAnime := 0
	episodes := anime.Media.AiringSchedule.Nodes

	for _, ep := range episodes {
		*checkedEpisodes = append(*checkedEpisodes, ep.ID)
		epName := fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode)

		isInTorrents := torrentsMap[epName]
		alreadySaved := savedEpisodesMap[ep.ID]

		shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, alreadySaved, &downloadedEpisodesOfAnime, isInTorrents)

		if shouldDownload {
			hash := tryDownloadEpisode(configs, torrentsService, ep, anime.Media.Title, epName)

			if hash != "" && !alreadySaved {
				*newEpisodes = append(*newEpisodes, files.EpisodeStruct{
					EpisodeID:   ep.ID,
					EpisodeHash: hash,
					EpisodeName: epName,
				})
			}
		} else if shouldDelete {
			*idsToDelete = append(*idsToDelete, ep.ID)
		}
	}
}

func checkEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaListEntry, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool) (bool, bool) {
	// TODO: Se der erro salvar na lista de episódios que falharam
	// TODO: Opção pra colocar episódios na blacklist pra não tentar baixar de novo

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

func shouldSkipEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaListEntry, epName string) bool {
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

// Retorna: (shouldDownload, shouldDelete)
func handleAlreadySavedEpisode(configs *files.Config, downloadedEpisodes *int, isInTorrents bool, epName string) (bool, bool) {
	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		fmt.Printf("Deleting %s (max episodes exceeded)\n", epName)
		return false, true
	}

	fmt.Printf("Skipping %s (already downloaded)\n", epName)
	*downloadedEpisodes++

	return !isInTorrents, false
}
