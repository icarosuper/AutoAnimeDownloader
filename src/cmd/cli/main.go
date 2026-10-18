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
	"AutoAnimeDownloader/src/internal/version"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
)

var (
	outputJSON  bool
	verbose     bool
	apiEndpoint string
)

type cliError struct {
	exitCode int
	code     string
	message  string
}

func (e *cliError) Error() string { return e.message }
func (e *cliError) ExitCode() int { return e.exitCode }

func init() {
	// --verbose takes -v; version stays on --version (urfave's default -v would collide).
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Usage: "print the version"}
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		printCLIError(err)
		os.Exit(exitCodeOf(err))
	}
}

func newApp() *cli.App {
	return &cli.App{
		Name:    "autoanimedownloader",
		Usage:   "CLI for the Auto Anime Downloader daemon",
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
				Usage:       "Output the API data value as JSON",
				Destination: &outputJSON,
			},
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Usage:       "Verbose output",
				Destination: &verbose,
			},
		},
		Commands: []*cli.Command{
			startCommand(),
			stopCommand(),
			loopCommand(),
			openCommand(),
			statusCommand(),
			lastCheckCommand(),
			checkCommand(),
			configCommand(),
			prioritiesCommand(),
			logsCommand(),
			reportCommand(),
			animesCommand(),
			torrentsCommand(),
			notificationsCommand(),
		},
		// Tests (and main) inspect the returned error; the default handler would os.Exit.
		ExitErrHandler: func(*cli.Context, error) {},
	}
}

func startCommand() *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "Start the daemon process",
		Action: func(c *cli.Context) error {
			return handleStart()
		},
	}
}

func stopCommand() *cli.Command {
	return &cli.Command{
		Name:  "stop",
		Usage: "Stop the daemon process",
		Action: func(c *cli.Context) error {
			return handleStop()
		},
	}
}

func loopCommand() *cli.Command {
	return &cli.Command{
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
	}
}

func openCommand() *cli.Command {
	return &cli.Command{
		Name:  "open",
		Usage: "Open the web UI in the browser",
		Action: func(c *cli.Context) error {
			return handleOpen()
		},
	}
}

func getClient() *api.Client {
	return api.NewClient(apiEndpoint)
}

func printCLIError(err error) {
	ce := &cliError{exitCode: 1, message: err.Error()}
	var typed *cliError
	if errors.As(err, &typed) {
		ce = typed
	} else {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			ce.code = apiErr.Code
			ce.message = apiErr.Message
		}
	}
	if outputJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"error": map[string]string{
				"code":    ce.code,
				"message": ce.message,
			},
		})
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", ce.message)
}

func exitCodeOf(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.exitCode
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}

func mapClientError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return &cliError{exitCode: 1, code: apiErr.Code, message: apiErr.Message}
	}
	return &cliError{exitCode: 1, code: "DAEMON_UNREACHABLE", message: err.Error()}
}

func requireYes(c *cli.Context, extraFlags ...string) error {
	if c.Bool("yes") {
		return nil
	}
	msg := "pass --yes"
	if len(extraFlags) > 0 {
		msg += " (optional: " + strings.Join(extraFlags, " ") + ")"
	}
	return &cliError{
		exitCode: 2,
		code:     "CONFIRMATION_REQUIRED",
		message:  msg,
	}
}

func yesFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:  "yes",
		Usage: "Confirm this destructive action (required; there is no prompt)",
	}
}

func parsePositiveInt(s, name string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, s)
	}
	return n, nil
}

func requireArgs(c *cli.Context, n int, usage string) error {
	if c.NArg() != n {
		return fmt.Errorf("usage: %s", usage)
	}
	return nil
}

func handleStart() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	daemonBinary := filepath.Join(execDir, "autoanimedownloader-daemon")

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
	if err := mapClientError(getClient().StartLoop()); err != nil {
		return err
	}
	return printMessage("Loop started successfully", "Loop started")
}

func handleLoopStop() error {
	if err := mapClientError(getClient().StopLoop()); err != nil {
		return err
	}
	return printMessage("Loop stopped successfully", "Loop stopped")
}

func handleOpen() error {
	webUIURL := strings.TrimSuffix(apiEndpoint, "/api/v1")
	webUIURL = strings.TrimSuffix(webUIURL, "/")

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

	return printMessage("Opening web UI at "+webUIURL, "Browser opened")
}

func printMessage(human, jsonMessage string) error {
	if outputJSON {
		outputJSONResponse(map[string]string{"message": jsonMessage})
		return nil
	}
	fmt.Println(human)
	return nil
}

func outputJSONResponse(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

func printValue(v any, emptyMsg string) error {
	if outputJSON {
		outputJSONResponse(v)
		return nil
	}
	if v != nil && reflect.ValueOf(v).Kind() == reflect.Slice {
		return writeSliceTable(v, emptyMsg)
	}
	return writeKeyValueTable(v)
}

func writeKeyValueTable(v any) error {
	fields, err := asJSONMap(v)
	if err != nil {
		return err
	}
	keys := jsonFieldOrder(v)
	if len(keys) == 0 {
		for name := range fields {
			keys = append(keys, name)
		}
		sort.Strings(keys)
	}
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Field", "Value"})
	for _, key := range keys {
		t.AppendRow(table.Row{key, formatJSONValue(fields[key])})
	}
	t.Render()
	return nil
}

func writeSliceTable(rows any, emptyMsg string) error {
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println(emptyMsg)
		return nil
	}
	keys := jsonFieldOrder(rows)
	if len(keys) == 0 {
		for name := range list[0] {
			keys = append(keys, name)
		}
		sort.Strings(keys)
	}
	header := make(table.Row, len(keys))
	for i, key := range keys {
		header[i] = key
	}
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(header)
	for _, row := range list {
		cells := make(table.Row, len(keys))
		for i, key := range keys {
			cells[i] = formatJSONValue(row[key])
		}
		t.AppendRow(cells)
	}
	t.Render()
	return nil
}

func asJSONMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func jsonFieldOrder(v any) []string {
	t := reflect.TypeOf(v)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func formatJSONValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}
