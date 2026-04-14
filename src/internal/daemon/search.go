package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
)

type nyaaSearchFunc func(title string) ([]nyaa.TorrentResult, error)

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

func searchNyaaForSingleEpisode(ep anilist.AiringNode, titles anilist.Title, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaa(title, ep.Episode)
	}, "single episode")
}

func searchNyaaForBatch(titles anilist.Title, requestedSeason *int, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForBatch(title, requestedSeason)
	}, "batch")
}

func searchNyaaForMovie(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMovie(title, isFormatMovie)
	}, "movie")
}

func searchNyaaForMultipleEpisodes(titles anilist.Title, episodes []int, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMultipleEpisodes(title, episodes)
	}, "multiple episodes")
}
