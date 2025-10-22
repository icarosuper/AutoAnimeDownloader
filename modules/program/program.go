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
	FileManager        *files.FileManager
	Interval           time.Duration
	ShowError          func(string, string)
	UpdateEpisodesList func()
	SetLoading         func(bool)
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
				animeVerification(p.FileManager, p.ShowError, p.UpdateEpisodesList)
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

func animeVerification(fileManager *files.FileManager, showError func(string, string), updateEpisodesList func()) {
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

	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []int
	var idsToDelete []int

	for _, anime := range anilistResponse.Data.Page.MediaList {
		title := anime.Media.Title.Romaji

		fmt.Println("----------------------------------------")
		fmt.Println(*title)

		downloadedEpisodesOfAnime := 0
		episodes := anime.Media.AiringSchedule.Nodes

		for _, ep := range episodes {
			checkedEpisodes = append(checkedEpisodes, ep.ID)
			epName := fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode)

			isInTorrents := episodeIsInTorrents(epName, downloadedTorrents)
			alreadySaved := idIsInStructList(ep.ID, savedEpisodes)

			shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, alreadySaved, &downloadedEpisodesOfAnime, isInTorrents)

			if shouldDownload {
				hash := tryDownloadEpisode(configs, torrentsService, ep, anime.Media.Title, epName)

				if hash != "" && !alreadySaved {
					newEpisodes = append(newEpisodes, files.EpisodeStruct{
						EpisodeID:   ep.ID,
						EpisodeHash: hash,
						EpisodeName: epName,
					})
				}
			} else if shouldDelete {
				idsToDelete = append(idsToDelete, ep.ID)
			}
		}
	}

	handleSavedEpisodes(fileManager, configs, torrentsService, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     idsToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
	})

	updateEpisodesList()

	time.Sleep(300 * time.Millisecond)

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

func episodeIsInTorrents(epName string, torrents []torrents.Torrent) bool {
	for _, torrent := range torrents {
		if torrent.Name == epName {
			return true
		}
	}
	return false
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
	// TODO: Refatorar essa parte que ficou difícil de entender
	var hashesToDelete []string

	// Se anime salvo não está mais no watching, é marcado pra remoção
	for _, savedEp := range data.savedEpisodes {
		if !idIsInIntList(savedEp.EpisodeID, data.checkedEpisodes) {
			data.idsToDelete = append(data.idsToDelete, savedEp.EpisodeID)
		}
	}

	// Obtém os hashes dos episódios que serão removidos
	for _, epID := range data.idsToDelete {
		for _, savedEp := range data.savedEpisodes {
			if savedEp.EpisodeID == epID {
				hashesToDelete = append(hashesToDelete, savedEp.EpisodeHash)
				break
			}
		}
	}

	if err := fileManager.SaveEpisodesToFile(data.newEpisodes); err != nil {
		fmt.Printf("Warning: failed to save episodes: %v\n", err)
	}

	if configs.DeleteWatchedEpisodes {
		if err := fileManager.DeleteEpisodesFromFile(data.idsToDelete); err != nil {
			fmt.Printf("Warning: failed to delete episodes: %v\n", err)
		}
		torrentsService.DeleteTorrents(hashesToDelete)
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
	nyaaResponse, err := nyaa.ScrapNyaa(*titles.Romaji, ep.Episode)
	if err != nil {
		fmt.Printf("Error searching Nyaa: %v\n", err)
		return ""
	}
	if nyaaResponse == nil {
		fmt.Println("No torrents found for", *titles.Romaji, ep.Episode)

		nyaaResponse, err := nyaa.ScrapNyaa(*titles.English, ep.Episode)
		if err != nil {
			fmt.Printf("Error searching Nyaa: %v\n", err)
			return ""
		}
		if nyaaResponse == nil {
			fmt.Println("No torrents found for", *titles.English, ep.Episode)
			return ""
		}
	}

	maxLoops := len(nyaaResponse)
	if configs.EpisodeRetryLimit < maxLoops {
		maxLoops = configs.EpisodeRetryLimit
	}

	var hash string
	for i := 0; i < maxLoops; i++ {
		fmt.Printf("Attempting to download %s (attempt %d/%d)\n", epName, i+1, configs.EpisodeRetryLimit)
		hash = torrentsService.DownloadTorrent(nyaaResponse[i].MagnetLink, *titles.English, epName)
		if hash != "" {
			break
		}
	}

	if hash == "" {
		fmt.Printf("Failed to download %s after %d attempts\n", epName, configs.EpisodeRetryLimit)
		return ""
	}

	fmt.Printf("Successfully added %s to qBittorrent\n", epName)
	return hash
}

func checkEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaListEntry, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool) (bool, bool) {
	// TODO: Se der erro salvar na lista de episódios que falharam
	// TODO: Opção pra colocar episódios na blacklist pra não tentar baixar de novo
	progress := anime.Progress
	titles := anime.Media.Title
	epName := fmt.Sprintf("%s - Episode %d", *titles.English, ep.Episode)

	isInExcludedList := animeIsInExcludedList(anime, configs.ExcludedList)

	if isInExcludedList {
		fmt.Printf("Skipping %s (in excluded list)\n", epName)
		return false, alreadySaved
	}

	if ep.Episode <= progress {
		fmt.Printf("Skipping %s (already watched)\n", epName)
		return false, alreadySaved
	}

	if alreadySaved {
		if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
			fmt.Printf("Deleting %s (max episodes exceeded)\n", epName)
			return false, true
		}

		fmt.Printf("Skipping %s (already downloaded)\n", epName)

		*downloadedEpisodes++
		return !isInTorrents, false
	}

	if ep.TimeUntilAiring > 0 {
		fmt.Printf("Skipping %s (not aired yet)\n", epName)
		return false, false
	}

	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		fmt.Printf("Skipping %s (max episodes per anime reached)\n", epName)
		return false, false
	}

	*downloadedEpisodes++
	return true, false
}

func idIsInIntList(id int, episodes []int) bool {
	for _, episodeID := range episodes {
		if episodeID == id {
			return true
		}
	}
	return false
}

func idIsInStructList(id int, episodes []files.EpisodeStruct) bool {
	for _, episode := range episodes {
		if episode.EpisodeID == id {
			return true
		}
	}
	return false
}
