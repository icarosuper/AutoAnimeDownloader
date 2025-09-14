package modules

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func CreateUi() {
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
