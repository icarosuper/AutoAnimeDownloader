//go:build headless
// +build headless

package main

import (
	"AutoAnimeDownloader/modules"
	"fmt"
	"time"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	configs := modules.LoadConfigs()
	interval := time.Duration(configs.CheckInterval) * time.Minute

	modules.StartLoop(modules.StartLoopPayload{
		Interval:                     interval,
		ShowDialog:                   func(string, string) {},
		UpdateDownloadedEpisodesList: func() {},
		SetLoading:                   func(bool) {},
	})

	select {}
}
