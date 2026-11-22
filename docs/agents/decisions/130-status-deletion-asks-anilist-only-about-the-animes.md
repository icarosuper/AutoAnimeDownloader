# 130. Status deletion asks AniList only about the animes on disk, never downloads the delete-status lists

**Location:** `internal/daemon/verification.go` — `fetchListStatuses` (one `mediaId_in` request
per account per 50 animes on disk, chained after `LoadSavedEpisodes`), `onDiskMediaIDs`,
`deletableMediaIDs` / `allAccountsAgreeOnDelete` (no network); `notification_sources.go` —
`recordCarrying`; `internal/anilist/anilist.go` —
`GetListStatusesByMediaIDs`, `listStatusCache` (persisted as `list_statuses`). Pinned by
`TestFetchListStatuses_AsksOnlyAboutAnimesOnDisk`, `TestFetchListStatuses_ChunksBy50`,
`TestFetchListStatuses_FailedAccountIsAbsent`, `TestListStatusesFallback`,
`TestListStatusesFailedChunkKeepsTheFreshOnes`, `TestListStatusesPrunesIdsThatLeftTheDisk`,
`TestRecordCarryingKeepsADeletedAnimeForOnePass` and the
`TestDeletableMediaIDs_*` tests.

**What it looks like:** `delete_statuses` is a list filter just like `download_statuses`, so
"fetch the account's list with `status_in: delete_statuses`" looks like the obvious query, and a
query keyed by the files on disk looks backwards.

**Why it's right:** deletion only ever acts on animes with an episode on disk — a few dozen —
while the delete statuses (`COMPLETED`, `DROPPED`, `PLANNING`) are the **biggest** lists an account
has. Measured on 20 Nov 2026 on the home-lab install: one account had 528 COMPLETED + 423 DROPPED +
173 PLANNING = 1124 entries, i.e. **23 pages per pass**, plus 3 for the second account, plus one
`GetMediaListStatus` tie-break per account that didn't list a candidate. With the download lists
and the series walk that is 31+ requests in seconds against a 30 req/min bucket
([#72](072-anilist-budget-is-measured-from-the-response.md)): a 429
on **every** pass since list pagination shipped (v2.10.0, before which only the first 50 entries
came and the bug hid). The tail of the pass paid for it — the series walk (coverage ownership
partial every pass) and the tie-break (an anime whose deletion never resolved).

`mediaList(userName:, mediaId_in:)` answers both questions in one shot: an id the account tracks
comes with its status (agree or veto), an id it doesn't track is omitted (no vote). Cost:
`ceil(onDisk/50)` per account, independent of list size, and the tie-break is gone. The AND rule of
[#43](043-an-animes-identity-is-mediaid-status-is-a-per.md)
is unchanged; an anime no account holds in a delete status is still not a candidate.

**The fallback is per (account, media id) and all or nothing.** A per-call key would never be
found again (the on-disk set changes every pass a file comes or goes), and a partial answer is
dangerous: a missing id reads as "not tracked" and silently drops that account's veto. So with
AniList down the answer comes from `listStatusCache` with `ErrFromCache` only when **every** id
has a stored answer (`""` = not tracked); one gap and the account is absent, which vetoes. Each
chunk is stored the moment it answers, so with 50+ animes on disk a failed second chunk leaves the
first one fresh and only the failed one stale. And once the whole set answered, the account's
entries for ids no longer on disk are pruned: they can never be asked again, and keeping them would
grow the snapshot with every anime that ever passed through the library. Using
the cached answer to vote is the same explicit owner decision the whole-list fallback had
([#92](092-the-on-disk-anilist-cache-restores-with-ttl-zero.md)).

**Trade-off accepted:** `passLists`' `listDelete` half now holds only on-disk animes. The webhook
source filter ([#126](126-a-webhooks-anime-filter-is-resolved-when-it-fires.md))
asks about animes with files — and about the ones a pass just deleted, whose `episode_deleted` can
still sit in a batch window when the next pass records without them. `recordCarrying` keeps each
account's previous answer for **one** more pass, which covers a `batch_window_seconds` shorter than
two check intervals (default 60 s against a 1 min minimum interval).

**Don't "fix" by:**
- Going back to `status_in: delete_statuses` "because it's one query": it's one query **per 50
  entries of the biggest lists**, and that is the 429 above.
- Caching the whole delete lists with a long TTL instead: it moves the 26-request burst to every
  N-th pass, which still 429s on that pass.
- Filling the gaps of a failed chunk with what did arrive: a missing id becomes "not tracked" and
  the account loses its veto — deleting files another account still wants.
- Running the status fetch in its own goroutine before the episodes are loaded: it needs the
  on-disk ids.
- Making `recordCarrying` keep everything it ever saw "to be safe": the delete half would grow
  with every anime ever deleted, which is the whole list this entry stopped downloading.
