package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"fmt"
	"slices"
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
func alertsAfterPass(cfg *files.Config, problems, limits []Issue) {
	// A statfs error skips the disk alert silently — same reasoning as checkDiskSpace.
	if total, free, err := diskSpace(cfg.CompletedAnimePath); err == nil {
		trackDiskLow(cfg, total, free, diskFullEpisodes(problems))
	}

	var fire []notifications.Episode
	notifications.UpdateStateIfChanged(func(s *notifications.AlertState) {
		// The first pass after the upgrade records, doesn't announce (AlertState.Seeded).
		seeding := !s.Seeded
		s.Seeded = true
		fire = append(fire, trackEpisodeLimit(s, limits, seeding)...)
	})
	notifications.NotifyAll(cfg, notifications.EpisodeLimitReached, fire)
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
