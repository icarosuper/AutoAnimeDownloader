package daemon

import (
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
