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
	"time"
)

type handleEpisodesData struct {
	savedEpisodes   []files.EpisodeStruct
	keysToDelete    []files.EpisodeKey
	checkedEpisodes []files.EpisodeKey
	newEpisodes     []files.EpisodeStruct
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

	downloadedEpisodesOfAnime := 0
	episodes := anilist.EpisodeList(anime, firstEpisodeToConsider(anime, savedEpisodes))
	keepSet := buildWatchedKeepSet(configs.WatchedEpisodesToKeep, anime.Media.Id, episodes, savedEpisodesMap, anime.Progress)
	var episodesToDownload []anilist.AiringNode

	totalEpisodes := 0
	if anime.Media.Episodes != nil {
		totalEpisodes = *anime.Media.Episodes
	}

	// A decisao batch-ou-um-a-um e tomada ANTES do loop, e e isso que a mantem pequena.
	willBatch := willBatchAnime(configs, anime)

	// Em batch o limite por anime nao se aplica: um batch e um torrent so, entao limitar os
	// registros nao limitaria os bytes nem os arquivos que vao para a biblioteca — so produziria
	// uma contagem mentirosa em episodes.json.
	maxEpisodes := configs.MaxEpisodesPerAnime
	if willBatch {
		maxEpisodes = len(episodes) + 1
	}

	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: ep.Episode}
		result.checkedEpisodes = append(result.checkedEpisodes, key)

		isInTorrents := episodeInTorrents(savedEpisodesFullMap[key].EpisodeHash, torrentsHashSet)
		alreadySaved := savedEpisodesMap[key]

		shouldDownload, shouldDelete := checkEpisode(configs, maxEpisodes, ep, anime, alreadySaved, &downloadedEpisodesOfAnime, isInTorrents, keepSet[key])

		if shouldDownload && !blockedMap[key] {
			episodesToDownload = append(episodesToDownload, ep)
		} else if shouldDelete {
			if savedEp, ok := savedEpisodesFullMap[key]; !ok || !savedEp.ManuallyManaged {
				result.keysToDelete = append(result.keysToDelete, key)
			}
		}
	}

	magnetsForEpisodes := resolveSearchStrategy(configs, anime, animeTitle, episodesToDownload, customQuery, searcher)

	// Tampao: o limite foi lancado contando com um batch, mas a busca de batch pode ter voltado
	// vazia (ou o filtro de tamanho a esvaziou) e a Estrategia 3 assumiu — o que baixaria os N
	// episodios um a um, sem limite. Nesse caso o limite volta. A ordem do airingSchedule garante
	// que o corte preserva os episodios mais antigos.
	if willBatch && !resolvedIsBatch(magnetsForEpisodes, episodesToDownload) && len(episodesToDownload) > configs.MaxEpisodesPerAnime {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Int("max_episodes", configs.MaxEpisodesPerAnime).
			Msg("Batch search did not resolve; re-applying the per-anime episode limit")
		episodesToDownload = episodesToDownload[:configs.MaxEpisodesPerAnime]
	}

	for _, ep := range episodesToDownload {
		epName := fmt.Sprintf("%s - Episode %d", animeTitle, ep.Episode)

		resolved := magnetsForEpisodes[ep.Episode]
		magnets := resolved.magnets
		skipSubfolder := resolved.skipSubfolder
		if resolved.overrideName != "" {
			epName = resolved.overrideName
		}

		if len(magnets) == 0 {
			singleResults := filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
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
				DownloadDate:       time.Now(),
			})
			// Completion is handled event-driven: the session's onComplete callback (and
			// the reconciliation pass as a safety net) enqueue JobOrganize, which hardlinks
			// the finished files into the library and fires the completion webhook.
		} else {
			reason := notifications.ReasonDownloadRejected
			if errors.Is(checkDiskSpace(configs), ErrInsufficientDiskSpace) {
				reason = notifications.ReasonNoDiskSpace
			}
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
}

// willBatchAnime reports whether this anime is eligible for a batch torrent: finalizado, nao
// filme e com contagem de episodios dentro de MaxBatchEpisodes.
//
// totalEpisodes desconhecido (Media.Episodes == nil) NAO e elegivel: sem saber o tamanho nao ha
// como afirmar que o pack cabe no teto — um RELEASING costuma vir com nil e um FINISHED com nil e
// dado incompleto da AniList, e nos dois casos o caminho conservador e continuar limitado.
//
// MaxBatchEpisodes <= 0 significa teto DESLIGADO, e por isso nao pode entrar na comparacao como
// numero: "totalEpisodes <= 0" seria sempre falso e desligar o teto teria o efeito oposto.
func willBatchAnime(configs *files.Config, anime anilist.MediaList) bool {
	if anime.Media.Status != anilist.MediaStatusFinished || isAnimeMovie(anime) {
		return false
	}
	if anime.Media.Episodes == nil || *anime.Media.Episodes <= 0 {
		return false
	}
	return configs.MaxBatchEpisodes <= 0 || *anime.Media.Episodes <= configs.MaxBatchEpisodes
}

