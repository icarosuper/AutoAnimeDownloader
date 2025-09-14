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
	// Uncomment the line below to test AniList functionality without GUI
	testAniListSearch()
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

// testAniListSearch demonstrates the AniList search functionality
func testAniListSearch() {
	fmt.Println("Testing AniList GraphQL Search...")

	response, err := modules.SearchAnimes("JediahSk")
	if err != nil {
		fmt.Printf("Error searching AniList: %v\n", err)
		return
	}

	modules.PrintAnimeResults(response)
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

	// Add AniList search button
	anilistBtn := widget.NewButton("Search AniList", func() {
		// Example: Search for user ID 1 with 10 results per page
		// You can change the userID to any valid AniList user ID
		userID := 1
		// perPage := 10

		fmt.Printf("Searching AniList for user ID: %d\n", userID)

		// response, err := modules.SearchAnimes(userID, perPage)
		// if err != nil {
		// 	fmt.Printf("Error searching AniList: %v\n", err)
		// 	return
		// }

		// modules.PrintAnimeResults(response)
	})

	c := container.NewGridWithRows(3, textBox, btn, anilistBtn)

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
