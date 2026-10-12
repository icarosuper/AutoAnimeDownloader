package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
	"time"
)

type AnimeEpisodeInfo struct {
	// EpisodeNumber is the episode's identity in the /animes/{id}/episodes/{episodeNumber}/* routes:
	// the AniList node id left the scene because it doesn't exist for an episode outside the schedule
	// window AniList keeps (see decisions.md #52).
	EpisodeNumber     int    `json:"episode_number"`
	AiringAt          int64  `json:"airing_at"`
	TimeUntilAiring   int    `json:"time_until_airing"`
	IsAired           bool   `json:"is_aired"`
	IsWatched         bool   `json:"is_watched"`
	IsDownloaded      bool   `json:"is_downloaded"`
	DownloadDate      string `json:"download_date,omitempty"`
	IsManuallyManaged bool   `json:"is_manually_managed,omitempty"`
	IsBlocked         bool   `json:"is_blocked,omitempty"`
	// EpisodeHash is the saved episode's torrent info hash. It is what lets the anime detail
	// screen join in live torrent progress exactly: a batch torrent's saved episodes all carry
	// the same hash but episode_number is meaningless for matching a torrent to them (the
	// torrent itself has no single episode number), so joining by number would miss batches
	// entirely. Joining by hash instead, single and batch episodes use the same path. Empty
	// (and omitted via omitempty) when the episode has no saved record.
	EpisodeHash string `json:"episode_hash,omitempty"`
	// BatchStart/BatchEnd are the REAL range of the pack this episode belongs to, read from the
	// torrent name at download time. The screen can't deduce it from the list: already-watched
	// episodes don't become records, so the min/max of one hash's records shrinks the pack (a 01-11
	// downloaded with 5 watched showed up as "6-11"). A pack with no range in its name ("(Season
	// 1+OVA) [Batch]") counts as complete and spans 1..total. Omitted when 0 — an old record, or not
	// even the AniList total is known —, and then the min/max is the best guess again.
	BatchStart int `json:"batch_start,omitempty"`
	BatchEnd   int `json:"batch_end,omitempty"`
}

type AnimeDetailResponse struct {
	// AnimeID is the AniList MEDIA id — the same key used in episodes.json and in the /animes/{id}/*
	// routes, and also the id in the anilist.co/anime/{id} link (see decisions.md #43).
	AnimeID       int                `json:"anime_id"`
	TotalEpisodes int                `json:"total_episodes"`
	Progress      int                `json:"progress"`
	Status        string             `json:"status"`
	CoverImage    string             `json:"cover_image,omitempty"`
	Episodes      []AnimeEpisodeInfo `json:"episodes"`
}

func handleAnimeEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		mediaList, err := resolveMediaList(server.FileManager, id, config.AnilistUsernames, config.MALUsernames, loadStandaloneSet(server.FileManager))
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to fetch anime detail from AniList")
			JSONInternalError(w, err)
			return
		}
		if mediaList == nil {
			JSONError(w, http.StatusNotFound, "ANIME_NOT_FOUND", "Anime is not in any configured AniList account")
			return
		}

		allEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		blockedKeys, err := server.FileManager.LoadBlockedEpisodes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to load blocked episodes")
			blockedKeys = []files.EpisodeKey{}
		}
		blockedSet := make(map[files.EpisodeKey]bool, len(blockedKeys))
		for _, k := range blockedKeys {
			blockedSet[k] = true
		}

		type downloadedInfo struct {
			date            string
			manuallyManaged bool
			hash            string
			batchStart      int
			batchEnd        int
		}
		downloadedByEpisode := make(map[int]downloadedInfo)
		for _, ep := range allEpisodes {
			if ep.AnimeID == id {
				downloadedByEpisode[ep.EpisodeNumber] = downloadedInfo{
					date:            ep.DownloadDate.Format(time.RFC3339),
					manuallyManaged: ep.ManuallyManaged,
					hash:            ep.EpisodeHash,
					batchStart:      ep.BatchStart,
					batchEnd:        ep.BatchEnd,
				}
			}
		}

		// The list comes from the builder, not from the raw airingSchedule: the screen also has to show
		// the episodes AniList already dropped from the schedule (One Piece 1 to 1122, a whole old
		// anime), otherwise the user has nowhere to click to download them by hand.
		allNodes := anilist.EpisodeList(*mediaList, 1)
		episodes := make([]AnimeEpisodeInfo, 0, len(allNodes))

		for _, node := range allNodes {
			info := AnimeEpisodeInfo{
				EpisodeNumber:   node.Episode,
				AiringAt:        node.AiringAt,
				TimeUntilAiring: node.TimeUntilAiring,
				IsAired:         node.TimeUntilAiring <= 0,
				IsWatched:       node.Episode <= mediaList.Progress,
				IsBlocked:       blockedSet[files.EpisodeKey{AnimeID: id, Episode: node.Episode}],
			}

			if downloaded, ok := downloadedByEpisode[node.Episode]; ok {
				info.IsDownloaded = true
				info.DownloadDate = downloaded.date
				info.IsManuallyManaged = downloaded.manuallyManaged
				info.EpisodeHash = downloaded.hash
				info.BatchStart = downloaded.batchStart
				info.BatchEnd = downloaded.batchEnd
			}

			episodes = append(episodes, info)
		}

		totalEpisodes := 0
		if mediaList.Media.Episodes != nil {
			totalEpisodes = *mediaList.Media.Episodes
		}

		coverImage := mediaList.Media.CoverImage.Large
		if coverImage == "" {
			coverImage = mediaList.Media.CoverImage.Medium
		}

		response := AnimeDetailResponse{
			AnimeID:       id,
			TotalEpisodes: totalEpisodes,
			Progress:      mediaList.Progress,
			Status:        string(mediaList.Status),
			CoverImage:    coverImage,
			Episodes:      episodes,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
