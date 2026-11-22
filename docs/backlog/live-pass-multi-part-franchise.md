# Live pass on a multi-part franchise (Attack on Titan)

Added on 2026-10-12 (moved from TODO.md on 2026-11-22). Not started. Delete this file when it is
done or dropped.

## Gap

The per-series absolute axis, the `packAxis` and ownership by coverage
([#77](../agents/decisions.md#77-the-per-series-absolute-axis-is-a-bfs-two-generations-at-a-time-and-the-cut-level-goes-back-to-the-queue)/[#78](../agents/decisions.md#78-a-torrents-unit-of-ownership-is-coverage-on-the-absolute-axis-not-the-anime_id-episode-key)/[#79](../agents/decisions.md#79-pack-choice-asks-does-it-cover-the-window-not-is-it-part-n--and-pack-numbering-is-a-guess-among-three-hypotheses)/[#84](../agents/decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything))
were written in August and only have unit tests plus a **listing** measurement. No real end-to-end
download has run on the S1 → Final Season Part 3 chain, which is exactly where the four numbering
conventions from the "Pack granularity and numbering" section of [sources.md](../agents/sources.md)
collide.

The download hygiene from 2026-09-07 (disk guard with torrent size, failures keeping bytes, stalled
torrents dropped —
[#93](../agents/decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_gb--0),
[#24](../agents/decisions.md#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist),
[#94](../agents/decisions.md#94-a-stalled-torrent-is-dropped-through-the-failure-path-and-only-one-that-is-trying-counts-as-stalled))
was never measured live either; this pass is what actually exercises it.

## What to check

The instrument already exists: `scripts/robustness-animes.txt` + `make debug-batch`.
