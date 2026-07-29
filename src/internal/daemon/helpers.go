package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type FileManagerInterface interface {
	LoadConfigs() (*files.Config, error)
	SaveConfigs(config *files.Config) error
	LoadSavedEpisodes() ([]files.EpisodeStruct, error)
	SaveEpisodesToFile(episodes []files.EpisodeStruct) error
	UpsertEpisodes(episodes []files.EpisodeStruct) error
	DeleteEpisodesFromFile(episodeIds []int) error
	DeleteEmptyFolders(completedAnimeSaveFolder string) error
	LoadBlockedEpisodes() ([]int, error)
	BlockEpisode(episodeID int) error
	UnblockEpisode(episodeID int) error
	UnmanageEpisode(episodeID int) error
	LoadAllAnimeSettings() (map[int]files.AnimeSettings, error)
	LoadAnimeSettings(animeID int) (*files.AnimeSettings, error)
	SaveAnimeSettings(animeID int, settings files.AnimeSettings) error
}

// HandleTorrentFailure reacts to a torrent the embedded client stopped with an error.
//
// rain leaves a failed torrent Stopped *inside* the session and never restarts it, and the
// per-torrent listener goroutine exits after NotifyStop. Left alone, episodeInTorrents would
// keep seeing the hash and the daemon would believe the episode was downloaded forever. So:
// fire the download_failed webhook (the anime name/episode come from the saved record joined
// by infohash) and drop the torrent from the session, which makes episodeInTorrents false on
// the next pass so the verification loop re-adds the magnet — retry with the machinery that
// already exists. See decisions.md for the accepted re-add-loop risk on a genuinely dead torrent.
func HandleTorrentFailure(hash string, cause error, backend torrents.TorrentBackend, fileManager FileManagerInterface) {
	logger.Logger.Warn().Err(cause).Str("hash", hash).Msg("Torrent stopped with error")

	animeName := ""
	episodeNumber := 0
	if saved, err := fileManager.LoadSavedEpisodes(); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Msg("Torrent failure: failed to load saved episodes for the webhook")
	} else {
		for _, ep := range saved {
			if ep.EpisodeHash == hash {
				animeName = ep.AnimeName
				episodeNumber = ep.EpisodeNumber
				break
			}
		}
	}

	reason := notifications.ReasonDownloadRejected
	if cause != nil {
		reason = cause.Error()
	}

	if configs, err := fileManager.LoadConfigs(); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Msg("Torrent failure: failed to load configs, skipping webhook")
	} else {
		notifications.Notify(configs, notifications.DownloadFailed, animeName, episodeNumber, reason)
	}

	if backend == nil {
		return
	}
	// keepData=false: the partial data is useless and the loop will re-download from scratch.
	if err := backend.Remove(hash, false); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Msg("Torrent failure: failed to remove the failed torrent from the session")
	}
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
	// SavePath e legado: uma instalacao migrada tem esse campo zerado, entao a checagem
	// usa o caminho de download derivado (Config.DownloadPath), nao SavePath diretamente.
	return len(config.AnilistUsernames) > 0 && config.DownloadPath() != ""
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

func buildTorrentsHashSet(t []torrents.TorrentInfo) map[string]bool {
	m := make(map[string]bool, len(t))
	for _, torrent := range t {
		if torrent.Hash != "" {
			m[torrent.Hash] = true
		}
	}
	return m
}

// episodeInTorrents reports whether a saved episode's torrent is still present in the
// embedded torrent client. The join is always by infohash (never by name), which is
// definitive even for batch torrents whose display name doesn't match the anime title.
func episodeInTorrents(savedHash string, torrentsHashSet map[string]bool) bool {
	return savedHash != "" && torrentsHashSet[savedHash]
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

// ExtractAnimeSeasonPart extrai os números de season e part dos dados do Anilist.
// Prioridade: english → romaji → synonyms. Cada campo é lido de forma independente —
// english pode dar season e synonym pode dar part.
func ExtractAnimeSeasonPart(title anilist.Title, synonyms []string) (season, part *int) {
	candidates := make([]string, 0, 2+len(synonyms))
	if title.English != nil {
		candidates = append(candidates, *title.English)
	}
	if title.Romaji != nil {
		candidates = append(candidates, *title.Romaji)
	}
	candidates = append(candidates, synonyms...)

	for _, s := range candidates {
		if season == nil {
			season = nyaa.ExtractSeason(s)
		}
		if part == nil {
			part = nyaa.ExtractPart(s)
		}
		if season != nil && part != nil {
			break
		}
	}
	return
}

// ComputeEpisodeOffset retorna o total de episódios do PREQUEL quando part >= 2.
// Usado para converter o progresso relativo do Anilist no número absoluto usado
// por fansubs com numeração contínua (ex: SubsPlease).
func ComputeEpisodeOffset(relations anilist.MediaRelations, part *int) int {
	if part == nil || *part < 2 {
		return 0
	}
	for _, edge := range relations.Edges {
		if edge.RelationType == "PREQUEL" && edge.Node.Episodes != nil {
			return *edge.Node.Episodes
		}
	}
	return 0
}

func isInDeleteStatuses(deleteStatuses []string, status anilist.MediaListStatus) bool {
	for _, s := range deleteStatuses {
		if s == string(status) {
			return true
		}
	}
	return false
}
