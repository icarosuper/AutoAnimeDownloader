# Old and long anime: download part of a pack

Added on 2026-10-12 (moved from TODO.md on 2026-11-22). Not started. Ships as a patch whenever it
is tackled. Delete this file when it is fixed or dropped.

## Symptom

Old or long anime doesn't download. Current-season anime downloads fine. The cause isn't finding a
candidate — it's that the candidate is either dead (0-2 seeders, blocked by `min_seeders` or stuck
at 0%) or a huge pack the size ceiling rejects.

Concrete case: for the One Piece 1-12 window, the only live pack that covers it is 171.8 GiB and the
100 GB ceiling blocks it; the loose episodes exist and are dead. No indexer swap solves this,
because 574 episodes weigh what they weigh.

## What to check

Three exits, each in its own spec:

- per-anime size ceiling in `AnimeSettings` (smallest diff, costs disk);
- **per-file selection** — `rain` v2.3.1 doesn't have it: `AddTorrentOptions` only exposes
  `Stopped`/`StopAfterDownload`/`StopAfterMetadata`, and `Files()`/`FileStats()` are read-only;
  `anacrolix/torrent` has per-file priority;
- Debrid.
