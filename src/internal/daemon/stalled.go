package daemon

import (
	"fmt"
	"sync"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
)

// stalledTorrentTimeout e quanto tempo um torrent pode ficar tentando com ZERO peers antes de o
// daemon desistir dele.
//
// ponytail: constante, nao config. Vira campo de config.json quando alguem pedir um numero
// diferente — o custo de descobrir que uma hora e curta demais e um passe extra, nao um download
// perdido (os bytes ficam, ver #24).
//
// O chip "sem seeds" da tela dispara em 10 minutos e este corte e MUITO mais folgado de proposito:
// la o efeito e pintar um aviso, aqui e derrubar o torrent da sessao.
const stalledTorrentTimeout = time.Hour

// stalledTorrents guarda, por hash, desde quando aquele torrent esta a zero peers.
//
// Estado de pacote (e nao campo de State) porque so o passe de verificacao escreve nele e nada
// fora do daemon precisa le-lo: promover para State significaria expor na API um relogio que a
// tela ja mede sozinha (frontend `lib/stores/stallTracker.ts`, que continua existindo — ele
// alimenta o chip, este aqui decide a acao).
//
// Em memoria, como o do frontend: um restart re-observa os stalls do zero, e o preco disso e no
// maximo um ciclo a mais de espera.
var stalledTorrents = struct {
	mu    sync.Mutex
	since map[string]time.Time
}{since: map[string]time.Time{}}

// stalling responde se este torrent esta tentando e nao conseguindo.
//
// Os dois status importam: fora deles zero peers nao e sintoma de nada. Um torrent ENFILEIRADO
// esta a zero peers porque nem comecou (queue.markQueued), um pausado porque o usuario mandou
// parar, e um completo porque ninguem esta puxando dele — derrubar qualquer um dos tres por
// "sem peers" seria matar o funcionamento normal do app.
func stalling(t torrents.TorrentInfo) bool {
	if t.Completed || t.PeersTotal != 0 {
		return false
	}
	return t.Status == torrents.StatusDownloading || t.Status == torrents.StatusDownloadingMetadata
}

// dropStalledTorrents derruba os torrents que passaram stalledTorrentTimeout sem nenhum peer.
//
// Torrent travado nao volta sozinho: a rain segue anunciando para um swarm que nao existe, e
// enquanto isso o torrent ocupa um slot de max_concurrent_downloads para sempre — com anime velho
// esse e o caso comum, nao a excecao. O passe media isso porque e ele quem ja tem o snapshot da
// sessao (backend.List), e roda no ritmo de check_interval.
//
// A acao reusa HandleTorrentFailure inteira em vez de inventar caminho novo: e exatamente o que o
// app ja faz com um torrent que a rain parou com erro — webhook download_failed, sai da sessao
// (guardando os bytes ja baixados) e o passe SEGUINTE procura de novo no Nyaa, possivelmente
// achando um release mais vivo (decisions.md #24). O snapshot recebido nao e alterado: quem saiu
// da sessao agora so e reprocurado no proximo passe, e nao no meio deste.
//
// Pausar em vez de derrubar nao serve: a fila trata pausado como "nunca sobe sozinho"
// (torrents/queue.go), entao o download travado viraria travado e silencioso.
func dropStalledTorrents(snapshot []torrents.TorrentInfo, backend torrents.TorrentBackend, fileManager FileManagerInterface, now time.Time) {
	stalledTorrents.mu.Lock()
	defer stalledTorrents.mu.Unlock()

	stillStalling := make(map[string]bool, len(snapshot))
	for _, t := range snapshot {
		if !stalling(t) {
			continue
		}
		stillStalling[t.Hash] = true

		since, seen := stalledTorrents.since[t.Hash]
		if !seen {
			stalledTorrents.since[t.Hash] = now
			continue
		}
		if now.Sub(since) < stalledTorrentTimeout {
			continue
		}

		delete(stalledTorrents.since, t.Hash)
		logger.Logger.Warn().Str("hash", t.Hash).Str("name", t.Name).Dur("stalled_for", now.Sub(since)).
			Msg("Dropping stalled torrent: no peers for too long")
		HandleTorrentFailure(t.Hash, fmt.Errorf("stalled: no peers for %s", now.Sub(since).Round(time.Minute)), backend, fileManager)
	}

	// Poda: hash que reencontrou peers, foi pausado, completou ou sumiu da sessao nao guarda
	// relogio. Sem isto o mapa cresceria para sempre e um torrent que travou, voltou e travou de
	// novo semanas depois seria derrubado na hora, pelo carimbo antigo.
	for hash := range stalledTorrents.since {
		if !stillStalling[hash] {
			delete(stalledTorrents.since, hash)
		}
	}
}
