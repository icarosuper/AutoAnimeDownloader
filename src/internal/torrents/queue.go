package torrents

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"sync"

	"AutoAnimeDownloader/src/internal/logger"
)

// queueOps e a fatia do backend que a fila precisa. Os tres metodos sao NAO-EXPORTADOS de
// proposito: sao as delegacoes CRUAS, sem nada da fila em cima.
//
//   - pause/resume: se fossem Pause/Resume, cada pausa decidida pela fila dispararia outro
//     enforce e a recursao nao teria fundo.
//   - list: se fosse List, a marcacao do status "queued" reentraria em queue.mu, que
//     enforce ja segura — deadlock, nao recursao.
type queueOps interface {
	list() []TorrentInfo
	pause(hash string) error
	resume(hash string) error
}

// queue e a lista ordenada de TODOS os torrents incompletos da sessao: os `limit` primeiros
// que nao estao pausados baixam, o resto espera. Conter tudo (e nao so quem a fila pausou) e
// o que faz "priorizar" preemptar de verdade e o que da uma posicao estavel para mostrar na
// tela — nada e inserido no meio, entao a posicao so muda por acao do usuario.
//
// Ordem de lock: queue.mu -> SessionManager.mu, NUNCA o contrario. enforce segura o
// queue.mu enquanto chama list/pause/resume, que pegam o RLock do manager; por isso todo
// metodo do SessionManager solta o proprio lock antes de tocar na fila.
type queue struct {
	mu     sync.Mutex
	limit  int      // 0 ou negativo = ilimitado
	order  []string // todo torrent incompleto, na ordem em que devem baixar
	paused []string // pausados pelo usuario: nao ocupam slot, nunca sobem sozinhos.
	// SO hashes incompletos: Pause/Resume de um torrent completo delegam direto para a rain
	// (seeding nunca ocupou slot, entao a fila nao tem escrituracao para ele).

	// queued e o resultado do passo 3 do ultimo enforce: quem esta em order, nao esta em
	// paused e ficou fora dos `limit` primeiros — mapeado para a POSICAO (1-based) na
	// espera. Existe para markQueued nao ter de refazer a conta a cada List (a UI faz
	// polling), e e o unico lugar de onde saem o slug e a posicao.
	queued map[string]int

	path      string // <dir do banco de resume>/queue.json
	lastSaved []byte // ultimo JSON gravado; a escrita so acontece quando o novo difere
	// seedPaused e o latch de upgrade: armado na leitura quando nao ha queue.json,
	// consumido pelo primeiro enforce COM sessao. Ver load.
	seedPaused bool
}

// queueFileName e o arquivo de estado da fila, ao lado do banco de resume da rain.
const queueFileName = "queue.json"

// queueState e o formato em disco. Fica ao lado do banco de resume da rain pelo mesmo motivo
// do download_root.id: e estado do torrent client, nao configuracao do usuario.
type queueState struct {
	Order  []string `json:"order"`
	Paused []string `json:"paused"`
}

// load le queue.json. Arquivo ausente, vazio ou ilegivel = fila vazia; nunca impede o daemon
// de subir, porque o passo 2 do enforce reconstroi a ordem por AddedAt.
//
// Arquivo AUSENTE arma o latch de semeadura: sem ele, o primeiro boot depois deste deploy
// retomaria os torrents que o usuario tinha pausado a mao (a rain os persiste como stopped,
// e o passo 2 anexa todo incompleto). Semear aqui e impossivel — em NewSessionManager ainda
// nao existe sessao para consultar —, entao a leitura so arma; quem consome e o enforce.
func (q *queue) load(path string) {
	q.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Logger.Warn().Err(err).Str("path", path).Msg("Could not read the download queue file; rebuilding the queue from scratch")
		}
		q.seedPaused = true
		return
	}
	var state queueState
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Logger.Warn().Err(err).Str("path", path).Msg("The download queue file is corrupted; rebuilding the queue from scratch")
		return
	}
	q.order, q.paused = state.Order, state.Paused
	q.lastSaved = data
}

