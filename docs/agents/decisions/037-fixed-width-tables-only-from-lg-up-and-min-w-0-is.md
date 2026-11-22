# 37. Fixed-width tables only from `lg` up, and `min-w-0` is required on a grid item that contains a scroll strip

**Location:** `frontend/src/routes/Status.svelte` (`LIST_GRID` and the hero card), `Downloads.svelte` (`ROW_GRID`/`ROW_INDENT`), `AnimeDetail.svelte` (`EP_GRID`). Covered by `frontend/tests/smoke/layout.spec.ts`.

**What it looks like:** two things that look redundant and are not.

1. The hero card's "Active downloads" strip has `overflow-x-auto` **and** the wrapping `<section>` has `min-w-0`. Looks like belt and suspenders — `overflow-x-auto` alone should be enough for the strip to scroll.
2. The three tables switch to stacked cards at `lg` (1024px), not at `md` (768px), even though the whole app uses `md` as the desktop/mobile breakpoint (including decision 36's `matchMedia`, which stays at 768px).

**Why it's right:**

1. A grid item has `min-width: auto`, i.e. its floor is the content's intrinsic size, not zero. With many downloads the `<section>` was stretched to max-content and `overflow-x-auto` **never activated**: measured on a 1280px grid, the section went to 2494px, the strip had `clientWidth == scrollWidth` and the right-hand column ran off the screen. With `min-w-0` the same section is back to 677px and the strip actually scrolls (639px visible for 2456px of content). `overflow-x-auto` describes the intent; `min-w-0` is what lets it happen.

2. It is arithmetic, not taste: the fixed tracks add up to ~754px (Status), ~740px (Downloads) and ~710px (AnimeDetail), while the usable width at `md` is `768 − 92 (rail) − 48 (main padding) ≈ 628px`. The cut at `md` was always wrong — the 768–880px band fell into horizontal scroll — and only did not bother anyone because nobody opens the app at that width. `lg` is the first breakpoint where the columns fit. The rail-vs-tab-bar breakpoint stays at 768px on purpose: it is about the **shell**, not about the width a table needs.

**Don't "fix" by:** removing `min-w-0` "because it already has `overflow-x-auto`" (that is the inverted order: without `min-w-0` the overflow is decorative); removing `overflow-x-auto` "because `min-w-0` already solves it" (then the strip clips the content instead of scrolling); dropping the tables back to `md` for consistency with the rest of the app (redo the arithmetic above first — usable width at `md` covers none of the three); adding a fixed-width column without revising the arithmetic comment that lives next to each grid constant.
