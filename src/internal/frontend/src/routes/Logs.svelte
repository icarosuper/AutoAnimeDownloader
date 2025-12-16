<script lang="ts">
  import { onMount } from "svelte";
  import { getLogs, type LogsResponse } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import ErrorMessage from "../components/ErrorMessage.svelte";

  let logs: string[] = [];
  let loading = true;
  let error: string | null = null;
  let linesToLoad = 1000;
  let filterLevel = "all"; // all, debug, info, warn, error
  let searchQuery = "";
  let filteredLogs: string[] = [];
  let parsedLogs: ParsedLogLine[] = [];
  let initialized = false;

  type ParsedLogLine = {
    level: string;
    timestamp: string;
    message: string;
    raw: string;
    extras?: string;
  };

  function parseLogLine(line: string): ParsedLogLine {
    // Try to parse zerolog format: {"level":"info","time":"2024-01-01T00:00:00Z","message":"..."}
    if (line.startsWith("{")) {
      try {
        const json = JSON.parse(line);

        const { level, time, message, ...rest } = json;

        const extras: Record<string, string> = {};
        for (const [key, value] of Object.entries(rest)) {
          try {
            if (typeof value === "object") {
              extras[key] = JSON.stringify(value);
            } else {
              extras[key] = String(value);
            }
          } catch {
            extras[key] = String(value);
          }
        }

        const extrasString =
          Object.keys(extras).length > 0
            ? Object.entries(extras)
                .map(([k, v]) => `${k}=${v}`)
                .join(" ")
            : undefined;

        return {
          level: level || "info",
          timestamp: time || "",
          message: message || line,
          raw: line,
          extras: extrasString,
        };
      } catch {
        // Not valid JSON, fall through
      }
    }

    // Try to parse console format: 2024-01-01T00:00:00Z INF message
    const levelMatch = line.match(/\b(DBG|INF|WRN|ERR|FAT)\b/);
    if (levelMatch) {
      const levelMap: Record<string, string> = {
        DBG: "debug",
        INF: "info",
        WRN: "warn",
        ERR: "error",
        FAT: "error",
      };
      return {
        level: levelMap[levelMatch[1]] || "info",
        timestamp: "",
        message: line,
        raw: line,
      };
    }

    // Default: try to detect level keywords
    const lowerLine = line.toLowerCase();
    let detectedLevel = "info";
    if (lowerLine.includes("error") || lowerLine.includes("err")) {
      detectedLevel = "error";
    } else if (lowerLine.includes("warn")) {
      detectedLevel = "warn";
    } else if (lowerLine.includes("debug") || lowerLine.includes("dbg")) {
      detectedLevel = "debug";
    }

    return {
      level: detectedLevel,
      timestamp: "",
      message: line,
      raw: line,
    };
  }

  function getLevelColor(level: string): string {
    switch (level) {
      case "error":
        return "text-red-600 dark:text-red-400";
      case "warn":
        return "text-yellow-600 dark:text-yellow-400";
      case "debug":
        return "text-gray-500 dark:text-gray-400";
      case "info":
      default:
        return "text-blue-600 dark:text-blue-400";
    }
  }

  function getLevelBgColor(level: string): string {
    switch (level) {
      case "error":
        return "bg-red-50 dark:bg-red-900/20";
      case "warn":
        return "bg-yellow-50 dark:bg-yellow-900/20";
      case "debug":
        return "bg-gray-50 dark:bg-gray-800";
      case "info":
      default:
        return "bg-blue-50 dark:bg-blue-900/20";
    }
  }

  function filterLogs(
    logs: string[],
    linesToLoad: number,
    filterLevel: string,
    searchQuery: string,
  ): string[] {
    let filtered = [...logs].slice(0, linesToLoad).reverse();

    if (filterLevel !== "all") {
      filtered = filtered.filter((line) => {
        const parsed = parseLogLine(line);
        return parsed.level === filterLevel;
      });
    }

    if (!searchQuery.trim()) return filtered;

    return filtered.filter((line) =>
      line.toLowerCase().includes(searchQuery.toLowerCase().trim()),
    );
  }

  $: filteredLogs = filterLogs(logs, linesToLoad, filterLevel, searchQuery);
  $: parsedLogs = filteredLogs.map((line) => parseLogLine(line));
  $: updateUrlQuery(linesToLoad, filterLevel, searchQuery);

  function updateUrlQuery(
    linesToLoad: number,
    filterLevel: string,
    searchQuery: string,
  ) {
    if (typeof window === "undefined" || !initialized) return;

    const params = new URLSearchParams();
    const currentHash = window.location.hash || "#/logs";

    // Extract the route path from hash (everything before ?)
    // Ensure we're always using /logs as the base path
    let hashPath = currentHash.split("?")[0];
    if (!hashPath || hashPath === "#" || !hashPath.includes("/logs")) {
      hashPath = "#/logs";
    }

    if (linesToLoad && linesToLoad !== 1000) {
      params.set("lines", String(linesToLoad));
    }

    if (filterLevel && filterLevel !== "all") {
      params.set("level", filterLevel);
    }

    const trimmedQuery = searchQuery.trim();
    if (trimmedQuery) {
      params.set("q", trimmedQuery);
    }

    const queryString = params.toString();

    // Build new hash with query params
    const newHash = queryString ? `${hashPath}?${queryString}` : hashPath;

    window.history.replaceState(
      {},
      "",
      window.location.pathname + window.location.search + newHash,
    );
  }

  async function loadLogs() {
    try {
      loading = true;
      error = null;
      const response: LogsResponse = await getLogs(linesToLoad);
      logs = response.lines;
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to load logs:", err);
    } finally {
      loading = false;
    }
  }

  function initFromUrl() {
    if (typeof window === "undefined") return;

    // Try to get params from hash first (for hash routing)
    let params: URLSearchParams | null = null;
    const hash = window.location.hash;

    if (hash) {
      const hashParts = hash.split("?");
      if (hashParts.length > 1) {
        params = new URLSearchParams(hashParts[1]);
      }
    }

    // Fallback to search params if not in hash
    if (!params || params.toString() === "") {
      params = new URLSearchParams(window.location.search);
    }

    const linesParam = params.get("lines");
    if (linesParam) {
      const parsed = parseInt(linesParam, 10);
      if (!Number.isNaN(parsed)) {
        linesToLoad = parsed;
      }
    }

    const levelParam = params.get("level");
    const validLevels = ["all", "debug", "info", "warn", "error"];
    if (levelParam && validLevels.includes(levelParam)) {
      filterLevel = levelParam;
    }

    const qParam = params.get("q") ?? params.get("search");
    if (qParam !== null) {
      searchQuery = qParam;
    }
  }

  onMount(() => {
    initFromUrl();
    loadLogs();
    initialized = true;
  });
