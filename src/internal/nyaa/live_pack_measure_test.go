package nyaa

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Medicao ao vivo dos dois tetos de pack da Etapa 1 do TODO:
//
//  1. numeracao que REINICIA por season (teto conhecido da decisions.md #84): PackFileRange
//     devolve min..max dos numeros lidos, entao um pack S01-S04 com uma pasta por season fica
//     com a faixa da MAIOR season em vez do total.
//  2. teto de pack por episodio em vez de por tamanho total (sources.md, "Pagina de detalhe",
//     item 2): so vale se o teto atual estiver reprovando pack que o usuario quer.
//
// So roda com AAD_LIVE_NYAA=1, para o `go test ./...` nao depender de rede.

// folderStats sao os arquivos de video de uma pasta da lista de arquivos.
type folderStats struct {
	path     string
	episodes []int
	seasons  map[int]int // season lida do NOME DO ARQUIVO -> quantos arquivos
	bytes    int64
}

func (f folderStats) span() (int, int) {
	lo, hi := f.episodes[0], f.episodes[0]
	for _, e := range f.episodes {
		if e < lo {
			lo = e
		}
		if e > hi {
			hi = e
		}
	}
	return lo, hi
}

// walkFileList percorre a arvore da lista de arquivos atribuindo cada arquivo de video a pasta
// que o contem. Mesma leitura de PackFileRange (IsVideoFile + extractEpisodeNumber), so que
// agrupada por pasta em vez de achatada num min..max unico.
func walkFileList(sel *goquery.Selection, path string, out *[]folderStats) {
	cur := folderStats{path: path, seasons: map[int]int{}}
	sel.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
		if link := li.Children().First(); link.Is("a.folder") {
			child := strings.TrimSpace(link.Text())
			sub := child
			if path != "" {
				sub = path + "/" + child
			}
			walkFileList(li.ChildrenFiltered("ul"), sub, out)
			return
		}
		name := fileListEntryName(li)
		if !IsVideoFile(name) {
			return
		}
		cur.bytes += parseSize(strings.Trim(strings.TrimSpace(li.Find("span.file-size").Text()), "()"))
		if s := extractSeason(name); s != nil {
			cur.seasons[*s]++
		}
		if ep := extractEpisodeNumber(name); ep != nil {
			cur.episodes = append(cur.episodes, *ep)
		}
	})
	if len(cur.episodes) > 0 {
		*out = append(*out, cur)
	}
}

// isExtrasFolder reconhece pasta cujos arquivos NAO sao episodio mas cujo nome ainda entrega um
// numero a extractEpisodeNumber (NCOP 01, PV 02...). Nao existe no codigo de producao — e
// justamente a medicao de quanto elas sujam a faixa.
func isExtrasFolder(path string) bool {
	low := strings.ToLower(path)
	for _, marker := range []string{"extra", "creditless", "/nc", "special", "/pv", "bonus", "menu"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

type packRow struct {
	name      string
	detailURL string
	size      int64
	seeders   int
}

// listPacks devolve as primeiras linhas de PACK de uma busca, na ordem de seeders — mesmo
// protocolo da medicao da #84.
func listPacks(t *testing.T, query string, limit int) []packRow {
	t.Helper()
	url := fmt.Sprintf("%s/?f=0&c=1_2&q=%s&s=seeders&o=desc", getNyaaBaseURL(), strings.ReplaceAll(query, " ", "+"))
	doc, err := fetchNyaaPage(url)
	if err != nil {
		t.Fatalf("busca %q: %v", query, err)
	}
	var rows []packRow
	doc.Find(".torrent-list tbody tr").EachWithBreak(func(_ int, tr *goquery.Selection) bool {
		cols := tr.Find("td")
		link := cols.Eq(1).Find("a").Last()
		name := strings.TrimSpace(link.AttrOr("title", link.Text()))
		if name == "" || !isBatch(name) || shouldIgnoreTorrent(name) {
			return true
		}
		rows = append(rows, packRow{
			name:      name,
			detailURL: detailURL(link.AttrOr("href", "")),
			size:      parseSize(strings.TrimSpace(cols.Eq(3).Text())),
			seeders:   parseSeeders(strings.TrimSpace(cols.Eq(5).Text())),
		})
		return len(rows) < limit
	})
	return rows
}

type verdict struct {
	restarts   bool
	unresolved bool
	extras     bool
	bytesPerEp int64
}

func measurePack(t *testing.T, p packRow) (v verdict) {
	t.Helper()
	defer time.Sleep(1500 * time.Millisecond) // educacao com o nyaa.si

	doc, err := fetchNyaaPage(p.detailURL)
	if err != nil {
		t.Logf("  ! detalhe indisponivel: %v", err)
		return verdict{unresolved: true}
	}

	var folders []folderStats
	walkFileList(doc.Find(".torrent-file-list > ul"), "", &folders)

	// Equivalente a PackFileRange sobre o mesmo documento (mesmos filtros), sem gastar uma
	// segunda requisicao na mesma pagina.
	fileInfo, ok := BatchInfo{}, false
	total := 0
	seen := map[int]int{}
	for _, f := range folders {
		lo, hi := f.span()
		if !ok || lo < fileInfo.StartEpisode {
			fileInfo.StartEpisode = lo
		}
		if !ok || hi > fileInfo.EndEpisode {
			fileInfo.EndEpisode = hi
		}
		ok = true
		total += len(f.episodes)
		for _, e := range f.episodes {
			seen[e]++
		}
	}
	dupes := 0
	for _, n := range seen {
		if n > 1 {
			dupes++
		}
	}

	nameInfo := extractBatchInfo(p.name)
	t.Logf("%s", p.name)
	t.Logf("  %s | S:%d | %s", p.detailURL, p.seeders, formatSize(p.size))
	t.Logf("  faixa do NOME:     %d..%d", nameInfo.StartEpisode, nameInfo.EndEpisode)
	if ok {
		t.Logf("  PackFileRange:     %d..%d  (span %d)", fileInfo.StartEpisode, fileInfo.EndEpisode,
			fileInfo.EndEpisode-fileInfo.StartEpisode+1)
	} else {
		t.Logf("  PackFileRange:     NAO RESOLVEU")
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].path < folders[j].path })
	for _, f := range folders {
		lo, hi := f.span()
		t.Logf("    pasta %-55q %3d arqs  eps %d..%d  seasons-no-nome %v  %s",
			f.path, len(f.episodes), lo, hi, f.seasons, formatSize(f.bytes))
	}
	t.Logf("  arquivos de video com numero: %d | numeros repetidos entre pastas: %d", total, dupes)

	v.restarts, v.unresolved = dupes > 0, !ok
	for _, f := range folders {
		if isExtrasFolder(f.path) {
			v.extras = true
			t.Logf("  >>> PASTA DE EXTRAS CONTADA COMO EPISODIO: %q (%d arqs)", f.path, len(f.episodes))
		}
	}
	if total > 0 && p.size > 0 {
		v.bytesPerEp = p.size / int64(total)
		t.Logf("  TAMANHO POR EPISODIO: %s  (pack inteiro %s)", formatSize(v.bytesPerEp), formatSize(p.size))
	}
	if dupes > 0 {
		t.Logf("  >>> NUMERACAO REINICIA: PackFileRange cobre %d numeros, o pack tem %d arquivos",
			fileInfo.EndEpisode-fileInfo.StartEpisode+1, total)
	}
	return v
}

