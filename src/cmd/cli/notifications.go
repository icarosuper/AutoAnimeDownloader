package main

import (
	"fmt"

	"AutoAnimeDownloader/src/internal/files"

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
			{
				Name:      "preview",
				Usage:     "Show what a named webhook would send, without sending it",
				ArgsUsage: "<name>",
				Description: "Renders the saved preset with the same sample values as `notifications test`\n" +
					"(method, URL, headers and body after {{variable}} substitution).",
				Action: handleNotificationsPreview,
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

func handleNotificationsPreview(c *cli.Context) error {
	if err := requireArgs(c, 1, "notifications preview <name>"); err != nil {
		return err
	}
	name := c.Args().Get(0)
	client := getClient()
	config, err := client.GetConfig()
	if err != nil {
		return mapClientError(err)
	}
	preset, ok := findWebhook(config.Notifications.Webhooks, name)
	if !ok {
		return fmt.Errorf("webhook not found: %q", name)
	}
	rendered, err := client.PreviewWebhook(preset)
	if err != nil {
		return mapClientError(err)
	}
	return printValue(rendered, "")
}

func findWebhook(webhooks []files.WebhookPreset, name string) (files.WebhookPreset, bool) {
	for _, w := range webhooks {
		if w.Name == name {
			return w, true
		}
	}
	return files.WebhookPreset{}, false
}
