<script lang="ts">
  import { onMount } from 'svelte'
  import { getEpisodes, type Episode } from '../lib/api/client.js'
  import Loading from '../components/Loading.svelte'
  import ErrorMessage from '../components/ErrorMessage.svelte'

  let episodes: Episode[] = []
  let loading = true
  let error: string | null = null

  function extractAnimeName(episodeName) {
    if (!episodeName) return 'Unknown'
    
    // Remove common episode numbering patterns
    const patterns = [
      /\s*-\s*[Ee]pisode\s*\d+.*$/,
      /\s*-\s*[Ee]p\s*\d+.*$/,
      /\s*-\s*\d+.*$/,
      /\s+\d+.*$/,
      /\s*\(.*\)\s*$/
    ]
    
    let result = episodeName
    for (const pattern of patterns) {
      result = result.replace(pattern, '')
    }
    
    result = result.trim()
    return result || episodeName
  }

  function formatDate(dateString) {
    if (!dateString) return 'N/A'
    const date = new Date(dateString)
    return date.toLocaleString()
  }

  async function loadEpisodes() {
    try {
      loading = true
      error = null
      episodes = await getEpisodes()
      // Sort by episode ID descending (newest first)
          episodes.sort((a, b) => b.episode_id - a.episode_id)
        } catch (err) {
          error = err instanceof Error ? err.message : 'Unknown error'
          console.error('Failed to load episodes:', err)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    loadEpisodes()
  })
</script>

<div>
  <div class="mb-6">
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">Episodes</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">List of all downloaded episodes</p>
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
      <p class="text-sm text-gray-500 dark:text-gray-400">No episodes downloaded yet.</p>
    </div>
      {:else}
        <div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
          <!-- Desktop Table View -->
          <div class="hidden md:block overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead class="bg-gray-50 dark:bg-gray-700">
                <tr>
                  <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    Episode Name
                  </th>
                  <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    Anime
                  </th>
                  <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    Episode ID
                  </th>
                  <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    Download Date
                  </th>
                </tr>
              </thead>
              <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                {#each episodes as episode}
                  <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
                      {episode.episode_name || 'N/A'}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {extractAnimeName(episode.episode_name)}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {episode.episode_id}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {formatDate(episode.download_date)}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          
          <!-- Mobile Card View -->
          <div class="md:hidden divide-y divide-gray-200 dark:divide-gray-700">
            {#each episodes as episode}
              <div class="p-4 hover:bg-gray-50 dark:hover:bg-gray-700">
                <div class="mb-2">
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Episode Name</p>
                  <p class="text-sm font-medium text-gray-900 dark:text-white break-words">
                    {episode.episode_name || 'N/A'}
                  </p>
                </div>
                <div class="grid grid-cols-2 gap-4 mt-3">
                  <div>
                    <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Anime</p>
                    <p class="text-sm text-gray-900 dark:text-white break-words">
                      {extractAnimeName(episode.episode_name)}
                    </p>
                  </div>
                  <div>
                    <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Episode ID</p>
                    <p class="text-sm text-gray-900 dark:text-white">
                      {episode.episode_id}
                    </p>
                  </div>
                </div>
                <div class="mt-3">
                  <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">Download Date</p>
                  <p class="text-sm text-gray-900 dark:text-white">
                    {formatDate(episode.download_date)}
                  </p>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}
</div>
