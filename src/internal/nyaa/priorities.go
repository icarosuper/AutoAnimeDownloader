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
		// health vem antes de fansub: era o 7º de 8 num sort lexicográfico e nunca era
		// alcançado (resolução/fansub decidiam antes), então um torrent com 12 seeders
		// ganhava de um com 400 por ser de um fansub melhor colocado.
		CriteriaOrder: []string{"uncensored", "source", "resolution", "health", "codec", "fansub", "audio", "size"},
		Fansubs: []string{
			"subsplease", "erai-raws", "judas", "toonshub", "asw",
			"ember", "hd-zone", "kamig", "remix", "aniverse", "dub", "raw",
		},
		// Só os tokens canônicos que ExtractResolution consegue devolver: "4k"/"uhd" viram
		// "2160p", "fhd" vira "1080p", "hd" vira "720p". 4K abaixo de 720p é deliberado —
		// anime em 4K é quase sempre upscale de um master 1080p.
		Resolutions: []string{"1080p", "720p", "1440p", "2160p", "480p"},
		// BDRemux é o bitstream intocado do disco; BDRip é reencode.
		Sources: []string{"bdremux", "bd", "bdrip", "web-dl", "webrip", "tv", "dvd", "hdtv"},
		// H.264 primeiro: toca direto em qualquer player, sem transcode no servidor (que
		// queima a legenda no vídeo e a dessincroniza). Quem prefere arquivo menor troca
		// pelo preset na tela de prioridades. "x265"/"x264" não entram porque ExtractCodec
		// já canonicaliza para "hevc"/"h.264" — seriam tokens inertes.
		Codecs:     []string{"h.264", "hevc", "av1", "xvid"},
		Audio:      []string{"flac", "dts-hd", "truehd", "ddp", "aac", "ac3", "mp3"},
		IgnoreList: []string{"[dub]", "[raw]", "[hardcoded]", "[hc]", "re-encode"},
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
	"source": func(a, b TorrentResult) int {
		return sourcePriority(extractSource(a.Name)) - sourcePriority(extractSource(b.Name))
	},
	"codec": codecCompare,
	"audio": func(a, b TorrentResult) int {
		return audioPriority(extractAudio(a.Name)) - audioPriority(extractAudio(b.Name))
	},
	"health": func(a, b TorrentResult) int { return healthTier(b) - healthTier(a) }, // faixa maior é melhor
	"size":   func(a, b TorrentResult) int { return sizeCompare(a, b) },
}

// healthTierFloors são os pisos das faixas de saúde: seeders 0 / 1-4 / 5-19 / 20-99 /
// 100-399 / 400+ ⇒ faixa 0..5. Cada faixa é ~4-5x a anterior.
var healthTierFloors = []int{1, 5, 20, 100, 400}

// healthTier devolve a faixa de saúde do torrent (maior = mais saudável).
//
// O critério compara FAIXA, não o score cru, porque o sort é lexicográfico: com o score cru
// qualquer diferença de 1 seeder já decidia e nenhum critério depois de health era consultado.
// Comparar faixa faz 400 seeders vencer 12 e faz 150 vs 300 empatar, deixando o critério
// seguinte (fansub) decidir — que é o comportamento pretendido.
//
// Faixa fixa em vez de razão (ex: "empata a menos que um tenha 2x o outro") de propósito:
// razão não é transitiva (100 ~ 150 ~ 220, mas 100 < 220) e sort.SliceStable com comparador
// intransitivo devolve ordem arbitrária.
func healthTier(r TorrentResult) int {
	seeders := ParseSeeders(r.Seeders)
	tier := 0
	for _, floor := range healthTierFloors {
		if seeders >= floor {
			tier++
		}
	}
	return tier
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

// codecCompare desempata por codec, mas só quando os DOIS nomes trazem uma tag de codec.
// Nome sem tag ("[SubsPlease] Anime - 07 (1080p) [HASH].mkv" não diz H.264 em lugar nenhum)
// devolve empate em vez de ir para o fim: priorityIndex trata token desconhecido como o pior,
// e sem essa guarda todo release sem tag perderia para qualquer x265 tagueado — justamente o
// contrário do que a lista de codecs configura. Mesma regra do tamanho ilegível em
// filterBySize: dado que não deu para ler não é motivo para punir o release.
func codecCompare(a, b TorrentResult) int {
	ca, cb := ExtractCodec(a.Name), ExtractCodec(b.Name)
	if ca == "" || cb == "" {
		return 0
	}
	return codecPriority(ca) - codecPriority(cb)
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

var episodeCriteria = map[string]bool{"uncensored": true, "resolution": true, "codec": true, "fansub": true, "health": true, "size": true}

func filterCriteria(order []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(order))
	for _, c := range order {
		if allowed[c] {
			out = append(out, c)
		}
	}
	return out
}
