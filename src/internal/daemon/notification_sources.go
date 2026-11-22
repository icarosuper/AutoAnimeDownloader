package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"slices"
	"sync"
	"time"
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
// to cost one uncached AniList query per account, on the budget decisions/065 protects, and a
// refused query silently dropped the notification (decisions/126).
//
// An account whose fetch failed keeps its previous answer: stale ownership is closer to the truth
// than none. The lists only cover the download statuses and, for the animes on disk, the delete
// statuses (decisions/130), so an anime in any other status (a manual deletion of a PAUSED
// anime) is unknown, and unknown is fail-open.
type accountLists struct {
	mu   sync.RWMutex
	byID map[listKind]map[string]map[int]bool // kind -> account source key -> media ids
	// at is when each account's list of each kind was last recorded: a list older than the pass
	// means that account's fetch failed in it (fresh and cached both).
	at map[listKind]map[string]time.Time
	// prevFresh is the answer each account gave on its previous recordCarrying, before the union —
	// what the next one carries over for one more pass.
	prevFresh map[listKind]map[string]map[int]bool
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
	if l.at == nil {
		l.at = make(map[listKind]map[string]time.Time)
	}
	if l.at[kind] == nil {
		l.at[kind] = make(map[string]time.Time)
	}
	l.at[kind][account] = time.Now()
}

// recordCarrying is record for a list that only holds what is on disk NOW (the delete half,
// decisions/130): it stores this answer united with the account's previous one. An anime the
// pass deletes leaves the disk, so the next pass no longer asks about it — and its episode_deleted
// can still be waiting in a batch window when that pass records. Carrying the previous answer for
// one pass keeps its owner until the window flushes, as long as batch_window_seconds is shorter
// than two check intervals; carrying it forever would grow the list with every anime ever deleted.
func (l *accountLists) recordCarrying(kind listKind, account string, fresh map[int]bool) {
	l.mu.Lock()
	if l.prevFresh == nil {
		l.prevFresh = make(map[listKind]map[string]map[int]bool)
	}
	if l.prevFresh[kind] == nil {
		l.prevFresh[kind] = make(map[string]map[int]bool)
	}
	union := make(map[int]bool, len(fresh))
	for id := range l.prevFresh[kind][account] {
		union[id] = true
	}
	for id := range fresh {
		union[id] = true
	}
	l.prevFresh[kind][account] = fresh
	l.mu.Unlock()
	l.record(kind, account, union)
}

// allRecordedSince reports whether every account answered its list of this kind at or after t —
// false when any of them failed in the pass that started at t.
func (l *accountLists) allRecordedSince(kind listKind, accounts []string, t time.Time) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, account := range accounts {
		if at, ok := l.at[kind][account]; !ok || at.Before(t) {
			return false
		}
	}
	return true
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
