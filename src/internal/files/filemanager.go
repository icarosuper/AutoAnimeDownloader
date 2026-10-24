package files

import (
	"AutoAnimeDownloader/src/internal/datadir"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const configFileName = "config.json"
const downloadedEpsFileName = "downloaded_episodes"
const blockedEpsFileName = "blocked_episodes"
const animeSettingsFileName = "anime_settings"
const standaloneAnimesFileName = "standalone_animes"

// anilistCacheFileName holds the snapshot of the anilist package's caches. Opaque bytes to files on
// purpose: the snapshot's schema belongs to anilist, and going through here as []byte is what keeps
// files from having to import anilist just to persist its cache.
const anilistCacheFileName = "anilist_cache"

// EpisodeKey identifies an episode. It's (anime, episode number) and not the AniList airingSchedule
// node id, because that id doesn't exist for every episode: AniList keeps a schedule window per
// media and drops the old ones, so One Piece 1 to 1122 and every old anime had no id at all — and
// therefore couldn't be downloaded (see decisions.md #52).
type EpisodeKey struct {
	AnimeID int `json:"anime_id"`
	Episode int `json:"episode"`
}

func (e EpisodeStruct) Key() EpisodeKey {
	return EpisodeKey{AnimeID: e.AnimeID, Episode: e.EpisodeNumber}
}

type EpisodeStruct struct {
	AnimeID            int       `json:"anime_id"`
	AnimeTotalEpisodes int       `json:"anime_total_episodes,omitempty"`
	AnimeName          string    `json:"anime_name,omitempty"`
	EpisodeHash        string    `json:"episode_hash"`
	EpisodeName        string    `json:"episode_name"`
	EpisodeNumber      int       `json:"episode_number"`
	DownloadDate       time.Time `json:"download_date"`
	ManuallyManaged    bool      `json:"manually_managed,omitempty"`
	// IsBatch marks episodes that came from a batch/movie torrent (multiple episodes share
	// one EpisodeHash; library files keep raw names, never Jellyfin-renamed).
	IsBatch bool `json:"is_batch,omitempty"`
	// BatchStart/BatchEnd are the pack's REAL range at download time, in the entry's LOCAL numbering.
	// Without it the detail screen rebuilt the range from the min/max of the SAVED episodes, which are
	// only the pending ones: a 01-11 pack downloaded with 5 episodes already watched showed up as
	// "6-11".
	//
	// The range comes from the torrent NAME when the name carries it and, when it doesn't, from the
	// file list of the Nyaa detail page (decisions.md #84). Zero means UNKNOWN — an old record, or
	// neither source resolved it — and never "1..total": that guess was read as a declared range and
	// spread made-up coverage to the series' other cours. Whoever displays it falls back to the
	// min/max.
	BatchStart int `json:"batch_start,omitempty"`
	BatchEnd   int `json:"batch_end,omitempty"`
	// TorrentSize is the size Nyaa announced for the torrent, in bytes, 0 when unknown (an old record
	// or a hand-pasted magnet). The disk guard needs it for a torrent still waiting in the queue: rain
	// only learns BytesTotal from the metadata, which a queued torrent never fetched (decisions.md #101).
	TorrentSize int64 `json:"torrent_size,omitempty"`
	// LibraryPaths are the hardlink paths created in the completed-anime library by
	// JobOrganize. Empty means "not yet organized" — the marker JobOrganize uses to fire
	// the completion webhook and write-back exactly once (idempotent across restarts).
	LibraryPaths []string `json:"library_paths,omitempty"`
}

type WebhookPreset struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Events  []string          `json:"events"`
}

type NotificationsConfig struct {
	Webhooks []WebhookPreset `json:"webhooks"`
	// BatchWindowSeconds groups the events of one window into a single webhook. 0 disables grouping
	// (one webhook per event, the original behavior). It exists because a library backfill fires one
	// `download_completed` per torrent and blows the quota of services like ntfy.sh — see decisions.md
	// #47.
	BatchWindowSeconds int `json:"batch_window_seconds"`
}

