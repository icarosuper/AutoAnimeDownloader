package modules

import (
	"encoding/json"
	"os"
)

const configFilePath = "config.json"

type Config struct {
	SavePath        string `json:"save_path"`
	SkipDialog      bool   `json:"skip_dialog"`
	MaxEpisodes     int    `json:"max_episodes"`
	AnilistUsername string `json:"anilist_username"`
}

func LoadConfigs() Config {
	config := Config{
		SavePath:        "~/Downloads/Animes",
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
