# 111. A pack that loses at reserve does not keep the window

**Location:** `daemon/episodes.go` (`chooseCoveringPacks`, wrapping `packsThatFit` + `pickBatches`
and retrying when `disk.reserve` refuses a chosen pack).

[#97](097-a-pack-that-doesnt-fit-on-disk-is-dropped-before.md)
drops packs the guard would refuse *before* `pickBatches`. That call is a snapshot: it checks
without reserving. Five animes of a catch-up pass run in parallel (`maxConcurrentAnimes`), and
between the check and the add another anime can spend the last bytes. The pack that then fails
at `Add` still owned the window, so the per-episode candidates never ran — #97 for the length of
the race, which a 57-torrent catch-up made ordinary.

**The rule:** `chooseCoveringPacks` reserves each chosen pack under the same lock `Add` will use.
If reserve fails, that pack is dropped and the choice runs again on what remains: the next pack
covers the window, or `magnetsByEpisode` / the per-episode search does. Same outcome as when
`packsThatFit` dropped it up front.

The reservation stays for the pass, so `attemptDownloadWithRetries` sees the hash already counted
and does not pay twice. If `Add` then fails for a non-disk reason, that reservation is not given
back — conservative for one pass, rebuilt from the session on the next.

**Don't "fix" by:** dropping reserved from `checkDiskSpace` (that is #101); splitting `reserve`
into a check and a later count (the parallel animes would race between the two); or waiting until
the next pass for the singles (the window was lost for every episode of that anime, this pass).
