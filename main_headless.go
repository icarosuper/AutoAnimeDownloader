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

	modules.StartLoop(interval, func(string, string) {}, func() {}, func(bool) {})

	select {}
}
