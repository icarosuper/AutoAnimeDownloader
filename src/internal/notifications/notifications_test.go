package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInterpolate(t *testing.T) {
	result := interpolate("Hello {{anime_name}}, ep {{episode}}", map[string]string{
		"anime_name": "Frieren",
		"episode":    "5",
	})
	if result != "Hello Frieren, ep 5" {
		t.Fatalf("got %q", result)
	}
}

func TestInterpolateMissingVarEmpty(t *testing.T) {
	result := interpolate("{{missing}}", map[string]string{})
	if result != "" {
		t.Fatalf("missing var should be empty string, got %q", result)
	}
}

func TestFireWebhookInterpolatesURLAndBody(t *testing.T) {
	var capturedBody string
	var capturedHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		capturedHeader = r.Header.Get("Title")
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name:    "test",
		URL:     srv.URL,
		Method:  "POST",
		Headers: map[string]string{"Title": "{{title}}"},
		Body:    "{{anime_name}} EP {{episode}}",
	}

	vars := buildVars("Frieren", 5, NewEpisode, "", "", 517276, "a" + strings.Repeat("0", 39))
	fireWebhook(preset, vars)

	if capturedBody != "Frieren EP 5" {
		t.Fatalf("body not interpolated: %q", capturedBody)
	}
	if capturedHeader == "" {
		t.Fatal("Title header not set")
	}
}

func TestBuildVarsDownloadFailedReason(t *testing.T) {
	vars := buildVars("Frieren", 5, DownloadFailed, ReasonNotFound, "", 517276, "a" + strings.Repeat("0", 39))
	if vars["reason"] != ReasonNotFound {
		t.Fatalf("reason var = %q", vars["reason"])
	}
	if want := "Frieren EP 5 falhou: " + ReasonNotFound; vars["message"] != want {
		t.Fatalf("message = %q, want %q", vars["message"], want)
	}
}

func TestFireTestWebhookNotFound(t *testing.T) {
	cfg := &files.Config{}
	err := FireTestWebhook(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing webhook")
	}
}

func TestFireTestWebhookFires(t *testing.T) {
	var fired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "my-hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test"},
			},
		},
	}
	if err := FireTestWebhook(cfg, "my-hook"); err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Fatal("webhook not fired")
	}
}

func TestNotifyNoOp_WhenNoWebhooks(t *testing.T) {
	Notify(&files.Config{}, NewEpisode, "Frieren", 5, "", 0, "")
}

func TestNotifyNoOp_WhenNilConfig(t *testing.T) {
	Notify(nil, NewEpisode, "Frieren", 5, "", 0, "")
}

func TestNotify_SkipsWebhookWhenEventNotSubscribed(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"download_failed"}},
			},
		},
	}

	Notify(cfg, NewEpisode, "Frieren", 5, "", 0, "")

	select {
	case <-fired:
		t.Fatal("webhook should not have fired for unsubscribed event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNotify_FiresWebhookWhenEventSubscribed(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	Notify(cfg, NewEpisode, "Frieren", 5, "", 0, "")

	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("webhook should have fired for subscribed event")
	}
}

// cfgWithWindow monta uma config de um webhook só, inscrito em todos os eventos.
func cfgWithWindow(url string, window int, headers map[string]string, body string) *files.Config {
	if headers == nil {
		headers = map[string]string{}
	}
	return &files.Config{
		Notifications: files.NotificationsConfig{
			BatchWindowSeconds: window,
			Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: url, Method: "POST", Headers: headers, Body: body,
				Events: []string{"new_episode", "download_failed", "download_completed"},
			}},
		},
	}
}

// collector devolve um servidor que empilha os bodies recebidos.
func collector(t *testing.T) (*httptest.Server, chan string) {
	t.Helper()
	bodies := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	t.Cleanup(srv.Close)
	return srv, bodies
}

// drain espera até `want` bodies ou o timeout, e devolve o que chegou.
func drain(bodies chan string, want int, timeout time.Duration) []string {
	var got []string
	deadline := time.After(timeout)
	for len(got) < want {
		select {
		case b := <-bodies:
			got = append(got, b)
		case <-deadline:
			return got
		}
	}
	// Dá uma folga pra pegar um request extra indevido.
	select {
	case b := <-bodies:
		got = append(got, b)
	case <-time.After(100 * time.Millisecond):
	}
	return got
}

