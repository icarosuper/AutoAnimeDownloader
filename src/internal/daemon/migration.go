package daemon

import (
	"errors"
	"fmt"
	"strings"
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
// account) to MEDIA id — see decisions/043. Until it runs, the records on disk are keyed in a
// way the rest of the code no longer understands, so the caller must ABORT the pass when it
// fails: going ahead would make every anime look not-downloaded and the daemon would re-download
// the whole library.
//
// It is idempotent via a completion mark kept in TWO places: the AnimeIDsAreMediaIDs config field
// and a marker file next to the data (FileManager.MarkAnimeIDs). Either one skips the migration,
// and a run that finds only one writes the other, so the mark follows the data when a data dir
// gets an older config.json (decisions/043). It only writes after resolving EVERY entry: a
// mid-run network failure does not leave half the records converted.
//
// Entries that no longer exist on AniList (the user removed the anime from the list) cannot be
// resolved; those records keep the old id and are only logged. The episodes stay on disk and
// listed — they just won't match AniList again if the anime is re-added.
//
// An entry owned by an account NOT in anilist_usernames keeps its id too: a data dir moved before
// the marker existed, with an older config.json, reruns on ids that are already media ids, and each
// resolves to a stranger's entry (decisions/043). Logged under ids_kept_not_own_entries.
//
// WHEN THIS CAN BE DELETED: the migration landed in v2.1.1 (2026-08-07). It is only needed for
// whoever upgrades from a v2.1.0 or EARLIER install — anyone who went through any version from
// v2.1.1 onward already has AnimeIDsAreMediaIDs set in the config and returns at the mark check
// (writing the missing half of the mark, best effort, the first time). Deleting it before that boundary is ancient history is not a light degradation:
// without the conversion, NOTHING on disk matches AniList, every anime looks not-downloaded and
// the first pass re-downloads the whole library, silently — to the daemon that is indistinguishable
// from a legitimate empty library. The project is distributed (AUR + releases), so the question
// is not "has mine already migrated".
//
// Cost of keeping it until then: a LoadConfigs and a Stat, per boot and per pass. Negligible.
func MigrateAnimeIDsToMedia(fm FileManagerInterface) error {
	configs, err := fm.LoadConfigs()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load configs: %w", err)
	}
	marked, err := fm.AnimeIDsMarked()
	if err != nil {
		if configs.AnimeIDsAreMediaIDs {
			// Already migrated by the config half; an unreadable data dir must not block the pass.
			logger.Logger.Warn().Err(err).Msg("Migration: failed to read the anime id marker; the config says migrated")
			return nil
		}
		return fmt.Errorf("anime id migration: failed to read the data dir marker: %w", err)
	}
	if configs.AnimeIDsAreMediaIDs || marked {
		// The ids are media ids either way: completing the mark is best effort and never blocks.
		if err := completeAnimeIDsMark(fm, configs, marked); err != nil {
			logger.Logger.Warn().Err(err).Msg("Migration: failed to complete the anime id mark")
		}
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

	// Ownership is judged by name, so every configured name must be a live account: one renamed
	// since would make all its entries "foreign", and the mark below would close the migration
	// with nothing converted. Abort instead; the pass stays blocked until the config is fixed.
	ownAccounts := make(map[string]bool, len(configs.AnilistUsernames))
	for _, name := range configs.AnilistUsernames {
		name = strings.TrimSpace(name)
		if len(oldIDs) > 0 {
			exists, err := anilist.AniListUserExists(name)
			if err != nil {
				return fmt.Errorf("anime id migration: failed to check AniList account %q: %w", name, err)
			}
			time.Sleep(migrateAnimeIDsThrottle)
			if !exists {
				return fmt.Errorf("anime id migration: AniList account %q does not exist (renamed?); fix anilist_usernames", name)
			}
		}
		ownAccounts[strings.ToLower(name)] = true
	}

	mediaByEntry := make(map[int]int, len(oldIDs))
	var unresolved, foreign []int
	for entryID := range oldIDs {
		mediaID, owner, err := anilist.GetMediaIDForEntry(entryID)
		if err != nil {
			// Nothing has been written yet: the next pass restarts the whole migration.
			return fmt.Errorf("anime id migration: failed to resolve entry %d: %w", entryID, err)
		}
		time.Sleep(migrateAnimeIDsThrottle)
		if mediaID == 0 {
			unresolved = append(unresolved, entryID)
			continue
		}
		if !ownAccounts[strings.ToLower(owner)] {
			foreign = append(foreign, entryID)
			continue
		}
		mediaByEntry[entryID] = mediaID
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

	if err := completeAnimeIDsMark(fm, configs, false); err != nil {
		return err
	}

	logger.Logger.Info().
		Int("entries_resolved", len(mediaByEntry)).
		Int("episodes_rewritten", len(updated)).
		Ints("entries_no_longer_on_anilist", unresolved).
		Ints("ids_kept_not_own_entries", foreign).
		Msg("Migration: anime IDs are now AniList media IDs")

	return nil
}

// completeAnimeIDsMark writes whichever half of the completion mark is missing: the marker file
// (an install migrated before it existed) or the config field (a data dir that arrived with an
// older config.json). One half is enough to skip the next run, so it errs only when neither half
// ends up set; a failure of one half alone is logged.
func completeAnimeIDsMark(fm FileManagerInterface, configs *files.Config, marked bool) error {
	var errs []error
	if !marked {
		if err := fm.MarkAnimeIDs(); err != nil {
			errs = append(errs, fmt.Errorf("data dir marker: %w", err))
		} else {
			marked = true
		}
	}
	if !configs.AnimeIDsAreMediaIDs {
		configs.AnimeIDsAreMediaIDs = true
		if err := fm.SaveConfigs(configs); err != nil {
			configs.AnimeIDsAreMediaIDs = false
			errs = append(errs, fmt.Errorf("config field: %w", err))
		}
	}
	if !marked && !configs.AnimeIDsAreMediaIDs {
		return fmt.Errorf("anime id migration: failed to record completion: %w", errors.Join(errs...))
	}
	if len(errs) > 0 {
		logger.Logger.Warn().Err(errors.Join(errs...)).Msg("Migration: one half of the anime id mark could not be written")
	}
	return nil
}
