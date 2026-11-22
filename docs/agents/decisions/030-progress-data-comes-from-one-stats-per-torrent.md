# 30. Progress data comes from one `Stats()` per torrent, pulled only while a screen is open

**Location:** `internal/torrents/session.go` (`toInfo`); `internal/api/endpoint_torrents.go`; `frontend/src/routes/Downloads.svelte`; `frontend/src/routes/Status.svelte`; `frontend/src/routes/AnimeDetail.svelte`.

**What it looks like:** `TorrentInfo` carries a dozen progress fields filled from a single
`t.Stats()` call, and the WebUI polls `GET /api/v1/torrents` on a plain client-side timer
while a screen that needs it is mounted — instead of the more familiar "push updates over
the existing WebSocket". Three screens poll it independently, at different rates:
`Downloads.svelte` every 2s (`POLL_MS`) while the downloads screen is open, `Status.svelte`
every 5s (`TORRENTS_POLL_MS`) for the global speed card, since Status is the default `#/`
landing route, and `AnimeDetail.svelte` adaptively — 2s while that anime has an active torrent,
15s otherwise, re-scheduled with `setTimeout` instead of `setInterval` so the delay can change
between ticks without ever running two timers at once. All of them are cleared on unmount, so
the endpoint is only hit while at least one of those three screens is on screen.

**Why it's right:** rain's `Stats()` is not a getter — it is a blocking round-trip into that
torrent's goroutine (`torrent/torrent_commands.go:141`), and so are `Peers()`, `Trackers()`
and `Webseeds()`. Reading fields one at a time, or on a server-side ticker, would hammer
every torrent's goroutine forever, including when nobody is looking. Pulling on demand costs
exactly zero when no screen is open. The same reasoning keeps `Peers()`/`Trackers()` out of
the list route entirely.

**Don't "fix" by:** adding a parallel `Stats()`-per-field accessor; moving the poll to a
server-side ticker or the WebSocket without gating it on `len(wsm.clients) > 0`; or calling
`Peers()`/`Trackers()`/`FileStats()` from the list endpoint.

**Related:** the API exposes a hand-written status slug (`statusSlug`) rather than rain's
`Status.String()`, which returns display text with a space (`"Downloading Metadata"`) and
can be reworded by any library upgrade. And `Session.Resume` re-arms the per-torrent listener,
because pausing makes the one-shot `NotifyStop` fire and kills the goroutine — without the
re-arm a resumed torrent would complete without ever enqueuing `JobOrganize`. Finally,
`Completed` is computed by `completedFromStats` from piece counts (`st.Pieces.Have >=
st.Pieces.Total`), not from `Status`: pausing a fully-downloaded torrent takes it out of
`Seeding`, and both `jobs.go`'s retry gate and `verification.go`'s reconciliation gate on
`Completed`, so deriving it from `Status` would make a paused-but-finished torrent invisible
to organization — the plan originally specified `Completed: st.Status == torrent.Seeding`,
and this was corrected during review specifically because `Pause` made the bug reachable.
The same lesson applies one layer up, to bytes rather than pieces: `Torrent.Stop()` frees
rain's piece data (`closeData` nils `t.pieces`), which zeroes `Bytes.Completed` while
`Bytes.Total` survives, so `buildTorrentResponse` falls back to the piece ratio
(`PiecesHave`/`PiecesTotal`) whenever `BytesCompleted` reads 0 — otherwise a torrent paused
mid-download renders an empty progress bar for its whole paused lifetime.
