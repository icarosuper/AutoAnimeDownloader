package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
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
	// EpisodesPending counts episodes already released, not yet watched and not yet downloaded. A
	// watched episode is never downloaded (daemon.shouldSkipEpisode), so it can't count as being
	// behind — that's what marked half the list as "behind" in the frontend.
	EpisodesPending     int    `json:"episodes_pending" example:"2"`
	TotalEpisodes       int    `json:"total_episodes" example:"12"`
	LatestEpisodeNumber int    `json:"latest_episode_number" example:"12"`
	LastDownloadDate    string `json:"last_download_date" example:"2026-02-24T10:30:00Z"`
	CoverImage          string `json:"cover_image,omitempty"`
	IsBlacklisted       bool   `json:"is_blacklisted,omitempty"`
	// IsStandalone marks the animes tracked by the standalone_animes file, not by an AniList or
	// MyAnimeList list. It's ORIGIN, not download state — that's why the screen shows it in its own
	// chip next to the derived chip, never inside it.
	IsStandalone bool `json:"is_standalone,omitempty"`
	// NextAiringAt is the unix timestamp of the next episode's premiere, 0 when none is scheduled
	// (finished/cancelled anime). It's the TIMESTAMP on purpose, not AniList's timeUntilAiring: the
	// front counts down on its own, so the number doesn't freeze along with a cached response.
	NextAiringAt int64 `json:"next_airing_at,omitempty" example:"1740394800"`
	// AltNames are the other titles of the same anime (romaji, English, native/kanji and synonyms),
	// without Name itself. It exists only so the dashboard search matches the name the user knows;
	// nothing renders this list.
	AltNames []string `json:"alt_names,omitempty" example:"ワンピース"`
}

// altNames gathers every title variant of a Media, dropping empties, duplicates and the name that
// already goes in AnimeInfo.Name. No normalization — the front does the comparing
// (lib/utils/status.ts).
func altNames(media anilist.Media, displayName string) []string {
	candidates := make([]string, 0, 3+len(media.Synonyms))
	for _, p := range []*string{media.Title.Romaji, media.Title.English, media.Title.Native} {
		if p != nil {
			candidates = append(candidates, *p)
		}
	}
	candidates = append(candidates, media.Synonyms...)

	seen := map[string]bool{strings.ToLower(strings.TrimSpace(displayName)): true}
	var out []string
	for _, c := range candidates {
		name := strings.TrimSpace(c)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

// nextAiringAt reads the next episode's timestamp. nextAiringEpisode is nil on a finished anime,
// and AniList also returns nil during the hiatus between seasons.
func nextAiringAt(next *anilist.AiringNode) int64 {
	if next == nil {
		return 0
	}
	return next.AiringAt
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
		// Episode numbers already on disk, per anime — the base of the EpisodesPending computed at the
		// end, after the AniList merge filled in released/watched.
		downloadedNums := make(map[string]map[int]bool)

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

			if episode.EpisodeNumber > 0 {
				if downloadedNums[key] == nil {
					downloadedNums[key] = make(map[int]bool)
				}
				downloadedNums[key][episode.EpisodeNumber] = true
			}

			if animeInfo, exists := animeMap[key]; exists {
				animeInfo.EpisodesDownloaded++
				if episode.EpisodeNumber > animeInfo.LatestEpisodeNumber {
					animeInfo.LatestEpisodeNumber = episode.EpisodeNumber
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
					AnimeID:             episode.AnimeID,
					Name:                displayName,
					EpisodesDownloaded:  1,
					TotalEpisodes:       episode.AnimeTotalEpisodes,
					LatestEpisodeNumber: episode.EpisodeNumber,
					LastDownloadDate:    episode.DownloadDate.Format(time.RFC3339),
				}
			}
		}

		// Merge CURRENT animes from AniList so they remain visible even with 0 downloaded episodes.
		// Tracks which AnimeIDs were covered by the filtered fetch, across all accounts, so that
		// already-downloaded animes whose current status fell outside the allowed sets (and thus
		// weren't covered) can be refreshed individually below instead of disappearing.
		var entries []anilist.MediaList
		covered := make(map[int]bool)
		mergeFailed := false
		fromCache := false
		for _, username := range config.AnilistUsernames {
			// nil (not an empty list) means the fetch failed — see fetchAniListEntries.
			list, cached := fetchAniListEntries(username, config.DownloadStatuses, config.DownloadMediaStatuses)
			if list == nil {
				mergeFailed = true
				continue
			}
			fromCache = fromCache || cached
			for _, ml := range list {
				covered[ml.Media.Id] = true
			}
			entries = append(entries, list...)
		}
		// MAL accounts follow the same contract: nil = failed, so coverage is unknown.
		for _, user := range config.MALUsernames {
			list, cached := fetchMALEntries(user, config.DownloadStatuses, config.DownloadMediaStatuses)
			if list == nil {
				mergeFailed = true
				continue
			}
			fromCache = fromCache || cached
			for _, ml := range list {
				covered[ml.Media.Id] = true
			}
			entries = append(entries, list...)
		}

		// Standalone animes go in AFTER the dedupe for the same reason as in the daemon: when the anime
		// is also in a list, the real entry (with progress) has to win over the synthetic one.
		standaloneSet := loadStandaloneSet(server.FileManager)
		entries = append(anilist.DedupeByMedia(entries), appendStandaloneEntries(server.FileManager, nil, standaloneSet, covered)...)

		mergeAniListAnimes(animeMap, entries, config.ExcludedLists)

		// With a failed list fetch there's no knowing what got covered, and treating "not covered" as
		// "needs refresh" would turn the failure into one GetAnimeInfo per downloaded anime, on every
		// frontend poll. That was exactly the feedback that pushed AniList to 429 — and the 429 made the
		// next list fail, locking the cycle.
		if mergeFailed {
			logger.Logger.Warn().Msg("Skipping orphan refresh: AniList list fetch failed, coverage unknown")
		} else if fromCache {
			// Coverage is known (the list came whole, from the cache), but AniList is down: every uncovered
			// anime would become a GetAnimeInfoByIDs doomed to fail, on every 30s poll per tab. It's the
			// same burst as decisions.md #65, now without even a chance of working.
			logger.Logger.Warn().Msg("Skipping orphan refresh: the AniList list came from the local cache")
		} else {
			refreshOrphanAnimes(server.FileManager, animeMap, covered, config.ExcludedLists, config.AnilistUsernames, config.MALUsernames, standaloneSet)
		}

		animes := make([]AnimeInfo, 0, len(animeMap))
		for key, animeInfo := range animeMap {
			animeInfo.EpisodesPending = countPendingEpisodes(animeInfo, downloadedNums[key])
			animeInfo.IsStandalone = standaloneSet[animeInfo.AnimeID]
			animes = append(animes, *animeInfo)
		}

		JSONSuccess(w, http.StatusOK, animes)
	}
}

