package api

import (
	"errors"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
)

// loadStandaloneSet reads the standalone file as a set. A read failure doesn't take the request
// down: the worst effect is the screen not showing the chip and the standalone's detail going back
// to 404, which is already the behavior for someone with no standalone anime at all.
func loadStandaloneSet(fm FileManagerInterface) map[int]bool {
	ids, err := fm.LoadStandaloneAnimes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load standalone animes, continuing without them")
		return nil
	}
	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// resolveMediaList reads an anime by media id with the standalone fallback.
//
// anilist.GetAnimeInfo returns (nil, nil) for an anime in no account's list, and a standalone anime
// is exactly that case — without the fallback its detail screen doesn't open. The fallback is gated
// on the standalone set on purpose: without that any AniList media id would start answering on the
// /animes/{id}/* routes, and the "this anime isn't yours" 404 would disappear.
func resolveMediaList(fm FileManagerInterface, id int, usernames, malUsers []string, standalone map[int]bool) (*anilist.MediaList, error) {
	ml, err := anilist.GetAnimeInfo(id, usernames, malUsers, anilist.PriorityCritical)
	// Served from the local cache counts as an answer: this was the screen that became entirely
	// unreachable with AniList down, and everything it does besides showing the episode list (block,
	// delete, re-download, mark as manual) is already local. The system banner says AniList is
	// degraded; returning 500 here only hid what still worked.
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, err
	}
	if ml != nil || !standalone[id] {
		// nil here, not err: at this point err is ErrFromCache or nothing, and the only caller
		// (handleAnimeEpisodes) would turn any error into a 500 — which is exactly what's being fixed.
		// What tells the user AniList went down is the banner, through anilist.Health.
		return ml, nil
	}
	ml, err = anilist.GetMediaByID(id, anilist.PriorityCritical)
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, err
	}
	return withStandaloneProgress(fm, ml), nil
}

// appendStandaloneEntries adds the standalone animes to the entries from the AniList lists,
// skipping the ones already in them — the same rule (and the same reason) as the daemon's append:
// the real entry has progress and the synthetic one doesn't.
//
// covered is updated along the way so refreshOrphanAnimes doesn't fetch again, per account, an
// anime no account tracks.
func appendStandaloneEntries(fm FileManagerInterface, entries []anilist.MediaList, standalone, covered map[int]bool) []anilist.MediaList {
	pending := make([]int, 0, len(standalone))
	for id := range standalone {
		if !covered[id] {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return entries
	}

	medias, err := anilist.GetMediaByIDs(pending, anilist.PriorityDisposable)
	switch {
	case errors.Is(err, anilist.ErrFromCache):
		logger.Logger.Warn().
			Msg("Serving standalone animes from the local cache: AniList is unreachable")
	case err != nil:
		logger.Logger.Warn().Err(err).
			Msg("Failed to fetch standalone animes from AniList; leaving the missing ones out of this response")
	}

	for _, id := range pending {
		ml, ok := medias[id]
		if !ok {
			continue // already reported by the batch error
		}
		if ml == nil {
			logger.Logger.Warn().Int("media_id", id).
				Msg("AniList does not know this standalone media id; leaving it out of this response")
			continue
		}
		entries = append(entries, *withStandaloneProgress(fm, ml))
		covered[id] = true
	}
	return entries
}

// withStandaloneProgress — see its twin in daemon/standalone.go. Without it the screen would show 0
// watched for a standalone anime whose progress the user just saved.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}
