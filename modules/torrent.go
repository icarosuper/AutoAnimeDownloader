package modules

import (
	"fmt"
	"os/exec"
)

func DownloadAnime(magnet string, savePath string, skipDialog bool) {
	skipDialogArg := fmt.Sprintf("--skip-dialog=%t", skipDialog)
	savePathArg := ""

	if savePath != "" {
		savePathArg = "--save-path=" + savePath
	}

	exec.Command("qbittorrent", magnet, savePathArg, skipDialogArg).Start()
}
