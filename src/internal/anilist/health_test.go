package anilist

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// respond monta uma resposta da AniList com corpo e cabecalhos arbitrarios.
func respond(status int, body string, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: h}
}

// TestHealthClassification trava o mapeamento codigo -> estado. E o que decide qual banner o
// usuario ve: confundir outage com bug nosso manda reportar uma issue que nao existe, e
// confundir bug nosso com outage manda esperar por uma correcao que nunca vem.
func TestHealthClassification(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		want    string
		wantMsg string
	}{
		{
			name:    "403 com API desligada vira outage e guarda a mensagem deles",
			status:  403,
			body:    `{"errors":[{"message":"The AniList API has been temporarily disabled due to severe stability issues.","status":403}],"data":null}`,
			want:    HealthOutage,
			wantMsg: "The AniList API has been temporarily disabled due to severe stability issues.",
		},
		{
			name:    "429 vira rate limit",
			status:  429,
			body:    `{"errors":[{"message":"Too Many Requests.","status":429}],"data":null}`,
			headers: map[string]string{"Retry-After": "30"},
			want:    HealthRateLimited,
			wantMsg: "Too Many Requests.",
		},
		{
			name:    "400 vira bug do app",
			status:  400,
			body:    `{"errors":[{"message":"Cannot query field \"nope\" on type \"MediaList\".","status":400}],"data":null}`,
			want:    HealthAppBug,
			wantMsg: `Cannot query field "nope" on type "MediaList".`,
		},
		{
			name:   "5xx vira outage",
			status: 502,
			body:   "bad gateway",
			want:   HealthOutage,
		},
		{
			// O caso que passava despercebido: 200 no HTTP, erro no envelope. Ver decisions.md #65.
			name:    "200 com errors no envelope e classificado pelo status de dentro",
			status:  200,
			body:    `{"errors":[{"message":"Too Many Requests.","status":429}],"data":null}`,
			want:    HealthRateLimited,
			wantMsg: "Too Many Requests.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
				return respond(tc.status, tc.body, tc.headers), nil
			})
			defer restore()

			if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err == nil {
				t.Fatal("esperava erro, veio nil")
			}

			got := CurrentHealth()
			if got.State != tc.want {
				t.Errorf("estado = %q, esperado %q", got.State, tc.want)
			}
			if tc.wantMsg != "" && got.Message != tc.wantMsg {
				t.Errorf("mensagem = %q, esperado %q", got.Message, tc.wantMsg)
			}
			if got.Since.IsZero() {
				t.Error("Since deveria estar preenchido num estado degradado")
			}
		})
	}
}

// TestHealthRetryAtFromHeader: o 429 e o unico caso em que se sabe quanto falta, e e disso que
// sai a contagem regressiva do banner. Sem isso ele so consegue dizer "tente mais tarde".
func TestHealthRetryAtFromHeader(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(429, `{"errors":[{"status":429}]}`, map[string]string{"Retry-After": "30"}), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	got := CurrentHealth().RetryAt
	if got.IsZero() {
		t.Fatal("RetryAt vazio: o Retry-After nao foi lido")
	}
	if d := time.Until(got); d < 25*time.Second || d > 31*time.Second {
		t.Errorf("RetryAt em %v, esperado ~30s", d)
	}
}

// TestHealthClearsOnFirstGoodResponse trava a regra do banner: ele some no primeiro 200 limpo,
// nunca por timer (decisions.md #66).
func TestHealthClearsOnFirstGoodResponse(t *testing.T) {
	fail := true
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		if fail {
			return respond(403, `{"errors":[{"message":"disabled","status":403}]}`, nil), nil
		}
		return respond(200, `{"data":{"Page":{"mediaList":[]}}}`, nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)
	if CurrentHealth().State != HealthOutage {
		t.Fatalf("esperava outage, veio %q", CurrentHealth().State)
	}

	fail = false
	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
		t.Fatalf("resposta boa devolveu erro: %v", err)
	}
	if got := CurrentHealth(); got.State != HealthOK {
		t.Errorf("estado = %q, esperado %q", got.State, HealthOK)
	}
}

// TestHealthSincePreservedAcrossRepeatedFailures: o banner mostra ha quanto tempo a AniList esta
// fora, nao ha quanto tempo foi a ultima tentativa. Num poll de 30s os dois sao bem diferentes.
func TestHealthSincePreservedAcrossRepeatedFailures(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(403, `{"errors":[{"message":"disabled","status":403}]}`, nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)
	first := CurrentHealth().Since

	time.Sleep(5 * time.Millisecond)
	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if got := CurrentHealth().Since; !got.Equal(first) {
		t.Errorf("Since virou %v, esperado continuar em %v", got, first)
	}
}

// TestNotFoundIsNotDegradedHealth: um 404 e "esse id nao existe", nao "a AniList caiu". Marcar
// outage aqui poria um banner na tela toda vez que o usuario removesse um anime da lista.
func TestNotFoundIsNotDegradedHealth(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(404, "", nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if got := CurrentHealth().State; got != HealthOK {
		t.Errorf("estado = %q, esperado %q", got, HealthOK)
	}
}
