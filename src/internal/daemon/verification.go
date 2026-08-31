package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"context"
	"fmt"
	"sync"
	"time"
)

// animeProcessResult holds the per-anime outputs from processAnimeEpisodes.
type animeProcessResult struct {
	newEpisodes     []files.EpisodeStruct
	checkedEpisodes []files.EpisodeKey
	keysToDelete    []files.EpisodeKey
	// issues sao os motivos pelos quais este anime deixou de baixar algo. Vem pelo canal que ja
	// existe de proposito: um *passReport compartilhado entre as goroutines de
	// maxConcurrentAnimes precisaria de mutex, e o fan-in ja resolve isso de graca.
	issues []Issue
}

// maxConcurrentAnimes limits simultaneous Nyaa HTTP searches to avoid rate limiting.
const maxConcurrentAnimes = 5

// verificationMu serializa os passes. O passe le episodes.json UMA vez no comeco e so escreve no
// fim (Fase 3), entao dois passes simultaneos — o do loop e o do POST /check, ou dois /check —
// enxergam a mesma lista velha e baixam tudo de novo: foi assim que um anime ja coberto por um
// pack ganhou torrents avulsos dos mesmos episodios, e a escrita do segundo passe sobrescreveu os
// registros do pack. Passe concorrente e DESCARTADO, nao enfileirado: ele leria o mesmo estado
// que o passe em andamento ja esta corrigindo, e o proximo tick do loop cobre o que faltar.
var verificationMu sync.Mutex

