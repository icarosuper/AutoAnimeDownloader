# Manual episode download skips the size and seeder filters

Found on 2026-11-22 while running /finish on the manual-download fallback fix (`8497d70`). Not
started. Delete this file when it is fixed or dropped.

## Symptom

`ManualDownloadEpisode` (`src/internal/daemon/manual_download.go`) hands the ranked search results
straight to `addAndPrioritize`. The scheduled pass (`episodes.go`, the `searchSingleEpisode` call)
and `RunAnimeDebug` (`debug.go`) wrap the same results in `filterSearchResults`, which drops
torrents over `max_episode_torrent_size_gb` or under `min_seeders`.

So with `max_episode_torrent_size_gb = 2`, a manual download can add a 10 GiB release the pass would
refuse, or a dead one with 0 seeders. Since `8497d70` the offset and split-entry fallbacks run on
the manual path too, so it reaches more releases that the limits would have cut.

## What to check

- Whether a manual download should honor the limits at all. A user who clicks "download" on one
  episode may want it even above the ceiling, and the disk guard (`checkDiskSpace`) already runs.
  Grep `docs/agents/decisions/` for `filterSearchResults` and `min_seeders` before deciding.
- If it should: wrap the call in `filterSearchResults(…, configs.MaxEpisodeTorrentSizeGB,
  configs.MinSeeders)` and return an error that names the filter when every result was dropped, so
  the UI doesn't just say "no torrents found".
- If it shouldn't: record that as a decision, so nobody "fixes" the difference later.
