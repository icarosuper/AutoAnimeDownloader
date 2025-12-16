// Auto Anime Downloader - Daemon
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

// @title           Auto Anime Downloader API
// @version         1.0
// @description     REST API for Auto Anime Downloader daemon
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  GPL-3.0
// @license.url   https://www.gnu.org/licenses/gpl-3.0.html

// @host      localhost:8091
// @BasePath  /api/v1

// @schemes   http
package main

import (
	"AutoAnimeDownloader/src/internal/api"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	_ "AutoAnimeDownloader/docs"
)

func getEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		return "dev"
	}
	return env
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return ":8091"
	}
	// Ensure port starts with : if not already present
	if !strings.HasPrefix(port, ":") {
		return ":" + port
	}
	return port
}

const pidFileName = "daemon.pid"

func getPIDFilePath() (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	pidDir := filepath.Join(baseFolder, ".autoAnimeDownloader")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create PID directory: %w", err)
	}

	return filepath.Join(pidDir, pidFileName), nil
}

func createPIDFile() error {
	pidPath, err := getPIDFilePath()
	if err != nil {
		return err
	}

	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

func removePIDFile() error {
	pidPath, err := getPIDFilePath()
	if err != nil {
		return err
	}

	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	return nil
}

func main() {
	environment := getEnvironment()
	isDevelopment := environment == "dev"
	logger.Init(isDevelopment)
	logger.Logger.Info().
		Str("environment", environment).
		Msg("Starting Auto Anime Downloader...")

	// Create PID file when daemon starts
	if err := createPIDFile(); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to create PID file, continuing anyway")
	}

	// Ensure PID file is removed on exit
	defer func() {
		if err := removePIDFile(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to remove PID file")
		}
	}()

	fileManager, err := files.NewDefaultFileManager()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to initialize files manager")
	}

	state := daemon.NewState()

	apiPort := getPort()
	apiServer := api.NewServer(apiPort, state, fileManager, func(p daemon.StartLoopPayload) func(time.Duration) {
		return daemon.StartLoop(p)
	})

	// Set WebSocket manager as state notifier
	state.SetNotifier(apiServer.WSManager)

	go func() {
		if err := apiServer.Start(); err != nil && err != context.Canceled {
			logger.Logger.Fatal().Err(err).Msg("API server failed")
		}
	}()

	logger.Logger.Info().
		Str("port", apiPort).
		Msg("API server started successfully")

	if err := apiServer.StartDaemonLoop(); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to start daemon loop automatically, daemon will start as stopped")
	} else {
		logger.Logger.Info().Msg("Daemon loop started automatically")
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Logger.Info().Msg("Shutdown signal received")

	// Stop daemon loop if running
	if state.GetStatus() != daemon.StatusStopped {
		apiServer.StopDaemonLoop()
		logger.Logger.Info().Msg("Daemon loop stopped")
	}

	// Stop API server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := apiServer.Stop(ctx); err != nil {
		logger.Logger.Error().Err(err).Msg("Error stopping API server")
	} else {
		logger.Logger.Info().Msg("API server stopped gracefully")
	}
}