func AnimeVerification(ctx context.Context, fileManager FileManagerInterface, state *State, jobQueue *JobQueue, backend torrents.TorrentBackend, librarian files.Librarian) {
	if !verificationMu.TryLock() {
		logger.Logger.Info().Msg("Verification already running; skipping this trigger")
		return
	}
	defer verificationMu.Unlock()

	configs, err := fileManager.LoadConfigs()
	if err != nil {
		logger.Logger.Error().Err(err).Stack().Msg("Failed to load configs")
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseConfig, err))
		return
	}

	if !isConfigComplete(configs) {
		logger.Logger.Warn().Msg("Missing required configuration, opening browser to config page")
		go func() {
			time.Sleep(500 * time.Millisecond)
			webUIURL := getWebUiURL()
			if err := OpenBrowser(webUIURL); err != nil {
				logger.Logger.Warn().Err(err).Msg("Failed to open browser to configuration page")
			}
		}()

		state.SetLastCheckError(fmt.Errorf("%w: falta a pasta da biblioteca", errCauseSetup))
		return
	}

	// A biblioteca e montada com hardlinks. O endpoint de save da config sonda isso, mas
	// configs escritos antes deste upgrade (ou direto no config.json pelo
	// docker/entrypoint.sh) nunca passaram por ele. Sem esta porta um filesystem sem
	// suporte a hardlink baixa alegremente enquanto todo JobOrganize morre, e a UI mostra
	// um daemon saudavel. Sondar aqui devolve a mesma mensagem acionavel do endpoint, e
	// aborta o passe: baixar o que nao da para organizar so enche o disco.
	if librarian != nil {
		if err := librarian.ProbePath(configs.CompletedAnimePath); err != nil {
			logger.Logger.Error().Err(err).
				Str("completed_anime_path", configs.CompletedAnimePath).
				Msg("Completed anime path failed the hardlink probe; skipping verification")
			state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseLibrary, err))
			return
		}
	}

	if backend == nil {
		logger.Logger.Error().Msg("Torrent backend not initialized; skipping verification")
		state.SetLastCheckError(fmt.Errorf("%w: não inicializado", errCauseTorrent))
		return
	}

	// Rede de segurança: main.go e o PUT /config já aplicam o limite, mas um config.json
	// editado à mão nunca passa por nenhum dos dois. É também o que faz da fila uma
	// reconciliação de verdade: todo SetMaxActiveDownloads roda um enforce, então um pause que
	// a rain recusou ou um `stopping` que ficou no meio do caminho se resolve no ciclo
	// seguinte. Roda ANTES do Ensure, quando ainda pode não haver sessão — o passo 0 do
	// enforce é quem trata isso (ver decisions.md #41).
	backend.SetMaxActiveDownloads(configs.MaxConcurrentDownloads)

	// Ensure the embedded torrent session exists for the current save path (created lazily,
	// recreated if the save path changed or if the download folder was swapped underneath).
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to initialize embedded torrent session")
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseTorrent, err))
		return
	}
	// Latched by Ensure, here or in an earlier manual-download call: the folder the session
	// was bound to is gone, so the records pointing into it must go too.
	if backend.ConsumeRootSwap() {
		clearLibraryPathsAfterRootSwap(fileManager, configs.CompletedAnimePath)
	}

	// downloadedTorrents is an in-memory snapshot of the embedded client (cheap, no I/O).
	downloadedTorrents := backend.List()

	// Mesma logica de aborto: com os AnimeID ainda no formato antigo (id de entrada) nada em
	// disco casa com a AniList, e um passe nesse estado rebaixaria a biblioteca inteira.
	if err := MigrateAnimeIDsToMedia(fileManager); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to migrate anime IDs to AniList media IDs; skipping verification")
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseStorage, err))
		return
	}

	// Phase 1: fetch all independent data sources in parallel.
	var (
		anilistResponse  *anilist.AniListResponse
		savedEpisodes    []files.EpisodeStruct
		blockedEpisodes  []files.EpisodeKey
		// inDeleteStatus[username][mediaId] — quais animes cada conta tem em algum status de
		// deleção. Uma conta cuja busca falhou fica ausente do mapa, e ausente nunca concorda.
		inDeleteStatus map[string]map[int]bool

		errAnilist  error
		errEpisodes error
	)

	// Leitura local e barata, feita antes do fan-out porque searchAnilist depende dela. Uma
	// falha aqui nao aborta o passe: sem o arquivo o daemon so deixa de cobrir os avulsos.
	standaloneIDs, err := fileManager.LoadStandaloneAnimes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load standalone animes, continuing with the AniList lists only")
		standaloneIDs = nil
	}

	var fetchWg sync.WaitGroup

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		anilistResponse, errAnilist = searchAnilist(fileManager, configs, standaloneIDs)
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		var e error
		savedEpisodes, e = fileManager.LoadSavedEpisodes()
		if e != nil {
			logger.Logger.Error().Err(e).Stack().Msg("Failed to load saved episodes")
			errEpisodes = e
		}
	}()

	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		var e error
		blockedEpisodes, e = fileManager.LoadBlockedEpisodes()
		if e != nil {
			logger.Logger.Warn().Err(e).Msg("Failed to load blocked episodes, continuing without block list")
			blockedEpisodes = []files.EpisodeKey{}
		}
	}()

	if len(configs.DeleteStatuses) > 0 {
		fetchWg.Add(1)
		go func() {
			defer fetchWg.Done()
			inDeleteStatus = make(map[string]map[int]bool, len(configs.AnilistUsernames))
			for _, username := range configs.AnilistUsernames {
				resp, e := anilist.GetAllCurrentAnime(username, configs.DeleteStatuses)
				if e != nil {
					logger.Logger.Warn().Err(e).Str("username", username).Msg("Failed to fetch AniList animes for delete statuses")
					// Conta sem resposta nao pode concordar com a deleção — ver deletableMediaIDs.
					continue
				}
				byMedia := make(map[int]bool, len(resp.Data.Page.MediaList))
				for _, ml := range resp.Data.Page.MediaList {
					byMedia[ml.Media.Id] = true
				}
				logger.Logger.Debug().
					Str("username", username).
					Int("animes_found", len(byMedia)).
					Msg("Fetched animes from Anilist for delete statuses")
				inDeleteStatus[username] = byMedia
			}
		}()
	}

	fetchWg.Wait()

	if errAnilist != nil {
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseAnilist, errAnilist))
		return
	}
	if errEpisodes != nil {
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseStorage, errEpisodes))
		return
	}

	// Reconciliation (durable safety net): enqueue JobOrganize for any completed torrent
	// whose episodes are not yet in the library. Covers completions missed while the daemon
	// was down and a save-path change. JobOrganize is idempotent, so re-runs are no-ops.
	reconcileLibrary(downloadedTorrents, savedEpisodes, jobQueue)

	blockedMap := make(map[files.EpisodeKey]bool, len(blockedEpisodes))
	for _, k := range blockedEpisodes {
		blockedMap[k] = true
	}

	animes := anilistResponse.Data.Page.MediaList

	// Eixo absoluto da serie de cada anime do passe E de cada anime_id com episodio em disco: e
	// o que permite reconhecer que um pack baixado sob outro cour ja cobre o episodio pendente
	// (coverage.go). Falha parcial nao aborta o passe — sem o offset o anime so nao adota nada.
	animeIDs := make([]int, 0, len(animes))
	for _, a := range animes {
		animeIDs = append(animeIDs, a.Media.Id)
	}
	seriesIndex := resolveSeriesIndex(animeIDs, savedEpisodes)

	// Regra de deleção por status: TODAS as contas que têm o anime precisam tê-lo em algum
	// status de deleção (não necessariamente o mesmo). A regra de download é a oposta —
	// basta UMA conta —, e ela já vem aplicada em searchAnilist pela união das listas.
	deletableMedia := deletableMediaIDs(configs, inDeleteStatus, savedEpisodes)

	var keysToDelete []files.EpisodeKey

	// Phase 2: process each anime concurrently, bounded by maxConcurrentAnimes.
	sem := make(chan struct{}, maxConcurrentAnimes)
	resultCh := make(chan animeProcessResult, len(animes))

	var animeWg sync.WaitGroup
	start := time.Now()

