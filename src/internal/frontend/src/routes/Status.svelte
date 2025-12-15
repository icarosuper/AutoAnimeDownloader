<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    getStatus,
    getAnimes,
    triggerCheck,
    startDaemon,
    stopDaemon,
    type StatusResponse,
    type AnimeInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import ErrorMessage from "../components/ErrorMessage.svelte";
  import Loading from "../components/Loading.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";

  let status: StatusResponse | null = null;
  let animes: AnimeInfo[] = [];
  let loading = true;
  let error: string | null = null;
  let actionLoading = false;
  let wsClient: WebSocketClient | null = null;
  let animesPollInterval: ReturnType<typeof setInterval> | null = null;

  async function loadAnimes() {
    try {
      const animesData = await getAnimes();
      animes = animesData.slice(0, 10);
    } catch (err) {
      console.error("Failed to load animes:", err);
    }
  }

  async function loadInitialData() {
    try {
      loading = true;
      error = null;
      const [statusData, animesData] = await Promise.all([
        getStatus(),
        getAnimes(),
      ]);
      status = statusData;
      animes = animesData.slice(0, 10);
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to load initial data:", err);
    } finally {
      loading = false;
    }
  }

  function handleWebSocketStatus(
    statusValue: string,
    lastCheck: string,
    hasError: boolean,
  ) {
    const previousStatus = status?.status;

    if (status) {
      status = {
        ...status,
        status: statusValue,
        last_check: lastCheck,
        has_error: hasError,
      };
    } else {
      status = {
        status: statusValue,
        last_check: lastCheck,
        has_error: hasError,
      };
    }

    // If status changed to "running", reload animes
    if (previousStatus !== "running" && statusValue === "running") {
      loadAnimes();
    }
  }

  async function handleStart() {
    try {
      actionLoading = true;
      await startDaemon();
      // Status will be updated via WebSocket
      await loadAnimes();
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
    } finally {
      actionLoading = false;
    }
  }

  async function handleStop() {
    try {
      actionLoading = true;
      await stopDaemon();
      // Status will be updated via WebSocket
      await loadAnimes();
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
    } finally {
      actionLoading = false;
    }
  }

  async function handleCheck() {
    try {
      actionLoading = true;
      await triggerCheck();
      // Status will be updated via WebSocket
      await loadAnimes();
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
    } finally {
      actionLoading = false;
    }
  }

  function formatDate(dateString: string) {
    if (!dateString) return "Never";
    const date = new Date(dateString);
    return date.toLocaleString();
  }

  onMount(() => {
    // Load initial data
    loadInitialData();

    // Connect WebSocket for real-time status updates
    wsClient = new WebSocketClient();
    wsClient.connect(handleWebSocketStatus);

    // Poll animes every 30 seconds (status is updated via WebSocket)
    animesPollInterval = setInterval(loadAnimes, 30000);
  });

  onDestroy(() => {
    if (wsClient) {
      wsClient.disconnect();
      wsClient = null;
    }
    if (animesPollInterval) {
      clearInterval(animesPollInterval);
    }
  });
</script>

