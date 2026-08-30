package notifications

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type Event int

const (
	NewEpisode Event = iota
	DownloadFailed
	DownloadCompleted
)

// Motivos de falha de download, usados como {{reason}} e na mensagem padrão.
const (
	ReasonNotFound         = "nenhum torrent encontrado"
	ReasonDownloadRejected = "torrent rejeitado"
	ReasonNoDiskSpace      = "espaço em disco insuficiente"
)

var reVar = regexp.MustCompile(`\{\{(\w+)\}\}`)

func eventString(e Event) string {
	switch e {
	case NewEpisode:
		return "new_episode"
	case DownloadFailed:
		return "download_failed"
	case DownloadCompleted:
		return "download_completed"
	}
	return ""
}

func interpolate(template string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(template, func(match string) string {
		key := reVar.FindStringSubmatch(match)[1]
		return vars[key]
	})
}

func buildVars(animeName string, episode int, event Event, reason string) map[string]string {
	return buildBatchVars(event, []item{{animeName: animeName, episode: episode, reason: reason}})
}

// buildBatchVars monta as variaveis de template para os itens de uma janela. Com um item so o
// resultado e identico ao de antes do agrupamento — e por isso que ligar o batching nao muda a
// aparencia das notificacoes de quem recebe uma de cada vez.
//
// Com N > 1, `anime_name`, `episode` e `reason` ficam VAZIOS: nao existe valor unico para eles e
// mandar o do primeiro item faria o template mentir sobre os outros N-1. Quem quer identificar os
// episodios usa `message` (uma linha por item) ou `count`.
func buildBatchVars(event Event, items []item) map[string]string {
	lines := make([]string, 0, len(items))
	for _, it := range items {
		_, message := eventStrings(it.animeName, it.episode, event, it.reason)
		lines = append(lines, message)
	}

	vars := map[string]string{
		"title":      batchTitle(event, items),
		"message":    strings.Join(lines, "\n"),
		"anime_name": "",
		"episode":    "",
		"reason":     "",
		"count":      fmt.Sprintf("%d", len(items)),
		"quality":    "",
		"file_path":  "",
		"timestamp":  time.Now().Format("2006-01-02 15:04"),
	}

	if len(items) == 1 {
		vars["anime_name"] = items[0].animeName
		vars["episode"] = fmt.Sprintf("%d", items[0].episode)
		vars["reason"] = items[0].reason
	}

	return vars
}

func batchTitle(event Event, items []item) string {
	if len(items) == 1 {
		title, _ := eventStrings(items[0].animeName, items[0].episode, event, items[0].reason)
		return title
	}
	switch event {
	case NewEpisode:
		return fmt.Sprintf("%d novos episódios detectados", len(items))
	case DownloadFailed:
		return fmt.Sprintf("%d erros no download", len(items))
	case DownloadCompleted:
		return fmt.Sprintf("%d downloads concluídos", len(items))
	}
	return ""
}

func eventStrings(animeName string, episode int, event Event, reason string) (title, message string) {
	switch event {
	case NewEpisode:
		return "Novo episódio detectado",
			fmt.Sprintf("%s EP %d detectado, iniciando download", animeName, episode)
	case DownloadFailed:
		if reason == "" {
			reason = "todas as tentativas falharam"
		}
		return "Erro no download",
			fmt.Sprintf("%s EP %d falhou: %s", animeName, episode, reason)
	case DownloadCompleted:
		return "Download concluído",
			fmt.Sprintf("%s EP %d foi baixado com sucesso", animeName, episode)
	}
	return "", ""
}

// jsonEscape devolve o valor pronto para ser colado DENTRO de uma string JSON (sem as aspas
// externas). Um nome de anime com aspas, barra invertida ou o \n do agrupamento quebraria o body
// dos presets que embutem {{message}} num JSON (Discord, Slack, Telegram, Gotify, Pushover,
// Apprise): o servico responderia 400 e a notificacao sumiria sem explicacao no log.
func jsonEscape(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil { // json.Marshal de string nao falha; se falhar, melhor o valor cru que nada
		return value
	}
	return string(encoded[1 : len(encoded)-1])
}

// escapeVarsForJSON aplica jsonEscape em todas as variaveis. So e usado no body, e so quando o
// preset se declara JSON: a URL tem regras de escape proprias e um body de texto puro (o do ntfy
// e `{{message}}` cru) tem de receber o \n de verdade.
func escapeVarsForJSON(vars map[string]string) map[string]string {
	escaped := make(map[string]string, len(vars))
	for k, v := range vars {
		escaped[k] = jsonEscape(v)
	}
	return escaped
}

func presetIsJSON(preset files.WebhookPreset) bool {
	for k, v := range preset.Headers {
		if strings.EqualFold(k, "Content-Type") && strings.Contains(strings.ToLower(v), "json") {
			return true
		}
	}
	return false
}

