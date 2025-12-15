// @title           Auto Anime Downloader API
// @version         1.0
// @description     REST API for Auto Anime Downloader daemon
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

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
	"os"
	"os/signal"
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

func main() {
	environment := getEnvironment()
	isDevelopment := environment == "dev"
	logger.Init(isDevelopment)
	logger.Logger.Info().
		Str("environment", environment).
		Msg("Starting Auto Anime Downloader...")

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
