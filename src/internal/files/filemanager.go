package files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const configsFolder = ".autoAnimeDownloader"
const configFileName = "config.json"
const downloadedEpsFileName = "downloaded_episodes"

type EpisodeStruct struct {
	EpisodeID   int    `json:"episode_id"`
	EpisodeHash string `json:"episode_hash"`
	EpisodeName string `json:"episode_name"`
}

type Config struct {
	SavePath              string `json:"save_path"`
	CompletedAnimePath    string `json:"completed_anime_path"`
	AnilistUsername       string `json:"anilist_username"`
	CheckInterval         int    `json:"check_interval"`
	QBittorrentUrl        string `json:"qbittorrent_url"`
	MaxEpisodesPerAnime   int    `json:"max_episodes_per_anime"`
	EpisodeRetryLimit     int    `json:"episode_retry_limit"`
	DeleteWatchedEpisodes bool   `json:"delete_watched_episodes"`
	ExcludedList          string `json:"excluded_list"`
}

type FileManager struct {
	fs           FileSystem
	configPath   string
	episodesPath string
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
		ExcludedList:          "",
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

func NewManager(fs FileSystem, configPath, episodesPath string) *FileManager {
	return &FileManager{
		fs:           fs,
		configPath:   configPath,
		episodesPath: episodesPath,
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

	return NewManager(fs, configPath, episodesPath), nil
}

func (m *FileManager) LoadConfigs() (*Config, error) {
	config := getDefaultConfig()

	_, err := m.fs.Stat(m.configPath)
	if os.IsNotExist(err) {
		if err := m.SaveConfigs(config); err != nil {
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

	if err := json.Unmarshal(file, config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if err := m.SaveConfigs(config); err != nil {
		return nil, fmt.Errorf("failed to update config file: %w", err)
	}

	return config, nil
}

func (m *FileManager) SaveConfigs(config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := m.fs.WriteFile(m.configPath, file, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
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

	return episodes, nil
}

func (m *FileManager) SaveEpisodesToFile(episodes []EpisodeStruct) error {
	if len(episodes) == 0 {
		return nil
	}

	var existingContent []byte
	existingContent, err := m.fs.ReadFile(m.episodesPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing episodes file: %w", err)
	}

	newContent := SerializeEpisodes(episodes)

	finalContent := append(existingContent, []byte(newContent)...)

	if err := m.fs.WriteFile(m.episodesPath, finalContent, 0644); err != nil {
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

	content := SerializeEpisodes(newSaved)
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
			fmt.Printf("Warning: failed to read folder %s: %v\n", folderPath, err)
			continue
		}

		if len(subEntries) == 0 {
			if err := m.fs.Remove(folderPath); err != nil {
				fmt.Printf("Warning: failed to delete empty folder %s: %v\n", folderPath, err)
			} else {
				fmt.Printf("Deleted empty folder: %s\n", folderPath)
			}
		}
	}

	return nil
}
