package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"fmt"
	"math"
	"time"
)

type handleEpisodesData struct {
	savedEpisodes   []files.EpisodeStruct
	keysToDelete    []files.EpisodeKey
	checkedEpisodes []files.EpisodeKey
	newEpisodes     []files.EpisodeStruct
}

// episodeSelection e o resultado do laco de selecao de um anime.
type episodeSelection struct {
	toDownload   []anilist.AiringNode
	keysToDelete []files.EpisodeKey
	checked      []files.EpisodeKey
	// downloaded e limitSkipped sao o par que o relatorio publica como "baixou N, sobraram M".
	// Vem do resultado FINAL de selectEpisodes de proposito: quando um pack foi escolhido a
	// selecao roda de novo com o limite levantado, e ai limitSkipped e zero — que e o certo,
	// porque naquele anime o limite nao barrou nada.
	downloaded   int
	limitSkipped int
}

// selectEpisodes decide, para cada episodio da lista, se ele deve ser baixado ou apagado.
//
// E PURA: sem I/O e sem estado entre chamadas (o contador de episodios baixados e local), e por
// isso pode rodar duas vezes no mesmo passe — uma com o limite por anime valendo (que e a que
// produz as delecoes) e outra com ele levantado quando um pack foi escolhido. Ver decisions.md.
func selectEpisodes(
	configs *files.Config,
	maxEpisodes int,
	anime anilist.MediaList,
	episodes []anilist.AiringNode,
	savedEpisodesMap map[files.EpisodeKey]bool,
	savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct,
	torrentsHashSet map[string]bool,
	keepSet, blockedMap map[files.EpisodeKey]bool,
) episodeSelection {
	var sel episodeSelection
	downloadedEpisodesOfAnime := 0

	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: ep.Episode}
		sel.checked = append(sel.checked, key)

		savedEp := savedEpisodesFullMap[key]
		isInTorrents := episodeInTorrents(savedEp.EpisodeHash, torrentsHashSet)

		shouldDownload, shouldDelete, skipCode := checkEpisode(configs, maxEpisodes, ep, anime, savedEpisodesMap[key], &downloadedEpisodesOfAnime, isInTorrents, keepSet[key], savedEp.IsBatch)

		if skipCode == IssueMaxEpisodesPerAnime {
			sel.limitSkipped++
		}

		if shouldDownload && !blockedMap[key] {
			sel.toDownload = append(sel.toDownload, ep)
		} else if shouldDelete && !savedEp.ManuallyManaged {
			sel.keysToDelete = append(sel.keysToDelete, key)
		}
	}

	sel.downloaded = downloadedEpisodesOfAnime

	return sel
}

// effectiveMax e o limite por anime aplicado ao passe. max_episodes_per_anime == 0 significa SEM
// TETO, e "sem teto" e expresso como len(episodes)+1 — o mesmo truque que o caminho de pack ja
// usava para levantar o limite, agora tambem para o teto desligado.
func effectiveMax(configs *files.Config, episodes []anilist.AiringNode) int {
	if configs.MaxEpisodesPerAnime <= 0 {
		return len(episodes) + 1
	}
	return configs.MaxEpisodesPerAnime
}

// windowEnd e o ultimo episodio que os packs precisam cobrir neste ciclo: o outro lado de
// max_episodes_per_anime. Com o teto desligado a janela e aberta, o que e caro de proposito (os
// freios passam a ser max_batch_torrent_size_gb, por torrent, e checkDiskSpace).
func windowEnd(configs *files.Config, firstPending int) int {
	if configs.MaxEpisodesPerAnime <= 0 {
		return math.MaxInt
	}
	return firstPending + configs.MaxEpisodesPerAnime - 1
}

