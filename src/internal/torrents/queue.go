package torrents

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"sync"

	"AutoAnimeDownloader/src/internal/logger"
)

// queueOps is the slice of the backend the queue needs. The three methods are UNEXPORTED on
// purpose: they are the RAW delegations, with none of the queue layered on top.
//
//   - pause/resume: if they were Pause/Resume, every pause decided by the queue would fire
//     another enforce and the recursion would have no bottom.
//   - list: if it were List, marking the "queued" status would re-enter queue.mu, which
//     enforce already holds — deadlock, not recursion.
type queueOps interface {
	list() []TorrentInfo
	pause(hash string) error
	resume(hash string) error
}

// queue is the ordered list of ALL incomplete torrents in the session: the first `limit`
// that are not paused download, the rest wait. Holding everything (not just what the queue
// paused) is what makes "prioritize" actually preempt and what gives a stable position to
// show on screen — nothing is inserted in the middle, so the position only changes by user
// action.
//
// Lock order: queue.mu -> SessionManager.mu, NEVER the other way. enforce holds
// queue.mu while calling list/pause/resume, which take the manager's RLock; that's why every
// SessionManager method drops its own lock before touching the queue.
type queue struct {
	mu     sync.Mutex
	limit  int      // 0 or negative = unlimited
	order  []string // every incomplete torrent, in the order they should download
	paused []string // paused by the user: do not occupy a slot, never start on their own.
	// ONLY incomplete hashes: Pause/Resume of a completed torrent delegate straight to rain
	// (seeding never occupied a slot, so the queue has no bookkeeping for it).

	// queued is the result of step 3 of the last enforce: whoever is in order, is not in
	// paused and fell outside the first `limit` — mapped to the 1-based POSITION in the
	// waiting line. Exists so markQueued does not have to redo the count on every List (the
	// UI polls), and is the only place the slug and the position come from.
	queued map[string]int

	path      string // <resume db dir>/queue.json
	lastSaved []byte // last JSON written; the write only happens when the new one differs
	// seedPaused is the upgrade latch: armed on read when there is no queue.json,
	// consumed by the first enforce WITH a session. See load.
	seedPaused bool
}

// queueFileName is the queue state file, next to rain's resume database.
const queueFileName = "queue.json"

// queueState is the on-disk format. It lives next to rain's resume database for the same
// reason as download_root.id: it is torrent-client state, not user configuration.
type queueState struct {
	Order  []string `json:"order"`
	Paused []string `json:"paused"`
}

// load reads queue.json. Missing, empty or unreadable file = empty queue; never stops the
// daemon from starting, because enforce step 2 rebuilds the order by AddedAt.
//
// A MISSING file arms the seeding latch: without it, the first boot after this deploy
// would resume torrents the user had paused by hand (rain persists them as stopped,
// and step 2 appends every incomplete). Seeding here is impossible — in NewSessionManager
// there is still no session to query — so the read only arms; enforce consumes it.
func (q *queue) load(path string) {
	q.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Logger.Warn().Err(err).Str("path", path).Msg("Could not read the download queue file; rebuilding the queue from scratch")
		}
		q.seedPaused = true
		return
	}
	var state queueState
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Logger.Warn().Err(err).Str("path", path).Msg("The download queue file is corrupted; rebuilding the queue from scratch")
		return
	}
	q.order, q.paused = state.Order, state.Paused
	q.lastSaved = data
}

// save writes order/paused when they differ from the last saved state. Called with q.mu held.
//
// lastSaved is ONLY updated after a successful Rename: updating it along with the attempt
// would turn a transient failure (disk full for a minute) into a permanent loss — the
// "only write when it changes" would never fire again and the file would freeze on the old
// version.
func (q *queue) save() {
	if q.path == "" {
		return
	}
	data, err := json.Marshal(queueState{Order: q.order, Paused: q.paused})
	if err != nil || bytes.Equal(data, q.lastSaved) {
		return
	}
	// tmp + Rename, the same pattern as FileManager.saveConfigsLocked.
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Logger.Warn().Err(err).Str("path", q.path).Msg("Failed to persist the download queue")
		return
	}
	if err := os.Rename(tmp, q.path); err != nil {
		_ = os.Remove(tmp)
		logger.Logger.Warn().Err(err).Str("path", q.path).Msg("Failed to persist the download queue")
		return
	}
	q.lastSaved = data
}

