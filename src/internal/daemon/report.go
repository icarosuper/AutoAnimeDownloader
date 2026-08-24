package daemon

import (
	"AutoAnimeDownloader/src/internal/files"

	"sort"
	"time"
)

// Codigos de PROBLEMA: algo que devia ter baixado e nao baixou.
const (
	IssueAllAboveSizeLimit = "all_above_size_limit"
	IssueNoSeeders         = "no_seeders"
	IssueNoTorrentFound    = "no_torrent_found"
	IssueDiskFull          = "disk_full"
	IssueTorrentRejected   = "torrent_rejected"
)

// Codigo de LIMITE: a config funcionando como configurada. Peso visual diferente na UI porque o
// usuario frequentemente nao relaciona o limite com o sintoma ("por que so baixou 12 de 47?").
const IssueMaxEpisodesPerAnime = "max_episodes_per_anime"

// Valores de Issue.BatchSkipped — o porque de max_episodes_per_anime estar valendo NAQUELE anime.
// Batch desligado nao bloqueia nada por si so: ele so deixa o limite por anime valendo, e o
// sintoma que o usuario sente e sempre o limite. Por isso e campo de detalhe, e nao codigo.
const (
	BatchSkippedNoResult       = "no_result"        // a busca nao devolveu linha de pack nenhuma
	BatchSkippedAboveSizeLimit = "above_size_limit" // havia packs, o filtro cortou todos
	BatchSkippedNoCoverage     = "no_coverage"      // packs sobreviveram, nenhum cobre a janela
)

// Issue e uma linha do relatorio: um par (anime, codigo), com os episodios afetados quando o
// codigo e de problema.
//
// Campos de detalhe achatados com omitempty, e nao um detail map[string]any: um mapa livre nao
// gera Swagger utilizavel nem tipo TS decente, e o preco e uma struct de dez campos com tres
// preenchidos por vez.
type Issue struct {
	AnimeID   int    `json:"anime_id" example:"269"`
	AnimeName string `json:"anime_name" example:"Bleach"`
	// Episodes so e preenchido em PROBLEMA: um problema acontece num episodio (ele foi buscado,
	// os candidatos foram avaliados, aquele episodio nao baixou). O limite e o contrario — o
	// daemon parou de considerar episodios ao atingir a conta, entao nao existe "os episodios
	// afetados", existe uma quantidade que sobrou (Downloaded/Pending).
	Episodes []int  `json:"episodes,omitempty"`
	Code     string `json:"code" example:"all_above_size_limit"`

	Candidates int     `json:"candidates,omitempty" example:"8"`
	LimitGB    float64 `json:"limit_gb,omitempty" example:"3"`
	MinSeeders int     `json:"min_seeders,omitempty" example:"1"`
	// Downloaded e quantos episodios CONSUMIRAM uma vaga sob o teto, nao quantos baixaram neste
	// passe: handleAlreadySavedEpisode tambem incrementa o contador para episodio ja salvo. E o
	// numero certo para a frase "baixou N, sobraram M" — os N ja estao na biblioteca —, mas quem
	// ler isso como "downloads deste passe" vai se enganar.
	Downloaded   int    `json:"downloaded,omitempty" example:"12"`
	Pending      int    `json:"pending,omitempty" example:"35"`
	BatchSkipped string `json:"batch_skipped,omitempty" example:"no_result"`
}

// CheckReport e o relatorio do ULTIMO passe, e so dele. Nao e historico.
type CheckReport struct {
	FinishedAt time.Time `json:"finished_at" example:"2026-08-19T12:00:00Z"`
	PassError  string    `json:"pass_error" example:""`
	// PassErrorCode e a CAUSA do aborto, para o frontend montar a frase. PassError continua
	// carregando o texto cru, que a tela mostra recolhido: sem ele um usuario que quer abrir
	// issue nao tem o que colar. Ver passerror.go.
	PassErrorCode string  `json:"pass_error_code" example:"anilist"`
	Problems      []Issue `json:"problems"`
	Limits        []Issue `json:"limits"`
}

// isLimitCode separa as duas categorias. Hoje ha um unico codigo de limite; a funcao existe para
// que acrescentar o segundo seja uma linha e nao uma cacada por comparacoes espalhadas.
func isLimitCode(code string) bool {
	return code == IssueMaxEpisodesPerAnime
}

// aggregateIssues junta os Issues crus de todos os animes em um por par (anime, codigo) e separa
// em problemas e limites, cada lista ordenada por nome do anime.
//
// ponytail: quando dois episodios do mesmo anime batem no mesmo codigo com detalhes diferentes
// (ep 12 com 8 candidatos, ep 13 com 4), os campos de detalhe do PRIMEIRO vencem e os do segundo
// somem. Somar seria mentira ("12 candidatos" nao existiu em busca nenhuma) e listar os dois
// exigiria um detalhe por episodio, que e o relatorio-por-episodio que a spec descartou. Se um
// dia isso incomodar, o caminho e Detail []struct{Episode int; ...} dentro do Issue.
func aggregateIssues(raw []Issue) (problems, limits []Issue) {
	type key struct {
		animeID int
		code    string
	}
	order := make([]key, 0, len(raw))
	merged := make(map[key]*Issue, len(raw))

	for _, in := range raw {
		k := key{in.AnimeID, in.Code}
		existing, ok := merged[k]
		if !ok {
			cp := in
			cp.Episodes = append([]int(nil), in.Episodes...)
			merged[k] = &cp
			order = append(order, k)
			continue
		}
		existing.Episodes = append(existing.Episodes, in.Episodes...)
	}

	for _, k := range order {
		issue := *merged[k]
		if isLimitCode(issue.Code) {
			limits = append(limits, issue)
			continue
		}
		problems = append(problems, issue)
	}

	sortIssues(problems)
	sortIssues(limits)
	return problems, limits
}

// sortIssues ordena por nome do anime, com o codigo como desempate — estavel e previsivel, sem
// inventar um ranking de severidade que ninguem pediu.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].AnimeName != issues[j].AnimeName {
			return issues[i].AnimeName < issues[j].AnimeName
		}
		return issues[i].Code < issues[j].Code
	})
}

// searchIssue traduz o que a busca por episodio descartou no codigo de problema mais especifico
// que couber.
//
// A ordem e CASCATA, nao conjunto: quando um filtro esvazia a lista, len(magnets) == 0 tambem e
// verdade, e "nenhum torrent encontrado" e a resposta menos acionavel das tres — e mentirosa,
// porque havia oito. A primeira condicao que casa vence, e a ordem e a regra de negocio; mesma
// disciplina da cascata de deriveAnimeChip no frontend.
func searchIssue(animeID int, animeName string, episode int, stats dropStats, configs *files.Config) Issue {
	issue := Issue{AnimeID: animeID, AnimeName: animeName, Episodes: []int{episode}}
	switch {
	case stats.Input > 0 && stats.BySize > 0:
		issue.Code = IssueAllAboveSizeLimit
		issue.Candidates = stats.Input
		issue.LimitGB = configs.MaxEpisodeTorrentSizeGB
	case stats.Input > 0 && stats.BySeeders > 0:
		issue.Code = IssueNoSeeders
		issue.Candidates = stats.Input
		issue.MinSeeders = configs.MinSeeders
	default:
		issue.Code = IssueNoTorrentFound
	}
	return issue
}