func processAnimeEpisodes(
	configs *files.Config,
	backend torrents.TorrentBackend,
	anime anilist.MediaList,
	dlTorrents []torrents.TorrentInfo,
	savedEpisodes []files.EpisodeStruct,
	blockedMap map[files.EpisodeKey]bool,
	customQuery string,
	searcher nyaaSearcher,
) animeProcessResult {
	var result animeProcessResult
	animeTitle := getAnimeTitleSafe(anime)
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Processing anime episodes")

	torrentsHashSet := buildTorrentsHashSet(dlTorrents)
	savedEpisodesMap := buildSavedEpisodesMap(savedEpisodes)
	savedEpisodesFullMap := buildSavedEpisodesFullMap(savedEpisodes)

	episodes := anilist.EpisodeList(anime, firstEpisodeToConsider(anime, savedEpisodes))
	keepSet := buildWatchedKeepSet(configs.WatchedEpisodesToKeep, anime.Media.Id, episodes, savedEpisodesFullMap, anime.Progress)

	totalEpisodes := mediaTotalEpisodes(anime)

	// A primeira selecao e a que vale quando nao ha pack, e e ela que produz as delecoes por
	// limite: com o limite levantado por palpite, handleAlreadySavedEpisode nunca disparava e
	// keysToDelete vinha vazio para todo mundo.
	sel := selectEpisodes(configs, effectiveMax(configs, episodes), anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)

	var magnetsForEpisodes map[int]resolvedMagnets
	// batchSkipped e o porque de max_episodes_per_anime estar valendo neste anime. Fica fora do
	// if porque o relatorio o publica la embaixo, depois de sel ter sido possivelmente refeito.
	batchSkipped := ""

	if isAnimeMovie(anime) {
		sel.toDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, sel.toDownload, customQuery, searcher)
	}

	if magnetsForEpisodes == nil && len(sel.toDownload) > 0 {
		packs, singles, packStats := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload), customQuery))

		// Elegibilidade a pack: nao e filme, tem mais de um episodio pendente e a busca FILTRADA
		// devolveu pack que cobre a janela. Nada disso e metadado do AniList — e o torrent que
		// esta la que decide (ver decisions.md).
		if !isAnimeMovie(anime) && len(sel.toDownload) > 1 {
			firstPending := sel.toDownload[0].Episode
			batches := pickBatches(packs, firstPending, windowEnd(configs, firstPending))
			switch {
			case len(batches) > 0:
				logger.Logger.Info().
					Str("anime", animeTitle).
					Int("packs", len(batches)).
					Strs("torrents", batchNames(batches)).
					Msg("Using batch torrents to cover the pending window")

				// Segunda selecao, com o limite levantado: os registros de episodes.json precisam
				// cobrir o pack INTEIRO, senao a contagem mente e a poda apagaria o que o pack
				// acabou de trazer.
				sel = selectEpisodes(configs, len(episodes)+1, anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)
				sel.toDownload, magnetsForEpisodes = assignBatches(animeTitle, totalEpisodes, sel.toDownload, batches)
			case packStats.Input == 0:
				batchSkipped = BatchSkippedNoResult
			case len(packs) == 0:
				batchSkipped = BatchSkippedAboveSizeLimit
			default:
				batchSkipped = BatchSkippedNoCoverage
			}
		}

		if magnetsForEpisodes == nil {
			magnetsForEpisodes = magnetsByEpisode(singles, sel.toDownload)
		}
	}

	result.checkedEpisodes = sel.checked
	result.keysToDelete = sel.keysToDelete
	episodesToDownload := sel.toDownload

	// O limite so vira linha do relatorio quando ele de fato barrou algo. Quando um pack foi
	// escolhido, a segunda selecao roda com o limite levantado e limitSkipped e zero — que e o
	// certo: naquele anime o limite nao impediu nada.
	if sel.limitSkipped > 0 {
		result.issues = append(result.issues, Issue{
			AnimeID:      anime.Media.Id,
			AnimeName:    animeTitle,
			Code:         IssueMaxEpisodesPerAnime,
			Downloaded:   sel.downloaded,
			Pending:      sel.limitSkipped,
			BatchSkipped: batchSkipped,
		})
	}

	// Tamanho da serie, so para a busca decidir o zero-padding da query do episodio.
	seriesLength := anilist.LastAiredEpisode(anime)

	for _, ep := range episodesToDownload {
		epName := fmt.Sprintf("%s - Episode %d", animeTitle, ep.Episode)

		resolved := magnetsForEpisodes[ep.Episode]
		magnets := resolved.magnets
		skipSubfolder := resolved.skipSubfolder
		if resolved.overrideName != "" {
			epName = resolved.overrideName
		}

		// ponytail: o dropStats do fallback por episodio e a UNICA fonte da cascata de problemas —
		// nem partitionSearchResults nem resolveMovie alimentam o relatorio. E ele quem da a
		// ultima palavra sobre este episodio: se achou magnet nao houve problema, se nao achou e o
		// corte dele que descreve por que. Se um dia o pack precisar do proprio codigo, o caminho
		// e passar o packStats para ca em vez de sobrescrever este.
		var searchStats dropStats
		if len(magnets) == 0 {
			var singleResults []nyaa.TorrentResult
			singleResults, searchStats = filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
			for _, tr := range singleResults {
				magnets = append(magnets, tr.MagnetLink)
			}
		}

		// Sem magnet nao ha o que tentar: avisar "iniciando download" aqui mandava um push
		// falso a cada passada do loop enquanto o episodio nao aparecesse no Nyaa, e ainda
		// fazia attemptDownloadWithRetries logar "falhou apos todas as tentativas" com zero
		// tentativas.
		if len(magnets) == 0 {
			logger.Logger.Warn().
				Str("episode", epName).
				Msg("No torrent found for episode")
			result.issues = append(result.issues, searchIssue(anime.Media.Id, animeTitle, ep.Episode, searchStats, configs))
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, notifications.ReasonNotFound)
			continue
		}

		notifications.Notify(configs, notifications.NewEpisode, animeTitle, ep.Episode, "")

		hash := attemptDownloadWithRetries(configs, backend, magnets, epName)

		if hash != "" {
			result.newEpisodes = append(result.newEpisodes, files.EpisodeStruct{
				AnimeID:            anime.Media.Id,
				AnimeTotalEpisodes: totalEpisodes,
				AnimeName:          animeTitle,
				EpisodeHash:        hash,
				EpisodeName:        epName,
				EpisodeNumber:      ep.Episode,
				IsBatch:            skipSubfolder,
				BatchStart:         resolved.batchStart,
				BatchEnd:           resolved.batchEnd,
				DownloadDate:       time.Now(),
			})
			// Completion is handled event-driven: the session's onComplete callback (and
			// the reconciliation pass as a safety net) enqueue JobOrganize, which hardlinks
			// the finished files into the library and fires the completion webhook.
		} else {
			reason := notifications.ReasonDownloadRejected
			issue := Issue{
				AnimeID:    anime.Media.Id,
				AnimeName:  animeTitle,
				Episodes:   []int{ep.Episode},
				Code:       IssueTorrentRejected,
				Candidates: len(magnets),
			}
			if errors.Is(checkDiskSpace(configs), ErrInsufficientDiskSpace) {
				reason = notifications.ReasonNoDiskSpace
				issue.Code = IssueDiskFull
				// Disco cheio nao e sobre os magnets: nenhum foi tentado (attemptDownloadWithRetries
				// sai antes do primeiro Add), entao "N candidatos" seria numero sem significado.
				issue.Candidates = 0
			}
			result.issues = append(result.issues, issue)
			// O batch de notificacoes (BatchWindowSeconds) junta os N episodios do passe numa
			// mensagem so, entao disco cheio nao vira enxurrada.
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, reason)
		}
	}

	return result
}

