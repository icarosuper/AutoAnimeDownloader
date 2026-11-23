# 135. A Checkbox whose parent owns the value takes onchange and is reset to the parent's value after the handler

**Location:** `src/internal/frontend/src/components/ui/Checkbox.svelte` — the `onchange` prop and
`handleChange`. Callers in controlled mode: `AnimeDetail.svelte` (select-all, per-episode),
`Downloads.svelte`, `DownloadsToolbar.svelte`, `Priorities.svelte`, `Notifications.svelte`. Pinned
by `tests/component/Checkbox.test.ts` ("reverts to the parent value when onchange leaves it
unchanged", "still restores the indeterminate dash...", "toggles itself without onchange").

**What it looks like:** the input already gets `{checked}` and `{indeterminate}` one way, yet
`handleChange` waits a `tick()` and writes `input.checked` / `input.indeterminate` by hand. And
`<Checkbox on:change={...}>` silently does nothing: the event is not forwarded.

**Why it's right:** the box has two kinds of callers. `bind:checked` callers (TorrentDeleteDialog,
the untrack dialog in AnimeDetail, Logs, Report) let the box own the click; there, without
`onchange`, the box assigns `checked` itself. Selection callers pass `checked` derived from their
own state and react to the click; the parent owns the value. With the old `bind:checked` on the
input, a click flipped the box's local `checked` first. When the parent's handler then left its
derived value unchanged (a select-all over an empty list, or one a poll replaced mid-click, a
disabled or refused toggle), Svelte saw no prop change and never pushed the value back, so the box
showed a state the parent didn't have. The browser also clears `indeterminate` on every click, and
an unchanged prop doesn't restore it. Resetting the DOM to the props after the parent has run
covers both: if the parent changed its value, the props already hold the new one; if not, the box
goes back. Found while making the anime page's select-all visible on mobile (2026-11-23).

**Don't "fix" by:**
- Forwarding `on:change` again next to `handleChange`: a caller that passes both would run its
  handler twice, and a caller written the Svelte-4 way would look like it works in bind mode only.
- Dropping the `tick()` reset because `{checked}` "already" binds it: an unchanged prop is never
  re-applied, so the stale box comes back.
- Moving a selection caller back to `bind:checked` or to a forwarded `on:change`: the box would own
  a value it only displays.
- Calling `onchange` after resetting the DOM: Notifications' handlers read
  `e.currentTarget.checked` as the requested value, so the reset has to come after them.
