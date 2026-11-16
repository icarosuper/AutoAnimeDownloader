package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// alertHook returns a config whose one webhook (no batch window) posts "{{title}}|{{message}}" for
// the given events, and the channel the bodies arrive on.
func alertHook(t *testing.T, events ...string) (*files.Config, chan string) {
	t.Helper()
	bodies := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	t.Cleanup(srv.Close)
	return &files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
		Name: "hook", URL: srv.URL, Method: "POST", Body: "{{title}}|{{message}}", Events: events,
	}}}}, bodies
}

// received waits for one body; "" when none arrived in time.
func received(bodies chan string) string {
	select {
	case b := <-bodies:
		return b
	case <-time.After(time.Second):
		return ""
	}
}

// noneReceived fails if a body arrives within a short wait.
func noneReceived(t *testing.T, bodies chan string) {
	t.Helper()
	select {
	case b := <-bodies:
		t.Errorf("expected no notification, got %q", b)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestNotifyPreviousCrash(t *testing.T) {
	cfg, bodies := alertHook(t, "daemon_crashed")
	at := time.Date(2026, 11, 14, 3, 12, 0, 0, time.Local)

	NotifyPreviousCrash(&orchestrationFM{configs: cfg}, at)

	got := received(bodies)
	if !strings.HasPrefix(got, "AutoAnimeDownloader crashed|") || !strings.Contains(got, "2026-11-14 03:12") {
		t.Errorf("got %q", got)
	}
}

func TestTrackCheckFailing(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)

	t.Run("fires once after 30 min and 2 passes, then the recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")

		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(10*time.Minute))
		noneReceived(t, bodies) // 2 passes but only 10 min

		trackCheckFailing(cfg, PassErrAnilist, t0.Add(31*time.Minute))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") || !strings.Contains(got, "AniList is not responding") || !strings.Contains(got, "10:00") {
			t.Errorf("got %q", got)
		}

		trackCheckFailing(cfg, PassErrAnilist, t0.Add(60*time.Minute))
		noneReceived(t, bodies) // still the same run

		trackCheckFailing(cfg, "", t0.Add(90*time.Minute))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are back to normal|") || !strings.Contains(got, "1h 30m") {
			t.Errorf("got %q", got)
		}

		trackCheckFailing(cfg, "", t0.Add(100*time.Minute))
		noneReceived(t, bodies)
	})

	t.Run("a long single failure does not fire", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, "", t0.Add(2*time.Hour))
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(3*time.Hour))
		noneReceived(t, bodies)
	})

	t.Run("a blip that never fired sends no recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrAnilist, t0)
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(time.Minute))
		trackCheckFailing(cfg, "", t0.Add(2*time.Minute))
		noneReceived(t, bodies)
	})

	t.Run("setup incomplete is neither failure nor recovery", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(cfg, PassErrSetup, t0)
		trackCheckFailing(cfg, PassErrSetup, t0.Add(time.Hour))
		noneReceived(t, bodies)
	})

	t.Run("a config that cannot be read waits for a pass that can send", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "check_failing")
		trackCheckFailing(nil, PassErrConfig, t0)
		trackCheckFailing(nil, PassErrConfig, t0.Add(time.Hour))
		trackCheckFailing(cfg, PassErrAnilist, t0.Add(2*time.Hour))
		if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") {
			t.Errorf("got %q", got)
		}
	})
}

// The store survives a restart: a run of failures that already fired still sends its recovery
// after the daemon comes back, and does not fire again.
func TestTrackCheckFailing_SurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), notifications.StateFileName)
	t.Cleanup(notifications.ResetState)
	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	cfg, bodies := alertHook(t, "check_failing")

	notifications.OpenState(path)
	trackCheckFailing(cfg, PassErrAnilist, t0)
	trackCheckFailing(cfg, PassErrAnilist, t0.Add(time.Hour))
	if got := received(bodies); !strings.HasPrefix(got, "Checks are failing|") {
		t.Fatalf("got %q", got)
	}

	notifications.OpenState(path) // restart
	trackCheckFailing(cfg, PassErrAnilist, t0.Add(2*time.Hour))
	noneReceived(t, bodies)
	trackCheckFailing(cfg, "", t0.Add(3*time.Hour))
	if got := received(bodies); !strings.HasPrefix(got, "Checks are back to normal|") {
		t.Errorf("got %q", got)
	}
}

