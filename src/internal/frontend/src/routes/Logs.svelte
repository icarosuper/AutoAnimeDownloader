<script lang="ts">
  // Logs — spec §9.5 (Phase 7). The viewer becomes a 4-column grid
  // (82px 60px 90px 1fr: time · level · source · message) on --bg-sunken, darker than the cards
  // around it, to read like a terminal.
  //
  // The SOURCE column is derived from zerolog's `caller` by `logSourceFromCaller` — a deliberate
  // approximation (the backend has no structured `component` field; that's backlog, §3 of the spec).
  //
  // Kept from the old screen: level filter (now pills WITH A COUNT, it was a <select> without one),
  // search with the match highlighted, line count selector, auto reload with a selectable interval,
  // follow-the-end, back-to-top button with the new-lines counter, line copy and the view state in
  // the querystring.
  import { onMount, onDestroy, tick } from "svelte";
  import { ArrowUp, Copy, Search, X } from "@lucide/svelte";
  import { getLogs, type LogsResponse } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import PulseDot from "../components/ui/PulseDot.svelte";
  import { countByLevel, parseLogLine, type LogLevel, type ParsedLogLine } from "../lib/domain/logLine.js";
  import { logSourceFromCaller } from "../lib/domain/logSource.js";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.logs_title(),
    subtitle: m.logs_subtitle(),
    labelLines: m.logs_label_lines(),
    filtersLabel: m.logs_filters_label(),
    searchPlaceholder: m.logs_search_placeholder(),
    clearSearch: m.logs_clear_search(),
    labelAutoscroll: m.logs_label_autoscroll(),
    labelLive: m.logs_label_live(),
    btnReload: m.logs_btn_reload(),
    colTime: m.logs_col_time(),
    colLevel: m.logs_col_level(),
    colSource: m.logs_col_source(),
    colMessage: m.logs_col_message(),
    copyLine: m.logs_copy_line(),
    levelAll: m.logs_level_all(),
    levelDebug: m.logs_level_debug(),
    levelInfo: m.logs_level_info(),
    levelWarn: m.logs_level_warn(),
    levelError: m.logs_level_error(),
    loading: m.logs_loading(),
    emptyFiltered: m.logs_empty_filtered(),
    empty: m.logs_empty(),
    filteredSuffix: m.logs_filtered_suffix(),
    scrollTop: m.logs_scroll_top(),
    newLogs: (count: number) => m.logs_new_logs({ count }),
  };

  type LevelFilter = LogLevel | "all";

  let logs: string[] = [];
  let loading = true;
  let linesToLoad = 1000;
  let filterLevel: LevelFilter = "all";
  let searchQuery = "";
  let autoScroll = true;
  let initialized = false;
  let logContainer: HTMLElement;
  let atTop = true;
  let newLogsCount = 0;
  let liveReload = true;
  let liveReloadSeconds = 5;
  let liveInterval: ReturnType<typeof setInterval> | null = null;

  function escapeHtml(text: string): string {
    return text
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  // Escapes BEFORE injecting the <mark>: the content comes from the daemon's log, which carries
  // arbitrary torrent names. That's why the search can highlight via {@html} without opening XSS.
  function highlightMatch(text: string, query: string): string {
    const safe = escapeHtml(text);
    if (!query.trim()) return safe;
    const escapedQuery = escapeHtml(query.trim()).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    return safe.replace(
      new RegExp(`(${escapedQuery})`, "gi"),
      '<mark class="rounded-[3px] bg-warn-tint/28 px-0.5 text-heading">$1</mark>'
    );
  }

  const LEVEL_TEXT: Record<LogLevel, string> = {
    error: "text-danger",
    warn: "text-warn",
    debug: "text-subtle",
    info: "text-accent",
  };

  const LEVEL_BADGE: Record<LogLevel, string> = {
    error: "border-danger-tint/28 bg-danger-tint/12 text-danger",
    warn: "border-warn-tint/28 bg-warn-tint/12 text-warn",
    debug: "border-default bg-control text-subtle",
    info: "border-accent-tint/28 bg-accent-tint/12 text-accent",
  };

  /**
   * The search runs over the RAW line (it also finds what's in extras/caller, not only in the
   * message); the level filter runs over the parsed line. The pill counts come from the search
   * result, before the level filter — otherwise picking "Error" would zero the count of every
   * other pill and the filter would become a path with no visible way back.
   */
  $: searched = [...logs].slice(0, linesToLoad).reverse().filter(
    (l) => !searchQuery.trim() || l.toLowerCase().includes(searchQuery.toLowerCase().trim())
  );
  $: searchedParsed = searched.map(parseLogLine);
  $: levelCounts = countByLevel(searchedParsed);
  $: parsedLogs = filterLevel === "all"
    ? searchedParsed
    : searchedParsed.filter((p) => p.level === filterLevel);

  $: levelPills = ($locale && [
    { id: "all" as LevelFilter, label: m.logs_level_all(), count: levelCounts.all },
    { id: "error" as LevelFilter, label: m.logs_level_error(), count: levelCounts.error },
    { id: "warn" as LevelFilter, label: m.logs_level_warn(), count: levelCounts.warn },
    { id: "info" as LevelFilter, label: m.logs_level_info(), count: levelCounts.info },
    { id: "debug" as LevelFilter, label: m.logs_level_debug(), count: levelCounts.debug },
  ]) || [];

  $: updateUrlQuery(linesToLoad, filterLevel, searchQuery);
  $: if (parsedLogs && autoScroll) scrollToTop();
  $: if (liveReload) startLiveReload(); else stopLiveReload();

  // The list renders newest first, so "follow the end" means scrolling to the TOP. The name is kept
  // from the old screen so it doesn't confuse whoever knows the code.
  async function scrollToTop() {
    await tick();
    if (logContainer) logContainer.scrollTop = 0;
    newLogsCount = 0;
  }

  function handleScroll() {
    if (logContainer) atTop = logContainer.scrollTop < 50;
  }

  function startLiveReload() {
    if (liveInterval) clearInterval(liveInterval);
    liveInterval = setInterval(loadLogs, liveReloadSeconds * 1000);
  }

  function stopLiveReload() {
    if (liveInterval) { clearInterval(liveInterval); liveInterval = null; }
  }

  function updateUrlQuery(linesToLoad: number, filterLevel: string, searchQuery: string) {
    if (typeof window === "undefined" || !initialized) return;
    const params = new URLSearchParams();
    let hashPath = (window.location.hash || "#/logs").split("?")[0];
    if (!hashPath.includes("/logs")) hashPath = "#/logs";
    if (linesToLoad !== 1000) params.set("lines", String(linesToLoad));
    if (filterLevel !== "all") params.set("level", filterLevel);
    if (searchQuery.trim()) params.set("q", searchQuery.trim());
    const qs = params.toString();
    window.history.replaceState({}, "", window.location.pathname + window.location.search + (qs ? `${hashPath}?${qs}` : hashPath));
  }

  async function loadLogs() {
    try {
      const isBackground = liveReload;
      if (!isBackground) loading = true;
      const prevCount = logs.length;
      const response: LogsResponse = await getLogs(linesToLoad);
      logs = response.lines;
      if (isBackground && !autoScroll && response.lines.length > prevCount) {
        newLogsCount += response.lines.length - prevCount;
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.logs_error_load());
    } finally {
      loading = false;
    }
  }

  async function copyLine(text: string) {
    try {
      // navigator.clipboard only exists in a secure context, and the UI is routinely reached over
      // plain http from another machine on the LAN. There, a selected textarea + execCommand still
      // copies.
      if (navigator.clipboard) await navigator.clipboard.writeText(text);
      else if (!copyBySelection(text)) throw new Error("copy refused");
      toast.success(m.logs_copy_success());
    } catch {
      toast.error(m.logs_copy_error());
    }
  }

  function copyBySelection(text: string): boolean {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    try {
      return document.execCommand("copy");
    } finally {
      area.remove();
    }
  }

  function initFromUrl() {
    if (typeof window === "undefined") return;
    const hash = window.location.hash;
    let params: URLSearchParams | null = null;
    if (hash) {
      const parts = hash.split("?");
      if (parts.length > 1) params = new URLSearchParams(parts[1]);
    }
    if (!params || params.toString() === "") params = new URLSearchParams(window.location.search);
    const linesParam = params.get("lines");
    if (linesParam) { const p = parseInt(linesParam, 10); if (!isNaN(p)) linesToLoad = p; }
    const levelParam = params.get("level");
    if (levelParam && ["all","debug","info","warn","error"].includes(levelParam)) {
      filterLevel = levelParam as LevelFilter;
    }
    const q = params.get("q") ?? params.get("search");
    if (q) searchQuery = q;
  }

  // The source is only interesting when it exists: a console-format line has no `caller`, and
  // "other" on every line would be noise in a fixed column.
  function sourceOf(parsed: ParsedLogLine): string {
    return parsed.caller ? logSourceFromCaller(parsed.caller) : "";
  }

  onMount(() => {
    initFromUrl();
    loadLogs();
    initialized = true;
  });

  onDestroy(() => stopLiveReload());
</script>

<div class="flex flex-col" style="height: calc(100vh - 8rem)">
  <div class="mb-4 flex-none">
    <h1 class="text-screen-title text-heading">{T && T.title}</h1>
    <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
  </div>

  <!-- Controls -->
  <div class="mb-3 flex-none rounded-card border border-default bg-card">
    <div class="flex flex-col gap-3 border-b border-divider p-3 sm:flex-row sm:items-center">
      <label class="flex w-full shrink-0 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-1.5 sm:w-64">
        <Search size={16} strokeWidth={2} class="shrink-0 text-subtle" />
        <input
          type="search"
          placeholder={(T && T.searchPlaceholder) || ""}
          bind:value={searchQuery}
          class="w-full min-w-0 bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
        />
        {#if searchQuery}
          <button
            type="button"
            class="shrink-0 text-subtle hover:text-body"
            aria-label={(T && T.clearSearch) || ""}
            on:click={() => (searchQuery = "")}
          >
            <X size={14} strokeWidth={2} />
          </button>
        {/if}
      </label>

      <div class="flex items-center gap-2 overflow-x-auto" role="group" aria-label={(T && T.filtersLabel) || ""}>
        {#each levelPills as pill (pill.id)}
          <button
            type="button"
            aria-pressed={filterLevel === pill.id}
            on:click={() => (filterLevel = pill.id)}
            class="inline-flex shrink-0 items-center gap-1.5 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {filterLevel ===
            pill.id
              ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
              : 'border-default bg-control text-subtle hover:text-body'}"
          >
            {pill.label}
            <span class="font-mono text-[12px] opacity-70">{pill.count}</span>
          </button>
        {/each}
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2.5">
      <label class="flex items-center gap-2 text-caption text-subtle">
        {T && T.labelLines}
        <input
          type="number" min="100" max="10000" step="100"
          bind:value={linesToLoad}
          on:change={loadLogs}
          class="w-20 rounded-control border border-default bg-control px-2 py-1 font-mono text-caption text-heading outline-none focus:border-accent"
        />
      </label>

      <Checkbox bind:checked={autoScroll} label={(T && T.labelAutoscroll) || ""} />

      <div class="flex items-center gap-2">
        <Checkbox bind:checked={liveReload} label={(T && T.labelLive) || ""} />
        {#if liveReload}
          <!-- PulseDot only where something is really live (§6): the poll is on. -->
          <PulseDot variant="ok" size={7} />
          <select
            bind:value={liveReloadSeconds}
            on:change={startLiveReload}
            aria-label={(T && T.labelLive) || ""}
            class="rounded-control border border-default bg-control px-1.5 py-1 font-mono text-caption text-heading outline-none focus:border-accent"
          >
            <option value={3}>3s</option>
            <option value={5}>5s</option>
            <option value={10}>10s</option>
            <option value={30}>30s</option>
          </select>
        {/if}
      </div>

      <button
        type="button"
        class="ml-auto rounded-control border border-default px-3 py-1.5 text-caption font-semibold text-body transition-colors hover:bg-control"
        on:click={loadLogs}
      >
        {T && T.btnReload}
      </button>
    </div>
  </div>

  <!-- Body -->
  {#if loading}
    <div class="flex flex-1 items-center justify-center">
      <Loading message={T && T.loading || ""} />
    </div>
  {:else}
    <div class="relative min-h-0 flex-1">
      <div class="absolute inset-0 flex flex-col overflow-hidden rounded-card border border-default bg-sunken">
        <!-- Column header: same grid as the rows, so the columns don't misalign. Hidden on
             mobile, where the rows stack and there are no columns to label. -->
        <div
          class="hidden flex-none grid-cols-[82px_60px_90px_1fr] gap-2 border-b border-divider px-3 py-2 font-mono text-mono-label uppercase text-subtle md:grid"
        >
          <span>{T && T.colTime}</span>
          <span>{T && T.colLevel}</span>
          <span>{T && T.colSource}</span>
          <span>{T && T.colMessage}</span>
        </div>

        <div bind:this={logContainer} on:scroll={handleScroll} class="min-h-0 flex-1 overflow-y-auto">
          {#if parsedLogs.length === 0}
            <div class="flex h-full items-center justify-center">
              <p class="text-copy text-subtle">
                {filterLevel !== "all" || searchQuery ? (T && T.emptyFiltered) : (T && T.empty)}
              </p>
            </div>
          {:else}
            <!-- <ul>/<li> and not <div>: the rows ARE a list, and that gives screen readers and the
                 smoke tests a role anchor (`listitem`) instead of a CSS class — which here would
                 change with the breakpoint, since the grid only exists from `md` up. -->
            <ul>
            {#each parsedLogs as parsed}
              <!-- The 4-column grid only applies from `md` up. Below that, the three fixed columns
                   (82+60+90px) would leave ~130px for the message on a 390px screen, and it would
                   wrap one word per line; on mobile the metadata sits on one line and the message
                   below it, full width. Same reason as E11 in Downloads. -->
              <li
                class="group flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-divider px-3 py-1.5 hover:bg-control/60 md:grid md:grid-cols-[82px_60px_90px_1fr] md:items-start"
              >
                <span class="select-none font-mono text-[12.5px] text-subtle">{parsed.time}</span>

                <span
                  class="inline-flex justify-center rounded-badge border px-1 py-0.5 font-mono text-[11px] font-bold uppercase leading-none {LEVEL_BADGE[
                    parsed.level
                  ]}"
                >
                  <!-- The whole level: the old screen cut it to 4 characters ("DEBU", "ERRO") to
                       fit a 40px column; §9.5's 60px column fits the full word in 10px mono. -->
                  {parsed.level}
                </span>

                <span class="select-none truncate font-mono text-[12.5px] text-tertiary" title={parsed.caller ?? ""}>
                  {sourceOf(parsed)}
                </span>

                <span class="flex w-full min-w-0 items-start gap-2 md:w-auto">
                  <span class="min-w-0 flex-1 break-words font-mono text-[13.5px] {LEVEL_TEXT[parsed.level]}">
                    {@html highlightMatch(parsed.message, searchQuery)}
                    {#if parsed.extras}
                      <span class="text-subtle">{@html highlightMatch(parsed.extras, searchQuery)}</span>
                    {/if}
                  </span>
                  <button
                    type="button"
                    class="shrink-0 text-subtle opacity-0 transition-opacity hover:text-body group-hover:opacity-100"
                    aria-label={(T && T.copyLine) || ""}
                    on:click={() => copyLine(parsed.raw)}
                  >
                    <Copy size={13} strokeWidth={2} />
                  </button>
                </span>
              </li>
            {/each}
            </ul>
          {/if}
        </div>
      </div>

      {#if !atTop}
        <button
          type="button"
          class="absolute bottom-4 right-4 inline-flex h-9 w-9 items-center justify-center rounded-full border border-default bg-menu text-body shadow-elevation transition-colors hover:bg-control"
          aria-label={(T && T.scrollTop) || "Scroll to top"}
          on:click={scrollToTop}
        >
          {#if newLogsCount > 0}
            <span
              class="absolute -right-1.5 -top-1.5 rounded-pill bg-warn px-1.5 py-0.5 font-mono text-[10px] font-extrabold text-on-warn"
            >
              {T && T.newLogs(newLogsCount)}
            </span>
          {/if}
          <ArrowUp size={16} strokeWidth={2} />
        </button>
      {/if}
    </div>

    <div class="flex-none pt-2 font-mono text-[12px] text-subtle">
      {$locale && m.logs_x_of_y({ shown: parsedLogs.length, total: logs.length })}
      {#if filterLevel !== "all" || searchQuery}{T && T.filteredSuffix}{/if}
    </div>
  {/if}
</div>
