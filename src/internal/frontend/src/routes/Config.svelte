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
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.config_title(),
    subtitle: m.config_subtitle(),
    missingBanner: m.config_missing_banner(),
    loading: m.config_loading(),
    sectionAnilist: m.config_section_anilist(),
    sectionDownloads: m.config_section_downloads(),
    sectionAutomation: m.config_section_automation(),
    sectionQbit: m.config_section_qbittorrent(),
    sectionFilters: m.config_section_filters(),
    labelUsername: m.config_label_username(),
    labelSavePath: m.config_label_save_path(),
    hintSavePath: m.config_hint_save_path(),
    labelUseCompletedPath: m.config_label_use_completed_path(),
    labelCompletedPath: m.config_label_completed_path(),
    hintCompletedPath: m.config_hint_completed_path(),
    labelDeleteWatched: m.config_label_delete_watched(),
    labelWatchedKeep: m.config_label_watched_keep(),
    hintWatchedKeep: m.config_hint_watched_keep(),
    labelCheckInterval: m.config_label_check_interval(),
    labelMaxEpisodes: m.config_label_max_episodes(),
    labelRetryLimit: m.config_label_retry_limit(),
    labelQbitUrl: m.config_label_qbit_url(),
    hintQbitUrl: m.config_hint_qbit_url(),
    labelRenameJellyfin: m.config_label_rename_jellyfin(),
    hintRenameJellyfin: m.config_hint_rename_jellyfin(),
    labelExcludedList: m.config_label_excluded_list(),
    hintExcludedList: m.config_hint_excluded_list(),
    btnRunCheck: m.config_btn_run_check(),
    btnReload: m.config_btn_reload(),
    btnSave: m.config_btn_save(),
    btnSaving: m.config_btn_saving(),
  }

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
    rename_files_for_jellyfin: false,
  };

  let loading = true;
  let saving = false;
  let showMissingConfigBanner = false;
  let useCompletedPath = false;

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
      useCompletedPath = !!config.completed_anime_path;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.config_error_load());
    } finally {
      loading = false;
    }
  }

  async function saveConfig() {
    try {
      saving = true;

      if (!config.anilist_username?.trim()) throw new Error(m.config_val_username());
      if (!config.save_path?.trim()) throw new Error(m.config_val_save_path());
      if (!config.qbittorrent_url?.trim()) throw new Error(m.config_val_qbit_url());
      if (config.check_interval <= 0) throw new Error(m.config_val_interval());
      if (config.max_episodes_per_anime <= 0) throw new Error(m.config_val_max_episodes());
      if (config.episode_retry_limit < 0) throw new Error(m.config_val_retry());
      if (config.delete_watched_episodes && config.watched_episodes_to_keep < 0)
        throw new Error(m.config_val_watched_keep());

      await updateConfig(config);
      toast.success(m.config_saved());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.config_error_save());
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
    <h1 class="text-2xl font-semibold text-base-content">{T && T.title}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{T && T.subtitle}</p>
  </div>

  {#if showMissingConfigBanner}
    <div role="alert" class="alert alert-warning">
      <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
      </svg>
      <span class="text-sm">{T && T.missingBanner}</span>
    </div>
  {/if}

  {#if loading}
    <Loading message={T && T.loading || ""} />
  {:else}
    <form on:submit|preventDefault={saveConfig} class="space-y-4">

      <!-- Anilist -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{T && T.sectionAnilist}</h2>
          <Input
            id="anilist_username"
            label={T && T.labelUsername || ""}
            type="text"
            bind:value={config.anilist_username}
            required={true}
          />
        </div>
      </div>

      <!-- Downloads -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{T && T.sectionDownloads}</h2>
          <Input
            id="save_path"
            label={T && T.labelSavePath || ""}
            subtitle={T && T.hintSavePath || ""}
            type="text"
            bind:value={config.save_path}
            placeholder="/path/to/downloads"
            required={true}
          />
          <div class="space-y-3">
            <div class="flex items-center gap-2">
              <input
                type="checkbox"
                id="use_completed_path"
                bind:checked={useCompletedPath}
                on:change={() => { if (!useCompletedPath) config.completed_anime_path = ""; }}
                class="checkbox checkbox-sm"
              />
              <label for="use_completed_path" class="text-sm text-base-content cursor-pointer">
                {T && T.labelUseCompletedPath}
              </label>
            </div>
            {#if useCompletedPath}
              <div class="pl-6">
                <Input
                  id="completed_anime_path"
                  label={T && T.labelCompletedPath || ""}
                  subtitle={T && T.hintCompletedPath || ""}
                  type="text"
                  bind:value={config.completed_anime_path}
                  placeholder="/path/to/completed"
                />
              </div>
            {/if}
          </div>
          <div class="space-y-3">
            <div class="flex items-center gap-2">
              <input
                type="checkbox"
                id="delete_watched_episodes"
                bind:checked={config.delete_watched_episodes}
                class="checkbox checkbox-sm"
              />
              <label for="delete_watched_episodes" class="text-sm text-base-content cursor-pointer">
                {T && T.labelDeleteWatched}
              </label>
            </div>
            {#if config.delete_watched_episodes}
              <div class="pl-6">
                <Input
                  id="watched_episodes_to_keep"
                  label={T && T.labelWatchedKeep || ""}
                  subtitle={T && T.hintWatchedKeep || ""}
                  type="number"
                  bind:value={config.watched_episodes_to_keep}
                  min="0"
                />
              </div>
            {/if}
          </div>
          <div class="flex flex-col gap-1">
            <div class="flex items-center gap-2">
              <input
                type="checkbox"
                id="rename_files_for_jellyfin"
                bind:checked={config.rename_files_for_jellyfin}
                class="checkbox checkbox-sm"
              />
              <label for="rename_files_for_jellyfin" class="text-sm text-base-content cursor-pointer">
                {T && T.labelRenameJellyfin}
              </label>
            </div>
            {#if config.rename_files_for_jellyfin}
              <p class="text-xs text-base-content/50 pl-6">{T && T.hintRenameJellyfin}</p>
            {/if}
          </div>
        </div>
      </div>

      <!-- Automation -->
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body p-5 gap-4">
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{T && T.sectionAutomation}</h2>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Input
              id="check_interval"
              label={T && T.labelCheckInterval || ""}
              type="number"
              bind:value={config.check_interval}
              min="1"
              required={true}
            />
            <Input
              id="max_episodes_per_anime"
              label={T && T.labelMaxEpisodes || ""}
              type="number"
              bind:value={config.max_episodes_per_anime}
              min="1"
              required={true}
            />
            <Input
              id="episode_retry_limit"
              label={T && T.labelRetryLimit || ""}
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
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{T && T.sectionQbit}</h2>
          <Input
            id="qbittorrent_url"
            label={T && T.labelQbitUrl || ""}
            subtitle={T && T.hintQbitUrl || ""}
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
          <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{T && T.sectionFilters}</h2>
          <Input
            id="excluded_list"
            label={T && T.labelExcludedList || ""}
            subtitle={T && T.hintExcludedList || ""}
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
          {T && T.btnRunCheck}
        </button>
        <button
          type="button"
          on:click={loadConfig}
          disabled={loading || saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {T && T.btnReload}
        </button>
        <button
          type="submit"
          disabled={saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {saving ? (T && T.btnSaving) : (T && T.btnSave)}
        </button>
      </div>
    </form>
  {/if}
</div>