type resolvedMagnets struct {
	magnets       []string
	skipSubfolder bool
	overrideName  string
	// batchStart/batchEnd: a faixa do pack lida do nome do torrent, 0 quando desconhecida. Vai
	// para o registro salvo porque a tela nao consegue reconstrui-la depois — os episodios ja
	// assistidos nao viram registro, e o min/max dos que viram mente sobre o pack.
	batchStart int
	batchEnd   int
}

// resolveMovie e o caminho de filme, inalterado: quando o filme e achado, todo episodio pendente
// (ou um episodio sintetico, se nao havia nenhum) recebe o magnet dele. Devolve (episodios, nil)
// quando nao ha filme, e o fluxo cai na busca por anime — que e o fallback que sempre existiu.
func resolveMovie(configs *files.Config, anime anilist.MediaList, animeTitle string, episodes []anilist.AiringNode, customQuery string, searcher nyaaSearcher) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Detected movie - searching for movie torrent")

	movieResult, _ := filterSearchResults(searcher.searchMovie(anime.Media.Title, true, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	if len(movieResult) == 0 {
		return episodes, nil
	}

	if len(episodes) == 0 {
		episodes = append(episodes, anilist.AiringNode{Episode: 1})
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Created fake episode for movie download")
	}

	result := make(map[int]resolvedMagnets, len(episodes))
	for _, ep := range episodes {
		result[ep.Episode] = resolvedMagnets{
			magnets:       []string{movieResult[0].MagnetLink},
			skipSubfolder: true,
			overrideName:  animeTitle,
		}
	}
	logger.Logger.Info().
		Str("anime", animeTitle).
		Str("torrent", movieResult[0].Name).
		Msg("Using movie torrent")
	return episodes, result
}

// mediaTotalEpisodes e a contagem que o AniList declara, 0 quando ela nao existe (serie em
// exibicao sem total anunciado).
func mediaTotalEpisodes(anime anilist.MediaList) int {
	if anime.Media.Episodes == nil {
		return 0
	}
	return *anime.Media.Episodes
}

// assignBatches da a cada episodio o magnet do SEU pack e devolve so os episodios cobertos.
//
// O truncamento e obrigatorio: sem ele, os episodios fora da cobertura cairiam em episodio solto e
// em One Piece o fallback nao acharia nada, disparando DownloadFailed/ReasonNotFound para ~1000
// episodios por passada. A lista vem da agenda em ordem crescente e os packs escolhidos sao
// contiguos a partir do primeiro pendente, entao o corte e sempre de prefixo.
//
// O overrideName leva a faixa quando ela e conhecida: dois packs do mesmo anime no mesmo ciclo
// apareceriam com nomes identicos na tela de downloads. O nome da PASTA da biblioteca nao muda —
// Organize usa AnimeName, nunca EpisodeName.
//
// totalEpisodes e a faixa registrada para pack SEM faixa no nome ("(Season 1+OVA) [Batch]"): ele
// foi escolhido como pack completo — e por isso que coveringBatch o deixa cobrir qualquer
// episodio —, entao a faixa dele e 1..total. 0 (total desconhecido) deixa a faixa zerada e quem
// exibe cai no min/max dos registros.
func assignBatches(animeTitle string, totalEpisodes int, episodes []anilist.AiringNode, batches []nyaa.TorrentResult) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	result := make(map[int]resolvedMagnets, len(episodes))
	var covered []anilist.AiringNode

	for _, ep := range episodes {
		batch := coveringBatch(batches, ep.Episode)
		if batch == nil {
			break
		}

		name := animeTitle
		info := nyaa.ExtractBatchInfo(batch.Name)
		if info.EndEpisode > 0 {
			name = fmt.Sprintf("%s %d-%d", animeTitle, info.StartEpisode, info.EndEpisode)
		} else if totalEpisodes > 0 {
			info.StartEpisode, info.EndEpisode = 1, totalEpisodes
		}

		result[ep.Episode] = resolvedMagnets{
			magnets:       []string{batch.MagnetLink},
			skipSubfolder: true,
			overrideName:  name,
			batchStart:    info.StartEpisode,
			batchEnd:      info.EndEpisode,
		}
		covered = append(covered, ep)
	}

	return covered, result
}

