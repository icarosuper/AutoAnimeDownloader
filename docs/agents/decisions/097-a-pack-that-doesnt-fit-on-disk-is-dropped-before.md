# 97. A pack that doesn't fit on disk is dropped before the choice, not at download time

**Location:** `daemon/episodes.go` (`packsThatFit`, `chooseCoveringPacks`, called right before
`assignBatches` in `processAnimeEpisodes`, and in `debug.go`).

The disk guard ([#93](093-the-disk-guard-has-two-triggers-and-the-size-one.md)) already ran per candidate in
`attemptDownloadWithRetries`, but by then `pickBatches` had given the pack the whole window. On
Flower and Asura the only covering pack was a 78.8 GB BD remux; it failed the guard and every
episode was left with no candidate, while the SubsPlease episodes had 26 seeders. Now the packs the
guard would refuse leave first: the next pack covers the window, or the per-episode search does.

**What looks redundant: the guard still runs again at download time.** The early call is a
snapshot; the space can change between the choice and the `Add`, and loose episodes never went
through this one. [#111](111-a-pack-that-loses-at-reserve-does-not-keep-the.md) retries the
choice when a chosen pack then loses at `reserve`, so the snapshot going stale no longer keeps
the window.

**Cost accepted:** a pack dropped here and one dropped at the `Add` look the same in the report
(`BatchSkippedNoCoverage` when no pack is left to cover). Telling them apart needs its own code.
