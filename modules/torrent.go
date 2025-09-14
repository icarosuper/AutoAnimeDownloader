package modules

import (
	"fmt"
	"os/exec"
)

func DownloadAnimes(magnets []string) {
	configs := LoadConfigs()

	for _, magnet := range magnets {
		fmt.Printf("Downloading: %s\n", magnet)
		downloadAnime(magnet, configs.SavePath, configs.SkipDialog)
	}
}

func downloadAnime(magnet string, savePath string, skipDialog bool) {
	skipDialogArg := fmt.Sprintf("--skip-dialog=%t", skipDialog)
	savePathArg := "--save-path=" + savePath

	exec.Command("qbittorrent", magnet, savePathArg, skipDialogArg).Start()
}
