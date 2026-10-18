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

// episodeSelection is the result of an anime's selection loop.
type episodeSelection struct {
	toDownload   []anilist.AiringNode
	keysToDelete []files.EpisodeKey
	checked      []files.EpisodeKey
	// downloaded and limitSkipped are the pair the report publishes as "downloaded N, M left". They
	// come from selectEpisodes' FINAL result on purpose: when a pack was chosen the selection runs
	// again with the limit lifted, and then limitSkipped is zero — which is right, because in that
	// anime the limit blocked nothing.
	downloaded   int
	limitSkipped int
}

// selectEpisodes decides, for each episode in the list, whether it should be downloaded or deleted.
//
// It is PURE: no I/O and no state between calls (the downloaded-episodes counter is local), which
// is why it can run twice in the same pass — once with the per-anime limit in force (the one that
// produces the deletions) and again with it lifted when a pack was chosen. See decisions.md.
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

// effectiveMax is the per-anime limit applied to the pass. max_episodes_per_anime == 0 means NO
// CEILING, and "no ceiling" is expressed as len(episodes)+1 — the same trick the pack path already
// used to lift the limit, now also for the ceiling turned off.
func effectiveMax(configs *files.Config, episodes []anilist.AiringNode) int {
	if configs.MaxEpisodesPerAnime <= 0 {
		return len(episodes) + 1
	}
	return configs.MaxEpisodesPerAnime
}

