package torrents

import "github.com/cenkalti/rain/v2/torrent"

// Status slugs the rest of the code needs to compare by name. StatusQueued is the only
// one that does NOT come from rain's enum: the queue writes it (queue.markQueued), because
// to rain a queued torrent is just another stopped torrent.
const (
	StatusStopped  = "stopped"
	StatusStopping = "stopping"
	StatusQueued   = "queued"
	// StatusDownloading and StatusDownloadingMetadata are the two states in which the torrent is
	// actually trying to talk to the swarm — and therefore the only ones in which "zero peers" is a
	// symptom of trouble, not just of not having started (see daemon.dropStalledTorrents).
	StatusDownloading         = "downloading"
	StatusDownloadingMetadata = "downloading_metadata"
)

// statusSlug converts rain's status enum into a stable API slug.
//
// rain exposes Status.String(), but it returns display text with a space
// ("Downloading Metadata") and may be rewritten on any lib upgrade. The slug here is
// a contract with the WebUI (translation key), so it is mapped by hand on purpose.
func statusSlug(s torrent.Status) string {
	switch s {
	case torrent.Stopped:
		return StatusStopped
	case torrent.DownloadingMetadata:
		return StatusDownloadingMetadata
	case torrent.Allocating:
		return "allocating"
	case torrent.Verifying:
		return "verifying"
	case torrent.Downloading:
		return StatusDownloading
	case torrent.Seeding:
		return "seeding"
	case torrent.Stopping:
		return StatusStopping
	default:
		return "unknown"
	}
}
