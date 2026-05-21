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
  import { wsConnectionState } from "../lib/stores/wsState.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { filterAnimes, sortAnimes, computeNextCheckIn, type SortKey, type SortDir } from "../lib/utils/status.js";

  // Reactive translations — re-evaluated when $locale changes, no remount needed
  $: T = $locale && {
    title: m.status_title(),
    subtitle: m.status_subtitle(),
    cardDaemon: m.status_card_daemon(),
    cardLastCheck: m.status_card_last_check(),
    cardNextCheck: m.status_card_next_check(),
    cardLibrary: m.status_card_library(),
    checking: m.status_checking(),
    never: m.common_never(),
    errorAlert: m.status_error_alert(),
    start: m.status_start(),
    starting: m.status_starting(),
    stop: m.status_stop(),
    stopping: m.status_stopping(),
    forceCheck: m.status_force_check(),
    animesHeader: m.status_animes_header(),
    searchPlaceholder: m.status_search_placeholder(),
    colName: m.status_col_name(),
    colBlacklist: m.status_col_blacklist(),
    colProgress: m.status_col_progress(),
    colLastDownload: m.status_col_last_download(),
    emptyTitle: m.status_empty_title(),
    emptyDesc: m.status_empty_desc(),
    goToConfig: m.status_go_to_config(),
    filterUnwatched: m.status_filter_unwatched(),
    legendWatched: m.status_legend_watched(),
    legendReleased: m.status_legend_released(),
    legendTotal: m.status_legend_total(),
  }

  let status: StatusResponse | null = null;
  let animes: AnimeInfo[] = [];
  let checkInterval = 0;
  let loading = true;
  let actionLoading = false;
  let search = "";
  let filterUnwatched = false;
  let now = Date.now();

  let sortKey: SortKey = "last_download_date";
  let sortDir: SortDir = "desc";

  $: filteredAnimes = filterAnimes(animes, search, filterUnwatched);
  $: sortedAnimes = sortAnimes(filteredAnimes, sortKey, sortDir);

  $: totalEpisodes = animes.reduce((sum, a) => sum + a.episodes_downloaded, 0);

  $: nextCheckIn = status
    ? computeNextCheckIn(status.last_check, checkInterval, status.status, now)
    : null;

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === "desc" ? "asc" : "desc";
    } else {
      sortKey = key;
      sortDir = key === "name" ? "asc" : "desc";
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
      status = { status: statusValue, last_check: lastCheck, has_error: hasError, version: "" };
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
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to trigger check");
    } finally {
      actionLoading = false;
    }
  }

  function formatDate(dateString: string) {
    if (!dateString) return m.common_never();
    const date = new Date(dateString);
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return m.common_never();
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
    const rtf = new Intl.RelativeTimeFormat($locale, { numeric: "auto" });
    if (diffSeconds < 60) return rtf.format(-diffSeconds, "second");
    if (diffMinutes < 60) return rtf.format(-diffMinutes, "minute");
    if (diffHours < 24) return rtf.format(-diffHours, "hour");
    return rtf.format(-diffDays, "day");
  }

  onMount(() => {
    loadInitialData();
    wsClient = new WebSocketClient();
    wsClient.connect(handleWebSocketStatus, (state) => wsConnectionState.set(state));
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
    <h1 class="text-2xl font-semibold text-base-content">{T && T.title}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{T && T.subtitle}</p>
  </div>

  {#if loading}
    <Loading message="Loading status..." />
  {:else if status}
    <!-- Stat Cards -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <!-- Daemon status -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardDaemon}</span>
          <StatusBadge status={status.status} />
        </div>
      </div>

      <!-- Last check -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardLastCheck}</span>
          <span class="text-base font-medium text-base-content">
            {formatTimeAgo(status.last_check) || (T && T.never)}
          </span>
          {#if status.last_check && formatTimeAgo(status.last_check)}
            <span class="text-xs text-base-content/40">{formatDate(status.last_check)}</span>
          {/if}
        </div>
      </div>

      <!-- Next check -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-4 gap-1">
          <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardNextCheck}</span>
          <span class="text-base font-medium text-base-content">
            {#if status.status === "stopped"}
              <span class="text-base-content/40">—</span>
            {:else if status.status === "checking"}
              <span class="text-warning">{T && T.checking}</span>
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
          <span class="text-xs text-base-content/50 uppercase tracking-wider">{T && T.cardLibrary}</span>
          <span class="text-base font-medium text-base-content">{$locale && m.status_animes_count({ count: animes.length })}</span>
          <span class="text-xs text-base-content/40">{$locale && m.status_episodes_count({ count: totalEpisodes })}</span>
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
        <span class="text-sm">{T && T.errorAlert}</span>
      </div>
    {/if}

    <!-- Controls -->
    <div class="flex flex-wrap gap-2">
      {#if status.status === "stopped"}
        <button
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          on:click={handleStart}
          disabled={actionLoading}
        >
          {actionLoading ? (T && T.starting) : (T && T.start)}
        </button>
      {:else}
        <button
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-red-600 hover:bg-red-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          on:click={handleStop}
          disabled={actionLoading}
        >
          {actionLoading ? (T && T.stopping) : (T && T.stop)}
        </button>
      {/if}
      <button
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        on:click={handleCheck}
        disabled={status.status === "checking" || actionLoading}
      >
        {status.status === "checking" ? (T && T.checking) : (T && T.forceCheck)}
      </button>
    </div>

    <!-- Anime list -->
    <div class="card bg-base-200 border border-base-300">
      <div class="card-body p-4 gap-4">
        <!-- List header -->
        <div class="flex flex-col sm:flex-row sm:items-center gap-3">
          <h2 class="text-base font-medium text-base-content flex-1">{T && T.animesHeader}</h2>
          {#if search || filterUnwatched}
            <span class="text-xs text-base-content/50 whitespace-nowrap">
              {$locale && m.status_x_of_y({ shown: filteredAnimes.length, total: animes.length })}
            </span>
          {/if}
          <!-- Search -->
          <label class="input input-sm input-bordered flex items-center gap-2 w-full sm:w-64">
            <svg class="w-4 h-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
            </svg>
            <input
              type="text"
              placeholder={T && T.searchPlaceholder || ""}
              bind:value={search}
              class="grow"
            />
            {#if search}
              <button class="opacity-50 hover:opacity-100" on:click={() => search = ""}>✕</button>
            {/if}
          </label>
          <!-- Unwatched filter chip -->
          <button
            class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium transition-colors {filterUnwatched ? 'bg-blue-600 text-white' : 'bg-base-300 text-base-content/60 hover:bg-base-300/80'}"
            on:click={() => filterUnwatched = !filterUnwatched}
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
            </svg>
            {T && T.filterUnwatched}
          </button>
        </div>

        {#if animes.length === 0}
          <!-- Empty state -->
          <div class="py-12 flex flex-col items-center gap-3 text-center">
            <svg class="w-12 h-12 text-base-content/20" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M7 4v16M17 4v16M3 8h4m10 0h4M3 12h18M3 16h4m10 0h4M4 20h16a1 1 0 001-1V5a1 1 0 00-1-1H4a1 1 0 00-1 1v14a1 1 0 001 1z"/>
            </svg>
            <div>
              <p class="font-medium text-base-content/60">{T && T.emptyTitle}</p>
              <p class="text-sm text-base-content/40 mt-1">{T && T.emptyDesc}</p>
            </div>
            <a href="#/config" class="btn btn-primary btn-sm mt-2">{T && T.goToConfig}</a>
          </div>
        {:else if filteredAnimes.length === 0}
          <div class="py-8 text-center">
            <p class="text-base-content/50">{$locale && m.status_no_results({ search })}</p>
          </div>
        {:else}
          <!-- Desktop Table -->
          <div class="hidden md:block overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table class="w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead class="bg-gray-50 dark:bg-gray-700">
                <tr>
                  <th class="px-4 py-3 w-14"></th>
                  <th
                    class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer select-none hover:text-gray-700 dark:hover:text-white"
                    on:click={() => handleSort("name")}
                  >
                    <span class="inline-flex items-center gap-1">
                      {T && T.colName}
                      {#if sortKey === "name"}
                        <span class="text-blue-500">{sortDir === "asc" ? "▲" : "▼"}</span>
                      {:else}
                        <span class="opacity-30">▲</span>
                      {/if}
                    </span>
                  </th>
                  <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider" title={T && T.colBlacklist}>{T && T.colBlacklist}</th>
                  <th
                    class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer select-none hover:text-gray-700 dark:hover:text-white"
                    on:click={() => handleSort("episodes_watched")}
                  >
                    <div class="inline-flex items-center justify-center gap-1">
                      {T && T.colProgress}
                      {#if sortKey === "episodes_watched"}
                        <span class="text-blue-500">{sortDir === "asc" ? "▲" : "▼"}</span>
                      {:else}
                        <span class="opacity-30">▲</span>
                      {/if}
                    </div>
                    <div class="flex items-center justify-center gap-2 mt-1 font-normal normal-case tracking-normal text-[10px] text-gray-400 dark:text-gray-500">
                      <span class="flex items-center gap-1">
                        <span class="w-2 h-2 rounded-sm bg-success inline-block"></span>
                        {T && T.legendWatched}
                      </span>
                      <span class="flex items-center gap-1">
                        <span class="w-2 h-2 rounded-sm bg-primary inline-block"></span>
                        {T && T.legendReleased}
                      </span>
                      <span class="flex items-center gap-1">
                        <span class="w-2 h-2 rounded-sm bg-base-300 border border-base-content/20 inline-block"></span>
                        {T && T.legendTotal}
                      </span>
                    </div>
                  </th>
                  <th
                    class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer select-none hover:text-gray-700 dark:hover:text-white"
                    on:click={() => handleSort("last_download_date")}
                  >
                    <span class="inline-flex items-center justify-center gap-1">
                      {T && T.colLastDownload}
                      {#if sortKey === "last_download_date"}
                        <span class="text-blue-500">{sortDir === "asc" ? "▲" : "▼"}</span>
                      {:else}
                        <span class="opacity-30">▲</span>
                      {/if}
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                {#each sortedAnimes as anime}
                  {@const _denom = anime.total_episodes > 0 ? anime.total_episodes : anime.episodes_released}
                  {@const _watched = _denom ? Math.min(anime.episodes_watched, _denom) : 0}
                  {@const _released = anime.total_episodes > 0 ? Math.min(Math.max(anime.episodes_released, _watched), anime.total_episodes) : _denom}
                  {@const watchedPct = _denom ? (_watched / _denom) * 100 : null}
                  {@const releasedPct = _denom ? ((_released - _watched) / _denom) * 100 : 0}
                  <tr
                    class="hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors {anime.anime_id ? 'cursor-pointer' : ''}"
                    on:click={() => anime.anime_id && (window.location.hash = `#/status/${anime.anime_id}`)}
                  >
                    <td class="px-0 py-1 pl-3 w-14 min-w-[3.5rem]">
                      {#if anime.cover_image}
                        <img
                          src={anime.cover_image}
                          alt={anime.name}
                          class="block w-12 h-16 object-cover rounded"
                          loading="lazy"
                        />
                      {:else}
                        <div class="w-12 h-16 rounded bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
                          <svg class="w-5 h-5 text-gray-300 dark:text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/>
                          </svg>
                        </div>
                      {/if}
                    </td>
                    <td class="px-6 py-4 text-sm font-medium text-gray-900 dark:text-white max-w-xs">
                      <div class="relative group/name">
                        {#if anime.anime_id}
                          <a
                            href="#/status/{anime.anime_id}"
                            class="text-blue-600 dark:text-blue-400 hover:underline block truncate"
                            on:click|stopPropagation
                          >{anime.name}</a>
                        {:else}
                          <span class="block truncate">{anime.name}</span>
                        {/if}
                        <div class="absolute bottom-full left-0 mb-2.5 invisible group-hover/name:visible z-50 pointer-events-none">
                          <div class="bg-gray-900 dark:bg-gray-950 text-white text-xs rounded-lg px-3 py-2 shadow-xl border border-white/10 max-w-xs">
                            <span class="text-gray-100 leading-snug">{anime.name}</span>
                          </div>
                          <div class="absolute top-full left-4 border-[5px] border-transparent border-t-gray-900 dark:border-t-gray-950"></div>
                        </div>
                      </div>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-center">
                      {#if anime.is_blacklisted}
                        <svg class="w-4 h-4 text-red-500 inline-block" aria-label={T && T.colBlacklist} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <circle cx="12" cy="12" r="10"/>
                          <line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
                        </svg>
                      {:else}
                        <span class="text-gray-300 dark:text-gray-600">—</span>
                      {/if}
                    </td>
                    <td class="px-6 py-4 min-w-36">
                      {#if watchedPct !== null}
                        <div class="flex items-center justify-center gap-2">
                          <div class="relative group/bar">
                            <div class="flex h-2 w-24 rounded-full overflow-hidden bg-base-300 cursor-default">
                              <div class="h-full bg-success" style="width: {watchedPct}%"></div>
                              <div class="h-full bg-primary" style="width: {releasedPct}%"></div>
                            </div>
                            <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2.5 invisible group-hover/bar:visible z-50 pointer-events-none">
                              <div class="bg-gray-900 dark:bg-gray-950 text-white text-xs rounded-lg px-3 py-2 shadow-xl whitespace-nowrap space-y-1.5 border border-white/10">
                                <div class="flex items-center gap-2">
                                  <span class="w-2 h-2 rounded-sm bg-success shrink-0"></span>
                                  <span class="text-gray-300">{T && T.legendWatched}</span>
                                  <span class="ml-auto font-semibold pl-4">{_watched}</span>
                                </div>
                                <div class="flex items-center gap-2">
                                  <span class="w-2 h-2 rounded-sm bg-primary shrink-0"></span>
                                  <span class="text-gray-300">{T && T.legendReleased}</span>
                                  <span class="ml-auto font-semibold pl-4">{_released}</span>
                                </div>
                                {#if anime.total_episodes > 0}
                                  <div class="flex items-center gap-2 border-t border-white/10 pt-1.5">
                                    <span class="w-2 h-2 rounded-sm bg-gray-500 shrink-0"></span>
                                    <span class="text-gray-300">{T && T.legendTotal}</span>
                                    <span class="ml-auto font-semibold pl-4">{anime.total_episodes}</span>
                                  </div>
                                {/if}
                              </div>
                              <div class="absolute top-full left-1/2 -translate-x-1/2 border-[5px] border-transparent border-t-gray-900 dark:border-t-gray-950"></div>
                            </div>
                          </div>
                          <span class="text-xs text-gray-500 dark:text-gray-400">{_watched}/{_denom}</span>
                        </div>
                      {:else}
                        <span class="text-gray-300 dark:text-gray-600 text-xs text-center block">—</span>
                      {/if}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400 text-center">
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
              {@const _denom = anime.total_episodes > 0 ? anime.total_episodes : anime.episodes_released}
              {@const _watched = _denom ? Math.min(anime.episodes_watched, _denom) : 0}
              {@const _released = anime.total_episodes > 0 ? Math.min(Math.max(anime.episodes_released, _watched), anime.total_episodes) : _denom}
              {@const watchedPct = _denom ? (_watched / _denom) * 100 : null}
              {@const releasedPct = _denom ? ((_released - _watched) / _denom) * 100 : 0}
              <div
                class="rounded-lg bg-base-100 border border-base-300 p-3 {anime.anime_id ? 'cursor-pointer active:opacity-80' : ''}"
                on:click={() => anime.anime_id && (window.location.hash = `#/status/${anime.anime_id}`)}
              >
                <div class="flex gap-3">
                  {#if anime.cover_image}
                    <img
                      src={anime.cover_image}
                      alt={anime.name}
                      class="w-12 h-16 object-cover rounded shrink-0"
                      loading="lazy"
                    />
                  {/if}
                  <div class="flex-1 min-w-0">
                <div class="flex items-start justify-between gap-2 mb-2">
                  <div class="flex items-center gap-1.5 min-w-0">
                    <p class="text-sm font-medium text-base-content truncate">
                      {anime.name}
                    </p>
                    {#if anime.is_blacklisted}
                      <svg class="w-3.5 h-3.5 text-red-500 shrink-0" aria-label={T && T.colBlacklist} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <circle cx="12" cy="12" r="10"/>
                        <line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
                      </svg>
                    {/if}
                  </div>
                </div>
                {#if watchedPct !== null}
                  <div class="flex items-center gap-2">
                    <div class="relative group/bar flex-1">
                      <div class="flex h-2 w-full rounded-full overflow-hidden bg-base-300 cursor-default">
                        <div class="h-full bg-success" style="width: {watchedPct}%"></div>
                        <div class="h-full bg-primary" style="width: {releasedPct}%"></div>
                      </div>
                      <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2.5 invisible group-hover/bar:visible z-50 pointer-events-none">
                        <div class="bg-gray-900 dark:bg-gray-950 text-white text-xs rounded-lg px-3 py-2 shadow-xl whitespace-nowrap space-y-1.5 border border-white/10">
                          <div class="flex items-center gap-2">
                            <span class="w-2 h-2 rounded-sm bg-success shrink-0"></span>
                            <span class="text-gray-300">{T && T.legendWatched}</span>
                            <span class="ml-auto font-semibold pl-4">{_watched}</span>
                          </div>
                          <div class="flex items-center gap-2">
                            <span class="w-2 h-2 rounded-sm bg-primary shrink-0"></span>
                            <span class="text-gray-300">{T && T.legendReleased}</span>
                            <span class="ml-auto font-semibold pl-4">{_released}</span>
                          </div>
                          {#if anime.total_episodes > 0}
                            <div class="flex items-center gap-2 border-t border-white/10 pt-1.5">
                              <span class="w-2 h-2 rounded-sm bg-gray-500 shrink-0"></span>
                              <span class="text-gray-300">{T && T.legendTotal}</span>
                              <span class="ml-auto font-semibold pl-4">{anime.total_episodes}</span>
                            </div>
                          {/if}
                        </div>
                        <div class="absolute top-full left-1/2 -translate-x-1/2 border-[5px] border-transparent border-t-gray-900 dark:border-t-gray-950"></div>
                      </div>
                    </div>
                    <span class="text-xs text-base-content/50">{_watched}/{_denom}</span>
                  </div>
                {/if}
                <p class="text-xs text-base-content/40 mt-1">
                  {formatTimeAgo(anime.last_download_date) || formatDate(anime.last_download_date)}
                </p>
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
