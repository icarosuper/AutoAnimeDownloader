package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// animeProcessResult holds the per-anime outputs from processAnimeEpisodes.
type animeProcessResult struct {
	newEpisodes     []files.EpisodeStruct
	checkedEpisodes []files.EpisodeKey
	keysToDelete    []files.EpisodeKey
	// issues are the reasons this anime stopped downloading something. It comes through the channel
	// that already exists on purpose: a *passReport shared between the maxConcurrentAnimes goroutines
	// would need a mutex, and the fan-in already solves that for free.
	issues []Issue
}

// maxConcurrentAnimes limits simultaneous Nyaa HTTP searches to avoid rate limiting.
const maxConcurrentAnimes = 5

// verificationMu serializes the passes. A pass reads episodes.json ONCE at the start and only
// writes at the end (Phase 3), so two simultaneous passes — the loop's and POST /check's, or two
// /check — see the same stale list and download everything again: that's how an anime already
// covered by a pack got standalone torrents of the same episodes, and the second pass's write
// overwrote the pack's records. A concurrent pass is DROPPED, not queued: it would read the same
// state the running pass is already fixing, and the loop's next tick covers whatever is missing.
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

	// The library is built with hardlinks. The config save endpoint probes this, but configs written
	// before this upgrade (or straight into config.json by docker/entrypoint.sh) never went through
	// it. Without this gate a filesystem without hardlink support happily downloads while every
	// JobOrganize dies, and the UI shows a healthy daemon. Probing here returns the same actionable
	// message as the endpoint, and aborts the pass: downloading what can't be organized only fills the
	// disk.
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
		state.SetLastCheckError(fmt.Errorf("%w: not initialized", errCauseTorrent))
		return
	}

	// Safety net: main.go and PUT /config already apply the limit, but a hand-edited config.json never
	// goes through either. It's also what makes the queue a real reconciliation: every
	// SetMaxActiveDownloads runs an enforce, so a pause rain refused or a `stopping` left halfway
	// resolves itself on the next cycle. Runs BEFORE Ensure, when there may be no session yet — step 0
	// of enforce is what handles that (see decisions.md #41).
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

	// A torrent at zero peers for hours doesn't come back on its own and holds a
	// max_concurrent_downloads slot indefinitely. Runs before the rest of the pass, but the snapshot
	// above is NOT retaken: whatever was dropped now is searched again on the next pass.
	dropStalledTorrents(downloadedTorrents, backend, fileManager, time.Now())

	// Same abort logic: with the AnimeIDs still in the old format (entry id) nothing on disk matches
	// AniList, and a pass in that state would re-download the whole library.
	if err := MigrateAnimeIDsToMedia(fileManager); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to migrate anime IDs to AniList media IDs; skipping verification")
		state.SetLastCheckError(fmt.Errorf("%w: %w", errCauseStorage, err))
		return
	}

	// Phase 1: fetch all independent data sources in parallel.
	var (
		anilistResponse *anilist.AniListResponse
		savedEpisodes   []files.EpisodeStruct
		blockedEpisodes []files.EpisodeKey
		// inDeleteStatus[account][mediaId] — which animes each account has in some delete status. An
		// account whose fetch failed is absent from the map, and absent never agrees. MAL accounts are
		// keyed by malAccountKey.
		inDeleteStatus map[string]map[int]bool

		errAnilist  error
		errEpisodes error
	)

	// Local, cheap read, done before the fan-out because searchAnilist depends on it. A failure here
	// doesn't abort the pass: without the file the daemon just stops covering the standalone animes.
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
			inDeleteStatus = make(map[string]map[int]bool, len(configs.AnilistUsernames)+len(configs.MALUsernames))
			for _, username := range configs.AnilistUsernames {
				resp, e := anilist.GetAllCurrentAnime(username, configs.DeleteStatuses)
				// ponytail: the local cache's list votes on deletion as if it were fresh, by explicit decision
				// of the project owner. The risk has a name: an anime that left a delete status AFTER the
				// snapshot (COMPLETED that became REPEATING, and the pass downloaded the season) shows up as
				// deletable again, and the pass deletes what it just downloaded. If this ever bites, the fix is
				// one line: treat ErrFromCache as an account with no answer here, and
				// allAccountsAgreeOnDelete's unanimity refuses on its own.
				if e != nil && !errors.Is(e, anilist.ErrFromCache) {
					logger.Logger.Warn().Err(e).Str("username", username).Msg("Failed to fetch AniList animes for delete statuses")
					// An account with no answer can't agree with the deletion — see deletableMediaIDs.
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
			for _, user := range configs.MALUsernames {
				entries, e := anilist.GetMALEntries(user, anilist.PriorityCritical)
				// ErrFromCache votes as if fresh, the same explicit choice as the AniList loop
				// above. ErrMALEmpty is a failure, not an empty answer: an account that "tracks
				// nothing" would lose its veto — see anilist.ErrMALEmpty.
				if e != nil && !errors.Is(e, anilist.ErrFromCache) {
					logger.Logger.Warn().Err(e).Str("mal_user", user).Msg("Failed to fetch MAL list for delete statuses")
					continue
				}
				byMedia := make(map[int]bool)
				for _, entry := range anilist.FilterMALEntries(entries, configs.DeleteStatuses) {
					byMedia[entry.MediaID] = true
				}
				inDeleteStatus[malAccountKey(user)] = byMedia
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

	// Absolute series axis of every anime in the pass AND of every anime_id with an episode on disk:
	// it's what lets the pass recognize that a pack downloaded under another cour already covers the
	// pending episode (coverage.go). A partial failure doesn't abort the pass — without the offset the
	// anime just doesn't adopt anything.
	animeIDs := make([]int, 0, len(animes))
	for _, a := range animes {
		animeIDs = append(animeIDs, a.Media.Id)
	}
	seriesIndex := resolveSeriesIndex(animeIDs, savedEpisodes)

	// Status deletion rule: EVERY account that has the anime must have it in some delete status (not
	// necessarily the same one). The download rule is the opposite — ONE account is enough —, and it's
	// already applied in searchAnilist through the union of the lists.
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

		// anime.Status belongs to ONE arbitrary account (the one that won the dedupe), so it can't decide
		// anything — the AND rule above is what answers.
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

	// AFTER SetLastCheckError, never before: it clears the report (see state.go). The cancellation
	// above also calls SetLastCheckError(nil) and returns, so an interrupted pass leaves no report —
	// which is right, it was incomplete.
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

// deletableMediaIDs decides which animes may have their episodes deleted by status.
//
// Rule: EVERY account (AniList or MyAnimeList) that has the anime in its list must have it in some
// delete status — the statuses don't have to be the same (DROPPED in one and COMPLETED in another
// deletes). An account that doesn't track the anime doesn't vote; one that has it in a neutral
// status (PLANNING, for example) vetoes, otherwise we'd delete episodes another account still means
// to watch.
//
// Only animes WITH an episode on disk are evaluated: the answer changes nothing for the rest, and
// that's what keeps the tie-break queries rare — a deletable anime leaves the disk in the same pass
// and never becomes a candidate again.
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
			// This account's fetch failed: without its opinion there's no unanimity.
			logger.Logger.Debug().Str("username", username).Int("media_id", mediaID).
				Msg("Skipping status deletion: account list unavailable")
			return false
		}
		if byMedia[mediaID] {
			continue
		}

		// The account didn't list the anime among the deletable ones. It may not track it (no vote) or
		// hold it in a neutral status (veto) — only a query tells them apart.
		status, tracked, err := anilist.GetMediaListStatus(username, mediaID)
		switch {
		case err != nil:
			logger.Logger.Warn().Err(err).Str("username", username).Int("media_id", mediaID).
				Msg("Skipping status deletion: could not resolve the account's status")
			return false
		case !tracked:
			continue // the account doesn't track this anime, no veto
		case isInDeleteStatuses(configs.DeleteStatuses, status):
			continue // a delete status other than the one the list fetch brought
		default:
			logger.Logger.Debug().Str("username", username).Int("media_id", mediaID).
				Str("status", string(status)).Msg("Status deletion vetoed by account")
			return false
		}
	}

	for _, user := range configs.MALUsernames {
		byMedia, fetched := inDeleteStatus[malAccountKey(user)]
		if !fetched {
			logger.Logger.Debug().Str("mal_user", user).Int("media_id", mediaID).
				Msg("Skipping status deletion: MAL list unavailable")
			return false
		}
		if byMedia[mediaID] {
			continue
		}
		// Not among the deletable ones: the cached MAL list says whether the account doesn't
		// track it (no vote) or holds it in a neutral status (veto). No extra request — the list
		// fetched above is still cached.
		entries, err := anilist.GetMALEntries(user, anilist.PriorityCritical)
		if err != nil && !errors.Is(err, anilist.ErrFromCache) {
			logger.Logger.Warn().Err(err).Str("mal_user", user).Int("media_id", mediaID).
				Msg("Skipping status deletion: could not resolve the MAL account's status")
			return false
		}
		for _, entry := range entries {
			if entry.MediaID == mediaID && !isInDeleteStatuses(configs.DeleteStatuses, entry.Status) {
				logger.Logger.Debug().Str("mal_user", user).Int("media_id", mediaID).
					Str("status", string(entry.Status)).Msg("Status deletion vetoed by MAL account")
				return false
			}
		}
	}
	return true
}