type Config struct {
	CompletedAnimePath string   `json:"completed_anime_path"`
	AnilistUsername    string   `json:"anilist_username,omitempty"`
	AnilistUsernames   []string `json:"anilist_usernames"`
	// MALUsernames are MyAnimeList accounts used as LIST sources only (status + progress);
	// everything else still comes from AniList — see decisions.md #95.
	MALUsernames  []string `json:"mal_usernames"`
	CheckInterval int      `json:"check_interval"`
	// MaxEpisodesPerAnime limits how many episodes of an anime exist at the same time, and applies
	// ONLY on the episode-by-episode path: a batch is a single torrent, so limiting records wouldn't
	// limit bytes or files in the library (see decisions.md). 0 means NO CEILING (see
	// effectiveMax/windowEnd in daemon/episodes.go).
	MaxEpisodesPerAnime int `json:"max_episodes_per_anime"`
	// MaxBatchTorrentSizeGB / MaxEpisodeTorrentSizeGB drop torrents above the ceiling from the search,
	// in GiB. 0 disables. The pack one is the ONLY pack guard since eligibility stopped being an
	// episode count: 100 fits a complete pack of a seasonal series in 1080p and doesn't fit a complete
	// One Piece pack — for a long series what passes is a partial pack.
	MaxBatchTorrentSizeGB   float64 `json:"max_batch_torrent_size_gb"`
	MaxEpisodeTorrentSizeGB float64 `json:"max_episode_torrent_size_gb"`
	// MinSeeders drops from the search any torrent with fewer seeders than this. 0 disables. The
	// default is 1: it only bars the literally dead torrent, which without a floor is a valid
	// candidate and even gets picked when it's the episode's only result.
	MinSeeders int `json:"min_seeders"`
	// MaxSearchPages is the ceiling of Nyaa pages per search. The search only goes down to the next
	// page while it has few accepted candidates, so the ceiling is a limit, not a fixed cost. <= 1
	// means searching only page 1.
	MaxSearchPages int `json:"max_search_pages"`
	// MinFreeDiskPercent bars adding new torrents below this percentage of free space on the library
	// volume. 0 disables.
	MinFreeDiskPercent int `json:"min_free_disk_percent"`
	EpisodeRetryLimit  int `json:"episode_retry_limit"`
	// MaxConcurrentDownloads limits how many INCOMPLETE torrents download at the same time; the excess
	// waits in the queue (torrents.queue). 0 disables the limit. Seeding never counts.
	//
	// No migration needed: LoadConfigs deserializes ON TOP of getDefaultConfig(), so a config.json
	// older than this field already loads with the default.
	MaxConcurrentDownloads int      `json:"max_concurrent_downloads"`
	DeleteWatchedEpisodes  bool     `json:"delete_watched_episodes"`
	WatchedEpisodesToKeep  int      `json:"watched_episodes_to_keep"`
	ExcludedList           string   `json:"excluded_list,omitempty"`
	ExcludedLists          []string `json:"excluded_lists"`
	RenameFilesForJellyfin bool     `json:"rename_files_for_jellyfin"`
	DownloadStatuses       []string `json:"download_statuses"`
	DownloadMediaStatuses  []string `json:"download_media_statuses"`
	DeleteStatuses         []string `json:"delete_statuses"`
	// AnimeIDsAreMediaIDs marks that daemon.MigrateAnimeIDsToMedia already converted the saved
	// AnimeIDs from ENTRY id (MediaList, per account) to MEDIA id (see decisions.md #43). The default
	// is false on purpose: a config.json older than this field deserializes on top of the default and
	// needs to migrate. On a new install the migration runs with nothing to do and turns the field on
	// in the first pass.
	AnimeIDsAreMediaIDs bool                `json:"anime_ids_are_media_ids"`
	Notifications       NotificationsConfig `json:"notifications"`
	Priorities          nyaa.Priorities     `json:"priorities"`
}

// downloadDirName is the name of the download directory inside the library. The dot hides it from
// Jellyfin's scanner on Linux; the .ignore file created by Librarian.ProbePath covers the other
// platforms.
const downloadDirName = ".torrents"

