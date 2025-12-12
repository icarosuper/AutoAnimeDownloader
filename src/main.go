package main

import (
	program "AutoAnimeDownloader/src/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"fmt"
	"log"
	"time"
)

func main() {
	// run daemon
	// run webui
	// prepare cli

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
