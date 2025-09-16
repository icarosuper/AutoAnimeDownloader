package modules

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func CreateUi(startLoop func(dur time.Duration, w fyne.Window, updateDownloadedEpisodesList func()) func(newDur time.Duration)) {
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

	downloadedEpisodesList, downloadedEpisodesData := downloadedEpisodesList()

	updateDownloadedEpisodes := func() {
		updateDownloadedEpisodesList(downloadedEpisodesData)
	}

	configs := LoadConfigs()

	interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
	restartLoop := startLoop(interval, w, updateDownloadedEpisodes)

	setWindowContent(w, restartLoop, downloadedEpisodesList, configs)

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window, restartLoop func(newDur time.Duration), downloadedEpisodesList *widget.List, configs Config) {
	tabs := container.NewAppTabs()

	tabs.Append(container.NewTabItem("Notificações", notificationsBox(restartLoop, downloadedEpisodesList)))
	tabs.Append(container.NewTabItem("Configurações", settingsBox(w, restartLoop, configs)))

	tabs.SelectIndex(0)

	w.SetContent(tabs)
}

func notificationsBox(restartLoop func(newDur time.Duration), downloadedEpisodesList *widget.List) *fyne.Container {
	// TODO: Últimos episódios baixados
	// TODO: Últimos episódios que falharam

	// TODO: Próximos episódios que vão sair
	// ^ Coloca o tempo que falta pra sair na tela atualizando constantemente, se chegar a 0 trigga o reset de checagem
	// incluir nas configs se deve checar automaticamente nesse caso ou não

	title := canvas.NewText("Últimos episódios baixados", color.White)
	title.Alignment = fyne.TextAlignCenter
	title.TextSize = 18

	checkNowBtn := widget.NewButton("Checar atualizações agora", func() {
		interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
		restartLoop(interval)
	})

	box := container.NewBorder(title, checkNowBtn, nil, nil, downloadedEpisodesList)

	return box
}

func downloadedEpisodesList() (*widget.List, binding.ExternalStringList) {
	data := binding.BindStringList(&[]string{})

	updateDownloadedEpisodesList(data)

	list := widget.NewListWithData(
		data,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			o.(*widget.Label).Bind(i.(binding.String))
		})

	return list, data
}

func updateDownloadedEpisodesList(data binding.ExternalStringList) {
	downloadedEpisodes := LoadSavedEpisodes()

	episodes := make([]string, 0, len(downloadedEpisodes))
	for i := len(downloadedEpisodes) - 1; i >= 0; i-- {
		episodes = append(episodes, downloadedEpisodes[i].EpisodeName)
	}

	data.Set(episodes)
}

func settingsBox(w fyne.Window, restartLoop func(newDur time.Duration), configs Config) *fyne.Container {
	changeSavePathBtn := changeSavePathBtn(w)

	qBittorrentEntry := changeQBittorrentBaseUrlEntry(configs)

	userNameEntry := changeUserNameEntry(configs)

	changeIntervalEntry := changeIntervalEntry(configs)

	deleteAfterWatched := deleteWatchedEpisodesCheck(configs)

	saveBtn := saveBtn(func() {
		saveConfigs(w, restartLoop, userNameEntry, changeIntervalEntry, qBittorrentEntry)
	})

	box := container.NewVBox()
	box.Add(changeSavePathBtn)
	box.Add(widget.NewLabel("Seu nome de usuário no AniList"))
	box.Add(userNameEntry)
	box.Add(widget.NewLabel("Intervalo de checagem de animes (em minutos)"))
	box.Add(changeIntervalEntry)
	box.Add(deleteAfterWatched)
	box.Add(widget.NewLabel("URL base do qBittorrent (não altere isso se não souber o que significa)"))
	box.Add(qBittorrentEntry)
	box.Add(saveBtn)

	// TODO: Retry limit
	// TODO: Max episodes per check
	// TODO: Max episodes in download folder
	// TODO: Cleanup watched episodes

	return box
}

func saveConfigs(w fyne.Window, restartLoop func(newDur time.Duration), userNameEntry, changeIntervalEntry, qBittorrentEntry *widget.Entry) {
	configs := LoadConfigs()

	if err := userNameEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := changeIntervalEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := qBittorrentEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if configs.SavePath == "" {
		dialog.ShowError(fmt.Errorf("caminho de salvamento não pode estar vazio"), w)
		return
	}

	newUsername := userNameEntry.Text
	newQBittorrentUrl := qBittorrentEntry.Text

	var newInterval int
	_, err := fmt.Sscanf(changeIntervalEntry.Text, "%d", &newInterval)
	if err != nil {
		dialog.ShowError(fmt.Errorf("o intervalo de checagem deve ser um número inteiro positivo"), w)
		return
	}

	if configs.AnilistUsername == newUsername && configs.CheckInterval == newInterval && configs.QBittorrentUrl == newQBittorrentUrl {
		return
	}

	configs.AnilistUsername = newUsername
	configs.CheckInterval = newInterval
	configs.QBittorrentUrl = newQBittorrentUrl
	SaveConfigs(configs)

	dialog.ShowInformation("Configurações salvas", "As configurações foram salvas com sucesso.", w)

	restartLoop(time.Duration(newInterval) * time.Minute)
}

func saveBtn(saveFunc func()) *widget.Button {
	return widget.NewButton("Salvar", saveFunc)
}

func deleteWatchedEpisodesCheck(configs Config) *widget.Check {
	check := widget.NewCheck("Deletar episódios assistidos", func(isChecked bool) {
		configs := LoadConfigs()
		configs.DeleteWatchedEpisodes = isChecked
		SaveConfigs(configs)
	})
	check.SetChecked(configs.DeleteWatchedEpisodes)
	return check
}

func changeSavePathBtn(w fyne.Window) *widget.Button {
	return widget.NewButton("Alterar caminho de salvamento dos animes", func() {
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
}

func changeUserNameEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Insira seu nome de usuário do AniList")
	entry.SetText(configs.AnilistUsername)

	entry.OnChanged = func(s string) {
		entry.SetText(strings.TrimSpace(s))
	}

	entry.Validator = func(s string) error {
		if len(s) == 0 {
			return fmt.Errorf("username do AniList não pode estar vazio")
		}
		return nil
	}

	return entry
}

func changeIntervalEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Intervalo de checagem (em minutos)")
	entry.SetText(fmt.Sprintf("%d", configs.CheckInterval))

	entry.Validator = func(s string) error {
		var v int
		_, err := fmt.Sscanf(s, "%d", &v)
		if err != nil || v <= 0 {
			return fmt.Errorf("o intervalo de checagem deve ser um número inteiro positivo")
		}
		return nil
	}

	return entry
}

func changeQBittorrentBaseUrlEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Insira a URL do qBittorrent")
	entry.SetText(configs.QBittorrentUrl)

	entry.Validator = func(s string) error {
		if len(s) == 0 {
			return fmt.Errorf("a URL base do qBittorrent não pode estar vazio")
		}
		return nil
	}

	return entry
}
