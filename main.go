package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"AutoAnimeDownloader/modules"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	// interval := time.Duration(modules.LoadConfigs().CheckInterval) * time.Minute
	interval := 5 * time.Second

	restart := startLoop(interval)
	_ = restart // keep restart available for use (prevents unused variable compile error)

	modules.CreateUi(restart)
}

func loopNow(restart func(newDur time.Duration)) {
	loop()

	dur := time.Duration(modules.LoadConfigs().CheckInterval) * time.Minute

	restart(dur)
}

func loop() {
	fmt.Println("Checking for new episodes...")
	// executeLogic()
}

func startLoop(interval time.Duration) func(newInterval time.Duration) {
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())

	start := func(d time.Duration, c context.Context) {
		fmt.Println("Starting loop with interval:", d)

		go func() {
			for {
				// verifica cancelamento antes de executar
				select {
				case <-c.Done():
					return
				default:
				}

				loop()
				// executeLogic()

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

func checkAnimes() {
	configs := modules.LoadConfigs()

	if configs.AnilistUsername == "" || configs.SavePath == "" {
		fmt.Println("Please set your AniList username and save path in the settings.")
		return
	}

	anilistResponse, err := modules.SearchAnimes(configs.AnilistUsername)
	if err != nil {
		fmt.Printf("Error searching AniList: %v\n", err)
		return
	}

	downloadedIDs := modules.LoadIdsFromFile()

	for _, anime := range anilistResponse.Data.Page.MediaList {
		progress := anime.Progress
		titles := anime.Media.Title
		episodes := anime.Media.AiringSchedule.Nodes

		for _, ep := range episodes {
			if ep.Episode <= progress {
				fmt.Printf("Skipping %s episode %d (already watched)\n", *titles.Romaji, ep.Episode)
				continue
			}

			if ep.TimeUntilAiring > 0 {
				fmt.Printf("Skipping %s episode %d (not aired yet)\n", *titles.Romaji, ep.Episode)
				continue
			}

			if idIsInList(ep.ID, downloadedIDs) {
				fmt.Printf("Skipping %s episode %d (already downloaded)\n", *titles.Romaji, ep.Episode)
				continue
			}

			nyaaResponse, err := modules.ScrapNyaa(*titles.Romaji, ep.Episode)
			if err != nil {
				fmt.Printf("Error searching Nyaa: %v\n", err)
				continue
			}

			if nyaaResponse == nil {
				fmt.Printf("No magnet link found for %s episode %d\n", *titles.Romaji, ep.Episode)
				continue
			}

			fmt.Printf("Downloading %s episode %d\n", *titles.Romaji, ep.Episode)
			modules.DownloadAnime(nyaaResponse.MagnetLink, configs.SavePath, *titles.English, configs.SkipDialog)

			modules.SaveIdToFile(ep.ID)
			fmt.Printf("Downloaded %s episode %d\n", *titles.Romaji, ep.Episode)
		}
	}
}

func idIsInList(id int, ids []string) bool {
	idStr := fmt.Sprintf("%d", id)
	for _, existingID := range ids {
		if existingID == idStr {
			return true
		}
	}
	return false
}
