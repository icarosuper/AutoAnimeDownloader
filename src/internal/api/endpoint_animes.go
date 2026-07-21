package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// reStripEpisodeNumber strips common episode-numbering suffixes from an episode name.
var reStripEpisodeNumber = []*regexp.Regexp{
	regexp.MustCompile(`\s*-\s*[Ee]pisode\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*[Ee]p\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*\d+.*$`),
	regexp.MustCompile(`\s+\d+.*$`),
	regexp.MustCompile(`\s*\(.*\)\s*$`),
}

type AnimeInfo struct {
	AnimeID            int    `json:"anime_id" example:"12345"`
	Name               string `json:"name" example:"Naruto"`
	EpisodesDownloaded int    `json:"episodes_downloaded" example:"8"`
	EpisodesReleased   int    `json:"episodes_released" example:"10"`
	EpisodesWatched    int    `json:"episodes_watched" example:"5"`
	TotalEpisodes      int    `json:"total_episodes" example:"12"`
	LatestEpisodeID    int    `json:"latest_episode_id" example:"12"`
	LastDownloadDate   string `json:"last_download_date" example:"2026-02-24T10:30:00Z"`
	CoverImage         string `json:"cover_image,omitempty"`
	IsBlacklisted      bool   `json:"is_blacklisted,omitempty"`
}

func extractAnimeName(episodeName string) string {
	if episodeName == "" {
		return "Unknown"
	}

	result := episodeName
	for _, pattern := range reStripEpisodeNumber {
		result = pattern.ReplaceAllString(result, "")
	}

	result = strings.TrimSpace(result)
	if result == "" {
		return episodeName
	}

	return result
}

// @Summary      Get animes list
// @Description  Returns a list of animes with aggregated episode information
// @Tags         animes
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]AnimeInfo}
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes [get]
func handleAnimes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		// Group episodes by anime.
		// Key: "id:<AnimeID>" when AnimeID is set, otherwise the extracted name (backward compat).
		animeMap := make(map[string]*AnimeInfo)

		for _, episode := range episodes {
			var key string
			if episode.AnimeID != 0 {
				key = fmt.Sprintf("id:%d", episode.AnimeID)
			} else {
				key = extractAnimeName(episode.EpisodeName)
			}

			// Prefer the persisted AnimeName; fall back to extracting from EpisodeName.
			displayName := episode.AnimeName
			if displayName == "" {
				displayName = extractAnimeName(episode.EpisodeName)
			}

			if animeInfo, exists := animeMap[key]; exists {
				animeInfo.EpisodesDownloaded++
				if episode.EpisodeID > animeInfo.LatestEpisodeID {
					animeInfo.LatestEpisodeID = episode.EpisodeID
				}
				lastDownloadedTime, _ := time.Parse(time.RFC3339, animeInfo.LastDownloadDate)
				if episode.DownloadDate.After(lastDownloadedTime) {
					animeInfo.LastDownloadDate = episode.DownloadDate.Format(time.RFC3339)
					// Update name from the most recent episode that has AnimeName set.
					if episode.AnimeName != "" {
						animeInfo.Name = episode.AnimeName
					}
				}
				if animeInfo.AnimeID == 0 && episode.AnimeID != 0 {
					animeInfo.AnimeID = episode.AnimeID
				}
				if animeInfo.TotalEpisodes == 0 && episode.AnimeTotalEpisodes != 0 {
					animeInfo.TotalEpisodes = episode.AnimeTotalEpisodes
				}
			} else {
				animeMap[key] = &AnimeInfo{
					AnimeID:            episode.AnimeID,
					Name:               displayName,
					EpisodesDownloaded: 1,
					TotalEpisodes:      episode.AnimeTotalEpisodes,
					LatestEpisodeID:    episode.EpisodeID,
					LastDownloadDate:   episode.DownloadDate.Format(time.RFC3339),
				}
			}
		}

		// Merge CURRENT animes from AniList so they remain visible even with 0 downloaded episodes.
		// Tracks which AnimeIDs were covered by the filtered fetch, across all accounts, so that
		// already-downloaded animes whose current status fell outside the allowed sets (and thus
		// weren't covered) can be refreshed individually below instead of disappearing.
		covered := make(map[int]bool)
		for _, username := range config.AnilistUsernames {
			for id := range mergeCurrentAniListAnimes(animeMap, username, config.ExcludedLists, config.DownloadStatuses, config.DownloadMediaStatuses) {
				covered[id] = true
			}
		}
		refreshOrphanAnimes(animeMap, covered, config.ExcludedLists)

		animes := make([]AnimeInfo, 0, len(animeMap))
		for _, animeInfo := range animeMap {
			animes = append(animes, *animeInfo)
		}

		JSONSuccess(w, http.StatusOK, animes)
	}
}

// maxConcurrentOrphanRefresh bounds concurrent per-anime AniList lookups for orphan refresh,
// mirroring maxConcurrentAnimes in the daemon's verification loop.
const maxConcurrentOrphanRefresh = 5