func TestNotifyUpdate_OncePerVersion(t *testing.T) {
	notifications.ResetState()
	cfg, bodies := alertHook(t, "update_available")
	cfg.Notifications.Webhooks[0].Body = "{{title}}|{{version}}|{{url}}"
	fm := &orchestrationFM{configs: cfg}

	NotifyUpdate(fm, "2.11.0", "https://example.invalid/v2.11.0")
	if got := received(bodies); got != "Update available|2.11.0|https://example.invalid/v2.11.0" {
		t.Errorf("got %q", got)
	}
	NotifyUpdate(fm, "2.11.0", "https://example.invalid/v2.11.0")
	noneReceived(t, bodies)

	NotifyUpdate(fm, "2.12.0", "https://example.invalid/v2.12.0")
	if got := received(bodies); got != "Update available|2.12.0|https://example.invalid/v2.12.0" {
		t.Errorf("got %q", got)
	}
}

func TestTrackDiskLow(t *testing.T) {
	const gib = uint64(1 << 30)
	const total = 1000 * gib
	newCfg := func(t *testing.T) (*files.Config, chan string) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "disk_low")
		cfg.CompletedAnimePath = "/media/anime"
		cfg.MinFreeDiskGB = 10 // warning floor 20 GiB
		return cfg, bodies
	}

	t.Run("warning once, re-armed only above the floor x1.25", func(t *testing.T) {
		cfg, bodies := newCfg(t)
		trackDiskLow(cfg, total, 30*gib, 0)
		noneReceived(t, bodies)

		trackDiskLow(cfg, total, 18*gib, 0)
		if got := received(bodies); got != "Disk almost full|Only 18.0 GB free on /media/anime; new downloads stop below 10 GB." {
			t.Errorf("got %q", got)
		}
		trackDiskLow(cfg, total, 17*gib, 0)
		trackDiskLow(cfg, total, 22*gib, 0) // above the floor, under x1.25: still armed off
		trackDiskLow(cfg, total, 19*gib, 0)
		noneReceived(t, bodies)

		trackDiskLow(cfg, total, 26*gib, 0) // re-armed
		trackDiskLow(cfg, total, 19*gib, 0)
		if got := received(bodies); !strings.HasPrefix(got, "Disk almost full|") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("straight to full fires blocked only, once", func(t *testing.T) {
		cfg, bodies := newCfg(t)
		trackDiskLow(cfg, total, 5*gib, 3)
		if got := received(bodies); got != "Downloads stopped: disk full|Downloads are stopped: only 5.0 GB free on /media/anime. 3 episodes are waiting for space." {
			t.Errorf("got %q", got)
		}
		trackDiskLow(cfg, total, 5*gib, 4)
		trackDiskLow(cfg, total, 9*gib, 0) // nothing refused, but still under min x1.25
		trackDiskLow(cfg, total, 5*gib, 2)
		noneReceived(t, bodies)

		trackDiskLow(cfg, total, 13*gib, 0) // freed: blocked re-armed, warning still fired (under 25)
		noneReceived(t, bodies)
		trackDiskLow(cfg, total, 5*gib, 1)
		if got := received(bodies); !strings.HasPrefix(got, "Downloads stopped: disk full|") || !strings.Contains(got, "1 episode is waiting") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("with the policy off the warning is 5% of the volume", func(t *testing.T) {
		cfg, bodies := newCfg(t)
		cfg.MinFreeDiskGB = 0
		trackDiskLow(cfg, total, 60*gib, 0)
		noneReceived(t, bodies)
		trackDiskLow(cfg, total, 40*gib, 0)
		if got := received(bodies); !strings.Contains(got, "new downloads stop once a torrent no longer fits") {
			t.Errorf("got %q", got)
		}
	})
}

func limitIssue(id int, name string, downloaded, pending int) Issue {
	return Issue{AnimeID: id, AnimeName: name, Code: IssueMaxEpisodesPerAnime, Downloaded: downloaded, Pending: pending}
}

// alertsAfterPass with the disk check stubbed out (always plenty of space).
func runAlerts(t *testing.T, cfg *files.Config, in passAlerts) {
	t.Helper()
	orig := diskSpace
	diskSpace = func(string) (uint64, uint64, error) { return 1 << 40, 1 << 39, nil }
	t.Cleanup(func() { diskSpace = orig })
	if in.now.IsZero() {
		in.now = time.Now()
	}
	alertsAfterPass(cfg, in)
}

func TestEpisodeLimitReached(t *testing.T) {
	notifications.ResetState()
	cfg, bodies := alertHook(t, "episode_limit_reached")

	// First pass after the upgrade: an anime at the limit for months is recorded, not announced.
	runAlerts(t, cfg, passAlerts{limits: []Issue{limitIssue(1, "Frieren", 12, 16)}})
	noneReceived(t, bodies)

	runAlerts(t, cfg, passAlerts{limits: []Issue{limitIssue(1, "Frieren", 12, 16), limitIssue(2, "One Piece", 12, 900)}})
	if got := received(bodies); got != "Episode limit reached|One Piece: 12 episodes downloaded, 900 more waiting for you to watch." {
		t.Errorf("got %q", got)
	}
	runAlerts(t, cfg, passAlerts{limits: []Issue{limitIssue(1, "Frieren", 12, 16), limitIssue(2, "One Piece", 12, 899)}})
	noneReceived(t, bodies)

	runAlerts(t, cfg, passAlerts{limits: []Issue{limitIssue(1, "Frieren", 12, 16)}}) // One Piece below the limit: re-armed
	runAlerts(t, cfg, passAlerts{limits: []Issue{limitIssue(1, "Frieren", 12, 16), limitIssue(2, "One Piece", 12, 898)}})
	if got := received(bodies); !strings.Contains(got, "One Piece") {
		t.Errorf("got %q", got)
	}
}

func stuckProblem(id int, name, code string, eps ...int) Issue {
	return Issue{AnimeID: id, AnimeName: name, Code: code, Episodes: eps}
}

func TestEpisodeStuck(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.Local)
	watching := map[int]bool{1: true}
	pass := func(cfg *files.Config, at time.Time, problems ...Issue) {
		runAlerts(t, cfg, passAlerts{problems: problems, watching: watching, now: at})
	}
	start := func(t *testing.T) (*files.Config, chan string) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "episode_stuck")
		pass(cfg, t0.Add(-time.Hour)) // seed
		return cfg, bodies
	}

	t.Run("fires at 48h, one line per run, then once per new code", func(t *testing.T) {
		cfg, bodies := start(t)
		pass(cfg, t0, stuckProblem(1, "Frieren", IssueNoTorrentFound, 5, 6, 7))
		pass(cfg, t0.Add(24*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5, 6, 7))
		noneReceived(t, bodies)

		pass(cfg, t0.Add(49*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5, 6, 7))
		if got := received(bodies); got != "3 episodes are not downloading|Frieren EPs 5-7: no torrent found for 2 days" {
			t.Errorf("got %q", got)
		}
		pass(cfg, t0.Add(50*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5, 6, 7))
		noneReceived(t, bodies)

		pass(cfg, t0.Add(51*time.Hour), stuckProblem(1, "Frieren", IssueAllAboveSizeLimit, 5))
		if got := received(bodies); got != "Episode is not downloading|Frieren EP 5: every torrent found is above the size limit for 2 days" {
			t.Errorf("got %q", got)
		}
		// Back to a code already announced: silent.
		pass(cfg, t0.Add(52*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5))
		noneReceived(t, bodies)
	})

	t.Run("awaiting an earlier episode never starts a clock", func(t *testing.T) {
		cfg, bodies := start(t)
		pass(cfg, t0, stuckProblem(1, "Frieren", IssueAwaitingEarlierEpisode, 6))
		pass(cfg, t0.Add(72*time.Hour), stuckProblem(1, "Frieren", IssueAwaitingEarlierEpisode, 6))
		noneReceived(t, bodies)
	})

	t.Run("organize clears the clock", func(t *testing.T) {
		cfg, bodies := start(t)
		pass(cfg, t0, stuckProblem(1, "Frieren", IssueNoTorrentFound, 5))
		clearStuck([]files.EpisodeStruct{{AnimeID: 1, EpisodeNumber: 5}})
		pass(cfg, t0.Add(47*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5)) // new clock
		pass(cfg, t0.Add(49*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5))
		noneReceived(t, bodies)
	})

	t.Run("a dead torrent's drops keep the clock running with no problem in the report", func(t *testing.T) {
		cfg, bodies := start(t)
		for h := 0; h <= 48; h += 2 {
			notifications.UpdateState(func(s *notifications.AlertState) bool {
				touchStuck(s, 1, "Frieren", 5, stuckTorrentDropped, t0.Add(time.Duration(h)*time.Hour))
				return true
			})
			pass(cfg, t0.Add(time.Duration(h)*time.Hour+time.Minute))
		}
		if got := received(bodies); got != "Episode is not downloading|Frieren EP 5: the torrent keeps stalling or failing for 2 days" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("a torrent that was found and is just downloading slowly is not stuck", func(t *testing.T) {
		cfg, bodies := start(t)
		pass(cfg, t0, stuckProblem(1, "Frieren", IssueNoTorrentFound, 5))
		pass(cfg, t0.Add(3*time.Hour)) // found, downloading
		pass(cfg, t0.Add(50*time.Hour))
		noneReceived(t, bodies)
	})

	t.Run("an anime that left the lists drops its clocks", func(t *testing.T) {
		cfg, bodies := start(t)
		pass(cfg, t0, stuckProblem(1, "Frieren", IssueNoTorrentFound, 5))
		runAlerts(t, cfg, passAlerts{watching: map[int]bool{}, now: t0.Add(time.Hour)})
		pass(cfg, t0.Add(49*time.Hour), stuckProblem(1, "Frieren", IssueNoTorrentFound, 5)) // new clock
		noneReceived(t, bodies)
	})
}

// A dropped pack starts the stuck clock of every episode it carries, not just the first record's.
func TestHandleTorrentFailure_StartsTheStuckClockOfEveryRecord(t *testing.T) {
	notifications.ResetState()
	const hash = "abababababababababababababababababababab"
	fm := &orchestrationFM{configs: &files.Config{}, saved: []files.EpisodeStruct{
		{AnimeID: 1, AnimeName: "Frieren", EpisodeNumber: 1, EpisodeHash: hash},
		{AnimeID: 1, AnimeName: "Frieren", EpisodeNumber: 2, EpisodeHash: hash},
		{AnimeID: 2, AnimeName: "Other", EpisodeNumber: 9, EpisodeHash: "cd"},
	}}

	HandleTorrentFailure(hash, nil, nil, fm)

	notifications.UpdateState(func(s *notifications.AlertState) bool {
		if len(s.Stuck) != 2 || s.Stuck["1:1"].Code != stuckTorrentDropped || s.Stuck["1:2"].Code != stuckTorrentDropped {
			t.Errorf("clocks = %+v", s.Stuck)
		}
		return false
	})
}

// download_failed reports a real failure once per (episode, torrent): the same magnet refused on
// every pass, or a dead torrent dropped every hour, is one push. A different torrent for the same
// episode is news; organizing the episode forgets the pairs (decisions.md #128).
func TestDownloadFailed_OncePerEpisodeAndTorrent(t *testing.T) {
	const hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	t.Run("dropped torrent", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "download_failed")
		saved := []files.EpisodeStruct{{AnimeID: 1, AnimeName: "Frieren", EpisodeNumber: 5, EpisodeHash: hashA}}
		fm := &orchestrationFM{configs: cfg, saved: saved}

		HandleTorrentFailure(hashA, errors.New("stalled: no peers for 1h0m"), nil, fm)
		if got := received(bodies); got != "Download failed|Frieren EP 5 failed: stalled: no peers for 1h0m" {
			t.Errorf("got %q", got)
		}
		HandleTorrentFailure(hashA, errors.New("stalled: no peers for 1h2m"), nil, fm)
		noneReceived(t, bodies)

		fm.saved[0].EpisodeHash = hashB
		HandleTorrentFailure(hashB, errors.New("boom"), nil, fm)
		if got := received(bodies); !strings.Contains(got, "boom") {
			t.Errorf("a different torrent must fire, got %q", got)
		}

		clearStuck(fm.saved) // organized
		fm.saved[0].EpisodeHash = hashA
		HandleTorrentFailure(hashA, errors.New("stalled again"), nil, fm)
		if got := received(bodies); !strings.Contains(got, "stalled again") {
			t.Errorf("organize must forget the pairs, got %q", got)
		}
	})

	t.Run("rejected add", func(t *testing.T) {
		notifications.ResetState()
		cfg, bodies := alertHook(t, "download_failed")
		cfg.MaxEpisodesPerAnime, cfg.EpisodeRetryLimit = 12, 1
		anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, false, "")
		searcher := searcherFor(nil, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(1)}}, nil)
		backend := torrents.NewFakeBackend()
		backend.AddErr = errors.New("refused")

		processAnimeEpisodes(cfg, backend, anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
		if got := received(bodies); !strings.HasPrefix(got, "Download failed|") {
			t.Errorf("got %q", got)
		}
		processAnimeEpisodes(cfg, backend, anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
		noneReceived(t, bodies)
	})
}

