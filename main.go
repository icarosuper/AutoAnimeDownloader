package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"AutoAnimeDownloader/modules"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	modules.CreateUi(startLoop)
}

func startLoop(interval time.Duration, w fyne.Window, updateDownloadedEpisodesList func(), isLoading binding.ExternalBool) func(newInterval time.Duration) {
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

				isLoading.Set(true)
				animeVerification(w, updateDownloadedEpisodesList)
				isLoading.Set(false)

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

	start(interval, ctx)

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

func animeVerification(w fyne.Window, updateDownloadedEpisodesList func()) {
	configs := modules.LoadConfigs()

	anilistResponse := searchAnilist(w, configs)
	if anilistResponse == nil {
		return
	}

	savedEpisodes := modules.LoadSavedEpisodes()
	var newEpisodes []modules.EpisodeStruct
	var checkedEpisodes []int
	var idsToDelete []int

	// TODO: Checar episódios que não estão mais no qBittorrent e remover do arquivo

	for _, anime := range anilistResponse.Data.Page.MediaList {
		title := anime.Media.Title.Romaji
		fmt.Println(title)

		downloadedEpisodesOfAnime := 0
		episodes := anime.Media.AiringSchedule.Nodes

		for _, ep := range episodes {
			checkedEpisodes = append(checkedEpisodes, ep.ID)
			shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, savedEpisodes, &downloadedEpisodesOfAnime)

			if shouldDownload {
				hash := tryDownloadEpisode(ep, anime.Media.Title, configs)

				if hash != "" {
					downloadedEpisodesOfAnime++
					newEpisodes = append(newEpisodes, modules.EpisodeStruct{
						EpisodeID:   ep.ID,
						EpisodeHash: hash,
						EpisodeName: fmt.Sprintf("%s - Episode %d", *anime.Media.Title.English, ep.Episode),
					})
				}
			} else if shouldDelete {
				idsToDelete = append(idsToDelete, ep.ID)
			}
		}
	}

	handleSavedEpisodes(configs, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     idsToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
	})

	updateDownloadedEpisodesList()
}

type handleEpisodesData struct {
	savedEpisodes   []modules.EpisodeStruct
	idsToDelete     []int
	checkedEpisodes []int
	newEpisodes     []modules.EpisodeStruct
}

func handleSavedEpisodes(configs modules.Config, data handleEpisodesData) {
	// TODO: Refatorar essa parte que ficou difícil de entender
	var hashesToDelete []string

	// Se anime não está mais no watching, é marcado pra remoção
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

	modules.SaveEpisodesToFile(data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		modules.DeleteEpisodesFromFile(data.idsToDelete)
		modules.DeleteTorrents(configs, hashesToDelete)
	}
}

func searchAnilist(w fyne.Window, configs modules.Config) *modules.AniListResponse {
	qBittorrentConnection := modules.TestQBittorrentConnection(configs)
	if !qBittorrentConnection {
		fmt.Println("Não foi possível conectar ao qBittorrent.")
		dialog.ShowInformation("Erro de conexão", "Não foi possível conectar ao qBittorrent. Por favor, verifique a URL nas configurações.", w)
		return nil
	}

	if configs.AnilistUsername == "" || configs.SavePath == "" {
		fmt.Println("Nome de usuário ou caminho de salvamento faltando.")
		dialog.ShowInformation("Configuração necessária", "Por favor, configure seu nome de usuário do AniList e o caminho de salvamento nas configurações.", w)
		return nil
	}

	anilistResponse, err := modules.SearchAnimes(configs.AnilistUsername)
	if err != nil {
		fmt.Printf("Erro ao buscar animes no AniList: %v\n", err)
		dialog.ShowInformation("Erro de conexão", "Erro ao buscar animes no AniList. Por favor, verifique seu nome de usuário nas configurações.", w)
		return nil
	}

	return anilistResponse
}

func tryDownloadEpisode(ep modules.AiringNode, titles modules.Title, configs modules.Config) string {
	nyaaResponse, err := modules.ScrapNyaa(*titles.Romaji, ep.Episode)
	if err != nil {
		fmt.Printf("Error searching Nyaa: %v\n", err)
		return ""
	}
	if nyaaResponse == nil {
		fmt.Printf("No magnet link found for %s episode %d\n", *titles.Romaji, ep.Episode)
		return ""
	}

	maxLoops := len(nyaaResponse)
	if configs.EpisodeRetryLimit < maxLoops {
		maxLoops = configs.EpisodeRetryLimit
	}

	var hash string
	for i := 0; i < maxLoops; i++ {
		fmt.Printf("Attempting to download %s episode %d (attempt %d/%d)\n", *titles.Romaji, ep.Episode, i+1, configs.EpisodeRetryLimit)
		hash = modules.DownloadAnime(configs, nyaaResponse[i].MagnetLink, *titles.English, ep.Episode)
		if hash != "" {
			break
		}
	}

	if hash == "" {
		fmt.Printf("Failed to download %s episode %d after %d attempts\n", *titles.Romaji, ep.Episode, configs.EpisodeRetryLimit)
		return ""
	}

	fmt.Printf("Successfully added %s episode %d to qBittorrent\n", *titles.Romaji, ep.Episode)
	return hash
}

func checkEpisode(configs modules.Config, ep modules.AiringNode, anime modules.MediaListEntry, savedEpisodes []modules.EpisodeStruct, downloadedEpisodes *int) (bool, bool) {
	// TODO: Se der erro salvar na lista de episódios que falharam
	// TODO: Opção pra colocar episódios na blacklist pra não tentar baixar de novo
	progress := anime.Progress
	titles := anime.Media.Title

	alreadySaved := idIsInStructList(ep.ID, savedEpisodes)

	if ep.Episode <= progress {
		fmt.Printf("Skipping %s episode %d (already watched)\n", *titles.Romaji, ep.Episode)
		return false, alreadySaved
	}

	if alreadySaved {
		fmt.Printf("Skipping %s episode %d (already downloaded)\n", *titles.Romaji, ep.Episode)

		if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
			return false, true
		}

		*downloadedEpisodes++
		return false, false
	}

	if ep.TimeUntilAiring > 0 {
		fmt.Printf("Skipping %s episode %d (not aired yet)\n", *titles.Romaji, ep.Episode)
		return false, false
	}

	if *downloadedEpisodes >= configs.MaxEpisodesPerAnime {
		fmt.Printf("Skipping %s episode %d (max episodes per anime reached)\n", *titles.Romaji, ep.Episode)
		return false, false
	}

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

func idIsInStructList(id int, episodes []modules.EpisodeStruct) bool {
	for _, episode := range episodes {
		if episode.EpisodeID == id {
			return true
		}
	}
	return false
}
