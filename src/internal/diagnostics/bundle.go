// Package diagnostics builds the bug report bundle. It reads everything from files in the data dir
// and never asks the running daemon: that's what lets a CLI command build the same bundle with the
// daemon dead (decisions.md #102).
package diagnostics

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	configFileName        = "config.json"
	animeSettingsFileName = "anime_settings"
	logFileName           = "daemon.log"
	logLinesInBundle      = 1000
)

// Options is what the report screen passes through: the titles checkbox and the context of the
// error toast that opened it.
type Options struct {
	Titles  bool
	Error   string
	Context string
	AnimeID string
}

type Report struct {
	Bundle string `json:"bundle"`
	// Environment is the short summary that goes in the issue URL.
	Environment string `json:"environment"`
}

// rawConfig is the part of config.json the bundle needs to understand. The config itself goes into
// the bundle as a generic map, so a field this struct doesn't know still shows up.
type rawConfig struct {
	CompletedAnimePath string   `json:"completed_anime_path"`
	MinFreeDiskPercent int      `json:"min_free_disk_percent"`
	AnilistUsername    string   `json:"anilist_username"`
	AnilistUsernames   []string `json:"anilist_usernames"`
	MALUsernames       []string `json:"mal_usernames"`
	Notifications      struct {
		Webhooks []rawWebhook `json:"webhooks"`
	} `json:"notifications"`
}

type rawWebhook struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// Build assembles the bundle from the files in dir.
func Build(dir string, opts Options) Report {
	configData, _ := os.ReadFile(filepath.Join(dir, configFileName))
	var cfg rawConfig
	_ = json.Unmarshal(configData, &cfg)

	home, _ := os.UserHomeDir()
	r := newRedactor(opts.Titles, home, configUsernames(cfg), webhookSecrets(cfg))

	env := environmentLines(realHost)
	var b strings.Builder

	section(&b, "Environment")
	for _, line := range env {
		b.WriteString(line + "\n")
	}
	b.WriteString(diskLine(cfg.CompletedAnimePath, cfg.MinFreeDiskPercent) + "\n")

	if opts.Error != "" || opts.Context != "" || opts.AnimeID != "" {
		section(&b, "Context")
		errorText, request, animeID := r.scrub(opts.Error), r.scrub(opts.Context), ""
		if opts.AnimeID != "" {
			animeID = r.value(title, opts.AnimeID).(string)
			// The request is "GET /animes/<id>/episodes 500": the id would survive the titles switch.
			errorText = strings.ReplaceAll(errorText, opts.AnimeID, animeID)
			request = strings.ReplaceAll(request, opts.AnimeID, animeID)
		}
		writeField(&b, "Error", errorText)
		writeField(&b, "Request", request)
		writeField(&b, "Anime media ID", animeID)
	}

	if crash := readPendingCrash(dir); crash != "" {
		section(&b, "Crash")
		b.WriteString(r.scrub(crash))
	}

	section(&b, "Config")
	b.WriteString(redactConfig(r, configData) + "\n")

	section(&b, "Anime settings")
	b.WriteString(redactAnimeSettings(r, dir) + "\n")

	section(&b, "Log")
	for _, line := range lastLogLines(dir, logLinesInBundle) {
		if redacted, ok := r.logLine(line); ok {
			b.WriteString(redacted + "\n")
		}
	}

	return Report{Bundle: b.String(), Environment: strings.Join(env, "\n")}
}

func section(b *strings.Builder, name string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("## " + name + "\n\n")
}

func writeField(b *strings.Builder, name, value string) {
	if value != "" {
		b.WriteString(name + ": " + value + "\n")
	}
}

func configUsernames(cfg rawConfig) []string {
	names := append([]string{cfg.AnilistUsername}, cfg.AnilistUsernames...)
	return append(names, cfg.MALUsernames...)
}

// webhookSecrets are the configured webhook URLs and header values. A URL with {{vars}} is matched
// by its static prefix, and only when that prefix still names a host: "https://" alone would match
// every URL in the log.
func webhookSecrets(cfg rawConfig) []string {
	var secrets []string
	for _, w := range cfg.Notifications.Webhooks {
		prefix, _, _ := strings.Cut(w.URL, "{{")
		if u, err := url.Parse(prefix); err == nil && u.Host != "" {
			secrets = append(secrets, prefix)
		}
		for _, v := range w.Headers {
			secrets = append(secrets, v)
		}
	}
	// Longest first, so a URL isn't half-replaced by a shorter secret it contains.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func redactConfig(r *redactor, data []byte) string {
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "(no readable config.json)"
	}
	for key, v := range cfg {
		switch key {
		case "anilist_username", "anilist_usernames", "mal_usernames":
			cfg[key] = r.value(pseudonymize, v)
		default:
			cfg[key] = r.value(keep, v)
		}
	}
	// The name and method stay: the name is a user-chosen label, and it's what the log's "webhook"
	// key refers to.
	if notifications, ok := cfg["notifications"].(map[string]any); ok {
		if webhooks, ok := notifications["webhooks"].([]any); ok {
			for _, w := range webhooks {
				if webhook, ok := w.(map[string]any); ok {
					for _, secret := range []string{"url", "headers", "body"} {
						webhook[secret] = "<redacted>"
					}
				}
			}
		}
	}
	return indentJSON(cfg)
}

// redactAnimeSettings: keyed by media id, which is a title by lookup.
func redactAnimeSettings(r *redactor, dir string) string {
	var settings map[string]any
	data, err := os.ReadFile(filepath.Join(dir, animeSettingsFileName))
	if err != nil || json.Unmarshal(data, &settings) != nil {
		return "(none)"
	}
	out := make(map[string]any, len(settings))
	for id, v := range settings {
		out[r.value(title, id).(string)] = v
	}
	return indentJSON(out)
}

func indentJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

// lastLogLines returns the last n lines worth reading (HTTP request lines don't count). Right after a
// rotation the active file is short, so the rest comes from the newest compressed backup.
func lastLogLines(dir string, n int) []string {
	lines := readLogLines(filepath.Join(dir, logFileName), nil)
	if len(lines) < n {
		if backup := newestLogBackup(dir); backup != "" {
			lines = append(readLogLines(backup, gzip.NewReader), lines...)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func readLogLines(path string, decompress func(io.Reader) (*gzip.Reader, error)) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var src io.Reader = f
	if decompress != nil {
		gz, err := decompress(f)
		if err != nil {
			return nil
		}
		defer gz.Close()
		src = gz
	}

	var lines []string
	scanner := bufio.NewScanner(src)
	// A stack trace or an AniList response makes lines far longer than the 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" && !isHTTPRequestLine(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// newestLogBackup finds lumberjack's most recent backup (daemon-<timestamp>.log.gz; the timestamp
// sorts chronologically).
func newestLogBackup(dir string) string {
	backups, _ := filepath.Glob(filepath.Join(dir, "daemon-*.log.gz"))
	if len(backups) == 0 {
		return ""
	}
	sort.Strings(backups)
	return backups[len(backups)-1]
}
