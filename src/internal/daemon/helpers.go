package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type FileManagerInterface interface {
	LoadConfigs() (*files.Config, error)
	SaveConfigs(config *files.Config) error
	LoadSavedEpisodes() ([]files.EpisodeStruct, error)
	SaveEpisodesToFile(episodes []files.EpisodeStruct) error
	UpsertEpisodes(episodes []files.EpisodeStruct) error
	DeleteEpisodesFromFile(keys []files.EpisodeKey) error
	DeleteEmptyFolders(completedAnimeSaveFolder string) error
	LoadBlockedEpisodes() ([]files.EpisodeKey, error)
	BlockEpisode(key files.EpisodeKey) error
	UnblockEpisode(key files.EpisodeKey) error
	UnmanageEpisode(key files.EpisodeKey) error
	LoadAllAnimeSettings() (map[int]files.AnimeSettings, error)
	LoadAnimeSettings(animeID int) (*files.AnimeSettings, error)
	SaveAnimeSettings(animeID int, settings files.AnimeSettings) error
	LoadStandaloneAnimes() ([]int, error)
	AddStandaloneAnime(mediaID int) error
	RemoveStandaloneAnime(mediaID int) error
}

// ErrInsufficientDiskSpace is returned by checkDiskSpace when the library volume is below
// Config.MinFreeDiskPercent of free space, or when the torrent to be added doesn't fit in what's
// left.
var ErrInsufficientDiskSpace = errors.New("insufficient free disk space")

// checkDiskSpace blocks ADDING new torrents when the library volume is full OR when the requested
// torrent doesn't fit in it.
//
// torrentSizeBytes is the size Nyaa announced, 0 when unknown (hand-pasted magnet) — and then the
// guard falls back to the percentage check it always was. Without it a 61 GiB pack got into a
// volume with 40 GiB free and only stopped when the disk filled up, mid-download: the free
// percentage only answers "is there space NOW", never "is there space at the end".
//
// reservedBytes is what the torrents already added still have to write (see diskBudget): free
// space alone let a pass queue 130 GiB of loose episodes, each one fitting on its own, into 6 GiB
// of headroom (decisions.md #101). It counts the same as the torrent's own size.
//
// The two halves have different triggers on purpose. The percentage is the user's policy and turns
// off with min_free_disk_percent = 0; "the torrent is bigger than the free space" is arithmetic, and
// holds even with the policy off.
//
// It doesn't block the verification pass: pruning watched episodes, deleteEpisodesByStatus and
// organize are exactly what FREES space, and an "if disk full { return }" at the start of the pass
// would leave the app stuck in the one state it can't unclog itself from.
//
// A statfs error does NOT block: a volume that doesn't answer (network, permission) is no proof of
// a full disk, and turning it into "stop downloading everything" is worse than the risk the guard
// covers.
func checkDiskSpace(configs *files.Config, torrentSizeBytes, reservedBytes int64) error {
	needed := torrentSizeBytes + reservedBytes
	if configs.CompletedAnimePath == "" || (configs.MinFreeDiskPercent <= 0 && needed <= 0) {
		return nil
	}
	// Same volume as the download directory, by construction (see Config.DownloadPath).
	total, free, err := files.DiskSpace(configs.CompletedAnimePath)
	if err != nil {
		logger.Logger.Debug().Err(err).Msg("Disk space guard: statfs failed, not blocking downloads")
		return nil
	}
	if total == 0 {
		return nil
	}
	if needed > 0 && float64(needed) > float64(free) {
		return fmt.Errorf("%w: torrent needs %d bytes on top of %d reserved by torrents still downloading, only %d free on %s", ErrInsufficientDiskSpace, torrentSizeBytes, reservedBytes, free, configs.CompletedAnimePath)
	}
	if configs.MinFreeDiskPercent <= 0 {
		return nil
	}
	if (float64(free)-float64(needed))/float64(total)*100 < float64(configs.MinFreeDiskPercent) {
		return fmt.Errorf("%w: %d%% free required on %s (torrent: %d bytes, reserved by torrents still downloading: %d bytes)", ErrInsufficientDiskSpace, configs.MinFreeDiskPercent, configs.CompletedAnimePath, torrentSizeBytes, reservedBytes)
	}
	return nil
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
	// The bytes already downloaded STAY when there are any: the next pass's re-add uses the same id
	// (Session.Add passes AddTorrentOptions{ID: hash}) and rain lands in the same <DataDir>/<id>,
	// sees the existing files (allocator.HasExisting) and runs the verifier, which rebuilds the
	// bitfield. The download resumes where it stopped. Without this, a transient error on a 90 GiB
	// pack at 90% cost the whole download again.
	//
	// PiecesHave is the progress source that survives the Stop — BytesCompleted zeroes when rain
	// frees the pieces, and the failed torrent is already stopped here.
	//
	// Zero verified pieces -> keepData=false: there's nothing to recover, and keeping the folder
	// would only leave permanent garbage on disk when the retry lands on another infohash (the
	// directory is per torrent id, and nothing sweeps the download path).
	keepData := false
	if info, ok := backend.Get(hash); ok && info.PiecesHave > 0 {
		keepData = true
		logger.Logger.Info().Str("hash", hash).Uint32("pieces_have", info.PiecesHave).Uint32("pieces_total", info.PiecesTotal).
			Msg("Torrent failure: keeping partial data for the re-add to resume from")
	}
	if err := backend.Remove(hash, keepData); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).Msg("Torrent failure: failed to remove the failed torrent from the session")
	}
}