// save grava order/paused quando diferem do ultimo estado gravado. Chamada com q.mu segurado.
//
// lastSaved SO e atualizado depois do Rename bem-sucedido: atualiza-lo junto com a tentativa
// transformaria uma falha transitoria (disco cheio por um minuto) em perda permanente — o
// "so grava quando muda" nunca mais dispararia e o arquivo ficaria congelado na versao velha.
func (q *queue) save() {
	if q.path == "" {
		return
	}
	data, err := json.Marshal(queueState{Order: q.order, Paused: q.paused})
	if err != nil || bytes.Equal(data, q.lastSaved) {
		return
	}
	// tmp + Rename, o mesmo padrao de FileManager.saveConfigsLocked.
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Logger.Warn().Err(err).Str("path", q.path).Msg("Failed to persist the download queue")
		return
	}
	if err := os.Rename(tmp, q.path); err != nil {
		_ = os.Remove(tmp)
		logger.Logger.Warn().Err(err).Str("path", q.path).Msg("Failed to persist the download queue")
		return
	}
	q.lastSaved = data
}

// setLimit troca o limite. O chamador roda enforce em seguida.
func (q *queue) setLimit(n int) {
	q.mu.Lock()
	q.limit = n
	q.mu.Unlock()
}

// drop tira um hash da fila inteira. Usado pela remocao.
func (q *queue) drop(hash string) {
	q.mu.Lock()
	q.order = without(q.order, hash)
	q.paused = without(q.paused, hash)
	q.mu.Unlock()
}

// pushBack manda o hash para o FIM da fila e o tira de paused. E o que um resume do usuario
// faz: volta para a fila, nao para o topo dela.
func (q *queue) pushBack(hash string) {
	q.mu.Lock()
	q.order = append(without(q.order, hash), hash)
	q.paused = without(q.paused, hash)
	q.mu.Unlock()
}

// markPaused registra a pausa manual sem mexer na posicao. Chamado ANTES do pause de verdade:
// no intervalo entre as duas chamadas, um enforce disparado por um torrent que acabou de
// completar promoveria justamente o torrent que o usuario esta parando.
func (q *queue) markPaused(hash string) {
	q.mu.Lock()
	if !contains(q.paused, hash) {
		q.paused = append(q.paused, hash)
	}
	q.mu.Unlock()
}

// prioritize move para o INICIO os hashes que ja estao em order e INSERE no inicio os que
// ainda nao estao, na ordem recebida. E "move ou insere" porque o hash pode ser incompleto e
// ainda nao estar em order (sessao recem-criada, nenhum enforce desde entao); tratar so o
// caso "ja esta la" faria o priorizado sumir da fila em vez de ir para a frente.
//
// Hash desconhecido ou completo entra e e podado pelo passo 1 do enforce seguinte.
func (q *queue) prioritize(hashes []string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	front := make([]string, 0, len(hashes))
	rest := q.order
	for _, h := range hashes {
		if contains(front, h) {
			continue
		}
		front = append(front, h)
		rest = without(rest, h)
		q.paused = without(q.paused, h)
	}
	q.order = append(front, rest...)
}

// markQueued escreve o slug "queued" e a posicao nos torrents que estao esperando. E o unico
// slug que nao sai de statusSlug() — a rain nao sabe que existe uma fila.
//
// A fonte e q.queued, nao order: order contem os ativos tambem, e marcar por pertencer a ela
// pintaria a tela inteira de "queued". Quem esta em paused mostra "stopped", que e o que a
// rain reporta; "queued" fica significando so "a fila vai chegar nele".
//
// QueuePosition 0 = nao enfileirado (ativo, completo ou pausado). Um pausado tem posicao em
// order, mas ela nao preve nada — ele nunca sobe sozinho — e publicar um numero que nao vai
// acontecer e pior do que nao publicar nenhum.
func (q *queue) markQueued(infos []TorrentInfo) []TorrentInfo {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range infos {
		if pos, ok := q.queued[infos[i].Hash]; ok {
			infos[i].Status = StatusQueued
			infos[i].QueuePosition = pos
		}
	}
	return infos
}

