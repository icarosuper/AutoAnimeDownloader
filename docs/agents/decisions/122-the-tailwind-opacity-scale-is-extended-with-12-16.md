# 122. The Tailwind opacity scale is extended with 12, 16, 28 and 32

**Location:** `src/internal/frontend/tailwind.config.js` — `theme.extend.opacity`. Guarded by the
`opacity modifiers` test in `tests/unit/designTokens.consistency.test.ts`.

**What it looks like:** four odd keys in the config that seem to duplicate what `bg-ok/12` already
says on its own.

**Why it's right:** Tailwind 3 only generates an opacity modifier whose number is a key of the
opacity scale, and the default scale goes in steps of 5. `bg-ok-tint/12` or `border-neutral/28`
with no matching key is not an error and not a warning — the class simply isn't in the CSS. A chip
then has no fill, and its border falls back to the preflight default, `currentColor`: a solid
outline in the text color. That is how every chip, alert card and active nav item looked from the
UI redesign (#39) until 2026-11-07 — the built CSS had 3 opacity classes where the code used 24.
The tint steps come from `tokens.css` (7–16% fills, 22–32% borders), so the scale follows them,
not the other way around. Found by a visual pass in the sandbox, not by any test: every layout and
content assertion still passed, since the elements were there and visible.

**Don't "fix" by:** rounding the classes to the default scale (`/10`, `/30`) — that changes the
designed tints; or using arbitrary values (`bg-ok/[.12]`) one by one — it works, but the next
`/28` written the normal way is silently dropped again, which is what the guard test is for. A new
step means a new key here; the test lists any `/N` used in `src/` that the config doesn't generate.
