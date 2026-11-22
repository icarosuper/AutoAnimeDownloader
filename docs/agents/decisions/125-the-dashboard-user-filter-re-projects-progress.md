# 125. The dashboard user filter re-projects progress onto the selected accounts, not just their membership

**Location:** `internal/anilist/anilist.go` — `MediaList.Accounts`, `StampAccount`, the accounts
union in `DedupeByMedia`; `internal/api/endpoint_animes.go` — `AnimeInfo.Accounts` (pending
recomputed per account at the end of `handleAnimes`); `frontend/src/lib/utils/status.ts` —
`scopeToAccounts`, run before `filterAnimes` in `routes/Status.svelte`; `cmd/cli/animes_filter.go` —
the same projection for `animes --account`. Pinned by
`TestDedupeByMedia_UnionsAccounts`, `Same anime in two accounts carries each account's progress`,
`TestInStatuses` and the orphan case of `TestHandleAnimes` (`endpoint_animes_test.go`), `scopeToAccounts` in `status.utils.test.ts` and
`Status — user filter` in `Status.filters.test.ts`, `TestAnimesAccountFilterProjectsProgress` (CLI).

**What it looks like:** a filter that rewrites data. Picking an account changes the
`episodes_watched` / `episodes_pending` of the rows it keeps, so the same anime shows "4 watched"
unfiltered and "10 watched" with one account picked. And `/api/v1/animes` ships progress twice:
the top-level pair and again inside `accounts`.

**Why it's right:** the top-level pair is the combined view — the lowest progress across accounts
([#43](043-an-animes-identity-is-mediaid-status-is-a-per.md)),
which is what the daemon downloads and deletes by. A filter that only kept the picked account's
animes would still answer "Unwatched", the progress bar, the "behind" chip, the sort and every
count for whichever account is furthest behind — possibly one the user didn't pick, which reads
as a bug. So `scopeToAccounts` keeps an anime when ANY picked account tracks it (OR) and replaces
the pair with the lowest progress among the picked accounts (highest pending, the same thing),
before every other filter, so everything downstream agrees. Pending can't be derived on the client
(it needs the episode numbers on disk), so the server sends it per account. The account list
survives the dedupe because `Accounts` is the one field `DedupeByMedia` unions instead of taking
from the winner; it is `json:"-"` and stamped by the caller that knows whose list it fetched, so
the on-disk cache snapshots never carry it and the daemon pass, which doesn't stamp, gets nil.

Membership is "the account has the anime in a **download** status", the same for every row: the
covered animes get it from the filtered list fetch, and the orphan refresh (whose
`GetAnimeInfoByIDs` answers in any status) cuts its accounts down to `download_statuses`
(`inStatuses`). An anime alice watches and bob dropped is not under bob. Only the list-status half
of the fetch's filter is per account; `download_media_statuses` is a property of the anime, so an
orphan uncovered only by it (a FINISHED show with files on disk) keeps its accounts. An orphan's
combined `episodes_watched` is then re-derived from the kept accounts — `GetAnimeInfoByIDs`'s
`Progress` is the lowest across ANY status, so a PLANNING entry at 0 would otherwise set the
unfiltered number from an account the row doesn't list.

An account whose list fetch failed (not even a cached copy) carries no anime, so picking it would
show an empty list that reads as "this user has nothing". `/api/v1/animes` names those accounts in
the `X-Failed-Accounts` header — a header so the body stays the bare array every client reads —
and the dashboard shows a notice when a picked account is in it; the CLI's `animes --account`
fails with `ACCOUNT_LIST_UNAVAILABLE`. Before, orphans counted any
status and covered animes only download ones, so the filter meant two things. The anime page's
"on lists" panel and the webhook filter ([#126](126-a-webhooks-anime-filter-is-resolved-when-it-fires.md)) still use any status — they ask "who has
it", not "who is it downloaded for".

UI: hidden with fewer than 2 configured accounts (options come from the config, not the animes),
and then no selection applies — a filter never acts behind a control that isn't on screen. Until
the config answers, the options are the URL selection itself, so the control shows and can be
unpicked even when the config keeps failing. Usernames match case-insensitively (AniList and MAL
treat them so) and the selection is rewritten to the configured spelling, which is what the
daemon stamps;
an `ActionMenu` in `multiple` mode on every width, whatever the number of accounts (per-account
chips were tried and took too much of the toolbar). The selection lives in the URL as repeated
`account=<source>:<username>`.

**Don't "fix" by:**
- Filtering by membership only: "Unwatched" would answer for an account the user didn't pick.
- Replacing the top-level pair with one account's progress on the server, or adding `?account=`:
  the combined pair is what the rest of the app means by progress, the list poll is cached per
  username ([#46](046-getfrontendanimelist-is-cached-for-60s-the.md)), and the type filter is
  already client-side.
- Giving orphans their accounts in any status "because they're still on the list": the filter
  would mean "being downloaded for" on some rows and "on the list" on others.
- Keying accounts by username alone: the same name can exist on AniList and on MAL.
- Appending onto the survivor's `Accounts` in `DedupeByMedia`: that slice may alias the caller's
  input; it builds a fresh one.
- Stamping inside `GetFrontendAnimeList` / the cache: the cached slice is shared across polls.
- Moving the failed accounts into the body: every client reads `/api/v1/animes` as a bare array.
- Applying `download_media_statuses` to an orphan's accounts: it isn't a per-account question.
