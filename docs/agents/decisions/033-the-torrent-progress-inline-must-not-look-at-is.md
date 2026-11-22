# 33. The torrent progress inline must not look at `is_downloaded`

**Location:** `src/internal/frontend/src/routes/AnimeDetail.svelte` (torrent progress inline), `src/internal/api/endpoint_anime_episodes.go` (`handleAnimeEpisodes`).

**What it looks like:** an inline progress bar on the anime screen appears based only on `!torrent.completed` — never on `!ep.is_downloaded`.

**Why it's right:** **`episode_hash != "" ⟺ is_downloaded`.** `handleAnimeEpisodes` fills `IsDownloaded` and `EpisodeHash` in the same `if` (a saved record exists for that episode), and the daemon writes the saved record at the moment the torrent is **added**, not when it finishes (`daemon/episodes.go`, `daemon/manual_download.go`). So an episode that is still downloading **already** arrives with `is_downloaded: true`. The old inline condition (`torrent && !torrent.completed && !ep.is_downloaded`) required a combination the API never emits, so the bar never appeared in production — and the smoke test missed it because the fixture assembled exactly that impossible state.

**Don't "fix" by:** going back to filtering the progress inline on `!ep.is_downloaded`; writing a test fixture with `is_downloaded: false` + a filled `episode_hash`.

**History:** this entry was born with a second half — daisyUI pinned on v4 while Tailwind was v3,
the cascade-layer conflict with preflight, and `data-tip` vs `.tooltip-content`. That half is all
DEAD: daisyUI was removed on 2026-08-31 (see #91), the `Layout.svelte` that hosted that tooltip no
longer exists, and the cleanup the old amendment left pending (the raw `bg-blue-600`/`bg-red-600` in
`Status.svelte`/`AnimeDetail.svelte` and `StatusBadge.svelte`) is already done — none of those
classes nor that component exist in the frontend today.
