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

package main

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/api"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/datadir"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/torrents"
	"AutoAnimeDownloader/src/internal/tray"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
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
	dir, err := datadir.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, pidFileName), nil
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
// media ID) and exits. Skips the PID file, API server, tray, and loop
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
	// Before Ensure: the new session runs the queue adoption as soon as it's born, and it needs to
	// know the limit already so it doesn't promote everything rain reopened stopped.
	manager.SetMaxActiveDownloads(configs.MaxConcurrentDownloads)
	downloadPath := configs.DownloadPath()
	if _, err := manager.Ensure(downloadPath); err != nil {
		logger.Logger.Error().Err(err).Str("download_path", downloadPath).Msg("Failed to create the embedded torrent session at startup; the verification pass will retry")
		return
	}
	logger.Logger.Info().Str("download_path", downloadPath).Msg("Embedded torrent session started; seeding is active independently of the daemon loop")
}

func main() {
	debugAnimeID := flag.Int("debug-anime", 0, "Run a one-shot debug pass for this AniList media ID and exit (no daemon, no downloads)")
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

	// Before any LoadConfigs: it's what creates config.json with the defaults, which would erase the
	// first-run signal.
	firstBoot := !fileManager.ConfigExists()

	// AniList cache on disk. Before anything that talks to AniList: it's what keeps the screens and
	// the pass standing while it's down, restart included.
	anilist.EnablePersistence(fileManager.LoadAnilistCache, fileManager.SaveAnilistCache)

	dataDir, err := datadir.Path()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to determine the data directory")
	}
	jobQueue := daemon.NewJobQueue(fileManager, filepath.Join(dataDir, "pending_jobs.json"))

	// Embedded torrent client (rain) and the library organizer (hardlinks into the
	// completed-anime folder). The session is created lazily by SessionManager once the
	// save path is known (config may be incomplete at startup).
	// The resume database lives in the data dir, not the download dir, so resume data survives a
	// save-path change.
	torrentManager := torrents.NewSessionManager(filepath.Join(dataDir, "session.db"))
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
	// Converts the saved AnimeIDs from entry id to media id (decisions.md #43). Runs at boot, not
	// only in the verification pass, because the API starts serving /animes before the first tick —
	// and with ids in the old format the listing duplicates animes that are in more than one
	// account. Failing here isn't fatal: the verification pass retries and aborts if it still
	// hasn't worked.
	if err := daemon.MigrateAnimeIDsToMedia(fileManager); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to migrate anime IDs to AniList media IDs; the verification pass will retry")
	} else if saved, err := fileManager.LoadSavedEpisodes(); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load saved episodes for the tvshow.nfo backfill")
	} else {
		// Only after the migration above succeeded: with an entry id the nfo would come out with the
		// wrong id, and it's never rewritten. Idempotent (doesn't overwrite), so it runs every boot.
		librarian.BackfillShowNFOs(saved)
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
	// whatever is in memory. With no library configured the session stays lazy: the Ensure
	// in the verification pass creates it once the config is saved. Startup reconciliation
	// stays in the verification pass (safety net) and runs on its first tick.
	ensureStartupSession(torrentManager, fileManager)

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
		// ErrServerClosed is the normal shutdown below (apiServer.Stop): treating it as fatal ran
		// os.Exit(1) in the middle of the shutdown, skipping the deferred session Close and PID removal.
		if err := apiServer.Start(); err != nil && err != context.Canceled && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Fatal().Err(err).Msg("API server failed")
		}
	}()

	logger.Logger.Info().
		Str("port", apiPort).
		Msg("API server started successfully")

	// First run: opens the UI on the status page. Doesn't depend on the config being complete — the
	// verification pass already opens /#/config when the library is missing, and this boot with new
	// defaults (see files.getDefaultConfig) usually doesn't trigger that path.
	//
	// Waits for the port to accept connections before opening: Start() above runs in a goroutine
	// and the log right below it comes out BEFORE the socket is bound, so opening right away is a
	// race whose losing side is a tab with ERR_CONNECTION_REFUSED.
	if firstBoot {
		go func() {
			if err := daemon.WaitForListener(apiPort, 10*time.Second); err != nil {
				logger.Logger.Warn().Err(err).Msg("Web UI did not come up in time; not opening the browser")
				return
			}
			if err := daemon.OpenBrowser(daemon.WebUIURL(apiPort, "/#/status")); err != nil {
				logger.Logger.Warn().Err(err).Msg("Failed to open the Web UI in the browser on first boot")
			}
		}()
	}

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

	// After stopping the loop (which is what produces notifications) and before exiting: sends
	// whatever is still waiting for the batching window.
	notifications.Flush()

	// Stop API server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := apiServer.Stop(ctx); err != nil {
		logger.Logger.Error().Err(err).Msg("Error stopping API server")
	} else {
		logger.Logger.Info().Msg("API server stopped gracefully")
	}
}