// setLimit changes the limit. The caller runs enforce afterwards.
func (q *queue) setLimit(n int) {
	q.mu.Lock()
	q.limit = n
	q.mu.Unlock()
}

// drop takes a hash out of the whole queue. Used by removal.
func (q *queue) drop(hash string) {
	q.mu.Lock()
	q.order = without(q.order, hash)
	q.paused = without(q.paused, hash)
	q.mu.Unlock()
}

// pushBack sends the hash to the END of the queue and takes it out of paused. That's what a
// user resume does: back into the queue, not to the top of it.
func (q *queue) pushBack(hash string) {
	q.mu.Lock()
	q.order = append(without(q.order, hash), hash)
	q.paused = without(q.paused, hash)
	q.mu.Unlock()
}

// markPaused records the manual pause without touching the position. Called BEFORE the real
// pause: in the gap between the two calls, an enforce fired by a torrent that just
// completed would promote exactly the torrent the user is stopping.
func (q *queue) markPaused(hash string) {
	q.mu.Lock()
	if !contains(q.paused, hash) {
		q.paused = append(q.paused, hash)
	}
	q.mu.Unlock()
}

// prioritize moves to the FRONT the hashes that are already in order and INSERTS at the
// front the ones that are not yet, in the received order. It is "move or insert" because the
// hash may be incomplete and still not be in order (freshly created session, no enforce since
// then); treating only the "already there" case would make the prioritized hash vanish from
// the queue instead of going to the front.
//
// An unknown or completed hash is inserted and then pruned by step 1 of the next enforce.
func (q *queue) prioritize(hashes []string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	front := make([]string, 0, len(hashes))
	rest := q.order
	for _, h := range hashes {
		if contains(front, h) {
			continue
		}
		front = append(front, h)
		rest = without(rest, h)
		q.paused = without(q.paused, h)
	}
	q.order = append(front, rest...)
}

// markQueued writes the "queued" slug and the position onto the torrents that are waiting.
// It is the only slug that does not come from statusSlug() — rain does not know a queue exists.
//
// The source is q.queued, not order: order also contains the active ones, and marking by
// belonging to it would paint the whole screen "queued". Whoever is in paused shows
// "stopped", which is what rain reports; "queued" only means "the queue will get to it".
//
// QueuePosition 0 = not queued (active, completed or paused). A paused one has a position in
// order, but it predicts nothing — it never starts on its own — and publishing a number that
// will not happen is worse than publishing none.
func (q *queue) markQueued(infos []TorrentInfo) []TorrentInfo {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range infos {
		if pos, ok := q.queued[infos[i].Hash]; ok {
			infos[i].Status = StatusQueued
			infos[i].QueuePosition = pos
		}
	}
	return infos
}

