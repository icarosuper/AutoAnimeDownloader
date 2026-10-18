package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"AutoAnimeDownloader/src/internal/logger"

	"github.com/urfave/cli/v2"
)

func logsCommand() *cli.Command {
	return &cli.Command{
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
	}
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