</script>

<div>
  <div class="mb-6">
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">Logs</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Daemon logs and system messages
    </p>
  </div>

  {#if error}
    <div class="mb-6">
      <ErrorMessage message={error} />
    </div>
  {/if}

  <!-- Controls -->
  <div class="bg-white dark:bg-gray-800 shadow rounded-lg mb-6 p-4">
    <div class="flex flex-wrap items-center gap-4">
      <div class="flex items-center gap-2">
        <label
          for="lines-input"
          class="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          Lines:
        </label>
        <input
          id="lines-input"
          type="number"
          min="100"
          max="10000"
          step="100"
          bind:value={linesToLoad}
          class="w-24 rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:border-blue-500 focus:ring-blue-500 py-1 px-2"
        />
      </div>

      <div class="flex items-center gap-2">
        <label
          for="filter-level"
          class="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          Filter:
        </label>
        <select
          id="filter-level"
          bind:value={filterLevel}
          class="rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:border-blue-500 focus:ring-blue-500 py-1 px-2"
        >
          <option value="all">All</option>
          <option value="debug">Debug</option>
          <option value="info">Info</option>
          <option value="warn">Warn</option>
          <option value="error">Error</option>
        </select>
      </div>

      <div class="flex items-center gap-2 flex-1 min-w-[200px]">
        <label
          for="search-input"
          class="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          Search:
        </label>
        <input
          id="search-input"
          type="text"
          bind:value={searchQuery}
          placeholder="Search logs..."
          class="flex-1 rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:border-blue-500 focus:ring-blue-500 py-1 px-2"
        />
      </div>
    </div>
  </div>

  {#if loading}
    <Loading message="Loading logs..." />
  {:else if filteredLogs.length === 0}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {filterLevel === "all"
          ? "No logs available."
          : `No ${filterLevel} logs found.`}
      </p>
    </div>
  {:else}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
      <div
        class="h-[600px] overflow-y-auto p-4 font-mono text-sm bg-gray-900 dark:bg-black"
      >
        {#each parsedLogs as parsed}
          <div
            class="mb-2 px-3 py-1.5 rounded {getLevelBgColor(
              parsed.level,
            )} hover:opacity-80"
          >
            <span class="{getLevelColor(parsed.level)} font-semibold">
              [{parsed.level.toUpperCase()}]
            </span>
            {#if parsed.timestamp}
              <span class="text-gray-400 dark:text-gray-500 ml-2">
                {parsed.timestamp}
              </span>
            {/if}
            <span class="text-gray-300 dark:text-gray-400 ml-2">
              {parsed.message}
            </span>
            {#if parsed.extras}
              <span class="text-gray-400 dark:text-gray-500 ml-2">
                {parsed.extras}
              </span>
            {/if}
          </div>
        {/each}
      </div>

      <div
        class="px-4 py-2 bg-gray-50 dark:bg-gray-700 border-t border-gray-200 dark:border-gray-600"
      >
        <p class="text-xs text-gray-500 dark:text-gray-400">
          Showing {filteredLogs.length} of {logs.length} log lines
          {#if filterLevel !== "all"}
            (filtered by {filterLevel})
          {/if}
        </p>
      </div>
    </div>
  {/if}
</div>
