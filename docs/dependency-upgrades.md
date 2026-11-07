# Dependency upgrades — November 2026

Working checklist for bringing the toolchain and libraries current, split into
three tiers: **what shipped in `v2.9.0`**, **what is worth doing but waits for its
own cycle**, and **what is not worth doing**.

Audit date: **2026-11-07**. Versions below were the latest published that day.
This replaces the October 2026 checklist that lived on the
`chore/dependency-upgrades` branch (commit `b5ebff5`). That branch only held the
checklist; nothing in it was ever applied to `master`.

**IDs are stable** and match the October checklist (`G0`, `F3`, ...). Refer to
them in chat (`taking G1–G3`, `F4 is done`). Do not renumber. Claim a slice by
writing `in progress (YYYY-MM-DD)` next to it; check the box when it is on
`master`.

---

## Rules for every slice

1. One commit per slice (`G0`, `G1`, ...), so a bad slice can be reverted alone.
2. `go test ./...` after every Go slice. After a frontend slice, from
   `src/internal/frontend/`: `bun run test:unit`, `bun run test:component`,
   `bun run check`.
3. Pin the **language** version in `go.mod` and the **image / CI** version in
   the Dockerfiles and workflows in the same slice. Go embeds the stdlib in the
   binary: updating the host Go does not fix a daemon built with 1.25.
4. Before the release: `make sandbox` and check that the daemon built with the
   new toolchain starts, searches and downloads.

---

## Tier 1 — done in `v2.9.0`

G0–G4 and F1 shipped in `v2.9.0` on 2026-11-07 (recorded in [TODO.md](TODO.md)).
Measured with rain 2.5.1: when deleting a torrent's data fails, `Remove` returns the error and the
torrent stays in the session, stopped; `removeEpisodesAndLinks` only logs it and forgets the
records, so the torrent shows up as an orphan the user can delete by hand.

---

## Tier 2 — worth doing, but not in this release

Each one is a breaking major and deserves its own cycle, with a visual pass in
the sandbox. Do them right after 2.9.0, one per slice.

### C1 — GitHub Actions majors

Cheap, but it can only be validated on CI — breaking the release workflow right
when shipping is the worst moment. **First thing after 2.9.0.**

- [ ] `actions/checkout@v4` → v7, `actions/setup-go@v5` → v7 (Node 24)
- [ ] `docker/setup-buildx-action@v3`, `actions/upload-artifact@v4` /
      `download-artifact@v4` → current major
- [ ] Check whether `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` is still needed

### F2 — Vite 8 + vite-plugin-svelte 7 + Vitest 5

The three move together. Vite 8 uses Rolldown.

- [ ] vite ^6 → ^8 (8.3.6), `@sveltejs/vite-plugin-svelte` ^5 → ^7 (7.3.1), vitest ^3 → ^5 (5.1.0)
- [ ] Read the Vite 7 and 8 migration notes; review `vite.config.js` and `vitest.config.ts`
- [ ] Test trio + production build (embedded in `src/internal/frontend/dist`)

### F3 — Tailwind 3.4 → 4.3

CSS-first config rewrite (`bunx @tailwindcss/upgrade`). Touches `postcss.config.js`,
`src/app.css`, `src/lib/design/tokens.css`. Needs a full visual pass (desktop +
mobile) — bad timing right after the round of UI fixes going into 2.9.0.

### F4 — svelte-spa-router 4 → 5

Svelte 5 rewrite. Touches every route (`src/App.svelte`, `src/lib/navItems.ts`,
every `push(...)` and `params` reader) and the tests that set `window.location.hash`.

### F5 — test DOM stack

- [ ] jsdom 25 → 31 (was 30 in October), `@testing-library/jest-dom` 6 → 7

---

## Tier 3 — not worth doing (for now)

| Item | Why not |
|---|---|
| **O1** — urfave/cli v2 → v3 | Different module and API; would rewrite the whole `src/cmd/cli/`. Nothing is broken on v2.27.7 (the last v2). Revisit only if v2 starts missing something we need. |
| **O2** — TypeScript 7 | Blocked: `svelte-check@4.7.8` still declares peer `typescript: ^5 \|\| ^6`. Revisit when that peer moves. |
| Paraglide 3 | New major that came out after the October audit. Paraglide 2 works and F1 already brings it to 2.29. No gain worth a migration. |
| Svelte 6 / SvelteKit 3 | Svelte 5 is the current major; we are a Vite SPA, not SvelteKit. |

---

## When this file is done

When Tier 2 is done too, delete this file — do not leave a finished checklist as
the source of truth.