outer:
	for _, anime := range animes {
		select {
		case <-ctx.Done():
			break outer
		default:
		}

		// anime.Status é de UMA conta arbitrária (a que venceu o dedup), então não serve para
		// decidir nada — quem responde é a regra AND acima.
		if deletableMedia[anime.Media.Id] {
			continue
		}

		animeWg.Add(1)
		go func(a anilist.MediaList) {
			defer animeWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			select {
			case <-ctx.Done():
				return
			default:
			}

			resultCh <- processAnimeEpisodes(configs, backend, a, downloadedTorrents, savedEpisodes, seriesIndex, blockedMap, defaultNyaaSearcher())
		}(anime)
	}

	animeWg.Wait()
	close(resultCh)
	elapsed := time.Since(start)

	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []files.EpisodeKey
	var issues []Issue
	for r := range resultCh {
		newEpisodes = append(newEpisodes, r.newEpisodes...)
		checkedEpisodes = append(checkedEpisodes, r.checkedEpisodes...)
		keysToDelete = append(keysToDelete, r.keysToDelete...)
		issues = append(issues, r.issues...)
	}

	select {
	case <-ctx.Done():
		logger.Logger.Info().Msg("Verification cancelled")
		state.SetLastCheckError(nil)
		return
	default:
	}

	// Phase 3: sequential cleanup (file writes must not overlap).
	deleteEpisodesByStatus(deletableMedia, fileManager, backend, librarian, savedEpisodes)

	handleSavedEpisodes(fileManager, configs, backend, librarian, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    keysToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
	})

	state.SetLastCheck(time.Now())
	state.SetLastCheckError(nil)

	// DEPOIS do SetLastCheckError, nunca antes: ele limpa o relatorio (ver state.go). O
	// cancelamento acima tambem chama SetLastCheckError(nil) e retorna, entao passe interrompido
	// nao deixa relatorio — que e o certo, ele estava incompleto.
	problems, limits := aggregateIssues(issues)
	state.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   problems,
		Limits:     limits,
	})

	if err := fileManager.DeleteEmptyFolders(configs.CompletedAnimePath); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to delete empty folders")
	}

	avgTime := time.Duration(0)
	if len(checkedEpisodes) > 0 {
		avgTime = elapsed / time.Duration(len(checkedEpisodes))
	}

	logger.Logger.Info().
		Int("animes_checked", len(anilistResponse.Data.Page.MediaList)).
		Int("episodes_checked", len(checkedEpisodes)).
		Int("episodes_downloaded", len(newEpisodes)).
		Dur("total_time", elapsed).
		Dur("avg_time_per_episode", avgTime).
		Msg("Verification completed")
}

