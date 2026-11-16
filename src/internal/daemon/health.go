package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// health.go holds the text and the edge tracking of the alert notifications — the events about the
// library and the daemon as a whole, not about one episode being fetched (notifications.isAlert).

// plural is "1 episode" / "3 episodes".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// NotifyPreviousCrash fires daemon_crashed at boot, when the previous run left a crash behind. at is
// the crash file's mtime — the same time /status exposes as pending_crash_at.
func NotifyPreviousCrash(fileManager FileManagerInterface, at time.Time) {
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load configs; the crash notification was not sent")
		return
	}
	notifications.Notify(configs, notifications.DaemonCrashed, crashItem(at))
}

func crashItem(at time.Time) notifications.Episode {
	return notifications.Episode{Text: fmt.Sprintf("The daemon crashed at %s and restarted. A crash report is waiting on the Report screen.", at.Format("2006-01-02 15:04"))}
}

// check_failing fires when passes have aborted for this long AND this many times in a row. Both: a
// 1-minute check_interval must not fire on a 2-minute blip, and a 60-minute one must not wait two
// hours.
// ponytail: constants until someone asks for other numbers; then they become config.json fields.
const (
	checkFailingAfter  = 30 * time.Minute
	checkFailingPasses = 2
)

// trackCheckFailing runs once per pass that ran — finished or aborted, never one that was cancelled
// (its SetLastCheckError(nil) would read as a recovery) or dropped because another was running.
// code is the pass's error code, "" when it finished. cfg may be nil (the config itself failed to
// load): the run is still counted, and the notification waits for a pass that can read the webhooks.
//
// Edge-triggered through the state store: fires once per run of failures, and the first finished
// pass after that sends the recovery — after a restart too (decisions.md #127).
func trackCheckFailing(cfg *files.Config, code string, now time.Time) {
	var item *notifications.Episode
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		cf := &s.CheckFailing
		switch code {
		case PassErrSetup:
			// A fresh install whose browser is already being opened on the setup page: neither a
			// failure worth a push nor proof that things work.
			return false
		case "":
			if cf.Streak == 0 && !cf.Fired {
				return false
			}
			if cf.Fired {
				it := checkRecoveredItem(cf.Since, now)
				item = &it
			}
			*cf = notifications.CheckFailingState{}
			return true
		}
		if cf.Streak == 0 {
			cf.Since = now
		}
		cf.Streak++
		cf.Code = code
		if !cf.Fired && cfg != nil && cf.Streak >= checkFailingPasses && now.Sub(cf.Since) >= checkFailingAfter {
			cf.Fired = true
			it := checkFailingItem(code, cf.Since)
			item = &it
		}
		return true
	})
	if item != nil {
		notifications.Notify(cfg, notifications.CheckFailing, *item)
	}
}

func checkFailingItem(code string, since time.Time) notifications.Episode {
	return notifications.Episode{
		Reason: code,
		Text:   fmt.Sprintf("Checks have been failing since %s: %s", since.Format("2006-01-02 15:04"), passErrorSentence(code)),
	}
}

func checkRecoveredItem(since, now time.Time) notifications.Episode {
	return notifications.Episode{
		Reason: notifications.ReasonRecovered,
		Text:   fmt.Sprintf("Checks are running again after failing for %s.", roughDuration(now.Sub(since))),
	}
}

// passErrorSentence is the cause in plain English — the same thing the frontend's passError.ts says
// on the Status banner, minus "the last check did not run".
func passErrorSentence(code string) string {
	switch code {
	case PassErrAnilist:
		return "AniList is not responding."
	case PassErrMAL:
		return "MyAnimeList is not responding, or the list is private or empty."
	case PassErrLibrary:
		return "the library folder does not support hardlinks. Pick a folder on the same disk as the downloads."
	case PassErrTorrent:
		return "the torrent client failed to start. Restart the app."
	case PassErrConfig:
		return "the configuration could not be read."
	case PassErrStorage:
		return "the app's data could not be read."
	default:
		return "the check did not finish. See the log for details."
	}
}

// roughDuration is "2h 15m" / "40m" / "3 days" — a push, not a stopwatch.
func roughDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d/(24*time.Hour)))
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	default:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
}