// enforce is the queue's only decision point: a reconciliation between order/paused and what
// the session reports. There is no "protected" argument — prioritize already repositions the
// hash at the front of order, so preemption is no longer a special case.
func (q *queue) enforce(ops queueOps) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 0. No session, leave immediately. list() returns nil while there is no session; a
	//    session with no torrents returns a NON-nil empty slice. That distinction is exactly
	//    the one that matters: nil is "I know nothing", not "there is nothing". Without this
	//    guard step 1 would prune order/paused entirely and step 5 would write the empty
	//    file over the real one — on EVERY boot, because verification calls
	//    SetMaxActiveDownloads before Ensure.
	all := ops.list()
	if all == nil {
		return
	}

	// Addition order, always: rain's ListTorrents iterates a map, so the slice order is
	// random on every call. AddedAt exists for one reason only: to give initial order to
	// torrents the queue does not know yet.
	sort.SliceStable(all, func(i, j int) bool { return all[i].AddedAt.Before(all[j].AddedAt) })

	byHash := make(map[string]TorrentInfo, len(all))
	for _, t := range all {
		byHash[t.Hash] = t
	}
	// A completed torrent does not enter order: seeding does not occupy a slot, and on
	// completion the hash leaves the queue freeing a slot.
	incomplete := func(h string) bool {
		t, ok := byHash[h]
		return ok && !t.Completed
	}

	// 1. Prune: leave order/paused whoever vanished from the session or completed.
	q.order = filter(q.order, incomplete)
	q.paused = filter(q.paused, incomplete)

	// Upgrade latch, once in the life of the install: without queue.json there is no way to
	// tell a manual pause from a torrent the old (memory-only) queue had paused, so both
	// groups enter paused. Freezing is visible on screen and costs a Play click;
	// resuming on its own is invisible and spends bandwidth undoing a user decision.
	if q.seedPaused {
		q.seedPaused = false
		seeded := 0
		for _, t := range all {
			if !t.Completed && t.Status == StatusStopped && !contains(q.paused, t.Hash) {
				q.paused = append(q.paused, t.Hash)
				seeded++
			}
		}
		if seeded > 0 {
			logger.Logger.Info().Int("count", seeded).
				Msg("No download queue file yet: torrents already stopped were kept paused; use Play or Prioritize to start them")
		}
	}

	// 2. Append: every incomplete from the session missing from order goes to the end, by
	//    AddedAt. That's the Add case and the boot with a stale queue.json.
	for _, t := range all {
		if !t.Completed && !contains(q.order, t.Hash) {
			q.order = append(q.order, t.Hash)
		}
	}

	// 3. Compute the desired set: walk order skipping paused, marking the first `limit` as
	//    active and the rest as queued, numbered from 1 in the order they will start. The
	//    numbering does not cost a second pass.
	wanted := make(map[string]bool, len(q.order))
	queued := make(map[string]int)
	active, waiting := 0, 0
	for _, h := range q.order {
		if contains(q.paused, h) {
			continue
		}
		if q.limit <= 0 || active < q.limit {
			wanted[h] = true
			active++
			continue
		}
		waiting++
		queued[h] = waiting
	}
	q.queued = queued

	// 4. Apply the diff by iterating ORDER, not the session. Walking the session would pause
	//    every seeder: a completed one never enters order, so it is never in `wanted`, so
	//    "should not be active". order is the list of incompletes and is exactly the set
	//    the queue has authority over.
	//
	//    "stopping" is a target of nothing: rain's pause is asynchronous, so treating it as
	//    stopped would send resume on top of a stop in progress, and treating it as running
	//    would pause it again. It stays out of both sides and the next pass decides on a
	//    stable state.
	for _, h := range q.order {
		t := byHash[h]
		switch {
		case t.Status == StatusStopping:
		case wanted[h]:
			if t.Status == StatusStopped {
				if err := ops.resume(h); err != nil {
					logger.Logger.Warn().Err(err).Str("hash", h).Msg("Download queue: failed to resume torrent")
				}
			}
		case t.Status != StatusStopped:
			if err := ops.pause(h); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", h).Msg("Download queue: failed to pause torrent")
				// q.queued is the DESIRED set, and the screen must not show it as if it were
				// real: the hash stays in position (the queue still wants that), but the slug
				// and the number come off, otherwise the row would display "Queued #3" on a
				// torrent downloading at full speed. The hole in the numbering lasts until
				// the next pass.
				delete(q.queued, h)
			}
		}
	}

	// 5. Save, if it changed.
	q.save()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func without(list []string, s string) []string {
	return filter(list, func(v string) bool { return v != s })
}

// filter compacts the slice in place. Safe for order/paused: they are private slices of the queue.
func filter(list []string, keep func(string) bool) []string {
	out := list[:0]
	for _, v := range list {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
