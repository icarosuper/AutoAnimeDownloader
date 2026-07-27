package torrents

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
	// allocating, verifying, downloading, seeding, stopping (or unknown).
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
}

// TorrentBackend abstracts the embedded BitTorrent client so the daemon and the tests
// share one seam. In production it is implemented by SessionManager, which owns the
// lazily-created rain-backed Session and delegates every method to it (Session itself has
// no Ensure — session creation/recreation is the manager's job). Tests use the in-memory
// FakeBackend.
type TorrentBackend interface {
	// Ensure creates the underlying session if needed (or recreates it if savePath
	// changed). Returns true when a new session was created (caller should reconcile).
	// For fakes this is a no-op returning (false, nil).
	Ensure(savePath string) (bool, error)
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
	Pause(hash string) error
	// Resume starts a paused torrent (rain's Torrent.Start) and re-arms its completion
	// listener — pausing consumes the one-shot NotifyStop, which would otherwise leave the
	// resumed torrent completing silently.
	Resume(hash string) error
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
