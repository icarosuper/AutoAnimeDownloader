package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// JobType identifies the kind of deferred work to be done.
type JobType string

const (
	// JobOrganize hardlinks a completed torrent's files into the library and fires the
	// completion webhook. It is idempotent and replaces the former poll-based
	// rename/move/notify jobs.
	JobOrganize JobType = "organize"
)

const (
	jobTickInterval    = 5 * time.Second
	maxRetriesOrganize = 20
)

// OrganizePayload carries the torrent hash to organize into the library.
type OrganizePayload struct {
	Hash string `json:"hash"`
}

// Job is a single unit of deferred work.
type Job struct {
	ID         string          `json:"id"`
	Type       JobType         `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Attempts   int             `json:"attempts"`
	MaxRetries int             `json:"max_retries"`
	NextRun    time.Time       `json:"next_run"`
	CreatedAt  time.Time       `json:"created_at"`
}

// JobQueue runs a background goroutine that processes deferred library-organization work
// every few seconds with exponential backoff on failure. State is persisted to disk so
// jobs survive daemon restarts (the durable work-list; torrent completion events only
// trigger enqueueing, they are not the source of truth).
type JobQueue struct {
	mu          sync.Mutex
	jobs        []*Job
	jobsPath    string
	fileManager FileManagerInterface
	backend     torrents.TorrentBackend
	librarian   files.Librarian
	stopCh      chan struct{}
	done        chan struct{}
}

// NewJobQueue creates a JobQueue. Call SetOrchestration to inject the torrent backend and
// librarian, then Start() to begin processing.
func NewJobQueue(fileManager FileManagerInterface, jobsPath string) *JobQueue {
	return &JobQueue{
		fileManager: fileManager,
		jobsPath:    jobsPath,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// SetOrchestration injects the torrent backend and librarian used by JobOrganize.
func (q *JobQueue) SetOrchestration(backend torrents.TorrentBackend, librarian files.Librarian) {
	q.mu.Lock()
	q.backend = backend
	q.librarian = librarian
	q.mu.Unlock()
}

// Start loads persisted jobs and begins the background processing ticker.
func (q *JobQueue) Start() {
	q.loadFromDisk()
	go q.run()
}

// Stop signals the background goroutine to exit and waits for it to finish.
func (q *JobQueue) Stop() {
	close(q.stopCh)
	<-q.done
}

// EnqueueOrganize schedules organizing a completed torrent into the library. It is a no-op
// if a JobOrganize for the same hash is already pending (avoids pile-up when both the
// completion event and the reconciliation pass enqueue).
func (q *JobQueue) EnqueueOrganize(hash string) {
	q.mu.Lock()
	for _, j := range q.jobs {
		if j.Type == JobOrganize {
			var p OrganizePayload
			if json.Unmarshal(j.Payload, &p) == nil && p.Hash == hash {
				q.mu.Unlock()
				return
			}
		}
	}
	q.mu.Unlock()
	q.enqueue(JobOrganize, OrganizePayload{Hash: hash}, maxRetriesOrganize)
}

func (q *JobQueue) enqueue(jobType JobType, payload any, maxRetries int) {
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.Error().Err(err).Str("type", string(jobType)).Msg("Job queue: failed to marshal payload")
		return
	}

	job := &Job{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:       jobType,
		Payload:    raw,
		MaxRetries: maxRetries,
		NextRun:    time.Now(),
		CreatedAt:  time.Now(),
	}

	q.mu.Lock()
	q.jobs = append(q.jobs, job)
	q.mu.Unlock()

	q.saveToDisk()

	logger.Logger.Debug().
		Str("type", string(jobType)).
		Str("id", job.ID).
		Msg("Job queue: enqueued")
}

func (q *JobQueue) run() {
	defer close(q.done)
	ticker := time.NewTicker(jobTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.processDueJobs()
		}
	}
}

func (q *JobQueue) processDueJobs() {
	q.mu.Lock()
	if len(q.jobs) == 0 {
		q.mu.Unlock()
		return
	}
	backend := q.backend
	librarian := q.librarian
	q.mu.Unlock()

	if backend == nil || librarian == nil {
		return // orchestration not wired yet
	}

	configs, err := q.fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Job queue: failed to load configs, skipping tick")
		return
	}

	now := time.Now()

	q.mu.Lock()
	due := make([]*Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		if !j.NextRun.After(now) {
			due = append(due, j)
		}
	}
	q.mu.Unlock()

	if len(due) == 0 {
		return
	}

	logger.Logger.Debug().Int("count", len(due)).Msg("Job queue: processing due jobs")

	changed := false
	for _, job := range due {
		success := q.executeJob(job, backend, librarian, configs)

		q.mu.Lock()
		if success {
			q.removeJobLocked(job.ID)
			changed = true
			logger.Logger.Info().
				Str("id", job.ID).
				Str("type", string(job.Type)).
				Msg("Job queue: job completed")
		} else {
			job.Attempts++
			if job.Attempts >= job.MaxRetries {
				logger.Logger.Warn().
					Str("id", job.ID).
					Str("type", string(job.Type)).
					Int("attempts", job.Attempts).
					Msg("Job queue: job exceeded max retries, dropping")
				q.removeJobLocked(job.ID)
			} else {
				backoff := retryBackoff(job.Attempts)
				job.NextRun = time.Now().Add(backoff)
				logger.Logger.Debug().
					Str("id", job.ID).
					Str("type", string(job.Type)).
					Int("attempts", job.Attempts).
					Dur("retry_in", backoff).
					Msg("Job queue: job failed, scheduled retry")
			}
			changed = true
		}
		q.mu.Unlock()
	}

	if changed {
		q.saveToDisk()
	}
}

// retryBackoff returns 30s * 2^(attempts-1), capped at 10 minutes.
func retryBackoff(attempts int) time.Duration {
	backoff := 30 * time.Second
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff > 10*time.Minute {
			return 10 * time.Minute
		}
	}
	return backoff
}

// executeJob runs one job. Returns true when the job is done (remove it) or should be
// dropped; false to retry with backoff.
func (q *JobQueue) executeJob(job *Job, backend torrents.TorrentBackend, librarian files.Librarian, configs *files.Config) bool {
	switch job.Type {
	case JobOrganize:
		var p OrganizePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			logger.Logger.Error().Err(err).Str("id", job.ID).Msg("Job queue: failed to unmarshal organize payload")
			return true // drop malformed job
		}
		return organizeTorrent(p.Hash, backend, librarian, q.fileManager, configs)

	default:
		logger.Logger.Warn().Str("type", string(job.Type)).Msg("Job queue: unknown job type, dropping")
		return true
	}
}

func (q *JobQueue) removeJobLocked(id string) {
	out := q.jobs[:0]
	for _, j := range q.jobs {
		if j.ID != id {
			out = append(out, j)
		}
	}
	q.jobs = out
}

func (q *JobQueue) saveToDisk() {
	q.mu.Lock()
	data, err := json.Marshal(q.jobs)
	q.mu.Unlock()

	if err != nil {
		logger.Logger.Error().Err(err).Msg("Job queue: failed to marshal jobs for persistence")
		return
	}
	if err := os.WriteFile(q.jobsPath, data, 0644); err != nil {
		logger.Logger.Error().Err(err).Msg("Job queue: failed to write jobs file")
	}
}

func (q *JobQueue) loadFromDisk() {
	data, err := os.ReadFile(q.jobsPath)
	if os.IsNotExist(err) || len(data) == 0 {
		return
	}
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Job queue: failed to read jobs file")
		return
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		logger.Logger.Warn().Err(err).Msg("Job queue: failed to parse jobs file, starting fresh")
		return
	}

	q.mu.Lock()
	q.jobs = jobs
	q.mu.Unlock()

	logger.Logger.Info().Int("count", len(jobs)).Msg("Job queue: loaded pending jobs from disk")
}

// organizeTorrent hardlinks a completed torrent's files into the library and, exactly once,
// writes back the created LibraryPaths and fires the completion webhook. Idempotent across
// restarts: episodes whose LibraryPaths are already set are treated as done (no webhook,
// no re-link). Shared by JobOrganize and directly testable.
//
// Returns true when done/dropped; false to retry with backoff.
func organizeTorrent(hash string, backend torrents.TorrentBackend, librarian files.Librarian, fm FileManagerInterface, configs *files.Config) bool {
	info, ok := backend.Get(hash)
	if !ok {
		logger.Logger.Debug().Str("hash", hash).Msg("Organize: torrent no longer present, dropping")
		return true
	}
	if !info.Completed {
		return false // not seeding yet, retry
	}

	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Organize: failed to load saved episodes")
		return false
	}

	var matched []files.EpisodeStruct
	for _, ep := range saved {
		if ep.EpisodeHash == hash {
			matched = append(matched, ep)
		}
	}
	if len(matched) == 0 {
		// The episode record may not be persisted yet (fast-completing torrent). Retry;
		// bounded by MaxRetries.
		logger.Logger.Debug().Str("hash", hash).Msg("Organize: no saved episode matches hash yet, retrying")
		return false
	}

	// The webhook + write-back run only when at least one matched episode is not yet
	// organized (empty LibraryPaths). This makes reconciliation re-runs no-ops.
	needsOrganize := false
	for _, ep := range matched {
		if len(ep.LibraryPaths) == 0 {
			needsOrganize = true
			break
		}
	}
	if !needsOrganize {
		return true // already organized on a previous run
	}

	isBatch := matched[0].IsBatch || len(matched) > 1
	req := files.OrganizeRequest{
		TorrentDataDir: info.DataDir,
		AnimeName:      matched[0].AnimeName,
		CompletedPath:  configs.CompletedAnimePath,
		IsBatch:        isBatch,
		RenameJellyfin: configs.RenameFilesForJellyfin,
	}
	if !isBatch {
		ep := matched[0].EpisodeNumber
		req.EpisodeNumber = &ep
	}

	created, err := librarian.Organize(req)
	if err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Str("anime", matched[0].AnimeName).Msg("Organize: failed to hardlink into library")
		return false // retry with backoff; permanent errors drop after MaxRetries
	}

	// Write back LibraryPaths (the "organized" marker) before firing the webhook, so a
	// crash after the webhook can't re-fire it, and a crash before write-back re-runs once.
	for i := range matched {
		matched[i].LibraryPaths = created
	}
	if err := fm.UpsertEpisodes(matched); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Msg("Organize: failed to persist library paths")
		return false // retry; hardlinks already exist so Organize will no-op next time
	}

	notifications.Notify(configs, notifications.DownloadCompleted, matched[0].AnimeName, matched[0].EpisodeNumber, "")
	logger.Logger.Info().Str("hash", hash).Str("anime", matched[0].AnimeName).Int("files", len(created)).Msg("Organized torrent into library")
	return true
}
