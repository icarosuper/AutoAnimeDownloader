package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type Event int

const (
	NewEpisode Event = iota
	DownloadFailed
	DownloadCompleted
	EpisodeDeleted
)

// Download-failure reasons, used as {{reason}} and in the default message.
const (
	ReasonNotFound         = "no torrent found"
	ReasonDownloadRejected = "torrent rejected"
	ReasonNoDiskSpace      = "insufficient disk space"
)

var reVar = regexp.MustCompile(`\{\{(\w+)\}\}`)

func eventString(e Event) string {
	switch e {
	case NewEpisode:
		return "new_episode"
	case DownloadFailed:
		return "download_failed"
	case DownloadCompleted:
		return "download_completed"
	case EpisodeDeleted:
		return "episode_deleted"
	}
	return ""
}

func interpolate(template string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(template, func(match string) string {
		key := reVar.FindStringSubmatch(match)[1]
		return vars[key]
	})
}

// Episode is what one notification is about. AnimeID and TorrentHash are optional — zero when the
// caller does not have them (a "no torrent found" failure has no hash) — and reach the templates
// as {{anime_id}} / {{torrent_hash}}, blank when unknown.
//
// Last turns the item into a run Number..Last of one torrent: a batch pack is ONE event (one push,
// one Jellyfin scan), not N. Zero (or equal to Number) is a single episode.
type Episode struct {
	AnimeID     int
	AnimeName   string
	Number      int
	Last        int
	Reason      string
	TorrentHash string
}

func (e Episode) last() int {
	if e.Last > e.Number {
		return e.Last
	}
	return e.Number
}

// Runs splits the episode numbers of one anime (and one torrent) into items of consecutive
// episodes, each a copy of base with Number..Last set. A pack whose records skip a watched episode
// in the middle becomes two runs instead of a range that claims the gap.
func Runs(base Episode, numbers []int) []Episode {
	numbers = slices.Clone(numbers)
	slices.Sort(numbers)
	numbers = slices.Compact(numbers)
	var runs []Episode
	for i, n := range numbers {
		if i > 0 && n == numbers[i-1]+1 {
			runs[len(runs)-1].Last = n
			continue
		}
		run := base
		run.Number, run.Last = n, n
		runs = append(runs, run)
	}
	return runs
}

func episodeCount(items []Episode) int {
	total := 0
	for _, it := range items {
		total += it.last() - it.Number + 1
	}
	return total
}

// buildBatchVars builds the template variables for the items of a window. With a single item the
// result is identical to before batching — which is why turning batching on does not change the
// look of notifications for anyone who still receives them one at a time.
//
// With N > 1, `anime_name`, `episode`, `reason`, `anime_id` and `torrent_hash` stay EMPTY: there
// is no single value for them and sending the first item's would make the template lie about the
// other N-1. Whoever needs to identify the episodes uses `message` or `count`.
func buildBatchVars(event Event, items []Episode) map[string]string {
	vars := map[string]string{
		"title":        batchTitle(event, items),
		"message":      strings.Join(batchLines(event, items), "\n"),
		"anime_name":   "",
		"episode":      "",
		"reason":       "",
		"anime_id":     "",
		"torrent_hash": "",
		"count":        fmt.Sprintf("%d", episodeCount(items)),
		"quality":      "",
		"file_path":    "",
		"timestamp":    time.Now().Format("2006-01-02 15:04"),
	}

	if len(items) == 1 {
		vars["anime_name"] = items[0].AnimeName
		vars["episode"] = fmt.Sprintf("%d", items[0].Number)
		if last := items[0].last(); last != items[0].Number {
			vars["episode"] = fmt.Sprintf("%d-%d", items[0].Number, last)
		}
		vars["reason"] = items[0].Reason
		if items[0].AnimeID != 0 {
			vars["anime_id"] = fmt.Sprintf("%d", items[0].AnimeID)
		}
		vars["torrent_hash"] = items[0].TorrentHash
	}

	return vars
}

