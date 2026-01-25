package nyaa

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"AutoAnimeDownloader/src/internal/logger"

	"github.com/PuerkitoBio/goquery"
)

// httpGet is an indirection for http.Get so tests can replace it.
var httpGet = http.Get

// TorrentResult representa um resultado de torrent do Nyaa
type TorrentResult struct {
	Name       string    `json:"name"`
	Date       time.Time `json:"date"`
	Seeders    string    `json:"seeders"`
	MagnetLink string    `json:"magnetLink"`
	Episode    *int      `json:"episode,omitempty"`
	Resolution *string   `json:"resolution,omitempty"`
	Season     *int      `json:"season,omitempty"`
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

// removeSpecialCharacters remove caracteres especiais de um título
// Mantém apenas letras, números e espaços
func removeSpecialCharacters(s string) string {
	// Converte para minúsculas
	s = strings.ToLower(s)
	// Remove tudo exceto letras, números e espaços
	re := regexp.MustCompile(`[^a-z0-9\s]`)
	s = re.ReplaceAllString(s, "")
	// Remove espaços múltiplos e trim
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	return s
}

// extractFansub extrai o nome do fansub do título do torrent
// Procura por padrões como [FANSUB] ou (FANSUB)
func extractFansub(name string) string {
	// Pattern para extrair fansub entre colchetes ou parênteses
	pattern := `(?i)^\[([^\]]+)\]|^\(([^\)]+)\)`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(name)

	if len(matches) > 1 {
		fansub := strings.ToLower(strings.TrimSpace(matches[1]))
		return fansub
	}

	return ""
}

// isBatch verifica se o torrent é um batch (múltiplos episódios)
// Baseado nas regras do documento (Seção 1 do RegrasFilmesBatches.md)
func isBatch(name string) bool {
	patterns := []string{
		// 1. Palavra "Batch" explícita
		`(?i)\(batch\)|\[batch\]|\s+batch\s+`,
		`(?i)\(unofficial\s+batch\)`,

		// 2. Faixa de episódios (range)
		`(?i)\(\d{1,3}\s*[-~]\s*\d{1,3}\)`, // (01-28), (01 ~ 28)
		`(?i)\s\d{1,3}\s*[-~]\s*\d{1,3}\s`, // 01-28, 01 ~ 28
		`(?i)\s\d{1,3}~\d{1,3}\s`,          // 01~28

		// 3. Palavra "Complete"
		`(?i)\(complete\)|\[complete\]|\s+complete\s+`,
		`(?i)complete\s+series|complete\s+season`,
		`(?i)\(season\s+\d+.*complete\)`,

		// 4. Temporada completa (sem episódio específico)
		`(?i)\(s\d{1,2}\)|\[s\d{1,2}\]`,         // (S01), [S01], (S1), [S1]
		`(?i)\(season\s+\d+\)|\[season\s+\d+\]`, // (Season 1), [Season 1]
		`(?i)\(\d{1,2}st\s+season\)|\(\d{1,2}nd\s+season\)|\(\d{1,2}rd\s+season\)|\(\d{1,2}th\s+season\)`, // (1st Season), (2nd Season)
		`(?i)\bseason\s+\d+\s*\([^)]*\)`, // Season 1 (Complete), Season 1 (1080p)

		// 5. S01/S02 sem parênteses, mas apenas se seguido de infos técnicas (não episódio)
		// Estes padrões evitam falsos positivos como "S2 - 7"
		`(?i)\.s\d{1,2}\.`,        // .S01. (ponto antes e depois)
		`(?i)\bs\d{1,2}\s+\(.*bd`, // S01 (BD... - temporada sem parênteses seguido de BD
		`(?i)s\d{1,2}\s+\(1080p\)|s\d{1,2}\s+\(720p\)|s\d{1,2}\s+\(480p\)`, // S01 (1080p)
		`(?i)s\d{1,2}\s+1080p|s\d{1,2}\s+720p`,                             // S01 1080p, S01 720p (com espaço)
		`(?i)s\d{1,2}\.+1080p|s\d{1,2}\.+720p`,                             // S01.1080p, S01.720p (com ponto)
	}

	nameLower := strings.ToLower(name)
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(nameLower) {
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

	// Extrair range de episódios (01-28, 01 ~ 28)
	rangePattern := regexp.MustCompile(`(?i)(\d{1,3})\s*[-~]\s*(\d{1,3})`)
	if matches := rangePattern.FindStringSubmatch(torrentName); matches != nil {
		if start, err := strconv.Atoi(matches[1]); err == nil {
			info.StartEpisode = start
		}
		if end, err := strconv.Atoi(matches[2]); err == nil {
			info.EndEpisode = end
		}
	}

	// Extrair temporada (reutilizar função existente)
	if season := extractSeason(torrentName); season != nil {
		info.Season = *season
	}

	// Verificar se está marcado como "complete" ou "batch"
	info.IsComplete = regexp.MustCompile(`(?i)complete|batch`).MatchString(torrentName)

	return info
}

// shouldIgnoreTorrent verifica se o torrent deve ser ignorado
// baseado em padrões indesejados (dub, raw, hardcoded, etc.)
func shouldIgnoreTorrent(name string) bool {
	ignorePatterns := []string{
		`(?i)\[dub\]`,       // Dublado
		`(?i)\[raw\]`,       // Sem legenda
		`(?i)\[hardcoded\]`, // Hardcoded subtitles
		`(?i)\[hc\]`,        // Hardcoded abbreviation
		`(?i)re-encode`,     // Re-encode de baixa qualidade
	}

	nameLower := strings.ToLower(name)
	for _, pattern := range ignorePatterns {
		if regexp.MustCompile(pattern).MatchString(nameLower) {
			return true
		}
	}

	return false
}

// IsBatch é uma versão exportável de isBatch para testes
func IsBatch(name string) bool {
	return isBatch(name)
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
	// 1. Usar formato do AniList se disponível (prioridade máxima)
	if isFormatMovie {
		return true
	}

	// 2. Palavras-chave explícitas de filme
	movieKeywords := []string{
		`(?i)\(movie\)|\[movie\]|\smovie\s`,
		`(?i)\(film\)|\[film\]`,
		`(?i)gekijouban`, // 劇場版 (movie em japonês)
		`(?i)theater\s+version`,
		`(?i)theatrical`,
	}

	for _, pattern := range movieKeywords {
		if regexp.MustCompile(pattern).MatchString(torrentName) {
			return true
		}
	}

	// 3. OVA/ONA
	ovaPattern := regexp.MustCompile(`(?i)\(?(ova|ona)\)?|original\s+(video|net)\s+animation`)
	if ovaPattern.MatchString(torrentName) {
		return true
	}

	// 4. Special
	specialPattern := regexp.MustCompile(`(?i)\(special\)|\[special\]|\sspecial\s|(tv\s+special|episode\s+sp)`)
	if specialPattern.MatchString(torrentName) {
		return true
	}

	// 5. Verificar se não tem número de episódio E não é batch
	// Se não tiver episode number e não for batch, pode ser filme
	hasEpisode := regexp.MustCompile(`(?i)(-\s?\d{1,3}|episode\s*\d{1,3}|s\d{1,2}e\d{1,3})`).MatchString(torrentName)
	isBatch := isBatch(torrentName)

	if !hasEpisode && !isBatch {
		// Provável filme ou especial
		return true
	}

	return false
}

// ScrapNyaa busca torrents no Nyaa baseado no nome do anime e episódio
func ScrapNyaa(animeName string, episode int) ([]TorrentResult, error) {
	// Extrair temporada solicitada (se houver) e sanitizar o nome base
	requestedSeason := extractSeason(animeName)

	// Remover informações de temporada do nome para formar a query base
	// Adicionar suporte para "Cour" além de "Season"
	seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)
	sanitizedRomajiName := seasonPattern.ReplaceAllString(animeName, "")

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

	// Fazer requisição HTTP (usando httpGet para permitir mock em testes)
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
		baseTitle := strings.ToLower(query)
		if baseTitle != "" {
			// Tenta match exato primeiro (mais rápido)
			if !strings.Contains(strings.ToLower(name), baseTitle) {
				// Se não encontrar, tenta removendo caracteres especiais
				cleanName := removeSpecialCharacters(name)
				cleanQuery := removeSpecialCharacters(query)
				if cleanQuery == "" || !strings.Contains(cleanName, cleanQuery) {
					return
				}
			}
		}

		// Filtrar por temporada
		if requestedSeason != nil {
			// Se uma temporada específica foi solicitada, o torrent deve ter essa temporada
			if season == nil || *season != *requestedSeason {
				return
			}
		} else {
			// Se nenhuma temporada foi especificada, aceitar apenas torrents sem temporada ou da primeira temporada
			if season != nil && *season != 1 {
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
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
		})
	})

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

