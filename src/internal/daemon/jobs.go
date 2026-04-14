package daemon

import (
	"AutoAnimeDownloader/src/internal/logger"
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
	JobRenameFile      JobType = "rename_file"
	JobMoveToCompleted JobType = "move_to_completed"
)

const (
	jobTickInterval         = 15 * time.Second
	maxRetriesRenameFile    = 20
	maxRetriesMoveCompleted = 10
)

// RenameFilePayload carries the data needed to rename a torrent file for Jellyfin.
type RenameFilePayload struct {
	Hash          string `json:"hash"`
	AnimeName     string `json:"anime_name"`
	EpisodeNumber int    `json:"episode_number"`
}

// MoveToCompletedPayload carries the data needed to move an anime to the completed folder.
type MoveToCompletedPayload struct {
	Hashes    []string `json:"hashes"`
	AnimeName string   `json:"anime_name"`
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

// JobQueue runs a background goroutine that processes deferred qBittorrent operations
// every 15 seconds with exponential backoff on failure. State is persisted to disk so
// jobs survive daemon restarts.
type JobQueue struct {
	mu          sync.Mutex
	jobs        []*Job
	jobsPath    string
	fileManager FileManagerInterface
	stopCh      chan struct{}
	done        chan struct{}
}

// NewJobQueue creates a JobQueue. Call Start() to begin processing.
func NewJobQueue(fileManager FileManagerInterface, jobsPath string) *JobQueue {
	return &JobQueue{
		fileManager: fileManager,
		jobsPath:    jobsPath,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
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

// EnqueueRenameFile schedules a Jellyfin file rename for the given torrent hash.
func (q *JobQueue) EnqueueRenameFile(hash, animeName string, episodeNumber int) {
	q.enqueue(JobRenameFile, RenameFilePayload{
		Hash:          hash,
		AnimeName:     animeName,
		EpisodeNumber: episodeNumber,
	}, maxRetriesRenameFile)
}

// EnqueueMoveToCompleted schedules moving an anime's torrents to the completed folder.
func (q *JobQueue) EnqueueMoveToCompleted(hashes []string, animeName string) {
	q.enqueue(JobMoveToCompleted, MoveToCompletedPayload{
		Hashes:    hashes,
		AnimeName: animeName,
	}, maxRetriesMoveCompleted)
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
	q.mu.Unlock()

	configs, err := q.fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Job queue: failed to load configs, skipping tick")
		return
	}

	qURL := getQBittorrentURL(configs.QBittorrentUrl)
	ts := torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qURL, configs.SavePath, configs.CompletedAnimePath)

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
		success := q.executeJob(job, ts)

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

func (q *JobQueue) executeJob(job *Job, ts *torrents.TorrentService) bool {
	switch job.Type {
	case JobRenameFile:
		var p RenameFilePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			logger.Logger.Error().Err(err).Str("id", job.ID).Msg("Job queue: failed to unmarshal rename payload")
			return false
		}
		return ts.RenameEpisodeFile(p.Hash, p.AnimeName, p.EpisodeNumber)

	case JobMoveToCompleted:
		var p MoveToCompletedPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			logger.Logger.Error().Err(err).Str("id", job.ID).Msg("Job queue: failed to unmarshal move payload")
			return false
		}
		if err := ts.SendAnimeToCompletedFolder(p.Hashes, p.AnimeName); err != nil {
			logger.Logger.Warn().Err(err).Str("anime", p.AnimeName).Msg("Job queue: failed to move anime to completed folder")
			return false
		}
		return true

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
