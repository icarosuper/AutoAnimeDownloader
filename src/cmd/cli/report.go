package main

import (
	"fmt"
	"os"

	"AutoAnimeDownloader/src/internal/datadir"
	"AutoAnimeDownloader/src/internal/diagnostics"

	"github.com/urfave/cli/v2"
)

func reportCommand() *cli.Command {
	return &cli.Command{
		Name:  "report",
		Usage: "Build a diagnostics bundle from the data dir (the daemon may be dead)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "output", Usage: "Write the bundle to PATH instead of stdout"},
			&cli.BoolFlag{Name: "no-titles", Usage: "Redact anime titles in the bundle"},
			&cli.StringFlag{Name: "error", Usage: "Error text to include as context"},
			&cli.StringFlag{Name: "context", Usage: "Request or screen context to include"},
			&cli.StringFlag{Name: "anime", Usage: "Anime id or name to include as context"},
		},
		Action: handleReport,
		Subcommands: []*cli.Command{
			{
				Name:  "crash-ack",
				Usage: "Archive the pending crash file",
				Action: func(c *cli.Context) error {
					return handleReportCrashAck()
				},
			},
		},
	}
}

func handleReport(c *cli.Context) error {
	dir, err := datadir.Path()
	if err != nil {
		return err
	}
	report := diagnostics.Build(dir, diagnostics.Options{
		Titles:  !c.Bool("no-titles"),
		Error:   c.String("error"),
		Context: c.String("context"),
		AnimeID: c.String("anime"),
	})
	if outputJSON {
		outputJSONResponse(report)
		return maybeWriteBundle(c.String("output"), report.Bundle)
	}
	if output := c.String("output"); output != "" {
		if err := os.WriteFile(output, []byte(report.Bundle), 0644); err != nil {
			return fmt.Errorf("failed to write report: %w", err)
		}
		fmt.Printf("Wrote report to %s\n", output)
		return nil
	}
	fmt.Print(report.Bundle)
	return nil
}

func maybeWriteBundle(output, bundle string) error {
	if output == "" {
		return nil
	}
	if err := os.WriteFile(output, []byte(bundle), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}
	return nil
}

func handleReportCrashAck() error {
	dir, err := datadir.Path()
	if err != nil {
		return err
	}
	if err := diagnostics.ArchivePendingCrash(dir); err != nil {
		return err
	}
	return printMessage("Pending crash archived", "Pending crash archived")
}
