# 62. Standalone progress lives in `AnimeSettings` and is injected into the synthetic `MediaList`, it does not become a parallel path

**Location:** `files/filemanager.go` — `AnimeSettings.Progress`; `api/endpoint_anime_settings.go` — `handleAnimeSettings` (partial `PUT`); `daemon/standalone.go` / `api/standalone.go` — `withStandaloneProgress` (two copies, one per package, same contract); `daemon/manual_download.go` — `resolveAnimeDetails`.

**What it looks like:** `AnimeSettings` (per anime, a file separate from `Config`) gains a `Progress int` field; every time a synthetic standalone `MediaList` is built (`appendStandaloneAnimes`, `DownloadStandaloneAnime`, `resolveAnimeDetails` when `GetAnimeInfo` returns `nil`), `withStandaloneProgress` overwrites `ml.Progress` with the saved value before the `MediaList` is used by anything.

**Why it's right:** with the data living in the same field a real list entry uses, `shouldSkipEpisode` (skips episode `<= Progress`), `firstEpisodeToConsider` (starts at `Progress + 1`), `buildWatchedKeepSet` (uses `Progress` to find which saved ones count as watched) and the UI's `EpisodesWatched` count work on standalone **with no `if isStandalone`** — it is the same read as always, only the source of truth for standalone is `AnimeSettings` instead of AniList. A parallel path (an explicit branch at each of those points) would duplicate the "what is watched" logic in four places and one of them would diverge sooner or later.

`resolveAnimeDetails` calls `withStandaloneProgress` on the SAME fallback branch `appendStandaloneAnimes` uses (`GetAnimeInfo` returns `nil` → `GetMediaByID`) — without that, `RunAnimeDebug` (which goes through `resolveAnimeDetails`) reported a standalone with saved progress as if it started at episode 1, diverging from the real pipeline it exists to mirror (#58). On `ManualDownload*` the effect is harmless — they select the episode by explicit number, not by `Progress` — but the call is the same so it does not depend on which path a future caller picks.

`PUT /animes/{id}/settings` uses a pointer (`*int`) and does a partial merge: a `PUT` whose body does not mention `progress` cannot zero it — a fresh `AnimeSettings{}` on every `PUT` would silently wipe the saved progress. The pointer is still needed with a single field, because `0` is valid progress and indistinguishable from absent without it. `progress < 0` is rejected with 400 in the handler validation, before the merge.

**Don't "fix" by:**
- An `if anime.IsStandalone` branch in `shouldSkipEpisode`/`firstEpisodeToConsider`/`buildWatchedKeepSet` "to make it explicit": that is exactly the duplication injecting into the `MediaList` avoids.
- A `PUT` replacing the whole `AnimeSettings`, or swapping the `*int` for `int` "because only one field is left": both zero progress on a body that did not mention it.
- Skipping `withStandaloneProgress` in `resolveAnimeDetails` "because `ManualDownload*` does not use `Progress`": `RunAnimeDebug` does, and both go through the same path.