func finishedAnime(id int, name string, episodes, progress int) anilist.MediaList {
	a := anilist.MediaList{Progress: progress}
	a.Media.Id = id
	a.Media.Status = anilist.MediaStatusFinished
	a.Media.Episodes = &episodes
	a.Media.Title.Romaji = &name
	return a
}

func inLibrary(id int, eps ...int) []files.EpisodeStruct {
	var out []files.EpisodeStruct
	for _, ep := range eps {
		out = append(out, files.EpisodeStruct{AnimeID: id, EpisodeNumber: ep, LibraryPaths: []string{"/lib/x.mkv"}, DownloadDate: time.Now()})
	}
	return out
}

func TestSeasonComplete(t *testing.T) {
	notifications.ResetState()
	cfg, bodies := alertHook(t, "season_complete")
	watching := map[int]bool{1: true, 2: true}
	pass := func(animes []anilist.MediaList, saved []files.EpisodeStruct) {
		runAlerts(t, cfg, passAlerts{watching: watching, animes: animes, saved: saved})
	}

	// Seeded: complete before the upgrade, never announced.
	pass([]anilist.MediaList{finishedAnime(2, "Old", 2, 0)}, inLibrary(2, 1, 2))
	noneReceived(t, bodies)

	pass([]anilist.MediaList{finishedAnime(1, "Frieren", 4, 1)}, inLibrary(1, 2, 3))
	noneReceived(t, bodies) // EP 4 missing

	pass([]anilist.MediaList{finishedAnime(1, "Frieren", 4, 1)}, inLibrary(1, 2, 3, 4))
	if got := received(bodies); got != "Ready to binge|Frieren is complete: 3 episodes ready to watch." {
		t.Errorf("got %q", got)
	}
	pass([]anilist.MediaList{finishedAnime(1, "Frieren", 4, 2)}, inLibrary(1, 3, 4))
	noneReceived(t, bodies) // once per anime

	notifications.ResetState()
	pass(nil, nil) // seed
	pass([]anilist.MediaList{finishedAnime(1, "Frieren", 4, 4)}, inLibrary(1, 1, 2, 3, 4))
	noneReceived(t, bodies) // fully watched is not "ready to binge"
}
