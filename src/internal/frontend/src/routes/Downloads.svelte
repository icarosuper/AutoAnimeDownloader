<script lang="ts">
  // Downloads — spec §9.3 (Phase 5). The flat table becomes an accordion per anime: a group
  // header with aggregate progress and bulk actions, torrent rows indented inside it.
  //
  // Everything the screen already did still works: search, sorting, pruning the selection to
  // what's visible, `busy` per hash — and, above all, the VIEW STATE IN THE URL, which now also
  // carries which groups are collapsed (see the `ViewState` comment in torrentFilters.ts).
  import { onMount, onDestroy } from "svelte";
  import { querystring, replace } from "svelte-spa-router";
  import { ChevronDown, ChevronRight, ChevronsUp, Pause, Play, RefreshCw, Trash2 } from "@lucide/svelte";
  import {
    getAnimes,
    getTorrents,
    pauseTorrent,
    resumeTorrent,
    prioritizeTorrent,
    prioritizeTorrents,
    announceTorrent,
    deleteTorrent,
    removeStandaloneAnime,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import {
    filterTorrents,
    sortTorrents,
    groupTorrents,
    isProblemTorrent,
    prioritizeOrder,
    selectionPrioritizeOrder,
    applyFilterPreset,
    activeFilterPreset,
    encodeViewState,
    decodeViewState,
    DEFAULT_VIEW_STATE,
    DOWNLOADING_SLUGS,
    type FilterPreset,
    type ViewState,
    type SortKey,
  } from "../lib/utils/torrentFilters.js";
  import { statusLabel } from "../lib/utils/torrentStatus.js";
  import { formatBytes, formatEta, formatPercent, formatSpeed, type FormatLocale } from "../lib/domain/format.js";
  import Loading from "../components/Loading.svelte";
  import DownloadsToolbar from "../components/DownloadsToolbar.svelte";
  import TorrentDeleteDialog from "../components/TorrentDeleteDialog.svelte";
  import TorrentFiles from "../components/TorrentFiles.svelte";
  import Button from "../components/ui/Button.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import ProgressBar from "../components/ui/ProgressBar.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { wsConnectionState } from "../lib/stores/wsState.js";

  const POLL_MS = 2000;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  $: T = $locale && {
    title: m.downloads_title(),
    subtitle: m.downloads_subtitle(),
    emptyTitle: m.downloads_empty_title(),
    emptyDesc: m.downloads_empty_desc(),
    emptyFilteredTitle: m.downloads_empty_filtered_title(),
    emptyFilteredDesc: m.downloads_empty_filtered_desc(),
    clearFilters: m.downloads_clear_filters(),
    colName: m.downloads_col_name(),
    colProgress: m.downloads_col_progress(),
    colSpeed: m.downloads_col_speed(),
    colEta: m.downloads_col_eta(),
    colPeers: m.downloads_col_peers(),
    colActions: m.downloads_col_actions(),
    colStatus: m.downloads_col_status(),
    colEpisode: m.downloads_col_episode(),
    sortBy: m.downloads_sort_by(),
    pause: m.downloads_pause(),
    resume: m.downloads_resume(),
    announce: m.downloads_announce(),
    prioritize: m.downloads_prioritize(),
    delete: m.downloads_delete(),
    bulkPrioritize: m.downloads_bulk_prioritize(),
    batch: m.downloads_batch(),
    wsBanner: m.downloads_ws_banner(),
    wsReconnect: m.downloads_ws_reconnect(),
    pollingNote: m.status_polling_note({ seconds: POLL_MS / 1000 }),
  };

  let torrents: TorrentInfo[] = [];
  let loading = true;
  // Hashes with an action in flight: they disable that row's buttons without freezing the table.
  let busy = new Set<string>();

  // Covers by anime_id. Fetched ONCE (not part of the 2s polling) — they're static presentation
  // data; if the call fails, `Cover` falls back to the hatched placeholder and the screen goes on.
  let coversByAnimeId = new Map<number, string | undefined>();

  // View state (search/filter/sort/collapsed groups) lives in the URL, not in localStorage — see
  // encodeViewState/decodeViewState in torrentFilters.ts.
  let viewState: ViewState = DEFAULT_VIEW_STATE;

  // Bulk selection: a Set of hashes, always pruned to what's visible (post-filter).
  let selected = new Set<string>();
  let bulkBusy = false;

  // Delete dialog — serves both a single row and the selected batch.
  let deleteDialogOpen = false;
  let pendingDeleteHashes: string[] = [];
  let pendingDeleteName = "";
  // Filled in only when deleting the whole group (the anime header's button).
  let pendingDeleteAnimeId: number | undefined;
  // Ids of the standalone animes, from the same GET /animes as the covers: for a standalone anime
  // the dialog's 2nd checkbox stops tracking the anime instead of only blocking the episodes.
  let standaloneAnimeIds = new Set<number>();

  // 'queued' can be paused: with an explicit `paused` in the queue, pausing a queued torrent means
  // "don't start on your own when a slot frees up" — before that, the only way to keep an episode
  // from starting was to delete the torrent.
  function canPause(t: TorrentInfo): boolean {
    return t.status !== "stopped" && t.status !== "stopping";
  }
  // 'queued' can be resumed: with a limit on simultaneous downloads, resuming means "back to the
  // END of the queue", so on an already-queued torrent the button deprioritizes it — exactly the
  // opposite of Prioritize next to it, which is why both exist side by side.
  function canResume(t: TorrentInfo): boolean {
    return t.status === "stopped" || t.status === "queued";
  }
  // Prioritizing is jumping the queue. It makes no sense on a torrent that already finished (the
  // backend returns an error) nor on one that's already downloading.
  function canPrioritize(t: TorrentInfo): boolean {
    return !t.completed && canResume(t);
  }

  // Pill counts: derived from the list with only the SEARCH applied, never the current filter. If
  // they honored the filter, picking "Seeding" would zero the other three and the user would lose
  // exactly the information the pill exists to give ("is there anything there?").
  $: searchedTorrents = filterTorrents(torrents, { query: viewState.query, statuses: [] });
  $: counts = {
    all: searchedTorrents.length,
    downloading: searchedTorrents.filter((t) => DOWNLOADING_SLUGS.includes(t.status)).length,
    seeding: searchedTorrents.filter((t) => t.status === "seeding").length,
    problems: searchedTorrents.filter(isProblemTorrent).length,
  };

  // Sorts only when the user clicks a header; 'default' keeps the backend's order.
  $: visibleTorrents = sortTorrents(filterTorrents(torrents, viewState), viewState.sort, viewState.dir);
  // The order OF THE GROUPS is fixed (problems → downloading → the rest, see groupTorrents); the
  // sort the user picks applies WITHIN each group. That way whoever sorts by "peers" still has the
  // broken groups at the top, where they can see them.
  $: groups = groupTorrents(visibleTorrents);

  $: speeds = visibleTorrents.reduce(
    (acc, t) => ({ download: acc.download + t.download_speed, upload: acc.upload + t.upload_speed }),
    { download: 0, upload: 0 },
  );

  // Prunes the selection whenever the visible list changes (filter/search changed, or the polling
  // brought/removed a torrent). It doesn't reference `selected` directly in the $: expression to
  // avoid a reactive loop — the read/write happens inside the function.
  $: pruneSelectionToVisible(visibleTorrents);

  function pruneSelectionToVisible(visible: TorrentInfo[]) {
    const visibleHashes = new Set(visible.map((t) => t.hash));
    let changed = false;
    const next = new Set<string>();
    selected.forEach((h) => {
      if (visibleHashes.has(h)) next.add(h);
      else changed = true;
    });
    if (changed) selected = next;
  }

  $: allVisibleSelected = visibleTorrents.length > 0 && visibleTorrents.every((t) => selected.has(t.hash));
  $: someVisibleSelected = selected.size > 0 && !allVisibleSelected;

  function toggleSelectAll() {
    // Select-all checks only the visible rows (post-filter), never the whole list.
    selected = allVisibleSelected ? new Set() : new Set(visibleTorrents.map((t) => t.hash));
  }

  function toggleSelect(hash: string) {
    const next = new Set(selected);
    if (next.has(hash)) next.delete(hash);
    else next.add(hash);
    selected = next;
  }

  function toggleSelectGroup(hashes: string[]) {
    const next = new Set(selected);
    const allIn = hashes.every((h) => next.has(h));
    hashes.forEach((h) => (allIn ? next.delete(h) : next.add(h)));
    selected = next;
  }

  function syncUrl() {
    const qs = encodeViewState(viewState);
    replace(`/downloads${qs ? `?${qs}` : ""}`);
  }

  function setQuery(q: string) {
    viewState = { ...viewState, query: q };
    syncUrl();
  }

  function applyPreset(preset: FilterPreset) {
    viewState = applyFilterPreset(viewState, preset);
    syncUrl();
  }

  $: activePreset = activeFilterPreset(viewState);

  function clearFilters() {
    viewState = { ...viewState, query: "", statuses: [], problems: false };
    syncUrl();
  }

  function setSort(key: SortKey) {
    if (viewState.sort === key) {
      viewState = { ...viewState, dir: viewState.dir === "asc" ? "desc" : "asc" };
    } else {
      viewState = { ...viewState, sort: key, dir: "asc" };
    }
    syncUrl();
  }

  function sortIndicator(key: SortKey): string {
    if (viewState.sort !== key) return "";
    return viewState.dir === "asc" ? "▲" : "▼";
  }

  function isClosed(key: string): boolean {
    return viewState.closed.includes(key);
  }

  function toggleGroup(key: string) {
    const closed = isClosed(key)
      ? viewState.closed.filter((k) => k !== key)
      : [...viewState.closed, key];
    viewState = { ...viewState, closed };
    syncUrl();
  }

  /**
   * Color of the group's aggregate bar. The first version always used `accent` (except for
   * problems), which painted a 100% complete group purple — reading as "still downloading" next
   * to green "Seeding" chips.
   */
  function groupVariant(g: { problemCount: number; downloadingCount: number; torrents: TorrentInfo[] }) {
    if (g.problemCount > 0) return "danger" as const;
    if (g.downloadingCount > 0) return "accent" as const;
    if (g.torrents.every((t) => t.completed)) return "ok" as const;
    return "neutral" as const;
  }

  function statusVariant(t: TorrentInfo): "accent" | "ok" | "warn" | "danger" | "neutral" {
    if (isProblemTorrent(t)) return "danger";
    switch (t.status) {
      case "seeding": return "ok";
      case "downloading": return "accent";
      case "downloading_metadata":
      case "allocating":
      case "verifying": return "warn";
      default: return "neutral";
    }
  }

  async function load() {
    try {
      torrents = await getTorrents();
    } catch (err) {
      console.error("Failed to load torrents:", err);
    } finally {
      loading = false;
      // Only the OPEN panels refetch the file list, and on the same tick as the screen's poll.
      // The list never goes into GET /torrents: that would multiply the payload of a 2s poll by
      // N for data nobody is looking at almost all the time.
      filesTick += 1;
    }
  }

  // Third level of the accordion: group per anime → torrent row → files. It applies to EVERY row,
  // single-episode torrents included — the file inside carries fansub, resolution and codec,
  // which the torrent name doesn't always repeat.
  // Local state, not in the URL: it's ephemeral ("let me see what came"), unlike the filter and
  // the collapsed groups, which the user shares as a link.
  let openFiles = new Set<string>();
  let filesTick = 0;

  function toggleFiles(hash: string) {
    const next = new Set(openFiles);
    next.has(hash) ? next.delete(hash) : next.add(hash);
    openFiles = next;
  }

  async function loadCovers() {
    try {
      const animes = await getAnimes();
      coversByAnimeId = new Map(animes.map((a) => [a.anime_id, a.cover_image]));
      standaloneAnimeIds = new Set(animes.filter((a) => a.is_standalone).map((a) => a.anime_id));
    } catch {
      // Presentation data — the accordion works the same with the hatched fallback.
    }
  }

  async function runAction(hash: string, action: (h: string) => Promise<void>) {
    busy = new Set(busy).add(hash);
    try {
      await action(hash);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.downloads_toast_action_error());
    } finally {
      const next = new Set(busy);
      next.delete(hash);
      busy = next;
    }
  }

  // Bulk actions: each one filters what makes sense (e.g. doesn't pause what's already
  // stopped/stopping) and fires N requests to the existing per-hash endpoints — there is no bulk
  // endpoint in the backend.
  async function runBulk(
    hashes: string[],
    action: (h: string) => Promise<void>,
  ): Promise<{ success: number; failed: number; total: number }> {
    bulkBusy = true;
    let success = 0;
    let failed = 0;
    await Promise.all(
      hashes.map(async (h) => {
        try {
          await action(h);
          success++;
        } catch {
          failed++;
        }
      }),
    );
    bulkBusy = false;
    return { success, failed, total: hashes.length };
  }

  function reportBulk(
    result: { success: number; failed: number; total: number },
    doneMsg: (a: { success: number; total: number }) => string,
  ) {
    if (result.total === 0) {
      toast.info(m.downloads_bulk_none());
      return;
    }
    if (result.success > 0) toast.success(doneMsg({ success: result.success, total: result.total }));
    if (result.failed > 0) toast.error(m.downloads_bulk_partial_error({ failed: result.failed }));
  }

  /** Targets of a bulk action: `scope` limits it to the group; without it, it's the whole selection. */
  function bulkTargets(scope: TorrentInfo[] | null, predicate?: (t: TorrentInfo) => boolean) {
    const pool = scope ?? visibleTorrents.filter((t) => selected.has(t.hash));
    return pool.filter((t) => (predicate ? predicate(t) : true)).map((t) => t.hash);
  }

  async function handleBulkPause(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope, canPause), pauseTorrent);
    reportBulk(result, m.downloads_bulk_paused);
    selected = new Set();
    await load();
  }

  async function handleBulkResume(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope, canResume), resumeTorrent);
    reportBulk(result, m.downloads_bulk_resumed);
    selected = new Set();
    await load();
  }

  /**
   * Bulk prioritize is ONE request, not N — N calls to the per-hash endpoint trip over each other
   * (each one jumps ahead of the previous) and would reverse the batch. That's why it doesn't go
   * through `runBulk`/`reportBulk`: there's no partial success to count, and the backend silently
   * ignores what already completed, so the denominator can't even be checked from here.
   */
  async function handlePrioritize(hashes: string[]) {
    if (hashes.length === 0) {
      toast.info(m.downloads_bulk_none());
      return;
    }
    bulkBusy = true;
    try {
      await prioritizeTorrents(hashes);
      toast.success(m.downloads_bulk_prioritized({ count: hashes.length }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.downloads_toast_action_error());
    } finally {
      bulkBusy = false;
    }
    selected = new Set();
    await load();
  }

  async function handleBulkAnnounce(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope), announceTorrent);
    reportBulk(result, m.downloads_bulk_announced);
    selected = new Set();
    await load();
  }

  // `animeId` is filled in only when this row is the anime's only torrent (a batch, or a group of
  // one): then deleting it is deleting the anime, and the dialog needs the anime text/untrack.
  function handleDeleteRow(t: TorrentInfo, animeId?: number) {
    pendingDeleteHashes = [t.hash];
    pendingDeleteName = t.anime_name || t.name;
    pendingDeleteAnimeId = animeId;
    deleteDialogOpen = true;
  }

  function handleBulkDeleteRequest(scope: TorrentInfo[] | null = null, name = "", animeId?: number) {
    pendingDeleteHashes = bulkTargets(scope);
    pendingDeleteName = name;
    pendingDeleteAnimeId = animeId;
    deleteDialogOpen = true;
  }

  async function confirmDelete(e: CustomEvent<{ keepData: boolean; block: boolean }>) {
    const { keepData, block } = e.detail;
    const hashes = pendingDeleteHashes;
    const animeId = pendingDeleteAnimeId;
    pendingDeleteHashes = [];
    pendingDeleteAnimeId = undefined;

    if (hashes.length <= 1) {
      const hash = hashes[0];
      if (!hash) return;
      busy = new Set(busy).add(hash);
      try {
        await deleteTorrent(hash, { keepData, block });
        toast.success(m.downloads_delete_toast_done());
      } catch (err) {
        toast.error(err instanceof Error ? err.message : m.downloads_delete_toast_error());
      } finally {
        const next = new Set(busy);
        next.delete(hash);
        busy = next;
      }
    } else {
      const result = await runBulk(hashes, (h) => deleteTorrent(h, { keepData, block }));
      reportBulk(result, m.downloads_bulk_deleted);
    }

    // Standalone + deleting the whole anime: the checkbox promised to stop tracking. Without this
    // the standalone record would stay and the next pass would download everything again.
    // delete_episodes=false because the files were already handled above, per the first checkbox.
    if (block && animeId !== undefined && standaloneAnimeIds.has(animeId)) {
      try {
        await removeStandaloneAnime(animeId, false);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : m.downloads_delete_toast_error());
      }
      await loadCovers();
    }

    selected = new Set();
    await load();
  }

  function cancelDelete() {
    pendingDeleteHashes = [];
  }

  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let wsClient: WebSocketClient | null = null;

  // The "no connection" banner (spec §9.3) needs a WebSocket state that holds for THIS screen.
  // Before, only Status.svelte opened the connection, so whoever went straight to /downloads saw
  // the store stuck at its initial 'disconnected' value and the banner would always show. The
  // screen opens its own connection, with an empty status callback — Downloads doesn't show
  // daemon state, it only needs to know whether the channel is up.
  function connectWs() {
    wsClient?.disconnect();
    wsClient = new WebSocketClient();
    wsClient.connect(
      () => {},
      (state) => wsConnectionState.set(state),
    );
  }

  onMount(() => {
    viewState = decodeViewState($querystring ?? "");
    load();
    loadCovers();
    connectWs();
    // Polling only while the screen is mounted: each snapshot costs one Stats() per torrent, which
    // is a blocking round-trip into each one's goroutine.
    pollInterval = setInterval(load, POLL_MS);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
    wsClient?.disconnect();
    wsClient = null;
  });

  // Grid of the torrent rows, shared with the column header so the two don't fall out of
  // alignment. The spec's 66px indent comes from padding-left, not from a column.
  //
  // Only from `lg` up: the seven tracks add up to ~740px, so below that the grid pushed the WHOLE
  // PAGE into horizontal scrolling and squeezed the group title down to "D..". On mobile it
  // becomes `flex-wrap`, and the cells flow in two or three lines within the available width.
  //
  // The cutoff used to be `md` (768px) and it wasn't enough: minus the rail and main's padding,
  // ~628px are left there, against the ~740px the tracks need — the horizontal scroll described
  // above happened exactly in the 768–880px range. `lg` (1024px) is the first breakpoint that
  // really fits. Same criterion as the anime list in Status.svelte (see LIST_GRID).
  const ROW_GRID =
    "flex flex-wrap items-center gap-x-3 gap-y-2 lg:grid lg:grid-cols-[28px_78px_128px_minmax(120px,1fr)_150px_104px] lg:items-center lg:gap-3";
  /** Indent of the rows inside the group — 66px on desktop, only the card's padding on mobile. */
  const ROW_INDENT = "pl-4 pr-4 lg:pl-[66px]";
