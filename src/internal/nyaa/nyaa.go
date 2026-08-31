package nyaa

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	Leechers   int       `json:"leechers,omitempty"`
	MagnetLink string    `json:"magnetLink"`
	Episode    *int      `json:"episode,omitempty"`
	Resolution *string   `json:"resolution,omitempty"`
	Season     *int      `json:"season,omitempty"`
	Part       *int      `json:"part,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Fansub     string    `json:"fansub,omitempty"`
	IsBatch    bool      `json:"IsBatch,omitempty"`
	// DetailURL e a pagina /view/<id> da linha. So o caminho de pack a usa (PackFileRange), e
	// so as buscas que devolvem pack a preenchem.
	DetailURL string `json:"detailUrl,omitempty"`
}

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".webm": true, ".flv": true, ".wmv": true, ".ts": true, ".mpg": true,
	".mpeg": true, ".ogm": true,
}

// IsVideoFile reporta se o nome tem extensao de video. Mora neste pacote, e nao no files, porque
// os dois leem a mesma lista de arquivos de um pack — o Librarian depois de baixar, PackFileRange
// antes — e files importa nyaa (o contrario daria ciclo).
func IsVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(name))]
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
		cleanRomaji := removeSpecialCharacters(romaji)
		addVariant(cleanRomaji)

		// Prioridade 2: Romaji original
		addVariant(romaji)
	}

	// Prioridade 3: English limpo (sem caracteres especiais)
	if english != "" && english != romaji {
		cleanEnglish := removeSpecialCharacters(english)
		addVariant(cleanEnglish)

		// Prioridade 4: English original
		addVariant(english)
	}

	return variants
}

// removeSpecialCharacters normaliza para minusculas com apenas letras, digitos e espaco.
// Morava num pacote stringutil proprio, com esta unica funcao e este unico chamador.
func removeSpecialCharacters(s string) string {
	s = strings.ToLower(s)
	s = reSpecialChars.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
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

// IsBatch verifica se o torrent é um batch (múltiplos episódios)
func IsBatch(name string) bool {
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
}

// ExtractBatchInfo extrai informações detalhadas de um batch
// Consumida fora do pacote pelo daemon: e a faixa dela que decide QUAIS episodios recebem o
// magnet do pack (pickBatches).
func ExtractBatchInfo(torrentName string) BatchInfo {
	info := BatchInfo{}

	if start, last, ok := batchRange(torrentName); ok {
		info.StartEpisode, info.EndEpisode = start, last
	}

	if season := ExtractSeason(torrentName); season != nil {
		info.Season = *season
	}

	return info
}

// batchRange devolve a primeira faixa PLAUSIVEL do nome, e nao o primeiro casamento: reBatchRange
// casa qualquer par de numeros ligados por "-" ou "~", e boa parte deles nao e episodio.
//
// FindAll (e nao FindString) porque um casamento reprovado nao encerra a busca — "[EMBER] ...
// (2021-2022) ..." tem o ano ANTES de onde uma faixa de verdade estaria. Ele e nao-sobreposto, o
// que mantem "Hibike! Euphonium 2 - 01 ~ 13" sem faixa: o "01" e consumido pelo casamento
// invertido "2 - 01" e o "01 ~ 13" nunca e visto. Faixa desconhecida cai em EndEpisode == 0, que
// o chamador trata como cobertura a resolver pela lista de arquivos.
//
// Indices (e nao FindAllStringSubmatch) porque a guarda de resolucao precisa do caractere
// SEGUINTE ao casamento.
func batchRange(torrentName string) (int, int, bool) {
	for _, loc := range reBatchRange.FindAllStringSubmatchIndex(torrentName, -1) {
		// "[720-1080p]" nao e uma faixa de 361 episodios.
		if end := loc[1]; end < len(torrentName) && strings.ContainsRune("pPiI", rune(torrentName[end])) {
			continue
		}
		start, errStart := strconv.Atoi(torrentName[loc[2]:loc[3]])
		last, errEnd := strconv.Atoi(torrentName[loc[4]:loc[5]])
		if errStart != nil || errEnd != nil {
			continue
		}
		// Faixa invertida nao e faixa: em "Hibike! Euphonium 2 - 01 ~ 13" o casamento pega
		// "2 - 01" (numero da temporada + primeiro episodio) e produz 2..1, que nao cobre
		// episodio nenhum — o pack sumia de pickBatches e um pack pior era baixado no lugar.
		if last <= start {
			continue
		}
		// Ano de lancamento: "[EMBER] ... (2021-2022) ..." virava a faixa 2021..2022, que
		// nenhuma hipotese de eixo cobre — o pack completo era descartado em silencio. A banda
		// so pega ano: nenhuma serie chega ao episodio 1900, e One Piece "1001-1100" passa.
		if start >= 1900 && last <= 2100 {
			continue
		}
		return start, last, true
	}
	return 0, 0, false
}

// PackFileRange devolve a faixa de episodios que um pack REALMENTE traz, lida da lista de
// arquivos da pagina de detalhe (`/view/<id>`), na numeracao que o grupo usou nos arquivos —
// a mesma regua do nome, entao o chamador converte com o mesmo packAxis.
//
// Existe porque o nome nao responde: medido em 24 packs de tres animes, 19 nao trazem faixa
// nenhuma (sources.md, "Pagina de detalhe"). Sem esta chamada, faixa desconhecida vale como
// "cobre tudo" e o daemon grava como baixado episodio que o pack nao tem. O caso que forcou:
// "[EMBER] … (Season 2 | Part 2) … (Batch)" nao declara faixa e traz os arquivos 13..24 —
// intersecao ZERO com o 1..12 que o daemon registrava.
//
// (info, false) quando o detalhe nao responde, nao tem lista de arquivos ou nenhum arquivo de
// video da o numero: nao ha faixa a afirmar, e quem chama decide o que fazer com o "nao sei".
//
// A faixa e min..max dos numeros lidos, entao pack cujos arquivos REINICIAM a numeracao por
// season (S01-S04 numa pasta cada) fica com a faixa da maior season em vez do total. MEDIDO em
// 30/ago/2026 e mantido de proposito: sao 6% dos packs, o delta 0 do packAxis escolhe todos eles
// corretamente mesmo assim, e o desempate por contagem de arquivos por pasta contaria extras
// ("Creditless Opening 1" -> episodio 1) como episodio. Numeros em decisions.md #84.
func PackFileRange(detailURL string) (BatchInfo, bool) {
	if detailURL == "" {
		return BatchInfo{}, false
	}
	doc, err := fetchNyaaPage(detailURL)
	if err != nil {
		logger.Logger.Debug().Err(err).Str("url", detailURL).Msg("Failed to fetch the torrent detail page")
		return BatchInfo{}, false
	}

	first, last, files := 0, 0, 0
	doc.Find(".torrent-file-list li").Each(func(_ int, s *goquery.Selection) {
		// Pasta e nao arquivo: o nome dela costuma trazer a faixa do release inteiro
		// ("… 001-206 [4x3]"), que e justamente o numero que nao vale para contar cobertura.
		if s.Children().First().Is("a.folder") {
			return
		}
		name := fileListEntryName(s)
		if !IsVideoFile(name) {
			return
		}
		// Numero ilegivel nao entra: e assim que NCOP/NCED e extras ficam de fora da faixa em
		// vez de alarga-la.
		episode := ExtractEpisodeNumber(name)
		if episode == nil {
			return
		}
		if files == 0 || *episode < first {
			first = *episode
		}
		if *episode > last {
			last = *episode
		}
		files++
	})

	if files == 0 {
		logger.Logger.Debug().Str("url", detailURL).Msg("Torrent detail page has no readable episode number")
		return BatchInfo{}, false
	}

	logger.Logger.Debug().
		Str("url", detailURL).
		Int("files", files).
		Int("first_episode", first).
		Int("last_episode", last).
		Msg("Pack range read from the torrent file list")

	return BatchInfo{StartEpisode: first, EndEpisode: last}, true
}

// detailURL absolutiza o href da coluna de titulo ("/view/1323474"). Vazio quando a linha nao
// traz link — sem ele PackFileRange nao tem o que buscar e a faixa fica desconhecida.
func detailURL(href string) string {
	if !strings.HasPrefix(href, "/") {
		return ""
	}
	return getNyaaBaseURL() + href
}

// fileListEntryName e o nome do arquivo numa linha da lista, sem o tamanho que vem junto no
// mesmo <li> ("… - 001.mkv (259.9 MiB)") e sem os filhos de uma sublista.
func fileListEntryName(s *goquery.Selection) string {
	entry := s.Clone()
	entry.Find("ul").Remove()
	entry.Find("span.file-size").Remove()
	return strings.TrimSpace(entry.Text())
}

// isMovie verifica se o torrent é um filme
// Se isFormatMovie for true, usa o formato do AniList (mais confiável)
func isMovie(torrentName, animeName string, isFormatMovie bool) bool {
	if isFormatMovie {
		return true
	}

	if hasMovieMarker(torrentName) {
		return true
	}

	if !reHasEpisode.MatchString(torrentName) && !IsBatch(torrentName) {
		return true
	}

	return false
}

// hasMovieMarker reporta se o nome traz marcador explícito de filme, OVA/ONA ou
// special. É a parte de isMovie que serve como guard na busca de episódio: o
// ramo "não tem marcador de episódio" de isMovie não pode ser usado lá porque
// reHasEpisode é mais restrito que ExtractEpisodeNumber (não cobre EP05, E05,
// [05], " 05 (" etc.) e rejeitaria episódios legítimos.
func hasMovieMarker(torrentName string) bool {
	for _, re := range reMovieKeywords {
		if re.MatchString(torrentName) {
			return true
		}
	}
	return reOvaPattern.MatchString(torrentName) || reSpecialPattern.MatchString(torrentName)
}

// enoughCandidates é o piso de candidatos aceitos que faz a busca parar de descer páginas.
// O Nyaa devolve ordenado por seeders desc, então a partir de um punhado de candidatos a
// página seguinte só traz opções piores — 3 já dá escolha ao ranking.
const enoughCandidates = 3

// maxSearchPages é o teto de páginas por busca (config max_search_pages), empurrado por
// files.LoadConfigs. Mesmo padrão de SetPriorities.
var maxSearchPages atomic.Int32

func init() { maxSearchPages.Store(5) }

// SetMaxSearchPages aplica o teto de páginas por busca e devolve uma função que restaura o
// valor anterior (padrão de SetPriorities/MockNyaaHttpGet). Valor <= 0 vale como 1 página.
func SetMaxSearchPages(pages int) (restore func()) {
	prev := maxSearchPages.Load()
	maxSearchPages.Store(int32(pages))
	return func() { maxSearchPages.Store(prev) }
}

// ActiveMaxSearchPages devolve o teto em uso, nunca menor que 1 (par de ActivePriorities).
func ActiveMaxSearchPages() int {
	if p := int(maxSearchPages.Load()); p > 1 {
		return p
	}
	return 1
}

// maxBatchSizeBytes é o teto de tamanho de pack (config max_batch_torrent_size_gb) em BYTES,
// empurrado por files.LoadConfigs igual a maxSearchPages. O daemon já descarta pack acima do teto
// em filterBySize, mas descartar SÓ lá é tarde demais: enoughCandidates conta o que a busca
// aceitou, então três packs gigantes na página 1 encerram a descida e os packs parciais que
// caberiam ficam nas páginas seguintes, nunca vistos (One Piece: os packs de 587 GiB / 171 GiB
// param a busca antes dos de 61 GiB / 20 GiB / 16 GiB). Aplicar o teto aqui mantém o contador
// honesto — só entra na conta pack que o daemon ainda pode usar.
//
// 0 desliga, e é o valor padrão: sem push de config (testes, uso do pacote isolado) o
// comportamento é o de antes.
var maxBatchSizeBytes atomic.Int64

// SetMaxBatchTorrentSizeGB aplica o teto de tamanho de pack e devolve uma função que restaura o
// valor anterior (padrão de SetMaxSearchPages). Valor <= 0 desliga o filtro.
func SetMaxBatchTorrentSizeGB(gb float64) (restore func()) {
	prev := maxBatchSizeBytes.Load()
	if gb <= 0 {
		maxBatchSizeBytes.Store(0)
	} else {
		maxBatchSizeBytes.Store(int64(gb * 1024 * 1024 * 1024))
	}
	return func() { maxBatchSizeBytes.Store(prev) }
}

// batchTooBig informa se o pack estoura o teto em uso. Tamanho 0 (não parseado) passa, mesma
// regra de daemon.filterBySize — um tamanho que não deu para ler não é motivo para descartar.
func batchTooBig(size int64) bool {
	max := maxBatchSizeBytes.Load()
	return max > 0 && size > 0 && size > max
}

// fetchSearchPages busca a página 1 de nyaaURL e continua para as seguintes ENQUANTO houver
// linhas e accepted() estiver abaixo de floor, até ActiveMaxSearchPages().
//
// parse recebe o documento da página e devolve quantas LINHAS ela trazia (não quantas foram
// aceitas): página vazia significa que a query acabou, e insistir seria fetch jogado fora.
//
// Adaptativo porque a página 2 era buscada SEMPRE: numa busca que já resolve na página 1 — a
// maioria — isso era um fetch desperdiçado, e é essa economia que paga o teto maior sem subir
// o tráfego médio contra o nyaa.si.
//
// Sequencial de propósito: cada página só é pedida porque a anterior não bastou. Ver
// decisions.md #57 para por que não há rajada paralela aqui.
//
// Só devolve erro se a PÁGINA 1 falhar (sem ela não há busca); falha em página seguinte
// encerra a descida em silêncio, que é o comportamento best-effort que a página 2 já tinha.
func fetchSearchPages(nyaaURL string, floor int, accepted func() int, parse func(*goquery.Document) int) error {
	maxPages := ActiveMaxSearchPages()
	for page := 1; page <= maxPages; page++ {
		pageURL := nyaaURL
		if page > 1 {
			pageURL = fmt.Sprintf("%s&p=%d", nyaaURL, page)
		}

		doc, err := fetchNyaaPage(pageURL)
		if err != nil {
			if page == 1 {
				return err
			}
			return nil
		}

		rows := parse(doc)
		if rows == 0 || accepted() >= floor {
			return nil
		}
	}
	return nil
}

// parsePagesWith devolve o parse para fetchSearchPages a partir do parseRow de cada busca.
func parsePagesWith(parseRow func(int, *goquery.Selection)) func(*goquery.Document) int {
	return func(doc *goquery.Document) int {
		rows := doc.Find(".torrent-list tbody tr")
		rows.Each(parseRow)
		return rows.Length()
	}
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

// torrentSummaries formats each result as "name | S:412/L:3 | 1.4GiB | t=5 h=4.21", in the
// order given (i.e. already sorted by priority when logged after SortTorrentResults).
// t é a faixa de saúde (o que o critério health compara); h é o score cru, que só informa.
//
// Só o nome não basta para auditar "por que esse ganhou": os campos que decidem o sort
// (seeders, tamanho, health) ficavam de fora e reproduzir a escolha exigia repetir a busca
// à mão. Tamanho desconhecido (parse falhou) sai como "?".
func torrentSummaries(results []TorrentResult) []string {
	summaries := make([]string, len(results))
	for i, r := range results {
		summaries[i] = fmt.Sprintf("%s | S:%d/L:%d | %s | t=%d h=%.2f",
			r.Name, ParseSeeders(r.Seeders), r.Leechers, formatSize(r.Size), healthTier(r), torrentHealthScore(r))
	}
	return summaries
}

// formatSize renders bytes for the log. Só para leitura humana — nada parseia de volta.
func formatSize(bytes int64) string {
	switch {
	case bytes <= 0:
		return "?"
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGiB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.0fMiB", float64(bytes)/(1<<20))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
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

// longSeriesEpisodes é o total de episódios a partir do qual a busca por episódio também
// tenta a query com zero-padding. Ver episodeQueries.
const longSeriesEpisodes = 100

// episodeQueries devolve as queries de busca do episódio: a simples ("one piece 1") e, em
// série longa, também a com padding de 3 dígitos ("one piece 001").
//
// Sem padding, `q=one piece 1` casa tudo que contém "1" e, ordenado por seeders, as primeiras
// centenas de linhas são os episódios 1160+ — o episódio 1 não aparece. Grupos numeram série
// longa com padding (`[Judas] One Piece 001-574`, `[Erai-raws] Naruto Shippuuden - 001 ~ 079`).
//
// É variante ADICIONAL, não substituição: padding é convenção, não regra, e a query simples
// continua pegando quem numera solto. Só entra acima de longSeriesEpisodes porque cada variante
// custa fetches, e episódio >= 100 já tem 3 dígitos (padding não mudaria nada).
func episodeQueries(query string, episode, totalEpisodes int) []string {
	queries := []string{fmt.Sprintf("%s %d", query, episode)}
	if totalEpisodes > longSeriesEpisodes && episode < 100 {
		queries = append(queries, fmt.Sprintf("%s %03d", query, episode))
	}
	return queries
}

// ScrapNyaa busca torrents no Nyaa baseado no nome do anime e episódio.
// requestedSeason e requestedPart são extraídos upstream dos dados do Anilist.
// totalEpisodes (opcional) é o total de episódios do anime, usado só para decidir o
// zero-padding da query — 0/ausente vale como desconhecido.
func ScrapNyaa(animeName string, episode int, requestedSeason, requestedPart *int, totalEpisodes ...int) ([]TorrentResult, error) {
	total := 0
	if len(totalEpisodes) > 0 {
		total = totalEpisodes[0]
	}

	sanitizedRomajiName := reSeasonStrip.ReplaceAllString(animeName, "")
	sanitizedRomajiName = rePartStrip.ReplaceAllString(sanitizedRomajiName, "")

	query := strings.TrimSpace(sanitizedRomajiName)

	buildURL := func(q string) string {
		params := url.Values{}
		params.Set("f", "0")       // Filtro: sem filtro
		params.Set("c", "1_2")     // Categoria: anime (english)
		params.Set("q", q)         // Query de busca com episódio
		params.Set("s", "seeders") // Ordenar por seeders
		params.Set("o", "desc")    // Ordem decrescente
		return fmt.Sprintf("%s/?%s", getNyaaBaseURL(), params.Encode())
	}

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
		leechers := ParseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		if name != "" {
			logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")
		}

		// Verificar se o torrent deve ser ignorado (dub, raw, hardcoded, etc.)
		if ShouldIgnore(name) {
			return
		}

		// Verificar se é batch - ignorar para busca de episódio único
		if IsBatch(name) {
			return
		}

		// Filme/OVA/special não é episódio: "Naruto Shippuuden Movie 3" casa o
		// padrão " 3 (" de ExtractEpisodeNumber e passaria como episódio 3.
		if hasMovieMarker(name) {
			return
		}

		// Extrair informações do nome do torrent
		var animeEpisode *int
		var season *int
		var resolution *string
		var fansub string

		if name != "" {
			animeEpisode = ExtractEpisodeNumber(name)
			season = ExtractSeason(name)
			res := ExtractResolution(name)
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
		part := ExtractPart(name)
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

	var searched bool
	var firstErr error
	for _, q := range episodeQueries(query, episode, total) {
		nyaaURL := buildURL(q)

		logger.Logger.Debug().
			Str("url", nyaaURL).
			Str("anime_name", animeName).
			Int("episode", episode).
			Msg("Searching Nyaa for single episode")

		err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		searched = true
	}
	// Só falha se NENHUMA variante respondeu: uma query que dá erro não pode anular a outra.
	if !searched {
		return nil, firstErr
	}

	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("episode", episode).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa results for single episode")

	if len(results) == 0 {
		return nil, nil // Nenhum resultado encontrado
	}

	// Ordenar resultados por qualidade e fansub
	sortedResults := SortTorrentResults(results)
	return sortedResults, nil
}

// ScrapNyaaForAnime busca UMA vez a pagina de resultados do anime e devolve packs e episodios na
// MESMA lista. As duas buscas separadas que esta funcao substitui montavam a mesma URL
// (?f=0&c=1_2&q=<titulo>&s=seeders&o=desc) e discordavam so no filtro de linha, o que custava
// duas descidas de ate max_search_pages sobre o mesmo HTML, por anime, por ciclo.
//
// Linha de pack entra com IsBatch: true; linha de episodio entra com Episode != nil. Quem
// particiona e o daemon (partitionSearchResults), que e onde os tetos de tamanho de pack e de
// episodio sao diferentes.
//
// Devolve UMA lista de proposito: searchNyaaWithVariants para na primeira variante de titulo com
// resultado, e "resultado" e uma fatia nao-vazia — uma assinatura de par exigiria generaliza-la.
func ScrapNyaaForAnime(animeName string, episodes []int, requestedSeason, requestedPart *int) ([]TorrentResult, error) {
	query := strings.TrimSpace(extractSeasonFromName(animeName))

	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders")
	params.Set("o", "desc")

	nyaaURL := fmt.Sprintf("%s/?%s", getNyaaBaseURL(), params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Int("episodes_count", len(episodes)).
		Msg("Searching Nyaa for anime (packs + episodes)")

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		if name == "" {
			return
		}
		logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")

		if ShouldIgnore(name) {
			return
		}
		// Filtrar por titulo base (garantir que o torrent pertence ao anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		season := ExtractSeason(name)
		part := ExtractPart(name)
		res := ExtractResolution(name)

		row := TorrentResult{
			Name:       name,
			Seeders:    strings.TrimSpace(cells.Eq(5).Text()),
			Leechers:   ParseSeeders(strings.TrimSpace(cells.Eq(6).Text())),
			MagnetLink: cells.Eq(2).Find("a").Eq(1).AttrOr("href", ""),
			Season:     season,
			Part:       part,
			Resolution: &res,
			Size:       parseSize(strings.TrimSpace(cells.Eq(3).Text())),
			Fansub:     extractFansub(name),
			DetailURL:  detailURL(cells.Eq(1).Find("a").Not(".comments").AttrOr("href", "")),
		}

		if IsBatch(name) {
			// Temporada no caminho de pack: sem pedido explicito, pack de qualquer temporada
			// serve. Era o comportamento da busca de batch antiga, e e diferente do caminho de
			// episodio logo abaixo — por isso os dois filtros nao podem ser fundidos.
			if requestedSeason != nil && (season == nil || *season != *requestedSeason) {
				return
			}
			// Part no caminho de pack: so rejeita CONFLITO declarado. Pack sem marcador de
			// part nao e pack errado — e o formato normal de lancamento, porque o pack e da
			// season inteira e a AniList e que quebra a season em cours (sources.md,
			// "Granularidade e numeracao dos packs"). Exigir o marcador zerava os packs de
			// toda entrada "Part N".
			//
			// Dali para a frente quem decide e a cobertura da faixa, no daemon (packAxis).
			// Pack que TAMBEM nao traz faixa no nome nao tem numero para conferir: a faixa
			// dele sai da lista de arquivos da pagina de detalhe (PackFileRange), e e isso
			// que distingue o pack da season inteira do pack so da Part 1 (decisions.md #84).
			if requestedPart != nil && part != nil && *part != *requestedPart {
				return
			}
			// Pack acima do teto sai aqui e não na filterBySize do daemon: aceitá-lo agora
			// contaria para enoughCandidates e encerraria a descida antes dos packs parciais
			// que cabem (ver maxBatchSizeBytes).
			if batchTooBig(row.Size) {
				logger.Logger.Debug().
					Str("torrent", name).
					Int64("size_bytes", row.Size).
					Msg("Batch above max_batch_torrent_size_gb, skipping row")
				return
			}
			row.IsBatch = true
			results = append(results, row)
			return
		}

		// Filme/OVA/special nao e episodio: "Naruto Shippuuden Movie 3" casa o padrao " 3 (" de
		// ExtractEpisodeNumber e passaria como episodio 3.
		if hasMovieMarker(name) {
			return
		}
		// Temporada no caminho de episodio: sem pedido explicito, so temporada 1 (ou sem
		// marcador) passa.
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else if season != nil && *season != 1 {
			return
		}
		if requestedPart != nil && (part == nil || *part != *requestedPart) {
			return
		}

		episode := ExtractEpisodeNumber(name)
		if episode == nil || !slices.Contains(episodes, *episode) {
			return
		}
		row.Episode = episode
		results = append(results, row)
	}

	// O piso de paginacao conta as duas listas somadas, entao pack que o daemon vai descartar por
	// tamanho nao pode entrar na conta — e por isso que o teto de pack chega aqui por push
	// (maxBatchSizeBytes), do mesmo jeito que max_search_pages. O filtro de seeders continua so
	// no daemon: ele nao trunca a busca da mesma forma, porque o Nyaa ja devolve ordenado por
	// seeders desc.
	if err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow)); err != nil {
		return nil, err
	}
	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa results for anime")

	if len(results) == 0 {
		return nil, nil
	}

	// Ordena a lista mista: particionar depois preserva a ordem relativa, entao cada lista sai
	// ordenada corretamente.
	return SortTorrentResults(results), nil
}

// ScrapNyaaForMovie busca torrents de filmes
// Detecta automaticamente filmes e ordena por qualidade técnica
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

	var results []TorrentResult

	// Parsear linhas da tabela de torrents
	parseRow := func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		torrentLink := cells.Eq(2).Find("a").Eq(1).AttrOr("href", "")
		sizeStr := strings.TrimSpace(cells.Eq(3).Text())
		size := parseSize(sizeStr)
		seeders := strings.TrimSpace(cells.Eq(5).Text())
		leechers := ParseSeeders(strings.TrimSpace(cells.Eq(6).Text()))

		if name != "" {
			logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")
		}

		// Verificar se o torrent deve ser ignorado
		if ShouldIgnore(name) {
			return
		}

		// Verificar se é filme
		if !isMovie(name, animeName, isMovieFormat) {
			return
		}

		// Extrair informações
		res := ExtractResolution(name)
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
	}

	if err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow)); err != nil {
		return nil, err
	}

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa movie results")

	if len(results) == 0 {
		return nil, nil // Nenhum filme encontrado
	}

	// Ordenar resultados usando ordenação específica para filmes
	sortedResults := SortMovieResults(results)
	return sortedResults, nil
}

// ExtractEpisodeNumber extrai o número do episódio do nome do torrent
// Testa os padrões em ordem de prioridade (do mais específico ao menos específico)
// O underscore vira espaco antes de tudo: todo padrao de reEpisodePatterns pede espaco ao redor
// do numero, e grupo que separa com "_" ("[DB]Vinland Saga_-_01_(...).mkv") nao casava nenhum.
// Vale para nome de torrent e, principalmente, para nome de ARQUIVO dentro de um pack — e daqui
// que o Librarian tira o numero para renomear no padrao Jellyfin.
func ExtractEpisodeNumber(name string) *int {
	name = strings.ReplaceAll(name, "_", " ")
	for _, p := range reEpisodePatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			if episodeNum, err := strconv.Atoi(matches[1]); err == nil {
				if episodeNum > 0 && episodeNum < 10000 {
					return &episodeNum
				}
			}
		}
	}
	return nil
}

// romanSeasonValues converts the roman numerals matched by reRomanSeason to their
// integer season number.
var romanSeasonValues = map[string]int{
	"II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
}

// ExtractSeason extrai o número da temporada do nome do torrent
// Testa os padrões em ordem de prioridade
func ExtractSeason(name string) *int {
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
	if matches := reRomanSeason.FindStringSubmatch(name); len(matches) > 1 {
		if seasonNum, ok := romanSeasonValues[matches[1]]; ok {
			return &seasonNum
		}
	}
	return nil
}

// ExtractPart extrai o número da parte/cour do nome do torrent ou título Anilist
// Testa os padrões em ordem de prioridade (mais específico primeiro)
//
// Nome que declara MAIS DE UMA part não tem part: "(Part 1 + Part 2)" e "(Season 4 Part 03+04)"
// cobrem as duas metades, e devolver a primeira que casa fazia o pack ser lido como "da part 1" —
// justamente o pack que serve para as duas. Sem número único a resposta é nil, e quem decide passa
// a ser a cobertura da faixa (ver daemon.packAxis e sources.md, item 4).
func ExtractPart(name string) *int {
	if declared := declaredParts(name); len(declared) > 1 {
		return nil
	}

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

// declaredParts sao os numeros de part/cour DISTINTOS que o nome declara, sem zero a esquerda.
func declaredParts(name string) map[int]bool {
	declared := make(map[int]bool, 2)
	for _, m := range reAnyPart.FindAllStringSubmatch(name, -1) {
		for _, group := range m[1:] {
			if n, err := strconv.Atoi(group); err == nil {
				declared[n] = true
			}
		}
	}
	return declared
}

// canonicalResolutions mapeia os apelidos de resolução nos tokens canônicos das listas
// de Priorities. Sem isso "4k" e "2160p" são a mesma resolução ocupando dois índices
// diferentes da lista, e qual vence depende só de como o grupo escreveu o nome do torrent.
// "8k" fica de fora de propósito: continua sendo devolvido cru e, por não estar na lista
// default, ranqueia como o pior — que é o tratamento certo para uma resolução que não
// existe em anime.
var canonicalResolutions = map[string]string{"4k": "2160p", "uhd": "2160p", "fhd": "1080p", "hd": "720p"}

// ExtractResolution extrai a resolução do nome do torrent, já canonicalizada
// (mesmo contrato de ExtractCodec, que devolve "HEVC" para "x265").
func ExtractResolution(name string) string {
	for _, p := range reResolutionPatterns {
		matches := p.re.FindStringSubmatch(name)
		if len(matches) > 1 {
			res := strings.ToLower(matches[1])
			if canon, ok := canonicalResolutions[res]; ok {
				return canon
			}
			// "1920x1080" -> "1080p": a altura é o que nomeia a resolução.
			if _, height, ok := strings.Cut(res, "x"); ok {
				return height + "p"
			}
			return res
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

// ExtractCodec extrai o codec de vídeo (HEVC, H.264, AV1, etc.)
func ExtractCodec(name string) string {
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
func sourcePriority(source string) int {
	return priorityIndex(ActivePriorities().Sources, source)
}

// codecPriority retorna prioridade do codec (menor = melhor)
func codecPriority(codec string) int {
	return priorityIndex(ActivePriorities().Codecs, codec)
}

// audioPriority retorna prioridade do áudio (menor = melhor)
func audioPriority(audio string) int {
	return priorityIndex(ActivePriorities().Audio, audio)
}

// SortTorrentResults ordena os torrents por qualidade, aplicando os critérios de
// CriteriaOrder que valem para episódio (episodeCriteria): uncensored, resolução,
// codec, health, fansub e tamanho. Só `source` fica de fora — episódio semanal é
// WEB-DL em ~todos os casos, então o critério nunca desempataria nada.
// Baseado nas regras do Nyaa (Seção 8 do documento de regras)
func SortTorrentResults(results []TorrentResult) []TorrentResult {
	return sortByCriteria(results, filterCriteria(ActivePriorities().CriteriaOrder, episodeCriteria))
}

// ParseSeeders converte a string de seeders para int (a coluna do Nyaa pode vir "-").
// Consumida fora do pacote pelo piso de seeders em daemon.filterBySeeders.
func ParseSeeders(seedersStr string) int {
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
	seeders := ParseSeeders(r.Seeders)
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
