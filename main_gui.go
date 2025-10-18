//go:build !headless

package main

import (
	"AutoAnimeDownloader/modules/gui"
	"fmt"
)

func main() {
	fmt.Println("Starting Auto Anime Downloader...")

	gui.CreateGui()
}
