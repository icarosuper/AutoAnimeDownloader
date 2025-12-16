<script lang="ts">
  import { onMount } from "svelte";
  import {
    getConfig,
    updateConfig,
    triggerCheck,
    type Config,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import ErrorMessage from "../components/ErrorMessage.svelte";
  import Input from "../components/Input.svelte";

  let config: Config = {
    anilist_username: "",
    save_path: "",
    completed_anime_path: "",
    check_interval: 10,
    qbittorrent_url: "http://127.0.0.1:8080",
    max_episodes_per_anime: 12,
    episode_retry_limit: 5,
    delete_watched_episodes: true,
    excluded_list: "",
  };

  let loading = true;
  let saving = false;
  let error: string | null = null;
  let success = false;
  let showMissingConfigBanner = false;

  function checkQueryParams() {
    if (typeof window !== "undefined") {
      // Verificar query params na URL (pode estar no search ou no hash)
      const search = window.location.search;
      const hash = window.location.hash;

      // Tentar pegar do search primeiro (query params antes do hash)
      if (search) {
        const urlParams = new URLSearchParams(search);
        showMissingConfigBanner = urlParams.has("missingConfig");
        if (showMissingConfigBanner) {
          console.log(
            "Missing config banner will be shown (from search params)",
          );
        }
        return;
      }

      // Se não tiver no search, verificar no hash (para hash routing)
      if (hash) {
        const hashParts = hash.split("?");
        if (hashParts.length > 1) {
          const urlParams = new URLSearchParams(hashParts[1]);
          showMissingConfigBanner = urlParams.has("missingConfig");
          if (showMissingConfigBanner) {
            console.log(
              "Missing config banner will be shown (from hash params)",
            );
          }
          return;
        }
      }
    }
  }

  async function loadConfig() {
    try {
      loading = true;
      error = null;
      const data = await getConfig();
      config = { ...data };
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to load config:", err);
    } finally {
      loading = false;
    }
  }

  async function saveConfig() {
    try {
      saving = true;
      error = null;
      success = false;

      // Validation
      if (!config.anilist_username || config.anilist_username.trim() === "") {
        throw new Error("Anilist username is required");
      }
      if (!config.save_path || config.save_path.trim() === "") {
        throw new Error("Save path is required");
      }
      if (!config.qbittorrent_url || config.qbittorrent_url.trim() === "") {
        throw new Error("qBittorrent URL is required");
      }
      if (config.check_interval <= 0) {
        throw new Error("Check interval must be greater than 0");
      }
      if (config.max_episodes_per_anime <= 0) {
        throw new Error("Max episodes per anime must be greater than 0");
      }
      if (config.episode_retry_limit < 0) {
        throw new Error("Episode retry limit must be non-negative");
      }

      await updateConfig(config);
      success = true;
    } catch (err) {
      error = err instanceof Error ? err.message : "Unknown error";
      console.error("Failed to save config:", err);
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    checkQueryParams();
    loadConfig();
  });
</script>

<div>
  {#if showMissingConfigBanner}
    <div
      class="mb-6 rounded-md bg-yellow-50 dark:bg-yellow-900/20 p-4 border border-yellow-200 dark:border-yellow-800"
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
          <p class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            Existem configurações faltantes, por favor insira-as para continuar
          </p>
        </div>
      </div>
    </div>
  {/if}

  <div class="mb-6">
    <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
      Configuration
    </h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Configure daemon behavior
    </p>
  </div>

  {#if error}
    <div class="mb-6">
      <ErrorMessage message={error} />
    </div>
  {/if}

  {#if success}
    <div class="mb-6 rounded-md bg-white dark:bg-gray-800 p-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <svg
              class="h-5 w-5 text-green-400"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fill-rule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                clip-rule="evenodd"
              />
            </svg>
          </div>
          <div class="ml-3">
            <p class="text-sm font-medium text-green-800 dark:text-green-200">
              Configuration saved successfully!
            </p>
          </div>
        </div>
        <button
          type="button"
          class="inline-flex items-center px-3 py-1.5 border border-transparent text-sm leading-4 font-semibold rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
          on:click={() => {
            triggerCheck().then(() => {
              window.location.href = "/status";
            });
          }}
          title="Run anime check"
        >
          <svg
            class="h-4 w-4 mr-1 -ml-1"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M13 5l7 7-7 7M5 5v14"
            />
          </svg>
          Run anime check now
        </button>
      </div>
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading configuration..." />
  {:else}
    <div class="bg-white dark:bg-gray-800 shadow rounded-lg">
      <form
        on:submit|preventDefault={saveConfig}
        class="px-4 py-5 sm:p-6 space-y-6"
      >
        <!-- Anilist Username -->
        <Input
          id="anilist_username"
          label="Anilist Username"
          type="text"
          bind:value={config.anilist_username}
          required={true}
        />

        <!-- Save Path -->
        <Input
          id="save_path"
          label="Save Path"
          type="text"
          bind:value={config.save_path}
          placeholder="/path/to/downloads"
          required={true}
        />

        <!-- Completed Anime Path -->
        <Input
          id="completed_anime_path"
          label="Completed Anime Path"
          type="text"
          bind:value={config.completed_anime_path}
          placeholder="/path/to/completed"
        />

        <!-- Check Interval -->
        <Input
          id="check_interval"
          label="Check Interval (minutes)"
          type="number"
          bind:value={config.check_interval}
          min="1"
          required={true}
        />

        <!-- qBittorrent URL -->
        <Input
          id="qbittorrent_url"
          label="qBittorrent URL"
          type="url"
          bind:value={config.qbittorrent_url}
          placeholder="http://127.0.0.1:8080"
          required={true}
        />

        <!-- Max Episodes Per Anime -->
        <Input
          id="max_episodes_per_anime"
          label="Max Episodes Per Anime"
          type="number"
          bind:value={config.max_episodes_per_anime}
          min="1"
          required={true}
        />

        <!-- Episode Retry Limit -->
        <Input
          id="episode_retry_limit"
          label="Episode Retry Limit"
          type="number"
          bind:value={config.episode_retry_limit}
          min="0"
          required={true}
        />

        <!-- Delete Watched Episodes -->
        <div class="flex items-center">
          <input
            type="checkbox"
            id="delete_watched_episodes"
            bind:checked={config.delete_watched_episodes}
            class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 dark:border-gray-600 rounded"
          />
          <label
            for="delete_watched_episodes"
            class="ml-2 block text-sm text-gray-900 dark:text-white"
          >
            Delete Watched Episodes
          </label>
        </div>

        <!-- Excluded List -->
        <Input
          id="excluded_list"
          label="Excluded List"
          type="text"
          bind:value={config.excluded_list}
          placeholder="One anime name per line"
        />

        <!-- Buttons -->
        <div
          class="flex justify-end space-x-3 pt-4 border-t border-gray-200 dark:border-gray-700"
        >
          <button
            type="button"
            on:click={loadConfig}
            disabled={loading || saving}
            class="inline-flex items-center px-4 py-2 border border-gray-300 dark:border-gray-600 shadow-sm text-sm font-medium rounded-md text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-700 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Reload
          </button>
          <button
            type="submit"
            disabled={saving}
            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </form>
    </div>
  {/if}
</div>
