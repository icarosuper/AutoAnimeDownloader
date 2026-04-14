package files

import (
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const configsFolder = ".autoAnimeDownloader"
const configFileName = "config.json"
const downloadedEpsFileName = "downloaded_episodes"
const blockedEpsFileName = "blocked_episodes"
const animeSettingsFileName = "anime_settings"

type EpisodeStruct struct {
	EpisodeID          int       `json:"episode_id"`
	AnimeID            int       `json:"anime_id,omitempty"`
	AnimeTotalEpisodes int       `json:"anime_total_episodes,omitempty"`
	AnimeName          string    `json:"anime_name,omitempty"`
	EpisodeHash        string    `json:"episode_hash"`
	EpisodeName        string    `json:"episode_name"`
	DownloadDate       time.Time `json:"download_date"`
	ManuallyManaged    bool      `json:"manually_managed,omitempty"`
}

type Config struct {
	SavePath               string   `json:"save_path"`
	CompletedAnimePath     string   `json:"completed_anime_path"`
	AnilistUsername        string   `json:"anilist_username"`
	CheckInterval          int      `json:"check_interval"`
	QBittorrentUrl         string   `json:"qbittorrent_url"`
	MaxEpisodesPerAnime    int      `json:"max_episodes_per_anime"`
	EpisodeRetryLimit      int      `json:"episode_retry_limit"`
	DeleteWatchedEpisodes  bool     `json:"delete_watched_episodes"`
	WatchedEpisodesToKeep  int      `json:"watched_episodes_to_keep"`
	ExcludedList           string   `json:"excluded_list,omitempty"`
	ExcludedLists          []string `json:"excluded_lists"`
	RenameFilesForJellyfin bool     `json:"rename_files_for_jellyfin"`
	DownloadStatuses       []string `json:"download_statuses"`
	DeleteStatuses         []string `json:"delete_statuses"`
}

type AnimeSettings struct {
	CustomSearchQuery string `json:"custom_search_query,omitempty"`
}

type FileManager struct {
	fs                  FileSystem
	configPath          string
	episodesPath        string
	blockedEpisodesPath string
	animeSettingsPath   string
	mu                  sync.Mutex
}

func getDefaultConfig() *Config {
	return &Config{
		SavePath:              "",
		AnilistUsername:       "",
		CheckInterval:         10,
		QBittorrentUrl:        "http://127.0.0.1:8080",
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
		DeleteWatchedEpisodes: true,
		WatchedEpisodesToKeep: 0,
		ExcludedLists:         []string{},
		DownloadStatuses:      []string{"CURRENT", "REPEATING"},
		DeleteStatuses:        []string{},
	}
}

func ensureConfigsFolder(fs FileSystem) (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	configsFolderPath := filepath.Join(baseFolder, configsFolder)

	_, err := fs.Stat(configsFolderPath)
	if os.IsNotExist(err) {
		if err := fs.Mkdir(configsFolderPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create configs folder: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to stat configs folder: %w", err)
	}

	return configsFolderPath, nil
}

func NewManager(fs FileSystem, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath string) *FileManager {
	return &FileManager{
		fs:                  fs,
		configPath:          configPath,
		episodesPath:        episodesPath,
		blockedEpisodesPath: blockedEpisodesPath,
		animeSettingsPath:   animeSettingsPath,
	}
}

func NewDefaultFileManager() (*FileManager, error) {
	fs := NewOSFileSystem()
	configsFolderPath, err := ensureConfigsFolder(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure configs folder: %w", err)
	}

	configPath := filepath.Join(configsFolderPath, configFileName)
	episodesPath := filepath.Join(configsFolderPath, downloadedEpsFileName)
	blockedEpisodesPath := filepath.Join(configsFolderPath, blockedEpsFileName)
	animeSettingsPath := filepath.Join(configsFolderPath, animeSettingsFileName)

	return NewManager(fs, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath), nil
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
		return config, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	file, err := m.fs.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Check if file is empty or contains only whitespace
	trimmed := strings.TrimSpace(string(file))
	if len(trimmed) == 0 {
		logger.Logger.Warn().Msg("Config file is empty, recreating with default values")
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		return config, nil
	}

	// Try to parse the JSON
	if err := json.Unmarshal(file, config); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to parse config JSON, recreating with default values")
		config = getDefaultConfig()
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config after parse error: %w", err)
		}
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

	return config, nil
}

func (m *FileManager) SaveConfigs(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveConfigsLocked(config)
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

	tmpPath := m.configPath + ".tmp"
	if err := m.fs.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config temp file: %w", err)
	}

	if err := m.fs.Rename(tmpPath, m.configPath); err != nil {
		_ = m.fs.Remove(tmpPath)
		return fmt.Errorf("failed to rename config temp file: %w", err)
	}

	return nil
}