// NotifyUpdate fires update_available for a version the checker found, once per version: the last
// one announced lives in the state store, so a restart doesn't announce it again.
func NotifyUpdate(fileManager FileManagerInterface, version, url string) {
	fire := false
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		if s.UpdateAnnounced == version {
			return false
		}
		s.UpdateAnnounced = version
		fire = true
		return true
	})
	if !fire {
		return
	}
	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load configs; the update notification was not sent")
		return
	}
	notifications.Notify(configs, notifications.UpdateAvailable, notifications.Episode{
		Version: version,
		URL:     url,
		Text:    fmt.Sprintf("AutoAnimeDownloader %s is available: %s", version, url),
	})
}

// disk_low's warning floor is twice min_free_disk_gb (the policy's own floor, where adds start being
// refused); with the policy off, a share of the volume. A level re-arms only once free space
// climbs back above its floor by this factor, so free space hovering at the line doesn't flap.
// ponytail: constants until someone asks for other numbers.
const (
	diskWarningFactor   = 2
	diskWarningShareOff = 0.05
	diskRearmFactor     = 1.25
)

// trackDiskLow runs at the end of a finished pass. blockedEpisodes is how many episodes the pass
// refused with IssueDiskFull. Two levels, each fired once and re-armed on its own: "warning" (free
// space under the warning floor) and "blocked" (the pass refused downloads for lack of space). A disk
// that goes straight from fine to full fires "blocked" only — the warning would be stale news in the
// same push (decisions.md #127).
func trackDiskLow(cfg *files.Config, total, free uint64, blockedEpisodes int) {
	if cfg == nil || cfg.CompletedAnimePath == "" || total == 0 {
		return
	}
	minFree := float64(cfg.MinFreeDiskGB) * float64(giB)
	warnFloor := minFree * diskWarningFactor
	if cfg.MinFreeDiskGB <= 0 {
		warnFloor = float64(total) * diskWarningShareOff
	}
	freeF := float64(free)

	var item *notifications.Episode
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		before := s.DiskLowFired
		d := &s.DiskLowFired
		switch {
		case blockedEpisodes > 0 && !d.Blocked:
			d.Blocked, d.Warning = true, true
			it := diskBlockedItem(cfg.CompletedAnimePath, free, blockedEpisodes)
			item = &it
		case blockedEpisodes == 0 && freeF < warnFloor && !d.Warning:
			d.Warning = true
			it := diskWarningItem(cfg, free)
			item = &it
		}
		if d.Blocked && blockedEpisodes == 0 && freeF > minFree*diskRearmFactor {
			d.Blocked = false
		}
		if d.Warning && freeF > warnFloor*diskRearmFactor {
			d.Warning = false
		}
		return *d != before
	})
	if item != nil {
		notifications.Notify(cfg, notifications.DiskLow, *item)
	}
}

func diskWarningItem(cfg *files.Config, free uint64) notifications.Episode {
	stop := "once a torrent no longer fits"
	if cfg.MinFreeDiskGB > 0 {
		stop = fmt.Sprintf("below %d GB", cfg.MinFreeDiskGB)
	}
	return notifications.Episode{
		Reason: notifications.DiskWarning,
		Text:   fmt.Sprintf("Only %s free on %s; new downloads stop %s.", gigabytes(free), cfg.CompletedAnimePath, stop),
	}
}

func diskBlockedItem(path string, free uint64, waiting int) notifications.Episode {
	return notifications.Episode{
		Reason: notifications.DiskBlocked,
		Text:   fmt.Sprintf("Downloads are stopped: only %s free on %s. %s waiting for space.", gigabytes(free), path, pluralVerb(waiting, "episode", "is", "are")),
	}
}

// gigabytes is a size in GiB, the unit min_free_disk_gb is in, written as GB like the config screen.
func gigabytes(bytes uint64) string {
	return fmt.Sprintf("%.1f GB", float64(bytes)/float64(giB))
}

// pluralVerb is "1 episode is" / "3 episodes are".
func pluralVerb(n int, noun, one, many string) string {
	if n == 1 {
		return plural(n, noun) + " " + one
	}
	return plural(n, noun) + " " + many
}

// diskFullEpisodes counts the episodes a pass refused for lack of space.
func diskFullEpisodes(problems []Issue) int {
	n := 0
	for _, p := range problems {
		if p.Code == IssueDiskFull {
			n += max(len(p.Episodes), 1)
		}
	}
	return n
}

