# 16. `anime_id` is the AniList **media** ID (it was the MediaList entry ID until the migration)

**Location:** `internal/anilist/anilist.go` (`Media.Id`, `GetAnimeInfo(mediaId, usernames)`); `internal/api/endpoint_animes.go` and `endpoint_anime_episodes.go` (`AnimeID` fields); `daemon/migration.go` (`MigrateAnimeIDsToMedia`).

**What it looks like:** `anime_id` is *the* anime identifier everywhere (settings file, episode records, route parameters) and also the component of the URL `anilist.co/anime/{id}` — there is no longer a separate `anilist_id` in the API.

**Why it's right:** until the migration, the value came from the `id` field of AniList's `MediaList` type, which is the **list-entry** id and therefore **per account**. With more than one account configured, the same anime had two `anime_id`s, and the app treated them as two animes. See decision #43 for the concrete bug and the current design. Old installations are converted once by `MigrateAnimeIDsToMedia`, marked by the config field `anime_ids_are_media_ids`.

**Don't "fix" by:** keying anything off `MediaList.Id` again (it is per-account — see #43); running the migration again after the mark is set (it would read media ids as entry ids).
