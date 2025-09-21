package modules

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type startLoopFuncType func(dur time.Duration, showDialog func(string, string), updateEpisodesList func(), setLoading func(bool)) func(newDur time.Duration)

func CreateUi(startLoop startLoopFuncType) {
	a := app.New()

	a.Settings().SetTheme(theme.DefaultTheme())

	w := a.NewWindow("Auto Anime Downloader")
	w.Resize(fyne.NewSize(1000, 700))
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

	interval := time.Duration(configs.CheckInterval) * time.Minute
	showDialog := func(title string, message string) {
		dialog.ShowInformation(title, message, w)
	}
	setLoading := func(loading bool) {
		isLoadingBoundData.Set(loading)
	}
	restartLoop := startLoop(interval, showDialog, updateDownloadedEpisodes, setLoading)

	notifications := notificationsBox(restartLoop, downloadedEpisodesList, isLoadingBoundData)
	settings := settingsBox(w, restartLoop, configs)
	setWindowContent(w, notifications, settings)

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window, notifications fyne.CanvasObject, settings fyne.CanvasObject) {
	tabs := container.NewAppTabs()

	customLayout := layout.NewCustomPaddedLayout(10, 10, 15, 15)

	tabs.Append(container.NewTabItem("Notificações", container.New(customLayout, notifications)))
	tabs.Append(container.NewTabItem("Configurações", container.New(customLayout, settings)))

	tabs.SelectIndex(0)

	w.SetContent(tabs)
}

func notificationsBox(restartLoop func(newDur time.Duration), downloadedEpisodesList *widget.List, isLoading binding.ExternalBool) *fyne.Container {
	title := canvas.NewText("Últimos episódios baixados", color.White)
	title.Alignment = fyne.TextAlignCenter
	title.TextSize = 18

	checkNowBtn := widget.NewButton("Checar atualizações agora", func() {
		interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
		restartLoop(interval)
	})
	loadingBar := widget.NewProgressBarInfinite()

	// Adicionar espaçamento ao botão
	buttonContainer := container.NewPadded(checkNowBtn)
	bottom := container.NewVBox(buttonContainer, loadingBar)

	// Adicionar padding geral
	box := container.NewBorder(
		container.NewPadded(title),
		container.NewPadded(bottom),
		nil, nil,
		container.NewPadded(downloadedEpisodesList),
	)

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

	err := data.Set(episodes)
	if err != nil {
		return
	}
}

