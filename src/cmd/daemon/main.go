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
	"AutoAnimeDownloader/src/internal/torrents"
	"AutoAnimeDownloader/src/internal/tray"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	_ "AutoAnimeDownloader/docs/swagger"
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

func getJobsFilePath() (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	return filepath.Join(baseFolder, ".autoAnimeDownloader", "pending_jobs.json"), nil
}

// getSessionDBPath returns the embedded torrent client's resume database path. It lives in
// the config folder (not the data dir) so resume data survives a save-path change.
func getSessionDBPath() (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	return filepath.Join(baseFolder, ".autoAnimeDownloader", "session.db"), nil
}

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

func shouldShowTray() bool {
	// Check if we're in a graphical environment
	// On Linux, check for DISPLAY variable
	if runtime.GOOS == "linux" {
		return os.Getenv("DISPLAY") != ""
	}
	// Windows and macOS always have GUI
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// runDebugAnime runs a one-shot debug pass for a single anime (AniList
// MediaList ID) and exits. Skips the PID file, API server, tray, and loop
// entirely — this is a read-only diagnostic, not a daemon startup path.
// Output goes to a new .debug_<animeId>_<N>/ directory in the current
// working directory (debug.jsonl trace + summary.json) — nothing is written
// to ~/.autoAnimeDownloader.
func runDebugAnime(animeId int) {
	debugDir, err := daemon.NextDebugDir(".", animeId)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine debug directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create debug directory %s: %v\n", debugDir, err)
		os.Exit(1)
	}

	closeLog, err := logger.InitDebug(filepath.Join(debugDir, "debug.jsonl"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize debug logger: %v\n", err)
		os.Exit(1)
	}
	defer closeLog()

	fileManager, err := files.NewDefaultFileManager()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to initialize files manager")
	}

	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to load configs")
	}

	summary, err := daemon.RunAnimeDebug(animeId, configs, fileManager)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Debug run failed")
		os.Exit(1)
	}

	if err := daemon.WriteDebugSummary(debugDir, summary); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to write summary.json")
	}

	logger.Logger.Info().Str("debug_dir", debugDir).Msg("Debug run complete")
	os.Exit(0)
}

// ensureStartupSession creates the embedded torrent session at boot when a save path is
// already configured, so seeding starts with the process instead of waiting for the first
// verification pass. An incomplete config is not an error here — the session stays lazy and
// the verification pass creates it once the config is saved.
func ensureStartupSession(manager *torrents.SessionManager, fileManager *files.FileManager) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load configs at startup; torrent session will be created on the first verification pass")
		return
	}
	if configs == nil || configs.DownloadPath() == "" {
		logger.Logger.Info().Msg("Completed anime path not configured; torrent session will be created once the configuration is saved")
		return
	}
	// Antes do Ensure: a sessao nova roda a adocao da fila assim que nasce, e ela precisa
	// ja saber o limite para nao promover tudo o que a rain reabriu parado.
	manager.SetMaxActiveDownloads(configs.MaxConcurrentDownloads)
	downloadPath := configs.DownloadPath()
	if _, err := manager.Ensure(downloadPath); err != nil {
		logger.Logger.Error().Err(err).Str("download_path", downloadPath).Msg("Failed to create the embedded torrent session at startup; the verification pass will retry")
		return
	}
	logger.Logger.Info().Str("download_path", downloadPath).Msg("Embedded torrent session started; seeding is active independently of the daemon loop")
}

