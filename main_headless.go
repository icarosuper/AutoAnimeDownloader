//go:build headless
// +build headless

package main

import (
	"AutoAnimeDownloader/modules/files"
	"AutoAnimeDownloader/modules/program"
	"fmt"
	"log"
	"time"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	fileManager, err := files.NewDefaultFileManager()
	if err != nil {
		log.Fatalf("Failed to initialize files manager: %v", err)
	}

	configs, err := fileManager.LoadConfigs()
	if err != nil {
		log.Fatalf("Failed to load configs: %v", err)
	}

	interval := time.Duration(configs.CheckInterval) * time.Minute

	program.StartLoop(program.StartLoopPayload{
		FileManager:            fileManager,
		Interval:               interval,
		ShowError:              func(string, string) {},
		UpdateEpisodesListView: func() {},
		SetLoading:             func(bool) {},
	})
	select {}
}
