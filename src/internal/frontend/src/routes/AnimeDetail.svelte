<script lang="ts">
  import {
    getAnimeDetail,
    getAnimes,
    downloadEpisode,
    deleteEpisode,
    releaseEpisode,
    type AnimeDetailResponse,
    type AnimeEpisodeInfo,
    type AnimeInfo,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";

  export let params: { id?: string } = {};

  $: animeId = parseInt(params.id || "0");

  let anime: AnimeInfo | null = null;
  let detail: AnimeDetailResponse | null = null;
  let loading = true;
  let actionLoading: Record<number, boolean> = {};
  let confirmOpen = false;
  let pendingDeleteEp: AnimeEpisodeInfo | null = null;

  // Bulk selection state
  let selectedEpisodes: Set<number> = new Set();
  let bulkLoading = false;
  let confirmBulkOpen = false;

  $: allEpisodes = detail?.episodes ?? [];
  $: allSelected = allEpisodes.length > 0 && allEpisodes.every(ep => selectedEpisodes.has(ep.episode_id));
  $: someSelected = selectedEpisodes.size > 0 && !allSelected;

  $: selectedList = allEpisodes.filter(ep => selectedEpisodes.has(ep.episode_id));
  $: canBulkDownload = selectedList.some(ep => ep.is_aired && !ep.is_downloaded);
  $: canBulkDelete = selectedList.some(ep => ep.is_downloaded);
  $: canBulkRelease = selectedList.some(ep => ep.is_manually_managed || ep.is_blocked);

  function toggleSelectAll() {
    if (allSelected) {
      selectedEpisodes = new Set();
    } else {
      selectedEpisodes = new Set(allEpisodes.map(ep => ep.episode_id));
    }
  }

  function toggleEpisode(id: number) {
    const next = new Set(selectedEpisodes);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selectedEpisodes = next;
  }

  function formatDate(dateString: string | undefined) {
    if (!dateString) return m.common_na();
    return new Date(dateString).toLocaleString();
  }

  function formatAiringAt(unixSeconds: number): string {
    if (!unixSeconds) return m.common_na();
    return new Date(unixSeconds * 1000).toLocaleString();
  }

  function getAniListBadge(ep: AnimeEpisodeInfo): { label: string; classes: string } {
    if (ep.is_watched) {
      return { label: m.detail_badge_watched(), classes: "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300" };
    }
    if (ep.is_aired) {
      return { label: m.detail_badge_not_watched(), classes: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300" };
    }
    return { label: m.detail_badge_upcoming(), classes: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400" };
  }

  function getDownloadBadge(ep: AnimeEpisodeInfo): { label: string; classes: string } {
    if (ep.is_downloaded) {
      return { label: m.detail_badge_downloaded(), classes: "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300" };
    }
    return { label: m.detail_badge_not_downloaded(), classes: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400" };
  }

  function formatTimeUntilAiring(seconds: number, isAired: boolean): string {
    if (isAired || seconds <= 0) return "Released";
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  }

  async function loadData(id: number) {
    if (!id || id <= 0) {
      loading = false;
      return;
    }

    try {
      loading = true;

      const [detailData, animesData] = await Promise.all([
        getAnimeDetail(id),
        getAnimes(),
      ]);

      detail = detailData;
      anime = animesData.find((a) => a.anime_id === id) ?? null;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_load_error());
    } finally {
      loading = false;
    }
  }

  async function handleDownload(ep: AnimeEpisodeInfo) {
    actionLoading = { ...actionLoading, [ep.episode_id]: true };
    try {
      await downloadEpisode(animeId, ep.episode_id);
      toast.success(m.detail_toast_queued({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_dl_error());
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_id]: false };
    }
  }

  function handleDelete(ep: AnimeEpisodeInfo) {
    pendingDeleteEp = ep;
    confirmOpen = true;
  }

  async function confirmDelete() {
    if (!pendingDeleteEp) return;
    const ep = pendingDeleteEp;
    pendingDeleteEp = null;
    actionLoading = { ...actionLoading, [ep.episode_id]: true };
    try {
      await deleteEpisode(animeId, ep.episode_id);
      toast.success(m.detail_toast_deleted({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_del_error());
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_id]: false };
    }
  }

  async function handleRelease(ep: AnimeEpisodeInfo) {
    actionLoading = { ...actionLoading, [ep.episode_id]: true };
    try {
      await releaseEpisode(animeId, ep.episode_id);
      await loadData(animeId);
    } catch (err) {
      console.error("Failed to release episode:", err);
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_id]: false };
    }
  }

  async function handleBulkDownload() {
    const targets = selectedList.filter(ep => ep.is_aired && !ep.is_downloaded);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await downloadEpisode(animeId, ep.episode_id);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_dl_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  function handleBulkDelete() {
    confirmBulkOpen = true;
  }

  async function confirmBulkDelete() {
    const targets = selectedList.filter(ep => ep.is_downloaded);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await deleteEpisode(animeId, ep.episode_id);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_del_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  async function handleBulkRelease() {
    const targets = selectedList.filter(ep => ep.is_manually_managed || ep.is_blocked);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await releaseEpisode(animeId, ep.episode_id);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_rel_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  $: loadData(animeId);
</script>

<ConfirmDialog
  bind:open={confirmOpen}
  title={m.detail_confirm_title()}
  message={pendingDeleteEp ? m.detail_confirm_msg({ number: pendingDeleteEp.episode_number }) : ""}
  confirmLabel={m.detail_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmDelete}
/>

<ConfirmDialog
  bind:open={confirmBulkOpen}
  title={m.detail_bulk_confirm_title()}
  message={m.detail_bulk_confirm_msg({ count: selectedList.filter(ep => ep.is_downloaded).length })}
  confirmLabel={m.detail_bulk_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmBulkDelete}
/>

<div>
  <div class="mb-6">
    <a
      href="#/status"
      class="inline-flex items-center text-sm text-blue-600 dark:text-blue-400 hover:underline mb-3"
    >
      <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
      {m.detail_back()}
    </a>
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
      {anime ? anime.name : m.detail_title_fallback()}
    </h1>
    {#if detail}
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {m.detail_progress({ progress: detail.progress, total: detail.total_episodes || "?", status: detail.status })}
      </p>
    {/if}
  </div>

  {#if loading}
    <Loading message={m.detail_loading()} />
  {:else if !detail || detail.episodes.length === 0}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.detail_empty()}</p>
    </div>
  {:else}
    <!-- Bulk action bar -->
    {#if selectedEpisodes.size > 0}
      <div class="mb-3 flex items-center gap-2 flex-wrap bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg px-4 py-2.5">
        <span class="text-sm font-medium text-blue-700 dark:text-blue-300 mr-1">
          {m.detail_bulk_selected({ count: selectedEpisodes.size })}
        </span>
        {#if canBulkDownload}
          <button
            on:click={handleBulkDownload}
            disabled={bulkLoading}
            class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/40 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {m.detail_bulk_download()}
          </button>
        {/if}
        {#if canBulkDelete}
          <button
            on:click={handleBulkDelete}
            disabled={bulkLoading}
            class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-red-500 text-red-600 dark:text-red-400 dark:border-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {m.detail_bulk_delete()}
          </button>
        {/if}
        {#if canBulkRelease}
          <button
            on:click={handleBulkRelease}
            disabled={bulkLoading}
            class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-gray-400 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {m.detail_bulk_release()}
          </button>
        {/if}
        <button
          on:click={() => { selectedEpisodes = new Set(); }}
          class="ml-auto inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-gray-300 dark:border-gray-600 text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50"
        >
          {m.detail_bulk_deselect_all()}
        </button>
      </div>
    {/if}

    <div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
      <!-- Desktop Table View -->
      <div class="hidden md:block overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead class="bg-gray-50 dark:bg-gray-700">
            <tr>
              <th class="px-4 py-3 w-10">
                <input
                  type="checkbox"
                  checked={allSelected}
                  indeterminate={someSelected}
                  on:change={toggleSelectAll}
                  class="w-4 h-4 rounded border-gray-300 dark:border-gray-600 text-blue-600 cursor-pointer"
                />
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_episode()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_anilist()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_downloaded()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_air_date()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_next_ep()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_download_date()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_actions()}
              </th>
            </tr>
          </thead>
          <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {#each detail.episodes as ep}
              {@const anilist = getAniListBadge(ep)}
              {@const download = getDownloadBadge(ep)}
              {@const isLoading = !!actionLoading[ep.episode_id]}
              {@const isSelected = selectedEpisodes.has(ep.episode_id)}
              <tr class="hover:bg-gray-50 dark:hover:bg-gray-700 {isSelected ? 'bg-blue-50 dark:bg-blue-900/10' : ''}">
                <td class="px-4 py-4 w-10">
                  <input
                    type="checkbox"
                    checked={isSelected}
                    on:change={() => toggleEpisode(ep.episode_id)}
                    class="w-4 h-4 rounded border-gray-300 dark:border-gray-600 text-blue-600 cursor-pointer"
                  />
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
                  {ep.episode_number}
                  {#if ep.is_manually_managed}
                    <span class="block text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_delete()}</span>
                  {/if}
                  {#if ep.is_blocked}
                    <span class="block text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_download()}</span>
                  {/if}
                </td>
                <td class="px-6 py-4 whitespace-nowrap">
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {anilist.classes}">
                    {anilist.label}
                  </span>
                </td>
                <td class="px-6 py-4 whitespace-nowrap">
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {download.classes}">
                    {download.label}
                  </span>
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {formatAiringAt(ep.airing_at)}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {formatTimeUntilAiring(ep.time_until_airing, ep.is_aired)}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {ep.is_downloaded ? formatDate(ep.download_date) : "—"}
                </td>
                <td class="px-6 py-4 whitespace-nowrap">
                  <div class="flex items-center gap-1.5">
                    {#if ep.is_aired && !ep.is_downloaded}
                      <button
                        on:click={() => handleDownload(ep)}
                        disabled={isLoading}
                        class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {isLoading ? "..." : m.detail_btn_download()}
                      </button>
                    {:else if ep.is_downloaded}
                      <button
                        on:click={() => handleDelete(ep)}
                        disabled={isLoading}
                        class="inline-flex items-center px-2.5 py-1 text-xs font-medium rounded border border-red-500 text-red-600 dark:text-red-400 dark:border-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {isLoading ? "..." : m.detail_btn_delete()}
                      </button>
                    {/if}
                    {#if ep.is_manually_managed || ep.is_blocked}
                      <button
                        on:click={() => handleRelease(ep)}
                        disabled={isLoading}
                        title="Soltar episódio"
                        class="inline-flex items-center px-1.5 py-1 text-xs font-medium rounded border border-gray-400 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <line x1="18" y1="6" x2="6" y2="18"></line>
                          <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Mobile Card View -->
      <div class="md:hidden divide-y divide-gray-200 dark:divide-gray-700">
        {#each detail.episodes as ep}
          {@const anilist = getAniListBadge(ep)}
          {@const download = getDownloadBadge(ep)}
          {@const isLoading = !!actionLoading[ep.episode_id]}
          {@const isSelected = selectedEpisodes.has(ep.episode_id)}
          <div class="p-4 hover:bg-gray-50 dark:hover:bg-gray-700 {isSelected ? 'bg-blue-50 dark:bg-blue-900/10' : ''}">
            <div class="flex items-start justify-between mb-2">
              <div class="flex items-start gap-3">
                <input
                  type="checkbox"
                  checked={isSelected}
                  on:change={() => toggleEpisode(ep.episode_id)}
                  class="mt-0.5 w-4 h-4 rounded border-gray-300 dark:border-gray-600 text-blue-600 cursor-pointer flex-shrink-0"
                />
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {m.detail_col_episode()} {ep.episode_number}
                  </p>
                  {#if ep.is_manually_managed}
                    <p class="text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_delete_short()}</p>
                  {/if}
                  {#if ep.is_blocked}
                    <p class="text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_download_short()}</p>
                  {/if}
                </div>
              </div>
              <div class="flex flex-col items-end gap-1">
                <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {anilist.classes}">
                  {anilist.label}
                </span>
                <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {download.classes}">
                  {download.label}
                </span>
              </div>
            </div>
            {#if ep.episode_name}
              <p class="text-xs text-gray-400 dark:text-gray-500 mb-2 break-words">
                {ep.episode_name}
              </p>
            {/if}
            <div class="grid grid-cols-2 gap-4 mt-2">
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">{m.detail_col_air_date()}</p>
                <p class="text-sm text-gray-900 dark:text-white">{formatAiringAt(ep.airing_at)}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">{m.detail_col_next_ep()}</p>
                <p class="text-sm text-gray-900 dark:text-white">{formatTimeUntilAiring(ep.time_until_airing, ep.is_aired)}</p>
              </div>
              {#if ep.is_downloaded}
                <div>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">{m.detail_col_download_date()}</p>
                  <p class="text-sm text-gray-900 dark:text-white">{formatDate(ep.download_date)}</p>
                </div>
              {/if}
            </div>
            <div class="mt-3 flex items-center gap-1.5">
              {#if ep.is_aired && !ep.is_downloaded}
                <button
                  on:click={() => handleDownload(ep)}
                  disabled={isLoading}
                  class="inline-flex items-center px-3 py-1.5 text-xs font-medium rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isLoading ? m.detail_btn_downloading() : m.detail_btn_download()}
                </button>
              {:else if ep.is_downloaded}
                <button
                  on:click={() => handleDelete(ep)}
                  disabled={isLoading}
                  class="inline-flex items-center px-3 py-1.5 text-xs font-medium rounded border border-red-500 text-red-600 dark:text-red-400 dark:border-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isLoading ? m.detail_btn_deleting() : m.detail_btn_delete()}
                </button>
              {/if}
              {#if ep.is_manually_managed || ep.is_blocked}
                <button
                  on:click={() => handleRelease(ep)}
                  disabled={isLoading}
                  title="Soltar episódio"
                  class="inline-flex items-center px-1.5 py-1.5 text-xs font-medium rounded border border-gray-400 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <line x1="18" y1="6" x2="6" y2="18"></line>
                    <line x1="6" y1="6" x2="18" y2="18"></line>
                  </svg>
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>
