package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
)

// mediaListFromDetail adapts the single-anime GetAnimeInfo response into the
// anilist.MediaList shape that checkEpisode/resolveSearchStrategy expect.
// CustomLists is left empty — GetAnimeInfo doesn't fetch per-anime custom-list
// membership, so excluded-list checks always evaluate false here.
// ponytail: known gap, not fixed — called out in RunAnimeDebug's warning banner.
func mediaListFromDetail(d anilist.MediaListDetail) anilist.MediaList {
	episodes := d.Media.Episodes
	return anilist.MediaList{
		Id:       d.Id,
		Status:   d.Status,
		Progress: d.Progress,
		Media: anilist.Media{
			Status:         d.Media.Status,
			Format:         d.Media.Format,
			Title:          d.Media.Title,
			Episodes:       &episodes,
			Synonyms:       d.Media.Synonyms,
			Relations:      d.Media.Relations,
			CoverImage:     d.Media.CoverImage,
			AiringSchedule: d.Media.AiringSchedule,
		},
	}
}

// DebugSummary is the machine-readable summary of a RunAnimeDebug pass,
// written to summary.json by the caller.
type DebugSummary struct {
	AnimeID   int                  `json:"anime_id"`
	AnimeName string               `json:"anime_name"`
	Episodes  []EpisodeDebugResult `json:"episodes"`
}

// EpisodeDebugResult is the per-episode outcome. MagnetsFound is only
// meaningful when WouldSearch is true — episodes skipped (not aired,
// already watched, etc.) always report 0. The skip reason itself is in
// debug.jsonl (shouldSkipEpisode already logs it), not duplicated here.
type EpisodeDebugResult struct {
	Episode      int  `json:"episode"`
	WouldSearch  bool `json:"would_search"`
	MagnetsFound int  `json:"magnets_found"`
}

// RunAnimeDebug reproduces the real search/match pipeline for a single anime
// and logs the AniList response, every raw Nyaa row, and which results
// matched — without touching qBittorrent or episodes.json. No download is
// attempted. Returns a DebugSummary for the caller to persist alongside the
// log trace.
//
// ponytail: episodes are always treated as not-yet-downloaded (no
// episodes.json / qBittorrent check) and excluded-list membership is never
// checked (GetAnimeInfo doesn't return it). Add both if this tool starts
// producing false positives in practice.
func RunAnimeDebug(animeId int, configs *files.Config, fileManager FileManagerInterface) (*DebugSummary, error) {
	logger.Logger.Warn().Msg("Debug mode: episodes are always treated as not-yet-downloaded, and excluded-list membership is not checked. Results may differ from a real run for those two reasons.")

	details, err := resolveAnimeDetails(animeId)
	if err != nil {
		return nil, err
	}

	if raw, err := json.Marshal(details.mediaList); err == nil {
		logger.Logger.Info().RawJSON("anilist_response", raw).Msg("Fetched anime from AniList")
	}

	anime := mediaListFromDetail(details.mediaList)
	animeTitle := getAnimeTitleSafe(anime)

	customQuery := ""
	if settings, err := fileManager.LoadAnimeSettings(animeId); err == nil && settings != nil {
		customQuery = settings.CustomSearchQuery
	}

	summary := &DebugSummary{AnimeID: animeId, AnimeName: animeTitle}

	downloadedEpisodes := 0
	var episodesToDownload []anilist.AiringNode
	for _, ep := range anime.Media.AiringSchedule.Nodes {
		shouldDownload, _ := checkEpisode(configs, ep, anime, false, &downloadedEpisodes, false, false)
		summary.Episodes = append(summary.Episodes, EpisodeDebugResult{
			Episode:     ep.Episode,
			WouldSearch: shouldDownload,
		})
		if shouldDownload {
			episodesToDownload = append(episodesToDownload, ep)
		}
	}

	logger.Logger.Info().
		Str("anime", animeTitle).
		Int("episodes_would_search", len(episodesToDownload)).
		Msg("Episodes selected for search (before Nyaa)")

	if len(episodesToDownload) == 0 {
		logger.Logger.Warn().
			Str("anime", animeTitle).
			Msg("No episodes selected to search — check progress/excluded-list/airing-time in the AniList response above")
		return summary, nil
	}

	magnetsForEpisodes := resolveSearchStrategy(anime, animeTitle, episodesToDownload, customQuery, defaultNyaaSearcher())

	magnetsByEpisodeNumber := make(map[int]int, len(episodesToDownload))
	for _, ep := range episodesToDownload {
		resolved := magnetsForEpisodes[ep.ID]
		magnetsByEpisodeNumber[ep.Episode] = len(resolved.magnets)
		logger.Logger.Info().
			Str("anime", animeTitle).
			Int("episode", ep.Episode).
			Int("magnets_found", len(resolved.magnets)).
			Msg("Debug result for episode")
	}

	for i := range summary.Episodes {
		if n, ok := magnetsByEpisodeNumber[summary.Episodes[i].Episode]; ok {
			summary.Episodes[i].MagnetsFound = n
		}
	}

	return summary, nil
}

var debugDirPattern = regexp.MustCompile(`^\.debug_(\d+)_(\d+)$`)

// NextDebugDir returns the next available ".debug_<animeId>_N" directory
// name (not yet created) for animeId inside baseDir, scanning for existing
// ones so repeated debug runs don't clobber each other's output.
func NextDebugDir(baseDir string, animeId int) (string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", baseDir, err)
	}

	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := debugDirPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil || id != animeId {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}

	return fmt.Sprintf(".debug_%d_%d", animeId, max+1), nil
}

// WriteDebugSummary marshals summary to <dir>/summary.json.
func WriteDebugSummary(dir string, summary *DebugSummary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug summary: %w", err)
	}

	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write debug summary: %w", err)
	}

	return nil
}