// countPendingEpisodes counts the released episodes above the watch progress that aren't on disk
// yet. It walks by number (not released-downloaded) because watched episodes may stay saved
// (watched_episodes_to_keep) and would inflate the subtraction.
func countPendingEpisodes(info *AnimeInfo, downloaded map[int]bool) int {
	pending := 0
	for n := info.EpisodesWatched + 1; n <= info.EpisodesReleased; n++ {
		if !downloaded[n] {
			pending++
		}
	}
	return pending
}

// refreshOrphanAnimes re-fetches AniList-derived fields for already-downloaded animes whose AnimeID
// wasn't covered by the filtered mergeCurrentAniListAnimes fetch (current list/media status fell
// outside the configured allowed sets). These animes stay visible regardless — this only tries to
// keep their cover/progress/blacklist fields fresh instead of stale/blank. A failed refresh is
// logged and left as-is; it never fails the overall request.
//
// ALL the AniList reading here is batched: one query per account (per 50 ids) and, for the
// standalone animes no account tracks, a second one per batch of 50. It used to be one request PER
// orphan anime, fired on every 30s poll of every open tab — the burst decisions.md #65 measured as
// the biggest consumer of the AniList budget, and the one its burst limiter punishes. That's also
// why the previous version needed a concurrency semaphore: with two queries in total, there's
// nothing left to limit.
func refreshOrphanAnimes(fm FileManagerInterface, animeMap map[string]*AnimeInfo, covered map[int]bool, excludedLists []string, usernames, malUsers []string, standalone map[int]bool) {
	var orphans []*AnimeInfo
	var ids []int
	for _, info := range animeMap {
		if info.AnimeID != 0 && !covered[info.AnimeID] {
			orphans = append(orphans, info)
			ids = append(ids, info.AnimeID)
		}
	}
	if len(orphans) == 0 {
		return
	}

	medias, err := anilist.GetAnimeInfoByIDs(ids, usernames, malUsers)
	if err != nil {
		// Partial: the map carries what answered, and each orphan missing from it keeps the fields that
		// came from episodes.json. The standalone fallback does NOT run in this case — with a failed
		// account, "no account tracks it" is a conclusion that can't be drawn, and drawing it would spend
		// more requests right when AniList is already complaining.
		logger.Logger.Warn().Err(err).Int("orphans", len(orphans)).
			Msg("Failed to refresh some orphaned animes, keeping existing data")
	} else {
		refreshStandaloneOrphans(fm, medias, ids, standalone)
	}

	for _, info := range orphans {
		ml := medias[info.AnimeID]
		if ml == nil {
			// No account tracks this anime anymore; the episodes on disk stay listed with the fields that
			// came from episodes.json.
			continue
		}

		name, totalEpisodes, episodesReleased, coverImage, isBlacklisted := computeAnimeFields(
			ml.Media.Title, ml.Media.Status, ml.Media.Episodes, ml.Media.CoverImage, ml.Media.AiringSchedule, ml.CustomLists, excludedLists,
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
		info.NextAiringAt = nextAiringAt(ml.Media.NextAiringEpisode)
		info.AltNames = altNames(ml.Media, info.Name)
	}
}

// refreshStandaloneOrphans fills in, in medias, the orphans no account tracks but that are in the
// standalone file — the same fallback (and the same reason) as resolveMediaList, only batched.
// Without it a standalone anime outside the lists shows up again with no cover and no name.
func refreshStandaloneOrphans(fm FileManagerInterface, medias map[int]*anilist.MediaList, ids []int, standalone map[int]bool) {
	var pending []int
	for _, id := range ids {
		if medias[id] == nil && standalone[id] {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return
	}

	fetched, err := anilist.GetMediaByIDs(pending, anilist.PriorityDisposable)
	switch {
	case errors.Is(err, anilist.ErrFromCache):
		logger.Logger.Warn().Int("standalone_orphans", len(pending)).
			Msg("Refreshing standalone orphans from the local cache: AniList is unreachable")
	case err != nil:
		logger.Logger.Warn().Err(err).Int("standalone_orphans", len(pending)).
			Msg("Failed to refresh some standalone orphans, keeping existing data")
	}
	for id, ml := range fetched {
		if ml != nil {
			medias[id] = withStandaloneProgress(fm, ml)
		}
	}
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
		isBlacklisted = isInExcludedList(customLists, excludedSet)
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

// fetchAniListEntries returns one account's AniList entries, filtered by both list status
// (server-side) and media status (here), with customLists overlaid. It returns nil — not an empty
// list — when the fetch fails, so the caller can tell "no anime covered" from "coverage unknown";
// the orphan refresh depends on that. The second return says the list came from the local cache.
// It's not a logging detail: it's what keeps the orphan refresh from running against an AniList
// that doesn't answer — see the call site.
func fetchAniListEntries(username string, statuses []string, mediaStatuses []string) ([]anilist.MediaList, bool) {
	// Fetch customLists via cached minimal query before the complex query that may null it out.
	clMap := anilist.GetCustomListsMap(username, statuses, anilist.PriorityDisposable)

	resp, err := anilist.GetFrontendAnimeList(username, statuses)
	fromCache := errors.Is(err, anilist.ErrFromCache)
	if err != nil && !fromCache {
		logger.Logger.Warn().Err(err).Msg("Failed to fetch AniList current animes, skipping merge")
		return nil, false
	}

	filtered := make([]anilist.MediaList, 0, len(resp.Data.Page.MediaList))
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
	return filtered, fromCache
}

// fetchMALEntries is fetchAniListEntries for a MyAnimeList account, with the same contract: nil
// (not an empty slice) on failure, and the second return says it came from the local cache.
// An empty MAL list is a failure too (anilist.ErrMALEmpty).
func fetchMALEntries(user string, statuses []string, mediaStatuses []string) ([]anilist.MediaList, bool) {
	list, err := anilist.GetMALList(user, statuses, anilist.PriorityDisposable)
	fromCache := errors.Is(err, anilist.ErrFromCache)
	if err != nil && !fromCache {
		logger.Logger.Warn().Err(err).Str("mal_user", user).Msg("Failed to fetch MyAnimeList animes, skipping merge")
		return nil, false
	}

	filtered := make([]anilist.MediaList, 0, len(list))
	for _, ml := range list {
		if anilist.MediaStatusAllowed(mediaStatuses, ml.Media.Status) {
			filtered = append(filtered, ml)
		}
	}
	return filtered, fromCache
}

// mergeAniListAnimes merges AniList entries into animeMap so they appear even with 0 downloaded
// episodes. It never removes existing animeMap entries — an anime with downloaded episodes stays
// visible even if its current status falls outside the allowed sets (see refreshOrphanAnimes).
func mergeAniListAnimes(animeMap map[string]*AnimeInfo, filtered []anilist.MediaList, excludedLists []string) {
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

		if existing, ok := knownByID[ml.Media.Id]; ok {
			existing.Name = name
			if existing.TotalEpisodes == 0 {
				existing.TotalEpisodes = totalEpisodes
			}
			existing.EpisodesReleased = episodesReleased
			existing.EpisodesWatched = ml.Progress
			existing.CoverImage = coverImage
			existing.IsBlacklisted = isBlacklisted
			existing.NextAiringAt = nextAiringAt(ml.Media.NextAiringEpisode)
			existing.AltNames = altNames(ml.Media, name)
			continue
		}

		animeMap[name] = &AnimeInfo{
			AnimeID:          ml.Media.Id,
			Name:             name,
			EpisodesReleased: episodesReleased,
			EpisodesWatched:  ml.Progress,
			TotalEpisodes:    totalEpisodes,
			CoverImage:       coverImage,
			IsBlacklisted:    isBlacklisted,
			NextAiringAt:     nextAiringAt(ml.Media.NextAiringEpisode),
			AltNames:         altNames(ml.Media, name),
		}
	}
}
