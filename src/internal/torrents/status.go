package torrents

import "github.com/cenkalti/rain/v2/torrent"

// statusSlug converte o enum de status da rain num slug estável de API.
//
// A rain expõe Status.String(), mas ele devolve display text com espaço
// ("Downloading Metadata") e pode ser reescrito em qualquer upgrade da lib. O slug daqui é
// contrato com a WebUI (chave de tradução), então é mapeado à mão de propósito.
func statusSlug(s torrent.Status) string {
	switch s {
	case torrent.Stopped:
		return "stopped"
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
		return "stopping"
	default:
		return "unknown"
	}
}
