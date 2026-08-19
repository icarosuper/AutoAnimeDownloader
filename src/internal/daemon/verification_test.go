package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func TestSearchAnilist_FiltersByMediaStatus(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}},
		{"id": 2, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 200, "format": "TV", "status": "NOT_YET_RELEASED", "episodes": 12,
			"title": {"english": "Unreleased Anime", "romaji": "Unreleased Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
	}

	resp, err := searchAnilist(&mockFileManagerForEpisodes{}, config, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("expected 1 anime after media-status filter, got %d", len(resp.Data.Page.MediaList))
	}
	if resp.Data.Page.MediaList[0].Media.Id != 100 {
		t.Errorf("expected surviving anime to be media id 100 (RELEASING), got %d", resp.Data.Page.MediaList[0].Media.Id)
	}
}

func TestSearchAnilist_EmptyMediaStatusesAllowsNothing(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{},
	}

	resp, err := searchAnilist(&mockFileManagerForEpisodes{}, config, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Fatalf("expected 0 animes with empty DownloadMediaStatuses, got %d", len(resp.Data.Page.MediaList))
	}
}

// --- Regra de deleção por status entre contas (decisions.md #43) ---
//
// Download é OR (basta uma conta querer) e deleção é AND (todas as contas que têm o anime
// precisam querer). O que estes testes protegem é o lado AND, onde errar apaga arquivo.

// mockStatusByAccount responde a consulta de desempate GetMediaListStatus por usuário.
// Um usuário ausente do mapa devolve lista vazia = "esta conta não acompanha o anime".
func mockStatusByAccount(t *testing.T, byUser map[string]string) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Variables struct {
				UserName string `json:"userName"`
			} `json:"variables"`
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("corpo inesperado: %s", body)
		}
		status, tracked := byUser[payload.Variables.UserName]
		if !tracked {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
			fmt.Sprintf(`{"data":{"Page":{"mediaList":[{"status":%q}]}}}`, status)))}, nil
	})
}

func deleteRuleConfig() *files.Config {
	return &files.Config{
		AnilistUsernames: []string{"accountA", "accountB"},
		DeleteStatuses:   []string{"DROPPED", "COMPLETED"},
	}
}

func TestDeletableMediaIDs_AllAccountsAgree(t *testing.T) {
	// A busca por lista já trouxe o anime como deletável nas DUAS contas: ninguém precisa
	// ser consultado, e o anime é apagado.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("nao deveria consultar a AniList quando as duas listas ja concordam")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {500: true},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("as duas contas querem apagar, o anime deve ser deletavel")
	}
}

func TestDeletableMediaIDs_DifferentDeleteStatusesStillDeletes(t *testing.T) {
	// DROPPED numa conta e COMPLETED na outra: statuses diferentes, ambos de deleção.
	defer mockStatusByAccount(t, map[string]string{"accountB": "COMPLETED"})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true}, // DROPPED, veio na lista
		"accountB": {},          // COMPLETED chega pelo desempate
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("statuses de delecao diferentes nas duas contas ainda devem apagar")
	}
}

func TestDeletableMediaIDs_NeutralStatusInOtherAccountVetoes(t *testing.T) {
	// A conta B ainda pretende assistir: apagar aqui destruiria episódios que ela quer.
	defer mockStatusByAccount(t, map[string]string{"accountB": "PLANNING"})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if got[500] {
		t.Error("uma conta com o anime em status neutro deve vetar a delecao")
	}
}

func TestDeletableMediaIDs_AccountWithoutTheAnimeDoesNotVeto(t *testing.T) {
	// A conta B nunca teve o anime na lista: não participa da votação. Sem isso, o caso
	// comum (anime acompanhado por uma conta só) nunca mais seria apagado.
	defer mockStatusByAccount(t, map[string]string{})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("conta que nao acompanha o anime nao pode vetar a delecao")
	}
}

