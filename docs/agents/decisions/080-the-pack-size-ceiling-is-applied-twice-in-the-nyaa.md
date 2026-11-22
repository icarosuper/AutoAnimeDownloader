# 80. The pack size ceiling is applied TWICE: in the Nyaa search and in the daemon filter

**Location:** `nyaa/nyaa.go` — `maxBatchSizeBytes`, `SetMaxBatchTorrentSizeGB`, `batchTooBig`, applied in `ScrapNyaaForAnime`'s `parseRow`; and `daemon/search.go` — `filterBySize`, via `partitionSearchResults`.

**What it looks like:** `max_batch_torrent_size_gb` is already applied by the daemon in `filterBySize`, and yet the `nyaa` package receives the same ceiling by push (`applyNyaaSettings`) and drops the pack row before adding it to the result. Looks like a duplicated filter, and `ScrapNyaaForAnime`'s old comment said explicitly that pushing a filter into `nyaa` was what `applyNyaaSettings` avoided.

**Why it's right:** the two filters do not do the same thing, because they run at different moments of the algorithm. The daemon's decides **what to download**. nyaa's decides **how far to descend the pages** — `fetchSearchPages` stops when `accepted()` reaches `enoughCandidates` (3), and `accepted()` counts what the search accepted. A pack the daemon will discard for size, but that the search accepted, spends one of the three floor slots and ends the descent too early.

Measured on Nyaa (August 2026, `?q=one+piece&s=seeders&o=desc`, which is exactly the query `ScrapNyaaForAnime` builds):

| page | pack | size |
|---|---|---|
| 1 | (complete pack) | 587.5 GiB |
| 1 | `[Judas] One Piece 001-574` | 171.8 GiB |
| 2 | `[Erai-raws] One Piece 0892~1089` | 190.3 GiB |
| 2 | (complete pack) | 1.3 TiB |
| 2 | `[Judas] One Piece 783-1023` | 86.2 GiB |
| 3 | `[Judas] One Piece 575-782` | 61.0 GiB |
| 3 | `One Piece S01E1109-E1122` | 20.0 GiB |
| 3 | `[ToonsHub] One Piece EP1144-EP1155` | 16.6 GiB |

The first three accepted are all giants, the floor closes, and the 86 / 61 / 20 / 16 GiB packs — which **fit** the default ceiling of 100 — are never seen. The reported symptom was "One Piece never downloads because every pack found is over the ceiling"; the cause is not the ceiling being low, it is the search stopping before finding the ones that fit. Raising the ceiling to 200 would hide the bug and still queue 171 GiB in place of 61 GiB.

Pushing config into `nyaa` is not a new pattern: it is the same as `SetPriorities` and `SetMaxSearchPages` (`nyaa` cannot import `files` — cycle — so config arrives by push). The default is `0` (off), so the package used without a push behaves as before.

**Accepted trade-off:** the pack ceiling appears in two places and needs to stay coherent between them (same `Size == 0` passes free rule). The daemon's remains the authority — if the push does not happen, nothing leaks into a download.

**Don't "fix" by:**
- Removing nyaa's filter "because `filterBySize` already does that" — that is exactly what breaks pagination.
- Removing the daemon's filter "because `nyaa` already filters now" — `filterBySize` also covers `max_episode_torrent_size_gb` and is what guarantees the result when config was not pushed (tests, direct use of the package).
- Pushing `min_seeders` into `nyaa` too "for symmetry" — Nyaa already returns ordered by seeders desc, so the seeders filter does not truncate the descent the same way: the best come first by construction.
- Raising `max_batch_torrent_size_gb` as a "solution" for a long series — see above.
