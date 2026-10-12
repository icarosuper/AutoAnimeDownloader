<script lang="ts">
  // Config — spec §9.4 (Phase 6). Pure re-skin: NO behavior change (decision D5). The Save button
  // is still the only write path — no autosave, no debounce — because `PUT /config` validates
  // everything at once and does filesystem I/O (`Librarian.ProbePath`), so saving mid-typing would
  // produce a 400 on every keystroke.
  //
  // The new layout is the 196px side index with ONE group visible at a time. The list fields
  // (anilist_usernames, mal_usernames, excluded_lists) swapped the "input + button +" pair for
  // artboard 1e's ChipsInput.
  import { onMount } from "svelte";
  import { ArrowUpRight, Check } from "@lucide/svelte";
  import {
    getConfig,
    updateConfig,
    triggerCheck,
    type Config,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import Input from "../components/Input.svelte";
  import Button from "../components/ui/Button.svelte";
  import ChipsInput from "../components/ui/ChipsInput.svelte";
  import Toggle from "../components/ui/Toggle.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import {
    ONBOARDING_STEP_IDS,
    onboardingDismissed,
    onboardingDone,
  } from "../lib/stores/onboarding.js";

  type GroupId = "library" | "anilist" | "downloads" | "search";

  $: T = $locale && {
    title: m.config_title(),
    subtitle: m.config_subtitle(),
    missingBanner: m.config_missing_banner(),
    loading: m.config_loading(),
    navLabel: m.config_nav_label(),
    requiredLegend: m.config_required_legend(),
    groupMissing: m.config_group_missing(),
    chipsPlaceholder: m.config_chips_placeholder(),
    linkPriorities: m.nav_priorities(),
    linkNotifications: m.nav_notifications(),
    labelUsername: m.config_label_username(),
    hintAnilistUsernames: m.config_hint_anilist_usernames(),
    labelMalUsernames: m.config_label_mal_usernames(),
    hintMalUsernames: m.config_hint_mal_usernames(),
    labelCompletedPath: m.config_label_completed_path(),
    hintCompletedPath: m.config_hint_completed_path(),
    labelDeleteWatched: m.config_label_delete_watched(),
    labelWatchedKeep: m.config_label_watched_keep(),
    hintWatchedKeep: m.config_hint_watched_keep(),
    labelCheckInterval: m.config_label_check_interval(),
    labelMaxEpisodes: m.config_label_max_episodes(),
    hintMaxEpisodes: m.config_hint_max_episodes(),
    labelRetryLimit: m.config_label_retry_limit(),
    labelMaxBatchSize: m.config_label_max_batch_size(),
    hintMaxBatchSize: m.config_hint_max_batch_size(),
    labelMaxEpisodeSize: m.config_label_max_episode_size(),
    hintMaxEpisodeSize: m.config_hint_max_episode_size(),
    labelMinSeeders: m.config_label_min_seeders(),
    hintMinSeeders: m.config_hint_min_seeders(),
    labelMaxSearchPages: m.config_label_max_search_pages(),
    hintMaxSearchPages: m.config_hint_max_search_pages(),
    labelMinFreeDisk: m.config_label_min_free_disk(),
    hintMinFreeDisk: m.config_hint_min_free_disk(),
    labelMaxConcurrent: m.config_label_max_concurrent(),
    hintMaxConcurrent: m.config_hint_max_concurrent(),
    labelRenameJellyfin: m.config_label_rename_jellyfin(),
    hintRenameJellyfin: m.config_hint_rename_jellyfin(),
    labelExcludedList: m.config_label_excluded_list(),
    hintExcludedList: m.config_hint_excluded_list(),
    labelDownloadStatuses: m.config_label_download_statuses(),
    hintDownloadStatuses: m.config_hint_download_statuses(),
    labelDownloadMediaStatuses: m.config_label_download_media_statuses(),
    hintDownloadMediaStatuses: m.config_hint_download_media_statuses(),
    labelDeleteStatuses: m.config_label_delete_statuses(),
    hintDeleteStatuses: m.config_hint_delete_statuses(),
    statusLabels: {
      CURRENT:   m.config_status_current(),
      REPEATING: m.config_status_repeating(),
      PLANNING:  m.config_status_planning(),
      PAUSED:    m.config_status_paused(),
      DROPPED:   m.config_status_dropped(),
      COMPLETED: m.config_status_completed(),
      RELEASING: m.config_status_releasing(),
      FINISHED:  m.config_status_finished(),
      CANCELLED: m.config_status_cancelled(),
      HIATUS:    m.config_status_hiatus(),
    } as Record<string, string>,
    onboardingRestoreLabel: m.onboarding_restore_label(),
    onboardingRestoreHint: m.onboarding_restore_hint(),
    onboardingRestoreButton: m.onboarding_restore_button(),
    btnRunCheck: m.config_btn_run_check(),
    btnSave: m.config_btn_save(),
    btnSaving: m.config_btn_saving(),
  }

  /**
   * Index order. Library first because it hosts the screen's only required field — and it's where
   * `#/config?missingConfig=true` sends the user.
   *
   * `advanced` draws the divider above the item: "Torrent search" is fine-tuning of torrent
   * selection, not something a new install needs to touch.
   */
  $: groups = ($locale && [
    { id: "library" as GroupId, label: m.config_section_library(), advanced: false },
    { id: "anilist" as GroupId, label: m.config_section_anilist(), advanced: false },
    { id: "downloads" as GroupId, label: m.config_section_downloads(), advanced: false },
    { id: "search" as GroupId, label: m.config_section_search(), advanced: true },
  ]) || [];

  let activeGroup: GroupId = "library";

  const ALL_STATUSES = ["CURRENT", "REPEATING", "PLANNING", "PAUSED", "DROPPED", "COMPLETED"];
  const ALL_MEDIA_STATUSES = ["RELEASING", "FINISHED", "CANCELLED", "HIATUS"];

  let config: Config = {
    anilist_usernames: [],
    mal_usernames: [],
    completed_anime_path: "",
    check_interval: 10,
    max_episodes_per_anime: 12,
    max_batch_torrent_size_gb: 0,
    max_episode_torrent_size_gb: 0,
    min_seeders: 1,
    max_search_pages: 5,
    min_free_disk_percent: 10,
    episode_retry_limit: 5,
    max_concurrent_downloads: 3,
    delete_watched_episodes: true,
    watched_episodes_to_keep: 0,
    excluded_lists: [],
    rename_files_for_jellyfin: false,
    download_statuses: ["CURRENT", "REPEATING"],
    download_media_statuses: ["RELEASING", "FINISHED"],
    delete_statuses: [],
    notifications: { webhooks: [], batch_window_seconds: 0 },
    priorities: {
      criteria_order: [],
      fansubs: [],
      resolutions: [],
      sources: [],
      codecs: [],
      audio: [],
      ignore_list: [],
    },
  };

  // A status can't be in "download" and "delete" at the same time — turning one on always turns the
  // other off. Pre-existing rule, kept verbatim.
  function toggleDownloadStatus(status: string) {
    const active = (config.download_statuses ?? []).includes(status);
    if (active) {
      config.download_statuses = (config.download_statuses ?? []).filter(s => s !== status);
    } else {
      config.download_statuses = [...(config.download_statuses ?? []), status];
      config.delete_statuses = (config.delete_statuses ?? []).filter(s => s !== status);
    }
  }

  function toggleDownloadMediaStatus(status: string) {
    const active = (config.download_media_statuses ?? []).includes(status);
    if (active) {
      config.download_media_statuses = (config.download_media_statuses ?? []).filter(s => s !== status);
    } else {
      config.download_media_statuses = [...(config.download_media_statuses ?? []), status];
    }
  }

  function toggleDeleteStatus(status: string) {
    const active = (config.delete_statuses ?? []).includes(status);
    if (active) {
      config.delete_statuses = (config.delete_statuses ?? []).filter(s => s !== status);
    } else {
      config.delete_statuses = [...(config.delete_statuses ?? []), status];
      config.download_statuses = (config.download_statuses ?? []).filter(s => s !== status);
    }
  }

  function statusPillClass(active: boolean, variant: "accent" | "danger"): string {
    if (!active) return "border-default bg-control text-subtle hover:text-body";
    return variant === "danger"
      ? "border-danger-tint/28 bg-danger-tint/12 text-danger"
      : "border-accent-tint/28 bg-accent-tint/12 text-accent";
  }

  let loading = true;
  let saving = false;
  let showMissingConfigBanner = false;

  /**
   * The app is a hash SPA, so the query can be in `?a=b#/config` (rare) or after the `?` INSIDE the
   * hash (`#/config?a=b`, the normal path). The params are resolved ONCE and every parameter comes
   * from there: with one `return` per branch, each new parameter would have to be read in both
   * places, and the two could diverge.
   */
  function resolveParams(): URLSearchParams | null {
    if (typeof window === "undefined") return null;
    if (window.location.search) return new URLSearchParams(window.location.search);
    const hashParts = window.location.hash.split("?");
    return hashParts.length > 1 ? new URLSearchParams(hashParts[1]) : null;
  }

  // The card also disappears when the user marks the three steps by hand, so bringing it back means
  // clearing both things — only resetting the dismissal would leave the button with no visible
  // effect.
  $: onboardingHidden =
    $onboardingDismissed || ONBOARDING_STEP_IDS.every((id) => $onboardingDone.includes(id));

  function restoreOnboarding() {
    onboardingDismissed.set(false);
    onboardingDone.reset();
  }

  function checkQueryParams() {
    const params = resolveParams();
    if (!params) return;

    showMissingConfigBanner = params.has("missingConfig");

    // Validation uses the `groups` array the screen already builds — a second list of ids would
    // drift from it. An unknown value is ignored and `activeGroup` stays on the default "library":
    // without the guard, an old or mistyped link would leave the screen on a group that renders
    // nothing.
    //
    // No conflict with validation: `firstValidationError()` only touches `activeGroup` on the Save
    // click, and this runs in `onMount`.
    const group = params.get("group");
    if (group && groups.some((g) => g.id === group)) activeGroup = group as GroupId;
  }

  async function loadConfig() {
    try {
      loading = true;
      const data = await getConfig();
      config = { ...data, anilist_usernames: data.anilist_usernames ?? [], mal_usernames: data.mal_usernames ?? [] };
      if (config.anilist_username && (config.anilist_usernames ?? []).length === 0) {
        config.anilist_usernames = [config.anilist_username];
      }
      if (!config.notifications) config.notifications = { webhooks: [], batch_window_seconds: 0 };
      if (!Array.isArray(config.notifications.webhooks)) config.notifications.webhooks = [];
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.config_error_load());
    } finally {
      loading = false;
    }
  }

  /**
   * The same six validations as before, in the same order — only now each one knows which group the
   * field it rejects lives in. That's a requirement of the new layout, not a new rule: with one
   * group visible at a time, a toast saying "folder is required" while the user looks at
   * "Automation" couldn't be acted on. The failing validation brings its group to the screen.
   *
   * They became a LIST (it used to be a chain of `if`) because the screen now uses them for two
   * things: the Save toast, as always, and the "needs filling" mark in the side index. Rewriting
   * the conditions in the second place would make the mark lie the first time a rule changed — the
   * index has to say exactly what blocks Save, nothing more.
   *
   * `message` holds the REFERENCE to the paraglide function, not the text: that way the message is
   * resolved in the language active at click time, as it was when each `if` called it.
   */
  $: requiredChecks = [
    // A list account (AniList or MyAnimeList) is NOT required: with standalone animes (#/add) the
    // app works entirely without any list, and the backend stopped validating it. A requirement
    // only in the frontend would be a rule the server doesn't know.
    {
      group: "library" as GroupId,
      ok: !!config.completed_anime_path?.trim(),
      message: m.config_val_completed_path,
    },
    {
      group: "downloads" as GroupId,
      ok: config.check_interval > 0,
      message: m.config_val_interval,
    },
    {
      group: "downloads" as GroupId,
      ok: config.max_episodes_per_anime >= 0,
      message: m.config_val_max_episodes,
    },
    {
      group: "search" as GroupId,
      ok: config.episode_retry_limit >= 0,
      message: m.config_val_retry,
    },
    {
      group: "downloads" as GroupId,
      ok: config.max_concurrent_downloads >= 0,
      message: m.config_val_max_concurrent,
    },
    {
      group: "downloads" as GroupId,
      ok: !(config.delete_watched_episodes && config.watched_episodes_to_keep < 0),
      message: m.config_val_watched_keep,
    },
    {
      group: "search" as GroupId,
      ok: config.max_batch_torrent_size_gb >= 0 && config.max_episode_torrent_size_gb >= 0,
      message: m.config_val_torrent_size,
    },
    {
      group: "search" as GroupId,
      ok: config.min_seeders >= 0,
      message: m.config_val_min_seeders,
    },
    {
      group: "search" as GroupId,
      ok: config.max_search_pages >= 0,
      message: m.config_val_max_search_pages,
    },
    {
      // 100 would block every download forever.
      group: "library" as GroupId,
      ok: config.min_free_disk_percent >= 0 && config.min_free_disk_percent <= 99,
      message: m.config_val_min_free_disk,
    },
  ];

  /** Groups with something pending — feeds the dot in the side index. */
  $: pendingGroups = new Set(requiredChecks.filter((c) => !c.ok).map((c) => c.group));

  function firstValidationError(): { message: string; group: GroupId } | null {
    const failed = requiredChecks.find((c) => !c.ok);
    return failed ? { message: failed.message(), group: failed.group } : null;
  }

  async function saveConfig() {
    try {
      saving = true;

      const invalid = firstValidationError();
      if (invalid) {
        activeGroup = invalid.group;
        throw new Error(invalid.message);
      }

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

<div class="space-y-4.5">
  <div>
    <h1 class="text-screen-title text-heading">{T && T.title}</h1>
    <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
  </div>

  {#if showMissingConfigBanner}
    <div
      role="alert"
      class="flex items-center gap-2 rounded-field border border-warn-tint/28 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
    >
      {T && T.missingBanner}
    </div>
  {/if}

  {#if loading}
    <Loading message={T && T.loading || ""} />
  {:else}
    <form on:submit|preventDefault={saveConfig} class="space-y-4">
      <!-- Asterisk legend. Without it the `*` is an implicit convention; with one group at a time,
           it's worth saying in text what the mark means. -->
      <p class="text-caption text-subtle">
        <span class="text-danger" aria-hidden="true">*</span>
        {T && T.requiredLegend}
      </p>

      <div class="grid gap-3.5 md:grid-cols-[196px_1fr] md:items-start">
        <!-- Index: a 196px column on desktop; on mobile the items WRAP, they don't scroll. Same
             decision (and same reason) as the filter pills of `DownloadsToolbar.svelte`: at 390px
             the strip had 358px of space for 644px of content, so half the options — including
             "Torrent search" and the two exit links — only showed up after dragging. `shrink-0`
             keeps the label whole instead of squeezing it.

             The dividers are `w-full`, so the wrap happens AT THEM: the three rows that come out of
             it are the screen's architecture — everyday groups, the advanced group, and what leaves
             the screen. On desktop the column says this implicitly; here it's explicit.

             The pending dot goes HERE because only one group is on screen at a time: the asterisk
             on the field covers the open group, and this dot is what tells the user something is
             still missing in the other three. The sr-only text goes into the button's accessible
             name ("Lists, required configuration missing"); the dot itself is decorative. -->
        <nav
          aria-label={(T && T.navLabel) || ""}
          class="flex flex-wrap gap-1 md:flex-col"
        >
          {#each groups as group (group.id)}
            {#if group.advanced}
              <!-- `w-full`: besides separating, it's what pushes "Torrent search" to the next row
                   on mobile. One axis at both widths. -->
              <div class="my-1 w-full border-t border-divider md:my-1.5" aria-hidden="true"></div>
            {/if}
            <button
              type="button"
              aria-current={activeGroup === group.id ? "true" : undefined}
              on:click={() => (activeGroup = group.id)}
              class="flex shrink-0 items-center gap-1.5 rounded-field px-3 py-2 text-left text-copy transition-colors md:w-full {activeGroup ===
              group.id
                ? 'bg-accent-tint/16 font-bold text-nav-active'
                : 'font-semibold text-subtle hover:text-body'}"
            >
              {group.label}
              {#if pendingGroups.has(group.id)}
                <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
                <span class="sr-only">{T && T.groupMissing}</span>
              {/if}
            </button>
          {/each}

          <!-- Divider: what comes below LEAVES the screen instead of switching the visible group. -->
          <div class="my-1 w-full border-t border-divider md:my-1.5" aria-hidden="true"></div>

          <a
            href="#/priorities"
            class="flex shrink-0 items-center gap-1.5 rounded-field px-3 py-2 text-left text-copy font-semibold text-subtle transition-colors hover:text-body md:w-full"
          >
            {T && T.linkPriorities}
            <ArrowUpRight size={14} strokeWidth={2.5} class="shrink-0 opacity-70" aria-hidden="true" />
          </a>
          <a
            href="#/notifications"
            class="flex shrink-0 items-center gap-1.5 rounded-field px-3 py-2 text-left text-copy font-semibold text-subtle transition-colors hover:text-body md:w-full"
          >
            {T && T.linkNotifications}
            <ArrowUpRight size={14} strokeWidth={2.5} class="shrink-0 opacity-70" aria-hidden="true" />
          </a>
        </nav>

        <!-- `divide-y` draws the 1px dividers BETWEEN the fields, with no leftover border on the
             first or the last row of the card. -->
        <div class="divide-y divide-divider rounded-card border border-default bg-card">
          {#if activeGroup === "anilist"}
            <div class="p-4.5">
              <ChipsInput
                id="anilist_usernames"
                bind:values={config.anilist_usernames}
                label={(T && T.labelUsername) || ""}
                hint={(T && T.hintAnilistUsernames) || ""}
                placeholder={(T && T.chipsPlaceholder) || ""}
                removeLabel={(item) => m.config_chips_remove({ item })}
              />
            </div>

            <div class="p-4.5">
              <ChipsInput
                id="mal_usernames"
                bind:values={config.mal_usernames}
                label={(T && T.labelMalUsernames) || ""}
                hint={(T && T.hintMalUsernames) || ""}
                placeholder={(T && T.chipsPlaceholder) || ""}
                removeLabel={(item) => m.config_chips_remove({ item })}
              />
            </div>

            <div class="p-4.5">
              <ChipsInput
                id="excluded_lists"
                bind:values={config.excluded_lists}
                label={(T && T.labelExcludedList) || ""}
                hint={(T && T.hintExcludedList) || ""}
                placeholder={(T && T.chipsPlaceholder) || ""}
                removeLabel={(item) => m.config_chips_remove({ item })}
              />
            </div>

            <fieldset class="p-4.5">
              <legend class="float-left w-full text-[14.5px] font-bold text-heading">{T && T.labelDownloadStatuses}</legend>
              <p class="text-caption text-subtle">{T && T.hintDownloadStatuses}</p>
              <div class="mt-2.5 flex flex-wrap gap-2">
                {#each ALL_STATUSES as status}
                  {@const active = (config.download_statuses ?? []).includes(status)}
                  <button
                    type="button"
                    aria-pressed={active}
                    on:click={() => toggleDownloadStatus(status)}
                    title={status}
                    class="inline-flex items-center gap-1 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {statusPillClass(
                      active,
                      'accent'
                    )}"
                  >
                    {#if active}<Check size={13} strokeWidth={3} />{/if}
                    {T ? T.statusLabels[status] : status}
                  </button>
                {/each}
              </div>
            </fieldset>

            <fieldset class="p-4.5">
              <legend class="float-left w-full text-[14.5px] font-bold text-heading">{T && T.labelDownloadMediaStatuses}</legend>
              <p class="text-caption text-subtle">{T && T.hintDownloadMediaStatuses}</p>
              <div class="mt-2.5 flex flex-wrap gap-2">
                {#each ALL_MEDIA_STATUSES as status}
                  {@const active = (config.download_media_statuses ?? []).includes(status)}
                  <button
                    type="button"
                    aria-pressed={active}
                    on:click={() => toggleDownloadMediaStatus(status)}
                    title={status}
                    class="inline-flex items-center gap-1 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {statusPillClass(
                      active,
                      'accent'
                    )}"
                  >
                    {#if active}<Check size={13} strokeWidth={3} />{/if}
                    {T ? T.statusLabels[status] : status}
                  </button>
                {/each}
              </div>
            </fieldset>

            <fieldset class="p-4.5">
              <legend class="float-left w-full text-[14.5px] font-bold text-heading">{T && T.labelDeleteStatuses}</legend>
              <p class="text-caption text-subtle">{T && T.hintDeleteStatuses}</p>
              <div class="mt-2.5 flex flex-wrap gap-2">
                {#each ALL_STATUSES as status}
                  {@const active = (config.delete_statuses ?? []).includes(status)}
                  <button
                    type="button"
                    aria-pressed={active}
                    on:click={() => toggleDeleteStatus(status)}
                    title={status}
                    class="inline-flex items-center gap-1 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {statusPillClass(
                      active,
                      'danger'
                    )}"
                  >
                    {#if active}<Check size={13} strokeWidth={3} />{/if}
                    {T ? T.statusLabels[status] : status}
                  </button>
                {/each}
              </div>
            </fieldset>
          {/if}

          {#if activeGroup === "library"}
            <div class="p-4.5">
              <Input
                id="completed_anime_path"
                label={T && T.labelCompletedPath || ""}
                subtitle={T && T.hintCompletedPath || ""}
                type="text"
                bind:value={config.completed_anime_path}
                placeholder="/path/to/completed"
                required={true}
              />
            </div>

            <!-- The hint always shows: it's precisely what helps decide whether to turn the switch
                 on, so hiding it behind the on state hides the useful information. `Toggle` has no
                 hint prop, hence the sibling <p>. -->
            <div class="space-y-1.5 p-4.5">
              <Toggle
                id="rename_files_for_jellyfin"
                bind:checked={config.rename_files_for_jellyfin}
                label={(T && T.labelRenameJellyfin) || ""}
                inline={true}
              />
              <p class="text-caption text-subtle">{T && T.hintRenameJellyfin}</p>
            </div>

            <div class="p-4.5">
              <Input
                id="min_free_disk_percent"
                label={T && T.labelMinFreeDisk || ""}
                subtitle={T && T.hintMinFreeDisk || ""}
                type="number"
                bind:value={config.min_free_disk_percent}
                min="0"
                max="99"
                inline={true}
                suffix="%"
              />
            </div>

            <!-- The way back to the first-steps card. It's NOT a config field: it doesn't go into
                 `requiredChecks` nor into the PUT body — it's a UI preference, per browser. The
                 divider comes for free from the container's `divide-y`. `Input`/`Toggle` don't fit
                 here because both `bind:` to a `config` field, and there's no field.

                 The disabled state is about the DISMISSAL, not about the card being visible: with
                 the three items green the card doesn't show anyway, and an enabled button here
                 would be a click with no visible effect. -->
            <div class="flex flex-wrap items-center justify-between gap-3 p-4.5">
              <div class="min-w-0">
                <p class="text-[14.5px] font-bold text-heading">{T && T.onboardingRestoreLabel}</p>
                <p class="text-caption text-subtle">{T && T.onboardingRestoreHint}</p>
              </div>
              <Button
                variant="ghost"
                disabled={!onboardingHidden}
                on:click={restoreOnboarding}
              >
                {T && T.onboardingRestoreButton}
              </Button>
            </div>
          {/if}

          {#if activeGroup === "downloads"}
            <div class="p-4.5">
              <Input
                id="check_interval"
                label={T && T.labelCheckInterval || ""}
                type="number"
                bind:value={config.check_interval}
                min="1"
                required={true}
                inline={true}
                suffix="min"
              />
            </div>
            <div class="space-y-1.5 p-4.5">
              <Input
                id="max_concurrent_downloads"
                label={T && T.labelMaxConcurrent || ""}
                type="number"
                bind:value={config.max_concurrent_downloads}
                min="0"
                required={true}
                inline={true}
              />
              <p class="text-caption text-subtle">{T && T.hintMaxConcurrent}</p>
            </div>
            <div class="p-4.5">
              <Input
                id="max_episodes_per_anime"
                label={T && T.labelMaxEpisodes || ""}
                subtitle={T && T.hintMaxEpisodes || ""}
                type="number"
                bind:value={config.max_episodes_per_anime}
                min="0"
                required={true}
                inline={true}
              />
            </div>

            <!-- No divider between the switch and the amount: the absence of the divider is what
                 says the second field depends on the first. -->
            <div class="space-y-3 p-4.5">
              <Toggle
                id="delete_watched_episodes"
                bind:checked={config.delete_watched_episodes}
                label={(T && T.labelDeleteWatched) || ""}
                inline={true}
              />
              {#if config.delete_watched_episodes}
                <Input
                  id="watched_episodes_to_keep"
                  label={T && T.labelWatchedKeep || ""}
                  subtitle={T && T.hintWatchedKeep || ""}
                  type="number"
                  bind:value={config.watched_episodes_to_keep}
                  min="0"
                  inline={true}
                />
              {/if}
            </div>
          {/if}

          {#if activeGroup === "search"}
            <div class="p-4.5">
              <Input
                id="min_seeders"
                label={T && T.labelMinSeeders || ""}
                subtitle={T && T.hintMinSeeders || ""}
                type="number"
                bind:value={config.min_seeders}
                min="0"
                inline={true}
              />
            </div>

            <div class="p-4.5">
              <Input
                id="max_search_pages"
                label={T && T.labelMaxSearchPages || ""}
                subtitle={T && T.hintMaxSearchPages || ""}
                type="number"
                bind:value={config.max_search_pages}
                min="1"
                inline={true}
              />
            </div>

            <div class="p-4.5">
              <Input
                id="episode_retry_limit"
                label={T && T.labelRetryLimit || ""}
                type="number"
                bind:value={config.episode_retry_limit}
                min="0"
                required={true}
                inline={true}
              />
            </div>

            <div class="p-4.5">
              <Input
                id="max_batch_torrent_size_gb"
                label={T && T.labelMaxBatchSize || ""}
                subtitle={T && T.hintMaxBatchSize || ""}
                type="number"
                bind:value={config.max_batch_torrent_size_gb}
                min="0"
                step="0.1"
                inline={true}
                suffix="GiB"
              />
            </div>

            <div class="p-4.5">
              <Input
                id="max_episode_torrent_size_gb"
                label={T && T.labelMaxEpisodeSize || ""}
                subtitle={T && T.hintMaxEpisodeSize || ""}
                type="number"
                bind:value={config.max_episode_torrent_size_gb}
                min="0"
                step="0.1"
                inline={true}
                suffix="GiB"
              />
            </div>
          {/if}
        </div>
      </div>

      <!-- Actions. `solid` only on Save: it's the screen's only solid accent (§4.1). -->
      <div class="flex flex-wrap justify-end gap-2.5">
        <Button
          variant="ghost"
          disabled={saving}
          on:click={async () => {
            await triggerCheck();
            window.location.hash = "#/status";
          }}
        >
          {T && T.btnRunCheck}
        </Button>
        <Button type="submit" variant="solid" disabled={saving}>
          {saving ? (T && T.btnSaving) : (T && T.btnSave)}
        </Button>
      </div>
    </form>
  {/if}
</div>
