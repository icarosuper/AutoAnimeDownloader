package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"AutoAnimeDownloader/src/internal/files"

	"github.com/urfave/cli/v2"
)

func configCommand() *cli.Command {
	return &cli.Command{
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
					"A dotted key reaches into a nested object (notifications.batch_window_seconds,\n" +
					"priorities.fansubs) and leaves its other fields untouched.\n" +
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
	}
}

func prioritiesCommand() *cli.Command {
	return &cli.Command{
		Name:  "priorities",
		Usage: "Priority list helpers",
		Subcommands: []*cli.Command{
			{
				Name:  "defaults",
				Usage: "Show the default torrent priority lists",
				Action: func(c *cli.Context) error {
					return handlePriorityDefaults()
				},
			},
		},
	}
}

func handleConfigGet() error {
	config, err := getClient().GetConfig()
	if err != nil {
		return mapClientError(err)
	}
	return printValue(config, "")
}

// handleConfigSet patches ONE key of config.json by its JSON name, without the CLI knowing the
// field list: it round-trips the config through a map, so every key the daemon serves is settable
// and a new field needs no change here. The old hand-written switch covered 7 of the 25 keys and
// answered "unknown config key" for the rest, which reads as "the field does not exist".
//
// The saved value is also the type oracle: a key that already holds a list accepts the
// comma-separated form, everything else is JSON-parsed and falls back to a plain string.
//
// A dotted key (`notifications.batch_window_seconds`) patches one field of a nested object. Without
// it, changing one field meant passing the whole object, and leaving a sibling out — the webhooks
// next to batch_window_seconds — erased it.
func handleConfigSet(key, value string) error {
	client := getClient()
	config, err := client.GetConfig()
	if err != nil {
		return mapClientError(err)
	}

	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to encode current config: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("failed to decode current config: %w", err)
	}

	parent, name, err := resolveConfigKey(fields, key)
	if err != nil {
		return err
	}
	parent[name] = parseConfigValue(parent[name], value)

	patched, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	var updated files.Config
	if err := json.Unmarshal(patched, &updated); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	if err := mapClientError(client.UpdateConfig(&updated)); err != nil {
		return err
	}

	if outputJSON {
		outputJSONResponse(map[string]string{"message": "Configuration updated"})
		return nil
	}
	fmt.Printf("Configuration updated: %s = %s\n", key, value)
	return nil
}

func handlePriorityDefaults() error {
	priorities, err := getClient().GetPriorityDefaults()
	if err != nil {
		return mapClientError(err)
	}
	return printValue(priorities, "")
}

// resolveConfigKey walks a dotted key into fields and returns the object holding the last segment
// plus that segment's real JSON name, so the caller writes the value in place. Every segment is
// matched like a top-level key (matchConfigKey); a segment that isn't there, or a dot after a value
// that isn't an object, is an unknown key.
func resolveConfigKey(fields map[string]any, key string) (map[string]any, string, error) {
	segments := strings.Split(key, ".")
	parent := fields
	for i, segment := range segments {
		name, ok := matchConfigKey(parent, segment)
		if !ok {
			return nil, "", fmt.Errorf("unknown config key: %s (run `config get` to see the keys)", key)
		}
		if i == len(segments)-1 {
			return parent, name, nil
		}
		child, ok := parent[name].(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("unknown config key: %s (%s is not an object)", key, strings.Join(segments[:i+1], "."))
		}
		parent = child
	}
	return nil, "", fmt.Errorf("unknown config key: %s", key)
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
