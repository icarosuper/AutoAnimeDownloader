# 129. Unsaved changes means different from what was loaded, and a capture-phase hashchange listener vetoes the router

**Location:** `src/internal/frontend/src/lib/stores/unsavedChanges.ts` — `snapshot`,
`setUnsaved`, `installUnsavedGuard`; installed in `App.svelte`; used by `Config.svelte`,
`Priorities.svelte` and `Notifications.svelte` (`baseline`, plus `formBaseline` for the open
webhook form). Tests: `tests/unit/unsavedChanges.test.ts`, the "unsaved changes" blocks in
`tests/component/Priorities.test.ts` and `tests/component/Notifications.duplicate.test.ts`.

**What it looks like:** a JSON serializer that sorts keys and drops empty arrays, where a "dirty"
flag set on input would do; and a `hashchange` listener in the **capture** phase that calls
`stopImmediatePropagation` and rewrites the URL with `replaceState`, instead of a router guard.

**Why it's right:** the user asked that changing a field and changing it back must not warn. A
"touched" flag can't do that, so each page compares the current state with a snapshot taken after
load (after the load's own normalization, e.g. Config's legacy `anilist_username` fill-in) and
again after a successful save. The snapshot ignores what doesn't change what would be saved: key
order (Priorities' "reset" rebuilds objects from the defaults in another order) and
`undefined`/absent/`[]` (opening a webhook in the form turns a missing `anime_sources` into `[]`;
Go reads both as an empty list). Array order still counts — in Priorities it's the ranking.

svelte-spa-router 4 has no "before leave" hook; it only listens to `hashchange` in the bubble
phase. Links, `push`, `replace` (it dispatches a synthetic `hashchange`) and the Back button all
reach it that way. A capture listener on `window` registered before `<Router>` mounts runs first,
so on cancel it stops the event before the router unmounts the page (which would lose the data)
and puts the old URL back with `replaceState`, which fires no event. A query-only change on the
same route (`#/config?group=x`) is not leaving. Reload/close uses `beforeunload`, whose text the
browser ignores. The flag is a per-page set so one page unmounting can't clear another's.
Verified in Chromium in the sandbox: sidebar click, `location.hash`, Back.

Trade-off: a cancelled push leaves a duplicate history entry for the current page (Back once more
returns to it). Accepted — the alternative, `history.back()` on cancel, can't tell a push from a
Back.

**Don't "fix" by:**
- Replacing the comparison with a dirty-on-input flag: undoing a change would still warn.
- Plain `JSON.stringify` for the snapshot: a reset or a reopened webhook reads as a change.
- Moving the listener to bubble phase or installing it inside a page: the router's listener runs
  first and the page is already gone when the confirm shows.
- Saving the open webhook form as part of "dirty" only when the list changed: unconfirmed form
  edits are lost on leaving too, so they count.
