# Live pass on a multi-part franchise (Attack on Titan)

Added on 2026-10-12 (moved from TODO.md on 2026-11-22). Not started. Delete this file when it is
done or dropped.

## Gap

The per-series absolute axis, the `packAxis` and ownership by coverage
([#77](../agents/decisions/077-the-per-series-absolute-axis-is-a-bfs-two.md)/[#78](../agents/decisions/078-a-torrents-unit-of-ownership-is-coverage-on-the.md)/[#79](../agents/decisions/079-pack-choice-asks-does-it-cover-the-window-not-is.md)/[#84](../agents/decisions/084-a-pack-without-a-range-in-its-name-gets-its.md))
were written in August and only have unit tests plus a **listing** measurement. No real end-to-end
download has run on the S1 → Final Season Part 3 chain, which is exactly where the four numbering
conventions from the "Pack granularity and numbering" section of [sources.md](../agents/sources.md)
collide.

The download hygiene from 2026-09-07 (disk guard with torrent size, failures keeping bytes, stalled
torrents dropped —
[#93](../agents/decisions/093-the-disk-guard-has-two-triggers-and-the-size-one.md),
[#24](../agents/decisions/024-a-failed-torrent-is-dropped-from-the-session-and.md),
[#94](../agents/decisions/094-a-stalled-torrent-is-dropped-through-the-failure.md))
was never measured live either; this pass is what actually exercises it.

## What to check

The instrument already exists: `scripts/robustness-animes.txt` + `make debug-batch`.
