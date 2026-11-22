# 123. A one-episode entry covers episode 1 even when the magnet declares no range

**Location:** `daemon/standalone.go` — `standaloneMagnetCoverage` (the `mediaTotalEpisodes == 1`
branch, which reuses `movieMagnetRecords`) and `DownloadStandaloneAnimeWithMagnet` (an empty
`name` falls back to `magnetDisplayName`, the magnet's `dn`); `daemon/episodes.go` —
`packsForWindow` (the automatic pass, and `debug.go`) and `packSet.covering` (the `totalEpisodes == 1`
branches); tests `TestDownloadStandaloneAnimeWithMagnet_OneEpisodeEntryRecordsEpisodeOne`,
`TestDownloadStandaloneAnimeWithMagnet_EmptyNameFallsBackToMagnetDN`,
`TestPickBatches_UnresolvedUnlabeledPackCoversAOneEpisodeEntry`,
`TestOneEpisodeEntryPacks_KeepsOnlyUnlabeled`.

**What it looks like:** an exception to [#84](084-a-pack-without-a-range-in-its-name-gets-its.md), which says an unresolved range
writes no record because guessing one fed ownership-by-coverage with invented data.

**Why it's right:** #84's danger is guessing `1..total` for a pack that may hold less. With
`total == 1` there is nothing to guess: whatever the torrent holds, the only episode it can be for is
episode 1. Measured on 2026-11-07 with Mushoku Tensei "Eris the Goblin Slayer" (141534, SPECIAL, one
episode): the Erai-raws release names no number, is a single file (so `PackFileRange` has nothing
either), and the add wrote zero records. The torrent downloaded, the organize job had no record to
place, and the anime stayed "1 behind" with the file only under `.torrents` until a manual
`replace-episode`.

The `dn` fallback covers the other half of that case: the CLI used to send only the magnet, so even
a release whose name does declare a range reached `ExtractBatchInfo` with `""`. The CLI now sends the
matching candidate's name / nyaa_id / size like the web picker; the fallback keeps any other client
that sends just a magnet from losing the range.

**The automatic pass has the same blind spot, one step earlier.** The pack path runs only with more
than one pending episode, and a release with no number is not an episode either, so a one-episode
entry whose release names no number was never downloaded by the pass — only by hand. For that entry
`packsForWindow` opens the pack path to **unlabeled** packs only (a labeled "Violet Evergarden - 01 ~
14 (Plus Specials) [BATCH]" covers "episode 1" on the axis and would bring 26 GB of series for a
special), and `covering` takes one whose range stays unknown as `1..1`, refusing one whose files span
more (1..26 is a series, not the special). Measured with `make debug-anime`: 141534 went from 0 to 1
magnet; 101432 (Violet Evergarden Special, only labeled series packs on Nyaa) stays at 0;
`make debug-batch` gave the same magnets per anime before and after.

**Don't "fix" by:**
- Opening the pack path to every pack for a one-episode entry: a labeled series pack covers
  "episode 1" and downloads the whole series for an OVA.
- Extending the rule to "total > 1 and the torrent has one file → episode 1": a single-file torrent
  of a 12-episode entry can be any of them; that is exactly #84's invented coverage.
- Writing the standalone record as a batch `1..1`: it is one file for one episode, the same shape as
  a movie record. (The automatic pass does record it as a pack `1..1`, because it chose it on the
  pack path; with a one-episode entry that range is the whole entry, so the deletion guard frees it
  the moment its one episode goes.)
- Dropping the `dn` fallback because the CLI now sends the name: the API is public and the web is not
  its only client.
