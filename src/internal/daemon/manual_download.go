package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"fmt"
	"time"
)

// addAndPrioritize adiciona o magnet e o poe na frente da fila de downloads.
//
// Um download pedido a mao tem de comecar agora, nao quando a fila chegar nele — mas
// continua respeitando max_concurrent_downloads: em vez de virar um ativo a mais, ele
// rebaixa o torrent automatico menos adiantado. Um limite que a UI mostra sendo furado e
// um limite em que ninguem confia.
//
// A falha do Prioritize e so logada: o torrent ja esta na sessao e vai baixar de qualquer
// jeito, so que na vez dele — abortar o download inteiro por causa disso seria pior.
func addAndPrioritize(backend torrents.TorrentBackend, magnet string, configs *files.Config) (string, error) {
	// A guarda de espaco em disco fica aqui (e nao em torrents.Session.Add) porque o pacote
	// torrents nao conhece files.Config — passar a config para la so para ler uma porcentagem
	// inverteria a dependencia. Este e o unico Add dos caminhos manuais.
	if err := checkDiskSpace(configs); err != nil {
		return "", err
	}
	hash, err := backend.Add(magnet)
	if err != nil || hash == "" {
		return hash, err
	}
	if err := backend.Prioritize(hash); err != nil {
		logger.Logger.Warn().Err(err).Str("hash", hash).
			Msg("Manual download added but could not be prioritized; it will start when the queue reaches it")
	}
	return hash, nil
}

type animeDetails struct {
	mediaList  anilist.MediaList
	title      string
	isFinished bool
}

// resolveAnimeDetails busca um anime pelo MEDIA id, colapsando as contas configuradas, e cai em
// GetMediaByID quando nenhuma conta o acompanha.
//
// O fallback e o que faz os botoes por episodio (download, redownload, replace) funcionarem num
// anime avulso: a tela de detalhe dele abre — resolveMediaList ja cobria isso —, mas cada acao
// morria aqui com "nao esta em nenhuma conta". Sem contas configuradas, que passou a ser um
// estado valido, esse era o caminho de TODO download manual.
//
// Nao ha checagem contra o arquivo de avulsos aqui, ao contrario de api.resolveMediaList: estas
// funcoes ja exigem um media id e um episode id que so a tela de detalhe fornece, e o resultado
// e sempre gravado com ManuallyManaged=true — o loop nunca o apaga por nao acompanhar o anime.
// Um id que nao existe na AniList continua devolvendo erro.
//
// O fallback (ml == nil) e o mesmo caminho sintetico de appendStandaloneAnimes: passa por
// withStandaloneProgress para injetar o progresso salvo. Sem isso RunAnimeDebug relatava um
// avulso com progresso gravado como se comecasse do episodio 1 — o pipeline de debug divergindo
// do pipeline real que ele existe para espelhar. Nos chamadores de ManualDownload* o efeito e
// inocuo: eles selecionam episodio por numero explicito (findEpisodeNode), nao por Progress.
func resolveAnimeDetails(fm FileManagerInterface, animeId int, usernames []string) (*animeDetails, error) {
	ml, err := anilist.GetAnimeInfo(animeId, usernames, anilist.PriorityCritical)
	if err != nil {
		return nil, fmt.Errorf("failed to get anime info: %w", err)
	}
	if ml == nil {
		if ml, err = anilist.GetMediaByID(animeId, anilist.PriorityCritical); err != nil {
			return nil, fmt.Errorf("failed to get anime info: %w", err)
		}
		ml = withStandaloneProgress(fm, ml)
	}
	if ml == nil {
		return nil, fmt.Errorf("anime %d does not exist on AniList", animeId)
	}

	title := ""
	if ml.Media.Title.English != nil && *ml.Media.Title.English != "" {
		title = *ml.Media.Title.English
	} else if ml.Media.Title.Romaji != nil {
		title = *ml.Media.Title.Romaji
	}

	return &animeDetails{
		mediaList:  *ml,
		title:      title,
		isFinished: ml.Media.Status == anilist.MediaStatusFinished,
	}, nil
}

// findEpisodeNode acha o episodio pelo NUMERO na lista completa do anime — e nao no
// airingSchedule cru, que nao tem no para episodio fora da janela que a AniList guarda. Sem isso
// os botoes por episodio morriam com "episodio nao encontrado" justamente nos animes em que a
// tela mostra os episodios sinteticos (ver anilist.EpisodeList e decisions.md #52).
func findEpisodeNode(ml anilist.MediaList, episodeNumber int) *anilist.AiringNode {
	for _, node := range anilist.EpisodeList(ml, 1) {
		if node.Episode == episodeNumber {
			n := node
			return &n
		}
	}
	return nil
}

