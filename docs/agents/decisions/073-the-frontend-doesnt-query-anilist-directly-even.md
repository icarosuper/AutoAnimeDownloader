# 73. The frontend doesn't query AniList directly, even though it could

**Location:** policy for `src/internal/frontend` and `src/internal/api/endpoint_animes.go` — today
every AniList access goes through the backend.

**What it looks like:** the limit is per public IP, and the frontend is the largest consumer (see
#65). If it talked to AniList directly, running on a PC different from the daemon the consumption
would fall on another bucket.

**The technical prerequisites exist** — measured on 28 Aug 2026, that is not what blocks it:

- AniList answers `Access-Control-Allow-Origin: *` and exposes the rate-limit headers in
  `Access-Control-Expose-Headers`. A browser can query directly, including reading the budget.
- The app **uses no token**: every query is public, by username. There is no credential to leak
  to the browser.

**The hole is NAT.** The limit is per **public** IP, not per machine. Daemon on the desktop and
browser on the laptop, in the same house, leave through the same IP → same bucket, zero gain. The
relief only exists when the browser is on a genuinely different network (phone on mobile data,
remote access, VPN) — the least common scenario — and the cost is paid in every scenario.

**The cost is silent duplication.** The frontend would have to replicate in TypeScript
`DedupeByMedia`, the `customLists` merge, `withStandaloneProgress` and `health.go`'s health
classification. Go logic duplicated in TS diverges with nobody noticing, and the daemon would
still need the same data for the pass: two paths for the same data, not one.

| | gain | cost | works when? |
|---|---|---|---|
| Frontend fetches directly | moves the poll to another IP | logic duplicated in TS | only if the browser is on another network |
| Batched `id_in` on standalones | N req/min → 1 | ~10 lines | always |

**Verdict: don't.** The idea points at a real problem, but it attacks the wrong slice with the
expensive instrument. **Re-evaluate only if**, after batched `id_in` and the priority gate (#72),
the rate limit tightens again.

**Don't "fix" by:** moving only "one small query" to the frontend as a middle ground — the cost is
duplicating the merge logic, and it starts on the first query.