// DownloadPath is the directory where torrents download and keep seeding. It's derived from
// CompletedAnimePath, never stored: that way the hardlink constraint (source and destination on the
// same filesystem) is impossible to violate through configuration.
//
// Returns "" when the library isn't configured. This guard is mandatory: without it filepath.Join
// would produce the relative path ".autoAnimeDownloader" and the rain session would be created in
// the process's working directory. With "", SessionManager.Ensure returns ErrSessionNotReady, which
// is the current behavior for an incomplete config.
func (c *Config) DownloadPath() string {
	if c.CompletedAnimePath == "" {
		return ""
	}
	return filepath.Join(c.CompletedAnimePath, downloadDirName)
}

type AnimeSettings struct {
	// Progress is the MANUAL progress, and exists for the standalone anime: GetMediaByID returns a
	// fixed Progress: 0, and without progress nothing is ever "watched", the pruning never runs and
	// the rotation of successive packs has nothing to move it. Absent reads as 0, which is the earlier
	// behavior. A list anime never uses this field: there its list (AniList or MyAnimeList) rules.
	Progress int `json:"progress,omitempty"`
}

type FileManager struct {
	fs                   FileSystem
	configPath           string
	episodesPath         string
	blockedEpisodesPath  string
	animeSettingsPath    string
	standaloneAnimesPath string
	anilistCachePath     string
	mu                   sync.Mutex
}

// applyNyaaSettings pushes to the nyaa package the config fields it consumes. Called on EVERY
// return of LoadConfigs — nyaa can't import files (cycle), so the config reaches it by push, never
// by read.
func applyNyaaSettings(config *Config) {
	nyaa.SetPriorities(config.Priorities)
	nyaa.SetMaxSearchPages(config.MaxSearchPages)
	nyaa.SetMaxBatchTorrentSizeGB(config.MaxBatchTorrentSizeGB)
}

func getDefaultConfig() *Config {
	// Library default: ~/Animes. If home doesn't exist (container without HOME), it stays "" and the
	// config remains "incomplete" as before, requiring the user to fill it in.
	completedPath := ""
	if home, err := os.UserHomeDir(); err == nil {
		completedPath = filepath.Join(home, "Animes")
	}

	return &Config{
		CompletedAnimePath:    completedPath,
		AnilistUsernames:      []string{},
		MALUsernames:          []string{},
		CheckInterval:         10,
		MaxEpisodesPerAnime:   12,
		MaxBatchTorrentSizeGB: 100,
		// Passa WEB-DL (~1,4 GB) e encode de BD (~2 GB), barra BDRemux (~6-8 GB).
		MaxEpisodeTorrentSizeGB: 5,
		MinSeeders:              3,
		MaxSearchPages:          5,
		MinFreeDiskPercent:      10,
		EpisodeRetryLimit:       5,
		MaxConcurrentDownloads:  3,
		DeleteWatchedEpisodes:   true,
		// DeleteWatchedEpisodes comes on by default: with no slack the episode disappears as soon as it's
		// marked watched. Two forgive "I marked it by mistake" and "I wanted to rewatch".
		WatchedEpisodesToKeep: 2,
		ExcludedLists:         []string{},
		DownloadStatuses:      []string{"CURRENT", "REPEATING"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
		DeleteStatuses:        []string{},
		Notifications:         NotificationsConfig{Webhooks: []WebhookPreset{}, BatchWindowSeconds: 60},
		Priorities:            nyaa.DefaultPriorities(),
	}
}

func NewManager(fs FileSystem, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath, standaloneAnimesPath string) *FileManager {
	return &FileManager{
		fs:                   fs,
		configPath:           configPath,
		episodesPath:         episodesPath,
		blockedEpisodesPath:  blockedEpisodesPath,
		animeSettingsPath:    animeSettingsPath,
		standaloneAnimesPath: standaloneAnimesPath,
		// Derived, not one more parameter: no caller needs to choose where the AniList cache lives, and
		// the constructor already has six positional ones.
		anilistCachePath: filepath.Join(filepath.Dir(configPath), anilistCacheFileName),
	}
}

func NewDefaultFileManager() (*FileManager, error) {
	fs := NewOSFileSystem()
	configsFolderPath, err := datadir.Path()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configsFolderPath, configFileName)
	episodesPath := filepath.Join(configsFolderPath, downloadedEpsFileName)
	blockedEpisodesPath := filepath.Join(configsFolderPath, blockedEpsFileName)
	animeSettingsPath := filepath.Join(configsFolderPath, animeSettingsFileName)
	standaloneAnimesPath := filepath.Join(configsFolderPath, standaloneAnimesFileName)

	return NewManager(fs, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath, standaloneAnimesPath), nil
}

