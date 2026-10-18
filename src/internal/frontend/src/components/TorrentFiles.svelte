<script lang="ts">
  // File panel of a torrent — spec 2026-08-28. Owns the fetch, loading and error, so the two
  // screens that use it don't duplicate all three.
  //
  // READ-ONLY on purpose: rain v2 has no SetWanted/FilePriority (the module's only `Priority`
  // is PEER), so there is no selecting or excluding a file from inside the pack.
  //
  // One endpoint, one component, TWO render MODES: the two screens want different cuts of
  // the same list, not different lists.
  import { onDestroy } from "svelte";
  import { getTorrentFiles, type TorrentFile } from "../lib/api/client.js";
  import { filesByEpisode } from "../lib/utils/torrentFiles.js";
  import { formatBytes, formatPercent, type FormatLocale } from "../lib/domain/format.js";
  import Chip from "./ui/Chip.svelte";
  import ProgressBar from "./ui/ProgressBar.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  export let hash: string;
  /** `raw`: everything, in metadata order, with the raw name. `episodes`: only what matches an episode. */
  export let mode: "raw" | "episodes" = "raw";
  /** Poll tick from the owning screen. While the panel is open it refetches with it; on
      close the component is destroyed and the refetch stops — the list NEVER enters GET /torrents. */
  export let tick = 0;
  /**
   * Grid class of the owning screen's rows (AnimeDetail's `EP_GRID`). With it, the file uses
   * EXACTLY the same columns as a standalone episode — #, episode, state, details, actions —
   * instead of a second list style right below the first. Empty = its own compact layout
   * (Downloads and the stacked mobile view).
   */
  export let gridClass = "";
  /**
   * Chip of the owning torrent, for the STATE column of rows that haven't finished yet: their
   * state IS the pack's (paused, downloading, queued). A complete file shows "Downloaded" and
   * doesn't depend on this.
   */
  export let chip: { variant: "accent" | "ok" | "warn" | "danger" | "neutral"; label: string } | undefined =
    undefined;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  let files: TorrentFile[] = [];
  let loaded = false;
  let destroyed = false;

  onDestroy(() => (destroyed = true));

  async function load(h: string) {
    try {
      const next = await getTorrentFiles(h);
      if (destroyed) return;
      files = next;
    } catch {
      // Degrade silently, like the rest of the screen: keep what it already had and move on.
    } finally {
      if (!destroyed) loaded = true;
    }
  }

  // `hash` and `tick` as dependencies: switching rows refetches from scratch, tick refetches in place.
  $: void tick, load(hash);

  $: rows = mode === "episodes" ? filesByEpisode(files) : files;

  // Only the file NAME: the folder is the same for every file in the torrent, so repeating it
  // on each row only pushes out of the truncate exactly the part that distinguishes the rows —
  // in the Erai-raws pack, with a long folder, the 13 rows looked visually identical. The full
  // path stays on `title`.
  const baseName = (p: string) => p.slice(p.lastIndexOf("/") + 1);

  /** null = unknown (stopped torrent: rain frees the pieces) — "—", never 0%. */
  function progressOf(f: TorrentFile): number | null {
    if (f.bytes_completed === null || f.size <= 0) return null;
    return f.bytes_completed / f.size;
  }

  /** "48% · 1.4 GB", or "— · 1.4 GB" when per-file progress is unknown. */
  function detailsOf(f: TorrentFile): string {
    const pct = progressOf(f);
    return `${pct === null ? "—" : formatPercent(pct, fmtLocale)} · ${formatBytes(f.size, fmtLocale)}`;
  }

  function chipFor(f: TorrentFile) {
    if (progressOf(f) === 1) return { variant: "ok" as const, label: m.detail_badge_downloaded() };
    return chip;
  }
</script>

