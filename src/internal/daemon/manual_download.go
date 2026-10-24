package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"fmt"
	"time"
)

// addAndPrioritize adds the magnet and puts it at the front of the download queue.
//
// A download asked for by hand has to start now, not when the queue gets to it — but it still
// respects max_concurrent_downloads: instead of becoming one more active torrent, it demotes the
// least advanced automatic one. A limit the UI shows being broken is a limit nobody trusts.
//
// A Prioritize failure is only logged: the torrent is already in the session and will download
// anyway, just in its turn — aborting the whole download because of that would be worse. sizeBytes
// is the size Nyaa announced; 0 when the magnet was pasted by the user and there's no way to know
// the size before the metadata.
func addAndPrioritize(backend torrents.TorrentBackend, magnet string, sizeBytes int64, configs *files.Config) (string, error) {
	// The disk space guard lives here (and not in torrents.Session.Add) because the torrents package
	// doesn't know files.Config — passing the config there just to read a percentage would invert the
	// dependency. This is the only Add of the manual paths.
	if err := checkDiskSpace(configs, sizeBytes, 0); err != nil {
		return "", err
	}
	hash, err := backend.Add(magnet)
	if err != nil || hash == "" {
		return hash, err
	}
	if err := backend.Prioritize(hash); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).
			Msg("Manual download added but could not be prioritized; it will start when the queue reaches it")
	}
	return hash, nil
}

type animeDetails struct {
	mediaList  anilist.MediaList
	title      string
	isFinished bool
}

// resolveAnimeDetails fetches an anime by MEDIA id, collapsing the configured accounts (AniList and
// MyAnimeList), and falls back to GetMediaByID when no account tracks it.
//
// The fallback is what makes the per-episode buttons (download, redownload, replace) work on a
// standalone anime: its detail screen opens — resolveMediaList already covered that —, but every
// action died here with "not in any account". With no accounts configured, which became a valid
// state, that was the path of EVERY manual download.
//
// There's no check against the standalone file here, unlike api.resolveMediaList: these functions
// already require a media id and an episode id only the detail screen provides, and the result is
// always saved with ManuallyManaged=true — the loop never deletes it for not tracking the anime. An
// id that doesn't exist on AniList still returns an error.
//
// The fallback (ml == nil) is the same synthetic path as appendStandaloneAnimes: it goes through
// withStandaloneProgress to inject the saved progress. Without that RunAnimeDebug reported a
// standalone anime with saved progress as if it started at episode 1 — the debug pipeline diverging
// from the real pipeline it exists to mirror. For the ManualDownload* callers the effect is
// harmless: they select the episode by explicit number (findEpisodeNode), not by Progress.
func resolveAnimeDetails(fm FileManagerInterface, animeId int, configs *files.Config) (*animeDetails, error) {
	ml, err := anilist.GetAnimeInfo(animeId, configs.AnilistUsernames, configs.MALUsernames, anilist.PriorityCritical)
	// The local cache serves: what's fetched here is title, relations and status to build the Nyaa
	// query, and Nyaa didn't go down with AniList.
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, fmt.Errorf("failed to get anime info: %w", err)
	}
	if ml == nil {
		if ml, err = anilist.GetMediaByID(animeId, anilist.PriorityCritical); err != nil && !errors.Is(err, anilist.ErrFromCache) {
			return nil, fmt.Errorf("failed to get anime info: %w", err)
		}
		ml = withStandaloneProgress(fm, ml)
	}
	if ml == nil {
		return nil, fmt.Errorf("anime %d does not exist on AniList", animeId)
	}

	title := ""
	if ml.Media.Title.English != nil && *ml.Media.Title.English != "" {
		title = *ml.Media.Title.English
	} else if ml.Media.Title.Romaji != nil {
		title = *ml.Media.Title.Romaji
	}

	return &animeDetails{
		mediaList:  *ml,
		title:      title,
		isFinished: ml.Media.Status == anilist.MediaStatusFinished,
	}, nil
}

// findEpisodeNode finds the episode by NUMBER in the anime's full list — not in the raw
// airingSchedule, which has no node for an episode outside the window AniList keeps. Without this
// the per-episode buttons died with "episode not found" precisely on the animes where the screen
// shows synthetic episodes (see anilist.EpisodeList and decisions.md #52).
func findEpisodeNode(ml anilist.MediaList, episodeNumber int) *anilist.AiringNode {
	for _, node := range anilist.EpisodeList(ml, 1) {
		if node.Episode == episodeNumber {
			n := node
			return &n
		}
	}
	return nil
}

// ManualDownloadEpisodeWithMagnet downloads a specific episode using a user-supplied magnet link.
// Skips Nyaa search entirely. Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisodeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, episodeNumber int, magnet string, configs *files.Config) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList, episodeNumber)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeNumber, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)
	hash, err := addAndPrioritize(backend, magnet, 0, configs)
	if err != nil || hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	return files.EpisodeStruct{
		AnimeID:         animeId,
		AnimeName:       details.title,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		EpisodeNumber:   targetNode.Episode,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}

// ManualDownloadAnimeWithMagnet downloads an entire anime using a user-supplied batch magnet link.
// Marks all aired episodes as downloaded sharing the same torrent hash.
func ManualDownloadAnimeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, magnet string, configs *files.Config) ([]files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return nil, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs)
	if err != nil {
		return nil, err
	}

	hash, err := addAndPrioritize(backend, magnet, 0, configs)
	if err != nil || hash == "" {
		return nil, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	now := time.Now()
	var episodes []files.EpisodeStruct
	for _, node := range anilist.EpisodeList(details.mediaList, 1) {
		if node.TimeUntilAiring > 0 {
			continue
		}
		epName := fmt.Sprintf("%s - Episode %d", details.title, node.Episode)
		episodes = append(episodes, files.EpisodeStruct{
			AnimeID:         animeId,
			AnimeName:       details.title,
			EpisodeHash:     hash,
			EpisodeName:     epName,
			EpisodeNumber:   node.Episode,
			IsBatch:         true,
			DownloadDate:    now,
			ManuallyManaged: true,
		})
	}

	if len(episodes) == 0 {
		return nil, fmt.Errorf("no aired episodes found for anime %d", animeId)
	}

	return episodes, nil
}

// ManualDownloadEpisode downloads a specific episode manually (called from API).
// Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisode(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, episodeNumber int, configs *files.Config) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList, episodeNumber)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeNumber, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)

	// Checked before the Nyaa search so the handler gets ErrInsufficientDiskSpace instead of the
	// generic "failed after N attempts" (which would become a 500 instead of a 409).
	if err := checkDiskSpace(configs, 0, 0); err != nil {
		return files.EpisodeStruct{}, err
	}

	results := rankSeriesResults(searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, nil, anilist.MediaRelations{}, anilist.LastAiredEpisode(details.mediaList)), details.isFinished)
	if len(results) == 0 {
		return files.EpisodeStruct{}, fmt.Errorf("no torrents found for episode %d", targetNode.Episode)
	}

	maxAttempts := min(configs.EpisodeRetryLimit, len(results))
	var hash string
	for i := range maxAttempts {
		h, err := addAndPrioritize(backend, results[i].MagnetLink, results[i].Size, configs)
		if err == nil && h != "" {
			hash = h
			break
		}
	}

	if hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to download episode after %d attempts", maxAttempts)
	}

	return files.EpisodeStruct{
		AnimeID:         animeId,
		AnimeName:       details.title,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		EpisodeNumber:   targetNode.Episode,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}
