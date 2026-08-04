package torrents

import "time"

// TorrentInfo is a backend-agnostic snapshot of a torrent, exposing what the daemon needs
// (identity, on-disk location, completion) plus the progress data the WebUI renders. It
// replaces the qBittorrent WebUI Torrent struct for the embedded client.
//
// Every field comes from a SINGLE rain Stats() call per torrent — Stats() is a blocking
// round-trip into that torrent's goroutine, not a getter, so a second call per snapshot
// doubles the cost for nothing.
type TorrentInfo struct {
	// Hash is the BitTorrent info hash (lowercase hex, 40 chars) — the join key with
	// downloaded_episodes' EpisodeHash.
	Hash string
	// Name is the torrent's display name (from the magnet / metadata).
	Name string
	// DataDir is the on-disk root of this torrent's content (<DataDir>/<id>).
	DataDir string
	// Completed reports whether all pieces are downloaded (seeding).
	Completed bool
	// Status is the API slug for rain's status enum: stopped, downloading_metadata,
	// allocating, verifying, downloading, seeding, stopping (or unknown) — plus "queued",
	// the one slug the queue writes instead of statusSlug (see queue.markQueued).
	Status string
	// BytesCompleted is the number of bytes downloaded and hash-checked.
	BytesCompleted int64
	// BytesTotal is the torrent's total size. It is 0 until the metadata arrives (a magnet
	// spends real time in downloading_metadata) — guard every division on it.
	BytesTotal int64
	// BytesUploaded is the number of bytes uploaded to the swarm.
	BytesUploaded int64
	// DownloadSpeed and UploadSpeed are bytes per second, as a 1-minute moving average.
	DownloadSpeed int
	UploadSpeed   int
	// PeersTotal is the number of connected, handshaked peers.
	PeersTotal int
	// PiecesHave and PiecesTotal are the progress source that survives a pause: Torrent.Stop()
	// frees rain's piece data and zeroes Bytes.Completed, while the bitfield these come from
	// stays intact. buildTorrentResponse falls back to their ratio so a paused torrent does not
	// render an empty bar, and Completed is derived from them for the same reason.
	PiecesHave  uint32
	PiecesTotal uint32
	// ETASeconds is the estimated time to completion, in seconds. nil means unknown or
	// infinite — rain only fills ETA while Downloading with a non-zero speed.
	ETASeconds *int64
	// SeededForSeconds is how long the torrent has been in seeding status.
	SeededForSeconds int64
	// AddedAt is when the torrent entered the session. It gives the initial order to torrents
	// the queue does not know yet: rain's ListTorrents iterates a map, so slice order carries
	// no information at all. rain persists it, so the order survives a restart.
	AddedAt time.Time
	// QueuePosition is the 1-based place in the download queue's waiting line, written by
	// queue.markQueued alongside the "queued" slug. 0 means NOT queued — which is every
	// active, every completed and every manually paused torrent (a paused one has a place in
	// the queue, but it never starts on its own, so publishing a number that predicts nothing
	// is worse than publishing none).
	QueuePosition int
}

// TorrentBackend abstracts the embedded BitTorrent client so the daemon and the tests
// share one seam. In production it is implemented by SessionManager, which owns the
// lazily-created rain-backed Session and delegates every method to it (Session itself has
// no Ensure — session creation/recreation is the manager's job). Tests use the in-memory
// FakeBackend.
type TorrentBackend interface {
	// Ensure creates the underlying session if needed, recreates it if savePath changed,
	// and recreates it when the download root on disk was swapped (see ConsumeRootSwap).
	// Returns true when a new session was created (caller should reconcile).
	// For fakes this is a no-op returning (false, nil).
	Ensure(savePath string) (bool, error)
	// ConsumeRootSwap reports — and clears — whether Ensure has detected that the download
	// folder the session was bound to is no longer the folder at the configured path: the
	// user moved it, trashed it, or replaced it.
	//
	// It matters because rain keeps writing through the file descriptors it already holds.
	// The bytes land in the folder the user moved away while the configured path stays
	// empty, so every hardlink into the library fails with "no such file or directory".
	// Ensure recreates the session, which makes rain reopen the files BY PATH, find them
	// missing and redownload them into the configured folder.
	//
	// The flag is latched rather than returned by Ensure because Ensure is also called by
	// the manual-download endpoints, which have no business reacting to it: the daemon's
	// verification pass must be the one to consume it and clear the stale library records.
	ConsumeRootSwap() bool
	// Add starts downloading/seeding a magnet and returns its info hash. Adding a magnet
	// that is already present is not an error — it returns the existing info hash.
	Add(magnet string) (string, error)
	// List returns a snapshot of all torrents in the backend.
	List() []TorrentInfo
	// Get returns the torrent with the given info hash, if present.
	Get(hash string) (TorrentInfo, bool)
	// Remove deletes a torrent. With keepData=false the seeding copy on disk is also removed.
	Remove(hash string, keepData bool) error
	// Pause stops a torrent (rain's Torrent.Stop). It does not block: the torrent enters
	// "stopping" and only reaches "stopped" up to ~5s later, after the stop event reaches
	// the trackers. rain persists the paused state, so it survives a restart.
	//
	// It also marks the torrent as PAUSED BY THE USER: it keeps its place in the queue but
	// never starts on its own, so a pause the user asked for is not undone by the queue
	// promoting it a second later. That makes pausing a queued torrent meaningful ("do not
	// start when a slot frees"). A completed (seeding) torrent bypasses the queue entirely.
	Pause(hash string) error
	// Resume puts a torrent at the BACK of the download queue and starts it if a slot is
	// free. It is deliberately not "start now": with a concurrent-download limit in place,
	// a resume that started unconditionally would silently exceed it. Use Prioritize for
	// "start now". A completed (seeding) torrent bypasses the queue entirely.
	//
	// It re-arms the completion listener — pausing consumes the one-shot NotifyStop, which
	// would otherwise leave the resumed torrent completing silently.
	Resume(hash string) error
	// Prioritize moves a torrent to the FRONT of the download queue and starts it, pausing
	// whichever active torrent is now last in queue order if that would exceed the limit
	// (position, not progress — position is the axis the user controls). Nothing is lost by
	// the demotion: rain keeps the piece bitfield across a stop. Returns an error when the
	// hash is unknown or already completed.
	Prioritize(hash string) error
	// PrioritizeAll is the batch form: the hashes go to the front in the ORDER RECEIVED, which
	// is why it is one call and not N Prioritize calls (each of those would front-push past
	// the previous one, reversing the batch). Unknown or already-completed hashes are ignored
	// rather than rejected — a list of 12 episodes must not fail whole because one of them
	// finished downloading between the render and the click.
	PrioritizeAll(hashes []string) error
	// SetMaxActiveDownloads caps how many incomplete torrents run at the same time; the rest
	// wait in the queue. 0 (or negative) disables the limit. Seeding is never capped.
	SetMaxActiveDownloads(n int)
	// Announce forces a re-announce to all trackers and DHT. It does not override the
	// trackers' minimum interval, so calling it in a loop achieves nothing.
	Announce(hash string) error
	// SetCallbacks registers handlers invoked when a torrent completes or fails. It also
	// arms listeners for torrents already present (e.g. loaded from resume data), except
	// those already completed (handled by startup reconciliation, not events).
	SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error))
	// Close shuts the backend down, flushing resume data.
	Close() error
}