func getWebUiURL() string {
	return WebUIURL(os.Getenv("PORT"), "/#/config?missingConfig=true")
}

// isConfigComplete: the LIBRARY is the only requirement. An AniList account doesn't count — since
// standalone animes (decisions.md #49) the app works entirely without any list, and requiring an
// account just for the pass to run would leave that installation stuck on the config screen.
func isConfigComplete(config *files.Config) bool {
	// The download directory is derived from the library (Config.DownloadPath), so checking the
	// derived one covers the only field the user configures.
	return config.DownloadPath() != ""
}

// OpenBrowser opens the URL in the user's browser. Used by the verification pass (incomplete
// config) and by the boot (first run, see main.go).
func OpenBrowser(webUIURL string) error {
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
		Msg("Opened the Web UI in the browser")

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

func buildSavedEpisodesMap(episodes []files.EpisodeStruct) map[files.EpisodeKey]bool {
	m := make(map[files.EpisodeKey]bool, len(episodes))
	for _, episode := range episodes {
		m[episode.Key()] = true
	}
	return m
}

func buildSavedEpisodesFullMap(episodes []files.EpisodeStruct) map[files.EpisodeKey]files.EpisodeStruct {
	m := make(map[files.EpisodeKey]files.EpisodeStruct, len(episodes))
	for _, ep := range episodes {
		m[ep.Key()] = ep
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

// ExtractAnimeSeasonPart extracts the season and part numbers from the AniList data.
// Priority: english → romaji → synonyms. Each field is read independently — english can give the
// season and a synonym the part.
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

// ComputeEpisodeOffset returns the PREQUEL's episode total when part >= 2.
// Used to convert AniList's relative progress into the absolute number used by fansubs with
// continuous numbering (e.g. SubsPlease).
//
// Only a TV/TV_SHORT PREQUEL counts: recap movies, OVAs and ONAs also arrive as PREQUEL with
// Episodes filled in, but they're not part of the count fansubs follow. When in doubt it returns
// 0 — a wrong offset sends the search to a nonexistent episode, while a missing offset falls back
// to relative numbering, which is what most groups use.
//
// The same "first TV/TV_SHORT PREQUEL" selection lives in anilist.prequelOf (anilist/series.go),
// and the two stay separate on purpose: that one walks the WHOLE chain (the series' absolute
// axis), this one takes ONE hop and is gated by part >= 2. They're the two DIFFERENT pack
// numbering hypotheses in daemon.packAxis — "continuous by season" and "absolute by series" —, so
// unifying them would erase the distinction the heuristic uses (decisions.md #79).
func ComputeEpisodeOffset(relations anilist.MediaRelations, part *int) int {
	if part == nil || *part < 2 {
		return 0
	}
	for _, edge := range relations.Edges {
		if edge.RelationType != "PREQUEL" || edge.Node.Episodes == nil {
			continue
		}
		if edge.Node.Format != anilist.MediaFormatTV && edge.Node.Format != anilist.MediaFormatTVShort {
			continue
		}
		return *edge.Node.Episodes
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
