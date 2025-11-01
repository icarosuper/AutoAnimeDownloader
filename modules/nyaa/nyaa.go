package nyaa

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
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
func ScrapNyaa(animeName string, episode int) ([]TorrentResult, error) {
	// Extrair temporada solicitada (se houver) e sanitizar o nome base
	requestedSeason := extractSeason(animeName)

	// Remover informações de temporada do nome para formar a query base
	seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season)`)
	sanitizedRomajiName := seasonPattern.ReplaceAllString(animeName, "")

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

	fmt.Printf("Searching Nyaa: %s\n", nyaaURL)

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

		//dateString := strings.TrimSpace(cells.Eq(4).Text())
		seeders := strings.TrimSpace(cells.Eq(5).Text())

		// Parsear a data
		//parsedDate, err := parseNyaaDate(dateString)
		//if err != nil || !isWithinThreeMonths(parsedDate) {
		//	return // Pular se a data não puder ser parseada ou for muito antiga
		//}

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
			Name: name,
			//Date:       parsedDate,
			Seeders:    seeders,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Resolution: resolution,
		})
	})

	fmt.Printf("Found %v result for %s ep %v\n", len(results), animeName, episode)

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
	// seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season)`)
	// sanitizedRomajiName := seasonPattern.ReplaceAllString(animeName, "")

	query := strings.TrimSpace(animeName)

	// Construir URL com parâmetros
	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	// params.Set("q", fmt.Sprintf())     // Query de busca
	params.Set("q", fmt.Sprintf("%s", query)) // Query de busca com episódio
	params.Set("s", "seeders")                // Ordenar por seeders
	params.Set("o", "desc")                   // Ordem decrescente

	nyaaURL := fmt.Sprintf("https://nyaa.si/?%s", params.Encode())

	fmt.Printf("Searching Nyaa: %s\n", nyaaURL)

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

		//dateString := strings.TrimSpace(cells.Eq(4).Text())
		seeders := strings.TrimSpace(cells.Eq(5).Text())

		// Parsear a data
		//parsedDate, err := parseNyaaDate(dateString)
		//if err != nil || !isWithinThreeMonths(parsedDate) {
		//	return // Pular se a data não puder ser parseada ou for muito antiga
		//}

		// Extrair número do episódio, temporada e resolução do nome
		var animeEpisode *int
		var season *int
		var resolution *string

		if name == "" {
			return
		}

		animeEpisode = extractEpisodeNumber(name)
		season = extractSeason(name)
		res := extractResolution(name)
		resolution = &res

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

		if animeEpisode == nil {
			return
		}

		epIsInWantedEpisodes := slices.Contains(episodes, *animeEpisode)
		if !epIsInWantedEpisodes {
			return
		}

		// Adicionar resultado ao array
		results = append(results, TorrentResult{
			Name: name,
			//Date:       parsedDate,
			Seeders:    seeders,
			MagnetLink: torrentLink,
			Episode:    animeEpisode,
			Season:     season,
			Resolution: resolution,
		})
	})

	fmt.Printf("Found %v result for %s\n", len(results), animeName)

	if len(results) == 0 {
		return nil, nil // Nenhum resultado encontrado
	}

	// Ordenar resultados por qualidade e fansub
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
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
		`(?i)\b(\d+)\s+-\s+\d+`,             // 2 - 07 (número seguido de hífen e outro número)
		`(?i)\b(\d+)\s+EP\d+`,               // 2 EP07 (número seguido de EP e outro número)
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

// resolutionPriority retorna um valor de prioridade para a resolução (menor = melhor)
func resolutionPriority(resolution string) int {
	priorityMap := map[string]int{
		"1080p": 0,
		"720p":  1,
		"480p":  2,
		"4k":    3,
		"8k":    4,
		"fhd":   5,
		"hd":    6,
		"uhd":   7,
	}

	normalized := strings.ToLower(resolution)
	if priority, exists := priorityMap[normalized]; exists {
		return priority
	}

	return 999 // Resolução desconhecida tem menor prioridade
}

// fansubPriority retorna um valor de prioridade para o fansub (menor = melhor)
func fansubPriority(torrentName string) int {
	fansubPriorities := map[string]int{
		"subsplease": 0,
		"erai-raws":  1,
		"judas":      2,
		"toonshub":   3,
		"asw":        4,
		"ember":      5,
		"dub":        6,
		"hd-zone":    7,
		"kamig":      8,
		"remix":      9,
		"aniverse":   10,
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

// SortTorrentResults ordena os torrents por qualidade (primeiro) e fansub (segundo)
// A ordenação é feita por qualidade em ordem decrescente (1080p > 720p > ...)
// e depois por fansub na ordem especificada
func SortTorrentResults(results []TorrentResult) []TorrentResult {
	sorted := make([]TorrentResult, len(results))
	copy(sorted, results)

	sort.Slice(sorted, func(i, j int) bool {
		// Se ambos têm resolução, comparar por prioridade de resolução
		if sorted[i].Resolution != nil && sorted[j].Resolution != nil {
			priorityI := resolutionPriority(*sorted[i].Resolution)
			priorityJ := resolutionPriority(*sorted[j].Resolution)

			if priorityI != priorityJ {
				return priorityI < priorityJ // Menor prioridade = melhor
			}

			// Se resoluções são iguais, comparar por fansub
			fansubI := fansubPriority(sorted[i].Name)
			fansubJ := fansubPriority(sorted[j].Name)
			return fansubI < fansubJ
		}

		// Se apenas um tem resolução, prefere o que tem
		if sorted[i].Resolution != nil {
			return true
		}
		if sorted[j].Resolution != nil {
			return false
		}

		// Se nenhum tem resolução, comparar por fansub
		fansubI := fansubPriority(sorted[i].Name)
		fansubJ := fansubPriority(sorted[j].Name)
		return fansubI < fansubJ
	})

	return sorted
}
