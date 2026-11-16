# Health notifications: library, daemon and update events

**Date:** 2026-11-15
**Status:** designed, not implemented — no fixed release ([TODO](../../TODO.md))
**Branch:** `feat/health-notifications` — every commit below lands there, in order; the branch is
merged into `master` once the last one is in
**Origin:** "are there more kinds of events worth a notification? Not just downloads — say the user
forgot to delete an anime after watching it". Of the eleven candidates proposed, 1–9 and 11 were
kept; 10 (a sequel or new season airing) was dropped: AniList already tells the user, it is out of
the project's scope.
**Scope:** `internal/notifications` (events, alert items, state store), `internal/daemon`
(`verification.go`, `loop.go`, `episodes.go`, `report.go`, new `health.go`), `internal/diagnostics`
(`crash.go`), new `internal/updates`, `cmd/daemon/main.go`, `api/endpoint_status.go`,
`cmd/cli/status.go`, `files/filemanager.go` (one config field), frontend
(`routes/Notifications.svelte`, `components/shell/NavRail.svelte`, `MoreMenu.svelte`, i18n).

---

## The problem

The four events that exist — `new_episode`, `download_failed`, `download_completed`,
`episode_deleted` — are all **occurrences** about an episode. The daemon runs headless, and the
things that go wrong with it *as a whole* only show up when someone opens the web UI: the Status
banner (pass error), the report screen (crash), the problems list (stuck episodes), or nowhere at
all (the library folder vanishing, which only reaches `daemon.log`).

