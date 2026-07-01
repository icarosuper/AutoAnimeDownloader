package nyaa

import (
	"sort"
	"strings"
	"sync/atomic"
)

// Priorities define as listas ordenadas que governam o ranking e o filtro
// de torrents. Índice na lista = prioridade (menor = melhor). Item ausente
// da lista é tratado como pior (vai para o fim).
type Priorities struct {
	// CriteriaOrder é a ordem em que os critérios de sort são aplicados.
	// Valores válidos: uncensored, source, resolution, codec, fansub, audio, health, size.
	CriteriaOrder []string `json:"criteria_order"`
	Fansubs       []string `json:"fansubs"`
	Resolutions   []string `json:"resolutions"`
	Sources       []string `json:"sources"`
	Codecs        []string `json:"codecs"`
	Audio         []string `json:"audio"`
	// IgnoreList: substrings (case-insensitive) que descartam o release de vez.
	IgnoreList []string `json:"ignore_list"`
}

// DefaultPriorities reproduz fielmente os maps/regex hardcoded originais.
// Tudo em minúsculas (o lookup compara contra tokens minúsculos).
func DefaultPriorities() Priorities {
	return Priorities{
		CriteriaOrder: []string{"uncensored", "source", "resolution", "codec", "fansub", "audio", "health", "size"},
		Fansubs: []string{
			"subsplease", "erai-raws", "judas", "toonshub", "asw",
			"ember", "hd-zone", "kamig", "remix", "aniverse", "dub", "raw",
		},
		Resolutions: []string{"1080p", "720p", "480p", "4k", "8k", "fhd", "uhd", "hd", "2160p", "1440p"},
		Sources:     []string{"bd", "bdrip", "bdremux", "web-dl", "webrip", "tv", "dvd", "hdtv"},
		Codecs:      []string{"hevc", "av1", "x265", "h.264", "x264", "xvid"},
		Audio:       []string{"flac", "dts-hd", "truehd", "ddp", "aac", "ac3", "mp3"},
		IgnoreList:  []string{"[dub]", "[raw]", "[hardcoded]", "[hc]", "re-encode"},
	}
}

// active guarda as prioridades em uso. Inicializado com os defaults no package
// init para que as funções de sort funcionem mesmo sem SetPriorities (ex: testes
// que chamam SortTorrentResults direto).
var active atomic.Pointer[Priorities]

func init() {
	d := DefaultPriorities()
	active.Store(&d)
}

// ActivePriorities retorna as prioridades em uso (nunca nil).
func ActivePriorities() Priorities { return *active.Load() }

// SetPriorities aplica novas prioridades e retorna uma função que restaura o
// valor anterior (padrão de MockNyaaHttpGet — evita vazar estado global em testes).
func SetPriorities(p Priorities) (restore func()) {
	prev := active.Load()
	active.Store(&p)
	return func() { active.Store(prev) }
}

// priorityIndex retorna o índice de token (minúsculo) em list, ou len(list)
// (pior) se ausente. token é comparado em minúsculas.
func priorityIndex(list []string, token string) int {
	t := strings.ToLower(token)
	for i, v := range list {
		if v == t {
			return i
		}
	}
	return len(list)
}

// ShouldIgnore reporta se o release deve ser descartado por casar com algum
// item (substring, case-insensitive) da IgnoreList ativa.
func ShouldIgnore(torrentName string) bool {
	nameLower := strings.ToLower(torrentName)
	for _, pat := range ActivePriorities().IgnoreList {
		if pat != "" && strings.Contains(nameLower, strings.ToLower(pat)) {
			return true
		}
	}
	return false
}

// criterionCompare mapeia nome do critério → comparador (a melhor que b ⇒ <0).
var criterionCompare = map[string]func(a, b TorrentResult) int{
	"uncensored": func(a, b TorrentResult) int { return boolBetter(isUncensored(a.Name), isUncensored(b.Name)) },
	"resolution": func(a, b TorrentResult) int { return resCompare(a, b) },
	"fansub":     func(a, b TorrentResult) int { return fansubPriority(a.Name) - fansubPriority(b.Name) },
	"source":     func(a, b TorrentResult) int { return sourcePriority(extractSource(a.Name)) - sourcePriority(extractSource(b.Name)) },
	"codec":      func(a, b TorrentResult) int { return codecPriority(extractCodec(a.Name)) - codecPriority(extractCodec(b.Name)) },
	"audio":      func(a, b TorrentResult) int { return audioPriority(extractAudio(a.Name)) - audioPriority(extractAudio(b.Name)) },
	"health":     func(a, b TorrentResult) int { return -cmpFloat(torrentHealthScore(a), torrentHealthScore(b)) }, // maior é melhor
	"size":       func(a, b TorrentResult) int { return sizeCompare(a, b) },
}

func boolBetter(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return -1
	}
	return 1
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// resCompare reproduz a lógica atual: ambos com resolução → por índice;
// quem tem resolução vence quem é nil; ambos nil → empate.
func resCompare(a, b TorrentResult) int {
	switch {
	case a.Resolution != nil && b.Resolution != nil:
		return resolutionPriority(*a.Resolution) - resolutionPriority(*b.Resolution)
	case a.Resolution != nil:
		return -1
	case b.Resolution != nil:
		return 1
	default:
		return 0
	}
}

// sizeCompare reproduz a regra atual: menor é melhor, mas só desempata quando
// as resoluções são iguais (ambas não-nil e iguais); caso contrário empate.
func sizeCompare(a, b TorrentResult) int {
	if a.Resolution != nil && b.Resolution != nil && *a.Resolution == *b.Resolution {
		switch {
		case a.Size < b.Size:
			return -1
		case a.Size > b.Size:
			return 1
		}
	}
	return 0
}

// sortByCriteria ordena results aplicando os critérios de criteria na ordem dada,
// pulando os que não estiverem no registry.
func sortByCriteria(results []TorrentResult, criteria []string) []TorrentResult {
	sorted := make([]TorrentResult, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		for _, c := range criteria {
			cmp, ok := criterionCompare[c]
			if !ok {
				continue
			}
			if d := cmp(sorted[i], sorted[j]); d != 0 {
				return d < 0
			}
		}
		return false
	})
	return sorted
}

var episodeCriteria = map[string]bool{"uncensored": true, "resolution": true, "fansub": true, "health": true, "size": true}

func filterCriteria(order []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(order))
	for _, c := range order {
		if allowed[c] {
			out = append(out, c)
		}
	}
	return out
}
