package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type FileManagerInterface interface {
	LoadConfigs() (*files.Config, error)
	SaveConfigs(config *files.Config) error
	LoadSavedEpisodes() ([]files.EpisodeStruct, error)
	SaveEpisodesToFile(episodes []files.EpisodeStruct) error
	DeleteEpisodesFromFile(episodeIds []int) error
	DeleteEmptyFolders(savePath string, completedAnimeSaveFolder string) error
	LoadBlockedEpisodes() ([]int, error)
	BlockEpisode(episodeID int) error
	UnblockEpisode(episodeID int) error
	UnmanageEpisode(episodeID int) error
	LoadAllAnimeSettings() (map[int]files.AnimeSettings, error)
	LoadAnimeSettings(animeID int) (*files.AnimeSettings, error)
	SaveAnimeSettings(animeID int, settings files.AnimeSettings) error
}

func getWebUiURL() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	} else {
		port = strings.TrimPrefix(port, ":")
	}
	return fmt.Sprintf("http://localhost:%s/#/config?missingConfig=true", port)
}

func isConfigComplete(config *files.Config) bool {
	return config.AnilistUsername != "" && config.SavePath != "" && config.QBittorrentUrl != ""
}

func openBrowserToConfig(webUIURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", webUIURL)
	case "darwin":
		cmd = exec.Command("open", webUIURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", webUIURL)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	logger.Logger.Info().
		Str("url", webUIURL).
		Msg("Opened browser to configuration page (missing required configuration)")

	return nil
}

func getQBittorrentURL(configURL string) string {
	if envURL := os.Getenv("QBITTORRENT_URL"); envURL != "" {
		return envURL
	}
	return configURL
}

func buildTorrentsMap(t []torrents.Torrent) map[string]bool {
	m := make(map[string]bool, len(t))
	for _, torrent := range t {
		m[torrent.Name] = true
	}
	return m
}

func buildSavedEpisodesMap(episodes []files.EpisodeStruct) map[int]bool {
	m := make(map[int]bool, len(episodes))
	for _, episode := range episodes {
		m[episode.EpisodeID] = true
	}
	return m
}

func buildSavedEpisodesFullMap(episodes []files.EpisodeStruct) map[int]files.EpisodeStruct {
	m := make(map[int]files.EpisodeStruct, len(episodes))
	for _, ep := range episodes {
		m[ep.EpisodeID] = ep
	}
	return m
}

func animeIsInExcludedList(anime anilist.MediaList, excludedLists []string) bool {
	if len(excludedLists) == 0 {
		return false
	}
	excludedSet := make(map[string]bool, len(excludedLists))
	for _, name := range excludedLists {
		excludedSet[name] = true
	}
	for listName, isInList := range anime.CustomLists {
		if excludedSet[listName] && isInList {
			return true
		}
	}
	return false
}

func isAnimeMovie(anime anilist.MediaList) bool {
	return anime.Media.Format == anilist.MediaFormatMovie
}

func getAnimeTitleSafe(anime anilist.MediaList) string {
	if anime.Media.Title.English != nil && *anime.Media.Title.English != "" {
		return *anime.Media.Title.English
	}
	if anime.Media.Title.Romaji != nil {
		return *anime.Media.Title.Romaji
	}
	return "Unknown"
}

func extractSeasonFromAnime(anime anilist.MediaList) *int {
	if anime.Media.Title.Romaji == nil {
		return nil
	}
	title := *anime.Media.Title.Romaji
	seasonPattern := regexp.MustCompile(`(?i)Season\s*(\d+)|S(\d{1,2})\b|(\d{1,2})(?:st|nd|rd|th)\s+Season`)

	matches := seasonPattern.FindStringSubmatch(title)
	if len(matches) > 1 {
		for i := 1; i < len(matches); i++ {
			if matches[i] != "" {
				if seasonNum, err := strconv.Atoi(matches[i]); err == nil {
					return &seasonNum
				}
			}
		}
	}

	return nil
}

func isInDeleteStatuses(deleteStatuses []string, status anilist.MediaListStatus) bool {
	for _, s := range deleteStatuses {
		if s == string(status) {
			return true
		}
	}
	return false
}