// fireWebhook dispara um preset e devolve o que houve de errado. O disparo automatico ignora o
// retorno (o log ja registra); quem precisa dele e o botao "Testar", que sem isso respondia
// sucesso com o servico recusando o token — ver decisions.md #86.
func fireWebhook(preset files.WebhookPreset, vars map[string]string) error {
	bodyVars := vars
	if presetIsJSON(preset) {
		bodyVars = escapeVarsForJSON(vars)
	}

	url := interpolate(preset.URL, vars)
	body := interpolate(preset.Body, bodyVars)

	req, err := http.NewRequest(preset.Method, url, strings.NewReader(body))
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Failed to build webhook request")
		return err
	}
	for k, v := range preset.Headers {
		// Um header com \n faz o net/http recusar a request inteira. O ntfy usa
		// `Title: {{title}}`, e nada impede alguem de colocar {{message}} num header.
		req.Header.Set(k, strings.NewReplacer("\n", " ", "\r", " ").Replace(interpolate(v, vars)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Str("webhook", preset.Name).Msg("Webhook request failed")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Logger.Warn().
			Str("webhook", preset.Name).
			Int("status", resp.StatusCode).
			Msg("Webhook returned error status")
		return fmt.Errorf("webhook %q returned status %d", preset.Name, resp.StatusCode)
	}
	return nil
}

// item e um evento aguardando na janela de agrupamento.
type item struct {
	animeName string
	episode   int
	reason    string
}

// batch acumula os itens de uma janela de um evento. O timer e guardado junto para que Flush
// possa cancela-lo: sem isso o flush do shutdown e o do timer disputariam a mesma fila.
type batch struct {
	items   []item
	timer   *time.Timer
	webhook []files.WebhookPreset
}

var (
	batchMu      sync.Mutex
	pendingBatch = make(map[Event]*batch)
)

// Notify fires webhooks subscribed to the given event in background goroutines.
// No-op if cfg is nil or has no webhooks.
//
// Com cfg.Notifications.BatchWindowSeconds > 0 o evento entra numa fila por tipo de evento e sai
// junto com os outros da mesma janela, num webhook so. Um backfill de biblioteca dispara um
// DownloadCompleted por torrent (77 num caso real) e servicos como o ntfy.sh respondem 429 a
// partir de um certo volume — ver decisions.md #47.
func Notify(cfg *files.Config, event Event, animeName string, episode int, reason string) {
	if cfg == nil || len(cfg.Notifications.Webhooks) == 0 {
		return
	}

	it := item{animeName: animeName, episode: episode, reason: reason}
	window := time.Duration(cfg.Notifications.BatchWindowSeconds) * time.Second
	if window <= 0 {
		fireBatch(event, []item{it}, cfg.Notifications.Webhooks, false)
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	if b, ok := pendingBatch[event]; ok {
		b.items = append(b.items, it)
		// Os webhooks da janela sao os da ultima config vista: se o usuario mexeu nos presets no
		// meio da janela, o que vale e o mais recente.
		b.webhook = cfg.Notifications.Webhooks
		return
	}

	b := &batch{items: []item{it}, webhook: cfg.Notifications.Webhooks}
	pendingBatch[event] = b
	b.timer = time.AfterFunc(window, func() { flushEvent(event, false) })
}

// Flush dispara agora tudo que esta esperando janela e SO retorna quando as requests terminaram.
// Chamado no shutdown do daemon: sem isso ate uma janela inteira de notificacoes morreria com o
// processo — e disparar em goroutine no shutdown seria o mesmo que nao disparar, porque o
// processo sai antes delas rodarem.
func Flush() {
	batchMu.Lock()
	events := make([]Event, 0, len(pendingBatch))
	for event := range pendingBatch {
		events = append(events, event)
	}
	batchMu.Unlock()

	for _, event := range events {
		flushEvent(event, true)
	}
}

func flushEvent(event Event, wait bool) {
	batchMu.Lock()
	b, ok := pendingBatch[event]
	if !ok {
		batchMu.Unlock()
		return
	}
	delete(pendingBatch, event)
	if b.timer != nil {
		b.timer.Stop()
	}
	batchMu.Unlock()

	fireBatch(event, b.items, b.webhook, wait)
}

func fireBatch(event Event, items []item, webhooks []files.WebhookPreset, wait bool) {
	if len(items) == 0 {
		return
	}
	eventStr := eventString(event)
	vars := buildBatchVars(event, items)

	var wg sync.WaitGroup
	for _, preset := range webhooks {
		if !slices.Contains(preset.Events, eventStr) {
			continue
		}
		wg.Add(1)
		go func(p files.WebhookPreset) {
			defer wg.Done()
			fireWebhook(p, vars)
		}(preset)
	}
	if wait {
		wg.Wait()
	}
}

// ErrWebhookNotFound distingue "esse preset nao existe" (404) de "o preset existe e o disparo
// falhou" (502). Sem a distincao o handler devolveria 404 para o token recusado pelo servico.
var ErrWebhookNotFound = errors.New("webhook not found")

// FireTestWebhook fires one named webhook with sample variables. Returns ErrWebhookNotFound if the
// preset does not exist, or the delivery error if the service refused it.
func FireTestWebhook(cfg *files.Config, name string) error {
	for _, preset := range cfg.Notifications.Webhooks {
		if preset.Name == name {
			vars := buildVars("Frieren Beyond Journey's End", 5, DownloadCompleted, "")
			return fireWebhook(preset, vars)
		}
	}
	return fmt.Errorf("%w: %q", ErrWebhookNotFound, name)
}