func TestDeletableMediaIDs_FailedAccountFetchVetoes(t *testing.T) {
	// A busca da conta B falhou (ela nem aparece no mapa): sem a opinião dela não há
	// unanimidade, e o passe seguinte tenta de novo. Apagar no escuro é irreversível.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("uma conta ausente do mapa e falha de busca, nao deve virar consulta")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if got[500] {
		t.Error("com a lista de uma conta indisponivel nada pode ser apagado")
	}
}

func TestDeletableMediaIDs_OnlyConsidersAnimesOnDisk(t *testing.T) {
	// Sem episódio em disco a resposta não muda nada — e é isso que mantém as consultas de
	// desempate raras em vez de uma por anime completado da lista inteira.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("anime sem episodio em disco nao deveria gerar consulta")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, nil)

	if len(got) != 0 {
		t.Errorf("esperava nenhum candidato sem episodios em disco, veio %v", got)
	}
}

// reportPassAniList mocks AniList with one releasing anime whose episode 1 already aired, so a
// full pass has exactly one episode to look for.
func reportPassAniList(t *testing.T) {
	t.Helper()
	const body = `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 900, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Report Anime", "romaji": "Report Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 1}]}
		}}
	]}}}`
	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	t.Cleanup(restore)
}

// emptyNyaa makes every Nyaa page come back with no rows, so the pass finds no torrent.
func emptyNyaa(t *testing.T) {
	t.Helper()
	restore := nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(restore)
}

func reportPassConfig(t *testing.T) *files.Config {
	t.Helper()
	base := t.TempDir()
	return &files.Config{
		AnilistUsernames:      []string{"tester"},
		SavePath:              base + "/downloads",
		CompletedAnimePath:    base + "/library",
		CheckInterval:         10,
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     1,
		MinSeeders:            1,
	}
}

// TestAnimeVerification_PublishesReport: o relatorio e publicado DEPOIS do SetLastCheckError(nil)
// do fim do passe. Se a ordem inverter, SetLastCheckError limpa o que acabou de ser publicado e
// o relatorio nunca aparece — que e o unico jeito de esta feature falhar silenciosamente.
func TestAnimeVerification_PublishesReport(t *testing.T) {
	reportPassAniList(t)
	emptyNyaa(t)

	fm := &lifecycleFM{configs: reportPassConfig(t)}
	state := NewState()

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	report := state.GetLastCheckReport()
	if report.FinishedAt.IsZero() {
		t.Fatal("o passe completo deve publicar um relatório com finished_at preenchido")
	}
	if len(report.Problems) == 0 {
		t.Fatalf("esperava ao menos um problema, obteve %+v", report)
	}
	if report.Problems[0].Code != IssueNoTorrentFound {
		t.Errorf("esperava %q, obteve %q", IssueNoTorrentFound, report.Problems[0].Code)
	}
}

// TestAnimeVerification_CancelledLeavesNoReport: passe interrompido nao deixa relatorio — ele
// estava incompleto, e um relatorio parcial diria "so este anime teve problema" quando os outros
// nem chegaram a ser olhados.
func TestAnimeVerification_CancelledLeavesNoReport(t *testing.T) {
	reportPassAniList(t)
	emptyNyaa(t)

	fm := &lifecycleFM{configs: reportPassConfig(t)}
	state := NewState()
	// Semear o relatorio do passe ANTERIOR: sem isso o teste passaria com o State recem-criado
	// mesmo se a limpeza inteira fosse apagada, porque um State novo ja nasce com o relatorio
	// zerado. O que precisa ser travado e que o passe interrompido NAO deixa o relatorio velho
	// na tela.
	state.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   []Issue{{AnimeID: 1, AnimeName: "Anterior", Code: IssueNoTorrentFound}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	AnimeVerification(ctx, fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	report := state.GetLastCheckReport()
	if !report.FinishedAt.IsZero() || len(report.Problems) != 0 || len(report.Limits) != 0 {
		t.Errorf("passe cancelado não deve deixar relatório, obteve %+v", report)
	}
}