func (m *FileManager) LoadSavedEpisodes() ([]EpisodeStruct, error) {
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

	// If we loaded episodes from old format, migrate to JSON format
	if len(episodes) > 0 && episodes[0].DownloadDate.IsZero() {
		// Check if any episode has zero date (indicating old format)
		needsMigration := false
		for _, ep := range episodes {
			if ep.DownloadDate.IsZero() {
				needsMigration = true
				break
			}
		}
		if needsMigration {
			// Migrate: set download date to current time for episodes without date
			for i := range episodes {
				if episodes[i].DownloadDate.IsZero() {
					episodes[i].DownloadDate = time.Now()
				}
			}
			// Save in new format
			if err := m.saveEpisodesToFileJSON(episodes); err != nil {
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

	// Load existing episodes
	existingEpisodes, err := m.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load existing episodes: %w", err)
	}

	// Create map of existing episodes by ID to avoid duplicates
	existingMap := make(map[int]bool)
	for _, ep := range existingEpisodes {
		existingMap[ep.EpisodeID] = true
	}

	// Append only new episodes
	var newEpisodes []EpisodeStruct
	for _, ep := range episodes {
		if !existingMap[ep.EpisodeID] {
			newEpisodes = append(newEpisodes, ep)
		}
	}

	if len(newEpisodes) == 0 {
		return nil
	}

	// Append new episodes to existing ones
	allEpisodes := append(existingEpisodes, newEpisodes...)

	// Save all episodes in JSON format
	return m.saveEpisodesToFileJSON(allEpisodes)
}

// saveEpisodesToFileJSON saves episodes in JSONL format
func (m *FileManager) saveEpisodesToFileJSON(episodes []EpisodeStruct) error {
	content, err := SerializeEpisodes(episodes)
	if err != nil {
		return fmt.Errorf("failed to serialize episodes: %w", err)
	}

	if err := m.fs.WriteFile(m.episodesPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write episodes to file: %w", err)
	}

	return nil
}

func (m *FileManager) DeleteEpisodesFromFile(episodeIds []int) error {
	if len(episodeIds) == 0 {
		return nil
	}

	savedEpisodes, err := m.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}

	if len(savedEpisodes) == 0 {
		return nil
	}

	idsToDelete := make(map[int]struct{}, len(episodeIds))
	for _, id := range episodeIds {
		idsToDelete[id] = struct{}{}
	}

	var newSaved []EpisodeStruct
	for _, ep := range savedEpisodes {
		if _, found := idsToDelete[ep.EpisodeID]; !found {
			newSaved = append(newSaved, ep)
		}
	}

	if len(newSaved) == len(savedEpisodes) {
		return nil
	}

	content, err := SerializeEpisodes(newSaved)
	if err != nil {
		return fmt.Errorf("failed to serialize episodes: %w", err)
	}

	if err := m.fs.WriteFile(m.episodesPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write filtered episodes: %w", err)
	}

	return nil
}

func (m *FileManager) DeleteEmptyFolders(savePath string, completedAnimeSaveFolder string) error {
	if savePath == "" {
		return fmt.Errorf("save path cannot be empty")
	}

	if err := m.deleteEmptyFolders(savePath); err != nil {
		return fmt.Errorf("failed to delete empty folders in save path: %w", err)
	}

	if completedAnimeSaveFolder != "" {
		if err := m.deleteEmptyFolders(completedAnimeSaveFolder); err != nil {
			return fmt.Errorf("failed to delete empty folders in completed anime save folder: %w", err)
		}
	}

	return nil
}

func (m *FileManager) LoadBlockedEpisodes() ([]int, error) {
	_, err := m.fs.Stat(m.blockedEpisodesPath)
	if os.IsNotExist(err) {
		return []int{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat blocked episodes file: %w", err)
	}

	b, err := m.fs.ReadFile(m.blockedEpisodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read blocked episodes file: %w", err)
	}

	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil {
		return nil, fmt.Errorf("failed to parse blocked episodes file: %w", err)
	}

	return ids, nil
}

func (m *FileManager) BlockEpisode(episodeID int) error {
	ids, err := m.LoadBlockedEpisodes()
	if err != nil {
		return err
	}

	for _, id := range ids {
		if id == episodeID {
			return nil // already blocked
		}
	}

	ids = append(ids, episodeID)
	return m.saveBlockedEpisodes(ids)
}

func (m *FileManager) UnmanageEpisode(episodeID int) error {
	episodes, err := m.LoadSavedEpisodes()
	if err != nil {
		return err
	}

	found := false
	for i, ep := range episodes {
		if ep.EpisodeID == episodeID {
			episodes[i].ManuallyManaged = false
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	return m.saveEpisodesToFileJSON(episodes)
}

func (m *FileManager) UnblockEpisode(episodeID int) error {
	ids, err := m.LoadBlockedEpisodes()
	if err != nil {
		return err
	}

	var filtered []int
	for _, id := range ids {
		if id != episodeID {
			filtered = append(filtered, id)
		}
	}

	if len(filtered) == len(ids) {
		return nil // not found, nothing to do
	}

	return m.saveBlockedEpisodes(filtered)
}

func (m *FileManager) saveBlockedEpisodes(ids []int) error {
	if ids == nil {
		ids = []int{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal blocked episodes: %w", err)
	}
	if err := m.fs.WriteFile(m.blockedEpisodesPath, b, 0644); err != nil {
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
	if err := m.fs.WriteFile(m.animeSettingsPath, b, 0644); err != nil {
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

		folderPath := filepath.Join(path, entry.Name())
		subEntries, err := m.fs.ReadDir(folderPath)
		if err != nil {
			logger.Logger.Warn().
				Err(err).
				Str("folder_path", folderPath).
				Msg("Failed to read folder while deleting empty folders")
			continue
		}

		if len(subEntries) == 0 {
			if err := m.fs.Remove(folderPath); err != nil {
				logger.Logger.Warn().
					Err(err).
					Str("folder_path", folderPath).
					Msg("Failed to delete empty folder")
			} else {
				logger.Logger.Info().
					Str("folder_path", folderPath).
					Msg("Deleted empty folder")
			}
		}
	}

	return nil
}
