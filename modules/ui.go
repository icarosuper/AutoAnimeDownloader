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

func CreateUi(startLoop func(dur time.Duration, w fyne.Window, updateEpisodesList func(), isLoading binding.ExternalBool) func(newDur time.Duration)) {
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

	downloadedEpisodesBoundData := binding.BindStringList(&[]string{})
	isLoading := false
	isLoadingBoundData := binding.BindBool(&isLoading)

	downloadedEpisodesList := downloadedEpisodesWidget(downloadedEpisodesBoundData)

	updateDownloadedEpisodes := func() {
		updateDownloadedEpisodesList(downloadedEpisodesBoundData)
	}

	configs := LoadConfigs()

	interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
	restartLoop := startLoop(interval, w, updateDownloadedEpisodes, isLoadingBoundData)

	notifications := notificationsBox(restartLoop, downloadedEpisodesList, isLoadingBoundData)
	settings := settingsBox(w, restartLoop, configs)
	setWindowContent(w, notifications, settings)

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window, notifications fyne.CanvasObject, settings fyne.CanvasObject) {
	tabs := container.NewAppTabs()

	tabs.Append(container.NewTabItem("Notificações", notifications))
	tabs.Append(container.NewTabItem("Configurações", settings))

	tabs.SelectIndex(0)

	w.SetContent(tabs)
}

func notificationsBox(restartLoop func(newDur time.Duration), downloadedEpisodesList *widget.List, isLoading binding.ExternalBool) *fyne.Container {
	// TODO: Últimos episódios que falharam
	// TODO: Próximos episódios que vão sair
	// ^ Coloca o tempo que falta pra sair na tela atualizando constantemente, se chegar a 0 trigga o reset de checagem

	title := canvas.NewText("Últimos episódios baixados", color.White)
	title.Alignment = fyne.TextAlignCenter
	title.TextSize = 18

	checkNowBtn := widget.NewButton("Checar atualizações agora", func() {
		interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
		restartLoop(interval)
	})
	loadingBar := widget.NewProgressBarInfinite()

	bottom := container.NewVBox(checkNowBtn, loadingBar)

	box := container.NewBorder(title, bottom, nil, nil, downloadedEpisodesList)

	isLoading.AddListener(binding.NewDataListener(func() {
		loading, err := isLoading.Get()
		if err != nil {
			fmt.Println(err)
		}

		if loading {
			checkNowBtn.Hide()
			loadingBar.Show()
		} else {
			checkNowBtn.Show()
			loadingBar.Hide()
		}
	}))

	return box
}

func downloadedEpisodesWidget(boundedList binding.ExternalStringList) *widget.List {
	updateDownloadedEpisodesList(boundedList)

	list := widget.NewListWithData(
		boundedList,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			o.(*widget.Label).Bind(i.(binding.String))
		})

	return list
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
	episodeRetryLimitEntry := changeEpisodeRetryLimitEntry(configs)
	maxEpisodesPerAnimeEntry := changeMaxEpisodesPerAnimeEntry(configs)

	inputs := configInputs{
		userNameEntry:            userNameEntry,
		changeIntervalEntry:      changeIntervalEntry,
		qBittorrentUrlEntry:      qBittorrentEntry,
		episodeRetryLimitEntry:   episodeRetryLimitEntry,
		maxEpisodesPerAnimeEntry: maxEpisodesPerAnimeEntry,
	}

	saveBtn := saveBtn(func() {
		saveConfigs(w, restartLoop, inputs)
	})

	box := container.NewVBox()
	box.Add(changeSavePathBtn)
	box.Add(widget.NewLabel("Seu nome de usuário no AniList"))
	box.Add(userNameEntry)
	box.Add(widget.NewLabel("Intervalo de checagem de animes (em minutos)"))
	box.Add(changeIntervalEntry)
	box.Add(widget.NewLabel("Limite de tentativas de download por episódio"))
	box.Add(episodeRetryLimitEntry)
	box.Add(widget.NewLabel("Máximo de episódios baixados por anime"))
	box.Add(maxEpisodesPerAnimeEntry)
	box.Add(deleteAfterWatched)
	box.Add(widget.NewLabel("URL base do qBittorrent (não altere isso se não souber o que significa)"))
	box.Add(qBittorrentEntry)
	box.Add(saveBtn)

	container := container.NewBorder(nil, saveBtn, nil, nil, box)

	return container
}