func ScrapNyaaForMultipleEpisodes(animeName string, episodes []int) ([]TorrentResult, error) {
	// Extrair temporada solicitada (se houver) e sanitizar o nome base
	requestedSeason := extractSeason(animeName)

	// Remover informações de temporada do nome para formar a query base
	// Adicionar suporte para "Cour" além de "Season"
	seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)
	sanitizedRomajiName := seasonPattern.ReplaceAllString(animeName, "")

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
		baseTitle := strings.ToLower(query)
		if baseTitle != "" {
			// Tenta match exato primeiro (mais rápido)
			if !strings.Contains(strings.ToLower(name), baseTitle) {
				// Se não encontrar, tenta removendo caracteres especiais
				cleanName := removeSpecialCharacters(name)
				cleanQuery := removeSpecialCharacters(query)
				if cleanQuery == "" || !strings.Contains(cleanName, cleanQuery) {
					return
				}
			}
		}

		// Filtrar por temporada
		if requestedSeason != nil {
			// Se uma temporada específica foi solicitada, o torrent deve ter essa temporada
			if season == nil || *season != *requestedSeason {
				return
			}
		} else {
			// Se nenhuma temporada foi especificada, aceitar apenas torrents sem temporada ou da primeira temporada
			if season != nil && *season != 1 {
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
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Resolution: resolution,
			Size:       size,
			Fansub:     fansub,
		})
	})

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

