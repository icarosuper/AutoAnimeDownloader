package anilist

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// budgetTestServer instala um transporte que responde 200 com o X-RateLimit-Remaining pedido e
// conta quantas requisicoes chegaram a sair. E a unica forma de verificar o gate: uma chamada
// recusada nao produz efeito nenhum alem de nao existir.
func budgetTestServer(t *testing.T, remaining string) *int {
	t.Helper()
	calls := 0
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		calls++
		return respond(200, `{"data":{}}`, map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	t.Cleanup(restore)
	return &calls
}

// TestBudgetGateRefusesDisposableWhenLow: com o balde no fim, o trafego descartavel para de sair.
// E o efeito inteiro da feature — o que sobra do orcamento fica para o passe do daemon.
func TestBudgetGateRefusesDisposableWhenLow(t *testing.T) {
	calls := budgetTestServer(t, "3")

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); err != nil {
		t.Fatalf("a primeira chamada nao pode ser recusada: nao ha leitura de orcamento ainda: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("esperava 1 requisicao emitida, veio %d", *calls)
	}

	_, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable)
	if !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("esperava ErrBudgetLow, veio %v", err)
	}
	if *calls != 1 {
		t.Fatalf("a chamada recusada nao pode ter ido para a rede: %d requisicoes", *calls)
	}
}

// TestBudgetGateAlwaysAllowsCritical: o passe do daemon nunca e recusado, por mais baixo que o
// orcamento esteja. Recusar aqui e episodio nao baixado.
func TestBudgetGateAlwaysAllowsCritical(t *testing.T) {
	calls := budgetTestServer(t, "0")

	for i := range 3 {
		if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
			t.Fatalf("chamada critica %d foi recusada: %v", i, err)
		}
	}
	if *calls != 3 {
		t.Fatalf("esperava 3 requisicoes emitidas, veio %d", *calls)
	}
}

// TestBudgetGateAllowsAgainAfterValidity e a SAIDA do gate: o balde da AniList reseta inteiro em
// <= 60s e nada nos avisa. Sem a validade da leitura, todo mundo seria recusado, nenhuma
// requisicao sairia, nenhuma leitura nova chegaria e o gate nunca mais abriria.
func TestBudgetGateAllowsAgainAfterValidity(t *testing.T) {
	calls := budgetTestServer(t, "1")

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
		t.Fatalf("semeando a leitura de orcamento: %v", err)
	}
	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("esperava ErrBudgetLow com o balde no fim, veio %v", err)
	}

	// Envelhece a leitura em vez de esperar 60s de relogio.
	budget.Store(&budgetReading{remaining: 1, at: time.Now().Add(-budgetValidity - time.Second)})

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); err != nil {
		t.Fatalf("leitura vencida tem que liberar o gate, veio %v", err)
	}
	if *calls != 2 {
		t.Fatalf("esperava 2 requisicoes emitidas, veio %d", *calls)
	}
}

// TestBudgetRecordedOnErrorResponse: um 404 e uma query invalida consomem cota igual a um 200
// (decisions.md #72). Um gate que so lesse resposta boa ficaria otimista exatamente quando as
// coisas ja estao indo mal.
func TestBudgetRecordedOnErrorResponse(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(404, "", map[string]string{"X-RateLimit-Remaining": "2"}), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("o header do 404 tinha que ter alimentado o gate, veio %v", err)
	}
}

// TestCustomListsServesStaleCacheWhenRefused e o par do teste abaixo, e ele existe porque a
// assimetria entre os dois quebrava a blacklist: a lista sai do cache vencido e responde com
// sucesso, mas o customLists recusado voltava nil e o merge de GET /animes concluia que nenhum
// anime esta em lista excluida.
func TestCustomListsServesStaleCacheWhenRefused(t *testing.T) {
	remaining := "29"
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"mediaList":[{"id":7,"customLists":{"Blacklist":true}}]}}}`,
			map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	defer restore()

	statuses := []string{"CURRENT"}
	if m := GetCustomListsMap("user", statuses, PriorityDisposable); len(m) != 1 {
		t.Fatalf("primeira busca deveria popular o cache, veio %+v", m)
	}

	// Vence o cache e afunda o orcamento: a proxima chamada e recusada pelo gate.
	customListsCache.set("user\x00CURRENT", map[int]CustomLists{7: {"Blacklist": true}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	m := GetCustomListsMap("user", statuses, PriorityDisposable)
	if !m[7]["Blacklist"] {
		t.Fatalf("esperava a entrada vencida com a blacklist, veio %+v", m)
	}
}

// TestFrontendListServesStaleCacheWhenRefused trava o fallback da tabela da decisions.md #72: o
// poll do frontend recusado serve a leitura vencida em vez de derrubar a tela.
func TestFrontendListServesStaleCacheWhenRefused(t *testing.T) {
	remaining := "29"
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"mediaList":[{"id":7}]}}}`,
			map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	defer restore()

	statuses := []string{"CURRENT"}
	if _, err := GetFrontendAnimeList("user", statuses); err != nil {
		t.Fatalf("primeira busca: %v", err)
	}

	// Vence o cache e afunda o orcamento: a proxima chamada e recusada pelo gate.
	frontendListCache.set("user\x00CURRENT", []MediaList{{Id: 7}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	resp, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("esperava o cache vencido, veio erro: %v", err)
	}
	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Id != 7 {
		t.Fatalf("esperava a entrada vencida, veio %+v", resp.Data.Page.MediaList)
	}
}