// malAccountKey keys a MAL account in inDeleteStatus. The prefix keeps an AniList and a MAL
// account with the same name from overwriting each other.
func malAccountKey(user string) string { return "mal:" + user }

// searchAnilist builds the pass's anime universe: the union of the configured accounts' lists
// (AniList and MyAnimeList) plus the standalone animes (§ standalone).
//
// fileManager comes in here, not just the config, because of the automatic removal: a standalone
// anime that later showed up in a list leaves the file, and the only moment that's known is exactly
// here, with the merged list in hand.
func searchAnilist(fileManager FileManagerInterface, configs *files.Config, standaloneIDs []int) (*anilist.AniListResponse, error) {
	// No list account is NOT an error: the pass still has work to do (the standalone animes), and
	// aborting here would make the feature never run on an install without a list. No library is:
	// there's nowhere to download to.
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
		// The local cache serves the pass: episodes already aired stay downloadable and Nyaa didn't go
		// down with it. TimeUntilAiring comes rebased (see anilist/persist.go), so the episode that was
		// ten minutes away when AniList went down enters the search on its own.
		if err != nil && !errors.Is(err, anilist.ErrFromCache) {
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

	var lastMALErr error
	for _, user := range configs.MALUsernames {
		list, err := anilist.GetMALList(user, configs.DownloadStatuses, anilist.PriorityCritical)
		if err != nil && !errors.Is(err, anilist.ErrFromCache) {
			logger.Logger.Error().Err(err).Str("mal_user", user).Msg("Failed to search animes on MyAnimeList")
			lastMALErr = err
			continue
		}
		count := 0
		for _, ml := range list {
			if anilist.MediaStatusAllowed(configs.DownloadMediaStatuses, ml.Media.Status) {
				merged.Data.Page.MediaList = append(merged.Data.Page.MediaList, ml)
				count++
			}
		}
		logger.Logger.Debug().Str("mal_user", user).Int("animes_found", count).Msg("Fetched animes from MyAnimeList for username")
	}

	if len(merged.Data.Page.MediaList) == 0 {
		// The banner names the source that failed. With both failing it stays AniList.
		if lastErr != nil {
			return nil, fmt.Errorf("failed to search animes on Anilist: %w", lastErr)
		}
		if lastMALErr != nil {
			return nil, fmt.Errorf("%w: failed to search animes on MyAnimeList: %w", errCauseMAL, lastMALErr)
		}
	}

	merged.Data.Page.MediaList = anilist.DedupeByMedia(merged.Data.Page.MediaList)

	// AFTER the dedupe, never before: the real entry has to win. DedupeByMedia keeps the LOWEST
	// progress, so the Progress 0 of GetMediaByID's synthetic MediaList would win and the daemon would
	// go back to downloading already-watched episodes.
	merged.Data.Page.MediaList = appendStandaloneAnimes(fileManager, merged.Data.Page.MediaList, standaloneIDs)

	logger.Logger.Info().
		Strs("usernames", configs.AnilistUsernames).
		Strs("mal_usernames", configs.MALUsernames).
		Int("animes_found", len(merged.Data.Page.MediaList)).
		Msg("Successfully fetched animes from Anilist")

	return merged, nil
}