// magnetsByEpisode e o caminho de episodio solto: cada episodio recebe os magnets das linhas que
// casaram com o SEU numero, na ordem de qualidade.
func magnetsByEpisode(singles []nyaa.TorrentResult, episodes []anilist.AiringNode) map[int]resolvedMagnets {
	byEpisode := make(map[int][]nyaa.TorrentResult, len(singles))
	for _, tr := range singles {
		byEpisode[*tr.Episode] = append(byEpisode[*tr.Episode], tr)
	}

	result := make(map[int]resolvedMagnets, len(episodes))
	for _, ep := range episodes {
		trs, ok := byEpisode[ep.Episode]
		if !ok {
			continue
		}
		magnets := make([]string, 0, len(trs))
		for _, tr := range trs {
			magnets = append(magnets, tr.MagnetLink)
		}
		result[ep.Episode] = resolvedMagnets{magnets: magnets}
	}
	return result
}

// batchNames e so para o log: qual pack cobriu qual faixa e a primeira coisa que se quer saber ao
// auditar uma escolha de pack.
func batchNames(batches []nyaa.TorrentResult) []string {
	out := make([]string, 0, len(batches))
	for _, b := range batches {
		out = append(out, b.Name)
	}
	return out
}

// partitionSearchResults separa a lista unica de ScrapNyaaForAnime em packs e episodios, e aplica
// a cada uma o SEU teto de tamanho: os dois tetos sao independentes (um pack de 40 GiB continua
// valido com max_episode_torrent_size_gb = 1.5). A ordem relativa vinda de SortTorrentResults e
// preservada, entao cada lista sai ordenada por qualidade.
//
// packStats descreve o que o filtro fez com as linhas de PACK, e so com elas: e ele que responde
// se "nao houve batch" foi porque a busca nao devolveu pack nenhum ou porque todos foram cortados
// pelo teto. O relatorio usa isso como campo de detalhe do limite por anime.
func partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) ([]nyaa.TorrentResult, []nyaa.TorrentResult, dropStats) {
	var packs, singles []nyaa.TorrentResult
	for _, tr := range results {
		switch {
		case tr.IsBatch:
			packs = append(packs, tr)
		case tr.Episode != nil:
			singles = append(singles, tr)
		}
	}
	filteredPacks, packStats := filterSearchResults(packs, configs.MaxBatchTorrentSizeGB, configs.MinSeeders)
	filteredSingles, _ := filterSearchResults(singles, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	return filteredPacks, filteredSingles, packStats
}

// pickBatches devolve o minimo de packs que cobre [firstPending, windowEnd], em ordem.
//
// results ja vem ordenado por SortTorrentResults e ja filtrado por tamanho e seeders, entao "o
// primeiro que cobre o cursor" e "o melhor que cobre o cursor" — e por isso a escolha de pack
// deixou de ser decidida por metadado do AniList e passou a ser decidida pelo resultado da busca.
//
// Termina sempre: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente a cada volta e nenhum pack pode ser escolhido duas vezes.
func pickBatches(results []nyaa.TorrentResult, firstPending, windowEnd int) []nyaa.TorrentResult {
	var picked []nyaa.TorrentResult
	for cursor := firstPending; cursor <= windowEnd; {
		next := coveringBatch(results, cursor)
		if next == nil {
			break
		}
		picked = append(picked, *next)
		info := nyaa.ExtractBatchInfo(next.Name)
		if info.EndEpisode == 0 {
			break // faixa desconhecida vale como pack completo: cobre o resto da janela
		}
		cursor = info.EndEpisode + 1
	}
	return picked
}

// coveringBatch devolve o primeiro pack da lista que cobre o episodio. Pack sem faixa no nome
// (EndEpisode == 0) conta como completo.
func coveringBatch(results []nyaa.TorrentResult, episode int) *nyaa.TorrentResult {
	for i := range results {
		info := nyaa.ExtractBatchInfo(results[i].Name)
		if info.EndEpisode == 0 || (info.StartEpisode <= episode && episode <= info.EndEpisode) {
			return &results[i]
		}
	}
	return nil
}

// episodeNumbers e a lista de numeros que a busca por anime usa para filtrar linha de episodio.
func episodeNumbers(episodes []anilist.AiringNode) []int {
	eps := make([]int, 0, len(episodes))
	for _, ep := range episodes {
		eps = append(eps, ep.Episode)
	}
	return eps
}

func buildWatchedKeepSet(n int, animeID int, episodes []anilist.AiringNode, savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct, progress int) map[files.EpisodeKey]bool {
	if n <= 0 {
		return nil
	}
	var watchedSaved []files.EpisodeKey
	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: animeID, Episode: ep.Episode}
		saved, ok := savedEpisodesFullMap[key]
		// Registro de pack fica FORA do keep-set: manter 3 dos 100 de um pack nao guarda 3
		// episodios, guarda o pack inteiro — allEpisodesInDeleteSet daria false, o torrent ficaria
		// no disco, o espaco nao voltaria e o pack seguinte nunca viria. O keep-set nao tem a
		// granularidade que promete dentro de um pack.
		if ep.Episode <= progress && ok && !saved.IsBatch {
			watchedSaved = append(watchedSaved, key)
		}
	}
	keepSet := make(map[files.EpisodeKey]bool)
	start := len(watchedSaved) - n
	if start < 0 {
		start = 0
	}
	for _, key := range watchedSaved[start:] {
		keepSet[key] = true
	}
	return keepSet
}

