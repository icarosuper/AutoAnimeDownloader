package daemon

import (
	"fmt"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
)

// migrateAnimeIDsThrottle spaces out the migration queries. AniList degrades to ~30
// requests per minute under load and the client has no retry: a migration that fires 50
// queries in a burst comes back with 429, aborts, and retries the burst on the next pass.
var migrateAnimeIDsThrottle = 250 * time.Millisecond

// MigrateAnimeIDsToMedia converts the persisted AnimeID from ENTRY id (MediaList, which is per
// account) to MEDIA id — see decisions.md #43. Until it runs, the records on disk are keyed in a
// way the rest of the code no longer understands, so the caller must ABORT the pass when it
// fails: going ahead would make every anime look not-downloaded and the daemon would re-download
// the whole library.
//
// It is idempotent via the AnimeIDsAreMediaIDs config field, and only writes after resolving
// EVERY entry: a mid-run network failure does not leave half the records converted.
//
// Entries that no longer exist on AniList (the user removed the anime from the list) cannot be
// resolved; those records keep the old id and are only logged. The episodes stay on disk and
// listed — they just won't match AniList again if the anime is re-added.
//
// WHEN THIS CAN BE DELETED: the migration landed in v2.1.1 (2026-08-07). It is only needed for
// whoever upgrades from a v2.1.0 or EARLIER install — anyone who went through any version from
// v2.1.1 onward already has AnimeIDsAreMediaIDs set in the config and hits the return nil on the
// third line. Deleting it before that boundary is ancient history is not a light degradation:
// without the conversion, NOTHING on disk matches AniList, every anime looks not-downloaded and
// the first pass re-downloads the whole library, silently — to the daemon that is indistinguishable
// from a legitimate empty library. The project is distributed (AUR + releases), so the question
// is not "has mine already migrated".
//
// Cost of keeping it until then: a LoadConfigs and an if, per boot and per pass. Negligible.
func MigrateAnimeIDsToMedia(fm FileManagerInterface) error {
	configs, err := fm.LoadConfigs()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load configs: %w", err)
	}
	if configs.AnimeIDsAreMediaIDs {
		return nil
	}

	episodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load saved episodes: %w", err)
	}
	settings, err := fm.LoadAllAnimeSettings()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load anime settings: %w", err)
	}

	oldIDs := make(map[int]bool)
	for _, ep := range episodes {
		if ep.AnimeID != 0 {
			oldIDs[ep.AnimeID] = true
		}
	}
	for animeID := range settings {
		if animeID != 0 {
			oldIDs[animeID] = true
		}
	}

	mediaByEntry := make(map[int]int, len(oldIDs))
	var unresolved []int
	for entryID := range oldIDs {
		mediaID, err := anilist.GetMediaIDForEntry(entryID)
		if err != nil {
			// Nothing has been written yet: the next pass restarts the whole migration.
			return fmt.Errorf("anime id migration: failed to resolve entry %d: %w", entryID, err)
		}
		if mediaID == 0 {
			unresolved = append(unresolved, entryID)
			continue
		}
		mediaByEntry[entryID] = mediaID
		time.Sleep(migrateAnimeIDsThrottle)
	}

	var updated []files.EpisodeStruct
	for _, ep := range episodes {
		if mediaID, ok := mediaByEntry[ep.AnimeID]; ok && mediaID != ep.AnimeID {
			ep.AnimeID = mediaID
			updated = append(updated, ep)
		}
	}
	if err := fm.UpsertEpisodes(updated); err != nil {
		return fmt.Errorf("anime id migration: failed to rewrite episodes: %w", err)
	}

	// The old keys stay in the settings file: there is no removal in the interface and an orphan
	// settings entry does not get in the way. Copying is what matters.
	for entryID, s := range settings {
		mediaID, ok := mediaByEntry[entryID]
		if !ok || mediaID == entryID {
			continue
		}
		if err := fm.SaveAnimeSettings(mediaID, s); err != nil {
			return fmt.Errorf("anime id migration: failed to rewrite settings for anime %d: %w", mediaID, err)
		}
	}

	configs.AnimeIDsAreMediaIDs = true
	if err := fm.SaveConfigs(configs); err != nil {
		return fmt.Errorf("anime id migration: failed to record completion: %w", err)
	}

	logger.Logger.Info().
		Int("entries_resolved", len(mediaByEntry)).
		Int("episodes_rewritten", len(updated)).
		Ints("entries_no_longer_on_anilist", unresolved).
		Msg("Migration: anime IDs are now AniList media IDs")

	return nil
}
