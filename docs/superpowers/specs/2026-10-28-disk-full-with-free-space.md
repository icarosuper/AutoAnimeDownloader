# Disk-full warnings while the volume still has hundreds of gigabytes free

**Date:** 2026-10-28
**Status:** diagnosed, not implemented — policy unit locked (flat GiB)
**Origin:** Status last-check showed *A Wild Last Boss Appeared!* eps 1–12 and *K-ON!* ep 9 as
`disk_full` ("Espaço em disco insuficiente") while the disk card read **177.7 GB free of 927.5 GB**.
Two torrents were downloading, seven were seeding. The volume was not full.

Diagnosis plus the locked policy below. The rest of the fix shape (last-check copy vs a new
issue code; whether `/status` grows a reserved-bytes field) is still open.

---

## Symptom

On 2026-10-28 the running daemon (`~/.autoAnimeDownloader/daemon.log`, two boots at 16:56:16 and
17:07:07) refused new torrents with:

```
insufficient free disk space: 10% free required on /home/icaro/Animes
(torrent: <Nyaa size> bytes, reserved by torrents still downloading: <74–88 GiB>)
```

The Status card used `GET /status` `disk_free` / `disk_total` (plain `statfs` `Bavail` /
`Blocks`, no reservation). `disk_low` was false. The last-check row used `IssueDiskFull`. The two
surfaces contradicted each other: the card said ~19% free, the problems list said the disk was
full.

Config at the time: `min_free_disk_percent: 10`, `max_concurrent_downloads: 3`,
`max_episodes_per_anime: 12`, `max_batch_torrent_size_gb: 100`. Library path
`/home/icaro/Animes` on `/dev/sda2` (total 995 907 784 704 bytes = 927.5 GiB).

---

## What is not broken

- `files.DiskSpace` (`statfs` `Bavail`) matches `df`. The 177.7 GB figure is real.
- Nyaa sizes in the error are ordinary (1.3–3.3 GiB singles/packs, one 17.5 GiB BD pack).
- The percentage half of `checkDiskSpace` is the one that fired (`needed < free`, so the torrent
  *did* fit in the bytes currently free). The "torrent bigger than the disk" half did not.
- Seeding torrents are not in `queue.json`. `newDiskBudget` skips `Completed`. The 74–88 GB
  reserved figure is the incomplete session, not a seeder leak.

---

## What actually happened

### 1. One catch-up pass filled the disk budget

The 16:56 boot started with an empty session and, in ~40 seconds, added **57 unique torrents**
(76 episode records; packs share a hash). `queue.json` still holds 46 incomplete hashes. Sum of
their recorded `torrent_size` values: **75.4 GB**. Log `reserved by torrents still downloading`
at the first refusals: **87.99 GB** (16:57), then **73.4–74.8 GB** after the 17:07 restart (some
had finished).

`max_concurrent_downloads` is 3, so Status showed "2 downloading · 7 seeding". The other ~44
torrents sat in the queue. Status does not list them (the queue block was dropped on purpose;
Downloads is the surface). The disk card therefore looked idle next to a 75 GB commitment.

Largest single reservation: `[hash 9c74951e…] I Want to Love You Till Your Dying Day 1-13` at
exactly 17.5 GiB (Nyaa size `18790481920`).

### 2. The 10% floor is 93 GB on this volume

`checkDiskSpace` refuses when

```
(free - torrent_size - reserved) / total * 100 < min_free_disk_percent
```

