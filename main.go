package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"AutoAnimeDownloader/modules"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	modules.CreateUi(startLoop)
}

func startLoop(interval time.Duration, w fyne.Window) func(newInterval time.Duration) {
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

				checkAnime(w)

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

func checkAnime(w fyne.Window) {
	configs := modules.LoadConfigs()

	qBittorrentConnection := modules.TestQBittorrentConnection(configs)
	if !qBittorrentConnection {
		fmt.Errorf("não foi possível conectar ao qBittorrent")
		dialog.ShowInformation("Erro de conexão", "Não foi possível conectar ao qBittorrent. Por favor, verifique a URL nas configurações.", w)
		return
	}

	if configs.AnilistUsername == "" || configs.SavePath == "" {
		fmt.Errorf("por favor, configure seu nome de usuário do AniList e o caminho de salvamento nas configurações")
		dialog.ShowInformation("Configuração necessária", "Por favor, configure seu nome de usuário do AniList e o caminho de salvamento nas configurações.", w)
		return
	}

	anilistResponse, err := modules.SearchAnimes(configs.AnilistUsername)
	if err != nil {
		fmt.Errorf("erro ao buscar animes no AniList: %v", err)
		dialog.ShowInformation("Erro de conexão", "Erro ao buscar animes no AniList. Por favor, verifique seu nome de usuário nas configurações.", w)
		return
	}

	savedEpisodes := modules.LoadSavedEpisodes()

	var newEpisodes []modules.EpisodeStruct
	var idsToDelete []int
	var hashesToDelete []string

	for _, anime := range anilistResponse.Data.Page.MediaList {
		// TODO: Máximo de episódios por anime
		episodes := anime.Media.AiringSchedule.Nodes

		for _, ep := range episodes {
			hash, shouldDelete := checkEpisode(ep, anime, savedEpisodes, configs)

			if hash != "" {
				newEpisodes = append(newEpisodes, modules.EpisodeStruct{
					EpisodeID:   ep.ID,
					EpisodeHash: hash,
				})
			}

			if shouldDelete {
				idsToDelete = append(idsToDelete, ep.ID)
			}
		}
	}

	for _, epID := range idsToDelete {
		for _, savedEp := range savedEpisodes {
			if savedEp.EpisodeID == epID {
				hashesToDelete = append(hashesToDelete, savedEp.EpisodeHash)
				break
			}
		}
	}

	modules.SaveEpisodesToFile(newEpisodes)

	if configs.DeleteWatchedEpisodes {
		modules.DeleteEpisodesFromFile(idsToDelete)
		modules.DeleteTorrents(configs, hashesToDelete)
	}
}

func checkEpisode(ep modules.AiringNode, anime modules.MediaListEntry, savedEpisodes []modules.EpisodeStruct, configs modules.Config) (string, bool) {
	// TODO: Se der erro salvar na lista de episódios falhados
	// TODO: Tentar pegar erro do qbittorrent também
	progress := anime.Progress
	titles := anime.Media.Title

	alreadySaved := idIsInList(ep.ID, savedEpisodes)

	if ep.Episode <= progress {
		fmt.Printf("Skipping %s episode %d (already watched)\n", *titles.Romaji, ep.Episode)
		return "", alreadySaved
	}

	if alreadySaved {
		fmt.Printf("Skipping %s episode %d (already downloaded)\n", *titles.Romaji, ep.Episode)
		return "", false
	}

	if ep.TimeUntilAiring > 0 {
		fmt.Printf("Skipping %s episode %d (not aired yet)\n", *titles.Romaji, ep.Episode)
		return "", false
	}

	nyaaResponse, err := modules.ScrapNyaa(*titles.Romaji, ep.Episode)
	if err != nil {
		fmt.Printf("Error searching Nyaa: %v\n", err)
		return "", false
	}
	if nyaaResponse == nil {
		fmt.Printf("No magnet link found for %s episode %d\n", *titles.Romaji, ep.Episode)
		return "", false
	}

	fmt.Printf("Downloading %s episode %d\n", *titles.Romaji, ep.Episode)
	hash := modules.DownloadAnime(configs, nyaaResponse.MagnetLink, *titles.English, ep.Episode)

	return hash, false
}

func idIsInList(id int, episodes []modules.EpisodeStruct) bool {
	for _, episode := range episodes {
		if episode.EpisodeID == id {
			return true
		}
	}
	return false
}