</script>

<TorrentDeleteDialog
  bind:open={deleteDialogOpen}
  count={pendingDeleteHashes.length || 1}
  name={pendingDeleteName}
  scope={pendingDeleteAnimeId !== undefined ? "anime" : "episode"}
  standalone={pendingDeleteAnimeId !== undefined && standaloneAnimeIds.has(pendingDeleteAnimeId)}
  on:confirm={confirmDelete}
  on:cancel={cancelDelete}
/>

<div class="space-y-4.5">
  <!-- Header: title + bandwidth summary bar.
       `min-w-[240px]` on the title block, not `min-w-0`: `flex-1` is flex-basis 0%, and a
       `flex-wrap` container decides line breaks by each item's HYPOTHETICAL main size — with
       basis 0 and min-width 0 the title contributed ~0, the speed box (433px max-content) never
       went to the next line and the title got the leftovers, breaking the subtitle into one word
       per line (measured: 7 lines at 500px, 4 at 768px). The min-width IS part of that
       hypothetical size, so it's what pushes the box to the next line — at any width, with no
       breakpoint (an `sm:flex-col` would fix 375px and leave the 500–768px range the same). -->
  <div class="flex flex-wrap items-center gap-3">
    <div class="min-w-[240px] flex-1">
      <h1 class="text-screen-title text-heading">{T && T.title}</h1>
      <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
    </div>

    <!-- `flex-wrap` in here too: at 375px the box's content measures ~337px against ~343px
         available, so a wider speed value would overflow the line. -->
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-[10px] border border-default bg-control px-3 py-2">
      <span class="font-mono text-[15px] font-bold text-ok">↓ {formatSpeed(speeds.download, fmtLocale)}</span>
      <span class="h-[14px] w-px bg-default" aria-hidden="true"></span>
      <span class="font-mono text-[15px] font-bold text-neutral">↑ {formatSpeed(speeds.upload, fmtLocale)}</span>
      <!-- This divider goes away below `sm`: that's exactly where the count drops to the box's
           second line, and a separator at the end of a line, with nothing after it, reads as junk.
           The `gap-x-3` still separates the two blocks when they fit on the same line. -->
      <span class="hidden h-[14px] w-px bg-default sm:inline-block" aria-hidden="true"></span>
      <span class="text-caption text-subtle">
        {$locale && m.downloads_summary_counts({ downloading: counts.downloading, seeding: counts.seeding })}
      </span>
    </div>

    <p class="flex items-center gap-1.5 font-mono text-[12px] text-subtle">
      <span class="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-neutral" aria-hidden="true"></span>
      {T && T.pollingNote}
    </p>
  </div>

  <!-- WebSocket down banner — only when disconnected (spec §9.3/§8) -->
  {#if $wsConnectionState === "disconnected"}
    <div
      role="alert"
      class="flex flex-wrap items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
    >
      <span class="flex-1">{T && T.wsBanner}</span>
      <button type="button" class="underline hover:opacity-80" on:click={connectWs}>
        {T && T.wsReconnect}
      </button>
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading torrents..." />
  {:else if torrents.length === 0}
    <div class="rounded-card border border-default bg-card py-12 text-center">
      <h2 class="text-card-title text-body">{T && T.emptyTitle}</h2>
      <p class="mt-1 text-caption text-subtle">{T && T.emptyDesc}</p>
    </div>
  {:else}
    <section class="rounded-card border border-default bg-card">
      <DownloadsToolbar
        query={viewState.query}
        preset={activePreset}
        {counts}
        selectedCount={selected.size}
        allSelected={allVisibleSelected}
        someSelected={someVisibleSelected}
        {bulkBusy}
        on:search={(e) => setQuery(e.detail)}
        on:presetChange={(e) => applyPreset(e.detail)}
        on:selectAll={toggleSelectAll}
        on:bulkPrioritize={() => handlePrioritize(selectionPrioritizeOrder(selected, visibleTorrents, canPrioritize))}
        on:bulkPause={() => handleBulkPause()}
        on:bulkResume={() => handleBulkResume()}
        on:bulkAnnounce={() => handleBulkAnnounce()}
        on:bulkDelete={() => handleBulkDeleteRequest()}
        on:deselectAll={() => (selected = new Set())}
      />

      {#if visibleTorrents.length === 0}
        <div class="py-12 text-center">
          <h2 class="text-card-title text-body">{T && T.emptyFilteredTitle}</h2>
          <p class="mt-1 text-caption text-subtle">{T && T.emptyFilteredDesc}</p>
          <div class="mt-3 flex justify-center">
            <Button variant="ghost" on:click={clearFilters}>{T && T.clearFilters}</Button>
          </div>
        </div>
      {:else}
        <!-- Sorts without a column of their own in the accordion stay reachable here, with the
             same `setSort` — no sort key was lost in the redesign. -->
        <div class="flex flex-wrap items-center gap-2 border-b border-divider px-4 py-2">
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.sortBy}</span>
          {#each [
            { key: "name" as SortKey, label: T && T.colName },
            { key: "download_speed" as SortKey, label: T && T.colSpeed },
            { key: "eta" as SortKey, label: T && T.colEta },
          ] as opt (opt.key)}
            <button
              type="button"
              aria-pressed={viewState.sort === opt.key}
              on:click={() => setSort(opt.key)}
              class="rounded-pill border px-2.5 py-1 text-caption font-semibold transition-colors {viewState.sort ===
              opt.key
                ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
                : 'border-default bg-control text-subtle hover:text-body'}"
            >
              {opt.label} {sortIndicator(opt.key)}
            </button>
          {/each}
        </div>

        <!-- Column header of the torrent rows. Indented like the rows, and it's where sorting by
             column (kept from the previous layout) still lives. -->
        <div class="{ROW_GRID} hidden border-b border-divider py-2.5 {ROW_INDENT} lg:grid">
          <span></span>
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colEpisode}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colStatus}</span>
          <button
            type="button"
            class="text-left font-mono text-mono-label uppercase text-subtle hover:text-body"
            on:click={() => setSort("progress")}
          >
            {T && T.colProgress} {sortIndicator("progress")}
          </button>
          <button
            type="button"
            class="text-left font-mono text-mono-label uppercase text-subtle hover:text-body"
            on:click={() => setSort("peers")}
          >
            {T && T.colPeers} {sortIndicator("peers")}
          </button>
          <span class="text-right font-mono text-mono-label uppercase text-subtle">{T && T.colActions}</span>
        </div>

        {#each groups as group (group.key)}
          {@const groupHashes = group.torrents.map((t) => t.hash)}
          {@const groupSelected = groupHashes.every((h) => selected.has(h))}
          {@const groupSome = groupHashes.some((h) => selected.has(h)) && !groupSelected}
          {@const closed = isClosed(group.key)}
          <div data-group class="border-b border-divider last:border-b-0">
            <div class="flex flex-wrap items-center gap-3 px-4 py-3">
              <Checkbox
                checked={groupSelected}
                indeterminate={groupSome}
                label={$locale ? m.downloads_group_select({ name: group.name }) : ""}
                labelHidden
                on:change={() => toggleSelectGroup(groupHashes)}
              />

              <div class="h-[46px] w-[34px] shrink-0 overflow-hidden">
                <Cover src={group.animeId !== undefined ? coversByAnimeId.get(group.animeId) : undefined} alt={group.name} />
              </div>

              <!-- The button covers title/count/bar: a big target to expand, without nesting the
                   checkbox and the group's actions inside another clickable element.
                   `min-w-[180px]`, not `min-w-0`: with the header in flex-wrap, a column that
                   shrinks to zero never pushes the bulk buttons to the next line — on mobile the
                   group name became "B..". -->
              <button
                type="button"
                class="flex min-w-[180px] flex-1 items-center gap-3 text-left"
                aria-expanded={!closed}
                aria-label={$locale ? m.downloads_group_toggle({ name: group.name }) : ""}
                on:click={() => toggleGroup(group.key)}
              >
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="truncate text-[16px] font-extrabold text-heading">{group.name}</span>
                    <Chip variant="neutral">{$locale && m.downloads_group_count({ count: group.torrents.length })}</Chip>
                    {#if group.problemCount > 0}
                      <Chip variant="danger">{$locale && m.downloads_group_problems({ count: group.problemCount })}</Chip>
                    {/if}
                  </div>
                  <div class="mt-1.5 flex items-center gap-2">
                    <div class="min-w-0 flex-1">
                      <ProgressBar
                        value={group.progress}
                        thickness={7}
                        variant={groupVariant(group)}
                        label={$locale ? m.downloads_group_progress({ name: group.name }) : group.name}
                      />
                    </div>
                    <span class="shrink-0 font-mono text-[12px] text-subtle">
                      {formatPercent(group.progress, fmtLocale)}
                    </span>
                    <span class="shrink-0 font-mono text-[12px] text-subtle">
                      ↓ {formatSpeed(group.downloadSpeed, fmtLocale)}
                    </span>
                  </div>
                </div>

                <ChevronDown
                  size={18}
                  strokeWidth={2}
                  class="shrink-0 text-subtle transition-transform {closed ? '-rotate-90' : ''}"
                />
              </button>

              <!-- The group's bulk actions, 32px squares.
                   The tooltip comes from the wrapper's `data-tip` (CSS in src/app.css, which
                   replaced daisyUI's), same pattern as the NavRail. `tooltip-left` because these
                   buttons sit against the card's right edge. The text is only the ACTION LABEL:
                   the `aria-label` appends " — {name}", which would be noise in the tooltip.

                   An open group with a single torrent (a batch, or an anime with one episode)
                   gets no bulk bar: it would repeat pause/delete an inch away from the row's own
                   buttons. The row's actions are a superset (they have announce and respect the
                   state), and its delete already uses the anime scope when the row is the whole
                   group. Collapsed, the bar comes back — otherwise the group would have no action. -->
              {#if group.torrents.length > 1 || closed}
              <div class="flex shrink-0 items-center gap-1">
                <div class="tooltip tooltip-left" data-tip={T && T.bulkPrioritize}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label={$locale ? m.downloads_group_prioritize({ name: group.name }) : ""}
                    disabled={bulkBusy}
                    on:click={() => handlePrioritize(prioritizeOrder(group.torrents.filter(canPrioritize)))}
                  >
                    <ChevronsUp size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.pause}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label="{T && T.pause} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkPause(group.torrents)}
                  >
                    <Pause size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.resume}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label="{T && T.resume} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkResume(group.torrents)}
                  >
                    <Play size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.delete}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-danger-tint/32 text-danger transition-colors hover:bg-danger-tint/12"
                    aria-label="{T && T.delete} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkDeleteRequest(group.torrents, group.name, group.animeId)}
                  >
                    <Trash2 size={15} strokeWidth={2} />
                  </button>
                </div>
              </div>
              {/if}
            </div>

            {#if !closed}
              {#each group.torrents as t (t.hash)}
                <div
                  data-torrent-row
                  class="{ROW_GRID} border-t border-divider py-2.5 {ROW_INDENT} {selected.has(t.hash)
                    ? 'bg-accent-tint/[.06]'
                    : ''}"
                >
                  <Checkbox
                    checked={selected.has(t.hash)}
                    label={$locale ? m.downloads_select_row({ name: t.anime_name || t.name }) : ""}
                    labelHidden
                    on:change={() => toggleSelect(t.hash)}
                  />

                  <!-- Expand arrow + episode label in the same target: the number alone wasn't
                       clickable, and a sixth button in the actions column (which already has
                       five in 104px) would overflow. -->
                  <button
                    type="button"
                    class="flex items-center gap-1 font-mono text-[13px] font-bold text-heading"
                    aria-expanded={openFiles.has(t.hash)}
                    aria-label={$locale ? m.files_toggle({ name: t.name }) : ""}
                    on:click={() => toggleFiles(t.hash)}
                  >
                    <svelte:component
                      this={openFiles.has(t.hash) ? ChevronDown : ChevronRight}
                      size={14}
                      strokeWidth={2}
                      class="shrink-0 text-subtle"
                    />
                    {#if t.is_batch}
                      {T && T.batch}
                    {:else if t.episode_number !== null}
                      {t.episode_number}
                    {:else}
                      —
                    {/if}
                  </button>

                  <!-- The queue position goes into the existing chip, with no new column. It does
                       NOT go into `statusLabel()`: the toolbar's status filter calls the same
                       function, and "Queued #7" would make no sense there.
                       The numbers come out non-contiguous within a group (#2, #5, #6) when
                       another anime is interleaved in the queue — that's exactly what tells the
                       queue is global; renumbering per group would destroy that information. -->
                  <span>
                    <Chip variant={statusVariant(t)}>
                      {$locale &&
                        (t.queue_position > 0
                          ? m.downloads_queue_position({ position: t.queue_position })
                          : statusLabel(t.status))}
                    </Chip>
                  </span>

                  <div class="w-full min-w-0 lg:w-auto">
                    <!-- bytes_total stays at 0 until the metadata arrives; without it the
                         percentage means nothing yet, so the bar stays on the empty track. -->
                    <ProgressBar
                      value={t.bytes_total > 0 ? t.progress : 0}
                      thickness={4}
                      variant={statusVariant(t)}
                      label={t.name}
                    />
                    <p class="mt-1 truncate font-mono text-[12px] text-subtle">
                      {#if t.bytes_total === 0}
                        —
                      {:else if t.bytes_completed === 0 && t.progress > 0}
                        <!-- Pausing zeroes bytes_completed (rain releases the pieces' data) but
                             `progress` still reports the real fraction. Showing "0 B / 1.4 GB"
                             next to a non-empty bar would read as a contradiction. -->
                        {formatPercent(t.progress, fmtLocale)}
                      {:else}
                        {formatPercent(t.progress, fmtLocale)} · {formatBytes(t.bytes_completed, fmtLocale)} / {formatBytes(t.bytes_total, fmtLocale)}
                      {/if}
                    </p>
                  </div>

                  <div class="min-w-0 font-mono text-[12px] text-subtle">
                    <p class="truncate">{$locale && m.downloads_peers_count({ count: t.peers_total })} · {formatEta(t.eta_seconds, fmtLocale)}</p>
                    <p class="truncate">↓ {formatSpeed(t.download_speed, fmtLocale)} · ↑ {formatSpeed(t.upload_speed, fmtLocale)}</p>
                  </div>

                  <!-- Per-row actions, 28px squares.
                       Play and Pause are two INDEPENDENT `{#if}`s, not the two branches of an
                       if/else: a `queued` torrent has `canResume === true`, so in the if/else it
                       fell into the Play branch and Pause never reached the DOM. Relaxing
                       `canPause` alone would render nothing. A queued torrent gets all three,
                       each in one direction: Prioritize moves it up, Play moves it to the end of
                       the queue, Pause takes it out of rotation. -->
                  <div class="flex items-center justify-end gap-1">
                    {#if canPrioritize(t)}
                      <div class="tooltip" data-tip={T && T.prioritize}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.prioritize} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, prioritizeTorrent)}
                        >
                          <ChevronsUp size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    {#if canResume(t)}
                      <div class="tooltip" data-tip={T && T.resume}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.resume} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, resumeTorrent)}
                        >
                          <Play size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    {#if canPause(t)}
                      <div class="tooltip" data-tip={T && T.pause}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.pause} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, pauseTorrent)}
                        >
                          <Pause size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    <div class="tooltip" data-tip={T && T.announce}>
                      <button
                        type="button"
                        class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                        aria-label="{T && T.announce} — {t.name}"
                        disabled={busy.has(t.hash)}
                        on:click={() => runAction(t.hash, announceTorrent)}
                      >
                        <RefreshCw size={14} strokeWidth={2} />
                      </button>
                    </div>
                    <div class="tooltip tooltip-left" data-tip={T && T.delete}>
                      <button
                        type="button"
                        class="flex h-7 w-7 items-center justify-center rounded-control border border-danger-tint/32 text-danger transition-colors hover:bg-danger-tint/12 disabled:opacity-50"
                        aria-label="{T && T.delete} — {t.name}"
                        disabled={busy.has(t.hash)}
                        on:click={() => handleDeleteRow(t, group.torrents.length === 1 ? group.animeId : undefined)}
                      >
                        <Trash2 size={14} strokeWidth={2} />
                      </button>
                    </div>
                  </div>
                </div>
                {#if openFiles.has(t.hash)}
                  <TorrentFiles hash={t.hash} mode="raw" tick={filesTick} />
                {/if}
              {/each}
            {/if}
          </div>
        {/each}
      {/if}
    </section>
  {/if}
</div>