// ManualDownloadEpisodeWithMagnet downloads a specific episode using a user-supplied magnet link.
// Skips Nyaa search entirely. Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisodeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, episodeNumber int, magnet string, configs *files.Config) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs.AnilistUsernames)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList, episodeNumber)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeNumber, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)
	hash, err := addAndPrioritize(backend, magnet, configs)
	if err != nil || hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	return files.EpisodeStruct{
		AnimeID:         animeId,
		AnimeName:       details.title,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		EpisodeNumber:   targetNode.Episode,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}

// ManualDownloadAnimeWithMagnet downloads an entire anime using a user-supplied batch magnet link.
// Marks all aired episodes as downloaded sharing the same torrent hash.
func ManualDownloadAnimeWithMagnet(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, magnet string, configs *files.Config) ([]files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return nil, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs.AnilistUsernames)
	if err != nil {
		return nil, err
	}

	hash, err := addAndPrioritize(backend, magnet, configs)
	if err != nil || hash == "" {
		return nil, fmt.Errorf("failed to add torrent to embedded client: %w", err)
	}

	now := time.Now()
	var episodes []files.EpisodeStruct
	for _, node := range anilist.EpisodeList(details.mediaList, 1) {
		if node.TimeUntilAiring > 0 {
			continue
		}
		epName := fmt.Sprintf("%s - Episode %d", details.title, node.Episode)
		episodes = append(episodes, files.EpisodeStruct{
			AnimeID:         animeId,
			AnimeName:       details.title,
			EpisodeHash:     hash,
			EpisodeName:     epName,
			EpisodeNumber:   node.Episode,
			IsBatch:         true,
			DownloadDate:    now,
			ManuallyManaged: true,
		})
	}

	if len(episodes) == 0 {
		return nil, fmt.Errorf("no aired episodes found for anime %d", animeId)
	}

	return episodes, nil
}

// ManualDownloadEpisode downloads a specific episode manually (called from API).
// Returns the saved EpisodeStruct with ManuallyManaged=true on success.
func ManualDownloadEpisode(fm FileManagerInterface, backend torrents.TorrentBackend, animeId int, episodeNumber int, configs *files.Config, customQuery string) (files.EpisodeStruct, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return files.EpisodeStruct{}, err
	}

	details, err := resolveAnimeDetails(fm, animeId, configs.AnilistUsernames)
	if err != nil {
		return files.EpisodeStruct{}, err
	}

	targetNode := findEpisodeNode(details.mediaList, episodeNumber)
	if targetNode == nil {
		return files.EpisodeStruct{}, fmt.Errorf("episode %d not found for anime %d", episodeNumber, animeId)
	}

	epName := fmt.Sprintf("%s - Episode %d", details.title, targetNode.Episode)

	// Checado antes da busca no Nyaa para o handler receber ErrInsufficientDiskSpace em vez do
	// "falhou apos N tentativas" genrico (que viraria 500 em vez de 409).
	if err := checkDiskSpace(configs); err != nil {
		return files.EpisodeStruct{}, err
	}

	results := searchNyaaForSingleEpisode(*targetNode, details.mediaList.Media.Title, nil, anilist.MediaRelations{}, customQuery, anilist.LastAiredEpisode(details.mediaList))
	var magnets []string
	for _, result := range results {
		magnets = append(magnets, result.MagnetLink)
	}

	if len(magnets) == 0 {
		return files.EpisodeStruct{}, fmt.Errorf("no torrents found for episode %d", targetNode.Episode)
	}

	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))
	var hash string
	for i := range maxAttempts {
		h, err := addAndPrioritize(backend, magnets[i], configs)
		if err == nil && h != "" {
			hash = h
			break
		}
	}

	if hash == "" {
		return files.EpisodeStruct{}, fmt.Errorf("failed to download episode after %d attempts", maxAttempts)
	}

	return files.EpisodeStruct{
		AnimeID:         animeId,
		AnimeName:       details.title,
		EpisodeHash:     hash,
		EpisodeName:     epName,
		EpisodeNumber:   targetNode.Episode,
		DownloadDate:    time.Now(),
		ManuallyManaged: true,
	}, nil
}
