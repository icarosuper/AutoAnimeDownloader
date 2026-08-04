<script lang="ts">
  // Config — spec §9.4 (Fase 6). Re-skin puro: NENHUMA mudança de comportamento (decisão D5).
  // O botão Salvar continua sendo o único caminho de escrita — sem autosave, sem debounce —
  // porque `PUT /config` valida tudo de uma vez e faz I/O de filesystem (`Librarian.ProbePath`),
  // então salvar no meio da digitação renderia 400 a cada tecla.
  //
  // O layout novo é o índice lateral de 196px com UM grupo visível por vez. Os campos de
  // lista (anilist_usernames, excluded_lists) trocaram o par "input + botão +" pelo
  // ChipsInput do artboard 1e.
  import { onMount } from "svelte";
  import { Check } from "@lucide/svelte";
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

  type GroupId = "anilist" | "downloads" | "automation" | "filters";

  $: T = $locale && {
    title: m.config_title(),
    subtitle: m.config_subtitle(),
    missingBanner: m.config_missing_banner(),
    loading: m.config_loading(),
    navLabel: m.config_nav_label(),
    requiredLegend: m.config_required_legend(),
    groupMissing: m.config_group_missing(),
    chipsPlaceholder: m.config_chips_placeholder(),
    sectionAnilist: m.config_section_anilist(),
    sectionDownloads: m.config_section_downloads(),
    sectionAutomation: m.config_section_automation(),
    sectionFilters: m.config_section_filters(),
    labelUsername: m.config_label_username(),
    hintAnilistUsernames: m.config_hint_anilist_usernames(),
    labelCompletedPath: m.config_label_completed_path(),
    hintCompletedPath: m.config_hint_completed_path(),
    labelDeleteWatched: m.config_label_delete_watched(),
    labelWatchedKeep: m.config_label_watched_keep(),
    hintWatchedKeep: m.config_hint_watched_keep(),
    labelCheckInterval: m.config_label_check_interval(),
    labelMaxEpisodes: m.config_label_max_episodes(),
    labelRetryLimit: m.config_label_retry_limit(),
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
    btnRunCheck: m.config_btn_run_check(),
    btnSave: m.config_btn_save(),
    btnSaving: m.config_btn_saving(),
  }

  $: groups = ($locale && [
    { id: "anilist" as GroupId, label: m.config_section_anilist() },
    { id: "downloads" as GroupId, label: m.config_section_downloads() },
    { id: "automation" as GroupId, label: m.config_section_automation() },
    { id: "filters" as GroupId, label: m.config_section_filters() },
  ]) || [];

  let activeGroup: GroupId = "anilist";

  const ALL_STATUSES = ["CURRENT", "REPEATING", "PLANNING", "PAUSED", "DROPPED", "COMPLETED"];
  const ALL_MEDIA_STATUSES = ["RELEASING", "FINISHED", "CANCELLED", "HIATUS"];

  let config: Config = {
    anilist_usernames: [],
    completed_anime_path: "",
    check_interval: 10,
    max_episodes_per_anime: 12,
    episode_retry_limit: 5,
    max_concurrent_downloads: 3,
    delete_watched_episodes: true,
    watched_episodes_to_keep: 0,
    excluded_lists: [],
    rename_files_for_jellyfin: false,
    download_statuses: ["CURRENT", "REPEATING"],
    download_media_statuses: ["RELEASING", "FINISHED"],
    delete_statuses: [],
    notifications: { webhooks: [] },
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

  // Um status não pode estar em "baixar" e "deletar" ao mesmo tempo — ligar um sempre desliga
  // o outro. Regra pré-existente, preservada verbatim.
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
      config = { ...data, anilist_usernames: data.anilist_usernames ?? [] };
      if (config.anilist_username && (config.anilist_usernames ?? []).length === 0) {
        config.anilist_usernames = [config.anilist_username];
      }
      if (!config.notifications) config.notifications = { webhooks: [] };
      if (!Array.isArray(config.notifications.webhooks)) config.notifications.webhooks = [];
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.config_error_load());
    } finally {
      loading = false;
    }
  }

  /**
   * Mesmas seis validações de antes, na mesma ordem — só que cada uma agora sabe em que grupo
   * mora o campo que ela reprova. Isso é exigência do layout novo, não uma regra nova: com um
   * grupo visível por vez, um toast dizendo "pasta é obrigatória" enquanto o usuário olha para
   * "Automação" não teria como ser acionável. A validação que falha traz o grupo dela à tela.
   *
   * Elas viraram uma LISTA (antes era uma cadeia de `if`) porque a tela agora as usa para duas
   * coisas: o toast do Salvar, como sempre, e a marca de "falta preencher" no índice lateral.
   * Reescrever as condições no segundo lugar deixaria a marca mentir na primeira vez que uma
   * regra mudasse — o índice tem de dizer exatamente o que barra o Salvar, nada mais.
   *
   * `message` guarda a REFERÊNCIA da função do paraglide, não o texto: assim a mensagem é
   * resolvida no idioma vigente no momento do clique, como era quando cada `if` a chamava.
   */
  $: requiredChecks = [
    {
      group: "anilist" as GroupId,
      ok: (config.anilist_usernames ?? []).length > 0,
      message: m.config_val_username,
    },
    {
      group: "downloads" as GroupId,
      ok: !!config.completed_anime_path?.trim(),
      message: m.config_val_completed_path,
    },
    {
      group: "automation" as GroupId,
      ok: config.check_interval > 0,
      message: m.config_val_interval,
    },
    {
      group: "automation" as GroupId,
      ok: config.max_episodes_per_anime > 0,
      message: m.config_val_max_episodes,
    },
    {
      group: "automation" as GroupId,
      ok: config.episode_retry_limit >= 0,
      message: m.config_val_retry,
    },
    {
      group: "automation" as GroupId,
      ok: config.max_concurrent_downloads >= 0,
      message: m.config_val_max_concurrent,
    },
    {
      group: "downloads" as GroupId,
      ok: !(config.delete_watched_episodes && config.watched_episodes_to_keep < 0),
      message: m.config_val_watched_keep,
    },
  ];

  /** Grupos com alguma pendência — alimenta o ponto no índice lateral. */
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
      <!-- Legenda do asterisco. Sem ela o `*` é convenção implícita; com um grupo por vez, vale
           dizer em texto o que a marca significa. -->
      <p class="text-caption text-subtle">
        <span class="text-danger" aria-hidden="true">*</span>
        {T && T.requiredLegend}
      </p>

      <div class="grid gap-3.5 md:grid-cols-[196px_1fr] md:items-start">
        <!-- Índice: coluna de 196px no desktop; no mobile vira faixa rolável horizontal, com
             `shrink-0` nos itens para a faixa rolar em vez de espremer os rótulos.
             O ponto de pendência entra AQUI porque só um grupo fica na tela por vez: o asterisco
             no campo resolve o grupo aberto, e este ponto é o que conta ao usuário que ainda
             falta algo nos outros três. O texto sr-only entra no nome acessível do botão
             ("Anilist, configuração obrigatória faltando"); o ponto em si é decorativo. -->
        <nav
          aria-label={(T && T.navLabel) || ""}
          class="flex gap-1 overflow-x-auto md:flex-col md:overflow-x-visible"
        >
          {#each groups as group (group.id)}
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
        </nav>

        <!-- `divide-y` desenha os divisores de 1px ENTRE os campos, sem borda sobrando na
             primeira nem na última linha do card. -->
        <div class="divide-y divide-divider rounded-card border border-default bg-card">
          {#if activeGroup === "anilist"}
            <div class="p-4.5">
              <ChipsInput
                id="anilist_usernames"
                bind:values={config.anilist_usernames}
                label={(T && T.labelUsername) || ""}
                required={true}
                hint={(T && T.hintAnilistUsernames) || ""}
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

          {#if activeGroup === "downloads"}
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

            <div class="space-y-3 p-4.5">
              <Toggle
                id="delete_watched_episodes"
                bind:checked={config.delete_watched_episodes}
                label={(T && T.labelDeleteWatched) || ""}
              />
              {#if config.delete_watched_episodes}
                <div class="pl-11">
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

            <div class="space-y-1.5 p-4.5">
              <Toggle
                id="rename_files_for_jellyfin"
                bind:checked={config.rename_files_for_jellyfin}
                label={(T && T.labelRenameJellyfin) || ""}
              />
              {#if config.rename_files_for_jellyfin}
                <p class="pl-11 text-caption text-subtle">{T && T.hintRenameJellyfin}</p>
              {/if}
            </div>
          {/if}

          {#if activeGroup === "automation"}
            <div class="p-4.5">
              <Input
                id="check_interval"
                label={T && T.labelCheckInterval || ""}
                type="number"
                bind:value={config.check_interval}
                min="1"
                required={true}
              />
            </div>
            <div class="p-4.5">
              <Input
                id="max_episodes_per_anime"
                label={T && T.labelMaxEpisodes || ""}
                type="number"
                bind:value={config.max_episodes_per_anime}
                min="1"
                required={true}
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
              />
              <p class="text-caption text-subtle">{T && T.hintMaxConcurrent}</p>
            </div>
          {/if}

          {#if activeGroup === "filters"}
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
          {/if}
        </div>
      </div>

      <!-- Ações. `solid` só no Salvar: é o único acento sólido da tela (§4.1). -->
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
