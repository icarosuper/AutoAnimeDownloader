package main

import (
	"AutoAnimeDownloader/modules"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func main() {
	configs := modules.LoadConfigs()
	fmt.Println(configs)

	a := app.New()
	w := a.NewWindow("Windows")

	w.Resize(fyne.NewSize(500, 300))
	w.SetCloseIntercept(func() {
		w.Hide()
	})

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("MyApp",
			fyne.NewMenuItem("Show", func() {
				w.Show()
			}))
		desk.SetSystemTrayMenu(m)
	}

	setWindowContent(w)

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window) {
	// label := widget.NewLabel("Anime Downloader")
	btn := widget.NewButton("Download Anime", downloadAnime)

	// c := container.NewWithoutLayout(label, btn)

	// label.Move(fyne.NewPos(180, 80))
	// btn.Move(fyne.NewPos(180, 120))

	w.SetContent(btn)
}

func downloadAnime() {
	magnet := "magnet:?xt=urn:btih:f07ad82935a47b09e40bc612a32fe294b2abb450&dn=%5BSubsPlease%5D%20Kimi%20to%20Idol%20Precure%20-%2032%20%28720p%29%20%5B4C6D126B%5D.mkv&tr=http%3A%2F%2Fnyaa.tracker.wf%3A7777%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fexodus.desync.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.torrent.eu.org%3A451%2Fannounce"

	magnets := []string{magnet}

	modules.DownloadAnimes(magnets)
}
