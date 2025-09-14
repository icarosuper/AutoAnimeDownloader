package modules

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func CreateUi() {
	a := app.New()
	w := a.NewWindow("Auto Anime Downloader")

	w.Resize(fyne.NewSize(800, 600))
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
	tabs := container.NewAppTabs()

	tabs.Append(container.NewTabItem("Notificações", notificationsBox()))
	tabs.Append(container.NewTabItem("Configurações", settingsBox(w)))

	tabs.SelectIndex(0)

	w.SetContent(tabs)
}

func notificationsBox() *fyne.Container {
	box := container.NewVBox(
		widget.NewLabel("Notificações"),
	)

	// TODO: Últimos episódios baixados
	// TODO: Últimos episódios que falharam
	// TODO: Próximos episódios que vão sair

	return box
}

func settingsBox(w fyne.Window) *fyne.Container {
	configs := LoadConfigs()

	box := container.NewVBox(
		widget.NewLabel("Configurações"),
	)

	selectFolderButton := widget.NewButton("Selecionar pasta para salvar", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, nil)
				return
			}
			if uri == nil {
				return
			}

			configs := LoadConfigs()
			configs.SavePath = uri.Path()
			SaveConfigs(configs)
		}, w)
	})

	userNameEntry := widget.NewEntry()
	userNameEntry.SetPlaceHolder("Usuário do AniList")
	userNameEntry.SetText(configs.AnilistUsername)

	saveBtn := widget.NewButton("Salvar", func() {
		configs := LoadConfigs()
		configs.AnilistUsername = userNameEntry.Text
		SaveConfigs(configs)
	})

	box.Add(selectFolderButton)
	box.Add(userNameEntry)
	box.Add(saveBtn)

	// TODO: Anilist Username
	// TODO: Save Path
	// TODO: Skip Dialog
	// TODO: Check Interval
	// TODO: Retry limit
	// TODO: Max episodes per check

	return box
}