// TestNotify_BatchesEventsWithinWindow: o motivo de existir o agrupamento. Um backfill de
// biblioteca dispara um download_completed por torrent (77 num caso real) e o ntfy.sh responde
// 429 a partir de certo volume — a janela tem que virar isso em uma request só.
func TestNotify_BatchesEventsWithinWindow(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, nil, "{{count}}|{{title}}|{{message}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "", 0, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "", 0, "")
	Notify(cfg, DownloadCompleted, "Dandadan", 3, "", 0, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("esperava 1 request para os 3 eventos, obteve %d: %v", len(got), got)
	}
	want := "3|3 downloads concluídos|" +
		"Re:ZERO EP 12 foi baixado com sucesso\n" +
		"Frieren EP 5 foi baixado com sucesso\n" +
		"Dandadan EP 3 foi baixado com sucesso"
	if got[0] != want {
		t.Errorf("body agrupado errado:\n obtido: %q\nesperado: %q", got[0], want)
	}
}

// TestNotify_SeparateBatchPerEvent: um preset inscrito só em download_failed não pode receber
// texto de sucesso. A fila é por evento justamente pra não misturar.
func TestNotify_SeparateBatchPerEvent(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, nil, "{{title}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "", 0, "")
	Notify(cfg, DownloadFailed, "Frieren", 5, ReasonNotFound, 0, "")

	got := drain(bodies, 2, 3*time.Second)
	if len(got) != 2 {
		t.Fatalf("esperava 2 requests (uma por evento), obteve %d: %v", len(got), got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen["Download concluído"] || !seen["Erro no download"] {
		t.Errorf("esperava um título por evento, obteve %v", got)
	}
}

// TestNotify_SingleItemBatchMatchesUnbatched: ligar o agrupamento não pode mudar a aparência de
// uma notificação que chegou sozinha na janela — é o que garante que os presets existentes
// continuam funcionando igual.
func TestNotify_SingleItemBatchMatchesUnbatched(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	const body = "{{title}}|{{message}}|{{anime_name}}|{{episode}}|{{reason}}"

	Notify(cfgWithWindow(srv.URL, 0, nil, body), DownloadFailed, "Frieren", 5, ReasonNotFound, 0, "")
	unbatched := drain(bodies, 1, time.Second)

	Notify(cfgWithWindow(srv.URL, 1, nil, body), DownloadFailed, "Frieren", 5, ReasonNotFound, 0, "")
	batched := drain(bodies, 1, 3*time.Second)

	if len(unbatched) != 1 || len(batched) != 1 {
		t.Fatalf("esperava 1 request de cada, obteve %d e %d", len(unbatched), len(batched))
	}
	if unbatched[0] != batched[0] {
		t.Errorf("janela de 1 item mudou o body:\n sem batch: %q\n com batch: %q", unbatched[0], batched[0])
	}
}

// TestNotify_WindowZeroFiresImmediately: 0 desliga o agrupamento, sem esperar timer nenhum.
func TestNotify_WindowZeroFiresImmediately(t *testing.T) {
	srv, bodies := collector(t)
	cfg := cfgWithWindow(srv.URL, 0, nil, "{{title}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "", 0, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "", 0, "")

	got := drain(bodies, 2, time.Second)
	if len(got) != 2 {
		t.Fatalf("com janela 0 esperava 2 requests imediatas, obteve %d", len(got))
	}
}

// TestFlush_SendsPendingBatch: sem isso uma janela inteira de notificações morreria com o
// processo no shutdown.
func TestFlush_SendsPendingBatch(t *testing.T) {
	srv, bodies := collector(t)
	// Janela longa: só o Flush pode fazer isso sair dentro do teste.
	cfg := cfgWithWindow(srv.URL, 3600, nil, "{{count}}")

	Notify(cfg, NewEpisode, "Re:ZERO", 12, "", 0, "")
	Notify(cfg, NewEpisode, "Frieren", 5, "", 0, "")

	if got := drain(bodies, 1, 200*time.Millisecond); len(got) != 0 {
		t.Fatalf("nada devia sair antes do Flush, obteve %v", got)
	}

	Flush() // síncrono: se voltar antes da request, o teste abaixo pega
	select {
	case b := <-bodies:
		if b != "2" {
			t.Errorf("Flush devia mandar os 2 itens juntos, obteve %q", b)
		}
	default:
		t.Error("Flush retornou sem a request ter chegado")
	}
}

// TestFireWebhook_EscapesJSONBody: nome com aspas e o \n do agrupamento quebrariam o body dos
// presets que embutem {{message}} num JSON (Discord, Slack, Telegram...), e o serviço responderia
// 400 com a notificação sumindo sem explicação.
// TestBuildVars_BatchCollapsesConsecutiveRun: 24 episodios consecutivos do mesmo anime num
// mesmo {{message}} viram uma linha so com range ("Index EPs 1-24 detectado, initiando download")
// em vez de 24 linhas identicas. Episodios nao consecutivos ou de animes diferentes ficam em
// linhas separadas — o colapsamento e por (anime, faixa continua), nunca por anime solto.
func TestBuildVars_BatchCollapsesConsecutiveRun(t *testing.T) {
	items := make([]item, 24)
	for i := range items {
		items[i] = item{animeName: "A Certain Magical Index", episode: i + 1}
	}
	vars := buildBatchVars(NewEpisode, items, "")

	want := "A Certain Magical Index EPs 1-24 detectado, iniciando download"
	if vars["message"] != want {
		t.Errorf("message = %q, want %q", vars["message"], want)
	}
	if vars["count"] != "24" {
		t.Errorf("count = %q, want 24", vars["count"])
	}
	// count e o total de itens da janela, nao o de linhas: o range inteiro e uma unica linha.
}

// TestBuildVars_BatchKeepsSeparateLinesOnGap: uma quebra de numero ou de anime reinicia a linha.
func TestBuildVars_BatchKeepsSeparateLinesOnGap(t *testing.T) {
	items := []item{
		{animeName: "Index", episode: 1},
		{animeName: "Index", episode: 2},
		{animeName: "Index", episode: 4}, // buraco: o 3 falta
		{animeName: "Frieren", episode: 5},
	}
	vars := buildBatchVars(NewEpisode, items, "")

	want := "Index EPs 1-2 detectado, iniciando download\n" +
		"Index EP 4 detectado, iniciando download\n" +
		"Frieren EP 5 detectado, iniciando download"
	if vars["message"] != want {
		t.Errorf("message = %q, want %q", vars["message"], want)
	}
}

// TestBuildVars_BatchRangeDownloadFailed: o range tambem vale para download_failed, e o reason
// do primeiro item da faixa e o que vira no {{reason}} (o default so entra quando estiver vazio).
func TestBuildVars_BatchRangeDownloadFailed(t *testing.T) {
	items := []item{
		{animeName: "Index", episode: 1, reason: ReasonNotFound},
		{animeName: "Index", episode: 2, reason: ReasonNotFound},
		{animeName: "Index", episode: 3, reason: ReasonNotFound},
	}
	vars := buildBatchVars(DownloadFailed, items, "")

	want := "Index EPs 1-3 falhou: " + ReasonNotFound
	if vars["message"] != want {
		t.Errorf("message = %q, want %q", vars["message"], want)
	}
}

// TestBuildVars_BatchRangeEnglish: a mesma linha em ingles.
func TestBuildVars_BatchRangeEnglish(t *testing.T) {
	items := make([]item, 5)
	for i := range items {
		items[i] = item{animeName: "Index", episode: i + 1}
	}
	vars := buildBatchVars(NewEpisode, items, "en")

	want := "Index EPs 1-5 detected, starting download"
	if vars["message"] != want {
		t.Errorf("message = %q, want %q", vars["message"], want)
	}
}

// TestBuildVars_SingleItemUnchanged: um item so continua idendico ao antes do colapsamento — a
// linha unica, sem range.
func TestBuildVars_SingleItemUnchanged(t *testing.T) {
vars := buildVars("Frieren", 5, NewEpisode, "", "", 517276, "a" + strings.Repeat("0", 39))
	if vars["message"] != "Frieren EP 5 detectado, iniciando download" {
		t.Errorf("message = %q", vars["message"])
	}
}

func TestFireWebhook_EscapesJSONBody(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1,
		map[string]string{"Content-Type": "application/json"},
		`{"content":"{{message}}"}`)

	Notify(cfg, DownloadCompleted, `Re:"ZERO"`, 12, "", 0, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "", 0, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("esperava 1 request, obteve %d", len(got))
	}

	var payload struct{ Content string }
	if err := json.Unmarshal([]byte(got[0]), &payload); err != nil {
		t.Fatalf("body não é JSON válido (%v): %s", err, got[0])
	}
	if !strings.Contains(payload.Content, `Re:"ZERO" EP 12`) {
		t.Errorf("aspas do nome perdidas no escape: %q", payload.Content)
	}
	if !strings.Contains(payload.Content, "\n") {
		t.Errorf("quebra de linha do agrupamento perdida: %q", payload.Content)
	}
}

// TestFireWebhook_DoesNotEscapePlainTextBody: o preset do ntfy manda `{{message}}` cru, então o
// \n tem que chegar como \n de verdade — escapar ali mandaria a barra-n literal.
func TestFireWebhook_DoesNotEscapePlainTextBody(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)
	cfg := cfgWithWindow(srv.URL, 1, map[string]string{"Title": "{{title}}"}, "{{message}}")

	Notify(cfg, DownloadCompleted, "Re:ZERO", 12, "", 0, "")
	Notify(cfg, DownloadCompleted, "Frieren", 5, "", 0, "")

	got := drain(bodies, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("esperava 1 request, obteve %d", len(got))
	}
	if strings.Contains(got[0], `\n`) {
		t.Errorf("body de texto puro não deve ser escapado: %q", got[0])
	}
	if !strings.Contains(got[0], "\n") {
		t.Errorf("esperava as duas linhas separadas por \\n: %q", got[0])
	}
}

// TestBuildVars_Language: o {{title}}/{{message}} do preset depende do campo `language`, e um
// config.json antigo (campo ausente) continua em português como antes.
func TestBuildVars_Language(t *testing.T) {
	pt := buildVars("Frieren", 5, NewEpisode, "", "", 517276, "a" + strings.Repeat("0", 39))
	if pt["title"] != "Novo episódio detectado" {
		t.Errorf("default lang title = %q", pt["title"])
	}
	if pt["message"] != "Frieren EP 5 detectado, iniciando download" {
		t.Errorf("default lang message = %q", pt["message"])
	}

	en := buildVars("Frieren", 5, NewEpisode, "", "en", 517276, "a" + strings.Repeat("0", 39))
	if en["title"] != "New episode detected" {
		t.Errorf("en title = %q", en["title"])
	}
	if en["message"] != "Frieren EP 5 detected, starting download" {
		t.Errorf("en message = %q", en["message"])
	}

	// Linguagem desconhecida cai no default.
	xx := buildVars("Frieren", 5, DownloadCompleted, "", "xx", 517276, "a" + strings.Repeat("0", 39))
	if xx["title"] != "Download concluído" {
		t.Errorf("unknown lang title = %q", xx["title"])
	}
}

// TestNotify_PresetLanguage: cada webhook da janela usa o proprio idioma, e um preset sem
// campo language (config.json antigo) continua disparando em português.
func TestNotify_PresetLanguage(t *testing.T) {
	srv, bodies := collector(t)
	t.Cleanup(Flush)

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			BatchWindowSeconds: 1,
			Webhooks: []files.WebhookPreset{
				{Name: "pt", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "{{title}}", Events: []string{"download_completed"}},
				{Name: "en", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "{{title}}", Language: "en", Events: []string{"download_completed"}},
			},
		},
	}

	Notify(cfg, DownloadCompleted, "Frieren", 5, "", 0, "")

	got := drain(bodies, 2, 3*time.Second)
	if len(got) != 2 {
		t.Fatalf("esperava 2 requests (um por preset), obteve %d: %v", len(got), got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen["Download concluído"] || !seen["Download completed"] {
		t.Errorf("cada preset devia usar o proprio idioma, obteve %v", got)
	}
}

// TestFireTestWebhook_UsesPresetLanguage: o disparo manual também respeita o idioma do preset.
func TestFireTestWebhook_UsesPresetLanguage(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "en", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "{{message}}", Language: "en"},
			},
		},
	}
