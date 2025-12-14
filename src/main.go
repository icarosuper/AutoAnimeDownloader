package main

import (
	"AutoAnimeDownloader/src/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"time"
)

func main() {
	logger.Init(true) // TODO: Passar esse bool pra env?
	logger.Logger.Info().Msg("Starting Auto Anime Downloader...")

	fileManager, err := files.NewDefaultFileManager()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to initialize files manager")
	}

	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to load configs")
	}

	state := daemon.NewState()
	state.SetStatus(daemon.StatusRunning)

	interval := time.Duration(configs.CheckInterval) * time.Minute

	_ = daemon.StartLoop(daemon.StartLoopPayload{
		FileManager: fileManager,
		Interval:    interval,
		State:       state,
	})

	logger.Logger.Info().
		Dur("interval", interval).
		Msg("Daemon started successfully")

	// TODO: Isso será trocado por um servidor HTTP em breve
	select {}
}
