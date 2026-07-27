package torrents

// TorrentInfo is a backend-agnostic snapshot of a torrent, exposing only what the
// daemon needs (identity, on-disk location, completion). It replaces the qBittorrent
// WebUI Torrent struct for the embedded client.
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
	// SetCallbacks registers handlers invoked when a torrent completes or fails. It also
	// arms listeners for torrents already present (e.g. loaded from resume data), except
	// those already completed (handled by startup reconciliation, not events).
	SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error))
	// Close shuts the backend down, flushing resume data.
	Close() error
}
