# 54. Seeders floor: unreadable seeders are discarded, unreadable size passes

**Location:** `daemon/search.go` — `filterBySeeders`, `filterSearchResults`, `filterBySize`; `files/filemanager.go` — `Config.MinSeeders` (default `1`); `nyaa/nyaa.go` — `ParseSeeders`.

**What it looks like:** two filters with an almost identical contract and an asymmetry of purpose — `Size == 0` (parse failed) **passes** the size ceiling, `seeders == 0` (including `"-"`) **does not pass** the floor.

**Why it's right:**

**(a) With no floor, a dead torrent is a valid candidate — and it wins when it is the only one.** Measured: `[DB] Naruto Shippuuden Movie 3` with **0 peers** was episode 3's only candidate and was downloaded. The default `1` bars only that; `0` turns it off, following the `max_batch_torrent_size_gb` convention.

**(b) The asymmetry with `filterBySize` is the point, not an oversight.** Unknown size means "I don't know if it fits", and discarding would swap "sometimes downloads a large torrent" for "downloads nothing" — a parsing bug would become a silent stall. Unknown seeders is different: the column is the **only** signal that someone is seeding, and a torrent with no seeder never completes. Letting it through is not the conservative side — it is pinning the episode on a torrent that does not download.

**(c) The filter stayed in the daemon, not in nyaa's `parseRow`.** The plan asked for it in parse, "before any sort". Because the sort is stable and the filters preserve order, the pick is the same; and in the daemon it reuses `filterBySize`'s pattern, holds for every search call site at one point (`filterSearchResults`) and keeps nyaa's `"Found N results"` log showing the dead torrent with its seeders — which is what diagnoses. `nyaa.ParseSeeders` exists only for that (same pattern as `IsBatch`/`ExtractSeason`).

**Don't "fix" by:**
- Making `Size == 0` discarded "for symmetry with seeders" (see (b)).
- Raising the default to "only a healthy torrent": ranking is `health`'s job, not the floor's — the floor exists for what does not download at all.
