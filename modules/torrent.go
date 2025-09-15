package modules

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func DownloadAnime(magnet string, animeName string, config Config) {
	qBittorrent := "qbittorrent"
	skipDialogArg := fmt.Sprintf("--skip-dialog=%t", config.SkipDialog)
	savePathArg := fmt.Sprintf("--save-path=%s%s%s", config.SavePath, string(os.PathSeparator), animeName)

	if runtime.GOOS == "windows" {
		qBittorrent = config.QBittorrentPath
	}

	exec.Command(qBittorrent, magnet, savePathArg, skipDialogArg).Start()
}
