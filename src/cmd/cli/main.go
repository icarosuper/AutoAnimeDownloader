// Auto Anime Downloader - CLI
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
	"AutoAnimeDownloader/src/internal/api"
	processcli "AutoAnimeDownloader/src/internal/cli"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
)

var (
	outputJSON  bool
	verbose     bool
	apiEndpoint string
)

func main() {
	app := &cli.App{
		Name:  "AutoAnimeDownloader",
		Usage: "CLI for Auto Anime Downloader daemon",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "endpoint",
				Aliases:     []string{"url"},
				Usage:       "API endpoint URL",
				Value:       "http://localhost:8091",
				Destination: &apiEndpoint,
				EnvVars:     []string{"AAD_API_ENDPOINT"},
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output in JSON format",
				Destination: &outputJSON,
			},
			&cli.BoolFlag{
				Name:        "verbose",
				Usage:       "Verbose output",
				Destination: &verbose,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the daemon process",
				Action: func(c *cli.Context) error {
					return handleStart()
				},
			},
			{
				Name:  "stop",
				Usage: "Stop the daemon process",
				Action: func(c *cli.Context) error {
					return handleStop()
				},
			},
			{
				Name:  "loop",
				Usage: "Control the daemon loop",
				Subcommands: []*cli.Command{
					{
						Name:  "start",
						Usage: "Start the verification loop",
						Action: func(c *cli.Context) error {
							return handleLoopStart()
						},
					},
					{
						Name:  "stop",
						Usage: "Stop the verification loop",
						Action: func(c *cli.Context) error {
							return handleLoopStop()
						},
					},
				},
			},
			{
				Name:  "status",
				Usage: "Show daemon status",
				Action: func(c *cli.Context) error {
					return handleStatus()
				},
			},
			{
				Name:  "config",
				Usage: "Manage configuration",
				Subcommands: []*cli.Command{
					{
						Name:  "get",
						Usage: "Get current configuration",
						Action: func(c *cli.Context) error {
							return handleConfigGet()
						},
					},
					{
						Name:      "set",
						Usage:     "Set configuration value",
						ArgsUsage: "<key> <value>",
						Description: `Set a configuration value. Available keys:
  - anilist_username (string)
  - save_path (string)
  - completed_anime_path (string)
  - check_interval (int, in minutes)
  - qbittorrent_url (string)
  - max_episodes_per_anime (int)
  - episode_retry_limit (int)
  - delete_watched_episodes (bool: true/false)
  - excluded_list (string)`,
						Action: func(c *cli.Context) error {
							if c.NArg() != 2 {
								return fmt.Errorf("usage: config set <key> <value>\n\nAvailable keys:\n  - anilist_username\n  - save_path\n  - completed_anime_path\n  - check_interval\n  - qbittorrent_url\n  - max_episodes_per_anime\n  - episode_retry_limit\n  - delete_watched_episodes\n  - excluded_list")
							}
							return handleConfigSet(c.Args().Get(0), c.Args().Get(1))
						},
					},
				},
			},
			{
				Name:  "check",
				Usage: "Trigger manual verification",
				Action: func(c *cli.Context) error {
					return handleCheck()
				},
			},
			{
				Name:  "animes",
				Usage: "List downloaded animes",
				Action: func(c *cli.Context) error {
					return handleAnimes()
				},
			},
			{
				Name:  "episodes",
				Usage: "List downloaded episodes",
				Action: func(c *cli.Context) error {
					return handleEpisodes()
				},
			},
			{
				Name:  "logs",
				Usage: "View daemon logs",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "lines",
						Usage: "Number of lines to show",
						Value: 50,
					},
				},
				Action: func(c *cli.Context) error {
					return handleLogs(c.Int("lines"))
				},
			},
			{
				Name:  "open",
				Usage: "Open the web UI in the browser",
				Action: func(c *cli.Context) error {
					return handleOpen()
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getClient() *api.Client {
	return api.NewClient(apiEndpoint)
}

func handleStart() error {
	// Encontrar o binário do daemon
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	
	// Try different possible names for the daemon binary
	possibleNames := []string{
		"autoanimedownloader-daemon",
		"AutoAnimeDownloader-daemon",
		"aad-daemon",
	}
	
	var daemonBinary string
	for _, name := range possibleNames {
		candidate := filepath.Join(execDir, name)
		if _, err := os.Stat(candidate); err == nil {
			daemonBinary = candidate
			break
		}
	}
	
	if daemonBinary == "" {
		return fmt.Errorf("daemon binary not found in %s. Tried: %v", execDir, possibleNames)
	}

	if err := processcli.Start(daemonBinary); err != nil {
		return err
	}

	fmt.Println("Daemon started successfully")
	return nil
}

func handleStop() error {
	if err := processcli.Stop(); err != nil {
		return err
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

func handleLoopStart() error {
	client := getClient()
	if err := client.StartLoop(); err != nil {
		return fmt.Errorf("failed to start loop: %w", err)
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Loop started"})
	} else {
		fmt.Println("Loop started successfully")
	}
	return nil
}

func handleLoopStop() error {
	client := getClient()
	if err := client.StopLoop(); err != nil {
		return fmt.Errorf("failed to stop loop: %w", err)
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Loop stopped"})
	} else {
		fmt.Println("Loop stopped successfully")
	}
	return nil
}

func handleStatus() error {
	client := getClient()
	status, err := client.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if outputJSON {
		outputJSONResponse(status)
	} else {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Field", "Value"})
		t.AppendRow(table.Row{"Status", status.Status})
		t.AppendRow(table.Row{"Last Check", status.LastCheck.Format(time.RFC3339)})
		t.AppendRow(table.Row{"Has Error", status.HasError})
		t.Render()
	}
	return nil
}

func handleConfigGet() error {
	client := getClient()
	config, err := client.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if outputJSON {
		outputJSONResponse(config)
	} else {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Field", "Value"})
		t.AppendRow(table.Row{"Anilist Username", config.AnilistUsername})
		t.AppendRow(table.Row{"Save Path", config.SavePath})
		t.AppendRow(table.Row{"Completed Anime Path", config.CompletedAnimePath})
		t.AppendRow(table.Row{"Check Interval", fmt.Sprintf("%d minutes", config.CheckInterval)})
		t.AppendRow(table.Row{"qBittorrent URL", config.QBittorrentUrl})
		t.AppendRow(table.Row{"Max Episodes Per Anime", config.MaxEpisodesPerAnime})
		t.AppendRow(table.Row{"Episode Retry Limit", config.EpisodeRetryLimit})
		t.AppendRow(table.Row{"Delete Watched Episodes", config.DeleteWatchedEpisodes})
		t.AppendRow(table.Row{"Excluded List", config.ExcludedList})
		t.Render()
	}
	return nil
}

func handleConfigSet(key, value string) error {
	client := getClient()
	config, err := client.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get current config: %w", err)
	}

	// Atualizar o campo apropriado
	switch strings.ToLower(key) {
	case "anilist_username", "anilistusername":
		config.AnilistUsername = value
	case "save_path", "savepath":
		config.SavePath = value
	case "completed_anime_path", "completedanimepath":
		config.CompletedAnimePath = value
	case "check_interval", "checkinterval":
		var interval int
		if _, err := fmt.Sscanf(value, "%d", &interval); err != nil {
			return fmt.Errorf("invalid check interval: %w", err)
		}
		config.CheckInterval = interval
	case "qbittorrent_url", "qbittorrenturl":
		config.QBittorrentUrl = value
	case "max_episodes_per_anime", "maxepisodesperanime":
		var max int
		if _, err := fmt.Sscanf(value, "%d", &max); err != nil {
			return fmt.Errorf("invalid max episodes: %w", err)
		}
		config.MaxEpisodesPerAnime = max
	case "episode_retry_limit", "episoderetrylimit":
		var limit int
		if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
			return fmt.Errorf("invalid retry limit: %w", err)
		}
		config.EpisodeRetryLimit = limit
	case "delete_watched_episodes", "deletewatchedepisodes":
		config.DeleteWatchedEpisodes = strings.ToLower(value) == "true"
	case "excluded_list", "excludedlist":
		config.ExcludedList = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	if err := client.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Configuration updated"})
	} else {
		fmt.Printf("Configuration updated: %s = %s\n", key, value)
	}
	return nil
}

func handleCheck() error {
	client := getClient()
	if err := client.TriggerCheck(); err != nil {
		return fmt.Errorf("failed to trigger check: %w", err)
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Verification started"})
	} else {
		fmt.Println("Verification started")
	}
	return nil
}

func handleAnimes() error {
	client := getClient()
	animes, err := client.GetAnimes()
	if err != nil {
		return fmt.Errorf("failed to get animes: %w", err)
	}

	if outputJSON {
		outputJSONResponse(animes)
	} else {
		if len(animes) == 0 {
			fmt.Println("No animes found")
			return nil
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Name", "Episodes", "Latest Episode ID"})
		for _, anime := range animes {
			t.AppendRow(table.Row{anime.Name, anime.EpisodesCount, anime.LatestEpisodeID})
		}
		t.Render()
	}
	return nil
}

func handleEpisodes() error {
	client := getClient()
	episodes, err := client.GetEpisodes()
	if err != nil {
		return fmt.Errorf("failed to get episodes: %w", err)
	}

	if outputJSON {
		outputJSONResponse(episodes)
	} else {
		if len(episodes) == 0 {
			fmt.Println("No episodes found")
			return nil
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"ID", "Name", "Hash"})
		for _, episode := range episodes {
			t.AppendRow(table.Row{episode.EpisodeID, episode.EpisodeName, episode.EpisodeHash})
		}
		t.Render()
	}
	return nil
}

func handleLogs(lines int) error {
	// Try to get log path from initialized logger first
	logPath := logger.GetLogFilePath()

	// If not available, calculate expected path
	if logPath == "" {
		var err error
		logPath, err = logger.GetExpectedLogFilePath()
		if err != nil {
			return fmt.Errorf("failed to determine log file path: %w", err)
		}
	}

	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logPath)
	}

	// Ler as últimas N linhas do arquivo
	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func handleOpen() error {
	// Construir a URL da web UI a partir do endpoint da API
	webUIURL := apiEndpoint
	
	// Remover /api/v1 se presente, já que a web UI está na raiz
	webUIURL = strings.TrimSuffix(webUIURL, "/api/v1")
	webUIURL = strings.TrimSuffix(webUIURL, "/")
	
	// Garantir que tenha http:// ou https://
	if !strings.HasPrefix(webUIURL, "http://") && !strings.HasPrefix(webUIURL, "https://") {
		webUIURL = "http://" + webUIURL
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", webUIURL)
	case "darwin":
		cmd = exec.Command("open", webUIURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", webUIURL)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	if !outputJSON {
		fmt.Printf("Opening web UI at %s\n", webUIURL)
	} else {
		outputJSONResponse(map[string]string{
			"message": "Browser opened",
			"url":     webUIURL,
		})
	}

	return nil
}

func outputJSONResponse(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}
