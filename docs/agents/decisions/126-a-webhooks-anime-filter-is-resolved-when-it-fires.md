# 126. A webhook's anime filter is resolved when it fires, from the lists the last pass fetched, not stored on the episode record

**Location:** `internal/daemon/notification_sources.go` — `accountLists`/`passLists` (the
snapshot), `NotificationSources` (the resolver, wired with `notifications.SetSourceResolver` in
`cmd/daemon/main.go`); `internal/daemon/verification.go` — the `passLists.record` calls in
`searchAnilist` and in `fetchListStatuses`; `internal/notifications/notifications.go` —
`fireBatch`, `itemsFor`, `resolveSources`; `files.WebhookPreset.AnimeSources`. Pinned by
`TestNotificationSources`, `TestFireBatch_SourceFilter` and
`TestFireBatch_FilteredWebhookWithoutResolverFailsOpen`.

**What it looks like:** two oddities. "Whose anime is this" is known when the episode is
downloaded, so it looks like it should be written on the episode record. And a filtered webhook
gets animes it can't place — a filter "only alice" that lets unknown animes through looks like a
leak.

**Why it's right:** the events fire from four places (the pass, the deletion, organize and the
torrent-failure callback), and the records are created in about eight. Stored on the record, every
record written before the filter existed would have no owner — and `episode_deleted` mostly fires
for exactly those. So the owners are asked at fire time, and the answer comes from what the pass
already fetched: each account's download-status list (before the media-status cut) and the
animes on disk it holds in a delete status (since [#130](130-status-deletion-asks-anilist-only-about-the-animes.md)), kept in memory per account and per kind, plus `standalone` from the standalone
file. No network at fire time.

It used to be a live `GetAnimeInfoByIDs` per account per window. That spent the AniList budget
([#65](065-anilist-budget-is-spent-by-the-orphan-refresh-not.md)) on every webhook when batching was off; with the budget low the query was refused,
the anime came back unknown and the filtered webhook dropped it — including the
`download_completed` that triggers a Jellyfin scan. It also ran on the shutdown `Flush`, which
waits, so a slow network could outlast systemd's stop timeout.

The trade-offs, accepted:
- Unknown is **fail-open**: an anime no fetched list holds (a manual deletion of an anime in a
  status outside both lists, say PAUSED, or anything before the first pass) reaches every filtered
  webhook. A spare push beats a lost scan. An anime known to belong to other accounts is still
  filtered out.
- An account whose fetch failed keeps its previous answer: stale ownership beats none. An account
  removed from the config stops counting even while its snapshot lingers.
- Each filtered webhook gets only its share of a window, and the template variables are built from
  that share (`buildBatchVars` per preset).
- Account keys are the dashboard's `<source>:<user>` ([#125](125-the-dashboard-user-filter-re-projects-progress.md)), so one account is spelled the
  same everywhere. The JSON field is `anime_sources`, not `sources`: `priorities.sources` (torrent
  sources like `bdremux`) already exists in the same file.
- The Test button (`FireTestWebhook`) ignores the filter: it is a delivery check.

**Don't "fix" by:**
- Storing the owners on `EpisodeStruct`: old records, and the adoption/manual/API paths that create
  records without a list in hand, would have no owner.
- Going back to a live AniList lookup in the resolver: budget spent per webhook, notifications
  dropped when it's refused, and a blocking call on shutdown.
- Skipping unknown animes on filtered webhooks: a refused or missing answer silently eats the
  library scan.
- Building the template once for all presets: a filtered webhook would describe episodes it was
  not told about. This keeps the one-torrent-one-notification shape of [#124](124-a-torrent-is-one-notification-a-pack-goes-out-as.md) per webhook.