if err := FireTestWebhook(cfg, "en"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "downloaded successfully") {
		t.Errorf("test webhook should fire in preset language, got %q", got)
	}
}

func TestFireWebhook_StripsNewlinesFromHeaders(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Title")
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{"Title": "{{message}}"},
		Body:    "x", Events: []string{"download_completed"},
	}
	fireWebhook(preset, buildBatchVars(DownloadCompleted, []item{
		{animeName: "Re:ZERO", episode: 12},
		{animeName: "Frieren", episode: 5},
	}, ""))

	select {
	case title := <-received:
		if title == "" {
			t.Error("header vazio: a request foi recusada em vez de sanitizada")
		}
		if strings.ContainsAny(title, "\n\r") {
			t.Errorf("header ainda tem quebra de linha: %q", title)
		}
	case <-time.After(time.Second):
		t.Fatal("request nunca chegou — net/http recusou o header")
	}
}

// TestFireWebhook_RetriesOn5xx: um 5xx e transitório — o servico de webhook pode
// ter um recém-ligado (cold start) ou pode estar sobrecarregado. O default e 3 tentativas
// com backoff exponencial, e o disparo so devolve erro apos todas.
func TestFireWebhook_RetriesOn5xx(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{}, Body: "x", Events: []string{"download_completed"},
		RetryAttempts: 3, RetryBackoffSeconds: 0, // backoff 0: sem sleep, o teste roda rápido
	}

	err := fireWebhook(preset, buildVars("Frieren", 5, DownloadCompleted, "", "", 517276, "a" + strings.Repeat("0", 39)))
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Errorf("esperava 3 tentativas, obteve %d", attempts)
	}
}