<div>
  <div class="mb-6">
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">Status</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Daemon monitoring and statistics
    </p>
  </div>

  {#if error}
    <div class="mb-6">
      <ErrorMessage message={error} />
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading status..." />
  {:else if status}
    <!-- Status Card -->
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg mb-6">
      <div class="px-4 py-5 sm:p-6">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-2xl font-medium text-gray-900 dark:text-white">
            Daemon Status
          </h2>
          <StatusBadge status={status.status} />
        </div>

        <dl class="grid grid-cols-1 gap-5 sm:grid-cols-3 mt-5">
          <div class="px-4 py-5 bg-gray-50 dark:bg-gray-700 rounded-lg">
            <dt class="text-sm font-medium text-gray-500 dark:text-gray-400">
              Last Check
            </dt>
            <dd
              class="mt-1 text-lg font-semibold text-gray-900 dark:text-white"
            >
              {formatDate(status.last_check)}
            </dd>
          </div>
          <div class="px-4 py-5 bg-gray-50 dark:bg-gray-700 rounded-lg">
            <dt class="text-sm font-medium text-gray-500 dark:text-gray-400">
              Downloaded Animes
            </dt>
            <dd
              class="mt-1 text-lg font-semibold text-gray-900 dark:text-white"
            >
              {animes.length}
            </dd>
          </div>
          <div class="px-4 py-5 bg-gray-50 dark:bg-gray-700 rounded-lg">
            <dt class="text-sm font-medium text-gray-500 dark:text-gray-400">
              Total Episodes
            </dt>
            <dd
              class="mt-1 text-lg font-semibold text-gray-900 dark:text-white"
            >
              {animes.reduce((sum, a) => sum + a.episodes_count, 0)}
            </dd>
          </div>
        </dl>

        {#if status.has_error}
          <div
            class="mt-4 rounded-md bg-yellow-50 dark:bg-yellow-900/20 p-4 border border-yellow-200 dark:border-yellow-800"
          >
            <div class="flex">
              <div class="flex-shrink-0">
                <svg
                  class="h-5 w-5 text-yellow-400 dark:text-yellow-500"
                  viewBox="0 0 20 20"
                  fill="currentColor"
                >
                  <path
                    fill-rule="evenodd"
                    d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                    clip-rule="evenodd"
                  />
                </svg>
              </div>
              <div class="ml-3">
                <p
                  class="text-sm font-medium text-yellow-800 dark:text-yellow-200"
                >
                  Error detected in last verification
                </p>
              </div>
            </div>
          </div>
        {/if}

        <!-- Control Buttons -->
        <div class="mt-6 flex space-x-3">
          {#if status.status === "stopped"}
            <button
              type="button"
              on:click={handleStart}
              disabled={actionLoading}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {actionLoading ? "Starting..." : "Start Daemon"}
            </button>
          {:else}
            <button
              type="button"
              on:click={handleStop}
              disabled={actionLoading}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {actionLoading ? "Stopping..." : "Stop Daemon"}
            </button>
          {/if}
          <button
            type="button"
            on:click={handleCheck}
            disabled={status.status === "checking"}
            class="inline-flex items-center px-4 py-2 border border-gray-300 dark:border-gray-600 text-sm font-medium rounded-md text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-700 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {status.status === "checking" ? "Checking..." : "Check"}
          </button>
        </div>
      </div>
    </div>

    <!-- Latest Animes -->
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg">
      <div class="px-4 py-5 sm:p-6">
        <h2 class="text-lg font-medium text-gray-900 dark:text-white mb-4">
          Latest Downloaded Animes
        </h2>
        {#if animes.length === 0}
          <p class="text-sm text-gray-500 dark:text-gray-400">
            No animes downloaded yet.
          </p>
        {:else}
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
                    Name
                  </th>
                  <th
                    class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                  >
                    Episodes
                  </th>
                  <th
                    class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider"
                  >
                    Latest Episode ID
                  </th>
                </tr>
              </thead>
              <tbody
                class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700"
              >
                {#each animes as anime}
                  <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td
                      class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white"
                    >
                      {anime.name}
                    </td>
                    <td
                      class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400"
                    >
                      {anime.episodes_count}
                    </td>
                    <td
                      class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400"
                    >
                      {anime.latest_episode_id}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>

          <!-- Mobile Card View -->
          <div class="md:hidden space-y-4">
            {#each animes as anime}
              <div class="bg-gray-50 dark:bg-gray-700 rounded-lg p-4">
                <div class="flex items-center justify-between mb-2">
                  <h3
                    class="text-sm font-medium text-gray-900 dark:text-white truncate pr-2"
                  >
                    {anime.name}
                  </h3>
                </div>
                <div class="grid grid-cols-2 gap-4 mt-3">
                  <div>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      Episodes
                    </p>
                    <p
                      class="text-sm font-medium text-gray-900 dark:text-white"
                    >
                      {anime.episodes_count}
                    </p>
                  </div>
                  <div>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      Latest Episode ID
                    </p>
                    <p
                      class="text-sm font-medium text-gray-900 dark:text-white"
                    >
                      {anime.latest_episode_id}
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
