package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type EpisodeStruct struct {
	EpisodeID   int    `json:"episode_id"`
	EpisodeHash string `json:"episode_hash"`
}

const configsFolder = ".autoanimedownloader"
const configFileName = ".config.json"
const downloadedEpsFileName = ".downloaded_episodes"

type Config struct {
	SavePath              string `json:"save_path"`
	AnilistUsername       string `json:"anilist_username"`
	CheckInterval         int    `json:"check_interval"`
	QBittorrentUrl        string `json:"qbittorrent_url"`
	MaxEpisodesPerAnime   int    `json:"max_episodes_per_anime"`
	DeleteWatchedEpisodes bool   `json:"delete_watched_episodes"`
}

func LoadConfigs() Config {
	config := Config{
		SavePath:              "",
		AnilistUsername:       "",
		CheckInterval:         10,
		QBittorrentUrl:        "http://127.0.0.1:8080",
		MaxEpisodesPerAnime:   10,
		DeleteWatchedEpisodes: true,
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

	file, err := os.ReadFile(downloadedEpsFilePath)
	if err != nil {
		panic(err)
	}

	lines := string(file)
	if lines == "" {
		return []EpisodeStruct{}
	}

	var savedEpisodes []EpisodeStruct
	for _, line := range splitLines(lines) {
		var episode EpisodeStruct
		_, err := fmt.Sscanf(line, "%d:%s", &episode.EpisodeID, &episode.EpisodeHash)
		if err == nil {
			savedEpisodes = append(savedEpisodes, episode)
		}
	}

	return savedEpisodes
}

func SaveEpisodesToFile(episodes []EpisodeStruct) {
	downloadedEpsFilePath := getDownloadedEpsFilePath()

	if _, err := os.Stat(downloadedEpsFilePath); os.IsNotExist(err) {
		file, err := os.Create(downloadedEpsFilePath)
		if err != nil {
			panic(err)
		}
		file.Close()
	}

	file, err := os.OpenFile(downloadedEpsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	for _, episode := range episodes {
		if _, err := file.WriteString(fmt.Sprintf("%d:%s\n", episode.EpisodeID, episode.EpisodeHash)); err != nil {
			panic(err)
		}
	}
}

func DeleteEpisodesFromFile(episodeIds []int) {
	savedEpisodes := LoadSavedEpisodes()

	if len(episodeIds) == 0 || len(savedEpisodes) == 0 {
		return
	}

	var newSavedEpisodes []EpisodeStruct

	for _, episode := range savedEpisodes {
		shouldDelete := false
		for _, id := range episodeIds {
			if episode.EpisodeID == id {
				shouldDelete = true
				break
			}
		}
		if !shouldDelete {
			newSavedEpisodes = append(newSavedEpisodes, episode)
		}
	}

	if len(newSavedEpisodes) == len(savedEpisodes) {
		return
	}

	file, err := os.Create(getDownloadedEpsFilePath())
	if err != nil {
		panic(err)
	}
	defer file.Close()

	for _, episode := range newSavedEpisodes {
		if _, err := file.WriteString(fmt.Sprintf("%d:%s\n", episode.EpisodeID, episode.EpisodeHash)); err != nil {
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
	// TODO: Testar essa solucao no Windows
	configsFolderPath := filepath.Join(os.Getenv("HOME"), configsFolder)

	if _, err := os.Stat(configsFolderPath); os.IsNotExist(err) {
		err := os.Mkdir(configsFolderPath, 0755)
		if err != nil {
			panic(err)
		}
	}

	return configsFolderPath
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
