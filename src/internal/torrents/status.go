package torrents

import "github.com/cenkalti/rain/v2/torrent"

// Slugs de status que o resto do código precisa comparar por nome. StatusQueued é o único
// que NÃO sai do enum da rain: quem o escreve é a fila (queue.markQueued), porque para a
// rain um torrent enfileirado é apenas mais um torrent parado.
const (
	StatusStopped  = "stopped"
	StatusStopping = "stopping"
	StatusQueued   = "queued"
)

// statusSlug converte o enum de status da rain num slug estável de API.
//
// A rain expõe Status.String(), mas ele devolve display text com espaço
// ("Downloading Metadata") e pode ser reescrito em qualquer upgrade da lib. O slug daqui é
// contrato com a WebUI (chave de tradução), então é mapeado à mão de propósito.
func statusSlug(s torrent.Status) string {
	switch s {
	case torrent.Stopped:
		return StatusStopped
	case torrent.DownloadingMetadata:
		return "downloading_metadata"
	case torrent.Allocating:
		return "allocating"
	case torrent.Verifying:
		return "verifying"
	case torrent.Downloading:
		return "downloading"
	case torrent.Seeding:
		return "seeding"
	case torrent.Stopping:
		return StatusStopping
	default:
		return "unknown"
	}
}