// TestFireWebhook_RetrySucceedsAfterFailure: a primeira falha (429) e seguida de uma
// resposta 200 — o disparo deve devolver nil e ter feito 2 requests.
func TestFireWebhook_RetrySucceedsAfterFailure(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{}, Body: "x", Events: []string{"download_completed"},
		RetryAttempts: 3, RetryBackoffSeconds: 0,
	}

	if err := fireWebhook(preset, buildVars("Frieren", 5, DownloadCompleted, "", "", 517276, "a" + strings.Repeat("0", 39))); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if attempts != 2 {
		t.Errorf("esperava 2 requests, obteve %d", attempts)
	}
}

// TestFireWebhook_NoRetryOnSuccess: um 200 na primeira tentativa não dispara retry.
func TestFireWebhook_NoRetryOnSuccess(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{}, Body: "x", Events: []string{"download_completed"},
		RetryAttempts: 5, RetryBackoffSeconds: 0,
	}

	if err := fireWebhook(preset, buildVars("Frieren", 5, DownloadCompleted, "", "", 517276, "a" + strings.Repeat("0", 39))); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("esperava 1 request, obteve %d", attempts)
	}
}

// TestFireWebhook_RetryDisabled: retry_attempts = 0 desliga a retentativa, e um 5xx vira
// erro imediato com uma so tentativa.
func TestFireWebhook_RetryDisabled(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	preset := files.WebhookPreset{
		Name: "hook", URL: srv.URL, Method: "POST",
		Headers: map[string]string{}, Body: "x", Events: []string{"download_completed"},
		RetryAttempts: 0, RetryBackoffSeconds: 0,
	}

	err := fireWebhook(preset, buildVars("Frieren", 5, DownloadCompleted, "", "", 517276, "a" + strings.Repeat("0", 39)))
	if err == nil {
		t.Fatal("expected error with retry disabled")
	}
	if attempts != 1 {
		t.Errorf("esperava 1 tentativa com retry desligado, obteve %d", attempts)
	}
}

