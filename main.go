package main

import (
	"AutoAnimeDownloader/modules"
	"fmt"
	"time"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	go func() {
		for {
			loop()
			time.Sleep(10 * time.Minute)
			break
		}
	}()

	modules.CreateUi()
}

func loop() {
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