{#if gridClass}
  <!-- Table mode: the file rows repeat the owning screen's columns, with a lightly tinted
       background as the only mark that they are the level inside the accordion. -->
  <div class="bg-control/30">
    {#if rows.length === 0}
      <p class="px-4 py-2.5 font-mono text-[12px] text-subtle">
        {loaded ? ($locale && m.files_waiting_metadata()) : "…"}
      </p>
    {:else}
      {#each rows as f (f.path)}
        {@const pct = progressOf(f)}
        {@const c = chipFor(f)}
        {@const details = detailsOf(f)}
        <div class="{gridClass} border-t border-divider px-4 py-2.5">
          <span></span>
          <!-- "Episode N", not the file name: the screen is about episodes, and the row has to
               read the same as the standalone episode just above. The raw name stays on `title`
               (and in full, always, on the Downloads list). The indent is the only mark that
               the row is the level inside the pack. -->
          <p class="truncate pl-5 text-copy text-heading" title={f.path}>
            {$locale && m.detail_ep_title({ number: f.episode ?? 0 })}
          </p>
          <div class="min-w-0">
            {#if c}
              <Chip variant={c.variant}>{c.label}</Chip>
            {/if}
            {#if pct !== null && pct < 1}
              <div class="mt-1.5">
                <ProgressBar value={pct} thickness={4} label={f.path} />
              </div>
            {/if}
          </div>
          <span class="truncate font-mono text-[12px] text-subtle" title={details}>{details}</span>
          <!-- Empty actions column: the panel is read-only (rain does not select files). -->
          <span></span>
        </div>
      {/each}
    {/if}
  </div>
{:else}
  <div class="border-t border-divider bg-control/30 px-4 py-2.5 lg:pl-[94px]">
    {#if rows.length === 0}
      <p class="font-mono text-[12px] text-subtle">
        {loaded ? ($locale && m.files_waiting_metadata()) : "…"}
      </p>
    {:else}
      <!-- CARD on mobile, ROW on desktop, from a single definition. On narrow screens the
           single row broke into three loose pieces (name, then the codec chip on its own,
           then bar and size), with nothing grouping what belonged to which file — `flex-wrap`
           has no way to know where an item ends. The card solves it with the border.
           `lg:contents` drops the footer wrapper on desktop, and then its children go back
           to being items of the same flex as the name — one row, no duplicated markup. -->
      <ul class="space-y-2 lg:space-y-1.5">
        {#each rows as f (f.path)}
          {@const pct = progressOf(f)}
          <li
            class="rounded-field border border-default bg-surface p-2.5 lg:flex lg:items-center lg:gap-x-3
                   lg:rounded-none lg:border-0 lg:bg-transparent lg:p-0"
          >
            <!-- The name is NOT `flex-1`: with it the name ate the whole row and pushed the
                 codec chip to the right edge, far from the file it refers to. Shrinking to
                 content, the chip stays glued to the name, and `flex-1` goes to the spacer. -->
            <span class="block min-w-0 truncate font-mono text-[12px] text-body" title={f.path}>
              {#if mode === "episodes"}
                {$locale && m.detail_ep_title({ number: f.episode ?? 0 })}
              {:else}
                {baseName(f.path)}
              {/if}
            </span>
            <div class="mt-2 flex items-center gap-2 lg:contents">
              <!-- Codec only in `raw` mode (Downloads): it's the answer to "what format did it
                   come in", and the anime screen is about episodes. Only shown when the backend
                   managed to read the header — a file in flight or outside mkv gets no chip,
                   instead of an "unknown" occupying the row. -->
              {#if mode === "raw" && f.codec}
                <span class="shrink-0"><Chip variant="neutral">{f.codec}</Chip></span>
              {/if}
              <span class="hidden lg:block lg:flex-1"></span>
              <span class="min-w-0 flex-1 lg:w-[104px] lg:flex-none">
                <ProgressBar value={pct ?? 0} thickness={4} variant={pct === 1 ? "ok" : "accent"} label={f.path} />
              </span>
              <span class="shrink-0 font-mono text-[12px] text-subtle">{detailsOf(f)}</span>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
