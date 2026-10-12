package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// Reasons an anime can't be added as standalone. They are also the error codes of the POST's 409
// and the tooltip keys of the search screen — one field, not four booleans, because they are
// mutually exclusive by precedence and the front needs a single label per card.
const (
	blockReasonBlacklist  = "blacklist"
	blockReasonStandalone = "standalone"
	blockReasonTracked    = "tracked"
	blockReasonDownloaded = "downloaded"
)

// standaloneGuard answers the only question the POST and the search ask: can this anime be added as
// standalone?
//
// One blocking function, two consumers. It's the same function for both because "the front doesn't
// let you click" and "the back returns an error" have to agree by construction — two definitions
// would produce a grey card the backend accepts, or the reverse.
//
// No new query: the four signals already exist and three of them already come from cache — the
// standalone file and episodes.json are local, tracked/blacklisted come from fetchAniListEntries
// (frontendListCache, 60s) and GetCustomListsMap (5min), exactly the pair GET /animes builds.
type standaloneGuard struct {
	standalone  map[int]bool // standalone_animes
	downloaded  map[int]int  // mediaID → nº de registros em episodes.json
	tracked     map[int]bool // snapshot of what the daemon PROCESSES (see newStandaloneGuard)
	blacklisted map[int]bool // customLists ∩ excluded_lists
}

// blockReason returns "" when the anime can be added, otherwise one of the four reasons.
//
// Precedence: blacklist > standalone > list > downloaded. Blacklist comes first because it's the
// only reason where adding would change the behavior FOR THE WORSE: a blacklisted anime in a status
// outside download_statuses escapes searchAnilist, the standalone record survives the merge and the
// user's filter is bypassed (GetMediaByID's synthetic MediaList has nil CustomLists, so
// animeIsInExcludedList never fires on it). The other three are harmless — the existing records
// already make the loop skip everything — and the message is just clarity.
func (g standaloneGuard) blockReason(mediaID, totalEpisodes int) string {
	switch {
	case g.blacklisted[mediaID]:
		return blockReasonBlacklist
	case g.standalone[mediaID]:
		return blockReasonStandalone
	case g.tracked[mediaID]:
		return blockReasonTracked
	// Only blocks on a KNOWN count: a 24-episode anime with 12 records (the per-anime limit) isn't
	// "already downloaded", and an unknown total doesn't allow claiming anything.
	case totalEpisodes > 0 && g.downloaded[mediaID] >= totalEpisodes:
		return blockReasonDownloaded
	}
	return ""
}

// newStandaloneGuard builds the guard from the state the server already has at hand.
//
// tracked comes from fetchAniListEntries, not from the daemon's searchAnilist: they are different
// queries with THE SAME TWO FILTERS (download_statuses server-side + MediaStatusAllowed), and the
// api package's one is the one already cached on the frontend path. The definition is "shows up in
// the set the daemon PROCESSES", not "there is an AniList entry" — an anime in PLANNING with
// download_statuses = [CURRENT] is in a list and the daemon ignores it, and adding it as standalone
// is the most obvious use case of the feature.
func newStandaloneGuard(fm FileManagerInterface, config *files.Config) (standaloneGuard, error) {
	guard := standaloneGuard{
		standalone:  map[int]bool{},
		downloaded:  map[int]int{},
		tracked:     map[int]bool{},
		blacklisted: map[int]bool{},
	}

	standaloneIDs, err := fm.LoadStandaloneAnimes()
	if err != nil {
		return guard, err
	}
	for _, id := range standaloneIDs {
		guard.standalone[id] = true
	}

	episodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return guard, err
	}
	for _, ep := range episodes {
		if ep.AnimeID != 0 {
			guard.downloaded[ep.AnimeID]++
		}
	}

	excluded := make(map[string]bool, len(config.ExcludedLists))
	for _, name := range config.ExcludedLists {
		excluded[name] = true
	}

	for _, username := range config.AnilistUsernames {
		// nil means a failed fetch (see fetchAniListEntries). Treating that as "nothing tracked" is the
		// right behavior here: the front is best-effort and the POST refuses again with the next call's
		// snapshot. The second return (came from the cache) changes nothing here: the guard wants to know
		// what the account tracks, and the local cache's answer is better than none.
		tracked, _ := fetchAniListEntries(username, config.DownloadStatuses, config.DownloadMediaStatuses)
		for _, ml := range tracked {
			guard.tracked[ml.Media.Id] = true
			if isInExcludedList(ml.CustomLists, excluded) {
				guard.blacklisted[ml.Media.Id] = true
			}
		}
	}
	// A MAL account is never blacklisted: MAL has no custom lists, so excluded_lists can't apply.
	for _, user := range config.MALUsernames {
		tracked, _ := fetchMALEntries(user, config.DownloadStatuses, config.DownloadMediaStatuses)
		for _, ml := range tracked {
			guard.tracked[ml.Media.Id] = true
		}
	}

	return guard, nil
}

func isInExcludedList(customLists anilist.CustomLists, excluded map[string]bool) bool {
	for name, inList := range customLists {
		if inList && excluded[name] {
			return true
		}
	}
	return false
}
