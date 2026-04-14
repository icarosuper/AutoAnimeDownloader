<script lang="ts">
  import {
    getAnimeDetail,
    getAnimes,
    downloadEpisode,
    deleteEpisode,
    releaseEpisode,
    redownloadEpisode,
    replaceEpisodeWithMagnet,
    replaceAnimeWithMagnet,
    updateAnimeSettings,
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
  let confirmRedownloadOpen = false;
  let pendingRedownloadEp: AnimeEpisodeInfo | null = null;

  // Bulk selection state
  let selectedEpisodes: Set<number> = new Set();
  let bulkLoading = false;
  let confirmBulkOpen = false;

  // Replace with magnet state
  let replaceEpOpen = false;
  let pendingReplaceEp: AnimeEpisodeInfo | null = null;
  let replaceEpMagnet = "";
  let replaceAnimeOpen = false;
  let replaceAnimeMagnet = "";
  let replaceLoading = false;

  // Custom search query state
  let customSearchQuery = "";
  let searchQuerySaving = false;

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
      customSearchQuery = detailData.custom_search_query ?? "";
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

  function handleRedownload(ep: AnimeEpisodeInfo) {
    pendingRedownloadEp = ep;
    confirmRedownloadOpen = true;
  }

  async function confirmRedownload() {
    if (!pendingRedownloadEp) return;
    const ep = pendingRedownloadEp;
    pendingRedownloadEp = null;
    actionLoading = { ...actionLoading, [ep.episode_id]: true };
    try {
      await redownloadEpisode(animeId, ep.episode_id);
      toast.success(m.detail_toast_redownload({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_redl_error());
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

  function handleReplace(ep: AnimeEpisodeInfo) {
    pendingReplaceEp = ep;
    replaceEpMagnet = "";
    replaceEpOpen = true;
  }

  async function confirmReplaceEp() {
    if (!pendingReplaceEp) return;
    if (!replaceEpMagnet.startsWith("magnet:")) {
      toast.error(m.detail_replace_invalid_magnet());
      return;
    }
    const ep = pendingReplaceEp;
    replaceLoading = true;
    try {
      await replaceEpisodeWithMagnet(animeId, ep.episode_id, replaceEpMagnet);
      toast.success(m.detail_replace_ep_done({ number: ep.episode_number }));
      replaceEpOpen = false;
      pendingReplaceEp = null;
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_replace_ep_error());
    } finally {
      replaceLoading = false;
    }
  }

  async function confirmReplaceAnime() {
    if (!replaceAnimeMagnet.startsWith("magnet:")) {
      toast.error(m.detail_replace_invalid_magnet());
      return;
    }
    replaceLoading = true;
    try {
      await replaceAnimeWithMagnet(animeId, replaceAnimeMagnet);
      toast.success(m.detail_replace_anime_done());
      replaceAnimeOpen = false;
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_replace_anime_error());
    } finally {
      replaceLoading = false;
    }
  }

  async function handleSaveSearchQuery() {
    searchQuerySaving = true;
    try {
      await updateAnimeSettings(animeId, { custom_search_query: customSearchQuery });
      toast.success(m.detail_search_query_saved());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_search_query_error());
    } finally {
      searchQuerySaving = false;
    }
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
  bind:open={confirmRedownloadOpen}
  title={m.detail_redownload_confirm_title()}
  message={pendingRedownloadEp ? m.detail_redownload_confirm_msg({ number: pendingRedownloadEp.episode_number }) : ""}
  confirmLabel={m.detail_redownload_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmRedownload}
/>

<ConfirmDialog
  bind:open={confirmBulkOpen}
  title={m.detail_bulk_confirm_title()}
  message={m.detail_bulk_confirm_msg({ count: selectedList.filter(ep => ep.is_downloaded).length })}
  confirmLabel={m.detail_bulk_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmBulkDelete}
/>

<!-- Replace Episode with Magnet Modal -->
{#if replaceEpOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" on:click|self={() => { replaceEpOpen = false; }}>
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">
        {m.detail_replace_ep_title({ number: pendingReplaceEp?.episode_number ?? "" })}
      </h3>
      <input
        type="text"
        bind:value={replaceEpMagnet}
        placeholder={m.detail_replace_magnet_placeholder()}
        class="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
        on:keydown={(e) => { if (e.key === 'Enter') confirmReplaceEp(); if (e.key === 'Escape') replaceEpOpen = false; }}
      />
      <div class="mt-4 flex justify-end gap-2">
        <button
          on:click={() => { replaceEpOpen = false; }}
          class="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
        >
          {m.common_cancel()}
        </button>
        <button
          on:click={confirmReplaceEp}
          disabled={replaceLoading}
          class="px-3 py-1.5 text-sm rounded border border-orange-500 text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {replaceLoading ? "..." : m.detail_btn_replace()}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Replace Anime with Magnet Modal -->
{#if replaceAnimeOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" on:click|self={() => { replaceAnimeOpen = false; }}>
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">
        {m.detail_replace_anime_title()}
      </h3>
      <input
        type="text"
        bind:value={replaceAnimeMagnet}
        placeholder={m.detail_replace_magnet_placeholder()}
        class="w-full px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
        on:keydown={(e) => { if (e.key === 'Enter') confirmReplaceAnime(); if (e.key === 'Escape') replaceAnimeOpen = false; }}
      />
      <div class="mt-4 flex justify-end gap-2">
        <button
          on:click={() => { replaceAnimeOpen = false; }}
          class="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
        >
          {m.common_cancel()}
        </button>
        <button
          on:click={confirmReplaceAnime}
          disabled={replaceLoading}
          class="px-3 py-1.5 text-sm rounded border border-orange-500 text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {replaceLoading ? "..." : m.detail_btn_replace()}
        </button>
      </div>
    </div>
  </div>
{/if}

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
    <div class="flex gap-4">
      {#if detail?.cover_image}
        <img
          src={detail.cover_image}
          alt={anime?.name ?? ""}
          class="w-24 h-36 sm:w-28 sm:h-40 object-cover rounded-lg shadow-md shrink-0"
        />
      {/if}
      <div class="flex-1 min-w-0">
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
          {anime ? anime.name : m.detail_title_fallback()}
        </h1>
        {#if detail}
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {m.detail_progress({ progress: detail.progress, total: detail.total_episodes || "?", status: detail.status })}
          </p>
          <div class="mt-2 flex items-center gap-2 flex-wrap">
            <button
              on:click={() => { replaceAnimeMagnet = ""; replaceAnimeOpen = true; }}
              class="inline-flex items-center px-3 py-1.5 text-xs font-medium rounded border border-orange-400 text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20"
            >
              {m.detail_replace_btn_anime()}
            </button>
          </div>
          <div class="mt-3 flex items-center gap-2 max-w-xl">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap shrink-0">
              {m.detail_search_query_label()}
            </label>
            <input
              type="text"
              bind:value={customSearchQuery}
              placeholder={m.detail_search_query_placeholder()}
              class="flex-1 min-w-0 px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              on:keydown={(e) => { if (e.key === 'Enter') handleSaveSearchQuery(); }}
            />
            <button
              on:click={handleSaveSearchQuery}
              disabled={searchQuerySaving}
              class="shrink-0 px-3 py-1 text-xs font-medium rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {searchQuerySaving ? "..." : "Save"}
            </button>
          </div>
        {/if}
      </div>
    </div>
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
              <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_episode()}
              </th>
              <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_anilist()}
              </th>
              <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_downloaded()}
              </th>
              <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_next_ep()}
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_download_date()}
              </th>
              <th class="px-6 py-3 text-center text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                {m.detail_col_actions()}
              </th>
            </tr>
          </thead>
          <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {#each detail.episodes as ep}
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
                <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white text-center">
                  {ep.episode_number}
                  {#if ep.is_manually_managed}
                    <span class="block text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_delete()}</span>
                  {/if}
                  {#if ep.is_blocked}
                    <span class="block text-xs text-gray-400 dark:text-gray-500">{m.detail_flag_no_download()}</span>
                  {/if}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-center">
                  {#if ep.is_watched}
                    <!-- Watched: eye icon -->
                    <svg class="w-4 h-4 text-blue-500 inline-block" aria-label={m.detail_badge_watched()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                      <circle cx="12" cy="12" r="3"/>
                    </svg>
                  {:else if ep.is_aired}
                    <!-- Aired but not watched: eye-off icon -->
                    <svg class="w-4 h-4 text-yellow-500 inline-block" aria-label={m.detail_badge_not_watched()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/>
                      <line x1="1" y1="1" x2="23" y2="23"/>
                    </svg>
                  {:else}
                    <!-- Upcoming: clock icon -->
                    <svg class="w-4 h-4 text-gray-400 inline-block" aria-label={m.detail_badge_upcoming()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <circle cx="12" cy="12" r="10"/>
                      <polyline points="12 6 12 12 16 14"/>
                    </svg>
                  {/if}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-center">
                  {#if ep.is_downloaded}
                    <!-- Downloaded: check circle -->
                    <svg class="w-4 h-4 text-green-500 inline-block" aria-label={m.detail_badge_downloaded()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
                      <polyline points="22 4 12 14.01 9 11.01"/>
                    </svg>
                  {:else}
                    <!-- Not downloaded: dash -->
                    <svg class="w-4 h-4 text-gray-300 dark:text-gray-600 inline-block" aria-label={m.detail_badge_not_downloaded()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <circle cx="12" cy="12" r="10"/>
                      <line x1="8" y1="12" x2="16" y2="12"/>
                    </svg>
                  {/if}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400 text-center">
                  {formatTimeUntilAiring(ep.time_until_airing, ep.is_aired)}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {ep.is_downloaded ? formatDate(ep.download_date) : "—"}
                </td>
                <td class="px-6 py-4 whitespace-nowrap">
                  <div class="flex items-center justify-center gap-1.5">
                    {#if ep.is_aired && !ep.is_downloaded}
                      <!-- Download -->
                      <button
                        on:click={() => handleDownload(ep)}
                        disabled={isLoading}
                        title={m.detail_btn_download()}
                        class="inline-flex items-center p-1.5 rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {#if isLoading}
                          <svg class="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10" stroke-opacity="0.25"/><path d="M12 2a10 10 0 0110 10" stroke-linecap="round"/></svg>
                        {:else}
                          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
                            <polyline points="7 10 12 15 17 10"/>
                            <line x1="12" y1="15" x2="12" y2="3"/>
                          </svg>
                        {/if}
                      </button>
                    {:else if ep.is_downloaded}
                      <!-- Delete -->
                      <button
                        on:click={() => handleDelete(ep)}
                        disabled={isLoading}
                        title={m.detail_btn_delete()}
                        class="inline-flex items-center p-1.5 rounded border border-red-500 text-red-600 dark:text-red-400 dark:border-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <polyline points="3 6 5 6 21 6"/>
                          <path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/>
                          <path d="M10 11v6M14 11v6"/>
                          <path d="M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2"/>
                        </svg>
                      </button>
                      <!-- Redownload -->
                      <button
                        on:click={() => handleRedownload(ep)}
                        disabled={isLoading}
                        title={m.detail_btn_redownload()}
                        class="inline-flex items-center p-1.5 rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <polyline points="23 4 23 10 17 10"/>
                          <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
                        </svg>
                      </button>
                    {/if}
                    {#if ep.is_aired}
                      <!-- Replace -->
                      <button
                        on:click={() => handleReplace(ep)}
                        disabled={isLoading}
                        title={m.detail_btn_replace()}
                        class="inline-flex items-center p-1.5 rounded border border-orange-400 text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <polyline points="17 1 21 5 17 9"/>
                          <path d="M3 11V9a4 4 0 014-4h14"/>
                          <polyline points="7 23 3 19 7 15"/>
                          <path d="M21 13v2a4 4 0 01-4 4H3"/>
                        </svg>
                      </button>
                    {/if}
                    {#if ep.is_manually_managed || ep.is_blocked}
                      <!-- Release -->
                      <button
                        on:click={() => handleRelease(ep)}
                        disabled={isLoading}
                        title="Soltar episódio"
                        class="inline-flex items-center p-1.5 rounded border border-gray-400 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
              <div class="flex items-center gap-2">
                {#if ep.is_watched}
                  <svg class="w-4 h-4 text-blue-500" aria-label={m.detail_badge_watched()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                    <circle cx="12" cy="12" r="3"/>
                  </svg>
                {:else if ep.is_aired}
                  <svg class="w-4 h-4 text-yellow-500" aria-label={m.detail_badge_not_watched()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/>
                    <line x1="1" y1="1" x2="23" y2="23"/>
                  </svg>
                {:else}
                  <svg class="w-4 h-4 text-gray-400" aria-label={m.detail_badge_upcoming()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10"/>
                    <polyline points="12 6 12 12 16 14"/>
                  </svg>
                {/if}
                {#if ep.is_downloaded}
                  <svg class="w-4 h-4 text-green-500" aria-label={m.detail_badge_downloaded()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
                    <polyline points="22 4 12 14.01 9 11.01"/>
                  </svg>
                {:else}
                  <svg class="w-4 h-4 text-gray-300 dark:text-gray-600" aria-label={m.detail_badge_not_downloaded()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10"/>
                    <line x1="8" y1="12" x2="16" y2="12"/>
                  </svg>
                {/if}
              </div>
            </div>
            {#if ep.episode_name}
              <p class="text-xs text-gray-400 dark:text-gray-500 mb-2 break-words">
                {ep.episode_name}
              </p>
            {/if}
            <div class="grid grid-cols-2 gap-4 mt-2">
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
                <!-- Download -->
                <button
                  on:click={() => handleDownload(ep)}
                  disabled={isLoading}
                  title={m.detail_btn_download()}
                  class="inline-flex items-center p-1.5 rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
                    <polyline points="7 10 12 15 17 10"/>
                    <line x1="12" y1="15" x2="12" y2="3"/>
                  </svg>
                </button>
              {:else if ep.is_downloaded}
                <!-- Delete -->
                <button
                  on:click={() => handleDelete(ep)}
                  disabled={isLoading}
                  title={m.detail_btn_delete()}
                  class="inline-flex items-center p-1.5 rounded border border-red-500 text-red-600 dark:text-red-400 dark:border-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/>
                    <path d="M10 11v6M14 11v6"/>
                    <path d="M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2"/>
                  </svg>
                </button>
                <!-- Redownload -->
                <button
                  on:click={() => handleRedownload(ep)}
                  disabled={isLoading}
                  title={m.detail_btn_redownload()}
                  class="inline-flex items-center p-1.5 rounded border border-blue-500 text-blue-600 dark:text-blue-400 dark:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="23 4 23 10 17 10"/>
                    <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
                  </svg>
                </button>
              {/if}
              {#if ep.is_aired}
                <!-- Replace -->
                <button
                  on:click={() => handleReplace(ep)}
                  disabled={isLoading}
                  title={m.detail_btn_replace()}
                  class="inline-flex items-center p-1.5 rounded border border-orange-400 text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="17 1 21 5 17 9"/>
                    <path d="M3 11V9a4 4 0 014-4h14"/>
                    <polyline points="7 23 3 19 7 15"/>
                    <path d="M21 13v2a4 4 0 01-4 4H3"/>
                  </svg>
                </button>
              {/if}
              {#if ep.is_manually_managed || ep.is_blocked}
                <!-- Release -->
                <button
                  on:click={() => handleRelease(ep)}
                  disabled={isLoading}
                  title="Soltar episódio"
                  class="inline-flex items-center p-1.5 rounded border border-gray-400 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
