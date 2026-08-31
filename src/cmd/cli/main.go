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
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/version"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
		Name:    "AutoAnimeDownloader",
		Usage:   "CLI for Auto Anime Downloader daemon",
		Version: version.Version,
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
						Description: "Set any key of config.json by its JSON name — `config get` lists them all.\n" +
							"Value is parsed as JSON when it parses (3, true, [\"a\",\"b\"]); otherwise it is\n" +
							"taken as a string. A comma-separated value is split into a list when the key\n" +
							"already holds one.",
						Action: func(c *cli.Context) error {
							if c.NArg() != 2 {
								return fmt.Errorf("usage: config set <key> <value>  (run `config get` to see the keys)")
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
						Name:    "lines",
						Aliases: []string{"n"},
						Usage:   "Number of lines to load",
						Value:   1000,
					},
					&cli.StringFlag{
						Name:    "level",
						Aliases: []string{"l"},
						Usage:   "Filter by level: all, debug, info, warn, error",
						Value:   "all",
					},
					&cli.StringFlag{
						Name:    "search",
						Aliases: []string{"q"},
						Usage:   "Filter lines containing text",
					},
				},
				Action: func(c *cli.Context) error {
					return handleLogs(c.Int("lines"), c.String("level"), c.String("search"))
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
	daemonBinary := filepath.Join(execDir, "autoanimedownloader-daemon")

	// Se não existir, tentar sem extensão (Linux/Mac)
	if _, err := os.Stat(daemonBinary); os.IsNotExist(err) {
		daemonBinary = filepath.Join(execDir, "aad-daemon")
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
		t.AppendRow(table.Row{"Version", status.Version})
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
		t.AppendRow(table.Row{"Anilist Usernames", strings.Join(config.AnilistUsernames, ", ")})
		t.AppendRow(table.Row{"Completed Anime Path", config.CompletedAnimePath})
		t.AppendRow(table.Row{"Check Interval", fmt.Sprintf("%d minutes", config.CheckInterval)})
		t.AppendRow(table.Row{"Max Episodes Per Anime", config.MaxEpisodesPerAnime})
		t.AppendRow(table.Row{"Episode Retry Limit", config.EpisodeRetryLimit})
		t.AppendRow(table.Row{"Delete Watched Episodes", config.DeleteWatchedEpisodes})
		t.AppendRow(table.Row{"Excluded Lists", strings.Join(config.ExcludedLists, ", ")})
		t.Render()
	}
	return nil
}

// handleConfigSet patches ONE key of config.json by its JSON name, without the CLI knowing the
// field list: it round-trips the config through a map, so every key the daemon serves is settable
// and a new field needs no change here. The old hand-written switch covered 7 of the 25 keys and
// answered "unknown config key" for the rest, which reads as "the field does not exist".
//
// The saved value is also the type oracle: a key that already holds a list accepts the
// comma-separated form, everything else is JSON-parsed and falls back to a plain string.
func handleConfigSet(key, value string) error {
	client := getClient()
	config, err := client.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get current config: %w", err)
	}

	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to encode current config: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("failed to decode current config: %w", err)
	}

	name, ok := matchConfigKey(fields, key)
	if !ok {
		return fmt.Errorf("unknown config key: %s (run `config get` to see the keys)", key)
	}
	fields[name] = parseConfigValue(fields[name], value)

	patched, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	var updated files.Config
	if err := json.Unmarshal(patched, &updated); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	if err := client.UpdateConfig(&updated); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Configuration updated"})
	} else {
		fmt.Printf("Configuration updated: %s = %s\n", name, value)
	}
	return nil
}

// matchConfigKey resolves a user-typed key to the real JSON name, ignoring case and underscores so
// `maxSearchPages`, `max_search_pages` and `MAX_SEARCH_PAGES` all land on the same field.
func matchConfigKey(fields map[string]any, key string) (string, bool) {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
	want := norm(key)
	for name := range fields {
		if norm(name) == want {
			return name, true
		}
	}
	return "", false
}

// parseConfigValue turns the raw CLI string into the JSON value the field expects. current is the
// value already saved under that key and is used only to recognise a list.
func parseConfigValue(current any, value string) any {
	if _, isList := current.([]any); isList && !strings.HasPrefix(strings.TrimSpace(value), "[") {
		items := []any{}
		for _, item := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	}

	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err == nil {
		return parsed
	}
	return value
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
			t.AppendRow(table.Row{anime.Name, anime.EpisodesDownloaded, anime.LatestEpisodeNumber})
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
		t.AppendHeader(table.Row{"Anime", "Ep", "Name", "Hash"})
		for _, episode := range episodes {
			t.AppendRow(table.Row{episode.AnimeID, episode.EpisodeNumber, episode.EpisodeName, episode.EpisodeHash})
		}
		t.Render()
	}
	return nil
}

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiDim    = "\033[2m"
	ansiBold   = "\033[1m"
)