// firstEpisodeToConsider e onde a lista de episodios de um anime comeca.
//
// Regra: progresso + 1 — o episodio seguinte ao ultimo assistido. Anime avulso nao tem entrada de
// lista e portanto tem progresso 0, o que faz a lista dele comecar no 1 sem precisar de flag.
//
// Ela recua para o menor episodio JA SALVO quando este e anterior a isso, porque
// watched_episodes_to_keep e a poda de assistidos so funcionam sobre episodios que aparecem na
// lista: um episodio salvo fora dela nunca seria "checado" e cairia direto em
// identifyEpisodesNotInWatching, ignorando o "quantos assistidos manter".
func firstEpisodeToConsider(anime anilist.MediaList, savedEpisodes []files.EpisodeStruct) int {
	first := anime.Progress + 1
	for _, ep := range savedEpisodes {
		if ep.AnimeID == anime.Media.Id && ep.EpisodeNumber > 0 && ep.EpisodeNumber < first {
			first = ep.EpisodeNumber
		}
	}
	return first
}

// checkEpisode decide se um episodio deve ser baixado ou apagado. maxEpisodes e o limite
// EFETIVO por anime, calculado pelo chamador: em batch ele vem ilimitado (ver processAnimeEpisodes).
//
// skipCode e o motivo do skip QUANDO ele entra no relatorio da ultima verificacao, e "" no resto.
// A condicao fica aqui, e nao no chamador que tem o mesmo ponteiro, para a regra nao existir em
// dois lugares e discordar depois.
func checkEpisode(configs *files.Config, maxEpisodes int, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched, isBatch bool) (shouldDownload bool, shouldDelete bool, skipCode string) {
	epName := fmt.Sprintf("%s - Episode %d", getAnimeTitleSafe(anime), ep.Episode)

	if shouldSkipEpisode(configs, ep, anime, epName) {
		return false, alreadySaved && !keepWatched, ""
	}

	if alreadySaved {
		download, del := handleAlreadySavedEpisode(maxEpisodes, downloadedEpisodes, isInTorrents, isBatch, epName)
		return download, del, ""
	}

	if *downloadedEpisodes >= maxEpisodes {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", maxEpisodes).
			Msg("Skipping episode: max episodes per anime reached")
		return false, false, IssueMaxEpisodesPerAnime
	}

	*downloadedEpisodes++
	return true, false, ""
}

