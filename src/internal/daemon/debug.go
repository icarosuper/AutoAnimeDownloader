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
// matched — without touching the torrent client or episodes.json. No download
// is attempted. Returns a DebugSummary for the caller to persist alongside the
// log trace.
//
// ponytail: episodes are always treated as not-yet-downloaded (no
// episodes.json / torrent-client check). Add it if this tool starts producing
// false positives in practice.
func RunAnimeDebug(animeId int, configs *files.Config, fileManager FileManagerInterface) (*DebugSummary, error) {
	logger.Logger.Warn().Msg("Debug mode: episodes are always treated as not-yet-downloaded. Results may differ from a real run for that reason.")

	details, err := resolveAnimeDetails(fileManager, animeId, configs)
	if err != nil {
		return nil, err
	}

	if raw, err := json.Marshal(details.mediaList); err == nil {
		logger.Logger.Info().RawJSON("anilist_response", raw).Msg("Fetched anime from AniList")
	}

	anime := details.mediaList
	animeTitle := details.title

	summary := &DebugSummary{AnimeID: animeId, AnimeName: animeTitle}

	// Mirrors processAnimeEpisodes: the synthetic list, not the raw schedule nodes. Iterating
	// AiringSchedule.Nodes here would report a pipeline the daemon doesn't run — on One Piece,
	// "searched 1123..1147" when production searches from 1. savedEpisodes is nil on purpose: debug
	// already states it treats every episode as not downloaded, and without it firstEpisodeToConsider
	// resolves to Progress+1.
	episodes := anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))

	maxEpisodes := effectiveMax(configs, episodes)

	// The SAME function as production: debug reported the wrong pipeline back when this part was a
	// copy kept in sync by hand. savedEpisodes/torrents/keepSet/blocked are empty because debug
	// already states it treats every episode as not downloaded.
	sel := selectEpisodes(configs, maxEpisodes, anime, episodes, nil, nil, nil, nil, nil)

	wouldSearch := make(map[int]bool, len(sel.toDownload))
	for _, ep := range sel.toDownload {
		wouldSearch[ep.Episode] = true
	}
	for _, ep := range episodes {
		summary.Episodes = append(summary.Episodes, EpisodeDebugResult{
			Episode:     ep.Episode,
			WouldSearch: wouldSearch[ep.Episode],
		})
	}
	episodesToDownload := sel.toDownload

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

	searcher := defaultNyaaSearcher()

	// Same flow as production, without the second selection: debug has no saved records to account for
	// (it already states it treats every episode as not downloaded).
	var magnetsForEpisodes map[int]resolvedMagnets
	if isAnimeMovie(anime) {
		episodesToDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, episodesToDownload, searcher)
	}
	if magnetsForEpisodes == nil {
		packs, singles, _ := partitionSearchResults(configs, anime, rankSeriesResults(searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(episodesToDownload)), details.isFinished))
		if !isAnimeMovie(anime) && len(episodesToDownload) > 1 {
			firstPending := episodesToDownload[0].Episode
			// Same numbering axis as production: without it debug would report "no pack covers" for every
			// entry whose pack is numbered by season or by series (packAxis).
			axis := newPackAxis(anime, resolveSeriesIndex([]int{animeId}, nil), mediaTotalEpisodes(anime))
			if batches := pickBatches(newPackSet(packsThatFit(configs, nil, packs), searcher.packRange), axis, firstPending, windowEnd(configs, firstPending)); batches.len() > 0 {
				episodesToDownload, magnetsForEpisodes = assignBatches(animeTitle, axis, episodesToDownload, batches)
			}
		}
		if magnetsForEpisodes == nil {
			magnetsForEpisodes = magnetsByEpisode(singles, episodesToDownload)
		}
	}

	// Series size, only so the search can decide the zero-padding of the episode query.
	seriesLength := anilist.LastAiredEpisode(anime)

	magnetsByEpisodeNumber := make(map[int]int, len(episodesToDownload))
	for _, ep := range episodesToDownload {
		resolved := magnetsForEpisodes[ep.Episode]
		magnets := resolved.candidates

		// Same fallback as processAnimeEpisodes: the multi search carries no episode number in the query,
		// and in a long series the first pages sorted by seeders are only the recent episodes.
		// searchSingleEpisode is what carries the zero-padding (decisions.md #56). Without this call
		// debug reported "0 magnets" on One Piece/Naruto because it hadn't searched, not because Nyaa
		// didn't have them.
		if len(magnets) == 0 {
			magnets, _ = filterSearchResults(rankSeriesResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, seriesLength), details.isFinished), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
		}

		magnetsByEpisodeNumber[ep.Episode] = len(magnets)
		logger.Logger.Info().
			Str("anime", animeTitle).
			Int("episode", ep.Episode).
			Int("magnets_found", len(magnets)).
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
