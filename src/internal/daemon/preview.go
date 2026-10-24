package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"

	"errors"
	"fmt"
	"strings"
)

// TorrentCandidate is one ranked whole-title torrent the picker can show. Range comes from the
// name only — preview never fetches /view.
type TorrentCandidate struct {
	Name       string
	Magnet     string
	SizeBytes  int64
	Seeders    int
	Leechers   int
	Fansub     string
	Resolution string
	IsBatch    bool
	RangeStart int // ExtractBatchInfo from the NAME only. 0 = unknown. Do not fetch /view.
	RangeEnd   int
	NyaaID     int
}

const maxTorrentCandidates = 8

func PreviewWholeTitleTorrents(configs *files.Config, mediaID int, query string) (string, []TorrentCandidate, error) {
	anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return "", nil, fmt.Errorf("failed to fetch anime %d from AniList: %w", mediaID, err)
	}
	if anime == nil {
		return "", nil, fmt.Errorf("anime %d does not exist on AniList", mediaID)
	}

	used := strings.TrimSpace(query)
	var results []nyaa.TorrentResult
	if isAnimeMovie(*anime) {
		if used == "" {
			used, results, err = previewSearchWithVariants(anime.Media.Title, func(title string) ([]nyaa.TorrentResult, error) {
				return nyaa.ScrapNyaaForMovie(title, true)
			})
		} else {
			results, err = nyaa.ScrapNyaaForMovie(used, true)
		}
		if err != nil {
			return used, nil, wrapNyaaSearchError(err)
		}
		results, _ = filterSearchResults(results, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	} else {
		if used == "" {
			season, part := ExtractAnimeSeasonPart(anime.Media.Title, anime.Media.Synonyms)
			used, results, err = previewSearchWithVariants(anime.Media.Title, func(title string) ([]nyaa.TorrentResult, error) {
				return nyaa.ScrapNyaaForAnime(title, nil, season, part)
			})
		} else {
			results, err = nyaa.ScrapNyaaForAnime(used, nil, nil, nil)
		}
		if err != nil {
			return used, nil, wrapNyaaSearchError(err)
		}
		packs, _, _ := partitionSearchResults(configs, *anime, results)
		results = packs
	}

	if len(results) > maxTorrentCandidates {
		results = results[:maxTorrentCandidates]
	}
	return used, toCandidates(results), nil
}

func wrapNyaaSearchError(err error) error {
	if errors.Is(err, nyaa.ErrNyaaUnavailable) {
		return fmt.Errorf("failed to search nyaa: %w", err)
	}
	return fmt.Errorf("failed to search nyaa: %w: %v", nyaa.ErrNyaaUnavailable, err)
}

// previewSearchWithVariants is the title-variant loop for the picker. Unlike
// searchNyaaWithVariants it surfaces a total Nyaa failure (every variant errored) so the
// picker can show "Nyaa down" instead of an empty list. An empty result still continues to
// the next variant, same as the pass.
func previewSearchWithVariants(titles anilist.Title, searchFn nyaaSearchFunc) (string, []nyaa.TorrentResult, error) {
	variants := buildTitleVariants(titles)
	if len(variants) == 0 {
		return "", nil, nil
	}

	var lastErr error
	errored := 0
	for i, variant := range variants {
		logger.Logger.Debug().
			Str("title", variant).
			Int("attempt", i+1).
			Int("total_attempts", len(variants)).
			Msg("Attempting preview search with title variant")

		result, err := searchFn(variant)
		if err != nil {
			logger.Logger.Debug().
				Err(err).
				Str("title", variant).
				Int("attempt", i+1).
				Msg("Error searching Nyaa for preview")
			lastErr = err
			errored++
			continue
		}
		if result != nil {
			logger.Logger.Info().
				Str("title", variant).
				Int("torrents_found", len(result)).
				Int("attempt", i+1).
				Msg("Found preview torrents on Nyaa")
			return variant, result, nil
		}
	}
	if errored == len(variants) {
		return variants[0], nil, lastErr
	}
	return variants[0], nil, nil
}

func toCandidates(results []nyaa.TorrentResult) []TorrentCandidate {
	if len(results) == 0 {
		return nil
	}
	out := make([]TorrentCandidate, len(results))
	for i, tr := range results {
		info := nyaa.ExtractBatchInfo(tr.Name)
		resolution := ""
		if tr.Resolution != nil {
			resolution = *tr.Resolution
		}
		out[i] = TorrentCandidate{
			Name:       tr.Name,
			Magnet:     tr.MagnetLink,
			SizeBytes:  tr.Size,
			Seeders:    nyaa.ParseSeeders(tr.Seeders),
			Leechers:   tr.Leechers,
			Fansub:     tr.Fansub,
			Resolution: resolution,
			IsBatch:    tr.IsBatch,
			RangeStart: info.StartEpisode,
			RangeEnd:   info.EndEpisode,
			NyaaID:     nyaa.ParseNyaaID(tr.DetailURL),
		}
	}
	return out
}
