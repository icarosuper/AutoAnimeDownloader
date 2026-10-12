package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"sync"
)

// diskBudget is the disk guard's view of one pass: the bytes the torrents already in the session
// still have to write, plus the size of every torrent the pass adds. Free space alone answers "is
// there room NOW", and a torrent only takes its room as it downloads — so each loose episode of a
// pass fit on its own and 130 GiB went into 6 GiB of headroom (decisions.md #101).
//
// Shared by the animes of a pass, which run in parallel: reserve checks and counts under one lock,
// or two episodes would both see the last free bytes. Keyed by infohash, because the N episodes of
// a pack add the same magnet N times and must pay for it once.
//
// nil = no budget, the bare checkDiskSpace: the tests and RunAnimeDebug, which add nothing.
type diskBudget struct {
	mu       sync.Mutex
	reserved int64
	counted  map[string]bool
}

// newDiskBudget reserves what each incomplete torrent of the session still has to write. A torrent
// whose metadata hasn't arrived (every queued one: it never started) has BytesTotal 0, and its size
// comes from the saved record instead — so after a restart the queue still counts.
//
// ponytail: a torrent with neither (a record from before torrent_size existed, or a hand-pasted
// magnet) reserves nothing until its metadata arrives; it can't be guessed.
func newDiskBudget(session []torrents.TorrentInfo, saved []files.EpisodeStruct) *diskBudget {
	recorded := make(map[string]int64, len(saved))
	for _, ep := range saved {
		if ep.TorrentSize > 0 {
			recorded[ep.EpisodeHash] = ep.TorrentSize
		}
	}

	b := &diskBudget{counted: make(map[string]bool, len(session))}
	for _, t := range session {
		b.counted[t.Hash] = true
		if t.Completed {
			continue
		}
		total := t.BytesTotal
		if total == 0 {
			total = recorded[t.Hash]
		}
		// The bitfield survives a pause and BytesCompleted doesn't (see TorrentInfo.PiecesHave):
		// without it a paused half-done torrent would reserve its full size again.
		written := t.BytesCompleted
		if t.PiecesTotal > 0 {
			written = max(written, total*int64(t.PiecesHave)/int64(t.PiecesTotal))
		}
		b.reserved += max(total-written, 0)
	}
	return b
}

// check answers whether a torrent of sizeBytes would fit on top of what's reserved, without
// reserving it. For the choices made before any add: packsThatFit and the report.
func (b *diskBudget) check(configs *files.Config, sizeBytes int64) error {
	if b == nil {
		return checkDiskSpace(configs, sizeBytes, 0)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return checkDiskSpace(configs, sizeBytes, b.reserved)
}

// reserve counts the candidate's size if it fits, and returns how to give it back when the add
// fails. A torrent already counted — in the session, or reserved by another episode of the same
// pack — fits for free: adding it again writes nothing new.
func (b *diskBudget) reserve(configs *files.Config, tr nyaa.TorrentResult) (undo func(), err error) {
	if b == nil {
		return func() {}, checkDiskSpace(configs, tr.Size, 0)
	}
	key := budgetKey(tr.MagnetLink)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.counted[key] {
		return func() {}, nil
	}
	if err := checkDiskSpace(configs, tr.Size, b.reserved); err != nil {
		return nil, err
	}
	b.counted[key] = true
	b.reserved += tr.Size
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.counted, key)
		b.reserved -= tr.Size
	}, nil
}

// counts reports whether one of the candidates is already paid for. The empty-disk check before the
// candidate loop must let those through: the second episode of a pack that just took the last free
// bytes still has to be recorded.
func (b *diskBudget) counts(candidates []nyaa.TorrentResult) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tr := range candidates {
		if b.counted[budgetKey(tr.MagnetLink)] {
			return true
		}
	}
	return false
}

// budgetKey is the hash the magnet will get in the session, or the magnet itself when it doesn't
// parse — then Add fails anyway, and the key only has to be stable.
func budgetKey(magnet string) string {
	if h, err := torrents.InfoHash(magnet); err == nil {
		return h
	}
	return magnet
}