type parsedLogLine struct {
	level     string
	timestamp string
	message   string
	extras    string
	raw       string
}

func parseLog(line string) parsedLogLine {
	if len(line) > 0 && line[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			var level, timestamp, message string
			if v, ok := obj["level"]; ok {
				_ = json.Unmarshal(v, &level)
			}
			if v, ok := obj["time"]; ok {
				_ = json.Unmarshal(v, &timestamp)
			}
			if v, ok := obj["message"]; ok {
				_ = json.Unmarshal(v, &message)
			}
			if level == "" {
				level = "info"
			}
			var extParts []string
			for k, v := range obj {
				if k == "level" || k == "time" || k == "message" {
					continue
				}
				var s string
				if err := json.Unmarshal(v, &s); err != nil {
					s = string(v)
				}
				extParts = append(extParts, fmt.Sprintf(`"%s"="%s"`, k, s))
			}
			sort.Strings(extParts)
			return parsedLogLine{
				level:     level,
				timestamp: timestamp,
				message:   message,
				extras:    strings.Join(extParts, " "),
				raw:       line,
			}
		}
	}

	abbrs := map[string]string{"DBG": "debug", "INF": "info", "WRN": "warn", "ERR": "error", "FAT": "error"}
	for abbr, lvl := range abbrs {
		if strings.Contains(line, abbr) {
			return parsedLogLine{level: lvl, message: line, raw: line}
		}
	}

	lower := strings.ToLower(line)
	level := "info"
	if strings.Contains(lower, "error") || strings.Contains(lower, "err") {
		level = "error"
	} else if strings.Contains(lower, "warn") {
		level = "warn"
	} else if strings.Contains(lower, "debug") || strings.Contains(lower, "dbg") {
		level = "debug"
	}
	return parsedLogLine{level: level, message: line, raw: line}
}

func levelAbbr(level string) string {
	switch level {
	case "debug":
		return "DEBU"
	case "info":
		return "INFO"
	case "warn":
		return "WARN"
	case "error":
		return "ERRO"
	default:
		if len(level) >= 4 {
			return strings.ToUpper(level[:4])
		}
		return strings.ToUpper(level)
	}
}

func levelAnsi(level string) string {
	switch level {
	case "error":
		return ansiRed
	case "warn":
		return ansiYellow
	case "debug":
		return ansiDim
	default:
		return ansiCyan
	}
}

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func handleLogs(lines int, filterLevel, searchQuery string) error {
	logPath := logger.GetLogFilePath()
	if logPath == "" {
		var err error
		logPath, err = logger.GetExpectedLogFilePath()
		if err != nil {
			return fmt.Errorf("failed to determine log file path: %w", err)
		}
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logPath)
	}

	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	total := len(allLines)
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}

	searchLower := strings.ToLower(strings.TrimSpace(searchQuery))
	var filtered []parsedLogLine
	for _, line := range allLines {
		if line == "" {
			continue
		}
		p := parseLog(line)
		if filterLevel != "all" && filterLevel != "" && p.level != filterLevel {
			continue
		}
		if searchLower != "" && !strings.Contains(strings.ToLower(line), searchLower) {
			continue
		}
		filtered = append(filtered, p)
	}

	if outputJSON {
		rawLines := make([]string, len(filtered))
		for i, p := range filtered {
			rawLines[i] = p.raw
		}
		outputJSONResponse(map[string]interface{}{
			"lines": rawLines,
			"total": total,
			"shown": len(filtered),
		})
		return nil
	}

	color := useColor()
	for _, p := range filtered {
		var sb strings.Builder
		if color {
			sb.WriteString(levelAnsi(p.level))
			sb.WriteString(ansiBold)
		}
		sb.WriteString(levelAbbr(p.level))
		if color {
			sb.WriteString(ansiReset)
		}
		sb.WriteString(" ")
		if p.timestamp != "" {
			if color {
				sb.WriteString(ansiDim)
			}
			sb.WriteString(p.timestamp)
			if color {
				sb.WriteString(ansiReset)
			}
			sb.WriteString(" ")
		}
		sb.WriteString(p.message)
		if p.extras != "" {
			if color {
				sb.WriteString(ansiDim)
			}
			sb.WriteString(" ")
			sb.WriteString(p.extras)
			if color {
				sb.WriteString(ansiReset)
			}
		}
		fmt.Println(sb.String())
	}

	isFiltered := (filterLevel != "all" && filterLevel != "") || searchLower != ""
	suffix := ""
	if isFiltered {
		suffix = " (filtered)"
	}
	fmt.Fprintf(os.Stderr, "%d of %d lines%s\n", len(filtered), total, suffix)

	return nil
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