On this disk, 10% of 927.5 GiB is **92.8 GiB that must remain free after the new torrent and
after everything the queue still has to write** ([decisions.md #93](../../agents/decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_percent--0),
[#101](../../agents/decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space)).

With 177.7 GiB free and 82 GiB reserved, leftover after reservation is ~96 GiB ≈ 10.3%. A 3.3 GiB
pack pushes it to **9.97%** and is refused. A 1.3 GiB single stays just above 10% and is accepted
— which is exactly what the 17:07 pass did for *A Wild Last Boss Appeared!* ep 1
(`b75c9bb8…`, 1.40 GB added) while eps 2–12 (same 1.3–1.4 GiB class, budget already spent on
ep 1) were refused.

The user-visible sentence "I have 177 GB free" is true. The guard's sentence "after the 88 GB the
queue will write, keeping 10% of a 928 GB volume leaves no room" is also true. Neither surface
says the second sentence.

### 3. `disk_low` does not mean what it claims

`GET /status` ([endpoint_status.go](../../../src/internal/api/endpoint_status.go)):

```go
diskLow = float64(diskFree)/float64(diskTotal)*100 < float64(cfg.MinFreeDiskPercent)
```

The comment on `DiskLow`, the comment in `Status.svelte` ("the same threshold that makes the
daemon stop adding torrents"), and `status.ts` ("Don't reintroduce the ratio to save a field")
all say this flag is the daemon's add-gate. It is not. It ignores `reserved`. On this incident
`disk_low` was false (~19% free) while `attemptDownloadWithRetries` was returning
`ErrInsufficientDiskSpace` for every new magnet.

The "Low disk space — downloads paused." banner therefore stayed hidden, and the disk number
stayed the normal heading color.

### 4. A pack that no longer fitted still owned the window (#97 via the #101 race)

*A Wild Last Boss Appeared!* at 16:57:03: `packsThatFit` dropped the 16.2 GiB SubsPlease batch
and kept the 3.30 GiB Judas batch (`3543348019` bytes). `pickBatches` assigned that pack to
episodes 1–12. At 16:57:06 every one of the 12 adds failed the guard (same 3.30 GiB, reserved
now 88 GB). Last-check listed **eps 1–12 as `disk_full`**.

`packsThatFit` calls `disk.check` (snapshot, does not reserve). Five animes run in parallel
(`maxConcurrentAnimes`). Between the check and the add, other animes reserved the last bytes —
*Encouragement of Climb* 1–12 and four *#I'm Looking For a Zombie* episodes landed at 16:57:05.
[#101](../../agents/decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space)
names this race and calls it rare. A catch-up pass that adds 57 torrents in 40 seconds makes it
ordinary.

Once `assignBatches` has filled `magnetsForEpisodes`, the per-episode search never runs. The
3.30 GiB pack that no longer fitted blocked the 1.3 GiB singles that still would have (ep 1 of
the same anime was added on the next pass, when the packs had been dropped and the code fell
through to singles). That is [#97](../../agents/decisions.md#97-a-pack-that-doesnt-fit-on-disk-is-dropped-before-the-choice-not-at-download-time)
happening at add time instead of before the choice: the window is owned by a pack that cannot be
added, and the live singles are never tried.

*K-ON!* was a different path: packs dropped, ep 3 had 10 torrents under 3 seeders, eps 4–8 and
10–12 inherited `awaiting_earlier_episode`, ep 9's 1.90 GiB candidate failed the same 10%+reserved
check, ep 13 was already in the session.

---

## Arithmetic (16:57, Judas pack)

| Quantity | Bytes | GiB |
|---|---:|---:|
| `statfs` total | 995 907 784 704 | 927.5 |
| 10% floor | 99 590 778 470 | 92.8 |
| reserved (log) | 87 992 195 686 | 82.0 |
| Judas pack | 3 543 348 019 | 3.30 |
| needed | 91 535 543 705 | 85.3 |
| free required to pass | 191 126 322 175 | 178.0 |
| free on the Status card | ~190 857 000 000 | 177.7 |

Failed by ~0.25 GiB. A 1.30 GiB single at the same reserved figure needed only 176.0 GiB free
and would have passed.

---

## Three defects

The guard's reservation arithmetic is doing what #101 asked. The incident is still a bug
because three contracts the rest of the app relies on are false:

1. **`disk_low` is not the add-gate.** Last-check can be full of `disk_full` while the disk card
   is healthy. The user is right to distrust the warning.
2. **`IssueDiskFull` / `lastcheck_disk_full` name a full volume.** The volume had 177 GB free.
   The thing that was exhausted is `free - reserved` against a 10% floor of a ~1 TB disk (93 GB
   of mandatory slack plus ~80 GB of queue). The string does not mention either.
3. **A pack that fails the guard at `Add` still owns the window.** `packsThatFit` is a snapshot;
   the parallel pass invalidates it; there is no fallback to `magnetsByEpisode(singles, …)`.
   #97 is reopened for the length of one race. On this pass that was the entire *Wild Last Boss*
   window.

A fourth, product-level surprise, not a code defect by itself: on a 928 GiB disk, `min_free_disk_percent: 10`
(the default) plus one seasonal catch-up queue (~75 GB) means Status must show **~170 GB free**
before a 1.4 GB episode is accepted. The config hint talks only about "percentage of free space".
That unit is locked to change (next section).

---

## Locked: the policy is a flat GiB value

Agreed 2026-10-28. Reopens [#93](../../agents/decisions.md#93-the-disk-guard-has-two-triggers-and-the-size-one-still-applies-with-min_free_disk_percent--0)
and the percentage half of [#101](../../agents/decisions.md#101-the-disk-guard-counts-what-the-queue-still-has-to-write-not-only-free-space).
Not implemented yet.

The slack the guard protects is a quantity of bytes ("do not fill the volume to the floor"). A
percentage measures a fraction of the disk: 10% of 50 GB is 5 GB, 10% of this 928 GB volume is
93 GB, 10% of 5 TB is 500 GB. Same config, unrelated meanings. This incident is that mismatch —
177 GB free, refused because 93 GB of mandatory slack plus ~80 GB of queue left no room.

**The field:** `min_free_disk_percent` is replaced by `min_free_disk_gb`. Default **10**. `0`
turns the policy off. The size-fits half stays: a torrent bigger than `free - reserved` is
refused even with the policy off. The `_gb` suffix matches `max_batch_torrent_size_gb` /
`max_episode_torrent_size_gb` and means **GiB** (2^30), not SI GB.

**Load:** `LoadConfigs` unmarshals over defaults and ignores unknown keys, so an old
`min_free_disk_percent` disappears and the installation gets 10 GiB. No migration of the old
percentage into a GiB value — that would preserve the 93 GiB floor this change exists to drop.
Breaking for anyone who had tuned the percentage.

**The add-gate** becomes `free - reserved < min_free_disk_gb` (plus the torrent-fits check). On
this volume that is 10 GiB of slack instead of 93. The 16:57 Judas pack (3.30 GiB on top of 82 GiB
reserved, 177.7 GiB free) would have passed.

Do not keep both fields. Do not cap a remaining percentage.

---

## What a fix has to make true

Do not "fix" by dropping reserved from `checkDiskSpace`. #101 exists because a pass otherwise
queues 130 GiB into 6 GiB of headroom (sparse files; `statfs` does not see the commitment).

The next change, whatever shape it takes, has to hold all of:

- Config exposes `min_free_disk_gb` (default 10, `0` = policy off) and does not expose
  `min_free_disk_percent`. Config UI, i18n hints, diagnostics bundle, `config.md`, and #93's
  write-up follow the new field in the same change.
- `disk_low` is true exactly when the daemon will refuse a new torrent of size 0 (the add-gate,
  including reserved, against the GiB floor). The Status banner and the disk-card color follow
  that flag and nothing else.
- A last-check `disk_full` row is readable next to a 177 GB disk card: either the copy names the
  reservation / the GiB floor, or the code is split so "volume actually full" and "queue has
  claimed the slack" are different issue codes.
- A pack that passes `packsThatFit` and then loses at `reserve` does not keep the window. The
  next pack or the per-episode candidates run, same as when `packsThatFit` dropped it up front.
- Tests cover: (a) `disk_low` true when reserved alone pushes `free - reserved` under the GiB
  floor, with `free/total` still looking healthy; (b) two parallel animes, first one fills the
  budget between `packsThatFit` and `Add`, second anime's pack loses the window and a single
  that still fits is added; (c) `min_free_disk_gb = 0` still refuses a torrent bigger than
  `free - reserved`.

CLI: `config set` already covers a new `config.json` field. If `/status` grows a reserved-bytes
(or equivalent) field, `status` prints it in the same change. Last-check copy is frontend i18n;
no new endpoint is required for the copy/code split if that is how (2) lands.

Out of scope: gating queue promotion on disk.

---

## Evidence (this machine, 2026-10-28)

- `~/.autoAnimeDownloader/daemon.log` — boots 16:56:16 and 17:07:07; 29 `Skipping candidate: it
  does not fit on disk`; 33 `Skipping pack: it does not fit on disk`; reserved figures
  87 992 195 686 then 73 361 393 964 / 74 757 258 335.
- `~/.autoAnimeDownloader/queue.json` — 46 incomplete hashes, 0 paused.
- `~/.autoAnimeDownloader/downloaded_episodes` — 76 records / 57 hashes; queue subset sums to
  75.43 GB of `torrent_size`.
- `~/.autoAnimeDownloader/config.json` — `min_free_disk_percent: 10`.
- Screenshot of Status: 177,7 GB livres de 927,5 GB; PROBLEMAS (5) with `disk_full` on *A Wild
  Last Boss Appeared!* 1–12 and *K-ON!* 9.
- Daemon was stopped from the tray at 17:07:51; not required to reproduce the log arithmetic.