type configInputs struct {
	userNameEntry            *widget.Entry
	changeIntervalEntry      *widget.Entry
	qBittorrentUrlEntry      *widget.Entry
	episodeRetryLimitEntry   *widget.Entry
	maxEpisodesPerAnimeEntry *widget.Entry
}

func saveConfigs(w fyne.Window, restartLoop func(newDur time.Duration), inputs configInputs) {
	configs := LoadConfigs()

	if err := inputs.userNameEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := inputs.changeIntervalEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := inputs.qBittorrentUrlEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := inputs.episodeRetryLimitEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if err := inputs.maxEpisodesPerAnimeEntry.Validate(); err != nil {
		dialog.ShowError(err, w)
		return
	}
	if configs.SavePath == "" {
		dialog.ShowError(fmt.Errorf("caminho de salvamento não pode estar vazio"), w)
		return
	}

	var newCheckInterval int
	_, err := fmt.Sscanf(inputs.changeIntervalEntry.Text, "%d", &newCheckInterval)
	if err != nil {
		dialog.ShowError(fmt.Errorf("o intervalo de checagem deve ser um número inteiro positivo"), w)
		return
	}

	var newEpisodeRetryLimit int
	_, err = fmt.Sscanf(inputs.episodeRetryLimitEntry.Text, "%d", &newEpisodeRetryLimit)
	if err != nil {
		dialog.ShowError(fmt.Errorf("o limite de tentativas por episódio deve ser um número inteiro positivo"), w)
		return
	}

	var newMaxEpisodesPerAnime int
	_, err = fmt.Sscanf(inputs.maxEpisodesPerAnimeEntry.Text, "%d", &newMaxEpisodesPerAnime)
	if err != nil {
		dialog.ShowError(fmt.Errorf("o máximo de episódios por anime deve ser um número inteiro positivo"), w)
		return
	}

	configs.AnilistUsername = inputs.userNameEntry.Text
	configs.CheckInterval = newCheckInterval
	configs.QBittorrentUrl = inputs.qBittorrentUrlEntry.Text
	configs.EpisodeRetryLimit = newEpisodeRetryLimit
	configs.MaxEpisodesPerAnime = newMaxEpisodesPerAnime
	SaveConfigs(configs)

	dialog.ShowInformation("Configurações salvas", "As configurações foram salvas com sucesso.", w)

	restartLoop(time.Duration(newCheckInterval) * time.Minute)
}

func saveBtn(saveFunc func()) *widget.Button {
	return widget.NewButton("Salvar configurações", saveFunc)
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

func changeEpisodeRetryLimitEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Limite de tentativas de download por episódio")
	entry.SetText(fmt.Sprintf("%d", configs.EpisodeRetryLimit))

	entry.Validator = func(s string) error {
		var v int
		_, err := fmt.Sscanf(s, "%d", &v)
		if err != nil || v <= 0 {
			return fmt.Errorf("o limite de tentativas por episódio deve ser um número inteiro positivo")
		}
		return nil
	}

	return entry
}

func changeMaxEpisodesPerAnimeEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Máximo de episódios baixados por anime")
	entry.SetText(fmt.Sprintf("%d", configs.MaxEpisodesPerAnime))

	entry.Validator = func(s string) error {
		var v int
		_, err := fmt.Sscanf(s, "%d", &v)
		if err != nil || v <= 0 {
			return fmt.Errorf("o máximo de episódios por anime deve ser um número inteiro positivo")
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
