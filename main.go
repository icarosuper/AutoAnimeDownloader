package main

import (
	"AutoAnimeDownloader/modules"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func main() {
	downloadAnime("New PANTY & STOCKING with GARTERBELT", 1)
	return

	a := app.New()
	w := a.NewWindow("Auto Anime Downloader")

	w.Resize(fyne.NewSize(500, 300))
	w.SetCloseIntercept(func() {
		w.Hide()
	})

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("MyApp",
			fyne.NewMenuItem("Abrir", func() {
				w.Show()
			}))
		desk.SetSystemTrayMenu(m)
	}

	setWindowContent(w)

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window) {
	textBox := widget.NewEntry()
	textBox.SetPlaceHolder("Enter anime name...")

	btn := widget.NewButton("Download Anime", func() {
		animeName := textBox.Text

		fmt.Println("Iniciando download para:", animeName)

		// if animeName != "" {
		// 	downloadAnime(animeName)
		// }
	})

	c := container.NewGridWithRows(2, textBox, btn)

	w.SetContent(c)
}

func downloadAnime(animeName string, episode int) {
	// Exemplo de uso do novo scraper do Nyaa
	fmt.Printf("Buscando torrents para: %s\n", animeName)

	// Buscar torrents (sem especificar episódio)
	results, err := modules.ScrapNyaa(animeName, "", episode)
	if err != nil {
		fmt.Printf("Erro ao buscar torrents: %v\n", err)
		return
	}

	// // Imprimir resultados
	modules.PrintTorrentResults(results)

	// Exemplo: buscar episódio específico
	// if len(results) > 0 && results[0].Episode != nil {
	// 	episode := *results[0].Episode
	// 	fmt.Printf("\nBuscando especificamente o episódio %d...\n", episode)

	// 	episodeResults, err := modules.ScrapNyaa(animeName, "", episode)
	// 	if err != nil {
	// 		fmt.Printf("Erro ao buscar episódio específico: %v\n", err)
	// 		return
	// 	}

	// 	modules.PrintTorrentResults(episodeResults)

	// 	// Se encontrou resultados, baixar o primeiro
	// 	if len(episodeResults) > 0 {
	// 		magnet := episodeResults[0].MagnetLink
	// 		if magnet != "" {
	// 			fmt.Printf("\nIniciando download do magnet: %s\n", magnet)
	// 			magnets := []string{magnet}
	// 			modules.DownloadAnimes(magnets)
	// 		}
	// 	}
	// }
}