// ConfigExists says whether a config.json already exists on disk. It's used to detect the first
// run, so it must be checked BEFORE the first LoadConfigs — which creates the file with the
// defaults when it doesn't exist.
func (m *FileManager) ConfigExists() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.fs.Stat(m.configPath)
	return err == nil
}

func (m *FileManager) LoadConfigs() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	config := getDefaultConfig()

	_, err := m.fs.Stat(m.configPath)
	if os.IsNotExist(err) {
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		applyNyaaSettings(config)
		return config, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	file, err := m.fs.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	trimmed := strings.TrimSpace(string(file))
	if len(trimmed) == 0 {
		logger.Logger.Warn().Msg("Config file is empty, recreating with default values")
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		applyNyaaSettings(config)
		return config, nil
	}

	if err := json.Unmarshal(file, config); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to parse config JSON, recreating with default values")
		config = getDefaultConfig()
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config after parse error: %w", err)
		}
		applyNyaaSettings(config)
		return config, nil
	}

	// Migrate deprecated excluded_list (string) → excluded_lists ([]string)
	if config.ExcludedList != "" && len(config.ExcludedLists) == 0 {
		for _, item := range strings.Split(config.ExcludedList, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				config.ExcludedLists = append(config.ExcludedLists, trimmed)
			}
		}
		config.ExcludedList = ""
		if err := m.saveConfigsLocked(config); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to save migrated config")
		}
	}

	if config.ExcludedLists == nil {
		config.ExcludedLists = []string{}
	}

	// Migrate deprecated anilist_username (string) → anilist_usernames ([]string)
	if config.AnilistUsername != "" && len(config.AnilistUsernames) == 0 {
		config.AnilistUsernames = []string{config.AnilistUsername}
		config.AnilistUsername = ""
		if err := m.saveConfigsLocked(config); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to save migrated config")
		}
	}

	if config.AnilistUsernames == nil {
		config.AnilistUsernames = []string{}
	}
	if config.MALUsernames == nil {
		config.MALUsernames = []string{}
	}

	applyNyaaSettings(config)
	return config, nil
}

func (m *FileManager) SaveConfigs(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveConfigsLocked(config)
}

// writeAtomic writes through a temp file + rename, so a reader (or a power loss mid-write) never
// sees the file truncated. Every state persisted here is rewritten whole on every change, so a
// direct WriteFile leaves a window where the file is half-written — that's how the episodes file
// got corrupted.
func (m *FileManager) writeAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := m.fs.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tmpPath, err)
	}

	if err := m.fs.Rename(tmpPath, path); err != nil {
		_ = m.fs.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file %s: %w", tmpPath, err)
	}

	return nil
}

// saveConfigsLocked performs an atomic write of config. Must be called with m.mu held.
func (m *FileManager) saveConfigsLocked(config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return m.writeAtomic(m.configPath, data)
}

// The episode routines below do read-modify-write on the same file. The UI fires several of them in
// parallel (releasing/deleting several episodes at once) while the daemon organizes torrents, so
// all of them must hold m.mu — otherwise updates get lost and concurrent writes corrupt the file.
func (m *FileManager) LoadSavedEpisodes() ([]EpisodeStruct, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadSavedEpisodesLocked()
}

