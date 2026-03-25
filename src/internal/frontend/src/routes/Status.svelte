<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    getStatus,
    getAnimes,
    getConfig,
    triggerCheck,
    startDaemon,
    stopDaemon,
    type StatusResponse,
    type AnimeInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import Loading from "../components/Loading.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { toast } from "../lib/stores/toast.js";

  let status: StatusResponse | null = null;
  let animes: AnimeInfo[] = [];
  let checkInterval = 0;
  let loading = true;
  let actionLoading = false;
  let search = "";
  let now = Date.now();

  type SortKey = "episodes_count" | "last_download_date";
  let sortKey: SortKey = "last_download_date";
  let sortDir: "asc" | "desc" = "desc";

  $: filteredAnimes = animes.filter(a =>
    a.name.toLowerCase().includes(search.toLowerCase())
  );

  $: sortedAnimes = [...filteredAnimes].sort((a, b) => {
    let valA = a[sortKey];
    let valB = b[sortKey];
    if (sortKey === "last_download_date") {
      valA = new Date((valA as string) || "1970-01-01").getTime() as any;
      valB = new Date((valB as string) || "1970-01-01").getTime() as any;
    }
    if (valA < valB) return sortDir === "asc" ? -1 : 1;
    if (valA > valB) return sortDir === "asc" ? 1 : -1;
    return 0;
  });

  $: totalEpisodes = animes.reduce((sum, a) => sum + a.episodes_count, 0);

  $: nextCheckIn = (() => {
    if (!status?.last_check || !checkInterval || status.status === "stopped") return null;
    const last = new Date(status.last_check).getTime();
    if (isNaN(last) || last < new Date("2010-01-01").getTime()) return null;
    const next = last + checkInterval * 60 * 1000;
    const diff = next - now;
    if (diff <= 0) return "soon";
    const mins = Math.floor(diff / 60000);
    const secs = Math.floor((diff % 60000) / 1000);
    return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`;
  })();

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === "desc" ? "asc" : "desc";
    } else {
      sortKey = key;
      sortDir = "desc";
    }
  }

  let wsClient: WebSocketClient | null = null;
  let animesPollInterval: ReturnType<typeof setInterval> | null = null;
  let tickInterval: ReturnType<typeof setInterval> | null = null;

  async function loadAnimes() {
    try {
      animes = await getAnimes();
    } catch (err) {
      console.error("Failed to load animes:", err);
    }
  }

  async function loadInitialData() {
    try {
      loading = true;
      const [statusData, animesData, configData] = await Promise.all([
        getStatus(),
        getAnimes(),
        getConfig(),
      ]);
      status = statusData;
      animes = animesData;
      checkInterval = configData.check_interval;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load data");
    } finally {
      loading = false;
    }
  }

  function handleWebSocketStatus(statusValue: string, lastCheck: string, hasError: boolean) {
    const previousStatus = status?.status;
    if (status) {
      status = { ...status, status: statusValue, last_check: lastCheck, has_error: hasError };
    } else {
      status = { status: statusValue, last_check: lastCheck, has_error: hasError };
    }
    if (previousStatus !== "running" && statusValue === "running") {
      loadAnimes();
    }
  }

  async function handleStart() {
    try {
      actionLoading = true;
      await startDaemon();
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to start daemon");
    } finally {
      actionLoading = false;
    }
  }

  async function handleStop() {
    try {
      actionLoading = true;
      await stopDaemon();
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to stop daemon");
    } finally {
      actionLoading = false;
    }
  }

  async function handleCheck() {
    try {
      actionLoading = true;
      await triggerCheck();
      toast.success("Check triggered");
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to trigger check");
    } finally {
      actionLoading = false;
    }
  }

  function formatDate(dateString: string) {
    if (!dateString) return "Never";
    const date = new Date(dateString);
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return "Never";
    return date.toLocaleString();
  }

  function formatTimeAgo(dateString: string): string {
    if (!dateString) return "";
    const date = new Date(dateString);
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return "";
    const diffMs = now - date.getTime();
    const diffSeconds = Math.floor(diffMs / 1000);
    const diffMinutes = Math.floor(diffSeconds / 60);
    const diffHours = Math.floor(diffMinutes / 60);
    const diffDays = Math.floor(diffHours / 24);
    if (diffSeconds < 60) return "just now";
    if (diffMinutes < 60) return `${diffMinutes}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    return `${diffDays}d ago`;
  }

  onMount(() => {
    loadInitialData();
    wsClient = new WebSocketClient();
    wsClient.connect(handleWebSocketStatus);
    animesPollInterval = setInterval(loadAnimes, 30000);
    tickInterval = setInterval(() => { now = Date.now(); }, 1000);
  });

  onDestroy(() => {
    wsClient?.disconnect();
    wsClient = null;
    if (animesPollInterval) clearInterval(animesPollInterval);
    if (tickInterval) clearInterval(tickInterval);
  });
</script>

<div class="space-y-6">
  <!-- Header -->
  <div>
    <h1 class="text-2xl font-semibold text-base-content">Status</h1>
    <p class="text-sm text-base-content/50 mt-0.5">Daemon monitoring and control</p>
  </div>

  {#if loading}
    <Loading message="Loading status..." />
  {:else if status}
    <!-- Stat Cards -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <!-- Daemon status -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">Daemon</span>
          <StatusBadge status={status.status} />
        </div>
      </div>

      <!-- Last check -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">Last Check</span>
          <span class="text-base font-medium text-base-content">
            {formatTimeAgo(status.last_check) || "Never"}
          </span>
          {#if status.last_check && formatTimeAgo(status.last_check)}
            <span class="text-xs text-base-content/40">{formatDate(status.last_check)}</span>
          {/if}
        </div>
      </div>

      <!-- Next check -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">Next Check</span>
          <span class="text-base font-medium text-base-content">
            {#if status.status === "stopped"}
              <span class="text-base-content/40">—</span>
            {:else if status.status === "checking"}
              <span class="text-warning">Checking...</span>
            {:else if nextCheckIn}
              {nextCheckIn}
            {:else}
              —
            {/if}
          </span>
        </div>
      </div>

      <!-- Totals -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">Library</span>
          <span class="text-base font-medium text-base-content">{animes.length} animes</span>
          <span class="text-xs text-base-content/40">{totalEpisodes} episodes</span>
        </div>
      </div>
    </div>

    <!-- Error warning -->
    {#if status.has_error && status.status !== "checking"}
      <div role="alert" class="alert alert-warning">
        <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
        </svg>
        <span class="text-sm">Error detected in last verification. Check the logs for details.</span>
      </div>
    {/if}

    <!-- Controls -->
    <div class="flex flex-wrap gap-2">
      {#if status.status === "stopped"}
        <button
          class="btn btn-primary btn-sm"
          on:click={handleStart}
          disabled={actionLoading}
        >
          {actionLoading ? "Starting..." : "Start Daemon"}
        </button>
      {:else}
        <button
          class="btn btn-error btn-sm"
          on:click={handleStop}
          disabled={actionLoading}
        >
          {actionLoading ? "Stopping..." : "Stop Daemon"}
        </button>
      {/if}
      <button
        class="btn btn-sm btn-outline"
        on:click={handleCheck}
        disabled={status.status === "checking" || actionLoading}
      >
        {status.status === "checking" ? "Checking..." : "Force Check"}
      </button>
    </div>

    <!-- Anime list -->
    <div class="card bg-base-200 border border-base-300">
      <div class="card-body p-4 gap-4">
        <!-- List header -->
        <div class="flex flex-col sm:flex-row sm:items-center gap-3">
          <h2 class="text-base font-medium text-base-content flex-1">Animes</h2>
          <!-- Search -->
          <label class="input input-sm input-bordered flex items-center gap-2 w-full sm:w-64">
            <svg class="w-4 h-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
            </svg>
            <input
              type="text"
              placeholder="Search animes..."
              bind:value={search}
              class="grow"
            />
            {#if search}
              <button class="opacity-50 hover:opacity-100" on:click={() => search = ""}>✕</button>
            {/if}
          </label>
          {#if search}
            <span class="text-xs text-base-content/50 whitespace-nowrap">
              {filteredAnimes.length} of {animes.length}
            </span>
          {/if}
        </div>

        {#if animes.length === 0}
          <!-- Empty state -->
          <div class="py-12 flex flex-col items-center gap-3 text-center">
            <svg class="w-12 h-12 text-base-content/20" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M7 4v16M17 4v16M3 8h4m10 0h4M3 12h18M3 16h4m10 0h4M4 20h16a1 1 0 001-1V5a1 1 0 00-1-1H4a1 1 0 00-1 1v14a1 1 0 001 1z"/>
            </svg>
            <div>
              <p class="font-medium text-base-content/60">No animes found</p>
              <p class="text-sm text-base-content/40 mt-1">
                Configure your Anilist username and start a check to begin
              </p>
            </div>
            <a href="#/config" class="btn btn-primary btn-sm mt-2">Go to Config</a>
          </div>
        {:else if filteredAnimes.length === 0}
          <div class="py-8 text-center">
            <p class="text-base-content/50">No animes match "<span class="font-medium">{search}</span>"</p>
          </div>
        {:else}
          <!-- Desktop Table -->
          <div class="hidden md:block overflow-x-auto">
            <table class="table table-sm w-full">
              <thead>
                <tr class="text-base-content/50">
                  <th>Name</th>
                  <th
                    class="cursor-pointer select-none hover:text-base-content"
                    on:click={() => handleSort("episodes_count")}
                  >
                    <span class="inline-flex items-center gap-1">
                      Episodes
                      {#if sortKey === "episodes_count"}
                        <span class="text-primary">{sortDir === "asc" ? "▲" : "▼"}</span>
                      {:else}
                        <span class="opacity-30">▲</span>
                      {/if}
                    </span>
                  </th>
                  <th>Progress</th>
                  <th
                    class="cursor-pointer select-none hover:text-base-content"
                    on:click={() => handleSort("last_download_date")}
                  >
                    <span class="inline-flex items-center gap-1">
                      Last Download
                      {#if sortKey === "last_download_date"}
                        <span class="text-primary">{sortDir === "asc" ? "▲" : "▼"}</span>
                      {:else}
                        <span class="opacity-30">▲</span>
                      {/if}
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {#each sortedAnimes as anime}
                  {@const pct = anime.total_episodes ? Math.round((anime.episodes_count / anime.total_episodes) * 100) : null}
                  <tr
                    class="hover {anime.anime_id ? 'cursor-pointer' : ''}"
                    on:click={() => anime.anime_id && (window.location.hash = `#/status/${anime.anime_id}`)}
                  >
                    <td class="font-medium">
                      {#if anime.anime_id}
                        <a
                          href="#/status/{anime.anime_id}"
                          class="link link-hover text-primary"
                          on:click|stopPropagation
                        >{anime.name}</a>
                      {:else}
                        {anime.name}
                      {/if}
                    </td>
                    <td class="text-base-content/60">
                      {anime.total_episodes ? `${anime.episodes_count}/${anime.total_episodes}` : anime.episodes_count}
                    </td>
                    <td class="min-w-32">
                      {#if pct !== null}
                        <div class="flex items-center gap-2">
                          <progress
                            class="progress w-24 {pct === 100 ? 'progress-success' : 'progress-primary'}"
                            value={pct}
                            max="100"
                          ></progress>
                          <span class="text-xs text-base-content/50">{pct}%</span>
                        </div>
                      {:else}
                        <span class="text-base-content/30 text-xs">—</span>
                      {/if}
                    </td>
                    <td class="text-base-content/60 text-xs">
                      {formatTimeAgo(anime.last_download_date) || formatDate(anime.last_download_date)}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>

          <!-- Mobile Cards -->
          <div class="md:hidden space-y-2">
            {#each sortedAnimes as anime}
              {@const pct = anime.total_episodes ? Math.round((anime.episodes_count / anime.total_episodes) * 100) : null}
              <div
                class="rounded-lg bg-base-100 border border-base-300 p-3 {anime.anime_id ? 'cursor-pointer active:opacity-80' : ''}"
                on:click={() => anime.anime_id && (window.location.hash = `#/status/${anime.anime_id}`)}
              >
                <div class="flex items-start justify-between gap-2 mb-2">
                  <p class="text-sm font-medium text-base-content truncate">
                    {anime.name}
                  </p>
                  <span class="text-xs text-base-content/50 whitespace-nowrap shrink-0">
                    {anime.total_episodes ? `${anime.episodes_count}/${anime.total_episodes}` : `${anime.episodes_count} eps`}
                  </span>
                </div>
                {#if pct !== null}
                  <div class="flex items-center gap-2">
                    <progress
                      class="progress flex-1 {pct === 100 ? 'progress-success' : 'progress-primary'}"
                      value={pct}
                      max="100"
                    ></progress>
                    <span class="text-xs text-base-content/50">{pct}%</span>
                  </div>
                {/if}
                <p class="text-xs text-base-content/40 mt-1">
                  {formatTimeAgo(anime.last_download_date) || formatDate(anime.last_download_date)}
                </p>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
