# 124. A torrent is one notification: a pack goes out as one item naming only the anime, not one webhook per record

**Location:** `internal/notifications/notifications.go` — `Episode.PackSize`, `packStrings`,
`buildBatchVars` (`{{episode}}` blank for a pack), `NotifyAll` (one webhook per item with the
window off), `episodeCount` (titles and `{{count}}`); `daemon/helpers.go` — `episodeItems`;
`daemon/jobs.go` — the completion webhook in organize; `daemon/episodes.go` — `torrentEpisodes`/
`notifiedNew`/`failed`/`added` in `processAnimeEpisodes`, and the `episode_deleted` webhook in
`removeEpisodesAndLinks`. Pinned by `TestNotifyAll_PackIsOneWebhookNamingTheAnime`,
`TestNotifyAll_WindowZeroFiresOnePerItem`, `TestBatchLines_PackNamesOnlyTheAnime`,
`TestWillBatch_NotifiesNewEpisodeOncePerPack`, `TestWillBatch_PackFailureWaitsForTheLoop`,
`TestRemoveEpisodesAndLinks_PackFiresOneEpisodeDeletedWebhook`,
`TestRemoveEpisodesAndLinks_TwoAnimesFireTwoWebhooks`, `TestEpisodeItems_PackIsOneItemPerAnimeAndTorrent`.

**What it looks like:** an `Episode` that can stand for a whole pack, a pack push that names no
episode at all ("Frieren downloaded successfully"), a blank `{{episode}}` exactly when the event is
about the most episodes, and a per-pass map keyed by magnet that holds back the failure push until
the episode loop ends — when "call `Notify` for each episode" looks simpler.

**Why it's right:** a pack is one torrent, and the user experiences it as one event. Before this,
the three paths each got it wrong in their own way: organize fires once per hash but named only
`matched[0]`'s episode, so a finished 12-episode pack pushed "EP 1 downloaded successfully";
`processAnimeEpisodes` meets the pack once per episode it covers (every episode gets the same
candidates), so `new_episode` fired N times; deleting a pack fired `episode_deleted` once per
record. With `batch_window_seconds = 0` that was N phone pushes and — for the Jellyfin/Plex scan
presets, subscribed to `download_completed` and `episode_deleted` — N library scans for one
deletion. The window from [#47](047-notifications-are-grouped-by-window-and-the-body.md) hid part of it, but only when on, and its title still
counted items.

So the unit is the torrent: a torrent's records become ONE item per anime and hash
(`episodeItems`), with `PackSize` = how many records it has. The loop key is the first candidate's
magnet because that is the torrent the episode will `Add` — two episodes sharing it are the same
download. An episode whose candidates came from its own per-episode search is not in the pre-scan
and is a single episode.

The pack's text names only the anime, by the owner's choice: a range was tried first and did not
hold. Built from the records it lies across a gap (records skip watched episodes), split into runs
it turned one pack into several items and so several webhooks, and `{{episode}}` = `1-12` broke
templates that use it as a number (`"ep": {{episode}}` becomes invalid JSON). So `{{episode}}` is a
single number or blank, never a range; `{{anime_name}}`, `{{anime_id}}` and `{{torrent_hash}}`
stay filled, and `{{count}}` and the window titles count **episodes** (`episodeCount`), because
one item can be a whole pack. Single-episode torrents in a window still collapse into "EPs 1-24"
lines — those are separate downloads, not a pack.

`NotifyAll` with the window off fires **one webhook per item**: deleting two animes at once is two
pushes, each with its own single-item variables ([#47](047-notifications-are-grouped-by-window-and-the-body.md) (b)), not one push whose
`{{anime_id}}` is blank because it holds two animes.

A pack's `download_failed` waits for the end of the loop and goes out only if no episode of it got
the torrent added: the next episode retries the same candidates, and a transient failure on the
first one used to push "failed" for a pack that was downloading.

**Don't "fix" by:**
- Looping `Notify` over a torrent's records: back to N pushes and N Jellyfin scans with the window off.
- Sending `matched[0].EpisodeNumber` on completion: the push names one episode of a pack.
- Putting a range back in `{{episode}}` or in the pack's text: see above — it lies across gaps or
  breaks numeric templates.
- Firing everything `NotifyAll` gets as one webhook with the window off: two animes share one push
  and its single-value variables go blank.
- Dropping the magnet dedupe in the loop "because the window merges them": it does not when the
  window is `0`, and the batch title would still say "26 new episodes" for one download.
- Notifying a pack's failure on the first failed episode: a later episode may still add it.
- Making `{{count}}` the number of items again: a pack would count as 1.
