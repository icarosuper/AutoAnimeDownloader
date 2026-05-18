<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { getLogs, type LogsResponse } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.logs_title(),
    subtitle: m.logs_subtitle(),
    labelLines: m.logs_label_lines(),
    labelLevel: m.logs_label_level(),
    searchPlaceholder: m.logs_search_placeholder(),
    labelAutoscroll: m.logs_label_autoscroll(),
    labelLive: m.logs_label_live(),
    btnReload: m.logs_btn_reload(),
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

  let logs: string[] = [];
  let loading = true;
  let linesToLoad = 1000;
  let filterLevel = "all";
  let searchQuery = "";
  let autoScroll = true;
  let initialized = false;
  let logContainer: HTMLElement;
  let atTop = true;
  let newLogsCount = 0;
  let liveReload = true;
  let liveReloadSeconds = 5;
  let liveInterval: ReturnType<typeof setInterval> | null = null;

  type ParsedLogLine = {
    level: string;
    timestamp: string;
    message: string;
    raw: string;
    extras?: string;
  };

  function escapeHtml(text: string): string {
    return text
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function highlightMatch(text: string, query: string): string {
    const safe = escapeHtml(text);
    if (!query.trim()) return safe;
    const escapedQuery = escapeHtml(query.trim()).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    return safe.replace(
      new RegExp(`(${escapedQuery})`, "gi"),
      '<mark class="bg-warning text-warning-content rounded px-0.5">$1</mark>'
    );
  }

  function parseLogLine(line: string): ParsedLogLine {
    if (line.startsWith("{")) {
      try {
        const json = JSON.parse(line);
        const { level, time, message, ...rest } = json;
        const extras: Record<string, string> = {};
        for (const [key, value] of Object.entries(rest)) {
          extras[key] = typeof value === "object" ? JSON.stringify(value) : String(value);
        }
        const extrasString = Object.keys(extras).length > 0
          ? Object.entries(extras).map(([k, v]) => `"${k}"="${v}"`).join(" ")
          : undefined;
        return { level: level || "info", timestamp: time || "", message: message || line, raw: line, extras: extrasString };
      } catch { /* fall through */ }
    }

    const levelMatch = line.match(/\b(DBG|INF|WRN|ERR|FAT)\b/);
    if (levelMatch) {
      const levelMap: Record<string, string> = { DBG: "debug", INF: "info", WRN: "warn", ERR: "error", FAT: "error" };
      return { level: levelMap[levelMatch[1]] || "info", timestamp: "", message: line, raw: line };
    }

    const lower = line.toLowerCase();
    const level = lower.includes("error") || lower.includes("err") ? "error"
      : lower.includes("warn") ? "warn"
      : lower.includes("debug") || lower.includes("dbg") ? "debug"
      : "info";
    return { level, timestamp: "", message: line, raw: line };
  }

  function getLevelColor(level: string): string {
    switch (level) {
      case "error": return "text-error";
      case "warn":  return "text-warning";
      case "debug": return "text-base-content/40";
      default:      return "text-info";
    }
  }

  function filterLogs(logs: string[], linesToLoad: number, filterLevel: string, searchQuery: string): string[] {
    let filtered = [...logs].slice(0, linesToLoad).reverse();
    if (filterLevel !== "all") filtered = filtered.filter(l => parseLogLine(l).level === filterLevel);
    if (searchQuery.trim()) filtered = filtered.filter(l => l.toLowerCase().includes(searchQuery.toLowerCase().trim()));
    return filtered;
  }

  $: filteredLogs = filterLogs(logs, linesToLoad, filterLevel, searchQuery);
  $: parsedLogs = filteredLogs.map(parseLogLine);
  $: updateUrlQuery(linesToLoad, filterLevel, searchQuery);
  $: if (parsedLogs && autoScroll) scrollToTop();
  $: if (liveReload) startLiveReload(); else stopLiveReload();

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
      await navigator.clipboard.writeText(text);
      toast.success(m.logs_copy_success());
    } catch {
      toast.error(m.logs_copy_error());
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
    if (levelParam && ["all","debug","info","warn","error"].includes(levelParam)) filterLevel = levelParam;
    const q = params.get("q") ?? params.get("search");
    if (q) searchQuery = q;
  }

  onMount(() => {
    initFromUrl();
    loadLogs();
    initialized = true;
  });

  onDestroy(() => stopLiveReload());
</script>