// refreshOrphanAnimes re-fetches AniList-derived fields for already-downloaded animes whose
// AnimeID wasn't covered by the filtered mergeCurrentAniListAnimes fetch (current list/media
// status fell outside the configured allowed sets). These animes stay visible regardless —
// this only tries to keep their cover/progress/blacklist fields fresh instead of stale/blank.
// A failed refresh is logged and left as-is; it never fails the overall request.
func refreshOrphanAnimes(animeMap map[string]*AnimeInfo, covered map[int]bool, excludedLists []string) {
	var orphans []*AnimeInfo
	for _, info := range animeMap {
		if info.AnimeID != 0 && !covered[info.AnimeID] {
			orphans = append(orphans, info)
		}
	}
	if len(orphans) == 0 {
		return
	}

	sem := make(chan struct{}, maxConcurrentOrphanRefresh)
	var wg sync.WaitGroup
	for _, info := range orphans {
		wg.Add(1)
		go func(info *AnimeInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			detail, err := anilist.GetAnimeInfo(info.AnimeID)
			if err != nil {
				logger.Logger.Warn().Err(err).Int("anime_id", info.AnimeID).Msg("Failed to refresh orphaned anime, keeping existing data")
				return
			}

			ml := detail.Data.MediaList
			name, totalEpisodes, episodesReleased, coverImage, isBlacklisted := computeAnimeFields(
				ml.Media.Title, ml.Media.Status, &ml.Media.Episodes, ml.Media.CoverImage, ml.Media.AiringSchedule, ml.CustomLists, excludedLists,
			)

			if name != "" {
				info.Name = name
			}
			if info.TotalEpisodes == 0 {
				info.TotalEpisodes = totalEpisodes
			}
			info.EpisodesReleased = episodesReleased
			info.EpisodesWatched = ml.Progress
			info.CoverImage = coverImage
			info.IsBlacklisted = isBlacklisted
		}(info)
	}
	wg.Wait()
}

// computeAnimeFields derives the AniList-sourced display fields shared by the batch merge loop
// (mergeCurrentAniListAnimes) and the single-anime orphan refresh (refreshOrphanAnimes).
func computeAnimeFields(title anilist.Title, status anilist.MediaStatus, episodes *int, cover anilist.CoverImage, schedule anilist.AiringSchedule, customLists anilist.CustomLists, excludedLists []string) (name string, totalEpisodes, episodesReleased int, coverImage string, isBlacklisted bool) {
	if title.English != nil && *title.English != "" {
		name = *title.English
	} else if title.Romaji != nil {
		name = *title.Romaji
	}

	if episodes != nil {
		totalEpisodes = *episodes
	}

	if len(excludedLists) > 0 {
		excludedSet := make(map[string]bool, len(excludedLists))
		for _, n := range excludedLists {
			excludedSet[n] = true
		}
		for listName, inList := range customLists {
			if excludedSet[listName] && inList {
				isBlacklisted = true
				break
			}
		}
	}

	coverImage = cover.Large
	if coverImage == "" {
		coverImage = cover.Medium
	}

	for _, node := range schedule.Nodes {
		if node.TimeUntilAiring <= 0 && node.Episode > episodesReleased {
			episodesReleased = node.Episode
		}
	}
	if episodesReleased == 0 && status == anilist.MediaStatusFinished {
		episodesReleased = totalEpisodes
	}

	return name, totalEpisodes, episodesReleased, coverImage, isBlacklisted
}

// mergeCurrentAniListAnimes merges animes fetched from AniList (filtered by both list status and
// media status) into animeMap so they appear even with 0 downloaded episodes. It returns the set
// of AnimeIDs it saw, so the caller can tell which already-downloaded animes weren't covered.
// It never removes existing animeMap entries — an anime with downloaded episodes stays visible
// even if its current status falls outside the allowed sets (see refreshOrphanAnimes).
func mergeCurrentAniListAnimes(animeMap map[string]*AnimeInfo, username string, excludedLists []string, statuses []string, mediaStatuses []string) map[int]bool {
	// Fetch customLists via cached minimal query before the complex query that may null it out.
	clMap := anilist.GetCustomListsMap(username, statuses)

	resp, err := anilist.GetFrontendAnimeList(username, statuses)
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to fetch AniList current animes, skipping merge")
		return nil
	}

	var filtered []anilist.MediaList
	for i := range resp.Data.Page.MediaList {
		ml := &resp.Data.Page.MediaList[i]
		if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 {
			ml.CustomLists = cl
		}
		if !anilist.MediaStatusAllowed(mediaStatuses, ml.Media.Status) {
			continue
		}
		filtered = append(filtered, *ml)
	}

	// Build set of covered AnimeIDs (list status filtered server-side, media status filtered above)
	covered := make(map[int]bool, len(filtered))
	for _, ml := range filtered {
		covered[ml.Id] = true
	}

	// Build map from AnimeID → *AnimeInfo pointer so we can update existing entries
	knownByID := make(map[int]*AnimeInfo)
	for _, info := range animeMap {
		if info.AnimeID != 0 {
			knownByID[info.AnimeID] = info
		}
	}

	for _, ml := range filtered {
		episodes := ml.Media.Episodes
		name, totalEpisodes, episodesReleased, coverImage, isBlacklisted := computeAnimeFields(
			ml.Media.Title, ml.Media.Status, episodes, ml.Media.CoverImage, ml.Media.AiringSchedule, ml.CustomLists, excludedLists,
		)
		if name == "" {
			continue
		}

		if existing, ok := knownByID[ml.Id]; ok {
			existing.Name = name
			if existing.TotalEpisodes == 0 {
				existing.TotalEpisodes = totalEpisodes
			}
			existing.EpisodesReleased = episodesReleased
			existing.EpisodesWatched = ml.Progress
			existing.CoverImage = coverImage
			existing.IsBlacklisted = isBlacklisted
			continue
		}

		animeMap[name] = &AnimeInfo{
			AnimeID:          ml.Id,
			Name:             name,
			EpisodesReleased: episodesReleased,
			EpisodesWatched:  ml.Progress,
			TotalEpisodes:    totalEpisodes,
			CoverImage:       coverImage,
			IsBlacklisted:    isBlacklisted,
		}
	}

	return covered
}
