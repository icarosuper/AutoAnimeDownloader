<script lang="ts">
  import { onMount } from "svelte";
  import {
    getConfig,
    updateConfig,
    triggerCheck,
    type Config,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import Input from "../components/Input.svelte";
  import { toast } from "../lib/stores/toast.js";

  let config: Config = {
    anilist_username: "",
    save_path: "",
    completed_anime_path: "",
    check_interval: 10,
    qbittorrent_url: "http://127.0.0.1:8080",
    max_episodes_per_anime: 12,
    episode_retry_limit: 5,
    delete_watched_episodes: true,
    watched_episodes_to_keep: 0,
    excluded_list: "",
  };

  let loading = true;
  let saving = false;
  let showMissingConfigBanner = false;

  function checkQueryParams() {
    if (typeof window === "undefined") return;
    const search = window.location.search;
    const hash = window.location.hash;
    if (search) {
      showMissingConfigBanner = new URLSearchParams(search).has("missingConfig");
      return;
    }
    const hashParts = hash.split("?");
    if (hashParts.length > 1) {
      showMissingConfigBanner = new URLSearchParams(hashParts[1]).has("missingConfig");
    }
  }

  async function loadConfig() {
    try {
      loading = true;
      const data = await getConfig();
      config = { ...data };
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load configuration");
    } finally {
      loading = false;
    }
  }

  async function saveConfig() {
    try {
      saving = true;

      if (!config.anilist_username?.trim()) throw new Error("Anilist username is required");
      if (!config.save_path?.trim()) throw new Error("Save path is required");
      if (!config.qbittorrent_url?.trim()) throw new Error("qBittorrent URL is required");
      if (config.check_interval <= 0) throw new Error("Check interval must be greater than 0");
      if (config.max_episodes_per_anime <= 0) throw new Error("Max episodes per anime must be greater than 0");
      if (config.episode_retry_limit < 0) throw new Error("Episode retry limit must be non-negative");
      if (config.delete_watched_episodes && config.watched_episodes_to_keep < 0)
        throw new Error("Watched episodes to keep must be non-negative");

      await updateConfig(config);
      toast.success("Configuration saved successfully");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save configuration");
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    checkQueryParams();
    loadConfig();
  });
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-base-content">Configuration</h1>
    <p class="text-sm text-base-content/50 mt-0.5">Configure daemon behavior</p>
  </div>

  {#if showMissingConfigBanner}
    <div role="alert" class="alert alert-warning">
      <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
      </svg>
      <span class="text-sm">There are missing configurations — please fill them in to continue.</span>
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading configuration..." />
  {:else}
    <form on:submit|preventDefault={saveConfig} class="space-y-4">

      <!-- Anilist -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">Anilist</h2>
          <Input
            id="anilist_username"
            label="Username"
            type="text"
            bind:value={config.anilist_username}
            required={true}
          />
        </div>
      </div>

      <!-- Downloads -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">Downloads</h2>
          <Input
            id="save_path"
            label="Save Path"
            subtitle="Where releasing anime episodes will be saved"
            type="text"
            bind:value={config.save_path}
            placeholder="/path/to/downloads"
            required={true}
          />
          <Input
            id="completed_anime_path"
            label="Completed Anime Path"
            subtitle="If empty, uses the save path above"
            type="text"
            bind:value={config.completed_anime_path}
            placeholder="/path/to/completed"
          />
          <div class="space-y-3">
            <div class="flex items-center gap-2">
              <input
                type="checkbox"
                id="delete_watched_episodes"
                bind:checked={config.delete_watched_episodes}
                class="checkbox checkbox-sm"
              />
              <label for="delete_watched_episodes" class="text-sm text-base-content cursor-pointer">
                Delete watched episodes automatically
              </label>
            </div>
            {#if config.delete_watched_episodes}
              <div class="pl-6">
                <Input
                  id="watched_episodes_to_keep"
                  label="Watched Episodes to Keep"
                  subtitle="Set to 0 to delete all watched episodes"
                  type="number"
                  bind:value={config.watched_episodes_to_keep}
                  min="0"
                />
              </div>
            {/if}
          </div>
        </div>
      </div>

      <!-- Automation -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">Automation</h2>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Input
              id="check_interval"
              label="Check Interval (minutes)"
              type="number"
              bind:value={config.check_interval}
              min="1"
              required={true}
            />
            <Input
              id="max_episodes_per_anime"
              label="Max Episodes per Anime"
              type="number"
              bind:value={config.max_episodes_per_anime}
              min="1"
              required={true}
            />
            <Input
              id="episode_retry_limit"
              label="Episode Retry Limit"
              type="number"
              bind:value={config.episode_retry_limit}
              min="0"
              required={true}
            />
          </div>
        </div>
      </div>

      <!-- qBittorrent -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">qBittorrent</h2>
          <Input
            id="qbittorrent_url"
            label="WebUI URL"
            subtitle="Default: http://127.0.0.1:8080"
            type="url"
            bind:value={config.qbittorrent_url}
            placeholder="http://127.0.0.1:8080"
            required={true}
          />
        </div>
      </div>

      <!-- Filters -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">Filters</h2>
          <Input
            id="excluded_list"
            label="Excluded List"
            subtitle="Lists that should not be downloaded"
            type="text"
            bind:value={config.excluded_list}
            placeholder="Name of excluded list"
          />
        </div>
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 pt-2">
        <button
          type="button"
          on:click={async () => {
            await triggerCheck();
            window.location.hash = "#/status";
          }}
          disabled={saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Run Check Now
        </button>
        <button
          type="button"
          on:click={loadConfig}
          disabled={loading || saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Reload
        </button>
        <button
          type="submit"
          disabled={saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
    </form>
  {/if}
</div>