// windowEnd is the last episode the packs must cover in this cycle: the other side of
// max_episodes_per_anime. With the ceiling off the window is open, which is expensive on purpose
// (the brakes become max_batch_torrent_size_gb, per torrent, and checkDiskSpace).
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
	seriesIndex map[int]anilist.Series,
	blockedMap map[files.EpisodeKey]bool,
	searcher nyaaSearcher,
	disk *diskBudget,
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

	// The first selection is the one that counts when there's no pack, and it's the one that produces
	// the limit deletions: with the limit lifted by guess, handleAlreadySavedEpisode never fired and
	// keysToDelete came back empty for everyone.
	sel := selectEpisodes(configs, effectiveMax(configs, episodes), anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)

	// Ownership by coverage, BEFORE any search: what an already downloaded pack covers on the series'
	// absolute axis doesn't go back to Nyaa (see coverage.go).
	adopted := adoptCoveredEpisodes(anime, animeTitle, totalEpisodes, sel.toDownload, savedEpisodes, seriesIndex, torrentsHashSet)
	sel.toDownload = dropAdopted(sel.toDownload, adopted)
	result.newEpisodes = append(result.newEpisodes, adopted...)

	var magnetsForEpisodes map[int]resolvedMagnets
	// batchSkipped is why max_episodes_per_anime is in force for this anime. It lives outside the if
	// because the report publishes it further down, after sel may have been redone.
	batchSkipped := ""

	if isAnimeMovie(anime) {
		sel.toDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, sel.toDownload, searcher)
	}

	if magnetsForEpisodes == nil && len(sel.toDownload) > 0 {
		packs, singles, packStats := partitionSearchResults(configs, anime, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload)))

		// Pack eligibility: not a movie, more than one pending episode, and the FILTERED search returned
		// a pack covering the window. None of this is AniList metadata — the torrent that's there decides
		// (see decisions.md).
		if !isAnimeMovie(anime) && len(sel.toDownload) > 1 {
			firstPending := sel.toDownload[0].Episode
			axis := newPackAxis(anime, seriesIndex, totalEpisodes)
			batches := pickBatches(newPackSet(packsThatFit(configs, disk, packs), searcher.packRange), axis, firstPending, windowEnd(configs, firstPending))
			switch {
			case batches.len() > 0:
				logger.Logger.Info().
					Str("anime", animeTitle).
					Int("packs", batches.len()).
					Strs("torrents", batchNames(batches)).
					Msg("Using batch torrents to cover the pending window")

				// Second selection, with the limit lifted: the episodes.json records must cover the WHOLE pack,
				// otherwise the count lies and pruning would delete what the pack just brought.
				sel = selectEpisodes(configs, len(episodes)+1, anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)
				sel.toDownload = dropAdopted(sel.toDownload, adopted)
				sel.toDownload, magnetsForEpisodes = assignBatches(animeTitle, axis, sel.toDownload, batches)
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

	// The limit only becomes a report row when it actually blocked something. When a pack was chosen,
	// the second selection runs with the limit lifted and limitSkipped is zero — which is right: in
	// that anime the limit prevented nothing.
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

	// Series length, only so the search can decide the zero-padding of the episode query.
	seriesLength := anilist.LastAiredEpisode(anime)

	// stuck: an earlier episode of this anime ended this pass without a torrent. Episodes are released
	// in order, so asking Nyaa for the ones after it is almost always wasted — they skip the
	// per-episode search too, and still get whatever the anime search found (decisions.md #100).
	stuck := false
	for _, ep := range episodesToDownload {
		epName := fmt.Sprintf("%s - Episode %d", animeTitle, ep.Episode)
		missKey := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: ep.Episode}

		resolved := magnetsForEpisodes[ep.Episode]
		candidates := resolved.candidates
		skipSubfolder := resolved.skipSubfolder
		if resolved.overrideName != "" {
			epName = resolved.overrideName
		}

		// ponytail: the per-episode fallback's dropStats is the ONLY source of the problems cascade —
		// neither partitionSearchResults nor resolveMovie feed the report. It has the last word on this
		// episode: if it found a magnet there was no problem, if it didn't, its cut describes why. If the
		// pack ever needs its own code, the way is to pass packStats here instead of overwriting this
		// one.
		var searchStats dropStats
		// awaitingEarlier: skipped for an earlier missing episode, with no search of its own to report.
		awaitingEarlier := false
		if len(candidates) == 0 {
			due := true
			if searcher.misses != nil {
				due, searchStats = searcher.misses.due(missKey, time.Now())
			}
			if stuck || !due {
				awaitingEarlier = stuck && searchStats == dropStats{}
				logger.Logger.Debug().
					Str("episode", epName).
					Bool("earlier_episode_missing", stuck).
					Msg("Skipping per-episode search: a recent search came back empty")
			} else {
				candidates, searchStats = filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
				if len(candidates) == 0 && searcher.misses != nil {
					searcher.misses.record(missKey, time.Now(), searchStats)
				}
			}
		}

		// Without a magnet there's nothing to try: announcing "starting download" here sent a false push
		// on every loop pass while the episode wasn't on Nyaa, and also made attemptDownloadWithRetries
		// log "failed after all attempts" with zero attempts.
		if len(candidates) == 0 {
			stuck = searcher.misses != nil
			logger.Logger.Warn().
				Str("episode", epName).
				Msg("No torrent found for episode")
			issue := searchIssue(anime.Media.Id, animeTitle, ep.Episode, searchStats, configs)
			if awaitingEarlier {
				issue.Code = IssueAwaitingEarlierEpisode
			}
			result.issues = append(result.issues, issue)
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, notifications.ReasonNotFound)
			continue
		}

		if searcher.misses != nil {
			searcher.misses.clear(missKey)
		}
		notifications.Notify(configs, notifications.NewEpisode, animeTitle, ep.Episode, "")

		hash, torrentSize := attemptDownloadWithRetries(configs, backend, disk, candidates, epName)

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
				TorrentSize:        torrentSize,
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
				Candidates: len(candidates),
			}
			// The best candidate's size counts: the disk can have headroom in percent and still not fit the
			// pack this episode wanted.
			if errors.Is(disk.check(configs, candidates[0].Size), ErrInsufficientDiskSpace) {
				reason = notifications.ReasonNoDiskSpace
				issue.Code = IssueDiskFull
				// A full disk isn't about the candidates: either none was tried (attemptDownloadWithRetries
				// returns before the first Add), or they were skipped for not fitting — in both cases "N
				// candidates" would be a meaningless number.
				issue.Candidates = 0
			}
			result.issues = append(result.issues, issue)
			// The notification batch (BatchWindowSeconds) merges the pass's N episodes into a single
			// message, so a full disk doesn't become a flood.
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, reason)
		}
	}

	return result
}

type resolvedMagnets struct {
	// candidates carries the whole TorrentResult, not just the magnet, because Size is what the disk
	// guard needs at Add time (checkDiskSpace).
	candidates    []nyaa.TorrentResult
	skipSubfolder bool
	overrideName  string
	// batchStart/batchEnd: the pack's range read from the torrent name, 0 when unknown. It goes into
	// the saved record because the screen can't rebuild it later — already watched episodes don't
	// become records, and the min/max of those that do lies about the pack.
	batchStart int
	batchEnd   int
}