// tally acumula o resumo de uma amostra.
type tally struct {
	packs, restarts, unresolved, extras int
	perEp                               []int64
}

func (s *tally) add(v verdict) {
	s.packs++
	if v.restarts {
		s.restarts++
	}
	if v.unresolved {
		s.unresolved++
	}
	if v.extras {
		s.extras++
	}
	if v.bytesPerEp > 0 {
		s.perEp = append(s.perEp, v.bytesPerEp)
	}
}

func (s *tally) report(t *testing.T) {
	t.Helper()
	sort.Slice(s.perEp, func(i, j int) bool { return s.perEp[i] < s.perEp[j] })
	t.Logf("RESUMO: %d packs | %d com numeracao que reinicia | %d sem faixa legivel | %d com pasta de extras contada",
		s.packs, s.restarts, s.unresolved, s.extras)
	if n := len(s.perEp); n > 0 {
		t.Logf("RESUMO tamanho/episodio: min %s | mediana %s | max %s",
			formatSize(s.perEp[0]), formatSize(s.perEp[n/2]), formatSize(s.perEp[n-1]))
	}
}

func runSample(t *testing.T, queries []string, perQuery int) {
	t.Helper()
	var sum tally
	for _, q := range queries {
		t.Logf("=== %s ===", q)
		for _, p := range listPacks(t, q, perQuery) {
			sum.add(measurePack(t, p))
		}
		time.Sleep(1500 * time.Millisecond)
	}
	sum.report(t)
}

// TestLive_PackNumberingRestart mede quao comum e o pack cuja numeracao reinicia por season, na
// amostra NEUTRA: `?q=<anime>+batch&s=seeders`, mesmas 8 primeiras linhas de pack da #84.
func TestLive_PackNumberingRestart(t *testing.T) {
	if os.Getenv("AAD_LIVE_NYAA") != "1" {
		t.Skip("defina AAD_LIVE_NYAA=1 para rodar")
	}
	queries := strings.Split(os.Getenv("AAD_LIVE_QUERIES"), "|")
	if len(queries) == 1 && queries[0] == "" {
		queries = []string{
			"shingeki no kyojin batch",
			"mushoku tensei batch",
			"vinland saga batch",
			"jujutsu kaisen batch",
			"one piece batch",
			"steins gate batch",
		}
	}
	runSample(t, queries, 8)
}

// TestLive_MultiSeasonPacks mede o caso ALVO: buscas que miram pack de varias seasons, onde o
// teto da #84 deveria doer.
func TestLive_MultiSeasonPacks(t *testing.T) {
	if os.Getenv("AAD_LIVE_NYAA") != "1" {
		t.Skip("defina AAD_LIVE_NYAA=1 para rodar")
	}
	runSample(t, []string{
		"shingeki no kyojin season 1 - 3",
		"mushoku tensei complete",
		"konosuba complete",
		"overlord season 1-4",
		"my hero academia complete",
		"haikyuu complete",
	}, 4)
}
