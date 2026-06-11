package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
)

type nyaaSearchFunc func(title string) ([]nyaa.TorrentResult, error)

type nyaaSearcher struct {
	searchBatch         func(titles anilist.Title, synonyms []string, customQuery string) []nyaa.TorrentResult
	searchSingleEpisode func(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, customQuery string) []nyaa.TorrentResult
	searchMovie         func(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult
	searchMultiple      func(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult
}

func defaultNyaaSearcher() nyaaSearcher {
	return nyaaSearcher{
		searchBatch:         searchNyaaForBatch,
		searchSingleEpisode: searchNyaaForSingleEpisode,
		searchMovie:         searchNyaaForMovie,
		searchMultiple:      searchNyaaForMultipleEpisodes,
	}
}

func buildTitleVariants(titles anilist.Title, customQuery string) []string {
	if customQuery != "" {
		return []string{customQuery}
	}
	var romaji, english string
	if titles.Romaji != nil {
		romaji = *titles.Romaji
	}
	if titles.English != nil {
		english = *titles.English
	}
	return nyaa.GenerateSearchTitleVariants(romaji, english)
}

func searchNyaaWithVariants(titles anilist.Title, customQuery string, searchFn nyaaSearchFunc, logLabel string) []nyaa.TorrentResult {
	variants := buildTitleVariants(titles, customQuery)

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

func searchNyaaForSingleEpisode(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, customQuery string) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)

	results := searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaa(title, ep.Episode, season, part)
	}, "single episode")

	if len(results) > 0 {
		return results
	}

	// Fallback com offset: converte progresso relativo em número absoluto para fansubs
	// com numeração contínua. Só aplica quando part >= 2 (gate obrigatório).
	if offset := ComputeEpisodeOffset(relations, part); offset > 0 {
		results = searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
			return nyaa.ScrapNyaa(title, ep.Episode+offset, season, nil)
		}, "single episode (offset fallback)")
	}

	return results
}

func searchNyaaForBatch(titles anilist.Title, synonyms []string, customQuery string) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForBatch(title, season, part)
	}, "batch")
}

func searchNyaaForMovie(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMovie(title, isFormatMovie)
	}, "movie")
}

func searchNyaaForMultipleEpisodes(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMultipleEpisodes(title, episodes, season, part)
	}, "multiple episodes")
}
