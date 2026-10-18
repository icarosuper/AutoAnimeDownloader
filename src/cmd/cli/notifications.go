package main

import (
	"github.com/urfave/cli/v2"
)

func notificationsCommand() *cli.Command {
	return &cli.Command{
		Name:  "notifications",
		Usage: "Notification helpers",
		Subcommands: []*cli.Command{
			{
				Name:      "test",
				Usage:     "Fire a test payload to a named webhook",
				ArgsUsage: "<name>",
				Action:    handleNotificationsTest,
			},
		},
	}
}

func handleNotificationsTest(c *cli.Context) error {
	if err := requireArgs(c, 1, "notifications test <name>"); err != nil {
		return err
	}
	if err := mapClientError(getClient().TestWebhook(c.Args().Get(0))); err != nil {
		return err
	}
	return printMessage("Webhook fired successfully", "Webhook fired successfully")
}
