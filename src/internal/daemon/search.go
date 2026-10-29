package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"sync"
	"time"
)

// dropStats counts what the search filters discarded in one call.
//
// It exists because "the list came back empty" has three different causes and only the counter tells
// them apart: the last verification's report must be able to say "eight candidates, all above 3 GB"
// instead of "no torrent found" — which is true too, and is the least actionable answer of the three.
// Input is how many came in, BEFORE any filter.
type dropStats struct {
	Input     int
	BySize    int
	BySeeders int
}

type nyaaSearchFunc func(title string) ([]nyaa.TorrentResult, error)

type nyaaSearcher struct {
	searchAnime         func(titles anilist.Title, synonyms []string, episodes []int) []nyaa.TorrentResult
	searchSingleEpisode func(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, totalEpisodes int) []nyaa.TorrentResult
	searchMovie         func(titles anilist.Title, isFormatMovie bool) []nyaa.TorrentResult
	// packRange reads a pack's REAL range from the Nyaa detail page. Separate from the searches
	// because it isn't one: it runs after the filter and the sorting, only for the pack the name
	// doesn't resolve, and only for the few the choice actually considers (see packSet).
	// totalEpisodes is the AniList entry total (isolated total+1 extras, decisions.md #113).
	packRange func(tr nyaa.TorrentResult, totalEpisodes int) (nyaa.BatchInfo, bool)
	// misses holds the per-episode searches that came back empty, so a dead episode isn't asked
	// again every check_interval (see searchMisses). nil = search every episode every time: the
	// manual download and the tests.
	misses *searchMisses
}

func defaultNyaaSearcher() nyaaSearcher {
	return nyaaSearcher{
		searchAnime:         searchNyaaForAnime,
		searchSingleEpisode: searchNyaaForSingleEpisode,
		searchMovie:         searchNyaaForMovie,
		packRange: func(tr nyaa.TorrentResult, total int) (nyaa.BatchInfo, bool) {
			return nyaa.PackFileRange(tr.DetailURL, total)
		},
		misses: episodeMisses,
	}
}

const (
	missFirstWait = time.Hour
	missMaxWait   = 24 * time.Hour
)

// searchMisses remembers, per episode, when its per-episode search may run again after coming back
// empty. The wait doubles on each miss (1h, 2h, 4h ... up to 24h): a swarm that's dead now is dead
// ten minutes from now too, and asking again every check_interval was most of the pass's traffic
// against Nyaa (decisions.md #100). The anime search still runs every pass, so a pack or a live
// episode that shows up is caught there without waiting.
//
// Package state in memory, like stalledTorrents: a restart forgets the misses, and the price is
// one full search per episode.
type searchMisses struct {
	mu    sync.Mutex
	byKey map[files.EpisodeKey]searchMiss
}

type searchMiss struct {
	count   int
	retryAt time.Time
	// stats is what the last real search cut, so a skipped episode keeps the same report row.
	stats dropStats
}

var episodeMisses = &searchMisses{byKey: map[files.EpisodeKey]searchMiss{}}

// packDetail binds the AniList entry total into packRange. Nil when tests leave packRange unset,
// so packSet spends no detail budget.
func packDetail(searcher nyaaSearcher, totalEpisodes int) func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
	if searcher.packRange == nil {
		return nil
	}
	return func(tr nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		return searcher.packRange(tr, totalEpisodes)
	}
}

// due reports whether the episode may be searched now, and the stats of its last miss.
func (m *searchMisses) due(key files.EpisodeKey, now time.Time) (bool, dropStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	miss, ok := m.byKey[key]
	return !ok || !now.Before(miss.retryAt), miss.stats
}

func (m *searchMisses) record(key files.EpisodeKey, now time.Time, stats dropStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	miss := m.byKey[key]
	miss.count++
	wait := missMaxWait
	if miss.count <= 5 {
		wait = min(missFirstWait<<(miss.count-1), missMaxWait)
	}
	miss.retryAt = now.Add(wait)
	miss.stats = stats
	m.byKey[key] = miss
}

func (m *searchMisses) clear(key files.EpisodeKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byKey, key)
}