// loadSavedEpisodesLocked must be called with m.mu held.
func (m *FileManager) loadSavedEpisodesLocked() ([]EpisodeStruct, error) {
	_, err := m.fs.Stat(m.episodesPath)
	if os.IsNotExist(err) {
		return []EpisodeStruct{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat episodes file: %w", err)
	}

	b, err := m.fs.ReadFile(m.episodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read episodes file: %w", err)
	}

	episodes, err := ParseEpisodes(string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to parse episodes: %w", err)
	}

	if len(episodes) > 0 && episodes[0].DownloadDate.IsZero() {
		needsMigration := false
		for _, ep := range episodes {
			if ep.DownloadDate.IsZero() {
				needsMigration = true
				break
			}
		}
		if needsMigration {
			for i := range episodes {
				if episodes[i].DownloadDate.IsZero() {
					episodes[i].DownloadDate = time.Now()
				}
			}
			if err := m.saveEpisodesLocked(episodes); err != nil {
				return nil, fmt.Errorf("failed to migrate episodes to JSON format: %w", err)
			}
		}
	}

	return episodes, nil
}

func (m *FileManager) SaveEpisodesToFile(episodes []EpisodeStruct) error {
	if len(episodes) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existingEpisodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load existing episodes: %w", err)
	}

	existingMap := make(map[EpisodeKey]bool)
	for _, ep := range existingEpisodes {
		existingMap[ep.Key()] = true
	}

	var newEpisodes []EpisodeStruct
	for _, ep := range episodes {
		if !existingMap[ep.Key()] {
			newEpisodes = append(newEpisodes, ep)
		}
	}

	if len(newEpisodes) == 0 {
		return nil
	}

	allEpisodes := append(existingEpisodes, newEpisodes...)

	return m.saveEpisodesLocked(allEpisodes)
}

// UpsertEpisodes updates existing saved episodes (matched by EpisodeKey) in place and
// appends any that are new. Unlike SaveEpisodesToFile, it overwrites existing records —
// used to write back LibraryPaths after a torrent is organized into the library.
func (m *FileManager) UpsertEpisodes(episodes []EpisodeStruct) error {
	if len(episodes) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load existing episodes: %w", err)
	}

	updates := make(map[EpisodeKey]EpisodeStruct, len(episodes))
	for _, ep := range episodes {
		updates[ep.Key()] = ep
	}

	result := make([]EpisodeStruct, 0, len(existing)+len(episodes))
	seen := make(map[EpisodeKey]bool, len(existing))
	for _, ep := range existing {
		if updated, ok := updates[ep.Key()]; ok {
			result = append(result, updated)
			seen[ep.Key()] = true
		} else {
			result = append(result, ep)
		}
	}
	for _, ep := range episodes {
		if !seen[ep.Key()] {
			result = append(result, ep)
			seen[ep.Key()] = true
		}
	}

	return m.saveEpisodesLocked(result)
}

// saveEpisodesLocked saves episodes in JSONL format. Must be called with m.mu held.
func (m *FileManager) saveEpisodesLocked(episodes []EpisodeStruct) error {
	content, err := SerializeEpisodes(episodes)
	if err != nil {
		return fmt.Errorf("failed to serialize episodes: %w", err)
	}

	if err := m.writeAtomic(m.episodesPath, []byte(content)); err != nil {
		return fmt.Errorf("failed to write episodes to file: %w", err)
	}

	return nil
}

func (m *FileManager) DeleteEpisodesFromFile(keys []EpisodeKey) error {
	if len(keys) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	savedEpisodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}

	if len(savedEpisodes) == 0 {
		return nil
	}

	toDelete := make(map[EpisodeKey]struct{}, len(keys))
	for _, k := range keys {
		toDelete[k] = struct{}{}
	}

	var newSaved []EpisodeStruct
	for _, ep := range savedEpisodes {
		if _, found := toDelete[ep.Key()]; !found {
			newSaved = append(newSaved, ep)
		}
	}

	if len(newSaved) == len(savedEpisodes) {
		return nil
	}

	return m.saveEpisodesLocked(newSaved)
}

