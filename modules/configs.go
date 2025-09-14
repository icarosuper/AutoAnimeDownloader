package modules

import (
	"encoding/json"
	"fmt"
	"os"
)

const configFilePath = "config.json"
const idsFilePath = ".downloaded_ids"

type Config struct {
	SavePath        string `json:"save_path"`
	SkipDialog      bool   `json:"skip_dialog"`
	MaxEpisodes     int    `json:"max_episodes"`
	AnilistUsername string `json:"anilist_username"`
}

func LoadConfigs() Config {
	config := Config{
		SavePath:        "",
		SkipDialog:      true,
		MaxEpisodes:     10,
		AnilistUsername: "",
	}

	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		SaveConfigs(config)
	} else {
		file, err := os.ReadFile(configFilePath)
		if err != nil {
			panic(err)
		}

		err = json.Unmarshal(file, &config)
		if err != nil {
			panic(err)
		}
	}

	return config
}

func SaveConfigs(config Config) {
	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(configFilePath, file, 0644)
	if err != nil {
		panic(err)
	}
}

func LoadIdsFromFile() []string {
	if _, err := os.Stat(idsFilePath); os.IsNotExist(err) {
		file, err := os.Create(idsFilePath)
		if err != nil {
			panic(err)
		}
		file.Close()
		return []string{}
	}

	file, err := os.ReadFile(idsFilePath)
	if err != nil {
		panic(err)
	}

	ids := string(file)
	if ids == "" {
		return []string{}
	}

	return splitLines(ids)
}

func SaveIdToFile(id int) {
	if _, err := os.Stat(idsFilePath); os.IsNotExist(err) {
		file, err := os.Create(idsFilePath)
		if err != nil {
			panic(err)
		}
		file.Close()
	}

	file, err := os.OpenFile(idsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if _, err := file.WriteString(fmt.Sprintf("%d\n", id)); err != nil {
		panic(err)
	}
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
