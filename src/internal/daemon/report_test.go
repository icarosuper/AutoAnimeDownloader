package daemon

import (
	"AutoAnimeDownloader/src/internal/files"

	"errors"
	"reflect"
	"testing"
	"time"
)

// TestAggregateIssues: um Issue por par (anime, codigo). Dois episodios do mesmo anime com o
// mesmo codigo viram UMA entrada com dois numeros; codigos diferentes viram duas.
func TestAggregateIssues(t *testing.T) {
	raw := []Issue{
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{12}, Code: IssueAllAboveSizeLimit, Candidates: 8, LimitGB: 3},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{15}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{13}, Code: IssueAllAboveSizeLimit, Candidates: 4, LimitGB: 3},
		{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35, BatchSkipped: BatchSkippedNoResult},
	}

	problems, limits := aggregateIssues(raw)

	if len(limits) != 1 {
		t.Fatalf("esperava 1 limite, obteve %d (%+v)", len(limits), limits)
	}
	if limits[0].Code != IssueMaxEpisodesPerAnime || limits[0].Downloaded != 12 || limits[0].Pending != 35 {
		t.Errorf("limite errado: %+v", limits[0])
	}
	// Limite nao carrega numeros de episodio: o daemon parou de CONSIDERAR episodios ao atingir
	// a conta, entao nao existe "os episodios afetados".
	if len(limits[0].Episodes) != 0 {
		t.Errorf("limite não deve trazer Episodes, obteve %+v", limits[0].Episodes)
	}

	if len(problems) != 2 {
		t.Fatalf("esperava 2 problemas, obteve %d (%+v)", len(problems), problems)
	}
	byCode := map[string]Issue{}
	for _, p := range problems {
		byCode[p.Code] = p
	}
	size, ok := byCode[IssueAllAboveSizeLimit]
	if !ok {
		t.Fatalf("faltou o problema de tamanho em %+v", problems)
	}
	if !reflect.DeepEqual(size.Episodes, []int{12, 13}) {
		t.Errorf("esperava episódios [12 13], obteve %+v", size.Episodes)
	}
	if size.Candidates != 8 || size.LimitGB != 3 {
		t.Errorf("detalhe do primeiro episódio deve vencer, obteve %+v", size)
	}
	if _, ok := byCode[IssueNoTorrentFound]; !ok {
		t.Errorf("faltou o problema de torrent não encontrado em %+v", problems)
	}
}

// TestAggregateIssues_SortedByAnimeName: ordem estavel e previsivel, sem ranking de severidade.
func TestAggregateIssues_SortedByAnimeName(t *testing.T) {
	raw := []Issue{
		{AnimeID: 3, AnimeName: "Zeta", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 1, AnimeName: "Alpha", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Mu", Episodes: []int{1}, Code: IssueNoTorrentFound},
	}
	problems, _ := aggregateIssues(raw)
	want := []string{"Alpha", "Mu", "Zeta"}
	for i, name := range want {
		if problems[i].AnimeName != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, problems[i].AnimeName)
		}
	}
}

func TestAggregateIssues_Empty(t *testing.T) {
	problems, limits := aggregateIssues(nil)
	if problems != nil || limits != nil {
		t.Errorf("passe limpo deve produzir nil/nil, obteve %+v / %+v", problems, limits)
	}
}

// TestSetLastCheckError_ClearsReport: um passe que abortou antes de olhar anime nenhum nao tem
// relatorio por anime — tem pass_error. Sem a limpeza, a tela mostraria os problemas do passe
// ANTERIOR lado a lado com um erro de passe novo.
func TestSetLastCheckError_ClearsReport(t *testing.T) {
	s := NewState()
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueNoTorrentFound}},
	})
	if len(s.GetLastCheckReport().Problems) != 1 {
		t.Fatal("o relatório deveria estar guardado antes do erro")
	}

	s.SetLastCheckError(errors.New("anilist caiu"))

	got := s.GetLastCheckReport()
	if len(got.Problems) != 0 || len(got.Limits) != 0 || !got.FinishedAt.IsZero() {
		t.Errorf("SetLastCheckError deve limpar o relatório, obteve %+v", got)
	}
}

// TestSetLastCheckReport_AfterClearingError: SetLastCheckError(nil) tambem limpa, entao o
// SetLastCheckReport do fim do passe TEM de vir depois dele — e sobreviver.
func TestSetLastCheckReport_AfterClearingError(t *testing.T) {
	s := NewState()
	s.SetLastCheckError(nil)
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Limits:     []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35}},
	})
	if len(s.GetLastCheckReport().Limits) != 1 {
		t.Error("o relatório publicado depois de SetLastCheckError(nil) deve sobreviver")
	}
}

// TestSearchIssue_PrecedenciaEmCascata: os tres primeiros problemas se sobrepoem — quando um
// filtro esvazia a lista, "nenhum torrent encontrado" tambem e verdade. O especifico TEM de
// ganhar do generico: "todos os candidatos tinham 8 GB e seu teto e 3 GB" e acionavel; "nenhum
// torrent encontrado" nao e, e ainda por cima e mentiroso — havia oito.
func TestSearchIssue_PrecedenciaEmCascata(t *testing.T) {
	configs := &files.Config{MaxEpisodeTorrentSizeGB: 3, MinSeeders: 5}

	t.Run("cortado por tamanho vence o genérico", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 8}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Fatalf("esperava %q, obteve %q", IssueAllAboveSizeLimit, got.Code)
		}
		if got.Candidates != 8 || got.LimitGB != 3 {
			t.Errorf("detalhes errados: %+v", got)
		}
		if len(got.Episodes) != 1 || got.Episodes[0] != 12 {
			t.Errorf("esperava episódio [12], obteve %+v", got.Episodes)
		}
	})

	t.Run("tamanho vence seeders quando os dois cortaram", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 5, BySeeders: 3}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Errorf("esperava %q, obteve %q", IssueAllAboveSizeLimit, got.Code)
		}
	})

	t.Run("cortado só por seeders", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 4, BySeeders: 4}, configs)
		if got.Code != IssueNoSeeders {
			t.Fatalf("esperava %q, obteve %q", IssueNoSeeders, got.Code)
		}
		if got.Candidates != 4 || got.MinSeeders != 5 {
			t.Errorf("detalhes errados: %+v", got)
		}
	})

	t.Run("busca realmente vazia", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{}, configs)
		if got.Code != IssueNoTorrentFound {
			t.Errorf("esperava %q, obteve %q", IssueNoTorrentFound, got.Code)
		}
		if got.Candidates != 0 || got.LimitGB != 0 || got.MinSeeders != 0 {
			t.Errorf("no_torrent_found não tem detalhe, obteve %+v", got)
		}
	})
}