// DeleteEmptyFolders removes leftover anime directories from the library: empty ones, and
// ones that only contain `.nfo` files (tvshow.nfo plus Jellyfin episode sidecars). A
// subdirectory, poster, or any other leftover keeps the folder. The download directory
// lives inside the library (see Config.DownloadPath), and rain allocates <download>/<id>
// before writing any byte — that's why the sweep skips it explicitly, otherwise it would
// delete freshly added torrents.
func (m *FileManager) DeleteEmptyFolders(completedAnimeSaveFolder string) error {
	if completedAnimeSaveFolder == "" {
		return fmt.Errorf("completed anime path cannot be empty")
	}

	if err := m.deleteEmptyFolders(completedAnimeSaveFolder); err != nil {
		return fmt.Errorf("failed to delete empty folders in completed anime save folder: %w", err)
	}

	return nil
}

func (m *FileManager) LoadBlockedEpisodes() ([]EpisodeKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadBlockedEpisodesLocked()
}

// loadIntListLocked reads a state file that is just a JSON array of ids (standalone_animes). A
// missing file is an empty list, not an error: it's the normal initial state. Must be called with
// m.mu held.
func (m *FileManager) loadIntListLocked(path, what string) ([]int, error) {
	_, err := m.fs.Stat(path)
	if os.IsNotExist(err) {
		return []int{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat %s file: %w", what, err)
	}

	b, err := m.fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s file: %w", what, err)
	}

	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil {
		return nil, fmt.Errorf("failed to parse %s file: %w", what, err)
	}

	return ids, nil
}

// saveIntListLocked must be called with m.mu held.
func (m *FileManager) saveIntListLocked(path string, ids []int, what string) error {
	if ids == nil {
		ids = []int{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", what, err)
	}
	if err := m.writeAtomic(path, b); err != nil {
		return fmt.Errorf("failed to write %s file: %w", what, err)
	}
	return nil
}

// loadBlockedEpisodesLocked reads blocked_episodes, a JSON array of EpisodeKey. Must be called with
// m.mu held.
//
// The file in the old format was an array of AniList node ids (`[416348, ...]`). Those ids no
// longer exist anywhere in the code, so there's no way to convert them: the legacy file is
// DISCARDED with a warning, and the user loses the manual blocks they had. Blocking is one click
// per episode on the detail screen; migrating by guesswork would be worse than redoing it.
func (m *FileManager) loadBlockedEpisodesLocked() ([]EpisodeKey, error) {
	_, err := m.fs.Stat(m.blockedEpisodesPath)
	if os.IsNotExist(err) {
		return []EpisodeKey{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat blocked episodes file: %w", err)
	}

	b, err := m.fs.ReadFile(m.blockedEpisodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read blocked episodes file: %w", err)
	}

	var keys []EpisodeKey
	if err := json.Unmarshal(b, &keys); err != nil {
		var legacyIDs []int
		if json.Unmarshal(b, &legacyIDs) == nil {
			logger.Logger.Warn().
				Int("count", len(legacyIDs)).
				Msg("Discarding legacy blocked_episodes file: AniList airing-node ids can no longer be resolved to episodes")
			return []EpisodeKey{}, nil
		}
		return nil, fmt.Errorf("failed to parse blocked episodes file: %w", err)
	}

	return keys, nil
}

func (m *FileManager) BlockEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys, err := m.loadBlockedEpisodesLocked()
	if err != nil {
		return err
	}

	for _, k := range keys {
		if k == key {
			return nil // already blocked
		}
	}

	return m.saveBlockedEpisodesLocked(append(keys, key))
}

func (m *FileManager) UnmanageEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	episodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return err
	}

	found := false
	for i, ep := range episodes {
		if ep.Key() == key {
			episodes[i].ManuallyManaged = false
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	return m.saveEpisodesLocked(episodes)
}

func (m *FileManager) UnblockEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys, err := m.loadBlockedEpisodesLocked()
	if err != nil {
		return err
	}

	filtered := make([]EpisodeKey, 0, len(keys))
	for _, k := range keys {
		if k != key {
			filtered = append(filtered, k)
		}
	}

	if len(filtered) == len(keys) {
		return nil // not found, nothing to do
	}

	return m.saveBlockedEpisodesLocked(filtered)
}