// TestNotify_AnimeOverrideFiltersWebhooks: quando AnimeSettings.WebhookPresetNames lista apenas
// um preset, apenas esse dispara; os outros saem do lote.
func TestNotify_AnimeOverrideFiltersWebhooks(t *testing.T) {
	firedA := make(chan struct{}, 1)
	firedB := make(chan struct{}, 1)
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firedA <- struct{}{}
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firedB <- struct{}{}
	}))
	defer srvB.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook-a", URL: srvA.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
				{Name: "hook-b", URL: srvB.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	settings := map[int]files.AnimeSettings{
		42: {WebhookPresetNames: []string{"hook-a"}},
	}
	Notify(cfg, NewEpisode, "Frieren", 5, "", 42, "", settings)

	select {
	case <-firedA:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("hook-a should have fired for anime override")
	}
	select {
	case <-firedB:
		t.Fatal("hook-b should NOT have fired for anime override")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestNotify_AnimeOverrideNoMatch: quando WebhookPresetNames lista nomes que nao existem,
// nenhum webhook dispara.
func TestNotify_AnimeOverrideNoMatch(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	settings := map[int]files.AnimeSettings{
		42: {WebhookPresetNames: []string{"nonexistent"}},
	}
	Notify(cfg, NewEpisode, "Frieren", 5, "", 42, "", settings)

	select {
	case <-fired:
		t.Fatal("no webhook should have fired for nonexistent preset name")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestNotify_AnimeOverrideEmptyNames: quando WebhookPresetNames esta vazio, todos disparam
// (mesmo comportamento de nao haver override).
func TestNotify_AnimeOverrideEmptyNames(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	settings := map[int]files.AnimeSettings{
		42: {}, // WebhookPresetNames vazio -> sem override
	}
	Notify(cfg, NewEpisode, "Frieren", 5, "", 42, "", settings)

	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("webhook should have fired when override names are empty")
	}
}

// TestNotify_AnimeOverrideNoSettings: quando nao sao passadas configuracoes de anime, todos
// disparam (comportamento atual preservado).
func TestNotify_AnimeOverrideNoSettings(t *testing.T) {
	fired := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired <- struct{}{}
	}))
	defer srv.Close()

	cfg := &files.Config{
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{
				{Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "test", Events: []string{"new_episode"}},
			},
		},
	}

	Notify(cfg, NewEpisode, "Frieren", 5, "", 42, "")
	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("webhook should have fired when no anime settings are passed")
	}
}
