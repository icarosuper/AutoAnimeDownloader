package main

import (
	"github.com/urfave/cli/v2"
)

func statusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show daemon status",
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
	return printValue(status, "")
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