// resolveMovie is the movie path, unchanged: when the movie is found, every pending episode (or a
// synthetic one, if there was none) gets its magnet. It returns (episodes, nil) when there's no
// movie, and the flow falls into the anime search — the fallback that always existed.
func resolveMovie(configs *files.Config, anime anilist.MediaList, animeTitle string, episodes []anilist.AiringNode, searcher nyaaSearcher) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Detected movie - searching for movie torrent")

	movieResult, _ := filterSearchResults(searcher.searchMovie(anime.Media.Title, true), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
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
			candidates:    movieResult[:1],
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

// mediaTotalEpisodes is the count AniList declares, 0 when it doesn't exist (an airing series with
// no announced total).
func mediaTotalEpisodes(anime anilist.MediaList) int {
	if anime.Media.Episodes == nil {
		return 0
	}
	return *anime.Media.Episodes
}

// assignBatches gives each episode the magnet of ITS pack and returns only the covered episodes.
//
// The truncation is required: without it, the episodes outside the coverage would fall into loose
// episodes and on One Piece the fallback would find nothing, firing DownloadFailed/ReasonNotFound
// for ~1000 episodes per pass. The list comes from the schedule in ascending order and the chosen
// packs are contiguous from the first pending one, so the cut is always a prefix.
//
// overrideName carries the range when it's known: two packs of the same anime in the same cycle
// would show up with identical names on the downloads screen. The library FOLDER name doesn't
// change — Organize uses AnimeName, never EpisodeName.
//
// An unknown range records range ZERO, not 1..total. The range comes from the name or, when the
// name doesn't carry it, from the pack's file list (packSet.rangeOf) — getting here without a range
// means both sources failed, and then "don't know" is the only honest record. 1..total was a guess
// that hasDeclaredRange read as a declared range, and from there ownership by coverage adopted,
// under another cour, episodes the pack might not have (decisions.md #84).
//
// The RECORDED range is the one converted to the local numbering (packAxis.localRange) and can
// start at zero or below; the displayed name clamps at 1, because "-10-12" tells the reader nothing
// and what matters there is the slice this entry gets.
func assignBatches(animeTitle string, axis packAxis, episodes []anilist.AiringNode, batches *packSet) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	result := make(map[int]resolvedMagnets, len(episodes))
	var covered []anilist.AiringNode

	for _, ep := range episodes {
		batch, info := batches.covering(axis, ep.Episode)
		if batch == nil {
			break
		}

		name := animeTitle
		if info.EndEpisode > 0 {
			name = packDisplayName(animeTitle, info.StartEpisode, info.EndEpisode)
		}

		result[ep.Episode] = resolvedMagnets{
			candidates:    []nyaa.TorrentResult{batch.torrent},
			skipSubfolder: true,
			overrideName:  name,
			batchStart:    info.StartEpisode,
			batchEnd:      info.EndEpisode,
		}
		covered = append(covered, ep)
	}

	return covered, result
}

// packDisplayName is the displayed name of a pack record.
//
// The start is clamped at 1 because the RECORDED range can start at zero or below — a season pack
// recorded under a later cour starts before that entry's episode 1 (#79). The negative range is
// correct as data and unreadable as text: "Anime -10-12" says nothing.
func packDisplayName(animeTitle string, start, end int) string {
	return fmt.Sprintf("%s %d-%d", animeTitle, max(1, start), end)
}

// magnetsByEpisode is the loose-episode path: each episode gets the magnets of the rows that
// matched ITS number, in quality order.
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
		result[ep.Episode] = resolvedMagnets{candidates: trs}
	}
	return result
}

// batchNames is only for the log: which pack covered which range is the first thing one wants to
// know when auditing a pack choice.
func batchNames(batches *packSet) []string {
	out := make([]string, 0, batches.len())
	for _, b := range batches.items {
		out = append(out, b.torrent.Name)
	}
	return out
}