// enforce e o unico ponto de decisao da fila: uma reconciliacao entre order/paused e o que a
// sessao reporta. Nao ha argumento "protected" — priorizar ja reposiciona o hash no inicio de
// order, entao a preempcao deixa de ser caso especial.
func (q *queue) enforce(ops queueOps) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 0. Sem sessao, sai na hora. list() devolve nil enquanto nao ha sessao; uma sessao sem
	//    torrents devolve slice vazio NAO-nil. A distincao e exatamente a que importa: nil e
	//    "nao sei nada", nao "nao ha nada". Sem esta guarda o passo 1 podaria order/paused
	//    inteiros e o passo 5 gravaria o arquivo vazio por cima — em TODO boot, porque a
	//    verificacao chama SetMaxActiveDownloads antes do Ensure.
	all := ops.list()
	if all == nil {
		return
	}

	// Ordem de adicao, sempre: ListTorrents da rain itera um map, entao a ordem da fatia e
	// aleatoria a cada chamada. AddedAt existe por um motivo so: dar ordem inicial aos
	// torrents que a fila ainda nao conhece.
	sort.SliceStable(all, func(i, j int) bool { return all[i].AddedAt.Before(all[j].AddedAt) })

	byHash := make(map[string]TorrentInfo, len(all))
	for _, t := range all {
		byHash[t.Hash] = t
	}
	// Torrent completo nao entra em order: seeding nao ocupa slot, e ao completar o hash sai
	// da fila liberando um slot.
	incomplete := func(h string) bool {
		t, ok := byHash[h]
		return ok && !t.Completed
	}

	// 1. Poda: sai de order/paused quem sumiu da sessao ou completou.
	q.order = filter(q.order, incomplete)
	q.paused = filter(q.paused, incomplete)

	// Latch de upgrade, uma vez na vida da instalacao: sem queue.json nao ha como distinguir
	// uma pausa manual de um torrent que a fila antiga (so memoria) tinha pausado, entao os
	// dois grupos entram em paused. Congelar e visivel na tela e custa um clique em Play;
	// retomar sozinho e invisivel e gasta banda desfazendo uma decisao do usuario.
	if q.seedPaused {
		q.seedPaused = false
		seeded := 0
		for _, t := range all {
			if !t.Completed && t.Status == StatusStopped && !contains(q.paused, t.Hash) {
				q.paused = append(q.paused, t.Hash)
				seeded++
			}
		}
		if seeded > 0 {
			logger.Logger.Info().Int("count", seeded).
				Msg("No download queue file yet: torrents already stopped were kept paused; use Play or Prioritize to start them")
		}
	}

	// 2. Anexa: todo incompleto da sessao ausente de order entra no fim, por AddedAt. E o caso
	//    de Add e o do boot com um queue.json desatualizado.
	for _, t := range all {
		if !t.Completed && !contains(q.order, t.Hash) {
			q.order = append(q.order, t.Hash)
		}
	}

	// 3. Calcula o desejado: percorre order pulando paused, marcando os `limit` primeiros como
	//    ativos e o resto como enfileirados, numerados a partir de 1 na ordem em que vao
	//    comecar. A numeracao nao custa uma segunda passada.
	wanted := make(map[string]bool, len(q.order))
	queued := make(map[string]int)
	active, waiting := 0, 0
	for _, h := range q.order {
		if contains(q.paused, h) {
			continue
		}
		if q.limit <= 0 || active < q.limit {
			wanted[h] = true
			active++
			continue
		}
		waiting++
		queued[h] = waiting
	}
	q.queued = queued

	// 4. Aplica a diferenca iterando ORDER, nao a sessao. Varrer a sessao pausaria todo
	//    seeder: completo nunca entra em order, logo nunca esta em `wanted`, logo "nao deveria
	//    estar ativo". order e a lista dos incompletos e e exatamente o conjunto sobre o qual
	//    a fila tem autoridade.
	//
	//    "stopping" nao e alvo de nada: a pausa da rain e assincrona, entao trata-lo como
	//    parado mandaria resume por cima de um stop em andamento, e trata-lo como rodando o
	//    pausaria de novo. Fica de fora dos dois lados e o passe seguinte decide sobre um
	//    estado estavel.
	for _, h := range q.order {
		t := byHash[h]
		switch {
		case t.Status == StatusStopping:
		case wanted[h]:
			if t.Status == StatusStopped {
				if err := ops.resume(h); err != nil {
					logger.Logger.Warn().Err(err).Str("hash", h).Msg("Download queue: failed to resume torrent")
				}
			}
		case t.Status != StatusStopped:
			if err := ops.pause(h); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", h).Msg("Download queue: failed to pause torrent")
				// q.queued e o DESEJADO, e a tela nao pode mostra-lo como se fosse o real: o
				// hash continua na posicao (a fila ainda quer aquilo), mas o slug e o numero
				// saem, senao a linha exibiria "Na fila #3" num torrent baixando a todo vapor.
				// O buraco na numeracao dura ate o proximo passe.
				delete(q.queued, h)
			}
		}
	}

	// 5. Salva, se mudou.
	q.save()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func without(list []string, s string) []string {
	return filter(list, func(v string) bool { return v != s })
}

// filter compacta a fatia no lugar. Seguro para order/paused: sao slices privados da fila.
func filter(list []string, keep func(string) bool) []string {
	out := list[:0]
	for _, v := range list {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