The new events are mostly **states**, not occurrences: "checks are failing", "the disk is almost
full", "this episode has not been found for two days". A state is true on every pass, so the naive
implementation — fire when the condition holds — sends a push every `check_interval`. That is the
same ntfy.sh quota blow-up [decisions.md #47](../../agents/decisions.md#47-notifications-are-grouped-by-window-and-the-body-is-escaped-only-when-the-preset-is-json)
measured. Every state event below is therefore **edge-triggered**: it fires when the state starts,
and re-arms only when the state ends.

## The events

| # | Event key | Kind | Fires when | Re-arms when |
|---|-----------|------|------------|--------------|
| 1 | `library_missing` | system | `backend.ConsumeRootSwap()` reports the download root was swapped | edge by construction (the latch fires once per swap) |
| 2 | `daemon_crashed` | system | boot promoted a non-empty `crash.log` to `crash-pending.log` | edge by construction (once per crash) |
| 3 | `check_failing` | system | passes have aborted for ≥ 30 min **and** ≥ 2 passes in a row | the next pass that finishes — sends the "back to normal" message |
| 4 | `update_available` | system | GitHub's latest release is newer than the running version | a newer version than the one already announced |
| 5 | `disk_low` | system | two levels: free space drops below the warning floor; downloads are being refused for lack of space | per level: free space climbs back above that level's floor × 1.25 |
| 6 | `episode_limit_reached` | anime | the `max_episodes_per_anime` limit issue appears for an anime | the issue disappears for that anime |
| 7 | `episode_stuck` | anime | an episode has gone ≥ 48 h without success since its first problem — empty search, refused add, or dropped torrent | the episode is organized into the library; a new problem code re-fires once per code |
| 8 | `season_complete` | anime | a `FINISHED` anime has every unwatched episode in the library | never — once per anime |
| 9 | `unwatched_backlog` | anime | the next episode to watch has been in the library for ≥ 30 days | the anime's progress changes |
| 10 | `leftover_files` | digest | at most once every 7 days, while watched / off-list files remain on disk | — (digest) |

The table follows the commit order, but its numbers are the events' own — the commit numbers are
in § Commit plan (the update check takes two commits). They are **not** the numbering of the
earlier conversation either.

Alongside the new events, `download_failed` itself changes: it stops reporting what is waiting and
reports a real failure once instead of on every pass (§ Changes to `download_failed`).

**Thresholds are constants, not config** — the same call as `stalledTorrentTimeout` in
`daemon/stalled.go`: `ponytail:` comment, becomes a `config.json` field the day someone asks for a
different number. The one exception is `check_for_updates` (§ 4), because it is about whether the app
talks to the internet on its own, not about a number.

### 1. `library_missing`

**Where:** `verification.go`, right after `if backend.ConsumeRootSwap()`. `clearLibraryPathsAfterRootSwap`
returns how many records it cleared; the event fires even when that number is 0 (an empty library
that vanished is still a folder that vanished).

**Text:** title `Library folder is gone`; message
`<completed_anime_path> is gone or was replaced. N episodes will be downloaded again.`

**Why first:** it is the most damaging thing the daemon does silently — an unmounted external drive
turns into a full re-download, and today only `daemon.log` knows.

**Non-goal:** stopping the re-download. That changes [decisions.md #34](../../agents/decisions.md#34-a-download-folder-swap-is-detected-by-a-dual-marker-and-it-tears-down-the-rain-session)
behavior and is a separate decision; this spec only makes it visible.

### 2. `daemon_crashed`

**Where:** `diagnostics.StartCrashCapture` returns `promoted bool` in addition to the file.
`cmd/daemon/main.go` keeps it and, once the config is loaded, fires the event. Crash capture runs
before `logger.Init` and before the config exists, so the flag has to wait — it cannot fire inside
`StartCrashCapture`.

**Text:** title `AutoAnimeDownloader crashed`; message
`The daemon crashed at <crash time> and restarted. A crash report is waiting on the Report screen.`
Crash time = the promoted file's mtime, the same timestamp `PendingCrashAt` exposes.

### 3. `check_failing`

**Where:** a tracker in `loop.go`, evaluated after each `AnimeVerification` call that was **not**
cancelled (a cancelled pass calls `SetLastCheckError(nil)` and would read as a recovery). It reads
`state.GetLastCheckErrorCode()`.

- Failing = a non-empty code other than `PassErrSetup`. Setup-incomplete is a fresh install whose
  browser is already being opened; notifying it is noise.
- An AniList outage served from the on-disk cache does **not** abort the pass
  ([decisions.md #92](../../agents/decisions.md#92-the-on-disk-anilist-cache-restores-with-ttl-zero-and-a-value-served-from-it-comes-with-errfromcache)),
  so it never reaches this event — correct, nothing is broken for the user.
- Fires once when failing for ≥ 30 min and ≥ 2 consecutive passes. The two conditions together
  keep a 1-minute `check_interval` from firing on a 2-minute blip, and a 60-minute interval from
  waiting two hours.
- On the first finished pass after it fired, sends the recovery message under the **same** event
  key. Subscribing to "checks are failing" without wanting "they're back" makes no sense, and a
  second key would be one more checkbox for nothing.

**Text:** title `Checks are failing` / `Checks are back to normal`; message carries the cause in
plain English (one sentence per `PassErr*` code, mirroring what the frontend's `checkIssue.ts`
says) and since when.

**State:** persisted (§ State store), so a restart in the middle of an outage still sends the
recovery message.

### 4. `update_available` + the version badge

**New package `internal/updates`:**
- `GET {GITHUB_API_URL}/repos/icarosuper/AutoAnimeDownloader/releases/latest`, default
  `GITHUB_API_URL=https://api.github.com`. `releases/latest` already excludes drafts and
  prereleases.
- Runs once at boot (after a short delay, so it doesn't compete with the first pass) and then every
  24 h. Unauthenticated limit is 60 req/h per IP — one a day is nothing.
- Compares `tag_name` (`vX.Y.Z`) with `version.Version` using a small semver compare (no new
  dependency). `version.Version == "dev"` → the checker never starts.
- Network error, non-200, or an unparsable tag → `Debug` log, try again next tick. Never a pass
  error, never a notification.
- Keeps the latest result in memory: `Latest() (version, url string)`.

**Config:** `check_for_updates bool`, default `true`. `false` = the checker does not start and no
request ever leaves. Settable through `config set` like any field (no new CLI command, per
[conventions.md](../../agents/conventions.md)). Users who install from the AUR already learn about
updates from their package manager; this is how they turn the duplicate off.

**Notification:** fires once per version. The last announced version lives in the state store, so
a restart does not announce 2.11.0 again. Variables: `{{version}}` (`2.11.0`) and `{{url}}`
(the release page), empty in every other event.

**`/status`:** gains `latest_version` and `latest_version_url`, both empty when there is no newer
version, the check is off, or it has not run yet. Status is the endpoint the shell already polls.

**Frontend badge:** `NavRail.svelte` (desktop) and `MoreMenu.svelte` (mobile) already show
`status.version`; next to it, when `latest_version` is set, a small badge "v2.11.0 available" that
links to `latest_version_url` (new tab). No modal, no dismiss — it disappears on its own after the
update.

**CLI:** `status` prints `Update available: v2.11.0 (<url>)` when the field is set, and `--json`
carries the two fields. That is the CLI half of the change — no new command.

### 5. `disk_low`

**Where:** end of a finished pass (`verification.go`), one `files.DiskSpace(CompletedAnimePath)`,
plus whether the pass recorded any `IssueDiskFull`.

Two levels, each edge-triggered on its own:

| Level | Fires when | Re-arms when | Text |
|-------|------------|--------------|------|
| `warning` | free < `2 × min_free_disk_gb` (default 20 GiB); with `min_free_disk_gb = 0` (policy off), 5 % of the volume | free > that floor × 1.25 | `Only X GB free on <path>; new downloads stop below Y GB.` |
| `blocked` | the pass refused at least one download with `IssueDiskFull` | a pass refuses nothing for disk **and** free > `min_free_disk_gb` × 1.25 | `Downloads are stopped: only X GB free on <path>. N episodes are waiting for space.` |

A disk that goes straight from fine to full fires `blocked` only — `warning` would be stale news in
the same push. A `statfs` error skips silently, same reasoning as `checkDiskSpace`.

**Why `blocked` lives here and not in `download_failed`:** a full disk is a state of the volume, not
a failure of one episode. The fix is the same for every episode (free space), so a line per episode
adds nothing, and today it repeats every pass for as long as the disk stays full. See § Changes to
`download_failed`.

### 6. `episode_limit_reached`

**Where:** end of the pass, from the aggregated `limits` (`IssueMaxEpisodesPerAnime`). Fires for an
anime when its limit issue is present and was not present at the previous evaluation; re-arms when
it is gone.

**Text:** `<anime>: N episodes downloaded, M more waiting for you to watch.` (`Downloaded` and
`Pending` from the `Issue`.)

**Seeded** on first run (§ State store) — without that, the first pass after the upgrade announces
every anime that has been at the limit for months.

### 7. `episode_stuck`

**The clock belongs to the episode, not to a problem code.** Key = (anime, episode). It starts at
the episode's **first problem of any kind**:

- a problem in the pass's aggregated `problems` (empty search, all candidates above the size limit,
  no seeders, add refused, disk full), or
- a torrent of that episode dropped by `HandleTorrentFailure` (stopped with an error, or stalled for
  `stalledTorrentTimeout`).

The second source is what closes the gap the report alone leaves: a dead torrent cycles
add → stall → drop → re-search → same torrent, and since every add **succeeds**, the episode never
shows up in `problems`. Without it, that loop would be told once by `download_failed` and then
never again.

**Clears** when the episode is organized into the library (the `download_completed` path in
`jobs.go`), not when a pass happens to report no problem — a torrent that was just re-added has no
problem in the report and is still not downloaded.

**Fires** once the clock reaches 48 h, with the latest problem code as the reason. **Re-fires once
per new code**: `no_torrent_found` → `all_above_size_limit` is new, actionable information (raise
the limit), so it goes out; but each code fires at most once per episode, because `no_seeders` and
`no_torrent_found` can alternate as seeders hover around `min_seeders`. The pushes per episode are
bounded by the number of codes (~5).

`IssueAwaitingEarlierEpisode` is excluded: it is a consequence of another stuck episode, which is
the one that gets announced. That is also why an anime nobody subs does not become one push a
week: once EP 5 is stuck, EPs 6, 7… are awaiting-earlier and stay silent.

Clocks and fired codes are persisted (§ State store): a restart neither resets the 48 h nor
repeats the push.

**Text:** one line per episode run, reusing `batchLines`' grouping:
`<anime> EPs 5-7: no torrent found for 2 days`. The reason sentence comes from the latest code
(`torrent keeps stalling` for a dropped torrent).

### 8. `season_complete`

**Where:** end of the pass. For each anime in the watching set with `Media.Status == FINISHED`,
`Media.Episodes != nil`, and every episode from `progress + 1` to `Episodes` with a record that has
`LibraryPaths` (a pack counts through its records, which already cover its range): fire once, record
the anime as announced. At least one unwatched episode is required — a fully watched anime is not
"ready to binge".

**Seeded** on first run, or the upgrade announces every completed anime in the library.

**Text:** `<anime> is complete: N episodes ready to watch.`

### 9. `unwatched_backlog`

**Where:** end of the pass. For each anime in the watching set: the record of episode
`progress + 1` has `LibraryPaths` and a `DownloadDate` older than 30 days → fire once for
(anime, progress). A different progress value re-arms it — watching one episode and stopping again
restarts the 30-day clock from that episode's own `DownloadDate`.

No progress history is stored: the age of the next episode already *is* "how long the user has not
watched this".

**Text:** `<anime>: EP N has been waiting for 30 days (M unwatched episodes, X GB).` Size = sum of
`os.Stat` over the unwatched records' `LibraryPaths`.

### 10. `leftover_files` (weekly digest)

**Definition**, evaluated at the end of the pass on the episodes that are **still on disk after the
pass's own deletions ran** — so whatever the automatic cleanup removed is never reported:

- an episode of an anime that is **not** in the watching set (it left every download status), or
- an episode with number ≤ `progress − watched_episodes_to_keep`.

With the defaults (`delete_watched_episodes: true`) this is almost always empty. It fills up when
the user turned `delete_watched_episodes` off, for `manually_managed` records, and for packs held
back by the unclaimed-content guard
([decisions.md #74](../../agents/decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records)).

**Guard:** a pass where any list account failed to fetch (nothing fresh and nothing cached) skips
the digest — every anime from that account would look "off-list" and be reported as leftover.

**Cadence:** if the set is non-empty and the last digest went out ≥ 7 days ago, send it. It repeats
weekly while the files stay: a reminder is the point. A user who keeps watched episodes on purpose
does not subscribe a webhook to this event.

**Text:** title `X GB of watched anime still on disk`; one line per anime —
`Frieren: 24 watched episodes (34.2 GB)` / `Dandadan: left your lists, 12 episodes (15.1 GB)`.

## Changes to `download_failed`

Separating **failure** from **waiting**. Today `download_failed` fires on every pass for all of
the following, and almost none of it is something the user can act on at that moment.

| Reason | Today | After |
|--------|-------|-------|
| `ReasonNotFound` (empty search) | every pass, per episode — also on passes that **skipped** the search because of the miss backoff, and for every awaiting-earlier episode, which was never searched at all (`episodes.go`, the `len(candidates) == 0` branch sits outside the search's `if`) | **not sent**. Not found is waiting, not failure: a weekly episode is routinely "not found" for the first hours after it airs, until the fansub posts it. `episode_stuck` reports it once it lasts 48 h |
| `ReasonNoDiskSpace` | every pass, per refused episode | **not sent**. Moves to `disk_low`'s `blocked` level (§ 5) |
| `ReasonDownloadRejected` (add refused) | every pass — the same magnet is tried again with no backoff | once per (episode, torrent hash) |
| `HandleTorrentFailure` (error / stalled) | once per drop — a dead torrent is re-picked and drops again every hour or so | once per (episode, torrent hash). The key is the hash, never the reason text: `stalled: no peers for 1h0m` changes text on every drop |

A **different** torrent failing for the same episode is new information and fires; the same one
failing again is not. Fired (episode, hash) pairs are persisted and pruned when the episode is
organized.

What the user sees, by scenario:

| Scenario | Pushes |
|----------|--------|
| Weekly episode that shows up a few hours late | `new_episode` + `download_completed` — no alarm |
| Episode that never shows up | one `episode_stuck` after 48 h |
| Stuck episode that later shows up | `episode_stuck`, then `download_completed` |
| Dead torrent cycling add → stall → drop | one `download_failed`, then one `episode_stuck` at 48 h |
| Disk fills up | one `disk_low` (`blocked`), again only after space was freed and it filled once more |

**Breaking for subscribers:** whoever subscribes `download_failed` today stops receiving "no torrent
found" and "insufficient disk space". Both move to events that say it once; the release notes must
say so. This change cannot land before `episode_stuck` (commit 8) and `disk_low`'s `blocked` level
(commit 6) exist, or the cases would go silent.

## Shared machinery

### Alert items

`notifications.Episode` is built around an episode number and `eventStrings`/`packStrings`. The new
events reuse it with one new field, `Text string`: when set, `batchLines` uses it verbatim as the
item's line and skips the episode-run grouping, and the title comes from a per-event
`alertTitle(event, items)`. System events (1–5) carry `AnimeID == 0`.

`itemsFor` already keeps items with `AnimeID == 0` for webhooks filtered by `anime_sources`
(fail-open). That is exactly the behavior system events need — a crash is not "from" any account —
so it stays as is, and a test pins it.

The batch window applies to every new event unchanged.

### State store

A new data-dir file, `notification_state` (JSON, no extension — same convention as the others),
owned by `notifications`:

```json
{
  "seeded": true,
  "check_failing": {"since": "...", "streak": 3, "fired": true},
  "update_announced": "2.11.0",
  "disk_low_fired": {"warning": false, "blocked": true},
  "limit_reached": [154587, 21],
  "stuck": {"154587:5": {"since": "...", "code": "no_torrent_found", "fired_codes": ["no_torrent_found"]}},
  "failed_torrents": ["154587:5:<infohash>"],
  "season_complete": [154587],
  "backlog_fired": {"154587": 4},
  "leftover_last_digest": "..."
}
```

- Missing or corrupted → start empty with a warning, never fatal (same rule as `queue.json`).
- **`seeded`:** on the first pass with an empty store, events 6 and 8 record their current state
  **without firing**. Event 7 doesn't need it (its first-seen clock starts at the upgrade), event 9
  does fire (a 60-day-old backlog is real news, and it goes out as one batched message).
- Written only when something changed, under its own mutex.
- Disposable: deleting it costs at most one round of repeated notifications.

Added to the state-file table in `architecture.md` and to the diagnostics bundle's file
classification ([decisions.md #103](../../agents/decisions.md#103-log-redaction-is-by-key-and-a-test-forces-every-new-key-to-be-classified):
a test forces every new key to be classified).

### Frontend: the event list

`ALL_EVENTS` in `Notifications.svelte` goes from 4 to 14. The checkboxes are grouped under three
headings — **Episodes** (the existing 4), **Library** (6–10), **System** (1–5) — so the list stays
readable. Existing webhooks keep exactly the events they have: nothing new is turned on for anyone
by the upgrade. New presets of the message kind (Discord, Slack, Telegram, Gotify, Pushover,
Apprise, ntfy) default to the old 4 plus `library_missing`, `daemon_crashed`, `check_failing` and
`update_available`. The Jellyfin/Plex scan presets don't change — a scan on a crash means nothing.

Every event gets an `en.json` / `pt-BR.json` label.

## Commit plan

All on `feat/health-notifications`, in this order. **Every commit** ships its event end to end and
passes `go test ./...` and the frontend tests on its own: Go event + default text + tests +
checkbox and i18n label + docs (`architecture.md`, `config.md` / `environment.md` when touched,
`decisions.md` through the [record-decision](../../../.claude/skills/record-decision/SKILL.md)
skill when the commit introduces a non-obvious pattern). Shared machinery lands with its **first
consumer**, never as dead code.

| # | Commit | Brings along |
|---|--------|--------------|
| 0 | `docs: spec health notifications for library, daemon and update events` | this file, README row, TODO entry |
| 1 | `feat: notify when the library folder disappears and episodes will be redownloaded` | `Episode.Text` + `alertTitle`; the grouped event list in the frontend; test pinning that `anime_sources` lets system events through |
| 2 | `feat: notify on boot when the previous run crashed` | `StartCrashCapture` returns `promoted` |
| 3 | `feat: notify when checks keep failing and when they recover` | the `notification_state` store; decision entry for edge-triggered state events |
| 4 | `feat: check github for a newer release and notify once per version` | `internal/updates`, `check_for_updates`, `GITHUB_API_URL`, mock in the sandbox |
| 5 | `feat: show the available update in the web ui and the cli status` | `/status` fields, NavRail/MoreMenu badge, CLI `status`, `docs/guides/cli.md` |
| 6 | `feat: warn before the library disk fills up and when it stops downloads` | both levels; `ReasonNoDiskSpace` still goes out until commit 8b |
| 7 | `feat: notify when an anime hits the per-anime episode limit` | the `seeded` first-run rule |
| 8 | `feat: notify when an episode goes two days without downloading` | the per-episode clock, fed by the report **and** `HandleTorrentFailure`, cleared by organize |
| 8b | `fix!: stop reporting missing torrents and a full disk as failed downloads` | § Changes to `download_failed`: drop `ReasonNotFound` and `ReasonNoDiskSpace`, dedup rejected/dropped by (episode, hash); decision entry; breaking note for the release |
| 9 | `feat: notify when a finished anime is fully in the library` | — |
| 10 | `feat: notify when the next episode to watch sits unwatched for 30 days` | library-size helper (`os.Stat` over `LibraryPaths`) |
| 11 | `feat: send a weekly digest of watched files still on disk` | reuses the size helper |
| 12 | `docs: record the health notifications and drop the executed spec` | delete this file and its README row; TODO entry marked done (release stamped later by the release skill) |

Commits 4 and 5 are split on purpose: 4 is the daemon behavior (and is useful headless), 5 is the
surface. Each one stands alone.

## Open questions

1. **`check_failing` thresholds** (30 min / 2 passes) and **`disk_low` floor** (2× the minimum) are
   first guesses. They are constants on purpose; revisit with real use.