// saveBlockedEpisodesLocked must be called with m.mu held.
func (m *FileManager) saveBlockedEpisodesLocked(keys []EpisodeKey) error {
	if keys == nil {
		keys = []EpisodeKey{}
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("failed to marshal blocked episodes: %w", err)
	}
	if err := m.writeAtomic(m.blockedEpisodesPath, b); err != nil {
		return fmt.Errorf("failed to write blocked episodes file: %w", err)
	}
	return nil
}

func (m *FileManager) loadAllAnimeSettings() (map[int]AnimeSettings, error) {
	_, err := m.fs.Stat(m.animeSettingsPath)
	if os.IsNotExist(err) {
		return map[int]AnimeSettings{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat anime settings file: %w", err)
	}

	b, err := m.fs.ReadFile(m.animeSettingsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read anime settings file: %w", err)
	}

	var settings map[int]AnimeSettings
	if err := json.Unmarshal(b, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse anime settings file: %w", err)
	}

	return settings, nil
}

func (m *FileManager) saveAllAnimeSettings(settings map[int]AnimeSettings) error {
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal anime settings: %w", err)
	}
	if err := m.writeAtomic(m.animeSettingsPath, b); err != nil {
		return fmt.Errorf("failed to write anime settings file: %w", err)
	}
	return nil
}

func (m *FileManager) LoadAnimeSettings(animeID int) (*AnimeSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAllAnimeSettings()
	if err != nil {
		return nil, err
	}

	s, ok := all[animeID]
	if !ok {
		return &AnimeSettings{}, nil
	}
	return &s, nil
}

func (m *FileManager) SaveAnimeSettings(animeID int, settings AnimeSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAllAnimeSettings()
	if err != nil {
		return err
	}

	all[animeID] = settings
	return m.saveAllAnimeSettings(all)
}

func (m *FileManager) LoadAllAnimeSettings() (map[int]AnimeSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadAllAnimeSettings()
}

func (m *FileManager) deleteEmptyFolders(path string) error {
	entries, err := m.fs.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read save path: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if entry.Name() == downloadDirName {
			continue
		}

		folderPath := filepath.Join(path, entry.Name())
		subEntries, err := m.fs.ReadDir(folderPath)
		if err != nil {
			logger.Logger.Warn().
				Err(err).
				Str("folder_path", folderPath).
				Msg("Failed to read folder while deleting empty folders")
			continue
		}

		if !nfoOnlyFolder(subEntries) {
			continue
		}

		failed := false
		for _, sub := range subEntries {
			filePath := filepath.Join(folderPath, sub.Name())
			if err := m.fs.Remove(filePath); err != nil {
				logger.Logger.Warn().
					Err(err).
					Str("path", filePath).
					Msg("Failed to delete leftover nfo")
				failed = true
			}
		}
		if failed {
			continue
		}

		if err := m.fs.Remove(folderPath); err != nil {
			logger.Logger.Warn().
				Err(err).
				Str("folder_path", folderPath).
				Msg("Failed to delete leftover library folder")
		} else {
			logger.Logger.Info().
				Str("folder_path", folderPath).
				Msg("Deleted leftover library folder")
		}
	}

	return nil
}

// nfoOnlyFolder reports whether every remaining entry is a file whose extension is `.nfo`
// (case-insensitive). Zero entries is the same rule: the folder is leftover.
func nfoOnlyFolder(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() {
			return false
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".nfo") {
			return false
		}
	}
	return true
}

// LoadAnilistCache reads the snapshot of the AniList caches. A missing file returns (nil, nil): a
// first run isn't a failure, and anilist treats nil as "start without a cache".
//
// It doesn't validate the content. A corrupt snapshot is the problem of whoever knows the schema —
// anilist discards it and goes on without a cache, which is an acceptable degradation for a file
// that exists only to survive the restart.
func (m *FileManager) LoadAnilistCache() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := m.fs.Stat(m.anilistCachePath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat anilist cache file: %w", err)
	}

	b, err := m.fs.ReadFile(m.anilistCachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read anilist cache file: %w", err)
	}
	return b, nil
}

func (m *FileManager) SaveAnilistCache(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.writeAtomic(m.anilistCachePath, data); err != nil {
		return fmt.Errorf("failed to write anilist cache file: %w", err)
	}
	return nil
}