func shouldSkipEpisode(configs *files.Config, ep anilist.AiringNode, anime anilist.MediaList, epName string) bool {
	if animeIsInExcludedList(anime, configs.ExcludedLists) {
		logger.Logger.Debug().
			Str("episode", epName).
			Msg("Skipping episode: in excluded list")
		return true
	}

	if ep.Episode <= anime.Progress {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("episode_number", ep.Episode).
			Int("progress", anime.Progress).
			Msg("Skipping episode: already watched")
		return true
	}

	if ep.TimeUntilAiring > 0 {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("time_until_airing", ep.TimeUntilAiring).
			Msg("Skipping episode: not yet aired")
		return true
	}

	return false
}

func handleAlreadySavedEpisode(maxEpisodes int, downloadedEpisodes *int, isInTorrents, isBatch bool, epName string) (shouldDownload bool, shouldDelete bool) {
	if *downloadedEpisodes >= maxEpisodes {
		if isBatch {
			// Mesma logica de "limitar registros de um pack nao limita bytes", agora no ponto de
			// DELECAO em vez do de levantamento: o pack e a unidade, apagar um registro nao libera
			// byte nenhum e o torrent continua no disco. Sem isso, o ciclo seguinte ao que trouxe o
			// pack apagaria os episodios acima do limite e o gate de espaco reabriria em seguida —
			// loop de baixar-e-apagar. Nao apaga, e tambem nao baixa mais nada deste anime.
			logger.Logger.Debug().
				Str("episode", epName).
				Msg("Skipping episode: batch record above the per-anime limit is never deleted")
			return false, false
		}
		logger.Logger.Info().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", maxEpisodes).
			Msg("Deleting episode: max episodes exceeded")
		return false, true
	}

	*downloadedEpisodes++

	if isInTorrents {
		logger.Logger.Debug().
			Str("episode", epName).
			Msg("Skipping episode: already downloaded")
		return false, false
	}

	logger.Logger.Info().
		Str("episode", epName).
		Msg("Redownloading episode: was missing from torrents")
	return true, false
}

func attemptDownloadWithRetries(configs *files.Config, backend torrents.TorrentBackend, magnets []string, fileName string) (hash string) {
	// Disco cheio: nem um magnet e tentado e nao ha retry — o magnets[i] nao e o problema, e
	// tentar 3 vezes so encheria o log.
	if err := checkDiskSpace(configs); err != nil {
		logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Skipping download: insufficient free disk space")
		return ""
	}

	maxAttempts := min(configs.EpisodeRetryLimit, len(magnets))

	for i := range maxAttempts {
		logger.Logger.Debug().
			Str("episode", fileName).
			Int("attempt", i+1).
			Int("max_attempts", configs.EpisodeRetryLimit).
			Msg("Attempting to download episode")

		h, err := backend.Add(magnets[i])
		if err != nil {
			logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Failed to add torrent to embedded client")
			continue
		}
		if h != "" {
			logger.Logger.Info().
				Str("episode", fileName).
				Str("hash", h).
				Msg("Successfully added episode to embedded torrent client")
			return h
		}
	}

	logger.Logger.Warn().
		Str("episode", fileName).
		Int("attempts", maxAttempts).
		Msg("Failed to download episode after all attempts")
	return ""
}

// RemoveEpisodesWithLinks removes the given episodes from the saved-episodes file and frees
// their disk space (library hardlink + seeding torrent, with the batch guard applied).
// Exposed for API handlers (manual delete / redownload / replace): it returns an error when the
// record could not actually leave the saved-episodes file, so the handler can answer 500 and
// abort instead of adding a new torrent that the stale record would shadow.
func RemoveEpisodesWithLinks(fm FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, keys []files.EpisodeKey) error {
	if len(keys) == 0 {
		return nil
	}
	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}
	return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, false)
}

// RemoveTorrentOptions configures RemoveTorrentWithEpisodes.
type RemoveTorrentOptions struct {
	// KeepData, when true, keeps both the library hardlink and the seeding copy (see
	// removeEpisodesAndLinks — they share an inode, so keeping only one frees no space).
	KeepData bool
	// Block, when true, blocks every episode in the group before removing its records, so the
	// automatic loop does not re-download it on the next pass.
	Block bool
}