// ScrapNyaaForBatch busca torrents de batch (anime completo)
// Prioriza torrents que contenham múltiplos episódios ou temporadas completas
// Baseado nas regras do documento (Seção 3 do RegrasFilmesBatches.md)
func ScrapNyaaForBatch(animeName string, season *int) ([]TorrentResult, error) {
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
		res := extractResolution(name)
		resolution := &res
		fansub := extractFansub(name)

		// Filtrar por título base
		baseTitle := strings.ToLower(query)
		if baseTitle != "" {
			if !strings.Contains(strings.ToLower(name), baseTitle) {
				cleanName := removeSpecialCharacters(name)
				cleanQuery := removeSpecialCharacters(query)
				if cleanQuery == "" || !strings.Contains(cleanName, cleanQuery) {
					return
				}
			}
		}

		// Filtrar por temporada
		if season != nil {
			if seasonNum == nil || *seasonNum != *season {
				return
			}
		}

		// Adicionar resultado
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			MagnetLink: torrentLink,
			Season:     seasonNum,
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

	// Adicionar "Movie" à query para melhorar resultados
	query = fmt.Sprintf("%s Movie", query)

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
		baseTitle := strings.ToLower(extractSeasonFromName(animeName))
		if baseTitle != "" {
			if !strings.Contains(strings.ToLower(name), baseTitle) {
				cleanName := removeSpecialCharacters(name)
				cleanQuery := removeSpecialCharacters(baseTitle)
				if cleanQuery == "" || !strings.Contains(cleanName, cleanQuery) {
					return
				}
			}
		}

		// Adicionar resultado
		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
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
// Baseado nas regras do Nyaa (Seção 2 do documento de regras)
func extractEpisodeNumber(name string) *int {
	// Padrões de episódio reconhecidos - em ordem de prioridade
	patterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)S\d{1,2}E(\d{1,3})`, "S01E05, S1E5 - formato temporada+episódio"},
		{`(?i)\s+-\s+(\d{1,3})(?:\s|v\d+|$|\[)`, "- 05, - 05v2 - padrão SubsPlease"},
		{`(?i)\s+-\s+(\d{1,3})\s*\(`, "- 05 (1080p) - padrão com parênteses"},
		{`(?i)EP\s*(\d{1,3})\b`, "EP05, EP5 - formato EP"},
		{`(?i)Episode\s*(\d{1,3})\b`, "Episode 05 - formato por extenso"},
		{`(?i)\bE(\d{1,3})\b`, "E05, E5 - formato E standalone"},
		{`(?i)\[(\d{1,3})\]`, "[05], [5] - episódios entre colchetes"},
		{`(?i)\s(\d{1,3})\s*\(`, " 05 ( - espaço + número + parêntese"},
		{`(?i)\s(\d{1,3})\.mkv`, " 05.mkv - espaço + número + .mkv"},
		{`(?i)\s(\d{1,3})$`, " 5 - número no final da string"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if episodeNum, err := strconv.Atoi(matches[1]); err == nil {
				// Validação: episódio deve estar entre 1 e 999
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
// Baseado nas regras do Nyaa (Seção 1 do documento de regras)
func extractSeason(name string) *int {
	// Padrões de temporada reconhecidos - em ordem de prioridade
	patterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bS(\d{1,2})E`, "S01E05 - S antes de E (prioridade máxima)"},
		{`(?i)\bS(\d{1,2})\b`, "S01, S1 - formato S standalone"},
		{`(?i)Season\s*(\d{1,2})`, "Season 1, Season 01 - por extenso"},
		{`(?i)(\d{1,2})(?:st|nd|rd|th)\s+Season`, "1st Season, 2nd Season - ordinal"},
		{`(?i)\b(\d{1,2})\s+-\s+\d{1,2}`, "2 - 07 (season - episode)"},
		{`(?i)\b(\d{1,2})\s+EP\d{1,3}`, "2 EP07 (número seguido de EP)"},
		{`(?i)Cour\s*(\d{1,2})`, "Cour 1, Cour 2 - formato cour"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if seasonNum, err := strconv.Atoi(matches[1]); err == nil {
				// Validação: temporada deve ser entre 1 e 99
				if seasonNum > 0 && seasonNum < 100 {
					return &seasonNum
				}
			}
		}
	}

	return nil
}

// extractResolution extrai a resolução do nome do torrent
// Baseado nas regras do Nyaa (Seção 3 do documento de regras)
func extractResolution(name string) string {
	// Padrões de resolução reconhecidos - em ordem de prioridade
	patterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\b(\d{3,4}p)\b`, "1080p, 720p, 480p - padrão principal"},
		{`(?i)\b(\d{3,4}x\d{3,4})\b`, "1920x1080, 1280x720 - formato dimensão"},
		{`(?i)\b(4K)\b`, "4K - Ultra HD"},
		{`(?i)\b(8K)\b`, "8K - Ultra Ultra HD"},
		{`(?i)\b(F?HD)\b`, "HD, FHD - Full HD"},
		{`(?i)\b(UHD)\b`, "UHD - Ultra HD"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindStringSubmatch(name)
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
	priorityMap := map[string]int{
		"1080p": 0, // Melhor opção - padrão mais comum
		"720p":  1, // Segunda melhor opção
		"480p":  2, // Aceitável
		"4k":    3, // 4K
		"8k":    4, // 8K
		"fhd":   5, // Full HD
		"uhd":   6, // Ultra HD
		"hd":    7, // HD genérico
		"2160p": 8, // 4K alternativo
		"1440p": 9, // 2K
	}

	normalized := strings.ToLower(resolution)
	if priority, exists := priorityMap[normalized]; exists {
		return priority
	}

	return 999 // Resolução desconhecida tem menor prioridade
}

// isUncensored verifica se o torrent contém "Uncensored" no título
func isUncensored(torrentName string) bool {
	return strings.Contains(strings.ToLower(torrentName), "uncensored")
}

// fansubPriority retorna um valor de prioridade para o fansub (menor = melhor)
// Baseado nas regras do Nyaa (Seção 4 do documento de regras)
func fansubPriority(torrentName string) int {
	// Fansubs reconhecidos - em ordem de prioridade (menor = melhor)
	fansubPriorities := map[string]int{
		// Melhores opções (releases consistentes e de alta qualidade)
		"subsplease": 0, // MELHOR - consistência semanal
		"erai-raws":  1, // Excelente qualidade

		// Boas opções
		"judas":    2, // Bom para dual audio
		"toonshub": 3, // Multi-subs
		"asw":      4, // Active Simulcast Watch

		// Aceitáveis
		"ember":    5, // Batches
		"hd-zone":  6,
		"kamig":    7,
		"remix":    8,
		"aniverse": 9,

		// Menos preferíveis
		"dub": 10, // Dublado (geralmente não queremos)
		"raw": 11, // Sem legendas
	}

	nameLower := strings.ToLower(torrentName)

	// Encontrar o fansub com maior prioridade (menor número)
	bestPriority := 999
	for fansub, priority := range fansubPriorities {
		if strings.Contains(nameLower, fansub) {
			if priority < bestPriority {
				bestPriority = priority
			}
		}
	}

	return bestPriority
}

// extractSource extrai a fonte do release (BD, WEB-DL, TV, etc.)
// Baseado nas regras do documento (Seção 6.1 do RegrasFilmesBatches.md)
func extractSource(name string) string {
	sourcePatterns := []struct {
		pattern string
		source  string
	}{
		{`(?i)BDRemux`, "BDRemux"},
		{`(?i)BDRip|BD\s*Ray`, "BDRip"},
		{`(?i)Blu-ray|Blu\s*ray`, "BD"},
		{`(?i)WEB-DL`, "WEB-DL"},
		{`(?i)WEBRip`, "WEBRip"},
		{`(?i)HDTV`, "HDTV"},
		{`(?i)\bTV\s*Rip|\bTV\b`, "TV"},
		{`(?i)DVD`, "DVD"},
	}

	nameLower := strings.ToLower(name)
	for _, sp := range sourcePatterns {
		if regexp.MustCompile(sp.pattern).MatchString(nameLower) {
			return sp.source
		}
	}

	return ""
}

// extractCodec extrai o codec de vídeo (HEVC, H.264, AV1, etc.)
// Baseado nas regras do documento (Seção 6.2 do RegrasFilmesBatches.md)
func extractCodec(name string) string {
	codecPatterns := []struct {
		pattern string
		codec   string
	}{
		{`(?i)HEVC|\s*x265\s*`, "HEVC"},
		{`(?i)AV1`, "AV1"},
		{`(?i)H\.264|\s*x264\s*`, "H.264"},
		{`(?i)XviD`, "XviD"},
	}

	nameLower := strings.ToLower(name)
	for _, cp := range codecPatterns {
		if regexp.MustCompile(cp.pattern).MatchString(nameLower) {
			return cp.codec
		}
	}

	return ""
}

// extractAudio extrai o codec de áudio (FLAC, DDP, AAC, etc.)
// Baseado nas regras do documento (Seção 6.3 do RegrasFilmesBatches.md)
func extractAudio(name string) string {
	audioPatterns := []struct {
		pattern string
		audio   string
	}{
		{`(?i)FLAC`, "FLAC"},
		{`(?i)DTS-HD`, "DTS-HD"},
		{`(?i)TrueHD`, "TrueHD"},
		{`(?i)DDP|Dolby\s*Digital\s*Plus`, "DDP"},
		{`(?i)AAC`, "AAC"},
		{`(?i)AC3|Dolby\s*Digital`, "AC3"},
		{`(?i)MP3`, "MP3"},
	}

	nameLower := strings.ToLower(name)
	for _, ap := range audioPatterns {
		if regexp.MustCompile(ap.pattern).MatchString(nameLower) {
			return ap.audio
		}
	}

	return ""
}

// sourcePriority retorna prioridade da fonte (menor = melhor)
// Baseado nas regras do documento (Seção 6.1 do RegrasFilmesBatches.md)
func sourcePriority(source string) int {
	priorityMap := map[string]int{
		"BD":      0, // Blu-ray (MELHOR)
		"BDRip":   1,
		"BDRemux": 2,
		"WEB-DL":  3, // Web download (bom)
		"WEBRip":  4,
		"TV":      5, // TV rip (aceitável)
		"DVD":     6,
		"HDTV":    7,
	}

	normalized := strings.ToLower(source)
	if priority, exists := priorityMap[normalized]; exists {
		return priority
	}

	return 999 // Source desconhecida
}

// codecPriority retorna prioridade do codec (menor = melhor)
// Baseado nas regras do documento (Seção 6.2 do RegrasFilmesBatches.md)
func codecPriority(codec string) int {
	priorityMap := map[string]int{
		"HEVC":  0, // H.265 (MELHOR - melhor compressão)
		"AV1":   1, // AV1 (novo, excelente)
		"x265":  2, // H.265 (mesmo que HEVC)
		"H.264": 3, // H.264 (padrão)
		"x264":  4, // H.264
		"XviD":  5, // Antigo
	}

	normalized := strings.ToLower(codec)
	if priority, exists := priorityMap[normalized]; exists {
		return priority
	}

	return 999 // Codec desconhecido
}

// audioPriority retorna prioridade do áudio (menor = melhor)
// Baseado nas regras do documento (Seção 6.3 do RegrasFilmesBatches.md)
func audioPriority(audio string) int {
	priorityMap := map[string]int{
		"FLAC":   0, // Lossless (MELHOR)
		"DTS-HD": 1,
		"TrueHD": 2,
		"DDP":    3, // Dolby Digital Plus
		"AAC":    4, // AAC (comum)
		"AC3":    5, // Dolby Digital
		"MP3":    6, // MP3 (menor qualidade)
	}

	normalized := strings.ToLower(audio)
	if priority, exists := priorityMap[normalized]; exists {
		return priority
	}

	return 999 // Áudio desconhecido
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
	sorted := make([]TorrentResult, len(results))
	copy(sorted, results)

	sort.Slice(sorted, func(i, j int) bool {
		// 1. Priorizar torrents com "Uncensored" no título
		uncensoredI := isUncensored(sorted[i].Name)
		uncensoredJ := isUncensored(sorted[j].Name)

		if uncensoredI != uncensoredJ {
			return uncensoredI // Se i é Uncensored e j não, i vem primeiro
		}

		// 2. Comparar por prioridade de resolução
		if sorted[i].Resolution != nil && sorted[j].Resolution != nil {
			priorityI := resolutionPriority(*sorted[i].Resolution)
			priorityJ := resolutionPriority(*sorted[j].Resolution)

			if priorityI != priorityJ {
				return priorityI < priorityJ // Menor prioridade = melhor
			}
		} else if sorted[i].Resolution != nil {
			return true // i tem resolução, j não
		} else if sorted[j].Resolution != nil {
			return false // j tem resolução, i não
		}

		// 3. Comparar por fansub
		fansubI := fansubPriority(sorted[i].Name)
		fansubJ := fansubPriority(sorted[j].Name)
		if fansubI != fansubJ {
			return fansubI < fansubJ // Menor prioridade = melhor
		}

		// 4. Comparar por número de seeders (converter string para int)
		seedersI := parseSeeders(sorted[i].Seeders)
		seedersJ := parseSeeders(sorted[j].Seeders)
		if seedersI != seedersJ {
			return seedersI > seedersJ // Mais seeders = melhor
		}

		// 5. Comparar por tamanho (menor é melhor para mesma qualidade)
		// Apenas se resolução for a mesma
		if sorted[i].Resolution != nil && sorted[j].Resolution != nil {
			if *sorted[i].Resolution == *sorted[j].Resolution {
				return sorted[i].Size < sorted[j].Size
			}
		}

		return false
	})

	return sorted
}

// parseSeeders converte a string de seeders para int
func parseSeeders(seedersStr string) int {
	// Remover espaços e converter
	seedersStr = strings.TrimSpace(seedersStr)
	if seedersStr == "" || seedersStr == "-" {
		return 0
	}

	// Tentar converter (pode ter formatação como "1,234")
	seedersStr = strings.ReplaceAll(seedersStr, ",", "")
	if seeders, err := strconv.Atoi(seedersStr); err == nil {
		return seeders
	}

	return 0
}

// parseSize converte a string de tamanho para bytes
// Formatos comuns: "1.5 GiB", "500 MiB", "2.3 GiB", etc.
func parseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" || sizeStr == "-" {
		return 0
	}

	// Regex para extrair número e unidade
	re := regexp.MustCompile(`(?i)^([\d.]+)\s*([KMGT]?i?B)$`)
	matches := re.FindStringSubmatch(sizeStr)

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

// generateSearchQueries gera múltiplas queries de busca em ordem de prioridade
// Baseado nas regras do Nyaa (Seção 6 do documento de regras)
func generateSearchQueries(baseName string, season *int, episode int) []string {
	queries := []string{}

	// Extrair temporada do nome se presente
	sanitizedName := extractSeasonFromName(baseName)

	// 1. Query mais específica: nome completo + episódio
	queries = append(queries, fmt.Sprintf("%s %d", sanitizedName, episode))

	// 2. Queries com temporada explícita
	if season != nil {
		queries = append(queries, fmt.Sprintf("%s S%d %d", sanitizedName, *season, episode))
		queries = append(queries, fmt.Sprintf("%s Season %d %d", sanitizedName, *season, episode))
		queries = append(queries, fmt.Sprintf("%s %dnd Season %d", sanitizedName, *season, episode))
	}

	// 3. Tentar nome curto (última palavra ou parte principal)
	shortName := extractShortName(sanitizedName)
	if shortName != sanitizedName {
		queries = append(queries, fmt.Sprintf("%s %d", shortName, episode))
	}

	return queries
}

// extractSeasonFromName remove informações de temporada do nome para query base
func extractSeasonFromName(name string) string {
	// Padrões para remover do nome
	patterns := []string{
		`(?i)\s+Season\s*\d+`,
		`(?i)\s+S\s*\d+`,
		`(?i)\s+\d+(?:st|nd|rd|th)\s+Season`,
		`(?i)\s+Cour\s*\d+`,
	}

	result := name
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}

	return strings.TrimSpace(result)
}

// extractShortName extrai um nome curto do anime para busca genérica
// Geralmente é a última palavra significativa ou uma versão abreviada
func extractShortName(name string) string {
	// Dividir por espaços
	parts := strings.Fields(name)

	if len(parts) == 0 {
		return name
	}

	// Se tiver mais de 3 palavras, tentar usar apenas as últimas 2-3
	if len(parts) > 3 {
		// Pegar as últimas 2 palavras
		return strings.Join(parts[len(parts)-2:], " ")
	}

	return name
}

// fetchNyaaResults busca torrents no Nyaa para uma query específica
// Retorna todos os resultados encontrados sem filtrar
func fetchNyaaResults(query string) ([]TorrentResult, error) {
	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders") // Ordenar por seeders
	params.Set("o", "desc")    // Ordem decrescente

	baseURL := getNyaaBaseURL()
	nyaaURL := fmt.Sprintf("%s/?%s", baseURL, params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("query", query).
		Msg("Fetching Nyaa results")

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

		if name == "" {
			return
		}

		results = append(results, TorrentResult{
			Name:       name,
			Seeders:    seeders,
			MagnetLink: torrentLink,
			Size:       size,
		})
	})

	return results, nil
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
	sorted := make([]TorrentResult, len(results))
	copy(sorted, results)

	sort.Slice(sorted, func(i, j int) bool {
		// Extrair informações técnicas
		sourceI := extractSource(sorted[i].Name)
		sourceJ := extractSource(sorted[j].Name)

		codecI := extractCodec(sorted[i].Name)
		codecJ := extractCodec(sorted[j].Name)

		audioI := extractAudio(sorted[i].Name)
		audioJ := extractAudio(sorted[j].Name)

		// 1. Comparar por source
		if sourceI != "" && sourceJ != "" {
			priorityI := sourcePriority(sourceI)
			priorityJ := sourcePriority(sourceJ)
			if priorityI != priorityJ {
				return priorityI < priorityJ // Menor prioridade = melhor
			}
		} else if sourceI != "" {
			return true // i tem source, j não
		} else if sourceJ != "" {
			return false // j tem source, i não
		}

		// 2. Comparar por resolução
		if sorted[i].Resolution != nil && sorted[j].Resolution != nil {
			priorityI := resolutionPriority(*sorted[i].Resolution)
			priorityJ := resolutionPriority(*sorted[j].Resolution)
			if priorityI != priorityJ {
				return priorityI < priorityJ
			}
		} else if sorted[i].Resolution != nil {
			return true
		} else if sorted[j].Resolution != nil {
			return false
		}

		// 3. Comparar por codec
		if codecI != "" && codecJ != "" {
			priorityI := codecPriority(codecI)
			priorityJ := codecPriority(codecJ)
			if priorityI != priorityJ {
				return priorityI < priorityJ
			}
		} else if codecI != "" {
			return true
		} else if codecJ != "" {
			return false
		}

		// 4. Comparar por fansub
		fansubI := fansubPriority(sorted[i].Name)
		fansubJ := fansubPriority(sorted[j].Name)
		if fansubI != fansubJ {
			return fansubI < fansubJ
		}

		// 5. Comparar por áudio
		if audioI != "" && audioJ != "" {
			priorityI := audioPriority(audioI)
			priorityJ := audioPriority(audioJ)
			if priorityI != priorityJ {
				return priorityI < priorityJ
			}
		}

		// 6. Comparar por seeders
		seedersI := parseSeeders(sorted[i].Seeders)
		seedersJ := parseSeeders(sorted[j].Seeders)
		if seedersI != seedersJ {
			return seedersI > seedersJ
		}

		// 7. Comparar por tamanho
		return sorted[i].Size < sorted[j].Size
	})

	return sorted
}
