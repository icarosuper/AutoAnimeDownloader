//go:build !cgo || notray
// +build !cgo notray

// Auto Anime Downloader - Tray Icon Manager (Stub)
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
)

// TrayManager manages the system tray icon (stub implementation)
type TrayManager struct {
	shutdownChan chan struct{}
	apiPort      string
}

// NewTrayManager creates a new TrayManager instance (stub)
func NewTrayManager(apiPort string) *TrayManager {
	return &TrayManager{
		shutdownChan: make(chan struct{}),
		apiPort:      apiPort,
	}
}

// Start initializes and starts the tray icon (stub - does nothing)
func (tm *TrayManager) Start() {
	logger.Logger.Debug().Msg("Tray icon support not available (built without CGO or systray dependencies)")
}

// WaitForShutdown returns a channel that will never close (stub)
func (tm *TrayManager) WaitForShutdown() <-chan struct{} {
	// Return a channel that never closes, so select will wait for signal instead
	c := make(chan struct{})
	return c
}