// partitionSearchResults splits ScrapNyaaForAnime's single list into packs and episodes, and
// applies to each its OWN size ceiling: the two ceilings are independent (a 40 GiB pack is still
// valid with max_episode_torrent_size_gb = 1.5). The relative order from SortTorrentResults is
// kept, so each list comes out sorted by quality.
//
// packStats describes what the filter did to the PACK rows, and only to them: it answers whether
// "there was no batch" because the search returned no pack at all or because all were cut by the
// ceiling. The report uses it as the detail field of the per-anime limit.
//
// A first-season entry also drops packs marked with a later season here (see isFirstSeasonEntry).
func partitionSearchResults(configs *files.Config, anime anilist.MediaList, results []nyaa.TorrentResult) ([]nyaa.TorrentResult, []nyaa.TorrentResult, dropStats) {
	firstEntry := isFirstSeasonEntry(anime)
	var packs, singles []nyaa.TorrentResult
	for _, tr := range results {
		switch {
		case tr.IsBatch && firstEntry && tr.Season != nil && *tr.Season > 1:
			logger.Logger.Debug().
				Str("torrent", tr.Name).
				Int("season", *tr.Season).
				Msg("Skipping pack of a later season for a first-season entry")
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

// packsThatFit drops the packs the disk guard would refuse. It runs BEFORE pickBatches because
// the guard in attemptDownloadWithRetries comes too late: by then the pack already owns the window,
// and a 78 GB remux that doesn't fit left every episode without a candidate even with live loose
// episodes on Nyaa. Dropped here, the next pack or the per-episode search takes over.
//
// It asks the same budget the add will (disk): asking free space alone would let a pack win the
// window here and lose it at the add to what the queue already reserved — the same bug.
func packsThatFit(configs *files.Config, disk *diskBudget, packs []nyaa.TorrentResult) []nyaa.TorrentResult {
	fit := make([]nyaa.TorrentResult, 0, len(packs))
	for _, tr := range packs {
		if err := disk.check(configs, tr.Size); err != nil {
			logger.Logger.Info().Err(err).Str("torrent", tr.Name).Msg("Skipping pack: it does not fit on disk")
			continue
		}
		fit = append(fit, tr)
	}
	return fit
}

// isFirstSeasonEntry reports whether the entry is certainly season 1: no season marker in its
// titles and no TV prequel. Only then can a pack marked "Season 2" be rejected outright — for a
// sequel without a marker ("DanMachi V") the pack's season is still a guess the coverage check
// settles, so the nyaa package keeps accepting packs of any season (nyaa.ScrapNyaaForAnime).
func isFirstSeasonEntry(anime anilist.MediaList) bool {
	if season, _ := ExtractAnimeSeasonPart(anime.Media.Title, anime.Media.Synonyms); season != nil {
		return *season == 1
	}
	for _, edge := range anime.Media.Relations.Edges {
		if edge.RelationType == "PREQUEL" && (edge.Node.Format == anilist.MediaFormatTV || edge.Node.Format == anilist.MediaFormatTVShort) {
			return false
		}
	}
	return true
}

// packAxis are the rulers the pack may be numbered on, expressed as how much to add to the entry's
// LOCAL number to reach the number the group used in the name.
//
// It exists because there isn't one convention, there are four (sources.md, "Pack granularity and
// numbering"): the same season pack shows up as "00 ~ 12" (restarts per season), "1..12"
// (restarts per part), "1..25" (continues through the whole season) or "76..87" (absolute for the
// series). No group announces which one it adopted, so mapping is always a guess — three hypotheses
// and a tie-break.
//
// The zero value reproduces the old behavior (only the relative hypothesis, no tie-break), which is
// what callers without series information want.
type packAxis struct {
	// altDeltas are the hypotheses BESIDES the relative one. Delta 0 is always tried first and is the
	// answer when in doubt: a missing offset falls into the relative numbering, which many groups use,
	// while a wrong offset picks a pack that doesn't have the episode.
	altDeltas []int
	// totalEpisodes is the ENTRY's count, the tie-break between hypotheses (see localRange).
	totalEpisodes int
}

// newPackAxis builds the numbering hypotheses for an anime.
func newPackAxis(anime anilist.MediaList, seriesIndex map[int]anilist.Series, totalEpisodes int) packAxis {
	axis := packAxis{totalEpisodes: totalEpisodes}
	add := func(d int) {
		if d <= 0 {
			return
		}
		for _, existing := range axis.altDeltas {
			if existing == d {
				return
			}
		}
		axis.altDeltas = append(axis.altDeltas, d)
	}

	// "Continues through the season": the pack starts at the season's first episode, which is the
	// immediate prequel's entry. This is where ComputeEpisodeOffset comes in — as ONE hypothesis, not
	// as the only truth (decisions.md #79).
	_, part := ExtractAnimeSeasonPart(anime.Media.Title, anime.Media.Synonyms)
	add(ComputeEpisodeOffset(anime.Media.Relations, part))
	// "Absolute for the series": the series index axis (decisions.md #77).
	add(seriesIndex[anime.Media.Id].Offset)

	return axis
}

// localRange converts the range declared in the pack name to the entry's local numbering, choosing
// among the hypotheses. It returns false when none covers the requested episode.
//
// Only a hypothesis that actually covers the episode competes. Among those that do, the one that
// makes the pack END on the entry's last episode wins: that's the count tie-break from sources.md —
// a 25-episode pack under a 12-episode entry can't be a local pack, and a 12-episode one under the
// same entry can't be anything else. Without it delta 0 would match both, and the "1..25" would be
// recorded as starting at THIS entry's episode 1 — lying to ownership by coverage (decisions.md
// #78).
//
// The converted start can be <= 0, and that's correct: a season pack downloaded under cour 2 starts
// before that entry's episode 1. Whoever reads the recorded range uses hasDeclaredRange, which uses
// the END as the sentinel precisely for that reason.
func (a packAxis) localRange(info nyaa.BatchInfo, episode int) (nyaa.BatchInfo, bool) {
	var chosen nyaa.BatchInfo
	found := false
	for _, d := range append([]int{0}, a.altDeltas...) {
		if info.StartEpisode > episode+d || episode+d > info.EndEpisode {
			continue
		}
		local := info
		local.StartEpisode, local.EndEpisode = info.StartEpisode-d, info.EndEpisode-d
		if a.totalEpisodes > 0 && local.EndEpisode == a.totalEpisodes {
			return local, true
		}
		if !found {
			chosen, found = local, true
		}
	}
	return chosen, found
}

// packCandidate is a pack and the range it covers. The range comes from the name when the name
// carries it and, when it doesn't, from the detail page's file list — resolved on demand by
// packSet.
type packCandidate struct {
	torrent nyaa.TorrentResult
	// info in the numbering the GROUP used (name or files), not the entry's: packAxis.localRange
	// converts it. EndEpisode == 0 = unknown even after the detail page.
	info     nyaa.BatchInfo
	resolved bool
}

// maxPackDetailFetches is how many detail pages an anime can cost per pass.
//
// The detail page costs ~30 KB (measured on 7 packs, sources.md) against ~125 KB for a listing, so
// the cost isn't bandwidth: it's not turning one search into dozens of requests against nyaa.si.
// Three covers the real choice — packs come sorted by quality, and pickBatches rarely goes past the
// first few to close the window.
const maxPackDetailFetches = 3

// packSet is the candidate packs with each one's range, resolved ON DEMAND.
//
// On demand rather than all at once because most searches never need the detail page: if the first
// pack already declares its range in the name and covers the window, no extra request goes out.
// Fetching the detail of every rangeless pack in the list would spend three requests per anime, per
// pass, on packs that would never be chosen.
type packSet struct {
	items []packCandidate
	// detail is nil in the already chosen set (pickBatches returns everything resolved) and in the
	// tests that only build ranges from the name.
	detail func(nyaa.TorrentResult) (nyaa.BatchInfo, bool)
	budget int
}

func newPackSet(results []nyaa.TorrentResult, detail func(nyaa.TorrentResult) (nyaa.BatchInfo, bool)) *packSet {
	items := make([]packCandidate, len(results))
	for i, tr := range results {
		items[i] = packCandidate{torrent: tr, info: nyaa.ExtractBatchInfo(tr.Name)}
	}
	return &packSet{items: items, detail: detail, budget: maxPackDetailFetches}
}

// rangeOf returns pack i's range, fetching the detail page the FIRST time it's requested for a pack
// whose name carries no range. It remembers failures too: a detail page that didn't answer isn't
// retried within the same pass.
func (p *packSet) rangeOf(i int) nyaa.BatchInfo {
	item := &p.items[i]
	if item.resolved {
		return item.info
	}
	item.resolved = true
	if item.info.EndEpisode == 0 && p.detail != nil && p.budget > 0 {
		p.budget--
		if fromFiles, ok := p.detail(item.torrent); ok {
			logger.Logger.Info().
				Str("torrent", item.torrent.Name).
				Int("start_episode", fromFiles.StartEpisode).
				Int("end_episode", fromFiles.EndEpisode).
				Msg("Pack range resolved from the Nyaa file list")
			item.info = fromFiles
		}
	}
	return item.info
}

// covering returns the first pack that covers the episode AND its range already converted to the
// entry's local numbering.
//
// A pack whose range stays unknown AFTER the detail page (page down, single-file torrent, no
// readable file name) leaves here without going through the axis and counts as complete — the same
// behavior as before this lookup existed. Not rejecting it is deliberate: a Nyaa outage would zero
// everyone's packs. What changes is what gets RECORDED in that case, see assignBatches. An
// unlabeled pack is the exception: it is a pack only if its file list says so.
func (p *packSet) covering(axis packAxis, episode int) (*packCandidate, nyaa.BatchInfo) {
	for i := range p.items {
		info := p.rangeOf(i)
		if info.EndEpisode == 0 {
			// A pack known only by its size has nothing but the file list to say it is a pack
			// of THIS window; without it, it could be a single long episode or a different cut.
			if p.items[i].torrent.UnlabeledPack {
				continue
			}
			return &p.items[i], info
		}
		if local, ok := axis.localRange(info, episode); ok {
			return &p.items[i], local
		}
	}
	return nil, nyaa.BatchInfo{}
}

func (p *packSet) len() int { return len(p.items) }

// pickBatches returns the minimum set of packs covering [firstPending, windowEnd], in order.
//
// results already come sorted by SortTorrentResults and filtered by size and seeders, so "the first
// that covers the cursor" is "the best that covers the cursor" — which is why the pack choice
// stopped being decided by AniList metadata and started being decided by the search result.
//
// It always ends: covering only returns a pack with EndEpisode >= cursor (already converted), so
// the cursor strictly grows each turn and no pack can be chosen twice.
//
// The returned set doesn't fetch any more detail: the chosen ones already went through rangeOf.
func pickBatches(candidates *packSet, axis packAxis, firstPending, windowEnd int) *packSet {
	var picked []packCandidate
	for cursor := firstPending; cursor <= windowEnd; {
		next, info := candidates.covering(axis, cursor)
		if next == nil {
			break
		}
		picked = append(picked, *next)
		if info.EndEpisode == 0 {
			break // an unknown range counts as a complete pack: it covers the rest of the window
		}
		cursor = info.EndEpisode + 1
	}
	return &packSet{items: picked}
}

// episodeNumbers is the list of numbers the anime search uses to filter episode rows.
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
		// A pack record stays OUT of the keep-set: keeping 3 of a pack's 100 doesn't keep 3 episodes, it
		// keeps the whole pack — canRemoveTorrent would return false, the torrent would stay on disk, the
		// space wouldn't come back and the next pack would never come. The keep-set doesn't have the
		// granularity it promises inside a pack.
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

// firstEpisodeToConsider is where an anime's episode list starts.
//
// Rule: progress + 1 — the episode after the last watched one. A standalone anime has no list entry
// and therefore progress 0, which makes its list start at 1 without needing a flag.
//
// It goes back to the lowest ALREADY SAVED episode when that one is earlier, because
// watched_episodes_to_keep and the pruning of watched episodes only work on episodes that appear in
// the list: an episode saved outside it would never be "checked" and would fall straight into
// identifyEpisodesNotInWatching, ignoring the "how many watched to keep".
func firstEpisodeToConsider(anime anilist.MediaList, savedEpisodes []files.EpisodeStruct) int {
	first := anime.Progress + 1
	for _, ep := range savedEpisodes {
		if ep.AnimeID == anime.Media.Id && ep.EpisodeNumber > 0 && ep.EpisodeNumber < first {
			first = ep.EpisodeNumber
		}
	}
	return first
}

// checkEpisode decides whether an episode should be downloaded or deleted. maxEpisodes is the
// EFFECTIVE per-anime limit, computed by the caller: for a batch it comes unlimited (see
// processAnimeEpisodes).
//
// skipCode is the skip reason WHEN it goes into the last-verification report, and "" otherwise. The
// condition lives here, not in the caller that has the same pointer, so the rule doesn't exist in
// two places and drift apart later.
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
			// The same "limiting a pack's records doesn't limit bytes" logic, now at the DELETION point
			// instead of the lifting one: the pack is the unit, deleting a record frees no byte and the
			// torrent stays on disk. Without this, the cycle after the one that brought the pack would
			// delete the episodes above the limit and the space gate would reopen right after — a
			// download-and-delete loop. It doesn't delete, and it doesn't download anything else of this
			// anime either.
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

// attemptDownloadWithRetries returns the hash of the torrent it added and the size Nyaa announced
// for it, which the record keeps so the disk budget can count the torrent while it waits in the
// queue (newDiskBudget).
func attemptDownloadWithRetries(configs *files.Config, backend torrents.TorrentBackend, disk *diskBudget, candidates []nyaa.TorrentResult, fileName string) (hash string, size int64) {
	// Full disk: not a single magnet is tried and there's no retry — the candidate isn't the problem,
	// and trying 3 times would only fill the log. Each candidate's size is checked in the loop. A
	// candidate already paid for (another episode of the same pack) skips this: it adds no bytes.
	if !disk.counts(candidates) {
		if err := disk.check(configs, 0); err != nil {
			logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Skipping download: insufficient free disk space")
			return "", 0
		}
	}

	maxAttempts := min(configs.EpisodeRetryLimit, len(candidates))

	for i := range maxAttempts {
		logger.Logger.Debug().
			Str("episode", fileName).
			Int("attempt", i+1).
			Int("max_attempts", configs.EpisodeRetryLimit).
			Msg("Attempting to download episode")

		// Per candidate, because each has a size: the next one in the list may fit where the first
		// didn't.
		undo, err := disk.reserve(configs, candidates[i])
		if err != nil {
			logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Skipping candidate: it does not fit on disk")
			continue
		}

		h, err := backend.Add(candidates[i].MagnetLink)
		if err != nil {
			undo()
			logger.Logger.Warn().Err(err).Str("episode", fileName).Msg("Failed to add torrent to embedded client")
			continue
		}
		if h != "" {
			logger.Logger.Info().
				Str("episode", fileName).
				Str("hash", h).
				Msg("Successfully added episode to embedded torrent client")
			return h, candidates[i].Size
		}
		undo()
	}

	logger.Logger.Warn().
		Str("episode", fileName).
		Int("attempts", maxAttempts).
		Msg("Failed to download episode after all attempts")
	return "", 0
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
	return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, false, false)
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

	return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, opts.KeepData, true)
}

// deleteEpisodesByStatus deletes the episodes of the anime that ALL accounts agree to delete.
// deletableMediaIDs (verification.go) decides that — it's only applied here.
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
	if err := removeEpisodesAndLinks(fileManager, backend, librarian, keysToDelete, savedEpisodes, false, false); err != nil {
		logger.Logger.Warn().Err(err).Msg("Status-based deletion: failed to delete episodes from file")
	}
}

