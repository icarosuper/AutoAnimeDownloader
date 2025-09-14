package modules

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func CreateUi(restartLoop func(newDur time.Duration)) {
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

	setWindowContent(w, restartLoop)

	configs := LoadConfigs()
	if configs.AnilistUsername == "" || configs.SavePath == "" {
		dialog.ShowInformation("Configuração necessária", "Por favor, configure seu nome de usuário do AniList e o caminho de salvamento nas configurações.", w)
	}

	w.ShowAndRun()
}

func setWindowContent(w fyne.Window, restartLoop func(newDur time.Duration)) {
	tabs := container.NewAppTabs()

	tabs.Append(container.NewTabItem("Notificações", notificationsBox(restartLoop)))
	tabs.Append(container.NewTabItem("Configurações", settingsBox(w, restartLoop)))

	tabs.SelectIndex(0)

	w.SetContent(tabs)
}

func notificationsBox(restartLoop func(newDur time.Duration)) *fyne.Container {
	box := container.NewVBox(
		widget.NewLabel("Notificações"),
	)

	// TODO: Últimos episódios baixados
	// TODO: Últimos episódios que falharam

	// TODO: Próximos episódios que vão sair
	// ^ Coloca o tempo que falta pra sair na tela atualizando constantemente, se chegar a 0 trigga o reset de checagem
	// incluir nas configs se deve checar automaticamente nesse caso ou não

	checkNowBtn := widget.NewButton("Checar atualizações agora", func() {
		interval := time.Duration(LoadConfigs().CheckInterval) * time.Minute
		restartLoop(interval)
	})

	box.Add(checkNowBtn)

	return box
}

func settingsBox(w fyne.Window, restartLoop func(newDur time.Duration)) *fyne.Container {
	configs := LoadConfigs()

	box := container.NewVBox(
		widget.NewLabel("Configurações"),
	)

	selectFolderButton := changePathBtn(w)

	userNameEntry := changeUserNameEntry(configs)

	changeIntervalEntry := changeIntervalEntry(configs)

	saveBtn := widget.NewButton("Salvar", func() {
		configs := LoadConfigs()

		if err := userNameEntry.Validate(); err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := changeIntervalEntry.Validate(); err != nil {
			dialog.ShowError(err, w)
			return
		}
		if configs.SavePath == "" {
			dialog.ShowError(fmt.Errorf("caminho de salvamento não pode estar vazio"), w)
			return
		}

		newUsername := userNameEntry.Text

		var newInterval int
		_, err := fmt.Sscanf(changeIntervalEntry.Text, "%d", &newInterval)
		if err != nil {
			dialog.ShowError(fmt.Errorf("o intervalo de checagem deve ser um número inteiro positivo"), w)
			return
		}

		if configs.AnilistUsername == newUsername && configs.CheckInterval == newInterval {
			return
		}

		configs.AnilistUsername = newUsername
		configs.CheckInterval = newInterval
		SaveConfigs(configs)

		dialog.ShowInformation("Configurações salvas", "As configurações foram salvas com sucesso.", w)

		restartLoop(time.Duration(newInterval) * time.Minute)
	})

	box.Add(selectFolderButton)
	box.Add(widget.NewLabel("Seu UserName no AniList"))
	box.Add(userNameEntry)
	box.Add(widget.NewLabel("Intervalo de checagem (em minutos)"))
	box.Add(changeIntervalEntry)
	box.Add(saveBtn)

	// TODO: Skip Dialog
	// TODO: Retry limit
	// TODO: Max episodes per check
	// TODO: Max episodes in download folder
	// TODO: Cleanup watched episodes

	return box
}

func changePathBtn(w fyne.Window) *widget.Button {
	return widget.NewButton("Alterar caminho de salvamento", func() {
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
	entry.SetPlaceHolder("AniList Username")
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
