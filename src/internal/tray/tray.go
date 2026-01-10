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
	"fmt"
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
	systray.SetTooltip("AutoAnimeDownloader - Click to open")

	mOpen := systray.AddMenuItem("Open", "Opens the app page on your browser")

	systray.AddSeparator()

	mCheckEpisodes := systray.AddMenuItem("Check for new episodes", "Starts a verification for new anime episodes")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit", "Shuts the app down")

	// Handle menu events
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				tm.openWebUI()
			case <-mCheckEpisodes.ClickedCh:
				tm.triggerCheck()
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
	url := tm.getWebUiURL()

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

func (tm *TrayManager) getWebUiURL() string {
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

func (tm *TrayManager) triggerCheck() {
	logger.Logger.Info().Msg("Triggering manual episode check via tray icon")

	go func() {
		url := fmt.Sprintf("%s/api/v1/check", tm.getWebUiURL())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
		if err != nil {
			logger.Logger.Error().
				Err(err).
				Msg("Failed to create check request")
			return
		}

		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			logger.Logger.Error().
				Err(err).
				Msg("Failed to trigger check")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			logger.Logger.Info().Msg("Manual check triggered successfully")
		} else {
			logger.Logger.Error().
				Int("status", resp.StatusCode).
				Msg("Failed to trigger check")
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
