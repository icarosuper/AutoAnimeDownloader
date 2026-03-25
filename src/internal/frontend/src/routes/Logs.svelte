<script lang="ts">
  import { onMount, tick } from "svelte";
  import { getLogs, type LogsResponse } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";

  let logs: string[] = [];
  let loading = true;
  let linesToLoad = 1000;
  let filterLevel = "all";
  let searchQuery = "";
  let autoScroll = true;
  let initialized = false;
  let logContainer: HTMLElement;

  type ParsedLogLine = {
    level: string;
    timestamp: string;
    message: string;
    raw: string;
    extras?: string;
  };

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
  $: if (parsedLogs && autoScroll) scrollToBottom();

  async function scrollToBottom() {
    await tick();
    if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
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
      loading = true;
      const response: LogsResponse = await getLogs(linesToLoad);
      logs = response.lines;
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
</script>

<div class="flex flex-col" style="height: calc(100vh - 8rem)">
  <!-- Header -->
  <div class="mb-4 flex-none">
    <h1 class="text-2xl font-semibold text-base-content">{m.logs_title()}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{m.logs_subtitle()}</p>
  </div>

  <!-- Controls -->
  <div class="card bg-base-200 border border-base-300 mb-4 flex-none">
    <div class="card-body p-3">
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 text-sm text-base-content/70">
          {m.logs_label_lines()}
          <input
            type="number" min="100" max="10000" step="100"
            bind:value={linesToLoad}
            on:change={loadLogs}
            class="input input-xs input-bordered w-24"
          />
        </label>

        <label class="flex items-center gap-2 text-sm text-base-content/70">
          {m.logs_label_level()}
          <select bind:value={filterLevel} class="select select-xs select-bordered">
            <option value="all">{m.logs_level_all()}</option>
            <option value="debug">{m.logs_level_debug()}</option>
            <option value="info">{m.logs_level_info()}</option>
            <option value="warn">{m.logs_level_warn()}</option>
            <option value="error">{m.logs_level_error()}</option>
          </select>
        </label>

        <label class="input input-xs input-bordered flex items-center gap-2 flex-1 min-w-40">
          <svg class="w-3 h-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
          </svg>
          <input type="text" placeholder={m.logs_search_placeholder()} bind:value={searchQuery} class="grow" />
          {#if searchQuery}
            <button class="opacity-50 hover:opacity-100" on:click={() => searchQuery = ""}>✕</button>
          {/if}
        </label>

        <label class="flex items-center gap-2 text-sm text-base-content/70 cursor-pointer">
          <input type="checkbox" bind:checked={autoScroll} class="checkbox checkbox-xs" />
          {m.logs_label_autoscroll()}
        </label>

        <button class="btn btn-xs btn-outline ml-auto" on:click={loadLogs}>{m.logs_btn_reload()}</button>
      </div>
    </div>
  </div>

  <!-- Log container -->
  {#if loading}
    <div class="flex-1 flex items-center justify-center">
      <Loading message={m.logs_loading()} />
    </div>
  {:else}
    <div class="flex-1 relative min-h-0">
      <div
        bind:this={logContainer}
        class="absolute inset-0 overflow-y-auto font-mono text-xs bg-base-300 rounded-lg border border-base-300 p-2 space-y-0.5"
      >
        {#if parsedLogs.length === 0}
          <div class="flex items-center justify-center h-full">
            <p class="text-base-content/40">
              {filterLevel !== "all" || searchQuery ? m.logs_empty_filtered() : m.logs_empty()}
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
                {parsed.message}
                {#if parsed.extras}
                  <span class="text-base-content/40 ml-1">{parsed.extras}</span>
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

      <!-- Scroll to bottom button -->
      <button
        class="absolute bottom-4 right-4 btn btn-circle btn-sm btn-neutral shadow-lg opacity-80 hover:opacity-100"
        title="Scroll to bottom"
        on:click={scrollToBottom}
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
        </svg>
      </button>
    </div>

    <!-- Footer -->
    <div class="flex-none pt-2 text-xs text-base-content/40">
      {m.logs_x_of_y({ shown: filteredLogs.length, total: logs.length })}
      {#if filterLevel !== "all" || searchQuery}{m.logs_filtered_suffix()}{/if}
    </div>
  {/if}
</div>