// RemoveTorrentWithEpisodes removes a torrent by hash and every saved episode sharing that hash,
// as a single unit: the deletion boundary here is the torrent, not the episode, so a batch's
// episodes always leave together. Exposed for the manual "delete torrent" API handler.
//
// An orphan torrent (no saved episode matches the hash — added by hand, or its record already
// gone) is removed directly via backend.Remove and that call's error is returned as-is; Block is
// meaningless there since there is no episode id to block.
func RemoveTorrentWithEpisodes(
	fm FileManagerInterface,
	backend torrents.TorrentBackend,
	librarian files.Librarian,
	hash string,
	opts RemoveTorrentOptions,
) error {
	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}

	var group []files.EpisodeStruct
	for _, ep := range saved {
		if ep.EpisodeHash == hash {
			group = append(group, ep)
		}
	}

	if len(group) == 0 {
		return backend.Remove(hash, opts.KeepData)
	}

	if opts.Block {
		for _, ep := range group {
			if err := fm.BlockEpisode(ep.Key()); err != nil {
				logger.Logger.Warn().Err(err).Int("anime_id", ep.AnimeID).Int("episode", ep.EpisodeNumber).Msg("Failed to block episode before torrent removal")
			}
		}
	}

	keys := make([]files.EpisodeKey, 0, len(group))
	for _, ep := range group {
		keys = append(keys, ep.Key())
	}

	return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, opts.KeepData)
}

// deleteEpisodesByStatus apaga os episódios dos animes que TODAS as contas concordam em
// deletar. Quem decide isso é deletableMediaIDs (verification.go) — aqui só se aplica.
func deleteEpisodesByStatus(deletableMedia map[int]bool, fileManager FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, savedEpisodes []files.EpisodeStruct) {
	if len(deletableMedia) == 0 {
		return
	}

	logger.Logger.Debug().
		Msg("Running status-based episode deletion")

	var keysToDelete []files.EpisodeKey
	for _, ep := range savedEpisodes {
		if deletableMedia[ep.AnimeID] && !ep.ManuallyManaged {
			keysToDelete = append(keysToDelete, ep.Key())
		}
	}

	if len(keysToDelete) == 0 {
		logger.Logger.Debug().Msg("Status-based deletion: no matching episodes found")
		return
	}

	logger.Logger.Info().
		Int("count", len(keysToDelete)).
		Msg("Deleting episodes for animes with delete statuses")

	// Best-effort: a failure here must not abort the verification pass.
	if err := removeEpisodesAndLinks(fileManager, backend, librarian, keysToDelete, savedEpisodes, false); err != nil {
		logger.Logger.Warn().Err(err).Msg("Status-based deletion: failed to delete episodes from file")
	}
}

func handleSavedEpisodes(fileManager FileManagerInterface, configs *files.Config, backend torrents.TorrentBackend, librarian files.Librarian, data handleEpisodesData) {
	episodesNotInWatching := identifyEpisodesNotInWatching(data.savedEpisodes, data.checkedEpisodes)

	saveEpisodesToFile(fileManager, data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		allKeys := append(append([]files.EpisodeKey{}, data.keysToDelete...), episodesNotInWatching...)
		// Best-effort: a failure here must not abort the verification pass.
		if err := removeEpisodesAndLinks(fileManager, backend, librarian, allKeys, data.savedEpisodes, false); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to delete episodes from file")
		}
	}
}

// removeEpisodesAndLinks deletes episodes and frees their disk space by removing BOTH links:
// the library hardlink and the seeding copy (via the torrent). A torrent is only removed
// when every one of its saved episodes is being deleted (batch guard) — a torrent with
// surviving siblings is kept, and for batches its library files are only removed when the
// whole torrent goes (raw filenames can't be safely mapped to a single episode).
//
// Freeing disk space is best-effort (a failed hardlink/torrent removal is logged and skipped),
// but a failure to drop the records from the saved-episodes file is returned: the caller decides
// whether that is fatal (API handlers) or merely logged (the automatic loop).
//
// keepData, when true, skips the library-hardlink removal loop entirely and is passed through to
// backend.Remove. Library files and the seeding copy are the same inode (hardlinks), so keeping
// one but not the other frees no disk space — keep_data is honestly binary: both stay or both go.
func removeEpisodesAndLinks(fm FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, keysToDelete []files.EpisodeKey, savedEpisodes []files.EpisodeStruct, keepData bool) error {
	if len(keysToDelete) == 0 {
		return nil
	}
	deleteSet := make(map[files.EpisodeKey]bool, len(keysToDelete))
	for _, k := range keysToDelete {
		deleteSet[k] = true
	}

	byHash := make(map[string][]files.EpisodeStruct)
	for _, ep := range savedEpisodes {
		if ep.EpisodeHash != "" {
			byHash[ep.EpisodeHash] = append(byHash[ep.EpisodeHash], ep)
		}
	}

	// Remove library hardlinks (skipped when keepData: keeping the library copy while the
	// torrent is removed anyway does not double as "keep everything" since it's the same inode).
	if !keepData {
		for _, ep := range savedEpisodes {
			if !deleteSet[ep.Key()] {
				continue
			}
			removingTorrent := ep.EpisodeHash == "" || allEpisodesInDeleteSet(byHash[ep.EpisodeHash], deleteSet)
			if ep.IsBatch && !removingTorrent {
				// Keep batch library files while siblings survive (can't identify a single
				// episode's raw-named file safely). Freed only when the whole torrent goes.
				continue
			}
			for _, p := range ep.LibraryPaths {
				if err := librarian.RemoveFromLibrary(p); err != nil {
					logger.Logger.Warn().Err(err).Str("path", p).Msg("Failed to remove library hardlink")
				}
			}
		}
	}

	// Remove torrents (seeding copy) with no surviving siblings.
	removedHashes := make(map[string]bool)
	for _, ep := range savedEpisodes {
		if !deleteSet[ep.Key()] || ep.EpisodeHash == "" || removedHashes[ep.EpisodeHash] {
			continue
		}
		if allEpisodesInDeleteSet(byHash[ep.EpisodeHash], deleteSet) {
			if err := backend.Remove(ep.EpisodeHash, keepData); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", ep.EpisodeHash).Msg("Failed to remove torrent")
			} else {
				logger.Logger.Info().Str("hash", ep.EpisodeHash).Msg("Removed torrent (seeding copy)")
			}
			removedHashes[ep.EpisodeHash] = true
		}
	}

	if err := fm.DeleteEpisodesFromFile(keysToDelete); err != nil {
		return fmt.Errorf("failed to delete episodes from file: %w", err)
	}

	return nil
}