// diskSpace is files.DiskSpace, swappable in tests.
var diskSpace = files.DiskSpace

// alertsAfterPass evaluates the state alerts at the end of a pass that finished. Each one is
// edge-triggered through the state store (decisions.md #127).
// passAlerts is what a finished pass hands the alerts.
type passAlerts struct {
	problems []Issue
	limits   []Issue
	// watching is every anime the pass covered (media id) — an anime outside it left every
	// download status, and its alerts are dropped.
	watching map[int]bool
	// animes is the pass's list (progress, airing status, episode count); saved is episodes.json
	// read AFTER the pass's own deletions, so what the cleanup removed is never reported.
	animes []anilist.MediaList
	saved  []files.EpisodeStruct
	// listsComplete is false when some list account failed to fetch in this pass (nothing fresh,
	// nothing cached): every anime of that account would look "off the lists".
	listsComplete bool
	now           time.Time
}

func alertsAfterPass(cfg *files.Config, in passAlerts) {
	// A statfs error skips the disk alert silently — same reasoning as checkDiskSpace.
	if total, free, err := diskSpace(cfg.CompletedAnimePath); err == nil {
		trackDiskLow(cfg, total, free, diskFullEpisodes(in.problems))
	}

	library := libraryIndex(in.saved)
	var limitItems, stuckItems, completeItems, backlogItems, leftoverItems []notifications.Episode
	notifications.UpdateStateIfChanged(func(s *notifications.AlertState) {
		// The first pass after the upgrade records, doesn't announce (AlertState.Seeded).
		seeding := !s.Seeded
		s.Seeded = true
		limitItems = trackEpisodeLimit(s, in.limits, seeding)
		stuckItems = trackEpisodeStuck(s, in.problems, in.watching, in.now)
		completeItems = trackSeasonComplete(s, in.animes, library, seeding)
		// Not seeded: a 60-day-old backlog is real news, and it goes out as one message.
		backlogItems = trackUnwatchedBacklog(s, in.animes, library, in.now)
		if in.listsComplete {
			leftoverItems = leftoverDigest(s, in.animes, in.watching, library, cfg.WatchedEpisodesToKeep, in.now)
		}
	})
	notifications.NotifyAll(cfg, notifications.EpisodeLimitReached, limitItems)
	notifications.NotifyAll(cfg, notifications.EpisodeStuck, stuckItems)
	notifications.NotifyAll(cfg, notifications.SeasonComplete, completeItems)
	notifications.NotifyAll(cfg, notifications.UnwatchedBacklog, backlogItems)
	notifications.NotifyAll(cfg, notifications.LeftoverFiles, leftoverItems)
}

// trackEpisodeLimit fires for each anime whose max_episodes_per_anime limit appeared since the last
// evaluation, and re-arms an anime once its limit is gone. Called under the state store's lock.
func trackEpisodeLimit(s *notifications.AlertState, limits []Issue, seeding bool) []notifications.Episode {
	var items []notifications.Episode
	current := make([]int, 0, len(limits))
	for _, l := range limits {
		if l.Code != IssueMaxEpisodesPerAnime || slices.Contains(current, l.AnimeID) {
			continue
		}
		current = append(current, l.AnimeID)
		if seeding || slices.Contains(s.LimitReached, l.AnimeID) {
			continue
		}
		items = append(items, notifications.Episode{
			AnimeID:   l.AnimeID,
			AnimeName: l.AnimeName,
			Text:      fmt.Sprintf("%s: %s downloaded, %d more waiting for you to watch.", l.AnimeName, plural(l.Downloaded, "episode"), l.Pending),
		})
	}
	slices.Sort(current)
	s.LimitReached = current
	return items
}

// episode_stuck fires once an episode has gone this long without success since its first problem.
// A clock only fires while its problem is recent: a torrent found after a few hours of "not found"
// that then downloads slowly for two days is not stuck — it is just no longer reported as anything.
// stuckRecent covers a dead torrent's cycle (re-added next pass, dropped stalledTorrentTimeout
// later).
// ponytail: constants until someone asks for other numbers.
const (
	stuckAfter  = 48 * time.Hour
	stuckRecent = 3 * time.Hour
)

