
/*
 * Semantic colors (phase 0 of the UI redesign). Source of truth: src/lib/design/tokens.css.
 *
 * Each entry below uses `rgb(var(--x-rgb) / <alpha-value>)` (not a fixed hex) for the
 * accent/status colors, which enables Tailwind's opacity modifier
 * (`bg-ok/12`, `border-ok-tint/28`, ...). Surfaces and text don't need variable opacity,
 * so they use `var(--x)` directly.
 *
 * `heading`/`body` correspond to --text-primary/--text-secondary from tokens.css. The names
 * were born this way so they wouldn't collide with daisyUI's `primary`/`secondary`, which
 * reserved those two keys for its brand palette. daisyUI is gone, but the names stay:
 * renaming today would touch every screen without changing a pixel.
 */
const semanticColors = {
  // backgrounds
  window: 'var(--bg-window)',
  surface: 'var(--bg-surface)',
  card: 'var(--bg-card)',
  sunken: 'var(--bg-sunken)',
  control: 'var(--bg-control)',
  menu: 'var(--bg-menu)',

  // borders (already rgba() in the token, don't need Tailwind's opacity modifier)
  default: 'var(--border-default)',
  divider: 'var(--border-divider)',

  // text — see the note above about the heading/body names
  heading: 'var(--text-primary)',
  body: 'var(--text-secondary)',
  tertiary: 'var(--text-tertiary)',
  subtle: 'var(--text-subtle)',

  // accent and status — solid text/border/fill
  accent: 'rgb(var(--accent-rgb) / <alpha-value>)',
  ok: 'rgb(var(--ok-rgb) / <alpha-value>)',
  warn: 'rgb(var(--warn-rgb) / <alpha-value>)',
  danger: 'rgb(var(--danger-rgb) / <alpha-value>)',
  neutral: 'rgb(var(--neutral-rgb) / <alpha-value>)',

  // variants for tinted backgrounds/borders (7–16% / 22–32% opacity — see tokens.css)
  'accent-tint': 'rgb(var(--accent-tint-rgb) / <alpha-value>)',
  'ok-tint': 'rgb(var(--ok-tint-rgb) / <alpha-value>)',
  'warn-tint': 'rgb(var(--warn-tint-rgb) / <alpha-value>)',
  'danger-tint': 'rgb(var(--danger-tint-rgb) / <alpha-value>)',

  // text on solid accent/status
  'on-accent': 'var(--on-accent)',
  'on-ok': 'var(--on-ok)',
  'on-warn': 'var(--on-warn)',
  'on-danger': 'var(--on-danger)',

  // progress-bar track (§4.2)
  track: 'var(--progress-track)',

  // text of the active rail/tab bar/MoreMenu item (§5, Phase 1) — a token because the spec
  // pins a literal hex (#c5bcfa) only for the dark theme; see tokens.css.
  'nav-active': 'var(--nav-active-text)',
}

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  darkMode: ['class', '[data-theme="aad-dark"]'],
  theme: {
    extend: {
      colors: semanticColors,
      fontFamily: {
        sans: ['Manrope', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      // §4.2 — spacing: fills the 18/22/26px holes in Tailwind's default scale
      // (which already covers 4/6/8/10/14px via the 1/1.5/2/2.5/3.5 keys).
      spacing: {
        4.5: '18px',
        5.5: '22px',
        6.5: '26px',
      },
      // §4.2 — radii. Own names (don't override md/lg/xl/2xl) so screens that still use
      // Tailwind's default radii before being redesigned aren't affected.
      borderRadius: {
        badge: '6px',
        control: '9px',
        field: '12px',
        card: '16px',
        modal: '18px',
        pill: '999px',
      },
      // §4.2 — elevation (modal, menu), varies by theme via tokens.css.
      boxShadow: {
        elevation: 'var(--elevation-shadow)',
      },
      // §4.3 — type scale. Registered here as a `text-*` utility. The spec gives ranges,
      // not unique values. `line-height` is not given by the spec, so it is left out
      // (Tailwind's default behavior) instead of inventing a value. `mono label` also
      // asks for uppercase — that is `uppercase` applied by the consumer together with
      // `text-mono-label`, not part of Tailwind's font-size tuple.
      //
      // RAISED by ~1px (small text) to ~2px (large numbers) over the spec values, at the
      // user's request: at the previous size the whole UI read as too small. The entire
      // scale went up together, not just the entries that were complained about, so the
      // proportions between title / body / secondary stay the same as the handoff. The
      // few sizes written by hand as `text-[Npx]` on the screens went up in the same ratio.
      fontSize: {
        // screen title: spec 20–22px/800, -.01em
        'screen-title': ['24px', { letterSpacing: '-0.01em', fontWeight: '800' }],
        // hero number: spec 46px/600, -.03em
        'hero-number': ['48px', { letterSpacing: '-0.03em', fontWeight: '600' }],
        // card number: spec 22–30px/650 (650 is literal from the spec; there is no 650
        // weight loaded — the browser matches the nearest available weight)
        'card-number': ['32px', { fontWeight: '650' }],
        // card title: spec 14–15px/700
        'card-title': ['16px', { fontWeight: '700' }],
        // body: spec 13–13.5px/600
        copy: ['14.5px', { fontWeight: '600' }],
        // secondary: spec 12–12.5px/500
        caption: ['13.5px', { fontWeight: '500' }],
        // mono label: spec 10.5–11px/600, letter-spacing .10–.12em, uppercase
        'mono-label': ['12px', { letterSpacing: '0.12em', fontWeight: '600' }],
      },
    },
  },
  plugins: [],
}