// allEpisodesInDeleteSet reports whether every episode in the group is in the delete set
// (i.e. no sibling survives, so the shared torrent can be removed).
func allEpisodesInDeleteSet(group []files.EpisodeStruct, deleteSet map[files.EpisodeKey]bool) bool {
	if len(group) == 0 {
		return true
	}
	for _, ep := range group {
		if !deleteSet[ep.Key()] {
			return false
		}
	}
	return true
}

func identifyEpisodesNotInWatching(savedEpisodes []files.EpisodeStruct, checkedEpisodes []files.EpisodeKey) []files.EpisodeKey {
	checkedMap := make(map[files.EpisodeKey]bool, len(checkedEpisodes))
	for _, k := range checkedEpisodes {
		checkedMap[k] = true
	}

	var episodesToDelete []files.EpisodeKey
	for _, savedEp := range savedEpisodes {
		if !checkedMap[savedEp.Key()] && !savedEp.ManuallyManaged {
			episodesToDelete = append(episodesToDelete, savedEp.Key())
		}
	}

	return episodesToDelete
}

// saveEpisodesToFile persists freshly downloaded episodes, merging over any existing record
// with the same EpisodeKey. FileManager.SaveEpisodesToFile dedupes by EpisodeKey and silently
// discards updates, which would leave a re-downloaded episode with its stale hash (breaking the
// JobOrganize join) and a stale EpisodeNumber (producing "Anime - E00.mkv" for records saved
// before EpisodeNumber existed). UpsertEpisodes alone would clobber ManuallyManaged, so the
// merge is done here: download metadata is refreshed, LibraryPaths is reset (the file on disk
// is a new one and must be organized again) and ManuallyManaged — a user flag — is preserved.
func saveEpisodesToFile(fileManager FileManagerInterface, newEpisodes []files.EpisodeStruct) {
	if len(newEpisodes) == 0 {
		return
	}

	existing, err := fileManager.LoadSavedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load saved episodes before merge, falling back to append-only save")
		if err := fileManager.SaveEpisodesToFile(newEpisodes); err != nil {
			logger.Logger.Warn().Err(err).Int("count", len(newEpisodes)).Msg("Failed to save episodes to file")
			return
		}
		logger.Logger.Info().Int("count", len(newEpisodes)).Msg("Saved episodes to file")
		return
	}

	existingByKey := make(map[files.EpisodeKey]files.EpisodeStruct, len(existing))
	for _, ep := range existing {
		existingByKey[ep.Key()] = ep
	}

	merged := make([]files.EpisodeStruct, 0, len(newEpisodes))
	for _, ep := range newEpisodes {
		merged = append(merged, mergeSavedEpisode(existingByKey[ep.Key()], ep))
	}

	if err := fileManager.UpsertEpisodes(merged); err != nil {
		logger.Logger.Warn().Err(err).Int("count", len(merged)).Msg("Failed to save episodes to file")
		return
	}
	logger.Logger.Info().Int("count", len(merged)).Msg("Saved episodes to file")
}

// mergeSavedEpisode merges a freshly downloaded record over the existing saved one (zero value
// when the episode is new). Every download field comes from the new record; only LibraryPaths
// (reset — the old hardlinks point at the previous release) and ManuallyManaged (a user flag the
// automatic loop must never clear) get special treatment.
func mergeSavedEpisode(existing, updated files.EpisodeStruct) files.EpisodeStruct {
	merged := updated
	merged.LibraryPaths = nil
	merged.ManuallyManaged = updated.ManuallyManaged || existing.ManuallyManaged
	return merged
}
