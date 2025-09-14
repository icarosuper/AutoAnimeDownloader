package modules

import (
	"fmt"
	"os/exec"
)

func DownloadAnime(magnet string, savePath string, animeName string, skipDialog bool) {
	skipDialogArg := fmt.Sprintf("--skip-dialog=%t", skipDialog)
	savePathArg := "--save-path=" + savePath + "/" + animeName

	exec.Command("qbittorrent", magnet, savePathArg, skipDialogArg).Start()
}
