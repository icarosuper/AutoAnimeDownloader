package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func statusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show daemon status (and the newer version, when the update check found one)",
		Action: func(c *cli.Context) error {
			return handleStatus()
		},
	}
}

func lastCheckCommand() *cli.Command {
	return &cli.Command{
		Name:  "last-check",
		Usage: "Show why the last automatic pass did not download episodes",
		Action: func(c *cli.Context) error {
			return handleLastCheck()
		},
	}
}

func checkCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Trigger a manual verification",
		Action: func(c *cli.Context) error {
			return handleCheck()
		},
	}
}

func handleStatus() error {
	status, err := getClient().GetStatus()
	if err != nil {
		return mapClientError(err)
	}
	if err := printValue(status, ""); err != nil {
		return err
	}
	// --json already carries latest_version / latest_version_url; the table gets a line of its own
	// so the update doesn't hide among the rows.
	if !outputJSON && status.LatestVersion != "" {
		fmt.Printf("Update available: v%s (%s)\n", status.LatestVersion, status.LatestVersionURL)
	}
	return nil
}

func handleLastCheck() error {
	report, err := getClient().GetLastCheck()
	if err != nil {
		return mapClientError(err)
	}
	return printValue(report, "")
}

func handleCheck() error {
	if err := mapClientError(getClient().TriggerCheck()); err != nil {
		return err
	}
	return printMessage("Verification started", "Verification started")
}
