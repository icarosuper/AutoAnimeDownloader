package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/notifications"
	"slices"
)

// NotificationSources builds the resolver behind the per-webhook source filter
// (files.WebhookPreset.AnimeSources). An anime's sources are the configured accounts that have it on
// their list RIGHT NOW, in any list status, plus "standalone" when the standalone file tracks it.
//
// Any status on purpose: an episode_deleted usually fires for an anime every account already
// finished (COMPLETED, outside the download statuses), and it still is that account's anime. It
// is asked at fire time instead of being stored on the episode record, so records from before the
// filter existed are covered too (decisions.md #126).
//
// One AniList query per account per 50 ids (GetAnimeInfoByIDs), only for windows that reach a
// filtered webhook. That query is PriorityDisposable: with the AniList budget low it is refused,
// the ids come back unknown and the filtered webhooks skip them — a filter can't prove ownership
// it couldn't look up. Webhooks without a filter are never affected.
func NotificationSources(fm FileManagerInterface) notifications.SourceResolver {
	return func(cfg *files.Config, animeIDs []int) map[int][]string {
		out := make(map[int][]string, len(animeIDs))

		if len(cfg.AnilistUsernames)+len(cfg.MALUsernames) > 0 {
			medias, err := anilist.GetAnimeInfoByIDs(animeIDs, cfg.AnilistUsernames, cfg.MALUsernames)
			if err != nil {
				// Partial: the map still holds the accounts that answered.
				logger.Logger.Warn().Err(err).Int("media_ids", len(animeIDs)).
					Msg("Notification source filter: could not resolve every account; unresolved animes skip filtered webhooks")
			}
			for id, ml := range medias {
				for _, acc := range ml.Accounts {
					out[id] = append(out[id], notifications.AccountSource(acc.Source, acc.Username))
				}
			}
		}

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
