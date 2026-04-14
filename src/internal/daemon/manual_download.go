package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"fmt"
	"time"
)

type animeDetails struct {
	mediaList  anilist.MediaListDetail
	title      string
	isFinished bool
}

func resolveAnimeDetails(animeId int) (*animeDetails, error) {
	detail, err := anilist.GetAnimeInfo(animeId)
	if err != nil {
		return nil, fmt.Errorf("failed to get anime info: %w", err)
	}

	ml := detail.Data.MediaList
	title := ""
	if ml.Media.Title.English != nil && *ml.Media.Title.English != "" {
		title = *ml.Media.Title.English
	} else if ml.Media.Title.Romaji != nil {
		title = *ml.Media.Title.Romaji
	}

	return &animeDetails{
		mediaList:  ml,
		title:      title,
		isFinished: ml.Media.Status == anilist.MediaStatusFinished,
	}, nil
}

func findEpisodeNode(nodes []anilist.AiringNode, episodeId int) *anilist.AiringNode {
	for _, node := range nodes {
		if node.ID == episodeId {
			n := node
			return &n
		}
	}
	return nil
}

func newTorrentServiceFromConfig(configs *files.Config) *torrents.TorrentService {
	qBittorrentURL := getQBittorrentURL(configs.QBittorrentUrl)
	return torrents.NewTorrentService(&torrents.DefaultHTTPClient{}, qBittorrentURL, configs.SavePath, configs.CompletedAnimePath)
}

// ManualDownloadEpisodeWithMagnet downloads a specific episode using a user-supplied magnet link.
// Skips Nyaa search entirely. Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisodeWithMagnet(animeId int, episodeId int, magnet string, configs *files.Config) (files.EpisodeStruct, error) {
	ts := newTorrentServiceFromConfig(configs)

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList.Media.AiringSchedule.Nodes, episodeId)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeId, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)
	hash := ts.DownloadTorrentWithOptions(magnet, details.title, epName, details.isFinished, false)
	if hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to add torrent to qBittorrent")
	}

	return files.EpisodeStruct{
		EpisodeID:       episodeId,
		AnimeID:         animeId,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}

// ManualDownloadAnimeWithMagnet downloads an entire anime using a user-supplied batch magnet link.
// Marks all aired episodes as downloaded sharing the same torrent hash.
func ManualDownloadAnimeWithMagnet(animeId int, magnet string, configs *files.Config) ([]files.EpisodeStruct, error) {
	ts := newTorrentServiceFromConfig(configs)

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return nil, err
	}

	hash := ts.DownloadTorrentWithOptions(magnet, details.title, details.title, details.isFinished, false)
	if hash == "" {
		return nil, fmt.Errorf("failed to add torrent to qBittorrent")
	}

	now := time.Now()
	var episodes []files.EpisodeStruct
	for _, node := range details.mediaList.Media.AiringSchedule.Nodes {
		if node.TimeUntilAiring > 0 {
			continue
		}
		epName := fmt.Sprintf("%s - Episode %d", details.title, node.Episode)
		episodes = append(episodes, files.EpisodeStruct{
			EpisodeID:       node.ID,
			AnimeID:         animeId,
			EpisodeHash:     hash,
			EpisodeName:     epName,
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
func ManualDownloadEpisode(animeId int, episodeId int, configs *files.Config, customQuery string) (files.EpisodeStruct, error) {
	ts := newTorrentServiceFromConfig(configs)

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList.Media.AiringSchedule.Nodes, episodeId)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeId, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)

	results := searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, customQuery)
	var magnets []string
	for _, result := range results {
		magnets = append(magnets, result.MagnetLink)
	}

	if len(magnets) == 0 {
		return files.EpisodeStruct{}, fmt.Errorf("no torrents found for episode %d", targetNode.Episode)
	}

	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))
	var hash string
	for i := range maxAttempts {
		h := ts.DownloadTorrentWithOptions(magnets[i], details.title, epName, details.isFinished, false)
		if h != "" {
			hash = h
			break
		}
	}

	if hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to download episode after %d attempts", maxAttempts)
	}

	return files.EpisodeStruct{
		EpisodeID:       episodeId,
		AnimeID:         animeId,
		AnimeName:       details.title,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}