// filterBySize discards torrents above maxGB (GiB). maxGB <= 0 turns the filter off.
//
// It runs AFTER the priority sort and keeps the order, so the chosen one is still the best among
// those that fit. Size == 0 passes: it's nyaa.parseSize's value when Nyaa changes the column format,
// and discarding an unknown size would trade "sometimes downloads a big torrent" for "downloads
// nothing" — a parsing bug would become a silent standstill.
func filterBySize(results []nyaa.TorrentResult, maxGB float64) ([]nyaa.TorrentResult, int) {
	if maxGB <= 0 || len(results) == 0 {
		return results, 0
	}
	maxBytes := int64(maxGB * 1024 * 1024 * 1024)
	filtered := make([]nyaa.TorrentResult, 0, len(results))
	for _, tr := range results {
		if tr.Size == 0 {
			logger.Logger.Debug().Str("torrent", tr.Name).Msg("Size filter: unknown size, passing without check")
			filtered = append(filtered, tr)
			continue
		}
		if tr.Size > maxBytes {
			logger.Logger.Debug().
				Str("torrent", tr.Name).
				Int64("size_bytes", tr.Size).
				Int64("max_bytes", maxBytes).
				Msg("Size filter: discarding torrent above the size ceiling")
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered, len(results) - len(filtered)
}

// filterBySeeders discards torrents with fewer than minSeeders. minSeeders <= 0 turns the filter off.
//
// Same contract as filterBySize (runs after the sort, keeps the order). Here, unlike the size, an
// unreadable seeders count counts as 0 and is DISCARDED with the default of 1: Nyaa's seeders column
// is the only sign that someone is seeding, and a torrent without a seeder never downloads — letting
// it pass trades "doesn't pick this one" for "locks the episode on a dead torrent", which is exactly
// what happened with the only candidate for Naruto Shippuuden episode 3 (0 peers).
func filterBySeeders(results []nyaa.TorrentResult, minSeeders int) ([]nyaa.TorrentResult, int) {
	if minSeeders <= 0 || len(results) == 0 {
		return results, 0
	}
	filtered := make([]nyaa.TorrentResult, 0, len(results))
	for _, tr := range results {
		if seeders := nyaa.ParseSeeders(tr.Seeders); seeders < minSeeders {
			logger.Logger.Debug().
				Str("torrent", tr.Name).
				Int("seeders", seeders).
				Int("min_seeders", minSeeders).
				Msg("Seeders filter: discarding torrent below the seeders floor")
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered, len(results) - len(filtered)
}

// filterSearchResults applies the two search filters (size ceiling + seeders floor) in the order the
// four search points need them, and returns what each one cut.
func filterSearchResults(results []nyaa.TorrentResult, maxGB float64, minSeeders int) ([]nyaa.TorrentResult, dropStats) {
	bySize, sizeDropped := filterBySize(results, maxGB)
	final, seedersDropped := filterBySeeders(bySize, minSeeders)
	return final, dropStats{Input: len(results), BySize: sizeDropped, BySeeders: seedersDropped}
}

func buildTitleVariants(titles anilist.Title) []string {
	var romaji, english string
	if titles.Romaji != nil {
		romaji = *titles.Romaji
	}
	if titles.English != nil {
		english = *titles.English
	}
	return nyaa.GenerateSearchTitleVariants(romaji, english)
}

func searchNyaaWithVariants(titles anilist.Title, searchFn nyaaSearchFunc, logLabel string) []nyaa.TorrentResult {
	variants := buildTitleVariants(titles)

	for i, variant := range variants {
		logger.Logger.Debug().
			Str("title", variant).
			Int("attempt", i+1).
			Int("total_attempts", len(variants)).
			Msgf("Attempting %s search with title variant", logLabel)

		result, err := searchFn(variant)
		if err != nil {
			logger.Logger.Debug().
				Err(err).
				Str("title", variant).
				Int("attempt", i+1).
				Msgf("Error searching Nyaa for %s", logLabel)
			continue
		}
		if result != nil {
			logger.Logger.Info().
				Str("title", variant).
				Int("torrents_found", len(result)).
				Int("attempt", i+1).
				Msgf("Found %s torrents on Nyaa", logLabel)
			return result
		}
	}

	return nil
}

// totalEpisodes goes to nyaa only to decide the query's zero-padding (0 = unknown).
func searchNyaaForSingleEpisode(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, totalEpisodes int) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)

	results := searchNyaaWithVariants(titles, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaa(title, ep.Episode, season, part, totalEpisodes)
	}, "single episode")

	if len(results) > 0 {
		return results
	}

	// Offset fallback: converts relative progress into the absolute number for fansubs with
	// continuous numbering. Only applies when part >= 2 (mandatory gate).
	if offset := ComputeEpisodeOffset(relations, part); offset > 0 {
		results = searchNyaaWithVariants(titles, func(title string) ([]nyaa.TorrentResult, error) {
			return nyaa.ScrapNyaa(title, ep.Episode+offset, season, nil, totalEpisodes)
		}, "single episode (offset fallback)")
	}

	return results
}

func searchNyaaForMovie(titles anilist.Title, isFormatMovie bool) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMovie(title, isFormatMovie)
	}, "movie")
}

// searchNyaaForAnime is the single search per anime: it returns packs and episodes in the same list
// (see nyaa.ScrapNyaaForAnime).
func searchNyaaForAnime(titles anilist.Title, synonyms []string, episodes []int) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)
	return searchNyaaWithVariants(titles, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForAnime(title, episodes, season, part)
	}, "anime")
}

func rankSeriesResults(results []nyaa.TorrentResult, finished bool) []nyaa.TorrentResult {
	if finished {
		return nyaa.SortFinishedSeriesResults(results)
	}
	return results
}
