package nyaa

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/stringutil"

	"github.com/PuerkitoBio/goquery"
)

// httpGet is an indirection for http.Get so tests can replace it.
var httpGet = http.Get

// TorrentResult representa um resultado de torrent do Nyaa
type TorrentResult struct {
	Name       string    `json:"name"`
	Date       time.Time `json:"date"`
	Seeders    string    `json:"seeders"`
	Leechers   int       `json:"leechers,omitempty"`
	MagnetLink string    `json:"magnetLink"`
	Episode    *int      `json:"episode,omitempty"`
	Resolution *string   `json:"resolution,omitempty"`
	Season     *int      `json:"season,omitempty"`
	Part       *int      `json:"part,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Fansub     string    `json:"fansub,omitempty"`
	IsBatch    bool      `json:"isBatch,omitempty"`
}

func getNyaaBaseURL() string {
	if url := os.Getenv("NYAA_URL"); url != "" {
		return url
	}
	return "https://nyaa.si"
}

// MockNyaaHttpGet allows tests or callers to replace the httpGet function used by
// this package. It returns a function that when called will restore the
// previous httpGet implementation.
func MockNyaaHttpGet(fn func(string) (*http.Response, error)) (restore func()) {
	prev := httpGet
	if fn == nil {
		// If caller passes nil, do nothing and return a no-op restore
		return func() { httpGet = prev }
	}
	httpGet = fn
	return func() { httpGet = prev }
}

// GenerateSearchTitleVariants gera múltiplas variantes de busca para um título
// priorizando versões limpas (sem caracteres especiais) sobre as originais
// IMPORTANTE: Esta função deve ser usada APENAS para gerar queries de busca,
// nunca para modificar títulos de torrents retornados pelo Nyaa
func GenerateSearchTitleVariants(romaji, english string) []string {
	variants := []string{}
	seen := make(map[string]bool)

	// Função auxiliar para adicionar variante se ainda não foi adicionada
	addVariant := func(title string) {
		if title == "" || seen[title] {
			return
		}
		seen[title] = true
		variants = append(variants, title)
	}

	// Prioridade 1: Romaji limpo (sem caracteres especiais)
	if romaji != "" {
		cleanRomaji := stringutil.RemoveSpecialCharacters(romaji)
		addVariant(cleanRomaji)

		// Prioridade 2: Romaji original
		addVariant(romaji)
	}

	// Prioridade 3: English limpo (sem caracteres especiais)
	if english != "" && english != romaji {
		cleanEnglish := stringutil.RemoveSpecialCharacters(english)
		addVariant(cleanEnglish)

		// Prioridade 4: English original
		addVariant(english)
	}

	return variants
}

// extractFansub extrai o nome do fansub do título do torrent
// Procura por padrões como [FANSUB] ou (FANSUB)
func extractFansub(name string) string {
	matches := reFansub.FindStringSubmatch(name)
	if len(matches) > 1 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}
	return ""
}

// isBatch verifica se o torrent é um batch (múltiplos episódios)
// Baseado nas regras do documento (Seção 1 do RegrasFilmesBatches.md)
func isBatch(name string) bool {
	nameLower := strings.ToLower(name)
	for _, re := range reBatchPatterns {
		if re.MatchString(nameLower) {
			return true
		}
	}
	return false
}

// BatchInfo contém informações extraídas de um torrent batch
type BatchInfo struct {
	StartEpisode int
	EndEpisode   int
	Season       int
	IsComplete   bool
}

// extractBatchInfo extrai informações detalhadas de um batch
// Baseado nas regras do documento (Seção 3.1 do RegrasFilmesBatches.md)
func extractBatchInfo(torrentName string) BatchInfo {
	info := BatchInfo{}

	if matches := reBatchRange.FindStringSubmatch(torrentName); matches != nil {
		if start, err := strconv.Atoi(matches[1]); err == nil {
			info.StartEpisode = start
		}
		if end, err := strconv.Atoi(matches[2]); err == nil {
			info.EndEpisode = end
		}
	}

	if season := extractSeason(torrentName); season != nil {
		info.Season = *season
	}

	info.IsComplete = reBatchComplete.MatchString(torrentName)

	return info
}

// shouldIgnoreTorrent verifica se o torrent deve ser ignorado
// baseado em padrões indesejados (dub, raw, hardcoded, etc.)
func shouldIgnoreTorrent(name string) bool {
	return ShouldIgnore(name)
}

// IsBatch é uma versão exportável de isBatch para testes
func IsBatch(name string) bool {
	return isBatch(name)
}

// ExtractSeason é uma versão exportável de extractSeason para uso externo ao pacote
func ExtractSeason(name string) *int {
	return extractSeason(name)
}

// ExtractPart é uma versão exportável de extractPart para uso externo ao pacote
func ExtractPart(name string) *int {
	return extractPart(name)
}

// IsMovie é uma versão exportável de isMovie para testes
// isFormatMovie indica se o AniList classifica como filme (format = MOVIE)
func IsMovie(torrentName, animeName string, isFormatMovie ...bool) bool {
	// Se o parâmetro opcional foi passado, usa ele; caso contrário, assume false
	if len(isFormatMovie) > 0 {
		return isMovie(torrentName, animeName, isFormatMovie[0])
	}
	return isMovie(torrentName, animeName, false)
}

// isMovie verifica se o torrent é um filme
// Baseado nas regras do documento (Seção 4 do RegrasFilmesBatches.md)
// Se isFormatMovie for true, usa o formato do AniList (mais confiável)
func isMovie(torrentName, animeName string, isFormatMovie bool) bool {
	if isFormatMovie {
		return true
	}

	for _, re := range reMovieKeywords {
		if re.MatchString(torrentName) {
			return true
		}
	}

	if reOvaPattern.MatchString(torrentName) {
		return true
	}

	if reSpecialPattern.MatchString(torrentName) {
		return true
	}

	if !reHasEpisode.MatchString(torrentName) && !isBatch(torrentName) {
		return true
	}

	return false
}

// fetchNyaaPage fetches a single Nyaa results page and returns the parsed document.
func fetchNyaaPage(nyaaURL string) (*goquery.Document, error) {
	logger.Logger.Debug().Str("url", nyaaURL).Msg("Fetching Nyaa page")

	resp, err := httpGet(nyaaURL)
	if err != nil {
		logger.Logger.Debug().Err(err).Str("url", nyaaURL).Msg("Failed to fetch Nyaa page")
		return nil, fmt.Errorf("erro ao fazer requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Logger.Debug().Int("status_code", resp.StatusCode).Str("url", nyaaURL).Msg("Nyaa returned non-200 status")
		return nil, fmt.Errorf("erro HTTP: status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear HTML: %v", err)
	}
	return doc, nil
}

// deduplicateByMagnet removes duplicate TorrentResult entries by magnet link.
func deduplicateByMagnet(results []TorrentResult) []TorrentResult {
	seen := make(map[string]bool, len(results))
	unique := make([]TorrentResult, 0, len(results))
	for _, r := range results {
		if !seen[r.MagnetLink] {
			seen[r.MagnetLink] = true
			unique = append(unique, r)
		}
	}
	return unique
}

// ScrapNyaa busca torrents no Nyaa baseado no nome do anime e episódio.
// requestedSeason e requestedPart são extraídos upstream dos dados do Anilist.
func ScrapNyaa(animeName string, episode int, requestedSeason, requestedPart *int) ([]TorrentResult, error) {
	sanitizedRomajiName := reSeasonStrip.ReplaceAllString(animeName, "")
	sanitizedRomajiName = rePartStrip.ReplaceAllString(sanitizedRomajiName, "")

	query := strings.TrimSpace(sanitizedRomajiName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	// params.Set("q", fmt.Sprintf())     // Query de busca
	params.Set("q", fmt.Sprintf("%s %d", query, episode)) // Query de busca com episódio
	params.Set("s", "seeders")                            // Ordenar por seeders
	params.Set("o", "desc")                               // Ordem decrescente

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Int("episode", episode).
		Msg("Searching Nyaa for single episode")

	doc1, err := fetchNyaaPage(nyaaURL)
	if err != nil {
		return nil, err
	}
	doc2, _ := fetchNyaaPage(nyaaURL + "&p=2") // best-effort; page 2 may not exist

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		// Encontrar todas as células (td) na linha atual
		cells := s.Find("td")

		// Extrair dados de cada célula baseado na posição
		// Preferir o texto visível do link (nome com espaços). Alguns sites
		// preenchem o atributo title com pontos em vez de espaços (tests do projeto)
		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")

		// Extrair tamanho (cells.Eq(3) no Nyaa)
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)

		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := parseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		// Verificar se o torrent deve ser ignorado (dub, raw, hardcoded, etc.)
		if shouldIgnoreTorrent(name) {
			return
		}

		// Verificar se é batch - ignorar para busca de episódio único
		if isBatch(name) {
			return
		}

		// Extrair informações do nome do torrent
		var animeEpisode *int
		var season *int
		var resolution *string
		var fansub string

		if name != "" {
			animeEpisode = extractEpisodeNumber(name)
			season = extractSeason(name)
			res := extractResolution(name)
			resolution = &res
			fansub = extractFansub(name)
		}

		// Filtrar por título base (garantir que o torrent pertence ao anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Filtrar por temporada
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else {
			if season != nil && *season != 1 {
				return
			}
		}

		// Filtrar por parte (hard filter: rejeita torrent sem marker ou com part errada)
		part := extractPart(name)
		if requestedPart != nil {
			if part == nil || *part != *requestedPart {
				return
			}
		}

		// Requer correspondência exata do episódio
		if animeEpisode == nil || *animeEpisode != episode {
			return
		}

		// Adicionar resultado ao array
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Part:       part,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
		})
	}

	doc1.Find(".torrent-list tbody tr").Each(parseRow)
	if doc2 != nil {
		doc2.Find(".torrent-list tbody tr").Each(parseRow)
	}
	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("episode", episode).
		Int("results", len(results)).
		Msg("Found Nyaa results for single episode")

	if len(results) == 0 {
		return nil, nil // Nenhum resultado encontrado
	}

	// Ordenar resultados por qualidade e fansub
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
}

// ScrapNyaaForMultipleEpisodes busca torrents para múltiplos episódios simultaneamente.
// requestedSeason e requestedPart são extraídos upstream dos dados do Anilist.
func ScrapNyaaForMultipleEpisodes(animeName string, episodes []int, requestedSeason, requestedPart *int) ([]TorrentResult, error) {
	sanitizedRomajiName := reSeasonStrip.ReplaceAllString(animeName, "")
	sanitizedRomajiName = rePartStrip.ReplaceAllString(sanitizedRomajiName, "")

	query := strings.TrimSpace(sanitizedRomajiName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	// params.Set("q", fmt.Sprintf())     // Query de busca
	params.Set("q", fmt.Sprintf("%s", query)) // Query de busca com episódio
	params.Set("s", "seeders")                // Ordenar por seeders
	params.Set("o", "desc")                   // Ordem decrescente

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Int("episodes_count", len(episodes)).
		Msg("Searching Nyaa for multiple episodes")

	doc1, err := fetchNyaaPage(nyaaURL)
	if err != nil {
		return nil, err
	}
	doc2, _ := fetchNyaaPage(nyaaURL + "&p=2") // best-effort; page 2 may not exist

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		// Encontrar todas as células (td) na linha atual
		cells := s.Find("td")

		// Extrair dados de cada célula baseado na posição
		// Preferir o texto visível do link (nome com espaços). Alguns sites
		// preenchem o atributo title com pontos em vez de espaços (tests do projeto)
		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")

		// Extrair tamanho (cells.Eq(3) no Nyaa)
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)

		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := parseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		// Verificar se o torrent deve ser ignorado (dub, raw, hardcoded, etc.)
		if shouldIgnoreTorrent(name) {
			return
		}

		// Extrair informações do nome do torrent
		var animeEpisode *int
		var season *int
		var resolution *string
		var fansub string

		if name == "" {
			return
		}

		animeEpisode = extractEpisodeNumber(name)
		season = extractSeason(name)
		res := extractResolution(name)
		resolution = &res
		fansub = extractFansub(name)

		// Filtrar por título base (garantir que o torrent pertence ao anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Filtrar por temporada
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else {
			if season != nil && *season != 1 {
				return
			}
		}

		// Filtrar por parte (hard filter)
		part := extractPart(name)
		if requestedPart != nil {
			if part == nil || *part != *requestedPart {
				return
			}
		}

		if animeEpisode == nil {
			return
		}

		epIsInWantedEpisodes := slices.Contains(episodes, *animeEpisode)
		if !epIsInWantedEpisodes {
			return
		}

		// Adicionar resultado ao array
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Part:       part,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
		})
	}

	doc1.Find(".torrent-list tbody tr").Each(parseRow)
	if doc2 != nil {
		doc2.Find(".torrent-list tbody tr").Each(parseRow)
	}
	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Msg("Found Nyaa results for multiple episodes")

	if len(results) == 0 {
		return nil, nil // Nenhum resultado encontrado
	}

	// Ordenar resultados por qualidade e fansub
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
}

// ScrapNyaaForBatch busca torrents de batch (anime completo).
// season e part são extraídos upstream dos dados do Anilist.
// Baseado nas regras do documento (Seção 3 do RegrasFilmesBatches.md)
func ScrapNyaaForBatch(animeName string, season, part *int) ([]TorrentResult, error) {
	// Extrair temporada do nome se presente
	sanitizedName := extractSeasonFromName(animeName)
	query := strings.TrimSpace(sanitizedName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	params.Set("q", query) // Query sem número de episódio
	params.Set("s", "seeders")
	params.Set("o", "desc")

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Msg("Searching Nyaa for batch")

	// Fazer requisição HTTP
	resp, err := httpGet(nyaaURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao fazer requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro HTTP: status %d", resp.StatusCode)
	}

	// Parsear HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear HTML: %v", err)
	}

	var results []TorrentResult

	// Parsear linhas da tabela de torrents
	doc.Find(".torrent-list tbody tr").Each(func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)
		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := parseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		// Verificar se o torrent deve ser ignorado (dub, raw, hardcoded, etc.)
		if shouldIgnoreTorrent(name) {
			return
		}

		// Verificar se é batch
		if !isBatch(name) {
			return // Ignorar torrents que não são batch
		}

		// Extrair informações do nome
		seasonNum := extractSeason(name)
		partNum := extractPart(name)
		res := extractResolution(name)
		resolution := &res
		fansub := extractFansub(name)

		// Filtrar por título base
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Filtrar por temporada
		if season != nil {
			if seasonNum == nil || *seasonNum != *season {
				return
			}
		}

		// Filtrar por parte (hard filter)
		if part != nil {
			if partNum == nil || *partNum != *part {
				return
			}
		}

		// Adicionar resultado
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Season:     seasonNum,
			Part:       partNum,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
			IsBatch:    true,
		})
	})

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Msg("Found Nyaa batch results")

	if len(results) == 0 {
		return nil, nil // Nenhum batch encontrado
	}

	// Ordenar resultados por qualidade
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
}

// ScrapNyaaForMovie busca torrents de filmes
// Detecta automaticamente filmes e ordena por qualidade técnica
// Baseado nas regras do documento (Seção 4-6 do RegrasFilmesBatches.md)
// isFormatMovie indica se o AniList classifica como filme (format = MOVIE)
func ScrapNyaaForMovie(animeName string, isFormatMovie ...bool) ([]TorrentResult, error) {
	// Se o parâmetro opcional foi passado, usa ele; caso contrário, assume false
	isMovieFormat := false
	if len(isFormatMovie) > 0 {
		isMovieFormat = isFormatMovie[0]
	}
	// Extrair temporada do nome se presente (filmes geralmente não têm)
	sanitizedName := extractSeasonFromName(animeName)
	query := strings.TrimSpace(sanitizedName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders")
	params.Set("o", "desc")

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Msg("Searching Nyaa for movie")

	// Fazer requisição HTTP
	resp, err := httpGet(nyaaURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao fazer requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro HTTP: status %d", resp.StatusCode)
	}

	// Parsear HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear HTML: %v", err)
	}

	var results []TorrentResult

	// Parsear linhas da tabela de torrents
	doc.Find(".torrent-list tbody tr").Each(func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)
		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := parseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		// Verificar se o torrent deve ser ignorado
		if shouldIgnoreTorrent(name) {
			return
		}

		// Verificar se é filme
		if !isMovie(name, animeName, isMovieFormat) {
			return
		}

		// Extrair informações
		res := extractResolution(name)
		resolution := &res
		fansub := extractFansub(name)

		// Filtrar por título base
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		// Adicionar resultado
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			Leechers:   leechers,
			MagnetLink: torrentLink,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
			IsBatch:    false,
		})
	})

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Msg("Found Nyaa movie results")

	if len(results) == 0 {
		return nil, nil // Nenhum filme encontrado
	}

	// Ordenar resultados usando ordenação específica para filmes
	sortedResults := SortMovieResults(results)
	return sortedResults, nil
}

// extractEpisodeNumber extrai o número do episódio do nome do torrent
// Testa os padrões em ordem de prioridade (do mais específico ao menos específico)
func extractEpisodeNumber(name string) *int {
	for _, p := range reEpisodePatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if episodeNum, err := strconv.Atoi(matches[1]); err == nil {
				if episodeNum > 0 && episodeNum < 1000 {
					return &episodeNum
				}
			}
		}
	}
	return nil
}

// extractSeason extrai o número da temporada do nome do torrent
// Testa os padrões em ordem de prioridade
func extractSeason(name string) *int {
	for _, p := range reSeasonPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if seasonNum, err := strconv.Atoi(matches[1]); err == nil {
				if seasonNum > 0 && seasonNum < 100 {
					return &seasonNum
				}
			}
		}
	}
	return nil
}

// extractPart extrai o número da parte/cour do nome do torrent ou título Anilist
// Testa os padrões em ordem de prioridade (mais específico primeiro)
func extractPart(name string) *int {
	for _, p := range rePartPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if partNum, err := strconv.Atoi(matches[1]); err == nil {
				if partNum > 0 && partNum < 100 {
					return &partNum
				}
			}
		}
	}
	return nil
}

// extractResolution extrai a resolução do nome do torrent
func extractResolution(name string) string {
	for _, p := range reResolutionPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			return strings.ToLower(matches[1])
		}
	}
	return ""
}

// parseNyaaDate parseia a string de data do Nyaa para um objeto time.Time
func parseNyaaDate(dateString string) (time.Time, error) {
	// As datas do Nyaa são tipicamente no formato "2025-07-06 14:49"
	// Tentar diferentes formatos de data
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04",
		"2006/01/02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateString); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("não foi possível parsear a data: %s", dateString)
}

// isWithinThreeMonths verifica se a data está dentro dos últimos 3 meses
func isWithinThreeMonths(date time.Time) bool {
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	return date.After(threeMonthsAgo)
}

// resolutionPriority retorna um valor de prioridade para a resolução (menor = melhor)
// Baseado nas regras do Nyaa (Seção 3 do documento de regras)
func resolutionPriority(resolution string) int {
	return priorityIndex(ActivePriorities().Resolutions, resolution)
}

// isUncensored verifica se o torrent contém "Uncensored" no título
func isUncensored(torrentName string) bool {
	return strings.Contains(strings.ToLower(torrentName), "uncensored")
}

// fansubPriority retorna um valor de prioridade para o fansub (menor = melhor)
// Baseado nas regras do Nyaa (Seção 4 do documento de regras)
func fansubPriority(torrentName string) int {
	nameLower := strings.ToLower(torrentName)
	fansubs := ActivePriorities().Fansubs
	best := len(fansubs)
	for i, f := range fansubs {
		if strings.Contains(nameLower, f) && i < best {
			best = i
		}
	}
	return best
}

// extractSource extrai a fonte do release (BD, WEB-DL, TV, etc.)
func extractSource(name string) string {
	nameLower := strings.ToLower(name)
	for _, sp := range reSourcePatterns {
		if sp.re.MatchString(nameLower) {
			return sp.source
		}
	}
	return ""
}

// extractCodec extrai o codec de vídeo (HEVC, H.264, AV1, etc.)
func extractCodec(name string) string {
	nameLower := strings.ToLower(name)
	for _, cp := range reCodecPatterns {
		if cp.re.MatchString(nameLower) {
			return cp.codec
		}
	}
	return ""
}

// extractAudio extrai o codec de áudio (FLAC, DDP, AAC, etc.)
func extractAudio(name string) string {
	nameLower := strings.ToLower(name)
	for _, ap := range reAudioPatterns {
		if ap.re.MatchString(nameLower) {
			return ap.audio
		}
	}
	return ""
}

// sourcePriority retorna prioridade da fonte (menor = melhor)
// Baseado nas regras do documento (Seção 6.1 do RegrasFilmesBatches.md)
func sourcePriority(source string) int {
	return priorityIndex(ActivePriorities().Sources, source)
}

// codecPriority retorna prioridade do codec (menor = melhor)
// Baseado nas regras do documento (Seção 6.2 do RegrasFilmesBatches.md)
func codecPriority(codec string) int {
	return priorityIndex(ActivePriorities().Codecs, codec)
}

// audioPriority retorna prioridade do áudio (menor = melhor)
// Baseado nas regras do documento (Seção 6.3 do RegrasFilmesBatches.md)
func audioPriority(audio string) int {
	return priorityIndex(ActivePriorities().Audio, audio)
}

// SortTorrentResults ordena os torrents por qualidade
// Critérios de ordenação (em ordem de prioridade):
// 1. Uncensored (conteúdo uncensored tem prioridade)
// 2. Resolução (1080p > 720p > 480p > ...)
// 3. Fansub (SubsPlease > Erai-raws > ...)
// 4. Seeders (mais seeders é melhor)
// 5. Tamanho (menor é melhor para mesma qualidade)
// Baseado nas regras do Nyaa (Seção 8 do documento de regras)
func SortTorrentResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, filterCriteria(ActivePriorities().CriteriaOrder, episodeCriteria))
}

// parseSeeders converte a string de seeders para int
func parseSeeders(seedersStr string) int {
	seedersStr = strings.TrimSpace(seedersStr)
	if seedersStr == "" || seedersStr == "-" {
		return 0
	}
	seedersStr = strings.ReplaceAll(seedersStr, ",", "")
	if seeders, err := strconv.Atoi(seedersStr); err == nil {
		return seeders
	}
	return 0
}

// torrentHealthScore returns a health value that combines seeder count and
// seeder/leecher ratio. Higher = healthier swarm.
// Uses log scale so a torrent with 200 seeders isn't infinitely better than one with 100.
func torrentHealthScore(r TorrentResult) float64 {
	seeders := parseSeeders(r.Seeders)
	if seeders == 0 {
		return 0.0
	}
	ratio := float64(seeders) / float64(seeders+r.Leechers+1)
	return ratio * math.Log(float64(seeders)+1)
}

// parseSize converte a string de tamanho para bytes
// Formatos comuns: "1.5 GiB", "500 MiB", "2.3 GiB", etc.
func parseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" || sizeStr == "-" {
		return 0
	}

	matches := reParseSizeRe.FindStringSubmatch(sizeStr)

	if len(matches) != 3 {
		return 0
	}

	// Converter número para float
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	// Converter para bytes baseado na unidade
	unit := strings.ToUpper(matches[2])
	var multiplier int64 = 1

	switch unit {
	case "KB", "KIB":
		multiplier = 1024
	case "MB", "MIB":
		multiplier = 1024 * 1024
	case "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(value * float64(multiplier))
}

// extractSeasonFromName remove informações de temporada do nome para query base
func extractSeasonFromName(name string) string {
	result := name
	for _, re := range reSeasonNamePatterns {
		result = re.ReplaceAllString(result, "")
	}
	return strings.TrimSpace(result)
}

// SortMovieResults ordena os resultados de busca de filmes por qualidade
// Baseado nas regras do documento (Seção 6.4 do RegrasFilmesBatches.md)
// Critérios de ordenação (em ordem de prioridade):
// 1. Source (BD > WEB-DL > TV)
// 2. Resolução (1080p > 720p > 480p)
// 3. Codec (HEVC > H.264)
// 4. Fansub (SubsPlease > Erai-raws > ...)
// 5. Áudio (FLAC > DDP > AAC)
// 6. Seeders (mais é melhor)
// 7. Tamanho (menor é melhor para mesma qualidade)
func SortMovieResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, ActivePriorities().CriteriaOrder)
}
