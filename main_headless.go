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

	manager, err := files.NewDefaultFileManager()
	if err != nil {
		log.Fatalf("Failed to initialize files manager: %v", err)
	}

	configs, err := manager.LoadConfigs()
	if err != nil {
		log.Fatalf("Failed to load configs: %v", err)
	}

	interval := time.Duration(configs.CheckInterval) * time.Minute

	program.StartLoop(program.StartLoopPayload{
		Manager:                      manager,
		Interval:                     interval,
		ShowError:                    func(string, string) {},
		UpdateDownloadedEpisodesList: func() {},
		SetLoading:                   func(bool) {},
	})
	select {}
}
