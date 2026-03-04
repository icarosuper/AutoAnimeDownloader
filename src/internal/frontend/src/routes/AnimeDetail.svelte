<script lang="ts">
  import { onMount } from "svelte";
  import {
    getAnimeDetail,
    getAnimes,
    type AnimeDetailResponse,
    type AnimeEpisodeInfo,
    type AnimeInfo,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import ErrorMessage from "../components/ErrorMessage.svelte";

  export let params: { id?: string } = {};

  $: animeId = parseInt(params.id || "0");

  let anime: AnimeInfo | null = null;
  let detail: AnimeDetailResponse | null = null;
  let loading = true;
  let error: string | null = null;

  function formatDate(dateString: string | undefined) {
    if (!dateString) return "N/A";
    return new Date(dateString).toLocaleString();
  }

  function formatAiringAt(unixSeconds: number): string {
    if (!unixSeconds) return "N/A";
    return new Date(unixSeconds * 1000).toLocaleString();
  }

  function getAniListBadge(ep: AnimeEpisodeInfo): { label: string; classes: string } {
    if (ep.is_watched) {
      return { label: "Watched", classes: "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300" };
    }
    if (ep.is_aired) {
      return { label: "Not Watched", classes: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300" };
    }
    return { label: "Upcoming", classes: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400" };
  }

  function getDownloadBadge(ep: AnimeEpisodeInfo): { label: string; classes: string } {
    if (ep.is_downloaded) {
      return { label: "Downloaded", classes: "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300" };
    }
    return { label: "Not Downloaded", classes: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400" };
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
      error = "Invalid anime ID";
      loading = false;
      return;
    }

    try {
      loading = true;
      error = null;

      const [detailData, animesData] = await Promise.all([
        getAnimeDetail(id),
        getAnimes(),
      ]);

      detail = detailData;
      anime = animesData.find((a) => a.anime_id === id) ?? null;
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to load anime detail:", err);
    } finally {
      loading = false;
    }
  }

  $: loadData(animeId);

  onMount(() => {
    loadData(animeId);
  });
</script>

<div>
  <div class="mb-6">
    <a
      href="#/status"
      class="inline-flex items-center text-sm text-blue-600 dark:text-blue-400 hover:underline mb-3"
    >
      <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
      Back to Status
    </a>
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
      {anime ? anime.name : "Anime Detail"}
    </h1>
    {#if detail}
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Progress: {detail.progress} / {detail.total_episodes || "?"} episodes &middot; {detail.status}
      </p>
    {/if}
  </div>

  {#if error}
    <div class="mb-6">
      <ErrorMessage message={error} />
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading episodes..." />
  {:else if !detail || detail.episodes.length === 0}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">No episodes found for this anime.</p>
    </div>
  {:else}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
      <!-- Desktop Table View -->
      <div class="hidden md:block overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead class="bg-gray-50 dark:bg-gray-700">
            <tr>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Episode
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                AniList
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Downloaded
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Air Date
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Next Episode In
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Download Date
              </th>
            </tr>
          </thead>
          <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {#each detail.episodes as ep}
              {@const anilist = getAniListBadge(ep)}
              {@const download = getDownloadBadge(ep)}
              <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
                  {ep.episode_number}
                  {#if ep.episode_name}
                    <span class="block text-xs text-gray-400 dark:text-gray-500 font-normal truncate max-w-xs">
                      {ep.episode_name}
                    </span>
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
          <div class="p-4 hover:bg-gray-50 dark:hover:bg-gray-700">
            <div class="flex items-start justify-between mb-2">
              <p class="text-sm font-medium text-gray-900 dark:text-white">
                Episode {ep.episode_number}
              </p>
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
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Air Date</p>
                <p class="text-sm text-gray-900 dark:text-white">{formatAiringAt(ep.airing_at)}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Next Episode In</p>
                <p class="text-sm text-gray-900 dark:text-white">{formatTimeUntilAiring(ep.time_until_airing, ep.is_aired)}</p>
              </div>
              {#if ep.is_downloaded}
                <div>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Download Date</p>
                  <p class="text-sm text-gray-900 dark:text-white">{formatDate(ep.download_date)}</p>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>
