package daemon

import (
	"fmt"
	"sync"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
)

// stalledTorrentTimeout is how long a torrent can keep trying with ZERO peers before the
// daemon gives up on it.
//
// ponytail: constant, not config. Becomes a config.json field when someone asks for a different
// number — the cost of finding out that an hour is too short is one extra pass, not a lost
// download (the bytes stay, see #24).
//
// The "no seeds" chip on the screen fires at 10 minutes and this cut is MUCH looser on purpose:
// there the effect is painting a warning, here it is dropping the torrent from the session.
const stalledTorrentTimeout = time.Hour

// stalledTorrents stores, by hash, since when that torrent has been at zero peers.
//
// Package state (not a State field) because only the verification pass writes it and nothing
// outside the daemon needs to read it: promoting it to State would mean exposing in the API a
// clock the screen already measures on its own (frontend `lib/stores/stallTracker.ts`, which
// still exists — it feeds the chip, this one decides the action).
//
// In memory, like the frontend one: a restart re-observes stalls from scratch, and the price of
// that is at most one extra wait cycle.
var stalledTorrents = struct {
	mu    sync.Mutex
	since map[string]time.Time
}{since: map[string]time.Time{}}

// stalling answers whether this torrent is trying and not succeeding.
//
// Both statuses matter: outside them zero peers is not a symptom of anything. A QUEUED torrent
// is at zero peers because it hasn't even started (queue.markQueued), a paused one because the
// user told it to stop, and a completed one because nobody is pulling from it — dropping any of
// the three for "no peers" would kill the app's normal operation.
func stalling(t torrents.TorrentInfo) bool {
	if t.Completed || t.PeersTotal != 0 {
		return false
	}
	return t.Status == torrents.StatusDownloading || t.Status == torrents.StatusDownloadingMetadata
}

// dropStalledTorrents drops torrents that spent stalledTorrentTimeout with no peers.
//
// A stalled torrent does not come back on its own: rain keeps announcing to a swarm that does
// not exist, and meanwhile the torrent occupies a max_concurrent_downloads slot forever — with
// old anime that is the common case, not the exception. The pass measures this because it is
// the one that already has the session snapshot (backend.List), and it runs at check_interval.
//
// The action reuses HandleTorrentFailure whole instead of inventing a new path: it is exactly
// what the app already does with a torrent rain stopped with an error — download_failed webhook,
// leaves the session (keeping the bytes already downloaded) and the NEXT pass searches Nyaa
// again, possibly finding a more alive release (decisions.md #24). The received snapshot is not
// mutated: whoever left the session now is only re-searched on the next pass, not in the middle
// of this one.
//
// Pausing instead of dropping does not work: the queue treats paused as "never climbs on its
// own" (torrents/queue.go), so the stalled download would become stalled and silent.
func dropStalledTorrents(snapshot []torrents.TorrentInfo, backend torrents.TorrentBackend, fileManager FileManagerInterface, now time.Time) {
	stalledTorrents.mu.Lock()
	defer stalledTorrents.mu.Unlock()

	stillStalling := make(map[string]bool, len(snapshot))
	for _, t := range snapshot {
		if !stalling(t) {
			continue
		}
		stillStalling[t.Hash] = true

		since, seen := stalledTorrents.since[t.Hash]
		if !seen {
			stalledTorrents.since[t.Hash] = now
			continue
		}
		if now.Sub(since) < stalledTorrentTimeout {
			continue
		}

		delete(stalledTorrents.since, t.Hash)
		logger.Logger.Warn().Str("hash", t.Hash).Str("name", t.Name).Dur("stalled_for", now.Sub(since)).
			Msg("Dropping stalled torrent: no peers for too long")
		HandleTorrentFailure(t.Hash, fmt.Errorf("stalled: no peers for %s", now.Sub(since).Round(time.Minute)), backend, fileManager)
	}

	// Prune: a hash that found peers again, was paused, completed, or left the session does not
	// keep a clock. Without this the map would grow forever and a torrent that stalled, came
	// back and stalled again weeks later would be dropped immediately, by the old stamp.
	for hash := range stalledTorrents.since {
		if !stillStalling[hash] {
			delete(stalledTorrents.since, hash)
		}
	}
}