// batchLines is {{message}}: one line per run of consecutive episodes of the same anime with the
// same reason, so a 24-episode pack completing is "Frieren EPs 1-24 downloaded successfully"
// instead of 24 lines. Episodes are sorted inside each anime before collapsing — organize finishes
// torrents in whatever order they complete, and 3, 1, 2 is still one run. Animes keep the order
// they first appeared in the window. The reason is part of the key: merging two failures with
// different reasons into one line would report only one of them.
func batchLines(event Event, items []Episode) []string {
	type groupKey struct {
		animeID int
		name    string // only when there is no id: an id already identifies the anime
		reason  string
	}
	var order []groupKey
	groups := make(map[groupKey][]Episode)
	for _, it := range items {
		key := groupKey{animeID: it.AnimeID, reason: it.Reason}
		if it.AnimeID == 0 {
			key.name = it.AnimeName
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], it)
	}

	lines := make([]string, 0, len(items))
	for _, key := range order {
		group := groups[key]
		var numbers []int
		for _, it := range group {
			for n := it.Number; n <= it.last(); n++ {
				numbers = append(numbers, n)
			}
		}
		slices.Sort(numbers)
		numbers = slices.Compact(numbers)

		first := group[0]
		start := numbers[0]
		for i := 1; i <= len(numbers); i++ {
			if i < len(numbers) && numbers[i] == numbers[i-1]+1 {
				continue
			}
			_, message := eventStrings(first.AnimeName, start, numbers[i-1], event, first.Reason)
			lines = append(lines, message)
			if i < len(numbers) {
				start = numbers[i]
			}
		}
	}
	return lines
}

func batchTitle(event Event, items []Episode) string {
	if len(items) == 1 {
		title, _ := eventStrings(items[0].AnimeName, items[0].Number, items[0].last(), event, items[0].Reason)
		return title
	}
	// Counts episodes, not items: one item can be a whole pack.
	count := episodeCount(items)
	switch event {
	case NewEpisode:
		return fmt.Sprintf("%d new episodes detected", count)
	case DownloadFailed:
		return fmt.Sprintf("%d episodes failed to download", count)
	case DownloadCompleted:
		return fmt.Sprintf("%d episodes downloaded", count)
	case EpisodeDeleted:
		return fmt.Sprintf("%d episodes deleted", count)
	}
	return ""
}

// eventStrings returns the title and the message line for episodes first..last of one anime;
// first == last is a single episode.
func eventStrings(animeName string, first, last int, event Event, reason string) (title, message string) {
	episodes := fmt.Sprintf("EP %d", first)
	if last != first {
		episodes = fmt.Sprintf("EPs %d-%d", first, last)
	}
	switch event {
	case NewEpisode:
		if last != first {
			return "New episodes detected",
				fmt.Sprintf("%s %s detected, starting download", animeName, episodes)
		}
		return "New episode detected",
			fmt.Sprintf("%s %s detected, starting download", animeName, episodes)
	case DownloadFailed:
		if reason == "" {
			reason = "all attempts failed"
		}
		return "Download failed",
			fmt.Sprintf("%s %s failed: %s", animeName, episodes, reason)
	case DownloadCompleted:
		return "Download completed",
			fmt.Sprintf("%s %s downloaded successfully", animeName, episodes)
	case EpisodeDeleted:
		verb := "was"
		if last != first {
			verb = "were"
		}
		title := "Episode deleted"
		if last != first {
			title = "Episodes deleted"
		}
		return title,
			fmt.Sprintf("%s %s %s removed from the library", animeName, episodes, verb)
	}
	return "", ""
}

// jsonEscape returns the value ready to be pasted INSIDE a JSON string (without the outer
// quotes). An anime name with quotes, a backslash, or the batching \n would break the body of
// presets that embed {{message}} in JSON (Discord, Slack, Telegram, Gotify, Pushover, Apprise):
// the service would answer 400 and the notification would vanish with no explanation in the log.
func jsonEscape(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil { // json.Marshal of a string does not fail; if it did, the raw value beats nothing
		return value
	}
	return string(encoded[1 : len(encoded)-1])
}

// escapeVarsForJSON applies jsonEscape to every variable. Used only on the body, and only when
// the preset declares itself JSON: the URL has its own escaping rules and a plain-text body
// (ntfy's is raw `{{message}}`) must receive a real \n.
func escapeVarsForJSON(vars map[string]string) map[string]string {
	escaped := make(map[string]string, len(vars))
	for k, v := range vars {
		escaped[k] = jsonEscape(v)
	}
	return escaped
}

func presetIsJSON(preset files.WebhookPreset) bool {
	for k, v := range preset.Headers {
		if strings.EqualFold(k, "Content-Type") && strings.Contains(strings.ToLower(v), "json") {
			return true
		}
	}
	return false
}

// RenderedWebhook is a preset with its variables filled in: exactly what fireWebhook sends, and
// what PreviewWebhook shows.
type RenderedWebhook struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func renderWebhook(preset files.WebhookPreset, vars map[string]string) RenderedWebhook {
	bodyVars := vars
	if presetIsJSON(preset) {
		bodyVars = escapeVarsForJSON(vars)
	}
	headers := make(map[string]string, len(preset.Headers))
	for k, v := range preset.Headers {
		// A header with \n makes net/http reject the whole request. ntfy uses
		// `Title: {{title}}`, and nothing stops someone from putting {{message}} in a header.
		headers[k] = strings.NewReplacer("\n", " ", "\r", " ").Replace(interpolate(v, vars))
	}
	return RenderedWebhook{
		Method:  preset.Method,
		URL:     interpolate(preset.URL, vars),
		Headers: headers,
		Body:    interpolate(preset.Body, bodyVars),
	}
}

