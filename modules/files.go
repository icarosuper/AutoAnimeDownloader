package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type EpisodeStruct struct {
	EpisodeID   int    `json:"episode_id"`
	EpisodeHash string `json:"episode_hash"`
	EpisodeName string `json:"episode_name"`
}

const configsFolder = ".autoAnimeDownloader"
const configFileName = ".config.json"
const downloadedEpsFileName = ".downloaded_episodes"

type Config struct {
	SavePath              string `json:"save_path"`
	AnilistUsername       string `json:"anilist_username"`
	CheckInterval         int    `json:"check_interval"`
	QBittorrentUrl        string `json:"qbittorrent_url"`
	MaxEpisodesPerAnime   int    `json:"max_episodes_per_anime"`
	EpisodeRetryLimit     int    `json:"episode_retry_limit"`
	DeleteWatchedEpisodes bool   `json:"delete_watched_episodes"`
	ExcludedList          string `json:"excluded_list"`
}

func LoadConfigs() Config {
	config := Config{
		SavePath:              "",
		AnilistUsername:       "",
		CheckInterval:         10,
		QBittorrentUrl:        "http://127.0.0.1:8080",
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     5,
		DeleteWatchedEpisodes: true,
		ExcludedList:          "",
	}

	configPath := getConfigsFilePath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		SaveConfigs(config)
	} else {
		file, err := os.ReadFile(configPath)
		if err != nil {
			panic(err)
		}

		err = json.Unmarshal(file, &config)
		if err != nil {
			panic(err)
		}

		SaveConfigs(config)
	}

	return config
}

func SaveConfigs(config Config) {
	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}

	configsPath := getConfigsFilePath()

	err = os.WriteFile(configsPath, file, 0644)
	if err != nil {
		panic(err)
	}
}

func LoadSavedEpisodes() []EpisodeStruct {
	downloadedEpsFilePath := getDownloadedEpsFilePath()

	if _, err := os.Stat(downloadedEpsFilePath); os.IsNotExist(err) {
		return []EpisodeStruct{}
	}

	b, err := os.ReadFile(downloadedEpsFilePath)
	if err != nil {
		panic(err)
	}

	content := string(b)
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return []EpisodeStruct{}
	}

	var savedEpisodes []EpisodeStruct
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		hash := parts[1]
		name := ""
		if len(parts) == 3 {
			name = parts[2]
		}
		savedEpisodes = append(savedEpisodes, EpisodeStruct{
			EpisodeID:   id,
			EpisodeHash: hash,
			EpisodeName: name,
		})
	}

	return savedEpisodes
}

func SaveEpisodesToFile(episodes []EpisodeStruct) {
	downloadedEpsFilePath := getDownloadedEpsFilePath()

	if _, err := os.Stat(downloadedEpsFilePath); os.IsNotExist(err) {
		f, err := os.Create(downloadedEpsFilePath)
		if err != nil {
			panic(err)
		}
		_ = f.Close()
	}

	f, err := os.OpenFile(downloadedEpsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	for _, episode := range episodes {
		var line string
		if episode.EpisodeName == "" {
			line = fmt.Sprintf("%d:%s\n", episode.EpisodeID, episode.EpisodeHash)
		} else {
			line = fmt.Sprintf("%d:%s:%s\n", episode.EpisodeID, episode.EpisodeHash, episode.EpisodeName)
		}
		if _, err := f.WriteString(line); err != nil {
			panic(err)
		}
	}
}

func DeleteEpisodesFromFile(episodeIds []int) {
	savedEpisodes := LoadSavedEpisodes()
	if len(episodeIds) == 0 || len(savedEpisodes) == 0 {
		return
	}

	ids := make(map[int]struct{}, len(episodeIds))
	for _, id := range episodeIds {
		ids[id] = struct{}{}
	}

	var newSaved []EpisodeStruct
	for _, ep := range savedEpisodes {
		if _, found := ids[ep.EpisodeID]; !found {
			newSaved = append(newSaved, ep)
		}
	}

	if len(newSaved) == len(savedEpisodes) {
		return
	}

	f, err := os.Create(getDownloadedEpsFilePath())
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	for _, episode := range newSaved {
		var line string
		if episode.EpisodeName == "" {
			line = fmt.Sprintf("%d:%s\n", episode.EpisodeID, episode.EpisodeHash)
		} else {
			line = fmt.Sprintf("%d:%s:%s\n", episode.EpisodeID, episode.EpisodeHash, episode.EpisodeName)
		}
		if _, err := f.WriteString(line); err != nil {
			panic(err)
		}
	}
}

func getConfigsFilePath() string {
	return filepath.Join(getConfigsFolderPath(), configFileName)
}

func getDownloadedEpsFilePath() string {
	return filepath.Join(getConfigsFolderPath(), downloadedEpsFileName)
}

func getConfigsFolderPath() string {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	configsFolderPath := filepath.Join(baseFolder, configsFolder)

	if _, err := os.Stat(configsFolderPath); os.IsNotExist(err) {
		if err := os.Mkdir(configsFolderPath, 0755); err != nil {
			panic(err)
		}
	}

	return configsFolderPath
}