// stuckTorrentDropped is the clock's code for a torrent HandleTorrentFailure dropped (stopped with
// an error, or stalled). Every add of that torrent SUCCEEDS, so without this the add → stall → drop
// → re-search loop never shows up in the pass's problems.
const stuckTorrentDropped = "torrent_dropped"

// trackEpisodeStuck feeds the clocks with the pass's problems and fires the ones that are due.
// Called under the state store's lock.
//
// The clock belongs to the episode, not to a problem code: it starts at the first problem of any
// kind and is cleared when the episode is organized (clearStuck, from organize) — not when a pass
// reports no problem, because a torrent just re-added has none and is still not downloaded. It
// re-fires once per NEW code (not_found → above the size limit is news: raise the limit), each code
// at most once, because no_seeders and no_torrent_found alternate as seeders hover at min_seeders.
// awaiting_earlier_episode is left out: it is the consequence of another stuck episode, the one
// that gets announced.
func trackEpisodeStuck(s *notifications.AlertState, problems []Issue, watching map[int]bool, now time.Time) []notifications.Episode {
	for _, p := range problems {
		if p.Code == IssueAwaitingEarlierEpisode {
			continue
		}
		for _, ep := range p.Episodes {
			touchStuck(s, p.AnimeID, p.AnimeName, ep, p.Code, now)
		}
	}

	var items []notifications.Episode
	for key, c := range s.Stuck {
		var animeID, episode int
		fmt.Sscanf(key, "%d:%d", &animeID, &episode)
		if !watching[animeID] {
			delete(s.Stuck, key) // left every download status: nothing will ever clear it
			continue
		}
		if now.Sub(c.Since) < stuckAfter || now.Sub(c.LastAt) > stuckRecent || slices.Contains(c.FiredCodes, c.Code) {
			continue
		}
		c.FiredCodes = append(c.FiredCodes, c.Code)
		s.Stuck[key] = c
		items = append(items, notifications.Episode{
			AnimeID:   animeID,
			AnimeName: c.AnimeName,
			Number:    episode,
			Reason:    stuckSentence(c.Code) + " for " + roughDuration(now.Sub(c.Since)),
		})
	}
	slices.SortFunc(items, func(a, b notifications.Episode) int {
		if a.AnimeName != b.AnimeName {
			return strings.Compare(a.AnimeName, b.AnimeName)
		}
		return a.Number - b.Number
	})
	return items
}

func touchStuck(s *notifications.AlertState, animeID int, animeName string, episode int, code string, now time.Time) {
	if s.Stuck == nil {
		s.Stuck = make(map[string]notifications.StuckClock)
	}
	key := notifications.StuckKey(animeID, episode)
	c, ok := s.Stuck[key]
	if !ok {
		c.Since = now
	}
	c.AnimeName, c.Code, c.LastAt = animeName, code, now
	s.Stuck[key] = c
}

// recordTorrentDrop starts (or feeds) the stuck clock of every episode of a torrent
// HandleTorrentFailure dropped — a pack drops all its episodes at once.
func recordTorrentDrop(records []files.EpisodeStruct, now time.Time) {
	if len(records) == 0 {
		return
	}
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		for _, r := range records {
			touchStuck(s, r.AnimeID, r.AnimeName, r.EpisodeNumber, stuckTorrentDropped, now)
		}
		return true
	})
}

// clearStuck stops the clocks of episodes that made it into the library, and forgets the torrents
// download_failed reported for them.
func clearStuck(records []files.EpisodeStruct) {
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		changed := false
		for _, r := range records {
			key := notifications.StuckKey(r.AnimeID, r.EpisodeNumber)
			if _, ok := s.Stuck[key]; ok {
				delete(s.Stuck, key)
				changed = true
			}
			before := len(s.FailedTorrents)
			s.FailedTorrents = slices.DeleteFunc(s.FailedTorrents, func(f string) bool { return strings.HasPrefix(f, key+":") })
			changed = changed || len(s.FailedTorrents) != before
		}
		return changed
	})
}