<div class="flex flex-col" style="height: calc(100vh - 8rem)">
  <!-- Header -->
  <div class="mb-4 flex-none">
    <h1 class="text-2xl font-semibold text-base-content">{T && T.title}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{T && T.subtitle}</p>
  </div>

  <!-- Controls -->
  <div class="card bg-base-200 border border-base-300 mb-4 flex-none">
    <div class="card-body p-3">
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 text-sm text-base-content/70">
          {T && T.labelLines}
          <input
            type="number" min="100" max="10000" step="100"
            bind:value={linesToLoad}
            on:change={loadLogs}
            class="input input-xs input-bordered w-24"
          />
        </label>

        <label class="flex items-center gap-2 text-sm text-base-content/70">
          {T && T.labelLevel}
          <select bind:value={filterLevel} class="select select-xs select-bordered">
            <option value="all">{T && T.levelAll}</option>
            <option value="debug">{T && T.levelDebug}</option>
            <option value="info">{T && T.levelInfo}</option>
            <option value="warn">{T && T.levelWarn}</option>
            <option value="error">{T && T.levelError}</option>
          </select>
        </label>

        <label class="input input-xs input-bordered flex items-center gap-2 flex-1 min-w-40">
          <svg class="w-3 h-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
          </svg>
          <input type="text" placeholder={T && T.searchPlaceholder || ""} bind:value={searchQuery} class="grow" />
          {#if searchQuery}
            <button class="opacity-50 hover:opacity-100" on:click={() => searchQuery = ""}>✕</button>
          {/if}
        </label>

        <label class="flex items-center gap-2 text-sm text-base-content/70 cursor-pointer">
          <input type="checkbox" bind:checked={autoScroll} class="checkbox checkbox-xs" />
          {T && T.labelAutoscroll}
        </label>

        <label class="flex items-center gap-2 text-sm text-base-content/70 cursor-pointer">
          <input type="checkbox" bind:checked={liveReload} class="checkbox checkbox-xs" />
          {T && T.labelLive}
          {#if liveReload}
            <select bind:value={liveReloadSeconds} on:change={startLiveReload} class="select select-xs select-bordered w-16">
              <option value={3}>3s</option>
              <option value={5}>5s</option>
              <option value={10}>10s</option>
              <option value={30}>30s</option>
            </select>
          {/if}
        </label>

        <button class="btn btn-xs btn-outline ml-auto" on:click={loadLogs}>{T && T.btnReload}</button>
      </div>
    </div>
  </div>

  <!-- Log container -->
  {#if loading}
    <div class="flex-1 flex items-center justify-center">
      <Loading message={T && T.loading || ""} />
    </div>
  {:else}
    <div class="flex-1 relative min-h-0">
      <div
        bind:this={logContainer}
        on:scroll={handleScroll}
        class="absolute inset-0 overflow-y-auto font-mono text-xs bg-base-300 rounded-lg border border-base-300 p-2 space-y-0.5"
      >
        {#if parsedLogs.length === 0}
          <div class="flex items-center justify-center h-full">
            <p class="text-base-content/40">
              {filterLevel !== "all" || searchQuery ? (T && T.emptyFiltered) : (T && T.empty)}
            </p>
          </div>
        {:else}
          {#each parsedLogs as parsed}
            <div class="group flex items-start gap-2 px-2 py-1 rounded hover:bg-base-content/5">
              <span class="{getLevelColor(parsed.level)} font-semibold w-10 shrink-0 select-none">
                {parsed.level.slice(0, 4).toUpperCase()}
              </span>
              {#if parsed.timestamp}
                <span class="text-base-content/30 shrink-0">{parsed.timestamp}</span>
              {/if}
              <span class="text-base-content/80 break-all flex-1">
                {@html highlightMatch(parsed.message, searchQuery)}
                {#if parsed.extras}
                  <span class="text-base-content/40 ml-1">{@html highlightMatch(parsed.extras, searchQuery)}</span>
                {/if}
              </span>
              <button
                class="opacity-0 group-hover:opacity-60 hover:!opacity-100 shrink-0 text-base-content/50 transition-opacity"
                title="Copy"
                on:click={() => copyLine(parsed.raw)}
              >
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                </svg>
              </button>
            </div>
          {/each}
        {/if}
      </div>

      <!-- Scroll to top button -->
      {#if !atTop}
        <button
          class="absolute bottom-4 right-4 btn btn-circle btn-sm btn-neutral shadow-lg opacity-80 hover:opacity-100"
          title={T && T.scrollTop || "Scroll to top"}
          on:click={scrollToTop}
        >
          {#if newLogsCount > 0}
            <span class="absolute -top-2 -right-2 badge badge-warning badge-xs text-xs font-bold px-1 min-w-fit">
              {T && T.newLogs(newLogsCount)}
            </span>
          {/if}
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7"/>
          </svg>
        </button>
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex-none pt-2 text-xs text-base-content/40">
      {$locale && m.logs_x_of_y({ shown: filteredLogs.length, total: logs.length })}
      {#if filterLevel !== "all" || searchQuery}{T && T.filteredSuffix}{/if}
    </div>
  {/if}
</div>
