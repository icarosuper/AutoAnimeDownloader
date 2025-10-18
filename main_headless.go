//go:build headless
// +build headless

package main

import (
	"AutoAnimeDownloader/modules/files"
	"AutoAnimeDownloader/modules/program"
	"fmt"
	"time"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	configs := files.LoadConfigs()
	interval := time.Duration(configs.CheckInterval) * time.Minute

	program.StartLoop(program.StartLoopPayload{
		Interval:                     interval,
		ShowDialog:                   func(string, string) {},
		UpdateDownloadedEpisodesList: func() {},
		SetLoading:                   func(bool) {},
	})

	select {}
}