// PreviewWebhook renders a preset with the same sample variables as the test fire, without sending
// anything. The preset does not have to be saved: the UI previews the form as it is being edited.
func PreviewWebhook(preset files.WebhookPreset) RenderedWebhook {
	return renderWebhook(preset, buildBatchVars(DownloadCompleted, []Episode{sampleEpisode}))
}

// fireWebhook fires a preset and returns what went wrong. Automatic delivery ignores the
// return (the log already records it); the "Test" button needs it, or it would report success
// while the service refused the token — see decisions.md #86.
func fireWebhook(preset files.WebhookPreset, vars map[string]string) error {
	rendered := renderWebhook(preset, vars)

	req, err := http.NewRequest(rendered.Method, rendered.URL, strings.NewReader(rendered.Body))
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Failed to build webhook request")
		return err
	}
	for k, v := range rendered.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Webhook request failed")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Logger.Warn().
			Str("webhook", preset.Name).
			Int("status", resp.StatusCode).
			Msg("Webhook returned error status")
		return fmt.Errorf("webhook %q returned status %d", preset.Name, resp.StatusCode)
	}
	return nil
}

// SourceStandalone is the WebhookPreset.AnimeSources key of the standalone animes.
const SourceStandalone = "standalone"

// AccountSource is the WebhookPreset.AnimeSources key of a list account — the same "<source>:<user>"
// the dashboard user filter puts in its URL, so one account is spelled the same everywhere.
func AccountSource(source, username string) string {
	return source + ":" + username
}

// SourceResolver answers, for each anime id, its sources: the keys of the configured accounts
// that have it on their list right now (any list status) plus SourceStandalone when it is tracked
// as standalone. An id missing from the map is UNKNOWN — a webhook filtered by source then skips
// it, it can't prove it belongs. It is resolved at fire time, not stored on the episode record, so
// records written before the filter existed (and deletions of long-finished animes) work too.
type SourceResolver func(cfg *files.Config, animeIDs []int) map[int][]string

var (
	resolverMu     sync.RWMutex
	sourceResolver SourceResolver
)

// SetSourceResolver wires the resolver. The daemon sets it at boot (it needs AniList and the
// standalone file, which this package doesn't know); without one, every filtered webhook skips.
func SetSourceResolver(r SourceResolver) {
	resolverMu.Lock()
	defer resolverMu.Unlock()
	sourceResolver = r
}