func resolvedIsBatch(resolved map[int]resolvedMagnets, episodes []anilist.AiringNode) bool {
	for _, ep := range episodes {
		if resolved[ep.Episode].skipSubfolder {
			return true
		}
	}
	return false
}

// resolveSearchStrategy picks the best Nyaa search strategy for the anime and returns
// magnets keyed by episode NUMBER (unique inside one anime, which is all this map spans).
// Tries movie → batch → multi-episode in priority order.
func resolveSearchStrategy(configs *files.Config, anime anilist.MediaList, animeTitle string, episodesToDownload []anilist.AiringNode, customQuery string, searcher nyaaSearcher) map[int]resolvedMagnets {
	result := make(map[int]resolvedMagnets, len(episodesToDownload))
	animeIsMovie := isAnimeMovie(anime)

	if animeIsMovie {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Detected movie - searching for movie torrent")

		movieResult := filterSearchResults(searcher.searchMovie(anime.Media.Title, true, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)

		if len(episodesToDownload) == 0 && len(movieResult) > 0 {
			fakeEp := anilist.AiringNode{Episode: 1}
			episodesToDownload = append(episodesToDownload, fakeEp)
			logger.Logger.Info().
				Str("anime", animeTitle).
				Msg("Created fake episode for movie download")
		}

		if len(movieResult) > 0 {
			for _, ep := range episodesToDownload {
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
			return result
		}
	}

	if len(episodesToDownload) == 0 {
		return result
	}

	// O teto de max_batch_episodes entra aqui tambem: sem isso um anime de 720 episodios com o
	// limite por anime em 12 continuaria baixando o pack inteiro para 12 registros, que e
	// exatamente o defeito que a regra corrige.
	if willBatchAnime(configs, anime) && len(episodesToDownload) > 1 {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Detected finished anime - searching for batch torrent")

		batchResult := filterSearchResults(searcher.searchBatch(anime.Media.Title, anime.Media.Synonyms, customQuery), configs.MaxBatchTorrentSizeGB, configs.MinSeeders)

		if len(batchResult) > 0 {
			for _, ep := range episodesToDownload {
				result[ep.Episode] = resolvedMagnets{
					magnets:       []string{batchResult[0].MagnetLink},
					skipSubfolder: true,
					overrideName:  animeTitle,
				}
			}
			logger.Logger.Info().
				Str("anime", animeTitle).
				Str("torrent", batchResult[0].Name).
				Msg("Using batch torrent for finished anime")
			return result
		}
	}

	var eps []int
	for _, ep := range episodesToDownload {
		eps = append(eps, ep.Episode)
	}

	multipleResult := filterSearchResults(searcher.searchMultiple(anime.Media.Title, anime.Media.Synonyms, eps, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	if len(multipleResult) > 0 {
		byEpisode := make(map[int][]nyaa.TorrentResult)
		for _, tr := range multipleResult {
			if tr.Episode != nil {
				byEpisode[*tr.Episode] = append(byEpisode[*tr.Episode], tr)
			}
		}
		for _, ep := range episodesToDownload {
			if trs, ok := byEpisode[ep.Episode]; ok {
				var magnets []string
				for _, tr := range trs {
					magnets = append(magnets, tr.MagnetLink)
				}
				result[ep.Episode] = resolvedMagnets{magnets: magnets}
			}
		}
	}

	return result
}

func buildWatchedKeepSet(n int, animeID int, episodes []anilist.AiringNode, savedEpisodesMap map[files.EpisodeKey]bool, progress int) map[files.EpisodeKey]bool {
	if n <= 0 {
		return nil
	}
	var watchedSaved []files.EpisodeKey
	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: animeID, Episode: ep.Episode}
		if ep.Episode <= progress && savedEpisodesMap[key] {
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
func checkEpisode(configs *files.Config, maxEpisodes int, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched bool) (shouldDownload bool, shouldDelete bool) {
	epName := fmt.Sprintf("%s - Episode %d", getAnimeTitleSafe(anime), ep.Episode)

	if shouldSkipEpisode(configs, ep, anime, epName) {
		return false, alreadySaved && !keepWatched
	}

	if alreadySaved {
		return handleAlreadySavedEpisode(maxEpisodes, downloadedEpisodes, isInTorrents, epName)
	}

	if *downloadedEpisodes >= maxEpisodes {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", maxEpisodes).
			Msg("Skipping episode: max episodes per anime reached")
		return false, false
	}

	*downloadedEpisodes++
	return true, false
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

func handleAlreadySavedEpisode(maxEpisodes int, downloadedEpisodes *int, isInTorrents bool, epName string) (shouldDownload bool, shouldDelete bool) {
	if *downloadedEpisodes >= maxEpisodes {
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