// clearLibraryPathsAfterRootSwap wipes the LibraryPaths of every episode after the download
// root was swapped, so reconcileLibrary enqueues them again and the library is rebuilt at the
// configured path once the redownloads finish.
//
// This is the documented exception to decision #29 (never clear LibraryPaths because a file
// is missing from disk). The rule exists so a per-file deletion by the user is not undone on
// the next pass, forever. A root swap is a different event: the whole folder the records
// point into is gone, the daemon is already redownloading its contents, and the detection is
// edge-triggered — Ensure reports the swap once, then rewrites the marker, so this runs a
// single time per swap and can never become the resurrection loop #29 guards against.
func clearLibraryPathsAfterRootSwap(fileManager FileManagerInterface, completedPath string) {
	saved, err := fileManager.LoadSavedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Root swap: failed to load saved episodes; library records left untouched")
		return
	}

	var stale []files.EpisodeStruct
	for _, ep := range saved {
		if len(ep.LibraryPaths) == 0 {
			continue
		}
		ep.LibraryPaths = nil
		stale = append(stale, ep)
	}
	if len(stale) == 0 {
		return
	}

	if err := fileManager.UpsertEpisodes(stale); err != nil {
		logger.Logger.Warn().Err(err).Msg("Root swap: failed to clear the stale library paths")
		return
	}
	logger.Logger.Warn().
		Int("episodes", len(stale)).
		Str("completed_anime_path", completedPath).
		Msg("Root swap: the library folder is gone, cleared the stale library links; episodes will be redownloaded and reorganized at the configured path")
}

// reconcileLibrary enqueues JobOrganize for completed torrents whose saved episodes have
// not yet been hardlinked into the library (empty LibraryPaths). Enqueue is deduped, so
// repeated passes are cheap.
func reconcileLibrary(downloaded []torrents.TorrentInfo, savedEpisodes []files.EpisodeStruct, jobQueue *JobQueue) {
	if jobQueue == nil {
		return
	}
	byHash := make(map[string][]files.EpisodeStruct)
	for _, ep := range savedEpisodes {
		if ep.EpisodeHash != "" {
			byHash[ep.EpisodeHash] = append(byHash[ep.EpisodeHash], ep)
		}
	}
	for _, t := range downloaded {
		if !t.Completed {
			continue
		}
		eps := byHash[t.Hash]
		if len(eps) == 0 {
			continue // orphan torrent with no episode record; nothing to organize
		}
		needs := false
		for _, ep := range eps {
			if len(ep.LibraryPaths) == 0 {
				needs = true
				break
			}
		}
		if needs {
			jobQueue.EnqueueOrganize(t.Hash)
		}
	}
}

// deletableMediaIDs decide quais animes podem ter os episódios apagados por status.
//
// Regra: TODA conta que tem o anime na lista precisa tê-lo em algum status de deleção — os
// statuses não precisam ser o mesmo (DROPPED numa e COMPLETED noutra apaga). Uma conta que
// não acompanha o anime não participa da votação; uma que o tem em status neutro (PLANNING,
// por exemplo) veta, senão apagaríamos episódios que outra conta ainda pretende assistir.
//
// Só animes COM episódio em disco são avaliados: a resposta não muda nada para os demais, e é
// isso que mantém as consultas de desempate raras — um anime deletável some do disco no mesmo
// passe e nunca mais volta a ser candidato.
func deletableMediaIDs(configs *files.Config, inDeleteStatus map[string]map[int]bool, savedEpisodes []files.EpisodeStruct) map[int]bool {
	if len(configs.DeleteStatuses) == 0 || len(inDeleteStatus) == 0 {
		return nil
	}

	onDisk := make(map[int]bool)
	for _, ep := range savedEpisodes {
		if ep.AnimeID != 0 && !ep.ManuallyManaged {
			onDisk[ep.AnimeID] = true
		}
	}

	candidates := make(map[int]bool)
	for _, byMedia := range inDeleteStatus {
		for mediaID := range byMedia {
			if onDisk[mediaID] {
				candidates[mediaID] = true
			}
		}
	}

	deletable := make(map[int]bool, len(candidates))
	for mediaID := range candidates {
		if allAccountsAgreeOnDelete(configs, inDeleteStatus, mediaID) {
			deletable[mediaID] = true
		}
	}
	return deletable
}

