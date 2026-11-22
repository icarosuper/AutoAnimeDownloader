# 91. There is no component library: daisyUI left because it was a second color vocabulary

**Location:** `frontend/tailwind.config.js` (`plugins: []`), `frontend/src/app.css` (base of `html`
and `.tooltip`), `frontend/src/components/ui/` (the primitives), `frontend/src/lib/design/tokens.css`
(the single source of truth for colour). Negative fact: **there is no** `daisyui` in
`package.json`, nor a `daisyui:` block in the config, nor a
`.btn`/`.card`/`.modal-*`/`.alert`/`.checkbox`/`.select` class on any screen.

**What it looks like:** impoverishment. A ready-made component library left and in its place came
hand-written Tailwind classes, which is normally the wrong way up the ladder.

**Why it left:** it was not being used as a component library. The real use was **45 colour
utilities** (`text-base-content` ×27, `bg-base-100/200`, `border-base-300`,
`bg-success/warning/error`) against **12 component classes**. And the 45 duplicated what the
project already had: `tokens.css` defines `--bg-card`, `--text-primary`, `--ok`, `--warn`,
`--danger` and does the theme swap through **its own** `[data-theme]` — daisyUI's theme block was
a manual hex copy, not a source.

Three pieces of evidence that the duplication was real and charged a toll:

1. **There were two tests just to watch it.** In `tests/unit/designTokens.consistency.test.ts`,
   they compared each colour of `tokens.css` with the hex copy in `tailwind.config.js`, because
   daisyUI v4 compiles theme keys to OKLCH at build time and cannot consume `var(--x)`. The file
   **still exists** with the invariant that remains (the two theme blocks need to exist and be
   filled); only the two comparison tests were deleted.
2. **The project's tokens were renamed to not collide.** `heading`/`body` are only called that
   because `primary`/`secondary` were reserved by daisyUI's brand palette.
3. **The comment that justified `base-300`'s value described a use that no longer existed** —
   20 lines propped on a `grep bg-base-300` that yielded 5 results; on the day of the removal it
   yielded zero (the 5 uses were `border-base-300`).

What replaced each thing, inventing nothing new: `.modal-*` → the `ui/Modal.svelte` that already
existed (and whose own comment said it was born for that, with migrating `ConfirmDialog` left for
later); `.checkbox` → `ui/Checkbox.svelte`; `.btn` → `ui/Button.svelte`; `.alert`/`.toast` → the
same `bg-menu` + `shadow-elevation` surface as `ActionMenu`, with the status colour only on the
border and the icon; `.card`/`.card-body` → `flex flex-col rounded-card`. Only `.tooltip` had no
equivalent and became ~40 lines of CSS in `app.css`, **keeping the class and `data-tip` contract**
so as not to touch the 14 use sites.

**Gains that were not the goal:** `ConfirmDialog` gained trapped focus, Esc and focus return (the
`<dialog>` + `.modal-open` did not have it), and two `svelte-check` a11y warnings left with it.

**Verification:** before/after screenshot of the 4 affected screens in the 2 themes. The difference
sat between **0.0002% and 0.002% of the pixels**, all of it explained: card border (`base-300` →
`border-default`, which is what the other 41 borders of the app already used), the ad-hoc
`text-base-content/40|50|60` ramp collapsed into `text-subtle` (the app has 3 named levels, not 4
opacities), and ~6px of shift on the checkbox rows.

**Don't "fix" by:**
- Re-adding daisyUI (or swapping for another component lib) to "not write CSS": the hand-written
  CSS is ~40 lines of tooltip; the rest is reuse of `components/ui/`.
- Reintroducing `bg-base-*`/`text-base-content`: those classes no longer exist, and the vocabulary
  is `bg-card`/`bg-sunken`/`text-heading`/`text-body`/`text-subtle`.
- Taking `background-color`/`color` off `html` in `app.css`: `AppShell` paints its wrapper, but
  nothing covers the overscroll area or the instant before the shell mounts. That came from
  daisyUI's `base: true`.
- Swapping `app.css`'s `.tooltip` for `title=""`: the native one has a ~1s delay and does not
  style, and they are 14 sites on a hot screen.