// firstFailure records that download_failed is about to report this (episode, torrent) and says
// whether it is the first time. A different torrent failing for the same episode is new
// information; the same one failing again is not (decisions.md #128).
func firstFailure(animeID, episode int, hash string) bool {
	key := notifications.StuckKey(animeID, episode) + ":" + strings.ToLower(hash)
	first := false
	notifications.UpdateState(func(s *notifications.AlertState) bool {
		if slices.Contains(s.FailedTorrents, key) {
			return false
		}
		s.FailedTorrents = append(s.FailedTorrents, key)
		first = true
		return true
	})
	return first
}

// stuckSentence is the reason in plain English, from the clock's latest code.
func stuckSentence(code string) string {
	switch code {
	case IssueNoTorrentFound:
		return "no torrent found"
	case IssueAllAboveSizeLimit:
		return "every torrent found is above the size limit"
	case IssueNoSeeders:
		return "no torrent with enough seeders"
	case IssueTorrentRejected:
		return "the torrent client keeps refusing the torrent"
	case IssueDiskFull:
		return "not enough disk space"
	case stuckTorrentDropped:
		return "the torrent keeps stalling or failing"
	default:
		return "not downloading"
	}
}

// libraryIndex maps (anime, episode) to the record that is in the library (has LibraryPaths). A
// pack counts through its records, which already cover its range.
func libraryIndex(saved []files.EpisodeStruct) map[files.EpisodeKey]files.EpisodeStruct {
	index := make(map[files.EpisodeKey]files.EpisodeStruct, len(saved))
	for _, ep := range saved {
		if len(ep.LibraryPaths) > 0 {
			index[files.EpisodeKey{AnimeID: ep.AnimeID, Episode: ep.EpisodeNumber}] = ep
		}
	}
	return index
}