func resolveSources(cfg *files.Config, items []Episode) map[int][]string {
	resolverMu.RLock()
	r := sourceResolver
	resolverMu.RUnlock()
	if r == nil {
		return nil
	}
	var ids []int
	for _, it := range items {
		if it.AnimeID != 0 && !slices.Contains(ids, it.AnimeID) {
			ids = append(ids, it.AnimeID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return r(cfg, ids)
}

// itemsFor keeps the items a preset wants: all of them when it has no AnimeSources, otherwise those
// whose anime has at least one of the preset's sources (OR).
func itemsFor(preset files.WebhookPreset, items []Episode, sources map[int][]string) []Episode {
	if len(preset.AnimeSources) == 0 {
		return items
	}
	var kept []Episode
	for _, it := range items {
		if slices.ContainsFunc(sources[it.AnimeID], func(s string) bool { return slices.Contains(preset.AnimeSources, s) }) {
			kept = append(kept, it)
		}
	}
	return kept
}

// batch accumulates the items of one event's window. The timer is stored with it so Flush can
// cancel it: without that, the shutdown flush and the timer would race on the same queue.
type batch struct {
	items []Episode
	timer *time.Timer
	cfg   *files.Config
}

var (
	batchMu      sync.Mutex
	pendingBatch = make(map[Event]*batch)
)

// Notify fires webhooks subscribed to the given event in background goroutines.
// No-op if cfg is nil or has no webhooks.
//
// With cfg.Notifications.BatchWindowSeconds > 0 the event enters a per-event-type queue and
// leaves with the others of the same window, as a single webhook. A library backfill fires one
// DownloadCompleted per torrent (77 in a real case) and services such as ntfy.sh answer 429
// past a certain volume — see decisions.md #47.
func Notify(cfg *files.Config, event Event, it Episode) {
	NotifyAll(cfg, event, []Episode{it})
}

// NotifyAll delivers items that belong to ONE event as a single webhook even with batching off —
// a pack completing, or a pack being deleted, is one push and one Jellyfin/Plex scan, not one per
// episode (decisions.md #124). With a batch window the items join the window like any other.
func NotifyAll(cfg *files.Config, event Event, items []Episode) {
	if cfg == nil || len(cfg.Notifications.Webhooks) == 0 || len(items) == 0 {
		return
	}

	window := time.Duration(cfg.Notifications.BatchWindowSeconds) * time.Second
	if window <= 0 {
		fireBatch(event, items, cfg, false)
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	if b, ok := pendingBatch[event]; ok {
		b.items = append(b.items, items...)
		// The window's webhooks (and accounts, for the source filter) are those of the last config
		// seen: if the user edited the presets mid-window, the latest one wins.
		b.cfg = cfg
		return
	}

	b := &batch{items: slices.Clone(items), cfg: cfg}
	pendingBatch[event] = b
	b.timer = time.AfterFunc(window, func() { flushEvent(event, false) })
}

// Flush fires everything currently waiting in a window and ONLY returns once the requests have
// finished. Called on daemon shutdown: without it a whole window of notifications would die
// with the process — and firing in a goroutine on shutdown would be the same as not firing,
// because the process exits before they run.
func Flush() {
	batchMu.Lock()
	events := make([]Event, 0, len(pendingBatch))
	for event := range pendingBatch {
		events = append(events, event)
	}
	batchMu.Unlock()

	for _, event := range events {
		flushEvent(event, true)
	}
}

func flushEvent(event Event, wait bool) {
	batchMu.Lock()
	b, ok := pendingBatch[event]
	if !ok {
		batchMu.Unlock()
		return
	}
	delete(pendingBatch, event)
	if b.timer != nil {
		b.timer.Stop()
	}
	batchMu.Unlock()

	fireBatch(event, b.items, b.cfg, wait)
}

// fireBatch sends the items to every preset subscribed to the event. A preset filtered by source
// gets only its share of the items, with the template variables built from that share — a window
// of 3 completions where 1 is the filtered account's reads as that 1, not as "3 episodes".
// The sources are looked up here, at fire time, not stored on the record (decisions.md #126).
//
// Without wait it returns at once: the source lookup can go to AniList, and with batching off
// this runs on the verification pass's goroutine.
func fireBatch(event Event, items []Episode, cfg *files.Config, wait bool) {
	if len(items) == 0 || cfg == nil {
		return
	}
	eventStr := eventString(event)
	var presets []files.WebhookPreset
	filtered := false
	for _, preset := range cfg.Notifications.Webhooks {
		if slices.Contains(preset.Events, eventStr) {
			presets = append(presets, preset)
			filtered = filtered || len(preset.AnimeSources) > 0
		}
	}
	if len(presets) == 0 {
		return
	}

	send := func() {
		// Only asked when some preset filters: an install with no filter never pays the lookup.
		var sources map[int][]string
		if filtered {
			sources = resolveSources(cfg, items)
		}
		var wg sync.WaitGroup
		for _, preset := range presets {
			mine := itemsFor(preset, items, sources)
			if len(mine) == 0 {
				continue
			}
			vars := buildBatchVars(event, mine)
			wg.Add(1)
			go func(p files.WebhookPreset) {
				defer wg.Done()
				fireWebhook(p, vars)
			}(preset)
		}
		wg.Wait()
	}
	if wait {
		send()
	} else {
		go send()
	}
}

// ErrWebhookNotFound distinguishes "this preset does not exist" (404) from "the preset exists
// and delivery failed" (502). Without the distinction the handler would return 404 for a token
// the service refused.
var ErrWebhookNotFound = errors.New("webhook not found")

// sampleEpisode fills the variables of a test fire, so a template using any of them shows
// something instead of a blank.
var sampleEpisode = Episode{
	AnimeID:     154587,
	AnimeName:   "Frieren Beyond Journey's End",
	Number:      5,
	TorrentHash: strings.Repeat("0", 40),
}

// FireTestWebhook fires one named webhook with sample variables. Returns ErrWebhookNotFound if the
// preset does not exist, or the delivery error if the service refused it.
func FireTestWebhook(cfg *files.Config, name string) error {
	for _, preset := range cfg.Notifications.Webhooks {
		if preset.Name == name {
			vars := buildBatchVars(DownloadCompleted, []Episode{sampleEpisode})
			return fireWebhook(preset, vars)
		}
	}
	return fmt.Errorf("%w: %q", ErrWebhookNotFound, name)
}
