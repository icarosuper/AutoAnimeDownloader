# 52. The episode list is synthesized, and an episode's key is (anime, number)

**Location:** `anilist/episodes.go` — `EpisodeList`, `lastAiredEpisode`; `anilist/anilist.go` — `Media.NextAiringEpisode`; `files/filemanager.go` — `EpisodeKey`, `EpisodeStruct.Key()`, `loadBlockedEpisodesLocked`; `daemon/episodes.go` — `firstEpisodeToConsider`; `api/endpoint_episode_actions.go` and the `/animes/{id}/episodes/{episodeNumber}/*` routes.

**What it looks like:** **fabricated** `AiringNode`s (id zero) for episodes AniList does not return; `EpisodeStruct` without `EpisodeID`; `blocked_episodes` storing objects; and the routes' `{episodeId}` becoming `{episodeNumber}`.

**Why it's right:**

**(a) `airingSchedule` is not an anime's episode list.** AniList keeps a **window** of ~500 schedule entries per media and discards the old ones. Measured on the API: `Media(21)` (One Piece) has `pageInfo.total = 500` and page 1 **starts at episode 1123** — 1 to 1122 do not exist on the API; `Media(20)` (Naruto, 220 eps) and `Media(16498)` (Shingeki no Kyojin, 25 eps) come back with `total = 0`, an **empty** schedule. While the list came from there, the daemon literally had no episode to process on those animes (the log showed `Processing anime episodes` and nothing after), and on One Piece it started at 1123 as if the earlier ones did not exist. On top of that the code never paginated the schedule, so even inside the window only the first 25 nodes arrived — and paginating would solve nothing, because later pages only bring **newer** episodes.

**(b) That is why the list is synthesized, and the last aired comes from three sources.** `lastAiredEpisode` combines the highest already-aired episode from the schedule, `nextAiringEpisode.episode - 1` and `media.episodes` — the last **only when `FINISHED`**: on a `RELEASING` it is the season's EXPECTED count, and using it would send the daemon hunting an episode that has not aired yet. None of the three works alone (clipped/empty schedule, null `nextAiringEpisode` on a finished anime).

**(c) The key became `(AnimeID, EpisodeNumber)`, not a synthetic id.** An episode outside the window has no node id — and inventing one (`-(mediaID*10000+ep)`) would make an episode's identity depend on a formula nothing validates, with silent collision as the failure mode. Episode number inside an anime is unique by definition and is what the user sees. `EpisodeStruct.EpisodeID` was **removed** (not left as a dead field) on purpose: it is the compiler that points at every place that still identified an episode by the old id.

**(d) Legacy `blocked_episodes` is discarded, not migrated.** It was `[416348, ...]` — node ids that nothing resolves anymore. A parse that fails would stall `LoadBlockedEpisodes` and with it the verification pass, so the old file is read, logged as a warning and treated as an empty list. The user remakes the blocks with one click per episode; guessing would be worse.

**(e) `firstEpisodeToConsider` walks back to the smallest saved episode.** The rule is `Progress + 1` (and a standalone anime has `Progress = 0`, so it starts at 1 with no flag needed). But `watched_episodes_to_keep` and watched pruning only operate on episodes that appear in the list: a saved episode below progress would sit outside it, never be "checked" and fall straight into `identifyEpisodesNotInWatching` — deleted ignoring "how many watched to keep".

**(f) `{episodeId}` became `{episodeNumber}` on the routes, and `episode_id` left the JSON.** The routes already had the right shape; keeping the old name with new semantics would be the worst possible combination. `AnimeInfo.latest_episode_id` became `latest_episode_number` for the same reason.

**Don't "fix" by:**
- Paginating `airingSchedule` "to get the missing episodes": later pages only have newer episodes (see (a)).
- Using `media.episodes` as last aired on a `RELEASING` anime (see (b)).
- Reintroducing a synthetic `EpisodeID` "to not touch on-disk state" (see (c)).
- Trying to convert the old `blocked_episodes` (see (d)).
- Making the list start at `Progress + 1` without the walk-back (see (e)).
