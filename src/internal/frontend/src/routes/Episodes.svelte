<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { getEpisodes, type Episode } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import Loading from "../components/Loading.svelte";
  import ErrorMessage from "../components/ErrorMessage.svelte";

  let episodes: Episode[] = [];
  let loading = true;
  let error: string | null = null;
  let searchQuery = "";
  let wsClient: WebSocketClient | null = null;
  let currentStatus: string = "";

  function formatDate(dateString: string) {
    if (!dateString) return "N/A";
    const date = new Date(dateString);
    return date.toLocaleString();
  }

  function extractAnimeName(episodeName: string) {
    if (!episodeName) return "Unknown";

    // Remove common episode numbering patterns
    const patterns = [
      /\s*-\s*[Ee]pisode\s*\d+.*$/,
      /\s*-\s*[Ee]p\s*\d+.*$/,
      /\s*-\s*\d+.*$/,
      /\s+\d+.*$/,
      /\s*\(.*\)\s*$/,
    ];

    let result = episodeName;
    for (const pattern of patterns) {
      result = result.replace(pattern, "");
    }

    result = result.trim();
    return result || episodeName;
  }

  async function loadEpisodes() {
    try {
      loading = true;
      error = null;
      episodes = await getEpisodes();
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to load episodes:", err);
    } finally {
      loading = false;
    }
  }

  function filterEpisodes(episodes: Episode[], query: string): Episode[] {
    if (!query.trim()) {
      return episodes;
    }

    const lowerQuery = query.toLowerCase().trim();

    return episodes.filter((episode) => {
      // Search in episode name
      const episodeName = (episode.episode_name || "").toLowerCase();
      if (episodeName.includes(lowerQuery)) return true;

      // Search in anime name (extracted)
      const animeName = extractAnimeName(episode.episode_name).toLowerCase();
      if (animeName.includes(lowerQuery)) return true;

      // Search in episode ID (as string)
      const episodeId = episode.episode_id.toString();
      if (episodeId.includes(lowerQuery)) return true;

      // Search in episode hash
      const episodeHash = (episode.episode_hash || "").toLowerCase();
      if (episodeHash.includes(lowerQuery)) return true;

      // Search in download date (formatted)
      const downloadDate = formatDate(episode.download_date).toLowerCase();
      if (downloadDate.includes(lowerQuery)) return true;

      return false;
    });
  }

  $: filteredEpisodes = filterEpisodes(episodes, searchQuery);

  function handleWebSocketStatus(statusValue: string) {
    const previousStatus = currentStatus;
    currentStatus = statusValue;

    // If status changed to "running", reload episodes
    if (previousStatus !== "running" && statusValue === "running") {
      loadEpisodes();
    }
  }

  onMount(() => {
    loadEpisodes();

    // Connect WebSocket for real-time status updates
    wsClient = new WebSocketClient();
    wsClient.connect(handleWebSocketStatus);
  });

  onDestroy(() => {
    if (wsClient) {
      wsClient.disconnect();
      wsClient = null;
    }
  });
</script>

<div>
  <div class="mb-6">
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">Episodes</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      List of all downloaded episodes
    </p>
  </div>

  {#if error}
    <div class="mb-6">
      <ErrorMessage message={error} />
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading episodes..." />
  {:else if episodes.length === 0}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        No episodes downloaded yet.
      </p>
    </div>
  {:else}
    <!-- Search Bar -->
    <div class="mb-6">
      <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-4">
        <label for="episode-search" class="sr-only">Search episodes</label>
        <div class="relative">
          <div
            class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none"
          >
            <svg
              class="h-5 w-5 text-gray-400 dark:text-gray-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
          </div>
          <input
            id="episode-search"
            type="text"
            placeholder="Search by episode name, anime, ID, hash, or date..."
            bind:value={searchQuery}
            class="block w-full pl-10 pr-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md leading-5 bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-500 dark:placeholder-gray-400 focus:outline-none focus:placeholder-gray-400 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 sm:text-sm"
          />
        </div>
        {#if searchQuery && filteredEpisodes.length !== episodes.length}
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            Showing {filteredEpisodes.length} of {episodes.length} episodes
          </p>
        {/if}
      </div>
    </div>

    {#if filteredEpisodes.length === 0}
      <div class="bg-white dark:bg-gray-800 shadow rounded-lg p-6">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          No episodes found matching your search.
        </p>
      </div>
    {:else}
      <div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
        <!-- Desktop Table View -->
        <div class="hidden md:block overflow-x-auto">
          <table
            class="min-w-full divide-y divide-gray-200 dark:divide-gray-700"
          >
            <thead class="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th
                  class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                >
                  Episode Name
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                >
                  Anime
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                >
                  Episode ID
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                >
                  Download Date
                </th>
              </tr>
            </thead>
            <tbody
              class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700"
            >
              {#each filteredEpisodes as episode}
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                  <td
                    class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white"
                  >
                    {episode.episode_name || "N/A"}
                  </td>
                  <td
                    class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400"
                  >
                    {extractAnimeName(episode.episode_name)}
                  </td>
                  <td
                    class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400"
                  >
                    {episode.episode_id}
                  </td>
                  <td
                    class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400"
                  >
                    {formatDate(episode.download_date)}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>

        <!-- Mobile Card View -->
        <div class="md:hidden divide-y divide-gray-200 dark:divide-gray-700">
          {#each filteredEpisodes as episode}
            <div class="p-4 hover:bg-gray-50 dark:hover:bg-gray-700">
              <div class="mb-2">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">
                  Episode Name
                </p>
                <p
                  class="text-sm font-medium text-gray-900 dark:text-white break-words"
                >
                  {episode.episode_name || "N/A"}
                </p>
              </div>
              <div class="grid grid-cols-2 gap-4 mt-3">
                <div>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">
                    Anime
                  </p>
                  <p class="text-sm text-gray-900 dark:text-white break-words">
                    {extractAnimeName(episode.episode_name)}
                  </p>
                </div>
                <div>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">
                    Episode ID
                  </p>
                  <p class="text-sm text-gray-900 dark:text-white">
                    {episode.episode_id}
                  </p>
                </div>
              </div>
              <div class="mt-3">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">
                  Download Date
                </p>
                <p class="text-sm text-gray-900 dark:text-white">
                  {formatDate(episode.download_date)}
                </p>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  {/if}
</div>
