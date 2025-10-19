package nyaa

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// httpGet is an indirection for http.Get so tests can replace it.
var httpGet = http.Get

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

// TorrentResult representa um resultado de torrent do Nyaa
type TorrentResult struct {
	Name       string    `json:"name"`
	Date       time.Time `json:"date"`
	Seeders    string    `json:"seeders"`
	MagnetLink string    `json:"magnetLink"`
	Episode    *int      `json:"episode,omitempty"`
	Resolution *string   `json:"resolution,omitempty"`
	Season     *int      `json:"season,omitempty"`
}

// ScrapNyaa busca torrents no Nyaa baseado no nome do anime e episódio
func ScrapNyaa(romajiName string, episode int) ([]TorrentResult, error) {
	// Extrair temporada solicitada (se houver) e sanitizar o nome base
	requestedSeason := extractSeason(romajiName)

	// Remover informações de temporada do nome para formar a query base
	seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season)`)
	sanitizedRomajiName := seasonPattern.ReplaceAllString(romajiName, "")

	query := strings.TrimSpace(sanitizedRomajiName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	// params.Set("q", fmt.Sprintf())     // Query de busca
	params.Set("q", fmt.Sprintf("%s %02d", query, episode)) // Query de busca com episódio
	params.Set("s", "seeders")                              // Ordenar por seeders
	params.Set("o", "desc")                                 // Ordem decrescente

	nyaaURL := fmt.Sprintf("https://nyaa.si/?%s", params.Encode())

	fmt.Printf("Searching Nyaa: %s (Episode: %v)\n", nyaaURL, episode)

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

		dateString := strings.TrimSpace(cells.Eq(4).Text())
		seeders := strings.TrimSpace(cells.Eq(5).Text())

		// Parsear a data
		parsedDate, err := parseNyaaDate(dateString)
		if err != nil || !isWithinThreeMonths(parsedDate) {
			return // Pular se a data não puder ser parseada ou for muito antiga
		}

		// Extrair número do episódio, temporada e resolução do nome
		var animeEpisode *int
		var season *int
		var resolution *string

		if name != "" {
			animeEpisode = extractEpisodeNumber(name)
			season = extractSeason(name)
			res := extractResolution(name)
			resolution = &res
		}

		// Filtrar por título base (garantir que o torrent pertence ao anime)
		baseTitle := strings.ToLower(query)
		if baseTitle != "" && !strings.Contains(strings.ToLower(name), baseTitle) {
			return
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
			Date:       parsedDate,
			Seeders:    seeders,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Resolution: resolution,
		})
	})

	// Filtrar por episódio se especificado e retornar apenas os 5 primeiros
	// if episode != nil {
	// 	var filteredResults []TorrentResult
	// 	for _, result := range results {
	// 		if result.Episode != nil && *result.Episode == *episode {
	// 			filteredResults = append(filteredResults, result)
	// 		}
	// 	}
	// 	results = filteredResults
	// }

	fmt.Printf("Found %d torrents\n", len(results))

	if len(results) == 0 {
		return nil, nil // Nenhum resultado encontrado
	}

	return results, nil
}

// extractEpisodeNumber extrai o número do episódio do nome do torrent
func extractEpisodeNumber(name string) *int {
	// Padrões de episódio para corresponder - ordenados do mais específico ao menos específico
	patterns := []string{
		`(?i)S\d+E(\d+)`,            // S01E01, S1E1
		`(?i)\s-\s(\d+)(?:\s|$|\[)`, // - 03 (seguido por espaço, fim, ou colchete)
		`(?i)EP(\d+)`,               // EP03, ep3
		`(?i)episode\s*(\d+)`,       // episode 03, Episode 3
		`(?i)\bE(\d+)\b`,            // E01, e1 (standalone)
		`(?i)\[(\d+)\]`,             // [01], [1] - para episódios entre colchetes
		`(?i)\s(\d+)$`,              // 5 (número no final do nome)
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if episodeNum, err := strconv.Atoi(matches[1]); err == nil {
				if episodeNum > 0 && episodeNum < 1000 { // faixa razoável de episódios
					return &episodeNum
				}
			}
		}
	}

	return nil
}

// extractSeason extrai o número da temporada do nome do torrent
func extractSeason(name string) *int {
	// Padrões de temporada para corresponder
	patterns := []string{
		`(?i)S(\d+)`,                        // S1, S2, S01, S02
		`(?i)Season\s*(\d+)`,                // Season 1, Season 2
		`(?i)(\d+)(?:st|nd|rd|th)\s+Season`, // 3rd Season, 2nd Season
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if seasonNum, err := strconv.Atoi(matches[1]); err == nil {
				return &seasonNum
			}
		}
	}

	return nil
}

// extractResolution extrai a resolução do nome do torrent
func extractResolution(name string) string {
	// Padrões de resolução comuns
	patterns := []string{
		`(?i)(\d{3,4}p)`,        // 1080p, 720p, 480p, 2160p
		`(?i)(\d{3,4}x\d{3,4})`, // 1920x1080, 1280x720
		`(?i)(4K)`,              // 4K
		`(?i)(8K)`,              // 8K
		`(?i)(HD)`,              // HD
		`(?i)(FHD)`,             // FHD (Full HD)
		`(?i)(UHD)`,             // UHD (Ultra HD)
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
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
