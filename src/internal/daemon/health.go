package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"fmt"
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
