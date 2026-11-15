package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"slices"
	"sync"
)

// listKind tells apart the two per-account lists a pass fetches: the download-status one
// (searchAnilist) and the delete-status one. Each replaces only its own half of the snapshot.
type listKind int

const (
	listDownload listKind = iota
	listDelete
)

// accountLists is the last list each account answered, per kind: which media ids it holds. It is
// what the per-webhook source filter reads instead of asking AniList at fire time — a webhook used
// to cost one uncached AniList query per account, on the budget decisions.md #65 protects, and a
// refused query silently dropped the notification (decisions.md #126).
//
// An account whose fetch failed keeps its previous answer: stale ownership is closer to the truth
// than none. The lists only cover the download and delete statuses, so an anime in any other status
// (a manual deletion of a PAUSED anime) is unknown, and unknown is fail-open.
type accountLists struct {
	mu   sync.RWMutex
	byID map[listKind]map[string]map[int]bool // kind -> account source key -> media ids
}

var passLists = &accountLists{byID: make(map[listKind]map[string]map[int]bool)}

// record stores one account's list of one kind, replacing what that account had for that kind.
func (l *accountLists) record(kind listKind, account string, ids map[int]bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byID[kind] == nil {
		l.byID[kind] = make(map[string]map[int]bool)
	}
	l.byID[kind][account] = ids
}

// owners returns, for each id, the configured accounts whose last known lists hold it. Accounts no
// longer in the config don't count, even if a snapshot of theirs is still around.
func (l *accountLists) owners(accounts []string, ids []int) map[int][]string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[int][]string)
	for _, account := range accounts {
		for _, id := range ids {
			for _, byAccount := range l.byID {
				if byAccount[account][id] {
					out[id] = append(out[id], account)
					break
				}
			}
		}
	}
	return out
}

// configuredAccounts is every list account in the config, as notifications.AccountSource keys.
func configuredAccounts(cfg *files.Config) []string {
	accounts := make([]string, 0, len(cfg.AnilistUsernames)+len(cfg.MALUsernames))
	for _, u := range cfg.AnilistUsernames {
		accounts = append(accounts, notifications.AccountSource(anilist.AccountSourceAniList, u))
	}
	for _, u := range cfg.MALUsernames {
		accounts = append(accounts, notifications.AccountSource(anilist.AccountSourceMAL, u))
	}
	return accounts
}

// NotificationSources builds the resolver behind the per-webhook source filter
// (files.WebhookPreset.AnimeSources): an anime's sources are the accounts whose last fetched lists
// hold it, plus "standalone" when the standalone file tracks it. No network — it runs on the
// shutdown Flush too. An anime no list holds and the standalone file doesn't track is left out of
// the map, and the notifications package sends it to every filtered webhook (fail-open).
func NotificationSources(fm FileManagerInterface) notifications.SourceResolver {
	return func(cfg *files.Config, animeIDs []int) map[int][]string {
		out := passLists.owners(configuredAccounts(cfg), animeIDs)

		standalone, err := fm.LoadStandaloneAnimes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Notification source filter: failed to load the standalone animes")
		}
		for _, id := range animeIDs {
			if slices.Contains(standalone, id) {
				out[id] = append(out[id], notifications.SourceStandalone)
			}
		}
		return out
	}
}
