//go:build cgo && !notray
// +build cgo,!notray

// Auto Anime Downloader - Tray Icon Manager
// Copyright (C) 2024 AutoAnimeDownloader Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tray

import (
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/getlantern/systray"
)

// TrayManager manages the system tray icon
type TrayManager struct {
	shutdownChan chan struct{}
	apiPort      string
}

// NewTrayManager creates a new TrayManager instance
func NewTrayManager(apiPort string) *TrayManager {
	return &TrayManager{
		shutdownChan: make(chan struct{}),
		apiPort:      apiPort,
	}
}

// Start initializes and starts the tray icon in a goroutine
func (tm *TrayManager) Start() {
	go func() {
		systray.Run(tm.onReady, tm.onExit)
	}()
}

// WaitForShutdown returns a channel that will be closed when shutdown is requested
func (tm *TrayManager) WaitForShutdown() <-chan struct{} {
	return tm.shutdownChan
}

func (tm *TrayManager) onReady() {
	// Set icon
	iconData := GetIconData()
	if iconData != nil && len(iconData) > 0 {
		systray.SetIcon(iconData)
	}

	systray.SetTitle("AutoAnimeDownloader")
	systray.SetTooltip("AutoAnimeDownloader - Clique para abrir Web UI")

	// Menu: Abrir Web UI
	mOpen := systray.AddMenuItem("Abrir Web UI", "Abre o frontend no navegador")

	systray.AddSeparator()

	// Menu: Checar Atualizações
	mCheckUpdates := systray.AddMenuItem("Checar Atualizações", "Verifica se há novas versões disponíveis")

	systray.AddSeparator()

	// Menu: Sair
	mQuit := systray.AddMenuItem("Sair", "Encerra o daemon")

	// Handle menu events
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				tm.openWebUI()
			case <-mCheckUpdates.ClickedCh:
				tm.checkUpdates()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (tm *TrayManager) onExit() {
	logger.Logger.Info().Msg("Tray icon exiting, shutting down daemon")
	close(tm.shutdownChan)
}

func (tm *TrayManager) openWebUI() {
	url := tm.getWebUIURL()

	if err := openBrowser(url); err != nil {
		logger.Logger.Error().
			Err(err).
			Str("url", url).
			Msg("Failed to open browser")
	} else {
		logger.Logger.Info().
			Str("url", url).
			Msg("Opened Web UI in browser")
	}
}

func (tm *TrayManager) getWebUIURL() string {
	port := tm.apiPort
	if port == "" {
		port = ":8091"
	}

	// Remove leading colon if present
	port = strings.TrimPrefix(port, ":")
	if port == "" {
		port = "8091"
	}

	return fmt.Sprintf("http://localhost:%s", port)
}

func (tm *TrayManager) checkUpdates() {
	logger.Logger.Info().Msg("Checking for updates...")

	go func() {
		latestVersion, err := getLatestVersion()
		if err != nil {
			logger.Logger.Error().
				Err(err).
				Msg("Failed to check for updates")
			return
		}

		currentVersion := GetCurrentVersion()
		if latestVersion != "" && latestVersion != currentVersion && currentVersion != "dev" {
			logger.Logger.Info().
				Str("current", currentVersion).
				Str("latest", latestVersion).
				Msg("Update available")
			// Open browser to releases page
			openBrowser(fmt.Sprintf("https://github.com/%s/releases/latest", githubRepo))
		} else {
			logger.Logger.Info().
				Str("version", currentVersion).
				Msg("Already running latest version")
		}
	}()
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return cmd.Run()
}

func getLatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}

	// Remove the "v" prefix if present
	tag := release.TagName
	if len(tag) > 0 && tag[0] == 'v' {
		tag = tag[1:]
	}

	return tag, nil
}
