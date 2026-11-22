# 48. `max_episodes_per_anime` never limited batches — the rule now lives on the pack window, not a count cap

**See also:** [#59](059-batch-eligibility-stopped-being-metadata-and.md) — pack eligibility is the filtered search result, not a count cap (`max_batch_episodes`, removed). Entries 59-62 document the current design.

**Location (historical):** when this decision was written, `daemon/episodes.go` had `willBatchAnime` and `resolveSearchStrategy`; both were removed by the batch-by-filter spec. What exists today: `daemon/episodes.go` — `selectEpisodes`, `effectiveMax`, `windowEnd`, `partitionSearchResults`, `pickBatches`, `packSet.covering`, `assignBatches`; `daemon/search.go` — `filterBySize`; `daemon/helpers.go` — `checkDiskSpace`/`ErrInsufficientDiskSpace`; `daemon/jobs.go` — `partiallyOrganized`; `api/endpoint_status.go` — `disk_low`.

**What it looks like:** a per-anime limit that sometimes does not apply, a size filter that lets size zero through, a disk guard that only bars `Add`, and a "disk low" boolean computed on the server for the frontend.

**Why it's right:**

**(a) The limit applies only episode by episode.** A batch is **one torrent**: a finished 24-episode anime with the limit at 12 downloaded the 24 files, `organize` hardlinked the 24, and `episodes.json` recorded 12 — the screen showed 12/24 forever with the 24 on disk. Limiting records does not limit bytes or files; it only produces a lying count. So, when a pack covers the pending window, the second `selectEpisodes` runs with the limit lifted (`len(episodes)+1`), and the effective limit becomes unlimited for that pack's episodes.

What decides pack eligibility today is **no longer** a count cap on the anime's episodes (`max_batch_episodes`, removed) — it is the filtered search result itself: `partitionSearchResults` splits packs from loose episodes and applies to each its OWN size ceiling (`max_batch_torrent_size_gb`/`max_episode_torrent_size_gb`) and the seeders floor; `pickBatches` picks the fewest packs that cover `[firstPending, firstPending + max_episodes_per_anime - 1]` (or the whole window, with the cap off). See entry 59 for why that swap.

**(b) The fallback.** If the pack search does not cover the pending window (no results, or the size filter empties it), the original selection — with the per-anime limit in force — remains: `episodesToDownload` was never replaced, so there is nothing to cut back. `airingSchedule` order guarantees the oldest ones are what stay.

**(c) `Size == 0` passes the size filter.** That is `nyaa.parseSize`'s value when Nyaa changes the column format. Discarding unknown size would swap "sometimes downloads a large torrent" for "downloads nothing": a parsing bug would become a silent stall. The filter runs **after** priority sort and preserves order, so the pick is the best **among those that fit**.

**(d) The disk guard bars `Add`, not the pass.** Watched pruning, `deleteEpisodesByStatus` and `organize` are exactly what **frees** space; an `if disk full { return }` at the start of the pass would leave the app stuck in the state where it cannot unclog itself. A `statfs` error does not block — a volume that does not answer (network, permission) is not proof of a full disk. When it bars, it tries no magnet and does not retry (the magnet is not the problem) and, since [#128](128-download-failed-reports-a-real-failure-once-per.md), does not notify per episode: the pass's `disk_full` issues feed `disk_low`'s `blocked` level, said once for the whole disk.

**(e) The disk threshold lives on the server.** `GET /status` returns `disk_low`, which is the add-gate for a torrent of size 0: `free - reserved` against `min_free_disk_gb` ([#93](093-the-disk-guard-has-two-triggers-and-the-size-one.md)). One place decides what "low" is, and the screen warning cannot disagree with what the daemon is doing.

**(f) `organizeTorrent` does not re-notify a partially organized group.** If part of the group already has `LibraryPaths`, the torrent already landed and the webhook already went out: it organizes (it is idempotent), writes the marker on the new records, does not notify. Without that, the first post-upgrade pass of an existing library — where rule (a) creates the missing records for packs already on disk — would become a flood of duplicate `download_completed`.

**Don't "fix" by:**
- Applying `max_episodes_per_anime` to batches "so the limit always holds": that brings the lying count back.
- Reintroducing an episode-count cap on the anime (like `max_batch_episodes`) to decide pack eligibility: episode count is a bad proxy for size — see #59.
- Discarding a torrent with `Size == 0` "because unknown size is suspicious".
- Consolidating the disk guard into a `return` at the top of the verification pass (see (d)).
- Moving the guard into `torrents.Session.Add` "because it's one site": the `torrents` package does not know `files.Config`, and passing config there inverts the dependency.
- Reintroducing a hardcoded ratio in `lib/utils/status.ts` to save a field on the response (see (e)).