func main() {
	debugAnimeID := flag.Int("debug-anime", 0, "Run a one-shot debug pass for this AniList MediaList ID and exit (no daemon, no downloads)")
	flag.Parse()

	if *debugAnimeID > 0 {
		runDebugAnime(*debugAnimeID)
		return
	}

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

	jobsFilePath, err := getJobsFilePath()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to determine jobs file path")
	}
	jobQueue := daemon.NewJobQueue(fileManager, jobsFilePath)

	// Embedded torrent client (rain) and the library organizer (hardlinks into the
	// completed-anime folder). The session is created lazily by SessionManager once the
	// save path is known (config may be incomplete at startup).
	sessionDBPath, err := getSessionDBPath()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to determine session database path")
	}
	torrentManager := torrents.NewSessionManager(sessionDBPath)
	librarian := files.NewLibrarian(files.NewOSFileSystem())

	// Completion events enqueue a durable JobOrganize; failures notify and drop the torrent
	// so the next verification pass re-adds it.
	torrentManager.SetCallbacks(
		func(hash string) { jobQueue.EnqueueOrganize(hash) },
		func(hash string, err error) {
			daemon.HandleTorrentFailure(hash, err, torrentManager, fileManager)
		},
	)
	jobQueue.SetOrchestration(torrentManager, librarian)
	jobQueue.Start()
	// Converte uma instalacao antiga (save_path configurado) para o modelo de pasta unica,
	// movendo os dados. Roda DEPOIS de jobQueue.Start(), entao Start() ja carregou a lista
	// de jobs persistida antes da sessao temporaria de migracao existir: se essa sessao
	// disparar uma conclusao espuria, ela NAO e descartada por Start(). O risco e limitado
	// e se autocorrige: um job de organize afetado bate em ErrSessionNotReady ou num DataDir
	// obsoleto, falha, e o passe de verificacao tenta de novo. Uma falha na migracao em si
	// tambem nao e fatal aqui — mas o boot so pode abrir a sessao real (ensureStartupSession)
	// no caminho derivado se a migracao teve sucesso; caso contrario os dados ainda estao no
	// caminho antigo e abrir a sessao no novo caminho reverificaria tudo a 0%.
	migrationErr := daemon.MigrateSavePath(files.NewOSFileSystem(), fileManager, torrentManager)
	if migrationErr != nil {
		logger.Logger.Error().Err(migrationErr).Msg("Failed to migrate the legacy save path; the verification pass will retry")
	}
	// Defers run LIFO: register Close first so jobQueue.Stop() (which may run organize jobs
	// that use the session) runs before the session is closed.
	defer func() {
		if err := torrentManager.Close(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Error closing torrent session")
		}
	}()
	defer jobQueue.Stop()

	// Create the session at startup when the config is already complete, so seeding resumes
	// with the process and is independent of the daemon loop (stopping the loop from the
	// WebUI must not stop seeding). Runs after jobQueue.Start() because arming the resume
	// listeners can fire a completion immediately, and Start() reloads jobs from disk over
	// whatever is in memory. With no save path configured the session stays lazy: the Ensure
	// in the verification pass creates it once the config is saved. Startup reconciliation
	// stays in the verification pass (safety net) and runs on its first tick.
	//
	// Skipped when the migration above failed: the data may still be sitting at the legacy
	// save path, so opening rain at the derived DownloadPath() now would resume every
	// torrent against an empty directory and re-download the whole library. Leaving the
	// session lazy here keeps the verification pass retrying the migration on each tick
	// until it succeeds (see decisions.md #31).
	if migrationErr == nil {
		ensureStartupSession(torrentManager, fileManager)
	}

	state := daemon.NewState()

	apiPort := getPort()
	apiServer := api.NewServer(apiPort, state, fileManager, func(p daemon.StartLoopPayload) *daemon.LoopControl {
		return daemon.StartLoop(p)
	})
	apiServer.JobQueue = jobQueue
	apiServer.Torrents = torrentManager
	apiServer.Librarian = librarian

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

	// Initialize tray icon if in graphical environment
	var trayManager *tray.TrayManager
	if shouldShowTray() {
		trayManager = tray.NewTrayManager(apiPort)
		trayManager.Start()
		logger.Logger.Info().Msg("System tray icon started")
	}

	// Graceful shutdown - wait for either signal or tray quit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var trayShutdownChan <-chan struct{}
	if trayManager != nil {
		trayShutdownChan = trayManager.WaitForShutdown()
	} else {
		// If no tray, create a channel that never closes
		c := make(chan struct{})
		trayShutdownChan = c
	}

	select {
	case <-sigChan:
		logger.Logger.Info().Msg("Shutdown signal received")
	case <-trayShutdownChan:
		logger.Logger.Info().Msg("Shutdown requested from tray icon")
	}

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
