package anilist

import (
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"
)

// Estados de saude da AniList. Sao os codigos que chegam ao frontend, que monta a frase — a
// mesma fronteira que lib/domain/checkIssue.ts documenta: o backend manda CODIGO, nunca texto
// pronto, senao o daemon precisaria saber o locale do navegador.
const (
	HealthOK          = "ok"
	HealthRateLimited = "rate_limited" // 429: timeout de 1 minuto, com Retry-After
	HealthOutage      = "outage"       // 403 (API desligada / IP bloqueado) ou 5xx
	HealthAppBug      = "app_bug"      // 400: a query nao bate mais com o schema
)

// Health e o ultimo estado conhecido da AniList. Vive no pacote (e nao em daemon.State) porque
// TODA chamada passa por sendAnilistRequest: o passe do daemon e o poll de /animes gravam aqui
// sem que nenhum chamador precise propagar erro. Ver decisions.md #65.
type Health struct {
	State string `json:"state" example:"ok"`
	// Message e a mensagem CRUA da AniList. Um 403 de IP bloqueado traz o motivo escrito para
	// ser lido por humano; e a unica informacao que o frontend nao tem como reconstruir.
	Message string `json:"message,omitempty"`
	// RetryAt vem do Retry-After de um 429. Zero quando nao se aplica: so o rate limit informa
	// quanto falta, e e o que permite mostrar contagem regressiva em vez de "tente mais tarde".
	RetryAt time.Time `json:"retry_at,omitempty"`
	Since   time.Time `json:"since,omitempty"`
}

var health atomic.Pointer[Health]

// CurrentHealth devolve o estado atual. Nunca nil: antes da primeira chamada o estado e "ok",
// porque "ainda nao perguntamos" nao e motivo para alarmar ninguem.
func CurrentHealth() Health {
	if h := health.Load(); h != nil {
		return *h
	}
	return Health{State: HealthOK}
}

// setHealth grava um estado degradado. Preserva o Since do estado anterior quando o tipo de
// falha nao mudou: o banner mostra ha quanto tempo a AniList esta fora, nao ha quanto tempo foi
// a ultima tentativa.
func setHealth(state, message string, retryAt time.Time) {
	now := time.Now()
	since := now
	if prev := health.Load(); prev != nil && prev.State == state && !prev.Since.IsZero() {
		since = prev.Since
	}
	health.Store(&Health{State: state, Message: message, RetryAt: retryAt, Since: since})
}

// clearHealth marca a AniList como sa. Chamado em toda resposta util — o banner some no primeiro
// 200 limpo, nunca por timer: o estado ou acabou ou nao acabou (ver decisions.md #66).
func clearHealth() {
	if h := health.Load(); h == nil || h.State != HealthOK {
		health.Store(&Health{State: HealthOK})
	}
}

// graphQLErrors e o envelope de erro da AniList. A doc oficial e explicita: um 200 pode carregar
// erro aqui (query invalida, campo suprimido). Sem ler este campo, esse caso vira resposta vazia
// sem diagnostico — ver decisions.md #65.
type graphQLErrors struct {
	Errors []struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"errors"`
}

// firstError extrai a primeira entrada de errors do corpo. Devolve status 0 quando nao ha erro
// nenhum, o que inclui o caso comum de o corpo nem ser um envelope GraphQL valido.
func firstError(body []byte) (status int, message string) {
	var env graphQLErrors
	if err := json.Unmarshal(body, &env); err != nil || len(env.Errors) == 0 {
		return 0, ""
	}
	e := env.Errors[0]
	return e.Status, e.Message
}

// classify traduz um codigo em estado de saude. O codigo vem de errors[].status quando existe e
// do HTTP quando nao — os dois caminhos convergem aqui de proposito, porque a AniList usa a
// mesma numeracao nos dois lugares.
func classify(status int) string {
	switch {
	case status == 429:
		return HealthRateLimited
	case status == 403 || status >= 500:
		return HealthOutage
	case status == 400:
		return HealthAppBug
	default:
		// 401/405/418… nao estao documentados para esta API. Tratar como outage e a leitura
		// segura: manda esperar em vez de mandar reportar um bug que pode nao ser nosso.
		return HealthOutage
	}
}

// retryAfter le o cabecalho Retry-After de um 429. A AniList manda segundos (nunca data HTTP).
//
// NAO confiar em X-RateLimit-Reset: ele aparece anunciado em Access-Control-Expose-Headers, mas
// nao veio em nenhuma resposta 200 observada (medido em 2026-08-28). Se ele existe, e so no 429 —
// nao verificado, porque provocar um 429 bloqueia o IP por ~1min e o daemon compartilha esse IP.
// X-RateLimit-Limit e X-RateLimit-Remaining, esses sim, vem em TODA resposta, inclusive 400 e 404.
func retryAfter(headerValue string) time.Time {
	secs, err := strconv.Atoi(headerValue)
	if err != nil || secs <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(secs) * time.Second)
}
