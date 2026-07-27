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

// ManualDownloadEpisodeWithMagnet downloads a specific episode using a user-supplied magnet link.
// Skips Nyaa search entirely. Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisodeWithMagnet(backend torrents.TorrentBackend, animeId int, episodeId int, magnet string, configs *files.Config) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList.Media.AiringSchedule.Nodes, episodeId)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeId, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)
	hash, err := backend.Add(magnet)
	if err != nil || hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	return files.EpisodeStruct{
		EpisodeID:       episodeId,
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
func ManualDownloadAnimeWithMagnet(backend torrents.TorrentBackend, animeId int, magnet string, configs *files.Config) ([]files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return nil, err
	}

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return nil, err
	}

	hash, err := backend.Add(magnet)
	if err != nil || hash == "" {
		return nil, fmt.Errorf("failed to add torrent to embedded client: %w", err)
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
func ManualDownloadEpisode(backend torrents.TorrentBackend, animeId int, episodeId int, configs *files.Config, customQuery string) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList.Media.AiringSchedule.Nodes, episodeId)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeId, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)

	results := searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, nil, anilist.MediaRelations{}, customQuery)
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
		h, err := backend.Add(magnets[i])
		if err == nil && h != "" {
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
		EpisodeNumber:   targetNode.Episode,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}