func handleSavedEpisodes(fileManager FileManagerInterface, configs *files.Config, backend torrents.TorrentBackend, librarian files.Librarian, data handleEpisodesData) {
	episodesNotInWatching := identifyEpisodesNotInWatching(data.savedEpisodes, data.checkedEpisodes)

	saveEpisodesToFile(fileManager, data.newEpisodes)

	if configs.DeleteWatchedEpisodes {
		allKeys := append(append([]files.EpisodeKey{}, data.keysToDelete...), episodesNotInWatching...)
		// data.savedEpisodes is the PRE-pass snapshot: the records saveEpisodesToFile just wrote are
		// invisible in it. If one of them reused the hash of a pack whose old records are leaving now,
		// the torrent guard would conclude "nobody survives" and delete the file the pass just adopted.
		saved := append(append([]files.EpisodeStruct{}, data.savedEpisodes...), data.newEpisodes...)
		// Best-effort: a failure here must not abort the verification pass.
		if err := removeEpisodesAndLinks(fileManager, backend, librarian, allKeys, saved, false, false); err != nil {
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
//
// force, when true, ignores the unclaimed-content guard. The guard exists so the AUTOMATIC pass
// doesn't delete the cour that wasn't downloaded yet; when the user clicks "delete torrent" the
// unit of deletion is the torrent and the user decided — without this the call would report success
// without deleting anything.
func removeEpisodesAndLinks(fm FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, keysToDelete []files.EpisodeKey, savedEpisodes []files.EpisodeStruct, keepData bool, force bool) error {
	if len(keysToDelete) == 0 {
		return nil
	}
	deleteSet := make(map[files.EpisodeKey]bool, len(keysToDelete))
	for _, k := range keysToDelete {
		deleteSet[k] = true
	}

	byHash := make(map[string][]files.EpisodeStruct)
	byKey := make(map[files.EpisodeKey]files.EpisodeStruct, len(savedEpisodes))
	for _, ep := range savedEpisodes {
		byKey[ep.Key()] = ep
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
			removingTorrent := ep.EpisodeHash == "" || force || canRemoveTorrent(byHash[ep.EpisodeHash], deleteSet)
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
		if force || canRemoveTorrent(byHash[ep.EpisodeHash], deleteSet) {
			if err := backend.Remove(ep.EpisodeHash, keepData); err != nil {
				logger.Logger.Warn().Err(err).Str("hash", ep.EpisodeHash).Msg("Failed to remove torrent")
			} else {
				logger.Logger.Info().Str("hash", ep.EpisodeHash).Msg("Removed torrent (seeding copy)")
			}
			removedHashes[ep.EpisodeHash] = true
		}
	}

	forgotten := forgettableKeys(keysToDelete, byKey, byHash, force)
	if err := fm.DeleteEpisodesFromFile(forgotten); err != nil {
		return fmt.Errorf("failed to delete episodes from file: %w", err)
	}

	// With keepData the library didn't change, so there's nothing for Jellyfin/Plex to rescan.
	if !keepData && len(forgotten) > 0 {
		if configs, err := fm.LoadConfigs(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Episode deletion: failed to load configs, skipping webhook")
		} else {
			for _, k := range forgotten {
				ep := byKey[k]
				notifications.Notify(configs, notifications.EpisodeDeleted, ep.AnimeName, ep.EpisodeNumber, "")
			}
		}
	}

	return nil
}

// forgettableKeys takes out of the deletion list the records of the pack that STAYED on disk for
// carrying unclaimed content.
//
// The record is the only place where the pack's declared range exists. Deleting it while the files
// are still there leaves an orphan pack: no future entry can adopt it (findCoveringPack requires
// the range) and the claimed-content count never closes again, so the torrent never leaves the
// disk. Kept, the record stays true — the file is there — and the next cour closes the count, which
// frees the whole pack at once.
//
// It only applies to that case: a pack with a live sibling (part of the window not watched yet)
// keeps losing the watched records, which is the usual rotation.
func forgettableKeys(
	keysToDelete []files.EpisodeKey,
	byKey map[files.EpisodeKey]files.EpisodeStruct,
	byHash map[string][]files.EpisodeStruct,
	force bool,
) []files.EpisodeKey {
	if force {
		return keysToDelete
	}
	keys := make([]files.EpisodeKey, 0, len(keysToDelete))
	for _, k := range keysToDelete {
		ep, ok := byKey[k]
		if ok && ep.EpisodeHash != "" && hasUnclaimedContent(byHash[ep.EpisodeHash]) {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

// canRemoveTorrent reports whether the shared torrent can be removed: no record of the group
// survives the delete set AND the pack carries no unclaimed content.
//
// The second condition exists because "every record" isn't "all the content": a pack that declares
// 1..23 in its name but has only cour 1's 11 episodes claimed covers 12 episodes that never had an
// owner — cour 2, not downloaded yet. With only the first condition, watching cour 1 deleted all of
// cour 2 from disk.
//
// ponytail: a pack whose range neither the name nor the file list resolved records BatchEnd == 0
// (unknown) and stays undetectable here. It's what's left of the rare case after #84 — before, it
// was every pack without a range in its name.
func canRemoveTorrent(group []files.EpisodeStruct, deleteSet map[files.EpisodeKey]bool) bool {
	if len(group) == 0 {
		return true
	}
	for _, ep := range group {
		if !deleteSet[ep.Key()] {
			return false
		}
	}
	return !hasUnclaimedContent(group)
}

// hasUnclaimedContent reports whether the pack covers content no entry of the group claims.
//
// The count is declared span x CLAIMED episodes — not x number of records. A record is only born
// for an episode the daemon downloaded: a 01-11 pack taken at progress 5 has 6 records, and
// comparing 6 with 11 would say "unclaimed content" forever. The torrent would never leave the disk
// and the records would be deleted anyway, leaving a permanent orphan pack — a leak, not
// protection. What says how much of the pack belongs to an entry is ITS episode total
// (AnimeTotalEpisodes): the episodes it didn't record (already watched at download time, blocked)
// are its all the same.
//
// An old record doesn't carry the total (0): then the record count of that anime_id is what's left,
// which is exactly the previous behavior.
func hasUnclaimedContent(group []files.EpisodeStruct) bool {
	span := declaredSpan(group)
	if span == 0 {
		return false
	}
	records := make(map[int]int, 2)
	totals := make(map[int]int, 2)
	for _, ep := range group {
		records[ep.AnimeID]++
		totals[ep.AnimeID] = max(totals[ep.AnimeID], ep.AnimeTotalEpisodes)
	}
	claimed := 0
	for id, n := range records {
		if totals[id] > 0 {
			n = totals[id]
		}
		claimed += n
	}
	return claimed < span
}

// hasDeclaredRange reports whether the record carries the pack's range.
//
// The "unknown" sentinel is the END, not the start: a zero or negative start is a legitimate range
// — "[Erai-raws] … - 00 ~ 12" starts at zero, and a season pack recorded under a later cour starts
// BEFORE that entry's episode 1, in its local numbering (see packAxis.localRange).
func hasDeclaredRange(ep files.EpisodeStruct) bool {
	return ep.BatchEnd > 0 && ep.BatchEnd >= ep.BatchStart
}

// declaredSpan is the pack's size, in episodes, according to what the group's records declare.
//
// It's the maximum PER anime_id, not the union of all: each entry records the range in ITS local
// numbering (packAxis.localRange), so 1..23 under cour 1 and -10..12 under cour 2 are the SAME
// 23-episode pack — merging them would give 34, a pack that doesn't exist, and the guard would
// never free the torrent. A record without a range doesn't count: unknown isn't empty, it just
// doesn't help. 0 when no record declares a range, and then the unclaimed-content count doesn't run
// — the old behavior.
func declaredSpan(group []files.EpisodeStruct) int {
	type bounds struct{ first, last int }
	byAnime := make(map[int]bounds, 2)
	for _, ep := range group {
		if !hasDeclaredRange(ep) {
			continue
		}
		b, ok := byAnime[ep.AnimeID]
		if !ok {
			byAnime[ep.AnimeID] = bounds{ep.BatchStart, ep.BatchEnd}
			continue
		}
		byAnime[ep.AnimeID] = bounds{min(b.first, ep.BatchStart), max(b.last, ep.BatchEnd)}
	}
	span := 0
	for _, b := range byAnime {
		span = max(span, b.last-b.first+1)
	}
	return span
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
