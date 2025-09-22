//go:build !headless

package main

import (
	"AutoAnimeDownloader/modules"
	"fmt"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	modules.CreateGui()
}