func allAccountsAgreeOnDelete(configs *files.Config, inDeleteStatus map[string]map[int]bool, mediaID int) bool {
	for _, username := range configs.AnilistUsernames {
		byMedia, fetched := inDeleteStatus[username]
		if !fetched {
			// A busca desta conta falhou: sem a opinião dela não há unanimidade.
			logger.Logger.Debug().Str("username", username).Int("media_id", mediaID).
				Msg("Skipping status deletion: account list unavailable")
			return false
		}
		if byMedia[mediaID] {
			continue
		}

		// A conta não listou o anime entre os deletáveis. Pode ser que ela não o acompanhe
		// (não vota) ou que o tenha em status neutro (veta) — só uma consulta distingue.
		status, tracked, err := anilist.GetMediaListStatus(username, mediaID)
		switch {
		case err != nil:
			logger.Logger.Warn().Err(err).Str("username", username).Int("media_id", mediaID).
				Msg("Skipping status deletion: could not resolve the account's status")
			return false
		case !tracked:
			continue // conta não acompanha este anime, não veta
		case isInDeleteStatuses(configs.DeleteStatuses, status):
			continue // status de deleção diferente do que a busca por lista trouxe
		default:
			logger.Logger.Debug().Str("username", username).Int("media_id", mediaID).
				Str("status", string(status)).Msg("Status deletion vetoed by account")
			return false
		}
	}
	return true
}

// searchAnilist monta o universo de animes do passe: a uniao das listas das contas
// configuradas mais os animes avulsos (§ standalone).
//
// fileManager entra aqui, e nao so a config, por causa da remocao automatica: um avulso que
// depois apareceu numa lista da AniList sai do arquivo, e a unica hora em que se sabe disso e
// exatamente aqui, com a lista mesclada na mao.
func searchAnilist(fileManager FileManagerInterface, configs *files.Config, standaloneIDs []int) (*anilist.AniListResponse, error) {
	// Sem conta da AniList NAO e erro: o passe ainda tem trabalho a fazer (os avulsos), e
	// abortar aqui faria a feature nunca rodar numa instalacao sem lista. Sem biblioteca e:
	// nao ha para onde baixar.
	if configs.DownloadPath() == "" {
		err := fmt.Errorf("missing required configuration: completed anime path")
		logger.Logger.Error().
			Err(err).
			Str("download_path", configs.DownloadPath()).
			Msg("Missing required configuration: completed anime path")
		return nil, err
	}

	merged := &anilist.AniListResponse{}
	var lastErr error
	for _, username := range configs.AnilistUsernames {
		// Fetch customLists first via a minimal query (before the complex query that may null it out).
		clMap := anilist.GetCustomListsMap(username, configs.DownloadStatuses, anilist.PriorityCritical)

		resp, err := anilist.GetAllCurrentAnime(username, configs.DownloadStatuses)
		if err != nil {
			logger.Logger.Error().Err(err).Stack().
				Str("username", username).
				Msg("Failed to search animes on Anilist")
			lastErr = err
			continue
		}

		var filtered []anilist.MediaList
		for i := range resp.Data.Page.MediaList {
			ml := &resp.Data.Page.MediaList[i]
			if !anilist.MediaStatusAllowed(configs.DownloadMediaStatuses, ml.Media.Status) {
				logger.Logger.Debug().
					Int("anime_id", ml.Id).
					Str("media_status", string(ml.Media.Status)).
					Msg("Skipping anime: media status not in DownloadMediaStatuses")
				continue
			}
			if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 {
				ml.CustomLists = cl
			}
			filtered = append(filtered, *ml)
		}
		resp.Data.Page.MediaList = filtered

		count := len(resp.Data.Page.MediaList)
		logger.Logger.Debug().
			Str("username", username).
			Int("animes_found", count).
			Msg("Fetched animes from Anilist for username")
		merged.Data.Page.MediaList = append(merged.Data.Page.MediaList, resp.Data.Page.MediaList...)
	}

	if len(merged.Data.Page.MediaList) == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to search animes on Anilist: %w", lastErr)
	}

	merged.Data.Page.MediaList = anilist.DedupeByMedia(merged.Data.Page.MediaList)

	// DEPOIS do dedupe, nunca antes: a entrada real precisa vencer. DedupeByMedia mantem o
	// MENOR progresso, entao o Progress 0 do MediaList sintetico de GetMediaByID ganharia e o
	// daemon voltaria a baixar episodios ja assistidos.
	merged.Data.Page.MediaList = appendStandaloneAnimes(fileManager, merged.Data.Page.MediaList, standaloneIDs)

	logger.Logger.Info().
		Strs("usernames", configs.AnilistUsernames).
		Int("animes_found", len(merged.Data.Page.MediaList)).
		Msg("Successfully fetched animes from Anilist")

	return merged, nil
}