func settingsBox(w fyne.Window, restartLoop func(newDur time.Duration), configs Config) *fyne.Container {
	changeSavePathBtn := changeSavePathBtn(w)
	qBittorrentEntry := changeQBittorrentBaseUrlEntry(configs)
	userNameEntry := changeUserNameEntry(configs)
	changeIntervalEntry := changeIntervalEntry(configs)
	deleteAfterWatched := deleteWatchedEpisodesCheck(configs)
	episodeRetryLimitEntry := changeEpisodeRetryLimitEntry(configs)
	maxEpisodesPerAnimeEntry := changeMaxEpisodesPerAnimeEntry(configs)
	excludedListEntry := changeExcludedListEntry(configs)

	inputs := configInputs{
		userNameEntry:            userNameEntry,
		changeIntervalEntry:      changeIntervalEntry,
		qBittorrentUrlEntry:      qBittorrentEntry,
		episodeRetryLimitEntry:   episodeRetryLimitEntry,
		maxEpisodesPerAnimeEntry: maxEpisodesPerAnimeEntry,
		excludedListEntry:        excludedListEntry,
	}

	saveBtn := saveBtn(func() {
		saveConfigs(w, restartLoop, inputs)
	})

	anilistCustomListsUrl, _ := url.Parse("https://anilist.co/settings/lists")
	anilistConfigsBox := sectionBox(
		"Configurações do AniList",
		entryBox(
			userNameEntry,
			"Nome de usuário no AniList",
		),
		entryBox(
			excludedListEntry,
			"Lista personalizada excluída",
			"Animes nessa lista não serão baixados",
		),
		widget.NewHyperlink("Clique aqui para gerenciar suas listas personalizadas.", anilistCustomListsUrl),
	)

	downloadConfigsBox := sectionBox(
		"Configurações de Download",
		changeSavePathBtn,
		container.New(layout.NewCenterLayout(), deleteAfterWatched),
	)

	checkingConfigsBox := sectionBox(
		"Configurações de Checagem",
		entryBox(
			changeIntervalEntry,
			"Intervalo de checagem de animes (em minutos)",
		),
		entryBox(
			episodeRetryLimitEntry,
			"Limite de tentativas de download por episódio",
		),
		entryBox(
			maxEpisodesPerAnimeEntry,
			"Máximo de episódios baixados por anime",
		),
	)

	adittionalConfigsBox := sectionBox(
		"Opções Avançadas",
		entryBox(
			qBittorrentEntry,
			"URL base do qBittorrent",
			"Não altere isso se não souber o que significa",
		),
	)

	box := paddedBox(
		anilistConfigsBox,
		downloadConfigsBox,
		checkingConfigsBox,
		adittionalConfigsBox,
	)

	scrollContainer := container.NewScroll(box)
	scrollContainer.SetMinSize(fyne.NewSize(400, 500))

	saveBtnBox := container.NewHBox(saveBtn)
	saveBtnBox.Layout = layout.NewCustomPaddedLayout(15, 0, 0, 0)

	return container.NewBorder(nil, saveBtnBox, nil, nil, scrollContainer)
}

func entryBox(entry *widget.Entry, labels ...string) *fyne.Container {
	box := container.NewVBox()

	for i, label := range labels {
		txt := canvas.NewText(label, color.White)

		if i == 0 {
			txt.TextSize = 15
			txt.TextStyle = fyne.TextStyle{Bold: true}
		}

		box.Add(txt)
	}

	box.Add(entry)

	return box
}

func sectionBox(title string, objects ...fyne.CanvasObject) *fyne.Container {
	box := container.New(layout.NewCustomPaddedVBoxLayout(15))

	t := canvas.NewText(title, color.White)
	t.TextSize = 20
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.Alignment = fyne.TextAlignCenter

	box.Add(t)
	for _, obj := range objects {
		box.Add(obj)
	}

	return box
}

func paddedBox(objects ...fyne.CanvasObject) *fyne.Container {
	box := container.New(layout.NewCustomPaddedVBoxLayout(25))

	for i, obj := range objects {
		box.Add(obj)
		if i < len(objects)-1 {
			box.Add(canvas.NewLine(color.White))
		}
	}

	return box
}

type configInputs struct {
	userNameEntry            *widget.Entry
	changeIntervalEntry      *widget.Entry
	qBittorrentUrlEntry      *widget.Entry
	episodeRetryLimitEntry   *widget.Entry
	maxEpisodesPerAnimeEntry *widget.Entry
	excludedListEntry        *widget.Entry
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
	if err := inputs.excludedListEntry.Validate(); err != nil {
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
	configs.QBittorrentUrl = inputs.qBittorrentUrlEntry.Text
	configs.ExcludedList = inputs.excludedListEntry.Text
	configs.CheckInterval = newCheckInterval
	configs.EpisodeRetryLimit = newEpisodeRetryLimit
	configs.MaxEpisodesPerAnime = newMaxEpisodesPerAnime
	SaveConfigs(configs)

	dialog.ShowInformation("Configurações salvas", "As configurações foram salvas com sucesso.", w)

	restartLoop(time.Duration(newCheckInterval) * time.Minute)
}

func saveBtn(saveFunc func()) *widget.Button {
	btn := widget.NewButton("Salvar configurações", saveFunc)
	btn.Importance = widget.HighImportance
	return btn
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

func changeExcludedListEntry(configs Config) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Lista personalizada excluída")
	entry.SetText(configs.ExcludedList)

	return entry
}
