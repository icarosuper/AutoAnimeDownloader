package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
)

// dropStats conta o que os filtros de busca descartaram numa chamada.
//
// Existe porque "a lista voltou vazia" tem tres causas diferentes e so o contador as distingue:
// o relatorio da ultima verificacao precisa poder dizer "oito candidatos, todos acima de 3 GB"
// em vez de "nenhum torrent encontrado" — que e verdade tambem, e e a resposta menos acionavel
// das tres. Input e quantos entraram, ANTES de qualquer filtro.
type dropStats struct {
	Input     int
	BySize    int
	BySeeders int
}

type nyaaSearchFunc func(title string) ([]nyaa.TorrentResult, error)

type nyaaSearcher struct {
	searchAnime         func(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult
	searchSingleEpisode func(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, customQuery string, totalEpisodes int) []nyaa.TorrentResult
	searchMovie         func(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult
	// packRange le a faixa REAL de um pack na pagina de detalhe do Nyaa. Separado das buscas
	// porque nao e busca: roda depois do filtro e da ordenacao, so para o pack que o nome nao
	// resolve, e so para os poucos que a escolha de fato considera (ver packSet).
	packRange func(tr nyaa.TorrentResult) (nyaa.BatchInfo, bool)
}

func defaultNyaaSearcher() nyaaSearcher {
	return nyaaSearcher{
		searchAnime:         searchNyaaForAnime,
		searchSingleEpisode: searchNyaaForSingleEpisode,
		searchMovie:         searchNyaaForMovie,
		packRange:           func(tr nyaa.TorrentResult) (nyaa.BatchInfo, bool) { return nyaa.PackFileRange(tr.DetailURL) },
	}
}

// filterBySize descarta torrents acima de maxGB (GiB). maxGB <= 0 desliga o filtro.
//
// Roda DEPOIS da ordenacao por prioridade e preserva a ordem, entao o escolhido continua sendo
// o melhor entre os que cabem. Size == 0 passa: e o valor de nyaa.parseSize quando o Nyaa muda o
// formato da coluna, e descartar tamanho desconhecido trocaria "as vezes baixa um torrent grande"
// por "nao baixa nada" — um bug de parsing viraria paralisacao silenciosa.
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

// filterBySeeders descarta torrents com menos de minSeeders. minSeeders <= 0 desliga o filtro.
//
// Mesmo contrato do filterBySize (roda depois da ordenacao, preserva a ordem). Aqui, ao contrario
// do tamanho, seeders ilegivel conta como 0 e e DESCARTADO com o default de 1: a coluna de seeders
// do Nyaa e o unico sinal de que existe alguem semeando, e um torrent sem semeador nao baixa nunca
// — deixar passar troca "nao escolhe esse" por "trava o episodio num torrent morto", que foi
// exatamente o que aconteceu com o unico candidato do Naruto Shippuuden episodio 3 (0 peers).
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

// filterSearchResults aplica os dois filtros de busca (teto de tamanho + piso de seeders) na
// ordem em que os quatro pontos de busca precisam deles, e devolve o que cada um cortou.
func filterSearchResults(results []nyaa.TorrentResult, maxGB float64, minSeeders int) ([]nyaa.TorrentResult, dropStats) {
	bySize, sizeDropped := filterBySize(results, maxGB)
	final, seedersDropped := filterBySeeders(bySize, minSeeders)
	return final, dropStats{Input: len(results), BySize: sizeDropped, BySeeders: seedersDropped}
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

// totalEpisodes vai para o nyaa apenas para decidir o zero-padding da query (0 = desconhecido).
func searchNyaaForSingleEpisode(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, customQuery string, totalEpisodes int) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)

	results := searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaa(title, ep.Episode, season, part, totalEpisodes)
	}, "single episode")

	if len(results) > 0 {
		return results
	}

	// Fallback com offset: converte progresso relativo em número absoluto para fansubs
	// com numeração contínua. Só aplica quando part >= 2 (gate obrigatório).
	if offset := ComputeEpisodeOffset(relations, part); offset > 0 {
		results = searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
			return nyaa.ScrapNyaa(title, ep.Episode+offset, season, nil, totalEpisodes)
		}, "single episode (offset fallback)")
	}

	return results
}

func searchNyaaForMovie(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult {
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForMovie(title, isFormatMovie)
	}, "movie")
}

// searchNyaaForAnime e a busca unica por anime: devolve packs e episodios na mesma lista (ver
// nyaa.ScrapNyaaForAnime).
func searchNyaaForAnime(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForAnime(title, episodes, season, part)
	}, "anime")
}