// trackSeasonComplete fires once per anime, ever, when a FINISHED anime has every unwatched episode
// in the library. At least one unwatched episode: a fully watched anime is not "ready to binge".
// Called under the state store's lock; seeded on the first run, or the upgrade would announce every
// completed anime in the library.
func trackSeasonComplete(s *notifications.AlertState, animes []anilist.MediaList, library map[files.EpisodeKey]files.EpisodeStruct, seeding bool) []notifications.Episode {
	var items []notifications.Episode
	for _, a := range animes {
		if a.Media.Status != anilist.MediaStatusFinished || a.Media.Episodes == nil || slices.Contains(s.SeasonComplete, a.Media.Id) {
			continue
		}
		total := *a.Media.Episodes
		if a.Progress >= total {
			continue
		}
		complete := true
		for ep := a.Progress + 1; ep <= total; ep++ {
			if _, ok := library[files.EpisodeKey{AnimeID: a.Media.Id, Episode: ep}]; !ok {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		s.SeasonComplete = append(s.SeasonComplete, a.Media.Id)
		if seeding {
			continue
		}
		name := getAnimeTitleSafe(a)
		items = append(items, notifications.Episode{
			AnimeID:   a.Media.Id,
			AnimeName: name,
			Text:      fmt.Sprintf("%s is complete: %s ready to watch.", name, plural(total-a.Progress, "episode")),
		})
	}
	return items
}

// unwatched_backlog fires when the next episode to watch has been in the library this long.
// ponytail: a constant until someone asks for another number.
const backlogAfter = 30 * 24 * time.Hour

// trackUnwatchedBacklog fires once per (anime, progress) when the record of episode progress+1 is in
// the library and older than backlogAfter. No progress history is stored: the age of the next episode
// already IS how long the user has not watched this. Watching one episode and stopping again
// restarts the clock from that episode's own DownloadDate. Called under the state store's lock.
func trackUnwatchedBacklog(s *notifications.AlertState, animes []anilist.MediaList, library map[files.EpisodeKey]files.EpisodeStruct, now time.Time) []notifications.Episode {
	listed := make(map[string]bool, len(animes))
	var items []notifications.Episode
	for _, a := range animes {
		key := strconv.Itoa(a.Media.Id)
		listed[key] = true
		next, ok := library[files.EpisodeKey{AnimeID: a.Media.Id, Episode: a.Progress + 1}]
		if !ok || next.DownloadDate.IsZero() || now.Sub(next.DownloadDate) < backlogAfter {
			continue
		}
		if fired, ok := s.BacklogFired[key]; ok && fired == a.Progress {
			continue
		}
		if s.BacklogFired == nil {
			s.BacklogFired = make(map[string]int)
		}
		s.BacklogFired[key] = a.Progress
		unwatched := unwatchedRecords(a.Media.Id, a.Progress, library)
		name := getAnimeTitleSafe(a)
		items = append(items, notifications.Episode{
			AnimeID:   a.Media.Id,
			AnimeName: name,
			Text: fmt.Sprintf("%s: EP %d has been waiting for %d days (%s unwatched, %s).", name, a.Progress+1,
				int(now.Sub(next.DownloadDate)/(24*time.Hour)), plural(len(unwatched), "episode"), gigabytes(librarySize(unwatched))),
		})
	}
	for key := range s.BacklogFired {
		if !listed[key] {
			delete(s.BacklogFired, key)
		}
	}
	return items
}

// unwatchedRecords is the anime's records in the library above progress.
func unwatchedRecords(animeID, progress int, library map[files.EpisodeKey]files.EpisodeStruct) []files.EpisodeStruct {
	var out []files.EpisodeStruct
	for k, ep := range library {
		if k.AnimeID == animeID && k.Episode > progress {
			out = append(out, ep)
		}
	}
	return out
}

// librarySize is what the records' library files take on disk: os.Stat over their LibraryPaths,
// each path once — every record of a pack carries the pack's whole file list. A file that is gone
// counts 0. Hardlinks share bytes with the seeding copy, so this is what deleting them would free
// only once the torrent is gone too; it is still the honest "how much of the library is this".
func librarySize(records []files.EpisodeStruct) uint64 {
	seen := make(map[string]bool)
	var total uint64
	for _, r := range records {
		for _, p := range r.LibraryPaths {
			if seen[p] {
				continue
			}
			seen[p] = true
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				total += uint64(info.Size())
			}
		}
	}
	return total
}

// leftover_files goes out at most this often, while watched or off-list files remain.
// ponytail: a constant until someone asks for another number.
const leftoverEvery = 7 * 24 * time.Hour

// leftoverDigest is the weekly digest of library files the user is done with, still on disk AFTER
// the pass's own deletions: an episode of an anime that left every download status, or one at or
// below progress − watched_episodes_to_keep. With the default cleanup on this is almost always empty;
// it fills up with delete_watched_episodes off, manually managed records, and packs the
// unclaimed-content guard holds back (decisions.md #74). It repeats weekly while the files stay — a
// reminder is the point; whoever keeps them on purpose doesn't subscribe this event. Called under
// the state store's lock, and only on a pass where every list account answered.
func leftoverDigest(s *notifications.AlertState, animes []anilist.MediaList, watching map[int]bool, library map[files.EpisodeKey]files.EpisodeStruct, keep int, now time.Time) []notifications.Episode {
	if !s.LeftoverLastDigest.IsZero() && now.Sub(s.LeftoverLastDigest) < leftoverEvery {
		return nil
	}
	progress := make(map[int]int, len(animes))
	for _, a := range animes {
		progress[a.Media.Id] = a.Progress
	}

	type group struct {
		name    string
		offList bool
		records []files.EpisodeStruct
	}
	groups := make(map[int]*group)
	for k, ep := range library {
		offList := !watching[k.AnimeID]
		if !offList && k.Episode > progress[k.AnimeID]-keep {
			continue
		}
		g := groups[k.AnimeID]
		if g == nil {
			g = &group{name: ep.AnimeName, offList: offList}
			groups[k.AnimeID] = g
		}
		g.records = append(g.records, ep)
	}
	if len(groups) == 0 {
		return nil
	}

	ids := make([]int, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b int) int { return strings.Compare(groups[a].name, groups[b].name) })

	var items []notifications.Episode
	var total uint64
	for _, id := range ids {
		g := groups[id]
		size := librarySize(g.records)
		total += size
		line := fmt.Sprintf("%s: %s watched (%s)", g.name, plural(len(g.records), "episode"), gigabytes(size))
		if g.offList {
			line = fmt.Sprintf("%s: left your lists, %s (%s)", g.name, plural(len(g.records), "episode"), gigabytes(size))
		}
		items = append(items, notifications.Episode{AnimeID: id, AnimeName: g.name, Text: line})
	}
	items[0].Title = fmt.Sprintf("%s of watched anime still on disk", gigabytes(total))
	s.LeftoverLastDigest = now
	return items
}
